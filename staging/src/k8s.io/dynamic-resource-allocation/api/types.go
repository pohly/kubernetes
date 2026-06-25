/*
Copyright 2024 The Kubernetes Authors.

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

package api

import (
	"fmt"
	"slices"

	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

// JSON tags exist to make the output more readable (klog, diff.Diff).
// They are intentionally not compatible with the normal encoding
// of a ResourceSlice to avoid accidentally using them with an apiserver
// request:
// - TypeMeta does not get encoded.
// - Fields from this package use upper case whereas types from the
//   real API use lower case.

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ResourceSlice struct {
	metav1.TypeMeta `json:"-"` // Not needed, not set consistently.
	metav1.ObjectMeta

	Spec ResourceSliceSpec

	// uniqueStringMap is a cache for looking up the UniqueString instances
	// used in this slice. MakeUniqueString would yield the same result,
	// but must lock.
	uniqueStringMap map[string]UniqueString
}

// MakeUniqueString ensures that the string is in the per-slice
// unique string cache and returns the unique string for it.
//
// Conversion of a resourceapi.ResourceSlice into this ResourceSlice
// uses this method, therefore all unique strings used for this
// instance are cached after conversion.
func (r *ResourceSlice) MakeUniqueString(str string) UniqueString {
	if r.uniqueStringMap == nil {
		r.uniqueStringMap = make(map[string]UniqueString)
	}
	u, ok := r.uniqueStringMap[str]
	if ok {
		return u
	}
	u = MakeUniqueString(str)
	r.uniqueStringMap[str] = u
	return u
}

// LookupUniqueString returns a unique string if the string is in
// the cache populated by MakeUniqueString, otherwise [NullUniqueString].
func (r *ResourceSlice) LookupUniqueString(str string) UniqueString {
	// Most likely string: the driver name. It's at the root of most
	// attribute and capacity lookups.
	if str == r.Spec.Driver.String() {
		return r.Spec.Driver
	}
	u, ok := r.uniqueStringMap[str]
	if ok {
		return u
	}
	return NullUniqueString
}

type ResourceSliceSpec struct {
	Driver                 UniqueString
	Pool                   ResourcePool
	NodeName               *string          `json:",omitempty"`
	NodeSelector           *v1.NodeSelector `json:",omitempty"`
	AllNodes               bool             `json:",omitempty"`
	Devices                []Device         `json:",omitempty"`
	PerDeviceNodeSelection *bool            `json:",omitempty"`
	SharedCounters         []CounterSet     `json:",omitempty"`
}

type CounterSet struct {
	Name     UniqueString
	Counters map[string]resourceapi.Counter `json:",omitempty"`
}

type ResourcePool struct {
	Name               UniqueString
	Generation         int64
	ResourceSliceCount int64
}

type Device struct {
	Name                            UniqueString
	Attributes                      DeviceAttributes                                               `json:",omitempty"`
	Capacity                        DeviceCapacities                                               `json:",omitempty"`
	ConsumesCounters                []DeviceCounterConsumption                                     `json:",omitempty"`
	NodeName                        *string                                                        `json:",omitempty"`
	NodeSelector                    *v1.NodeSelector                                               `json:",omitempty"`
	AllNodes                        *bool                                                          `json:",omitempty"`
	Taints                          []resourceapi.DeviceTaint                                      `json:",omitempty"`
	BindsToNode                     bool                                                           `json:",omitempty"`
	BindingConditions               []string                                                       `json:",omitempty"`
	BindingFailureConditions        []string                                                       `json:",omitempty"`
	AllowMultipleAllocations        *bool                                                          `json:",omitempty"`
	NodeAllocatableResourceMappings map[v1.ResourceName]resourceapi.NodeAllocatableResourceMapping `json:",omitempty"`
}

type DeviceCounterConsumption struct {
	CounterSet UniqueString
	Counters   map[string]resourceapi.Counter `json:",omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ResourceSliceList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []ResourceSlice
}

// DeviceAttributes maps domain + id from a FullyQualifiedName to the attribute value.
// String/bool/int values are stored as such. Version values are stored as
// apiservercel.Semver. Lists are stored as slices of their values, again using
// apiservercel.Semver.
type DeviceAttributes map[UniqueString]map[UniqueString]any

func (m DeviceAttributes) DeepCopy() DeviceAttributes {
	if m == nil {
		return nil
	}
	out := make(DeviceAttributes, len(m))
	for k, v := range m {
		if v == nil {
			out[k] = nil
			continue
		}
		inner := make(map[UniqueString]any, len(v))
		for k, v := range v {
			switch v := v.(type) {
			case int64:
				inner[k] = v
			case bool:
				inner[k] = v
			case string:
				inner[k] = v
			case apiservercel.Semver:
				inner[k] = v
			case []int64:
				inner[k] = slices.Clone(v)
			case []bool:
				inner[k] = slices.Clone(v)
			case []string:
				inner[k] = slices.Clone(v)
			case []apiservercel.Semver:
				inner[k] = slices.Clone(v)
			default:
				panic(fmt.Sprintf("internal error, missing case for %T", v))
			}
		}
		out[k] = inner
	}
	return out
}

func (m DeviceAttributes) Lookup(domain, name UniqueString) any {
	return m[domain][name]
}

// DeviceCapacity holds the capacity value for a single capacity entry.
// The value uses apiservercel.Quantity for direct use in CEL evaluation.
type DeviceCapacity struct {
	Value         apiservercel.Quantity
	RequestPolicy *resourceapi.CapacityRequestPolicy
}

// DeepCopy returns a deep copy of DeviceCapacity.
func (in *DeviceCapacity) DeepCopy() *DeviceCapacity {
	if in == nil {
		return nil
	}
	out := new(DeviceCapacity)
	valueCopy := in.Value.DeepCopy()
	out.Value = apiservercel.Quantity{Quantity: &valueCopy}
	out.RequestPolicy = in.RequestPolicy.DeepCopy()
	return out
}

// DeviceCapacities maps domain + unqualified name to the capacity value.
type DeviceCapacities map[UniqueString]map[UniqueString]DeviceCapacity

func (m DeviceCapacities) DeepCopy() DeviceCapacities {
	if m == nil {
		return nil
	}
	out := make(DeviceCapacities, len(m))
	for domain, inner := range m {
		if inner == nil {
			out[domain] = nil
			continue
		}
		innerCopy := make(map[UniqueString]DeviceCapacity, len(inner))
		for name, cap := range inner {
			innerCopy[name] = *cap.DeepCopy()
		}
		out[domain] = innerCopy
	}
	return out
}

func (m DeviceCapacities) Lookup(domain, name UniqueString) (DeviceCapacity, bool) {
	inner, ok := m[domain]
	if !ok {
		return DeviceCapacity{}, false
	}
	cap, ok := inner[name]
	return cap, ok
}
