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
	"unique"

	v1 "k8s.io/api/resource/v1"
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

func Convert_api_TrackedDeviceTaint_To_v1_DeviceTaint(in *TrackedDeviceTaint, out *v1.DeviceTaint, s conversion.Scope) error {
	if in == nil {
		*out = v1.DeviceTaint{}
		return nil
	}

	// Lossy conversion: all additional fields are only used internally and not converted.
	*out = in.DeviceTaint
	return nil
}

func Convert_v1_DeviceTaint_To_api_TrackedDeviceTaint(in *v1.DeviceTaint, out *TrackedDeviceTaint, s conversion.Scope) error {
	if in == nil {
		*out = TrackedDeviceTaint{}
		return nil
	}
	*out = TrackedDeviceTaint{DeviceTaint: *in}
	return nil
}
