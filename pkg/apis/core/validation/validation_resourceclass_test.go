/*
Copyright 2014 The Kubernetes Authors.

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

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/utils/pointer"
)

func testResourceClass(name, driverName string) *core.ResourceClass {
	return &core.ResourceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		DriverName: driverName,
	}
}

func TestValidateResourceClass(t *testing.T) {
	goodName := "foo"
	now := metav1.Now()
	goodParameters := core.ResourceClassParametersReference{
		Name:      "valid",
		Namespace: "valid",
		Kind:      "foo",
	}
	badName := "!@#$%^"
	badValue := "spaces not allowed"

	scenarios := map[string]struct {
		class        *core.ResourceClass
		wantFailures field.ErrorList
	}{
		"good-class": {
			class: testResourceClass(goodName, goodName),
		},
		"missing-name": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("metadata", "name"), "name or generateName is required")},
			class:        testResourceClass("", goodName),
		},
		"bad-name": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "name"), badName, "a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')")},
			class:        testResourceClass(badName, goodName),
		},
		"generate-name": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.GenerateName = "pvc-"
				return class
			}(),
		},
		"uid": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return class
			}(),
		},
		"resource-version": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.ResourceVersion = "1"
				return class
			}(),
		},
		"generation": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.Generation = 100
				return class
			}(),
		},
		"creation-timestamp": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.CreationTimestamp = now
				return class
			}(),
		},
		"deletion-grace-period-seconds": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.DeletionGracePeriodSeconds = pointer.Int64(10)
				return class
			}(),
		},
		"owner-references": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "pod",
						Name:       "foo",
						UID:        "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d",
					},
				}
				return class
			}(),
		},
		"finalizers": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.Finalizers = []string{
					"example.com/foo",
				}
				return class
			}(),
		},
		"managed-fields": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.ManagedFields = []metav1.ManagedFieldsEntry{
					{
						FieldsType: "FieldsV1",
						Operation:  "Apply",
						APIVersion: "apps/v1",
						Manager:    "foo",
					},
				}
				return class
			}(),
		},
		"good-labels": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.Labels = map[string]string{
					"apps.kubernetes.io/name": "test",
				}
				return class
			}(),
		},
		"bad-labels": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "labels"), badValue, "a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')")},
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.Labels = map[string]string{
					"hello-world": badValue,
				}
				return class
			}(),
		},
		"good-annotations": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.Annotations = map[string]string{
					"foo": "bar",
				}
				return class
			}(),
		},
		"bad-annotations": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "annotations"), badName, "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')")},
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.Annotations = map[string]string{
					badName: "hello world",
				}
				return class
			}(),
		},
		"missing-driver-name": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("driverName"), "")},
			class:        testResourceClass(goodName, ""),
		},
		"invalid-driver-name": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("driverName"), badName, "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')")},
			class:        testResourceClass(goodName, badName),
		},
		"good-parameters": {
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.ParametersRef = goodParameters.DeepCopy()
				return class
			}(),
		},
		"missing-parameters-name": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("parametersRef", "name"), "")},
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.ParametersRef = goodParameters.DeepCopy()
				class.ParametersRef.Name = ""
				return class
			}(),
		},
		"bad-parameters-namespace": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("parametersRef", "namespace"), badName, "a lowercase RFC 1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')")},
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.ParametersRef = goodParameters.DeepCopy()
				class.ParametersRef.Namespace = badName
				return class
			}(),
		},
		"missing-parameters-kind": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("parametersRef", "kind"), "")},
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, goodName)
				class.ParametersRef = goodParameters.DeepCopy()
				class.ParametersRef.Kind = ""
				return class
			}(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			errs := ValidateResourceClass(scenario.class)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}

func TestValidateResourceClassUpdate(t *testing.T) {
	validClass := testResourceClass("foo", "valid")

	scenarios := map[string]struct {
		oldClass     *core.ResourceClass
		update       func(class *core.ResourceClass) *core.ResourceClass
		wantFailures field.ErrorList
	}{
		"valid-no-op-update": {
			oldClass: validClass,
			update:   func(class *core.ResourceClass) *core.ResourceClass { return class },
		},
		"update-driver": {
			oldClass: validClass,
			update: func(class *core.ResourceClass) *core.ResourceClass {
				class.DriverName += "2"
				return class
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldClass.ResourceVersion = "1"
			errs := ValidateResourceClassUpdate(scenario.update(scenario.oldClass.DeepCopy()), scenario.oldClass)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}
