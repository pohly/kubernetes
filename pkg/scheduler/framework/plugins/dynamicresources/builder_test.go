/*
Copyright 2021 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	resourcev1alpha1 "k8s.io/api/resource/v1alpha1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	resourcev1alpha1ac "k8s.io/client-go/applyconfigurations/resource/v1alpha1"
)

func toNode(in *corev1ac.NodeApplyConfiguration) *corev1.Node {
	out := &corev1.Node{}
	convert(in, out)
	return out
}

func toPod(in *corev1ac.PodApplyConfiguration) *corev1.Pod {
	out := &corev1.Pod{}
	convert(in, out)
	return out
}

func toPodScheduling(in *resourcev1alpha1ac.PodSchedulingApplyConfiguration) *resourcev1alpha1.PodScheduling {
	out := &resourcev1alpha1.PodScheduling{}
	convert(in, out)
	return out
}

func toResourceClaim(in *resourcev1alpha1ac.ResourceClaimApplyConfiguration) *resourcev1alpha1.ResourceClaim {
	out := &resourcev1alpha1.ResourceClaim{}
	convert(in, out)
	return out
}

func toResourceClass(in *resourcev1alpha1ac.ResourceClassApplyConfiguration) *resourcev1alpha1.ResourceClass {
	out := &resourcev1alpha1.ResourceClass{}
	convert(in, out)
	return out
}

// convert can convert between an apply configuration and the corresponding object
// (in both directions!) by first marshaling to and and from JSON. That works because
// both types have the same JSON encoding.
func convert(in, out interface{}) {
	buffer, err := json.Marshal(in)
	if err != nil {
		panic(fmt.Errorf("encoding %T as JSON: %v", in, err))
	}
	if err := json.Unmarshal(buffer, out); err != nil {
		panic(fmt.Errorf("decoding %T from JSON: %v", out, err))
	}
}
