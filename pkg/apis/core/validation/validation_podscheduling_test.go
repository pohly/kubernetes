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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/apis/core"
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
	ten := int64(10)

	scenarios := map[string]struct {
		isExpectedFailure bool
		scheduling        *core.PodScheduling
	}{
		"good-scheduling": {
			isExpectedFailure: false,
			scheduling:        testPodScheduling(goodName, goodNS, goodPodSchedulingSpec),
		},
		"missing-name": {
			isExpectedFailure: true,
			scheduling:        testPodScheduling("", goodNS, goodPodSchedulingSpec),
		},
		"bad-name": {
			isExpectedFailure: true,
			scheduling:        testPodScheduling("!@#$%^", goodNS, goodPodSchedulingSpec),
		},
		"missing-namespace": {
			isExpectedFailure: true,
			scheduling:        testPodScheduling(goodName, "", goodPodSchedulingSpec),
		},
		"generate-name": {
			isExpectedFailure: false,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.GenerateName = "pvc-"
				return scheduling
			}(),
		},
		"uid": {
			isExpectedFailure: false,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return scheduling
			}(),
		},
		"resource-version": {
			isExpectedFailure: false,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.ResourceVersion = "1"
				return scheduling
			}(),
		},
		"generation": {
			isExpectedFailure: false,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Generation = 100
				return scheduling
			}(),
		},
		"creation-timestamp": {
			isExpectedFailure: false,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.CreationTimestamp = now
				return scheduling
			}(),
		},
		"deletion-grace-period-seconds": {
			isExpectedFailure: false,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.DeletionGracePeriodSeconds = &ten
				return scheduling
			}(),
		},
		"owner-references": {
			isExpectedFailure: false,
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
			isExpectedFailure: false,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Finalizers = []string{
					"example.com/foo",
				}
				return scheduling
			}(),
		},
		"managed-fields": {
			isExpectedFailure: false,
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
			isExpectedFailure: true,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return scheduling
			}(),
		},
		"good-annotations": {
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Labels = map[string]string{
					"foo": "bar",
				}
				return scheduling
			}(),
		},
		"bad-annotations": {
			isExpectedFailure: true,
			scheduling: func() *core.PodScheduling {
				scheduling := testPodScheduling(goodName, goodNS, goodPodSchedulingSpec)
				scheduling.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return scheduling
			}(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			errs := ValidatePodScheduling(scenario.scheduling)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Error("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}

func TestValidatePodSchedulingUpdate(t *testing.T) {
	validScheduling := testPodScheduling("foo", "ns", core.PodSchedulingSpec{})

	scenarios := map[string]struct {
		isExpectedFailure bool
		oldScheduling     *core.PodScheduling
		newScheduling     func(scheduling *core.PodScheduling) *core.PodScheduling
	}{
		"valid-no-op-update": {
			isExpectedFailure: false,
			oldScheduling:     validScheduling,
			newScheduling:     func(scheduling *core.PodScheduling) *core.PodScheduling { return scheduling },
		},
		"add-selected-node": {
			isExpectedFailure: false,
			oldScheduling:     validScheduling,
			newScheduling: func(scheduling *core.PodScheduling) *core.PodScheduling {
				scheduling.Spec.SelectedNode = "worker1"
				return scheduling
			},
		},
		"add-potential-nodes": {
			isExpectedFailure: false,
			oldScheduling:     validScheduling,
			newScheduling: func(scheduling *core.PodScheduling) *core.PodScheduling {
				for i := 0; i < core.PodSchedulingNodeListMaxSize; i++ {
					scheduling.Spec.PotentialNodes = append(scheduling.Spec.PotentialNodes, fmt.Sprintf("worker%d", i))
				}
				return scheduling
			},
		},
		"invalid-potential-nodes": {
			isExpectedFailure: true,
			oldScheduling:     validScheduling,
			newScheduling: func(scheduling *core.PodScheduling) *core.PodScheduling {
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
			errs := ValidatePodSchedulingUpdate(scenario.newScheduling(scenario.oldScheduling.DeepCopy()), scenario.oldScheduling)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}

func TestValidatePodSchedulingStatusUpdate(t *testing.T) {
	validScheduling := testPodScheduling("foo", "ns", core.PodSchedulingSpec{})

	scenarios := map[string]struct {
		isExpectedFailure bool
		oldScheduling     *core.PodScheduling
		newScheduling     func(scheduling *core.PodScheduling) *core.PodScheduling
	}{
		"valid-no-op-update": {
			isExpectedFailure: false,
			oldScheduling:     validScheduling,
			newScheduling:     func(scheduling *core.PodScheduling) *core.PodScheduling { return scheduling },
		},
		"add-claim-status": {
			isExpectedFailure: false,
			oldScheduling:     validScheduling,
			newScheduling: func(scheduling *core.PodScheduling) *core.PodScheduling {
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
			isExpectedFailure: true,
			oldScheduling:     validScheduling,
			newScheduling: func(scheduling *core.PodScheduling) *core.PodScheduling {
				for i := 0; i < 2; i++ {
					scheduling.Status.Claims = append(scheduling.Status.Claims,
						core.ResourceClaimSchedulingStatus{PodResourceClaimName: "my-claim"},
					)
				}
				return scheduling
			},
		},
		"invalid-too-long-claim-status": {
			isExpectedFailure: true,
			oldScheduling:     validScheduling,
			newScheduling: func(scheduling *core.PodScheduling) *core.PodScheduling {
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
			errs := ValidatePodSchedulingStatusUpdate(scenario.newScheduling(scenario.oldScheduling.DeepCopy()), scenario.oldScheduling)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}
