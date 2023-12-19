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
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// ResourceEventHandlerFuncs is a typed variant of [cache.ResourceEventHandlerFuncs].
// Objects get cast to the expected type before invoking the provided callbacks.
// Unexpected objects are silently ignored, which should never happen.
// [cache.DeletedFinalStateUnknown] is handled by extracting the object contained in it.
type ResourceEventHandlerFuncs[T interface{}] struct {
	AddFunc    func(obj T)
	UpdateFunc func(oldObj, newObj T)
	DeleteFunc func(obj T)
}

var _ cache.ResourceEventHandler = &ResourceEventHandlerFuncs[int]{}

// OnAdd calls AddFunc if it's not nil.
func (r ResourceEventHandlerFuncs[T]) OnAdd(obj interface{}, isInInitialList bool) {
	if r.AddFunc != nil {
		obj, ok := obj.(T)
		if !ok {
			return
		}
		r.AddFunc(obj)
	}
}

// OnUpdate calls UpdateFunc if it's not nil.
func (r ResourceEventHandlerFuncs[T]) OnUpdate(oldObj, newObj interface{}) {
	if r.UpdateFunc != nil {
		oldObj, ok := oldObj.(T)
		if !ok {
			return
		}
		newObj, ok := newObj.(T)
		if !ok {
			return
		}
		r.UpdateFunc(oldObj, newObj)
	}
}

// OnDelete calls DeleteFunc if it's not nil.
func (r ResourceEventHandlerFuncs[T]) OnDelete(obj interface{}) {
	if r.DeleteFunc != nil {
		if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			obj = unknown.Obj
		}
		obj, ok := obj.(T)
		if !ok {
			return
		}
		r.DeleteFunc(obj)
	}
}

// NewResourceEventHandlerFuncs creates a [ResourceEventHandlerFuncs] where
// the provided logger is passed through to all non-nil callbacks.
func NewResourceEventHandlerFuncs[T interface{}](logger klog.Logger, addFunc func(logger klog.Logger, obj T), updateFunc func(logger klog.Logger, oldObj, newObj T), deleteFunc func(logger klog.Logger, obj T)) ResourceEventHandlerFuncs[T] {
	var r ResourceEventHandlerFuncs[T]
	if addFunc != nil {
		r.AddFunc = func(obj T) {
			addFunc(logger, obj)
		}
	}
	if updateFunc != nil {
		r.UpdateFunc = func(oldObj, newObj T) {
			updateFunc(logger, oldObj, newObj)
		}
	}
	if deleteFunc != nil {
		r.DeleteFunc = func(obj T) {
			deleteFunc(logger, obj)
		}
	}
	return r
}
