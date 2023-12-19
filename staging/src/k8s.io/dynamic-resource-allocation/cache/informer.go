/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamiclister"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// GenericListerFactory is similar to a normal SharedInformerFactory.
// The difference is that it can look up resources based on
// group/kind and that each individual informer is only kept running
// as long as someone needs it. For the sake of simplicity, only
// listing and getting items is supported.
//
// Background activities are associated with the context that was
// passed in when starting the factory. When a client register to
// get access to an individual listers, it provides a separate context
// that identifies the client.
type GenericListerFactory struct {
	wg         sync.WaitGroup
	client     dynamic.Interface
	restMapper RESTMapper
	ctx        context.Context
	logger     klog.Logger
	cancel     func()

	mutex     sync.Mutex
	informers map[schema.GroupKind]*informerForType
}

type informerForType struct {
	gk       schema.GroupKind
	contexts sets.Set[context.Context]
	cancel   func()
	lister   cache.GenericLister
}

// RESTMapper is the minimal set of methods needed by [NewGenericInformerFactory].
// [k8s.io/client-go/restmapper.DeferredDiscoveryRESTMapper] can be used as
// implementation.
type RESTMapper interface {
	RESTMapping(gk schema.GroupKind, versions ...string) (m *meta.RESTMapping, err error)
	Reset()
}

// StartGenericInformerFactory constructs a new factory.
// Goroutines stop when the context gets canceled or Shutdown
// is called, whatever happens sooner.
func StartGenericListerFactory(ctx context.Context, client dynamic.Interface, restMapper RESTMapper) *GenericListerFactory {
	f := &GenericListerFactory{
		client:     client,
		restMapper: restMapper,
		logger:     klog.LoggerWithName(klog.FromContext(ctx), "GenericListerFactory"),
		informers:  make(map[schema.GroupKind]*informerForType),
	}
	// New context needed for explicit Shutdown call.
	f.ctx, f.cancel = context.WithCancel(ctx)
	return f
}

// Shutdown stops all goroutines and blocks until they have stopped.
func (f *GenericListerFactory) Shutdown() {
	f.cancel()
	f.wg.Wait()
}

// ForType ensures that there is an informer for the given type and keeps it
// running until the context gets canceled. It handles errors like not being
// able to look up the corresponding resource internally. While there are such
// errors or while still receiving data the informer's store will be
// incomplete. Objects in the store are of type [unstructured.Unstructured].
//
// TODO: extend to GroupVersionKind, same in class: the field name may only
// be valid in a particular version.
func (f *GenericListerFactory) ForType(ctx context.Context, gk schema.GroupKind) cache.GenericLister {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	informer := f.informers[gk]
	if informer != nil {
		// Already running, just add another context for future calls.
		f.addClient(ctx, informer)
		return informer.lister
	}

	// The informer will run until shutdown (f.ctx) or until no
	// longer needed (informerCtx).
	informerCtx, cancel := context.WithCancel(f.ctx)
	informer = &informerForType{
		gk:       gk,
		contexts: sets.New[context.Context](),
		cancel:   cancel,
	}

	// This code is similar to https://github.com/kubernetes/client-go/blob/fb8b7346aacefea5ee2ab2e234afc4451c90c435/dynamic/dynamicinformer/informer.go#L146
	// The difference is that we don't know the resource in advance.
	indexers := cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
	indexInformer := cache.NewSharedIndexInformerWithOptions(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return f.list(informer, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return f.watch(informer, options)
			},
		},
		&unstructured.Unstructured{},
		cache.SharedIndexInformerOptions{
			ResyncPeriod:      time.Hour, /* TODO: how long? */
			Indexers:          indexers,
			ObjectDescription: gk.String(),
		},
	)
	lister := dynamiclister.NewRuntimeObjectShim(dynamiclister.New(indexInformer.GetIndexer(), schema.GroupVersionResource{Group: gk.Group /* only used for errors, so doesn't need to be complete */}))
	informer.lister = lister
	f.informers[gk] = informer
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		defer f.logger.V(3).Info("Informer stoppped", "type", gk)
		f.logger.V(3).Info("Informer starting", "type", gk)
		indexInformer.Run(informerCtx.Done())
	}()

	f.addClient(ctx, informer)
	return informer.lister
}

func (f *GenericListerFactory) addClient(ctx context.Context, informer *informerForType) {
	logger := klog.FromContext(ctx)
	// TODO: https://github.com/kubernetes/kubernetes/issues/121998
	logger = klog.LoggerWithValues(logger, "type", informer.gk)
	logger = klog.LoggerWithName(logger, "GenericListerFactory")
	logger.V(3).Info("Adding client to generic lister", "type", informer.gk)
	informer.contexts.Insert(ctx)
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		select {
		case <-ctx.Done():
			logger.V(3).Info("Removing client from generic lister, it has unregistered", "type", informer.gk)
		case <-f.ctx.Done():
			logger.V(3).Info("Removing client from generic lister, shutting down", "type", informer.gk)
		}
		f.mutex.Lock()
		defer f.mutex.Unlock()
		informer.contexts.Delete(ctx)
		if informer.contexts.Len() == 0 {
			// Last client is gone, stop informer.
			informer.cancel()
		}
	}()
}

func (f *GenericListerFactory) list(informer *informerForType, options metav1.ListOptions) (runtime.Object, error) {
	resource, err := f.restMapping(informer.gk)
	if err != nil {
		return nil, err
	}
	f.logger.V(5).Info("Listing resources", "type", informer.gk, "resource", resource)
	obj, err := f.client.Resource(resource).List(f.ctx, options)
	if err != nil {
		// Could be out-dated rest mapping. Get a new one next time.
		f.restMapper.Reset()
	}
	return obj, err
}

func (f *GenericListerFactory) watch(informer *informerForType, options metav1.ListOptions) (watch.Interface, error) {
	resource, err := f.restMapping(informer.gk)
	if err != nil {
		return nil, err
	}
	f.logger.V(5).Info("Watching resources", "type", informer.gk, "resource", resource)
	w, err := f.client.Resource(resource).Watch(f.ctx, options)
	if err != nil {
		// Could be out-dated rest mapping. Get a new one next time.
		f.restMapper.Reset()
	}
	return w, err
}

func (f *GenericListerFactory) restMapping(gk schema.GroupKind) (schema.GroupVersionResource, error) {
	restMapping, err := f.restMapper.RESTMapping(gk)
	if err != nil {
		// Retry once.
		f.restMapper.Reset()
		restMapping, err = f.restMapper.RESTMapping(gk)
	}
	if err != nil {
		// TODO: should this be surfaced in clients?
		return schema.GroupVersionResource{}, fmt.Errorf("failed to look up REST mapping for %q: %v", gk, err)
	}
	return restMapping.Resource, nil
}
