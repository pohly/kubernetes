/*
Copyright 2022 The Kubernetes Authors.

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

package dynamicresources

import (
	"context"

	v1 "k8s.io/api/core/v1"
	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
	"k8s.io/dynamic-resource-allocation/simulation"
)

// unlimitedPlugin doesn't have any state and therefore implements
// all required simulation interfaces in a single type.
// It mimicks a driver where resources are always available,
// accessible on all nodes, and not shared.
type unlimitedPlugin struct{}

func (p unlimitedPlugin) Start(ctx context.Context) (simulation.StartedPlugin, error) {
	return p, nil
}

func (p unlimitedPlugin) Clone() simulation.StartedPlugin {
	return p
}

func (p unlimitedPlugin) NodeIsSuitable(ctx context.Context, pod *v1.Pod, claim *resourcev1alpha2.ResourceClaim, node *v1.Node) (bool, error) {
	return true, nil
}

func (p unlimitedPlugin) Allocate(ctx context.Context, claim *resourcev1alpha2.ResourceClaim, node *v1.Node) (*resourcev1alpha2.AllocationResult, error) {
	return &resourcev1alpha2.AllocationResult{}, nil
}

func (p unlimitedPlugin) Deallocate(ctx context.Context, claim *resourcev1alpha2.ResourceClaim) error {
	return nil
}
