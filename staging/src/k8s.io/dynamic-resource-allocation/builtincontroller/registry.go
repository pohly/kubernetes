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

package builtincontroller

import (
	"context"
	"sync"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	dracache "k8s.io/dynamic-resource-allocation/cache"
)

// DefaultRegistry stores all controllers which were registered during program
// initialization.
var DefaultRegistry Registry

// Registry is a thread-safe collection of inactive controllers.
type Registry struct {
	mutex       sync.Mutex
	controllers []Controller
}

// Add adds a controller.
func (r *Registry) Add(controller Controller) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.controllers = append(r.controllers, controller)
}

// Clone returns a new registry with the same controllers as the old one.
func (r *Registry) Clone() *Registry {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	n := len(r.controllers)
	clone := &Registry{
		// Three-index slicing prevents overwriting elements
		// in the original slice.
		controllers: r.controllers[0:n:n],
	}
	return clone
}

// Activate calls Activate for each registered controller and returns the results.
func (r *Registry) Activate(ctx context.Context, client kubernetes.Interface, informerFactory informers.SharedInformerFactory, genericInformerFactory *dracache.GenericListerFactory) ([]ActiveController, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	activeControllers := make([]ActiveController, 0, len(r.controllers))
	for _, controller := range r.controllers {
		activeController, err := controller.Activate(ctx, client, informerFactory, genericInformerFactory)
		if err != nil {
			return nil, err
		}
		activeControllers = append(activeControllers, activeController)
	}
	return activeControllers, nil
}
