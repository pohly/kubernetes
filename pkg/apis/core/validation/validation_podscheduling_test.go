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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/utils/pointer"
)

func testPodScheduling(name, namespace string, spec core.PodSchedulingSpec) *core.PodScheduling {
	return &core.PodScheduling{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

func TestValidatePodScheduling(t *testing.T) {
	goodName := "foo"
	goodNS := "ns"
	goodPodSchedulingSpec := core.PodSchedulingSpec{}
	now := metav1.Now()
	badName := "!@#$%^"
	badValue := "spaces not allowed"

	scenarios := map[string]struct {
		scheduling   *core.PodScheduling
		wantFailures field.ErrorList
	}{
		"good-scheduling": {
			scheduling: testPodScheduling(goodName, goodNS, goodPodSchedulingSpec),
		},
		"missing-name": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("metadata", "name"), "name or generateName is required")},
			scheduling:   testPodScheduling("", goodNS, goodPodSchedulingSpec),
		},
		"bad-name": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "name"), badName, "a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')")},
			scheduling:   testPodScheduling(badName, goodNS, goodPodSchedulingSpec),
		},
		"missing-namespace": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("metadata", "namespace"), "")},
			scheduling:   testPodScheduling(goodName, "", goodPodSchedulingSpec),
		},
		"generate-name": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.GenerateName = "pvc-"
				return scheduling
			}(),
		},
		"uid": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return scheduling
			}(),
		},
		"resource-version": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.ResourceVersion = "1"
				return scheduling
			}(),
		},
		"generation": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Generation = 100
				return scheduling
			}(),
		},
		"creation-timestamp": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.CreationTimestamp = now
				return scheduling
			}(),
		},
		"deletion-grace-period-seconds": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.DeletionGracePeriodSeconds = pointer.Int64(10)
				return scheduling
			}(),
		},
		"owner-references": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "pod",
						Name:       "foo",
						UID:        "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d",
					},
				}
				return scheduling
			}(),
		},
		"finalizers": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Finalizers = []string{
					"example.com/foo",
				}
				return scheduling
			}(),
		},
		"managed-fields": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.ManagedFields = []metav1.ManagedFieldsEntry{
					{
						FieldsType: "FieldsV1",
						Operation:  "Apply",
						APIVersion: "apps/v1",
						Manager:    "foo",
					},
				}
				return scheduling
			}(),
		},
		"good-labels": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Labels = map[string]string{
					"apps.kubernetes.io/name": "test",
				}
				return scheduling
			}(),
		},
		"bad-labels": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "labels"), badValue, "a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')")},
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Labels = map[string]string{
					"hello-world": badValue,
				}
				return scheduling
			}(),
		},
		"good-annotations": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Annotations = map[string]string{
					"foo": "bar",
				}
				return scheduling
			}(),
		},
		"bad-annotations": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "annotations"), badName, "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')")},
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Annotations = map[string]string{
					badName: "hello world",
				}
				return scheduling
			}(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			errs := ValidatePodScheduling(scenario.scheduling)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}

