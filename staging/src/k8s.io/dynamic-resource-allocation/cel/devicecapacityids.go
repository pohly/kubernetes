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

var capacityIDType = apiservercel.NewMapType(apiservercel.StringType, apiservercel.QuantityDeclType, resourceapi.ResourceSliceMaxAttributesAndCapacitiesPerDevice)

// deviceCapacityIDs is the CEL ref.Value for a Device.Capacitys with a specific domain. It implements traits.Mapper, with
// dynamic key and value lookup.
type deviceCapacityIDs map[string]resource.Quantity

var _ traits.Mapper = &deviceCapacityIDs{}

// Contains implements the traits.Container interface method.
func (d deviceCapacityIDs) Contains(index ref.Val) ref.Val {
	key, ok := index.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return types.False
	}
	_, ok = d[key]
	return types.Bool(ok)
}

// ConvertToNative implements the ref.Val interface method.
func (d deviceCapacityIDs) ConvertToNative(typeDesc reflect.Type) (any, error) {
	return nil, fmt.Errorf("type conversion error from '%s' to '%v'", d.Type(), typeDesc)
}

// ConvertToType implements the ref.Val interface method.
func (d deviceCapacityIDs) ConvertToType(typeVal ref.Type) ref.Val {
	switch typeVal {
	case types.MapType, capacityIDType:
		return d
	case types.TypeType:
		return types.MapType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", d.Type(), typeVal)
}

// Equal implements the ref.Val interface method.
func (d deviceCapacityIDs) Equal(other ref.Val) ref.Val {
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
func (d deviceCapacityIDs) Get(key ref.Val) ref.Val {
	v, found := d.Find(key)
	if !found {
		// This doesn't get reached because github.com/google/cel-go/interpreter/capacity.go
		// checks for a traits.Mapper first and then calls Find. In that case, the "no such key"
		// error comes from the CEL runtime.
		return types.ValOrErr(v, "no such capacity: %v", key)
	}
	return v
}

// Iterator implements the traits.Iterable interface method.
func (d deviceCapacityIDs) Iterator() traits.Iterator {
	return newStringIteratorForMap(d)
}

// Find implements the traits.Mapper interface method.
func (d deviceCapacityIDs) Find(key ref.Val) (ref.Val, bool) {
	strKey, ok := key.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return nil, false
	}

	quantity, ok := d[strKey]
	if !ok {
		return nil, false
	}
	return &apiservercel.Quantity{Quantity: &quantity}, true
}

// IsZeroValue returns true if there are no capacity with the domain.
func (d deviceCapacityIDs) IsZeroValue() bool {
	return len(d) == 0
}

// Size implements the traits.Sizer interface method.
func (d deviceCapacityIDs) Size() ref.Val {
	return types.Int(len(d))
}

// String converts the map into a human-readable string.
func (d deviceCapacityIDs) String() string {
	return fmt.Sprintf("%v", d.Value())
}

// Type implements the ref.Val interface method.
func (d deviceCapacityIDs) Type() ref.Type {
	return capacityIDType
}

// Value implements the ref.Val interface method.
func (d deviceCapacityIDs) Value() any {
	return map[string]resource.Quantity(d)
}
