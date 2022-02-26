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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/apis/core"
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
	invalidClassName := "-invalid-"
	validClassName := "valid"
	goodName := "foo"
	now := metav1.Now()
	ten := int64(10)
	goodParameters := core.ResourceClassParametersReference{
		Name:      "valid",
		Namespace: "valid",
		Kind:      "foo",
	}

	scenarios := map[string]struct {
		isExpectedFailure bool
		class             *core.ResourceClass
	}{
		"good-class": {
			isExpectedFailure: false,
			class:             testResourceClass(goodName, validClassName),
		},
		"missing-name": {
			isExpectedFailure: true,
			class:             testResourceClass("", validClassName),
		},
		"bad-name": {
			isExpectedFailure: true,
			class:             testResourceClass("!@#$%^", validClassName),
		},
		"generate-name": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.GenerateName = "pvc-"
				return class
			}(),
		},
		"uid": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return class
			}(),
		},
		"resource-version": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.ResourceVersion = "1"
				return class
			}(),
		},
		"generation": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.Generation = 100
				return class
			}(),
		},
		"creation-timestamp": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.CreationTimestamp = now
				return class
			}(),
		},
		"deletion-grace-period-seconds": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.DeletionGracePeriodSeconds = &ten
				return class
			}(),
		},
		"owner-references": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
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
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.Finalizers = []string{
					"example.com/foo",
				}
				return class
			}(),
		},
		"managed-fields": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
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
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.Labels = map[string]string{
					"apps.kubernetes.io/name": "test",
				}
				return class
			}(),
		},
		"bad-labels": {
			isExpectedFailure: true,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return class
			}(),
		},
		"good-annotations": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.Labels = map[string]string{
					"foo": "bar",
				}
				return class
			}(),
		},
		"bad-annotations": {
			isExpectedFailure: true,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return class
			}(),
		},
		"missing-driver-name": {
			isExpectedFailure: true,
			class:             testResourceClass(goodName, ""),
		},
		"invalid-driver-name": {
			isExpectedFailure: true,
			class:             testResourceClass(goodName, invalidClassName),
		},
		"good-parameters": {
			isExpectedFailure: false,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.ParametersRef = goodParameters.DeepCopy()
				return class
			}(),
		},
		"missing-parameters-name": {
			isExpectedFailure: true,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.ParametersRef = goodParameters.DeepCopy()
				class.ParametersRef.Name = ""
				return class
			}(),
		},
		"bad-parameters-namespace": {
			isExpectedFailure: true,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.ParametersRef = goodParameters.DeepCopy()
				class.ParametersRef.Namespace = "%$#@%"
				return class
			}(),
		},
		"missing-parameters-kind": {
			isExpectedFailure: true,
			class: func() *core.ResourceClass {
				class := testResourceClass(goodName, validClassName)
				class.ParametersRef = goodParameters.DeepCopy()
				class.ParametersRef.Kind = ""
				return class
			}(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			errs := ValidateResourceClass(scenario.class)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Error("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}

func TestValidateResourceClassUpdate(t *testing.T) {
	validClass := testResourceClass("foo", "valid")

	scenarios := map[string]struct {
		isExpectedFailure bool
		oldClass          *core.ResourceClass
		newClass          func(class *core.ResourceClass) *core.ResourceClass
	}{
		"valid-no-op-update": {
			isExpectedFailure: false,
			oldClass:          validClass,
			newClass:          func(class *core.ResourceClass) *core.ResourceClass { return class },
		},
		"update-driver": {
			isExpectedFailure: false,
			oldClass:          validClass,
			newClass: func(class *core.ResourceClass) *core.ResourceClass {
				class.DriverName += "2"
				return class
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldClass.ResourceVersion = "1"
			errs := ValidateResourceClassUpdate(scenario.newClass(scenario.oldClass.DeepCopy()), scenario.oldClass)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}
