/*
Copyright 2014 The Kubernetes Authors.

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

// Package informers creates informers in a shared informer factory which don't
// fail when the resource API group isn't enabled. Instead, syncing succeeds
// with no objects. This corresponds to
// [k8s.io/client-go/informers/resource/v1beta1] and can server as a drop-in
// replacement.
//
// The informers remain inactive once they have discovered that the apiserver
// does not serve the resources they need, even if the resyncing is enabled.
// This is done to avoid any potential issues that could arise when returning
// a fake empty list at one point and later some real list or watch event.
//
// Should the API group get enabled in the apiserver later, the components must
// be restarted to use the API group.
//
// For the sake of simplicity, restricting informers to namespaces and tweaking
// list options are not supported. None of those are currently needed.
// In contrast to the normal SharedInformerFactory, a context is supported for:
//   - contextual logging (in particular, a message that the informers get
//     deactivated)
//   - the client-go calls
//
// Supporting a context through the SharedInformerFactory would be better because
// the context used when constructing informers is shared among all users of
// the shared informer. This is okay the way these functions are used for DRA
// (those contexts don't get cancelled), but it is not okay in general.
package informers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	resourceapi "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	resourceinformers "k8s.io/client-go/informers/resource/v1beta1"
	"k8s.io/client-go/kubernetes"
	resourcelisters "k8s.io/client-go/listers/resource/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func ResourceClaims(ctx context.Context, factory informers.SharedInformerFactory) resourceinformers.ResourceClaimInformer {
	return &resourceClaimInformer{ctx: ctx, factory: factory}
}

type resourceClaimInformer struct {
	ctx     context.Context
	factory informers.SharedInformerFactory
}

func (f *resourceClaimInformer) defaultInformer(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return newInformer(f.ctx, resyncPeriod, &resourceapi.ResourceClaim{}, &resourceapi.ResourceClaimList{},
		cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.ResourceV1beta1().ResourceClaims("").List(f.ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.ResourceV1beta1().ResourceClaims("").Watch(f.ctx, options)
			},
		},
	)
}

func (f *resourceClaimInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&resourceapi.ResourceClaim{}, f.defaultInformer)
}

func (f *resourceClaimInformer) Lister() resourcelisters.ResourceClaimLister {
	return resourcelisters.NewResourceClaimLister(f.Informer().GetIndexer())
}

func ResourceClaimTemplates(ctx context.Context, factory informers.SharedInformerFactory) resourceinformers.ResourceClaimTemplateInformer {
	return &resourceClaimTemplateInformer{ctx: ctx, factory: factory}
}

type resourceClaimTemplateInformer struct {
	ctx     context.Context
	factory informers.SharedInformerFactory
}

func (f *resourceClaimTemplateInformer) defaultInformer(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return newInformer(f.ctx, resyncPeriod, &resourceapi.ResourceClaimTemplate{}, &resourceapi.ResourceClaimTemplateList{},
		cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.ResourceV1beta1().ResourceClaimTemplates("").List(f.ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.ResourceV1beta1().ResourceClaimTemplates("").Watch(f.ctx, options)
			},
		},
	)
}

func (f *resourceClaimTemplateInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&resourceapi.ResourceClaimTemplate{}, f.defaultInformer)
}

func (f *resourceClaimTemplateInformer) Lister() resourcelisters.ResourceClaimTemplateLister {
	return resourcelisters.NewResourceClaimTemplateLister(f.Informer().GetIndexer())
}

func DeviceClassInformers(ctx context.Context, factory informers.SharedInformerFactory) resourceinformers.DeviceClassInformer {
	return &deviceClassInformer{ctx: ctx, factory: factory}
}

type deviceClassInformer struct {
	ctx     context.Context
	factory informers.SharedInformerFactory
}

func (f *deviceClassInformer) defaultInformer(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return newInformer(f.ctx, resyncPeriod, &resourceapi.DeviceClass{}, &resourceapi.DeviceClassList{},
		cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.ResourceV1beta1().DeviceClasses().List(f.ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.ResourceV1beta1().DeviceClasses().Watch(f.ctx, options)
			},
		},
	)
}

func (f *deviceClassInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&resourceapi.DeviceClass{}, f.defaultInformer)
}

func (f *deviceClassInformer) Lister() resourcelisters.DeviceClassLister {
	return resourcelisters.NewDeviceClassLister(f.Informer().GetIndexer())
}

func ResourceSlices(ctx context.Context, factory informers.SharedInformerFactory) resourceinformers.ResourceSliceInformer {
	return &resourceSliceInformer{ctx: ctx, factory: factory}
}

type resourceSliceInformer struct {
	ctx     context.Context
	factory informers.SharedInformerFactory
}

func (f *resourceSliceInformer) defaultInformer(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return newInformer(f.ctx, resyncPeriod, &resourceapi.ResourceSlice{}, &resourceapi.ResourceSliceList{},
		cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.ResourceV1beta1().ResourceSlices().List(f.ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.ResourceV1beta1().ResourceSlices().Watch(f.ctx, options)
			},
		},
	)
}

func (f *resourceSliceInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&resourceapi.ResourceSlice{}, f.defaultInformer)
}

func (f *resourceSliceInformer) Lister() resourcelisters.ResourceSliceLister {
	return resourcelisters.NewResourceSliceLister(f.Informer().GetIndexer())
}

func newInformer(ctx context.Context, resyncPeriod time.Duration, exampleObject, exampleList runtime.Object, lw cache.ListWatch) cache.SharedIndexInformer {
	olw := &optionalListWatch{
		ctx:           ctx,
		base:          lw,
		exampleObject: exampleObject,
		exampleList:   exampleList,
	}

	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc:  olw.list,
			WatchFunc: olw.watch,
		},
		exampleObject,
		resyncPeriod,
		cache.Indexers{},
	)
}

type optionalListWatch struct {
	ctx           context.Context
	base          cache.ListWatch
	exampleObject runtime.Object
	exampleList   runtime.Object
	notFound      int32 // atomic, 0 if found or unknown, 1 if resource not found
}

// wasNotFound records that the resource was not found. The first time that happens
// a log message gets emitted.
func (olw *optionalListWatch) wasNotFound() {
	oldNotFound := atomic.SwapInt32(&olw.notFound, 1)
	if oldNotFound == 0 {
		klog.FromContext(olw.ctx).Info("Resource not found by informer, proceeding without it (restart required if it becomes available later)", "type", fmt.Sprintf("%T", olw.exampleObject))
	}
}

func (olw *optionalListWatch) list(options metav1.ListOptions) (runtime.Object, error) {
	notFound := atomic.LoadInt32(&olw.notFound)
	if notFound != 0 {
		// Resource not found earlier, don't try again.
		return olw.exampleList.DeepCopyObject(), nil
	}

	obj, err := olw.base.List(options)
	if apierrors.IsNotFound(err) {
		// Resource does not exist. Keep going with an empty list
		// and remember this.
		olw.wasNotFound()
		return olw.exampleList.DeepCopyObject(), nil
	}
	return obj, err
}

func (olw *optionalListWatch) watch(options metav1.ListOptions) (watch.Interface, error) {
	notFound := atomic.LoadInt32(&olw.notFound)
	if notFound != 0 {
		// Resource not found earlier, don't try again.
		return &nopWatch{
			resultChan: make(chan watch.Event),
		}, nil
	}

	w, err := olw.base.Watch(options)
	if apierrors.IsNotFound(err) {
		// Resource does not exist. Keep going with a fake watch
		// and remember this.
		olw.wasNotFound()
		return &nopWatch{
			resultChan: make(chan watch.Event),
		}, nil
	}
	return w, err
}

type nopWatch struct {
	mutex      sync.Mutex
	resultChan chan watch.Event
	closed     bool
}

func (n *nopWatch) Stop() {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if !n.closed {
		close(n.resultChan)
		n.closed = true
	}
}

func (n *nopWatch) ResultChan() <-chan watch.Event {
	return n.resultChan
}
