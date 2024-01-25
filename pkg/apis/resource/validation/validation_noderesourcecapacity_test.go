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

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/resource"
	"k8s.io/utils/pointer"
)

func testNodeResourceCapacity(name, nodeName, driverName string) *resource.NodeResourceCapacity {
	return &resource.NodeResourceCapacity{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		NodeName:   nodeName,
		DriverName: driverName,
	}
}

func TestValidateNodeResourceCapacity(t *testing.T) {
	goodName := "foo"
	badName := "!@#$%^"
	driverName := "test.example.com"
	now := metav1.Now()
	badValue := "spaces not allowed"

	scenarios := map[string]struct {
		capacity     *resource.NodeResourceCapacity
		wantFailures field.ErrorList
	}{
		"good-claim": {
			capacity: testNodeResourceCapacity(goodName, goodName, driverName),
		},
		"missing-name": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("metadata", "name"), "name or generateName is required")},
			capacity:     testNodeResourceCapacity("", goodName, driverName),
		},
		"bad-name": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "name"), badName, "a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')")},
			capacity:     testNodeResourceCapacity(badName, goodName, driverName),
		},
		"generate-name": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.GenerateName = "pvc-"
				return capacity
			}(),
		},
		"uid": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return capacity
			}(),
		},
		"resource-version": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.ResourceVersion = "1"
				return capacity
			}(),
		},
		"generation": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.Generation = 100
				return capacity
			}(),
		},
		"creation-timestamp": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.CreationTimestamp = now
				return capacity
			}(),
		},
		"deletion-grace-period-seconds": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.DeletionGracePeriodSeconds = pointer.Int64(10)
				return capacity
			}(),
		},
		"owner-references": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "pod",
						Name:       "foo",
						UID:        "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d",
					},
				}
				return capacity
			}(),
		},
		"finalizers": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.Finalizers = []string{
					"example.com/foo",
				}
				return capacity
			}(),
		},
		"managed-fields": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.ManagedFields = []metav1.ManagedFieldsEntry{
					{
						FieldsType: "FieldsV1",
						Operation:  "Apply",
						APIVersion: "apps/v1",
						Manager:    "foo",
					},
				}
				return capacity
			}(),
		},
		"good-labels": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.Labels = map[string]string{
					"apps.kubernetes.io/name": "test",
				}
				return capacity
			}(),
		},
		"bad-labels": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "labels"), badValue, "a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')")},
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.Labels = map[string]string{
					"hello-world": badValue,
				}
				return capacity
			}(),
		},
		"good-annotations": {
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.Annotations = map[string]string{
					"foo": "bar",
				}
				return capacity
			}(),
		},
		"bad-annotations": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "annotations"), badName, "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')")},
			capacity: func() *resource.NodeResourceCapacity {
				capacity := testNodeResourceCapacity(goodName, goodName, driverName)
				capacity.Annotations = map[string]string{
					badName: "hello world",
				}
				return capacity
			}(),
		},
		"bad-nodename": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("nodeName"), badName, "a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')")},
			capacity:     testNodeResourceCapacity(goodName, badName, driverName),
		},
		"bad-drivername": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("driverName"), badName, "a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')")},
			capacity:     testNodeResourceCapacity(goodName, goodName, badName),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			errs := ValidateNodeResourceCapacity(scenario.capacity)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}

func TestValidateNodeResourceCapacityUpdate(t *testing.T) {
	name := "valid"
	validNodeResourceCapacity := testNodeResourceCapacity(name, name, name)

	scenarios := map[string]struct {
		oldNodeResourceCapacity *resource.NodeResourceCapacity
		update                  func(claim *resource.NodeResourceCapacity) *resource.NodeResourceCapacity
		wantFailures            field.ErrorList
	}{
		"valid-no-op-update": {
			oldNodeResourceCapacity: validNodeResourceCapacity,
			update:                  func(claim *resource.NodeResourceCapacity) *resource.NodeResourceCapacity { return claim },
		},
		"invalid-update-nodename": {
			wantFailures:            field.ErrorList{field.Invalid(field.NewPath("nodeName"), name+"-updated", "field is immutable")},
			oldNodeResourceCapacity: validNodeResourceCapacity,
			update: func(capacity *resource.NodeResourceCapacity) *resource.NodeResourceCapacity {
				capacity.NodeName += "-updated"
				return capacity
			},
		},
		"invalid-update-drivername": {
			wantFailures:            field.ErrorList{field.Invalid(field.NewPath("driverName"), name+"-updated", "field is immutable")},
			oldNodeResourceCapacity: validNodeResourceCapacity,
			update: func(capacity *resource.NodeResourceCapacity) *resource.NodeResourceCapacity {
				capacity.DriverName += "-updated"
				return capacity
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldNodeResourceCapacity.ResourceVersion = "1"
			errs := ValidateNodeResourceCapacityUpdate(scenario.update(scenario.oldNodeResourceCapacity.DeepCopy()), scenario.oldNodeResourceCapacity)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}
