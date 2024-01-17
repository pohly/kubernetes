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

package counter

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Capacity defines how much of certain items are available respectively
// required.
type Capacity struct {
	// ObjectMeta may be set when publishing capacity, but is not required.
	// The labels specified here can be used to filter when requesting
	// capacity.
	metav1.ObjectMeta

	// TypeMeta must have non-empty kind and apiVersion.
	metav1.TypeMeta

	// Count is the total number of available items per node.
	Count int64
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Parameters defines how much and which resources are needed for a claim.
type Parameters struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	metav1.TypeMeta   `json:",inline"`

	// Count is the required number of items.
	Count int64 `json:"count"`

	// Label selector for the resource instance. The labels that are
	// matched against are the labels of the [Capacity] instances published
	// by the driver. An empty selector matches all resource instances.
	//
	// In addition to the "In", "NotIn", "Exists", "DoesNotExist"
	// operators from [k8s.io/apimachinery/pkg/apis/meta/v1.LabelSelectorOperator],
	// additional operators from [k8s.io/dynamic-resource-allocation/apis/meta/v1.LabelSelectorOperator]
	// are also supported:
	//
	// - "Version>=", "Version>", "Version<=", "Version<", "Version=", "Version!=":
	//   the Selector.MatchExpressions values must
	//   contain a single semantic version strong. The expression
	//   matches if the version in the label satisfy the condition
	//   as defined by semantic versioning 2.0.0.
	// - ">=", ">", "<=", "<", "=", "!=": the Selector.MatchExpressions values must
	//   contain a single string which can be parsed as [resource.Quantity],
	//   with comparison against the labels' value using the numeric operation.
	Selector metav1.LabelSelector
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AllocationResult captures class and claim parameters and records which node
// capacity instance was used. The JSON dump of this gets stored in the driver
// handle field of an allocated claim.
type AllocationResult struct {
	// TypeMeta must have non-empty kind and apiVersion.
	metav1.TypeMeta

	// ClassParameters are the parameters from the time that the claim
	// was allocated. They can be arbitrary setup parameters that are
	// ignore by the counter controller.
	ClassParameters runtime.RawExtension

	// ClaimParameters are the parameters from the time that the claim was
	// allocated. Some of the fields were used by the counter controller to
	// allocated resources, others can be arbitrary setup parameters.
	ClaimParameters runtime.RawExtension

	// NodeName is the name of the node providing the necessary resources.
	NodeName string

	// DriverName is the name of the driver on that node.
	DriverName string

	// InstanceID is the unique ID chosen by that driver for the resource
	// instance that was picked for the claim.
	InstanceID types.UID

	// Count is the amount that was requested for the claim.
	Count int64
}
