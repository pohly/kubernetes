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

	apiservercel "k8s.io/apiserver/pkg/cel"
)

const (
	driverVar     = "driver"
	attributesVar = "attributes"
	capacityVar   = "capacity"
)

var deviceFields = []types.String{driverVar, attributesVar, capacityVar}

var deviceType = apiservercel.NewObjectType("kubernetes.DRADevice", fields(
	field(driverVar, apiservercel.StringType, true),
	field(attributesVar, attributesType, true),
	field(capacityVar, capacityType, true),
))

func field(name string, declType *apiservercel.DeclType, required bool) *apiservercel.DeclField {
	return apiservercel.NewDeclField(name, declType, required, nil, nil)
}
func fields(fields ...*apiservercel.DeclField) map[string]*apiservercel.DeclField {
	result := make(map[string]*apiservercel.DeclField, len(fields))
	for _, f := range fields {
		result[f.Name] = f
	}
	return result
}

// device is the CEL ref.Val for a Device. It implements traits.Mapper, with
// fixed names for the fields.
type device Device

var _ traits.Mapper = &device{}

// Contains implements the traits.Container interface method.
func (d *device) Contains(index ref.Val) ref.Val {
	key, ok := index.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return types.False
	}
	switch key {
	case driverVar, attributesVar, capacityVar:
		return types.True
	default:
		return types.False
	}
}

// ConvertToNative implements the ref.Val interface method.
func (d *device) ConvertToNative(typeDesc reflect.Type) (any, error) {
	// If the map is already assignable to the desired type return it, e.g. interfaces and
	// maps with the same key value types.
	if reflect.TypeOf(d).AssignableTo(typeDesc) {
		return d, nil
	}
	return nil, fmt.Errorf("type conversion error from '%s' to '%v'", d.Type(), typeDesc)
}

// ConvertToType implements the ref.Val interface method.
func (d *device) ConvertToType(typeVal ref.Type) ref.Val {
	switch typeVal {
	case types.MapType, deviceType:
		return d
	case types.TypeType:
		return types.MapType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", d.Type(), typeVal)
}

// Equal implements the ref.Val interface method.
func (d *device) Equal(other ref.Val) ref.Val {
	otherMap, ok := other.(traits.Mapper)
	if !ok {
		return types.False
	}
	if otherMap.Size() != types.Int(len(deviceFields)) {
		return types.False
	}
	for _, key := range deviceFields {
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
func (d *device) Get(key ref.Val) ref.Val {
	v, found := d.Find(key)
	if !found {
		return types.ValOrErr(v, "no such key: %v", key)
	}
	return v
}

// Iterator implements the traits.Iterable interface method.
func (d *device) Iterator() traits.Iterator {
	return &deviceFieldIterator{Int: -1}
}

type deviceFieldIterator struct {
	types.Int
}

func (i *deviceFieldIterator) HasNext() ref.Val {
	return types.Bool(i.Int+1 < types.Int(len(deviceFields)))
}

func (i *deviceFieldIterator) Next() ref.Val {
	i.Int++
	return deviceFields[i.Int]
}

// Find implements the traits.Mapper interface method.
func (d *device) Find(key ref.Val) (ref.Val, bool) {
	strKey, ok := key.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return nil, false
	}
	switch strKey {
	case driverVar:
		return types.String(d.Driver), true
	case attributesVar:
		return deviceAttributeDomains(d.Attributes), true
	case capacityVar:
		return deviceCapacityDomains(d.Capacity), true
	default:
		return nil, false
	}
}

// IsZeroValue returns true if the struct is empty.
func (d *device) IsZeroValue() bool {
	return d.Driver == "" &&
		len(d.Attributes) == 0 &&
		len(d.Capacity) == 0
}

// Size implements the traits.Sizer interface method.
func (d *device) Size() ref.Val {
	return types.Int(len(deviceFields))
}

// String converts the map into a human-readable string.
func (d *device) String() string {
	return fmt.Sprintf("%v", (*Device)(d))
}

// Type implements the ref.Val interface method.
func (d *device) Type() ref.Type {
	return deviceType
}

// Value implements the ref.Val interface method.
func (d *device) Value() any {
	return (*Device)(d)
}
