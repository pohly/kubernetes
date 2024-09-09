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
	"k8s.io/apimachinery/pkg/api/resource"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

var capacityType = apiservercel.NewMapType(apiservercel.StringType, capacityIDType, resourceapi.ResourceSliceMaxAttributesAndCapacitiesPerDevice)

// deviceCapacityDomains is the CEL ref.Val for a Device.Capacity. It implements traits.Mapper, with
// dynamic key and value lookup.
type deviceCapacityDomains map[string]map[string]resource.Quantity

var _ traits.Mapper = &deviceCapacityDomains{}

// Contains implements the traits.Container interface method.
func (d deviceCapacityDomains) Contains(index ref.Val) ref.Val {
	strKey, ok := index.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return types.False
	}
	_, ok = d[strKey]
	return types.Bool(ok)
}

// ConvertToNative implements the ref.Val interface method.
func (d deviceCapacityDomains) ConvertToNative(typeDesc reflect.Type) (any, error) {
	// If the map is already assignable to the desired type return it, e.g. interfaces and
	// maps with the same key value types.
	if reflect.TypeOf(d).AssignableTo(typeDesc) {
		return d, nil
	}
	return nil, fmt.Errorf("type conversion error from '%s' to '%v'", d.Type(), typeDesc)
}

// ConvertToType implements the ref.Val interface method.
func (d deviceCapacityDomains) ConvertToType(typeVal ref.Type) ref.Val {
	switch typeVal {
	case types.MapType, capacityType:
		return d
	case types.TypeType:
		return types.MapType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", d.Type(), typeVal)
}

// Equal implements the ref.Val interface method.
func (d deviceCapacityDomains) Equal(other ref.Val) ref.Val {
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
func (d deviceCapacityDomains) Get(key ref.Val) ref.Val {
	v, found := d.Find(key)
	if !found {
		return types.ValOrErr(v, "no such key: %v", key)
	}
	return v
}

// Iterator implements the traits.Iterable interface method.
func (d deviceCapacityDomains) Iterator() traits.Iterator {
	return newStringIteratorForMap(d)
}

// Find implements the traits.Mapper interface method.
func (d deviceCapacityDomains) Find(key ref.Val) (ref.Val, bool) {
	strKey, ok := key.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return nil, false
	}

	// The first-level lookup by domain never fails. We merely
	// remember the domain and use that when looking up
	// by ID at the next level.
	return deviceCapacityIDs(d[strKey]), true
}

// IsZeroValue returns true if the struct is empty.
func (d deviceCapacityDomains) IsZeroValue() bool {
	return len(d) == 0
}

// Size implements the traits.Sizer interface method.
func (d deviceCapacityDomains) Size() ref.Val {
	return types.Int(len(d))
}

// String converts the map into a human-readable string.
func (d deviceCapacityDomains) String() string {
	return fmt.Sprintf("%v", d.Value())
}

// Type implements the ref.Val interface method.
func (d deviceCapacityDomains) Type() ref.Type {
	return capacityType
}

// Value implements the ref.Val interface method.
func (d deviceCapacityDomains) Value() any {
	return map[string]map[string]resource.Quantity(d)
}
