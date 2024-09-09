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
	"errors"
	"strings"
	"unique"

	v1alpha3 "k8s.io/api/resource/v1alpha3"
	"k8s.io/apimachinery/pkg/api/resource"
	conversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	localSchemeBuilder runtime.SchemeBuilder
	AddToScheme        = localSchemeBuilder.AddToScheme
)

func Convert_api_UniqueString_To_string(in *UniqueString, out *string, s conversion.Scope) error {
	if *in == NullUniqueString {
		*out = ""
		return nil
	}
	*out = in.String()
	return nil
}

func Convert_string_To_api_UniqueString(in *string, out *UniqueString, s conversion.Scope) error {
	if *in == "" {
		*out = NullUniqueString
		return nil
	}
	*out = UniqueString(unique.Make(*in))
	return nil
}

func Convert_v1alpha3_BasicDevice_To_api_BasicDevice(in *v1alpha3.BasicDevice, out *BasicDevice, s conversion.Scope) error {
	if s == nil {
		return errors.New("conversion scope required with driver name as context")
	}
	driverName, ok := s.Meta().Context.(string)
	if !ok {
		return errors.New("conversion meta must have driver name as context")
	}

	var attributes map[string]map[string]DeviceAttribute
	for name, attrIn := range in.Attributes {
		domain, id := parseQualifiedName(name, driverName)
		if attributes == nil {
			attributes = make(map[string]map[string]DeviceAttribute)
		}
		if attributes[domain] == nil {
			attributes[domain] = make(map[string]DeviceAttribute)
		}
		var attrOut DeviceAttribute
		if err := Convert_v1alpha3_DeviceAttribute_To_api_DeviceAttribute(&attrIn, &attrOut, s); err != nil {
			return err
		}
		attributes[domain][id] = attrOut
	}

	var capacity map[string]map[string]resource.Quantity
	for name, quantity := range in.Capacity {
		domain, id := parseQualifiedName(name, driverName)
		if capacity == nil {
			capacity = make(map[string]map[string]resource.Quantity)
		}
		if capacity[domain] == nil {
			capacity[domain] = make(map[string]resource.Quantity)
		}
		capacity[domain][id] = quantity
	}

	out.Attributes = attributes
	out.Capacity = capacity

	return nil
}

func Convert_BasicDevice(in *v1alpha3.BasicDevice, out *BasicDevice, driverName string) error {
	return Convert_v1alpha3_BasicDevice_To_api_BasicDevice(in, out, &basicDeviceScope{Context: driverName})
}

func Convert_v1alpha3_ResourceSliceSpec_To_api_ResourceSliceSpec(in *v1alpha3.ResourceSliceSpec, out *ResourceSliceSpec, s conversion.Scope) error {
	if s == nil {
		// Pass down driver name through scope.
		s = &basicDeviceScope{Context: in.Driver}
	}
	return autoConvert_v1alpha3_ResourceSliceSpec_To_api_ResourceSliceSpec(in, out, s)
}

type basicDeviceScope conversion.Meta

func (b *basicDeviceScope) Convert(src, dest interface{}) error {
	return errors.New("basicDeviceScope.Convert not implemented")
}

func (b *basicDeviceScope) Meta() *conversion.Meta {
	return (*conversion.Meta)(b)
}

func Convert_api_BasicDevice_To_v1alpha3_BasicDevice(in *BasicDevice, out *v1alpha3.BasicDevice, s conversion.Scope) error {
	return errors.New("not implemented")
}

// parseQualifiedName splits into domain and identified, using the default domain
// if the name does not contain one.
func parseQualifiedName(name v1alpha3.QualifiedName, defaultDomain string) (string, string) {
	sep := strings.Index(string(name), "/")
	if sep == -1 {
		return defaultDomain, string(name)
	}
	return string(name[0:sep]), string(name[sep+1:])
}
