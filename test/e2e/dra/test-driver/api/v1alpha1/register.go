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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const GroupName = "dra.e2e.example.com"
const Version = "v1alpha1"

var (
	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		return addKnownTypes(scheme, SchemeGroupVersion)
	})
	AddToScheme = SchemeBuilder.AddToScheme
)

// Install adds the types to the scheme with a group name chosen by the caller.
// This allows E2E tests to run in parallel.
func Install(scheme *runtime.Scheme, groupName string) {
	schemeBuilder := runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		return addKnownTypes(scheme, schema.GroupVersion{Group: groupName, Version: Version})
	})
	utilruntime.Must(schemeBuilder.AddToScheme(scheme))
}

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// Adds the list of known types to api.Scheme.
func addKnownTypes(scheme *runtime.Scheme, gv schema.GroupVersion) error {
	scheme.AddKnownTypes(gv,
		&ClaimParameter{},
		&ClassParameter{},
	)
	return nil
}
