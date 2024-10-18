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
	"fmt"
	"unique"

	v1beta1 "k8s.io/api/resource/v1beta1"
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

func Convert_v1beta1_Device_To_api_Device(in *v1beta1.Device, out *Device, s conversion.Scope) error {
	out.Name = UniqueString(unique.Make(in.Name))
	switch {
	case in.Basic != nil:
		var outBasic BasicDevice
		if err := Convert_v1beta1_BasicDevice_To_api_BasicDevice(in.Basic, &outBasic, s); err != nil {
			return err
		}
		out.Composite = CompositeDevice{
			Attributes: outBasic.Attributes,
			Capacity:   outBasic.Capacity,
		}
	case in.Composite != nil:
		var outComposite CompositeDevice
		if err := Convert_v1beta1_CompositeDevice_To_api_CompositeDevice(in.Composite, &outComposite, s); err != nil {
			return err
		}
		out.Composite = outComposite
	default:
		return fmt.Errorf("unsupported device type in %+v", in)
	}
	return nil
}

func Convert_api_Device_To_v1beta1_Device(in *Device, out *v1beta1.Device, s conversion.Scope) error {
	return errors.New("conversion to v1beta1.Device not supported")
}
