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
)

const (
	DriverParameterAPIGroup = "dra.e2e.example.com"
)

// +genclient
// +kubebuilder:storageversion
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClaimParameter is the type used for ResourceClaims.
//
// +kubebuilder:resource:path=claimparameters,scope=Namespaced,singular=claimparameter
type ClaimParameter struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	metav1.TypeMeta   `json:",inline"`

	Spec ClaimParameterSpec `json:"spec"`
}

// Spec is used above for these reasons:
// - Familarity - most types have it, even when they have no status.
// - It groups non-standard fields together in a YAML dump.
// - It contains metav1.TypeMeta for use as numeric parameters,
//   which would clash with the metav1.TypeMeta in the root (could
//   be fixed by using a different TypeMeta for numeric parameters).

// ClaimParameterSpec holds the custom parameters for a ClaimParameter.
type ClaimParameterSpec struct {
	// TypeMeta must have non-empty kind and apiVersion. The defaults
	// ensure that.
	TypeMeta `json:",inline"`

	// Count is the required number of items per claim.
	//
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Count int64 `json:"count,omitempty"`

	// Env are environment variables that get injected into
	// the container sandbox by the driver.
	Env map[string]string `json:"env,omitempty"`
}

// Implementation detail, not relevant for users:
// the Count field above corresponds to k8s.io/dynamic-resource-allocation/apis/counter/v1alpha1/Parameter,
// which makes it possible for core Kubernetes to handle it.
//
// The Selector feature is not supported by the test driver. Users
// never get to see it.

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClassParameter is the type used for ResourceClasss.
//
// +kubebuilder:resource:path=classparameters,scope=Namespaced,singular=classparameter
type ClassParameter struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	metav1.TypeMeta   `json:",inline"`

	Spec ClassParameterSpec `json:"spec"`
}

// ClassParameterSpec holds the custom values of a ClassParameter.
//
// In contrast to ClassParameterSpec, only setup parameters can be specified.
type ClassParameterSpec struct {
	// Env are environment variables that get injected into
	// the container sandbox by the driver.
	Env map[string]string `json:"env,omitempty"`
}

// TypeMeta matches metav1.TypeMeta. It gets re-defined here to add the right
// defaults.
type TypeMeta struct {
	// Kind must be "Parameter".
	//
	// +kubebuilder:default=Parameter
	// +kubebuilder:validation:Enum=Parameter
	Kind string `json:"kind,omitempty"`

	// APIVersion must be one which is supported by the driver.
	//
	// +kubebuilder:default=counter.dra.config.k8s.io/v1alpha1
	// +kubebuilder:validation:Enum=counter.dra.config.k8s.io/v1alpha1
	APIVersion string `json:"apiVersion,omitempty"`
}
