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
	"errors"
	"fmt"
	"reflect"

	"github.com/blang/semver/v4"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"

	resourceapi "k8s.io/api/resource/v1alpha3"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/dynamic-resource-allocation/api"
)

var attributesIDType = apiservercel.NewMapType(apiservercel.StringType, apiservercel.AnyType, resourceapi.ResourceSliceMaxAttributesAndCapacitiesPerDevice)

// deviceAttributeIDs is the CEL ref.Val for a Device.Attributes with a
// specific domain. It implements traits.Mapper, with dynamic key and value
// lookup.
type deviceAttributeIDs map[string]api.DeviceAttribute

var _ traits.Mapper = deviceAttributeIDs{}

// Contains implements the traits.Container interface method.
func (d deviceAttributeIDs) Contains(index ref.Val) ref.Val {
	key, ok := index.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return types.False
	}

	_, ok = d[key]
	return types.Bool(ok)
}

// ConvertToNative implements the ref.Val interface method.
func (d deviceAttributeIDs) ConvertToNative(typeDesc reflect.Type) (any, error) {
	return nil, fmt.Errorf("type conversion error from '%s' to '%v'", d.Type(), typeDesc)
}

// ConvertToType implements the ref.Val interface method.
func (d deviceAttributeIDs) ConvertToType(typeVal ref.Type) ref.Val {
	switch typeVal {
	case types.MapType, attributesIDType:
		return d
	case types.TypeType:
		return types.MapType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", d.Type(), typeVal)
}

// Equal implements the ref.Val interface method.
func (d deviceAttributeIDs) Equal(other ref.Val) ref.Val {
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
func (d deviceAttributeIDs) Get(key ref.Val) ref.Val {
	v, found := d.Find(key)
	if !found {
		// This doesn't get reached because github.com/google/cel-go/interpreter/attributes.go
		// checks for a traits.Mapper first and then calls Find. In that case, the "no such key"
		// error comes from the CEL runtime.
		return types.ValOrErr(v, "no such attribute: %v", key)
	}
	return v
}

// Iterator implements the traits.Iterable interface method.
func (d deviceAttributeIDs) Iterator() traits.Iterator {
	strings := make([]types.String, 0, len(d))
	for id := range d {
		strings = append(strings, types.String(id))
	}
	return &stringIterator{Int: -1, strings: strings}
}

// Find implements the traits.Mapper interface method.
func (d deviceAttributeIDs) Find(key ref.Val) (ref.Val, bool) {
	strKey, ok := key.ConvertToType(types.StringType).Value().(string)
	if !ok {
		return nil, false
	}

	attr, ok := d[strKey]
	if !ok {
		return nil, false
	}
	switch {
	case attr.IntValue != nil:
		return types.Int(*attr.IntValue), true
	case attr.BoolValue != nil:
		return types.Bool(*attr.BoolValue), true
	case attr.StringValue != nil:
		return types.String(*attr.StringValue), true
	case attr.VersionValue != nil:
		v, err := semver.Parse(*attr.VersionValue)
		if err != nil {
			return types.WrapErr(err), false
		}
		return apiservercel.Semver{Version: v}, true
	default:
		return types.WrapErr(errors.New("unsupported attribute type")), false
	}
}

// IsZeroValue returns true if there are no attributes with the domain.
func (d deviceAttributeIDs) IsZeroValue() bool {
	return len(d) == 0
}

// Size implements the traits.Sizer interface method.
func (d deviceAttributeIDs) Size() ref.Val {
	return types.Int(len(d))
}

// String converts the map into a human-readable string.
func (d deviceAttributeIDs) String() string {
	return fmt.Sprintf("%v", d.Value())
}

// Type implements the ref.Val interface method.
func (d deviceAttributeIDs) Type() ref.Type {
	return attributesIDType
}

// Value implements the ref.Val interface method.
func (d deviceAttributeIDs) Value() any {
	return map[string]api.DeviceAttribute(d)
}
