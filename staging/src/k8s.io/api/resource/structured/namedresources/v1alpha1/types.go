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
	"k8s.io/apimachinery/pkg/api/resource"
)

// Resources is used in NodeResourceModel.
type Resources struct {
	// +listType=atomic
	Instances []Instance `json:"instances" protobuf:"bytes,1,name=instances"`
}

type Instance struct {
	// Name is unique identifier among all resource instances managed by
	// the driver on the node. It must be a DNS subdomain.
	Name string `json:"name" protobuf:"bytes,1,name=name"`

	// Attributes defines the attributes of this resource instance.
	// The name of each attribute must be unique.
	//
	// +listType=atomic
	// +optional
	Attributes []Attribute `json:"attributes,omitempty" protobuf:"bytes,2,opt,name=attributes"`
}

type Attribute struct {
	// Name is unique identifier among all resource instances managed by
	// the driver on the node. It must be a DNS subdomain.
	Name string `json:"name" protobuf:"bytes,1,name=name"`

	AttributeValue `json:",inline" protobuf:"bytes,2,opt,name=attributeValue"`
}

// AttributeValue must have one and only one field set.
type AttributeValue struct {
	QuantityValue    *resource.Quantity `json:"quantityValue,omitempty" protobuf:"bytes,6,opt,name=quantityValue"`
	BoolValue        *bool              `json:"boolValue,omitempty" protobuf:"bytes,2,opt,name=boolValue"`
	IntValue         *int64             `json:"intValue,omitempty" protobuf:"varint,7,opt,name=intValue"`
	IntSliceValue    *Int64Slice        `json:"intSliceValue,omitempty" protobuf:"varint,8,rep,name=intSliceValue"`
	StringValue      *string            `json:"stringValue,omitempty" protobuf:"bytes,5,opt,name=stringValue"`
	StringSliceValue *StringSlice       `json:"stringSliceValue,omitempty" protobuf:"bytes,9,rep,name=stringSliceValue"`
	// TODO: VersionValue     *SemVersion        `json:"version,omitempty" protobuf:"bytes,7,opt,name=versionValue"`
}

type Int64Slice struct {
	// +listType=atomic
	Ints []int64 `json:",inline" protobuf:"bytes,1,opt,name=ints"`
}

type StringSlice struct {
	// +listType=atomic
	Strings []string `json:",inline" protobuf:"bytes,1,opt,name=strings"`
}

// TODO
//
// A wrapper around https://pkg.go.dev/github.com/blang/semver/v4#Version which
// is encoded as a string. During decoding, it validates that the string
// can be parsed using tolerant parsing (currently trims spaces, removes a "v" prefix,
// adds a 0 patch number to versions with only major and minor components specified,
// and removes leading 0s).
// type SemVersion struct {
//	semver.Version
//}

// Request is used in ResourceRequestModel.
type Request struct {
	// Selector is a CEL expression which must evaluate to true if a
	// resource instance is suitable. The language is as defined in
	// https://kubernetes.io/docs/reference/using-api/cel/
	//
	// In addition, for each type in AttributeValue there is a map that
	// resolves to the corresponding value of the instance under evaluation.
	// For example:
	//
	//    attributes.quantity["a"].isGreaterThan(quantity("0")) &&
	//    attributes.stringslice["b"].isSorted()
	Selector string `json:"selector" protobuf:"bytes,1,name=selector"`
}

// Filter is used in ResourceFilterModel.
type Filter struct {
	// Selector is a selector like the one in Request. It must be true for
	// a resource instance to be suitable for a claim using the class.
	Selector string `json:"selector" protobuf:"bytes,1,name=selector"`
}

// AllocationResult is used in AllocationResultModel.
type AllocationResult struct {
	// Name is the name of the selected resource instance.
	Name string `json:"name" protobuf:"bytes,1,name=name"`
}
