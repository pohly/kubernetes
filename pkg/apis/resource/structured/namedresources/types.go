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

package namedresources

import "k8s.io/apimachinery/pkg/api/resource"

// Resources is used in NodeResourceModel.
type Resources struct {
	Instances []Instance
}

type Instance struct {
	// Name is unique identifier among all resource instances managed by
	// the driver on the node. It must be a DNS subdomain.
	Name string

	// Attributes defines the attributes of this resource instance.
	// The name of each attribute must be unique.
	Attributes []Attribute
}

type Attribute struct {
	// Name is unique identifier among all resource instances managed by
	// the driver on the node. It must be a DNS subdomain.
	Name string

	AttributeValue
}

// AttributeValue must have one and only one field set.
type AttributeValue struct {
	QuantityValue    *resource.Quantity
	BoolValue        *bool
	IntValue         *int64
	IntSliceValue    *IntSlice
	StringValue      *string
	StringSliceValue *StringSlice
	// TODO: Version     *SemVersion
}

type IntSlice struct {
	Ints []int64
}

type StringSlice struct {
	Strings []string
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
	Selector string
}

// Filter is used in ResourceFilterModel.
type Filter struct {
	// Selector is a selector like the one in Request. It must be true for
	// a resource instance to be suitable for a claim using the class.
	Selector string
}

// AllocationResult is used in AllocationResultModel.
type AllocationResult struct {
	// Name is the name of the selected resource instance.
	Name string
}
