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

package cel

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"

	resourceapi "k8s.io/api/resource/v1alpha3"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/dynamic-resource-allocation/api"
)

var attributesType = apiservercel.NewMapType(apiservercel.StringType, attributesIDType, resourceapi.ResourceSliceMaxAttributesAndCapacitiesPerDevice)

// deviceAttributeDomains is the CEL ref.Value for a Device.Attributes. It implements traits.Mapper, with
// dynamic key and value lookup.
type deviceAttributeDomains map[string]map[string]api.DeviceAttribute

var _ traits.Mapper = &deviceAttributeDomains{}

// Contains implements the traits.Container interface method.
func (d deviceAttributeDomains) Contains(index ref.Val) ref.Val {
	strKey, ok := index.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return types.False
	}
	_, ok = d[strKey]
	return types.Bool(ok)
}

// ConvertToNative implements the ref.Val interface method.
func (d deviceAttributeDomains) ConvertToNative(typeDesc reflect.Type) (any, error) {
	// If the map is already assignable to the desired type return it, e.g. interfaces and
	// maps with the same key value types.
	if reflect.TypeOf(d).AssignableTo(typeDesc) {
		return d, nil
	}
	return nil, fmt.Errorf("type conversion error from '%s' to '%v'", d.Type(), typeDesc)
}

// ConvertToType implements the ref.Val interface method.
func (d deviceAttributeDomains) ConvertToType(typeVal ref.Type) ref.Val {
	switch typeVal {
	case types.MapType, attributesType:
		return d
	case types.TypeType:
		return types.MapType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", d.Type(), typeVal)
}

// Equal implements the ref.Val interface method.
func (d deviceAttributeDomains) Equal(other ref.Val) ref.Val {
	otherMap, ok := other.(traits.Mapper)
	if !ok {
		return types.False
	}
	if otherMap.Size() != types.Int(len(d)) {
		return types.False
	}
	for key := range d {
		key := types.String(key)
		thisVal, _ := d.Find(key)
		otherVal, found := otherMap.Find(key)
		if !found {
			return types.False
		}
		valEq := types.Equal(thisVal, otherVal)
		if valEq == types.False {
			return types.False
		}
	}
	return types.True
}

// Get implements the traits.Indexer interface method.
func (d deviceAttributeDomains) Get(key ref.Val) ref.Val {
	v, found := d.Find(key)
	if !found {
		return types.ValOrErr(v, "no such key: %v", key)
	}
	return v
}

// Iterator implements the traits.Iterable interface method.
func (d deviceAttributeDomains) Iterator() traits.Iterator {
	return newStringIteratorForMap(d)
}

// Find implements the traits.Mapper interface method.
func (d deviceAttributeDomains) Find(key ref.Val) (ref.Val, bool) {
	strKey, ok := key.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return nil, false
	}

	// The first-level lookup by domain never fails. We merely
	// remember the domain and use that when looking up
	// by ID at the next level.
	return deviceAttributeIDs(d[strKey]), true
}

// IsZeroValue returns true if the struct is empty.
func (d deviceAttributeDomains) IsZeroValue() bool {
	return len(d) == 0
}

// Size implements the traits.Sizer interface method.
func (d deviceAttributeDomains) Size() ref.Val {
	return types.Int(len(d))
}

// String converts the map into a human-readable string.
func (d deviceAttributeDomains) String() string {
	return fmt.Sprintf("%v", d.Value())
}

// Type implements the ref.Val interface method.
func (d deviceAttributeDomains) Type() ref.Type {
	return attributesType
}

// Value implements the ref.Val interface method.
func (d deviceAttributeDomains) Value() any {
	return map[string]map[string]api.DeviceAttribute(d)
}
