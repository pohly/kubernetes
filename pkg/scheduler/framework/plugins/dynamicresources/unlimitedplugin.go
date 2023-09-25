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
	"k8s.io/dynamic-resource-allocation/builtincontroller"
)

// unlimitedController simulates a DRA driver which has no per-node limitations
// and thus can always allocate a claim for a node. May only be used for
// simulations.
type unlimitedController struct{}

var _ builtincontroller.ClaimController = unlimitedController{}

func (p unlimitedController) ControllerName() string {
	return "unlimited-controller.k8s.io"
}

func (p unlimitedController) NodeIsSuitable(ctx context.Context, pod *v1.Pod, node *v1.Node) (bool, error) {
	return true, nil
}

func (p unlimitedController) Allocate(ctx context.Context, nodeName string) (string, *resourcev1alpha2.AllocationResult, error) {
	return p.ControllerName(), &resourcev1alpha2.AllocationResult{}, nil
}

func (p unlimitedController) Deallocate(ctx context.Context) {
}
