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

// Package internal is used to define the state which tracks resource capacity
// and usage because then the types and fields can be exported, which enables
// using deepcopy-gen and JSON serialization (useful for testing).
//
// +k8s:deepcopy-gen=package
package internal

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +k8s:deepcopy-gen=true
type State struct {
	// Resources maps node name to resource usage on that node.
	Resources map[string]NodeResources

	// AllocatedClaims maps the UID of an allocated claim to
	// the resources allocated for it.
	Claims map[types.UID]ClaimResources
}

// +k8s:deepcopy-gen=true
type NodeResources struct {
	// PerDriver maps a driver name to resources managed by that driver.
	PerDriver map[string]DriverResources
}

// +k8s:deepcopy-gen=true
type DriverResources struct {
	// PerInstance maps the UID of each hardware instance to information
	// published by the driver and how its used.
	PerInstance map[types.UID]InstanceResources
}

// +k8s:deepcopy-gen=true
type InstanceResources struct {
	// ObjectMeta contains the Name and Labels as reported by the driver.
	metav1.ObjectMeta
	// Capacity is the total amount of items available.
	Capacity int64
	// Allocated is how much of those are in use.
	Allocated int64
}

// +k8s:deepcopy-gen=true
type ClaimResources struct {
	// Name and Namespace are stored for logging.
	Name, Namespace string

	NodeName   string
	DriverName string
	InstanceID types.UID
	Count      int64
}
