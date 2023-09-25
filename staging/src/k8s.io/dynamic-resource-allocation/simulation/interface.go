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

	v1 "k8s.io/api/core/v1"
	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

// Plugin is used to register a plugin.
type Plugin interface {
	// Activate will get called to prepared the plugin for usage.
	Activate(ctx context.Context, client kubernetes.Interface, informerFactory informers.SharedInformerFactory) (ActivePlugin, error)
}

// ActivePlugin is a plugin which is ready to start a simulation.
type ActivePlugin interface {
	// Start will get called at the start of a simulation. The plugin must
	// capture the current cluster state.
	Start(ctx context.Context) (StartedPlugin, error)
}

// StartedPlugin is a plugin which encapsulates a certain cluster state and
// can make changes to it.
type StartedPlugin interface {
	// Clone must create a new, independent copy of the current state.
	// This must be fast and cannot fail. If it has to do some long-running
	// operation, then it must do that in a new goroutine and check the
	// result when some method is called in the returned instance.
	Clone() StartedPlugin

	// NodeIsSuitable checks whether a claim could be allocated for
	// a pod such that it will be available on the node.
	NodeIsSuitable(ctx context.Context, pod *v1.Pod, claim *resourcev1alpha2.ResourceClaim, node *v1.Node) (bool, error)

	// Allocate must adapt the cluster state as if the claim
	// had been allocated for use on the selected node and return
	// the result for the claim. It must not modify the claim,
	// that will be done by the caller.
	Allocate(ctx context.Context, claim *resourcev1alpha2.ResourceClaim, node *v1.Node) (*resourcev1alpha2.AllocationResult, error)

	// Deallocate must adapt the cluster state as if the claim
	// had been deallocated. It must not modify the claim,
	// that will be done by the caller.
	Deallocate(ctx context.Context, claim *resourcev1alpha2.ResourceClaim) error
}
