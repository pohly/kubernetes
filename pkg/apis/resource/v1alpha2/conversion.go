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

package v1alpha2

import (
	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
	conversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	resource "k8s.io/kubernetes/pkg/apis/resource"
)

func addConversionFuncs(scheme *runtime.Scheme) error {
	return nil
}

func Convert_resource_PodSchedulingContextSpec_To_v1alpha2_PodSchedulingContextSpec(in *resource.PodSchedulingContextSpec, out *resourcev1alpha2.PodSchedulingContextSpec, s conversion.Scope) error {
	if err := autoConvert_resource_PodSchedulingContextSpec_To_v1alpha2_PodSchedulingContextSpec(in, out, s); err != nil {
		return err
	}
	out.PotentialNodes = in.PotentialNodes.UnsortedList()
	return nil
}

func Convert_v1alpha2_PodSchedulingContextSpec_To_resource_PodSchedulingContextSpec(in *resourcev1alpha2.PodSchedulingContextSpec, out *resource.PodSchedulingContextSpec, s conversion.Scope) error {
	if err := autoConvert_v1alpha2_PodSchedulingContextSpec_To_resource_PodSchedulingContextSpec(in, out, s); err != nil {
		return err
	}
	out.PotentialNodes = sets.New(in.PotentialNodes...)
	return nil
}

func Convert_resource_ResourceClaimSchedulingStatus_To_v1alpha2_ResourceClaimSchedulingStatus(in *resource.ResourceClaimSchedulingStatus, out *resourcev1alpha2.ResourceClaimSchedulingStatus, s conversion.Scope) error {
	if err := autoConvert_resource_ResourceClaimSchedulingStatus_To_v1alpha2_ResourceClaimSchedulingStatus(in, out, s); err != nil {
		return err
	}
	out.UnsuitableNodes = in.UnsuitableNodes.UnsortedList()
	return nil
}

func Convert_v1alpha2_ResourceClaimSchedulingStatus_To_resource_ResourceClaimSchedulingStatus(in *resourcev1alpha2.ResourceClaimSchedulingStatus, out *resource.ResourceClaimSchedulingStatus, s conversion.Scope) error {
	if err := autoConvert_v1alpha2_ResourceClaimSchedulingStatus_To_resource_ResourceClaimSchedulingStatus(in, out, s); err != nil {
		return err
	}
	out.UnsuitableNodes = sets.New(in.UnsuitableNodes...)
	return nil
}
