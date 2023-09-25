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

package simulation

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

// Registry stores all known plugins which can simulate claim allocation.
// It is thread-safe.
var Registry registry

type registry struct {
	mutex   sync.Mutex
	plugins map[PluginName]Plugin
}

// PluginName is a special type that is used to look up plugins for a claim.
// For now it must be the same as the driver name in the resource class of a
// claim.
type PluginName string

// Add adds or overwrites the plugin for a certain name.
func (r *registry) Add(name PluginName, plugin Plugin) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.plugins == nil {
		r.plugins = make(map[PluginName]Plugin)
	}
	r.plugins[name] = plugin
}

// Remove removes the plugin for a certain name, It is okay
// if nothing is registered under the name.
func (r *registry) Removed(name PluginName) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.plugins, name)
}

// Lookup returns the registered plugin or nil if none is found.
func (r *registry) Lookup(name PluginName) Plugin {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.plugins[name]
}

// Activate calls Activate for each registered plugin and returns the results.
func (r *registry) Activate(ctx context.Context, client kubernetes.Interface, informerFactory informers.SharedInformerFactory) (map[PluginName]ActivePlugin, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	plugins := make(map[PluginName]ActivePlugin, len(r.plugins))
	for name, plugin := range r.plugins {
		plugin, err := plugin.Activate(ctx, client, informerFactory)
		if err != nil {
			return nil, fmt.Errorf("activating %s: %v", name, err)
		}
		plugins[name] = plugin
	}
	return plugins, nil
}