func TestValidatePodSchedulingUpdate(t *testing.T) {
	validScheduling := testPodScheduling("foo", "ns", core.PodSchedulingSpec{})

	scenarios := map[string]struct {
		oldScheduling *core.PodScheduling
		update        func(scheduling *core.PodScheduling) *core.PodScheduling
		wantFailures  field.ErrorList
	}{
		"valid-no-op-update": {
			oldScheduling: validScheduling,
			update:        func(scheduling *core.PodScheduling) *core.PodScheduling { return scheduling },
		},
		"add-selected-node": {
			oldScheduling: validScheduling,
			update: func(scheduling *core.PodScheduling) *core.PodScheduling {
				scheduling.Spec.SelectedNode = "worker1"
				return scheduling
			},
		},
		"add-potential-nodes": {
			oldScheduling: validScheduling,
			update: func(scheduling *core.PodScheduling) *core.PodScheduling {
				for i := 0; i < core.PodSchedulingNodeListMaxSize; i++ {
					scheduling.Spec.PotentialNodes = append(scheduling.Spec.PotentialNodes, fmt.Sprintf("worker%d", i))
				}
				return scheduling
			},
		},
		"invalid-potential-nodes": {
			wantFailures:  field.ErrorList{field.TooLongMaxLength(field.NewPath("spec", "potentialNodes"), nil, core.PodSchedulingNodeListMaxSize)},
			oldScheduling: validScheduling,
			update: func(scheduling *core.PodScheduling) *core.PodScheduling {
				for i := 0; i < core.PodSchedulingNodeListMaxSize+1; i++ {
					scheduling.Spec.PotentialNodes = append(scheduling.Spec.PotentialNodes, fmt.Sprintf("worker%d", i))
				}
				return scheduling
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldScheduling.ResourceVersion = "1"
			errs := ValidatePodSchedulingUpdate(scenario.update(scenario.oldScheduling.DeepCopy()), scenario.oldScheduling)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}

func TestValidatePodSchedulingStatusUpdate(t *testing.T) {
	validScheduling := testPodScheduling("foo", "ns", core.PodSchedulingSpec{})

	scenarios := map[string]struct {
		oldScheduling *core.PodScheduling
		update        func(scheduling *core.PodScheduling) *core.PodScheduling
		wantFailures  field.ErrorList
	}{
		"valid-no-op-update": {
			oldScheduling: validScheduling,
			update:        func(scheduling *core.PodScheduling) *core.PodScheduling { return scheduling },
		},
		"add-claim-status": {
			oldScheduling: validScheduling,
			update: func(scheduling *core.PodScheduling) *core.PodScheduling {
				scheduling.Status.Claims = append(scheduling.Status.Claims,
					core.ResourceClaimSchedulingStatus{
						PodResourceClaimName: "my-claim",
					},
				)
				for i := 0; i < core.PodSchedulingNodeListMaxSize; i++ {
					scheduling.Status.Claims[0].UnsuitableNodes = append(
						scheduling.Status.Claims[0].UnsuitableNodes,
						fmt.Sprintf("worker%d", i),
					)
				}
				return scheduling
			},
		},
		"invalid-duplicated-claim-status": {
			wantFailures:  field.ErrorList{field.Duplicate(field.NewPath("status", "claims").Index(1), "my-claim")},
			oldScheduling: validScheduling,
			update: func(scheduling *core.PodScheduling) *core.PodScheduling {
				for i := 0; i < 2; i++ {
					scheduling.Status.Claims = append(scheduling.Status.Claims,
						core.ResourceClaimSchedulingStatus{PodResourceClaimName: "my-claim"},
					)
				}
				return scheduling
			},
		},
		"invalid-too-long-claim-status": {
			wantFailures:  field.ErrorList{field.TooLongMaxLength(field.NewPath("status", "claims").Index(0).Child("unsuitableNodes"), nil, core.PodSchedulingNodeListMaxSize)},
			oldScheduling: validScheduling,
			update: func(scheduling *core.PodScheduling) *core.PodScheduling {
				scheduling.Status.Claims = append(scheduling.Status.Claims,
					core.ResourceClaimSchedulingStatus{
						PodResourceClaimName: "my-claim",
					},
				)
				for i := 0; i < core.PodSchedulingNodeListMaxSize+1; i++ {
					scheduling.Status.Claims[0].UnsuitableNodes = append(
						scheduling.Status.Claims[0].UnsuitableNodes,
						fmt.Sprintf("worker%d", i),
					)
				}
				return scheduling
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldScheduling.ResourceVersion = "1"
			errs := ValidatePodSchedulingStatusUpdate(scenario.update(scenario.oldScheduling.DeepCopy()), scenario.oldScheduling)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}
