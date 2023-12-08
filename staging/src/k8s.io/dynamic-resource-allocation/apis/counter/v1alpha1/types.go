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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:storageversion

// Capacity defines how much of certain items are available respectively
// required.
type Capacity struct {
	// ObjectMeta may be set when publishing capacity, but is not required.
	// It is ignored when matching requests against available capacity.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// TypeMeta must have non-empty kind and apiVersion.
	metav1.TypeMeta `json:",inline"`

	// Count is the total number of available items per node when
	// publishing capacity and the required number of items when requesting
	// resources.
	Count int64 `json:"count"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AllocationResult captures class and claim parameters and records which node
// capacity instance was used. The JSON dump of this gets stored in the driver
// handle field of an allocated claim.
type AllocationResult struct {
	// TypeMeta must have non-empty kind and apiVersion.
	metav1.TypeMeta `json:",inline"`

	// ClassParameters are the parameters from the time that the claim
	// was allocated. They can be arbitrary setup parameters that are
	// ignore by the counter controller.
	ClassParameters runtime.RawExtension `json:"classParameters,omitempty"`

	// ClaimParameters are the parameters from the time that the claim was
	// allocated. Some of the fields were used by the counter controller to
	// allocated resources, others can be arbitrary setup parameters.
	ClaimParameters runtime.RawExtension `json:"claimParameters,omitempty"`

	// NodeName is the name of the node providing the necessary resources.
	NodeName string `json:"nodeName"`

	// DriverName is the name of the driver on that node.
	DriverName string `json:"driverName"`

	// InstanceID is the unique ID chosen by that driver for the resource
	// instance that was picked for the claim.
	InstanceID types.UID `json:"instanceID"`

	// Count is the amount that was requested for the claim.
	Count int64 `json:"count"`
}
