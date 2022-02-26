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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/apis/core"
)

func testResourceClaim(name, namespace string, spec core.ResourceClaimSpec) *core.ResourceClaim {
	return &core.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

func TestValidateResourceClaim(t *testing.T) {
	invalidClassName := "-invalid-"
	validClassName := "valid"
	validMode := core.AllocationModeImmediate
	invalidMode := core.AllocationMode("invalid")
	goodName := "foo"
	goodNS := "ns"
	goodClaimSpec := core.ResourceClaimSpec{
		ResourceClassName: validClassName,
		AllocationMode:    validMode,
	}
	now := metav1.Now()
	ten := int64(10)

	scenarios := map[string]struct {
		isExpectedFailure bool
		claim             *core.ResourceClaim
	}{
		"good-claim": {
			isExpectedFailure: false,
			claim:             testResourceClaim(goodName, goodNS, goodClaimSpec),
		},
		"missing-name": {
			isExpectedFailure: true,
			claim:             testResourceClaim("", goodNS, goodClaimSpec),
		},
		"bad-name": {
			isExpectedFailure: true,
			claim:             testResourceClaim("!@#$%^", goodNS, goodClaimSpec),
		},
		"missing-namespace": {
			isExpectedFailure: true,
			claim:             testResourceClaim(goodName, "", goodClaimSpec),
		},
		"generate-name": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.GenerateName = "pvc-"
				return claim
			}(),
		},
		"uid": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return claim
			}(),
		},
		"resource-version": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.ResourceVersion = "1"
				return claim
			}(),
		},
		"generation": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Generation = 100
				return claim
			}(),
		},
		"creation-timestamp": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.CreationTimestamp = now
				return claim
			}(),
		},
		"deletion-grace-period-seconds": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.DeletionGracePeriodSeconds = &ten
				return claim
			}(),
		},
		"owner-references": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "pod",
						Name:       "foo",
						UID:        "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d",
					},
				}
				return claim
			}(),
		},
		"finalizers": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Finalizers = []string{
					"example.com/foo",
				}
				return claim
			}(),
		},
		"managed-fields": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.ManagedFields = []metav1.ManagedFieldsEntry{
					{
						FieldsType: "FieldsV1",
						Operation:  "Apply",
						APIVersion: "apps/v1",
						Manager:    "foo",
					},
				}
				return claim
			}(),
		},
		"good-labels": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"apps.kubernetes.io/name": "test",
				}
				return claim
			}(),
		},
		"bad-labels": {
			isExpectedFailure: true,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return claim
			}(),
		},
		"good-annotations": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"foo": "bar",
				}
				return claim
			}(),
		},
		"bad-annotations": {
			isExpectedFailure: true,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"hello-world": "hyphen not allowed",
				}
				return claim
			}(),
		},
		"bad-classname": {
			isExpectedFailure: true,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.ResourceClassName = invalidClassName
				return claim
			}(),
		},
		"bad-mode": {
			isExpectedFailure: true,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.AllocationMode = invalidMode
				return claim
			}(),
		},
		"good-parameters": {
			isExpectedFailure: false,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.ParametersRef = &core.ResourceClaimParametersReference{
					Kind: "foo",
					Name: "bar",
				}
				return claim
			}(),
		},
		"missing-parameters-kind": {
			isExpectedFailure: true,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.ParametersRef = &core.ResourceClaimParametersReference{
					Name: "bar",
				}
				return claim
			}(),
		},
		"missing-parameters-name": {
			isExpectedFailure: true,
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.ParametersRef = &core.ResourceClaimParametersReference{
					Kind: "foo",
				}
				return claim
			}(),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			errs := ValidateResourceClaim(scenario.claim)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Error("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}

func TestValidateResourceClaimUpdate(t *testing.T) {
	validClaim := testResourceClaim("foo", "ns", core.ResourceClaimSpec{
		ResourceClassName: "valid",
		AllocationMode:    core.AllocationModeImmediate,
		ParametersRef: &core.ResourceClaimParametersReference{
			Kind: "foo",
			Name: "bar",
		},
	})

	scenarios := map[string]struct {
		isExpectedFailure bool
		oldClaim          *core.ResourceClaim
		newClaim          func(claim *core.ResourceClaim) *core.ResourceClaim
	}{
		"valid-no-op-update": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim:          func(claim *core.ResourceClaim) *core.ResourceClaim { return claim },
		},
		"invalid-update-class": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Spec.ResourceClassName += "2"
				return claim
			},
		},
		"invalid-update-remove-parameters": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Spec.ParametersRef = nil
				return claim
			},
		},
		"invalid-update-mode": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Spec.AllocationMode = core.AllocationModeWaitForFirstConsumer
				return claim
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldClaim.ResourceVersion = "1"
			errs := ValidateResourceClaimUpdate(scenario.newClaim(scenario.oldClaim.DeepCopy()), scenario.oldClaim)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}

func TestValidateResourceClaimStatusUpdate(t *testing.T) {
	validClaim := testResourceClaim("foo", "ns", core.ResourceClaimSpec{
		ResourceClassName: "valid",
		AllocationMode:    core.AllocationModeImmediate,
	})

	validAllocatedClaim := validClaim.DeepCopy()
	validAllocatedClaim.Status = core.ResourceClaimStatus{
		DriverName: "valid",
		Allocation: &core.AllocationResult{
			ResourceHandle: strings.Repeat(" ", core.ResourceHandleMaxSize),
			SharedResource: true,
		},
	}

	scenarios := map[string]struct {
		isExpectedFailure bool
		oldClaim          *core.ResourceClaim
		newClaim          func(claim *core.ResourceClaim) *core.ResourceClaim
	}{
		"valid-no-op-update": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim:          func(claim *core.ResourceClaim) *core.ResourceClaim { return claim },
		},
		"add-driver": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				return claim
			},
		},
		"invalid-add-allocation": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				// DriverName must also get set here!
				claim.Status.Allocation = &core.AllocationResult{}
				return claim
			},
		},
		"valid-add-allocation": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				claim.Status.Allocation = &core.AllocationResult{
					ResourceHandle: strings.Repeat(" ", core.ResourceHandleMaxSize),
				}
				return claim
			},
		},
		"invalid-allocation-handle": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				claim.Status.Allocation = &core.AllocationResult{
					ResourceHandle: strings.Repeat(" ", core.ResourceHandleMaxSize+1),
				}
				return claim
			},
		},
		"invalid-node-selector": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				claim.Status.Allocation = &core.AllocationResult{
					AvailableOnNodes: &core.NodeSelector{
						// Must not be empty.
					},
				}
				return claim
			},
		},
		"add-reservation": {
			isExpectedFailure: false,
			oldClaim:          validAllocatedClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				for i := 0; i < core.ResourceClaimReservedForMaxSize; i++ {
					claim.Status.ReservedFor = append(claim.Status.ReservedFor,
						core.ResourceClaimUserReference{
							Resource: "pods",
							Name:     fmt.Sprintf("foo-%d", i),
							UID:      "1",
						})
				}
				return claim
			},
		},
		"add-reservation-and-allocation": {
			isExpectedFailure: false,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status = *validAllocatedClaim.Status.DeepCopy()
				for i := 0; i < core.ResourceClaimReservedForMaxSize; i++ {
					claim.Status.ReservedFor = append(claim.Status.ReservedFor,
						core.ResourceClaimUserReference{
							Resource: "pods",
							Name:     fmt.Sprintf("foo-%d", i),
							UID:      "1",
						})
				}
				return claim
			},
		},
		"invalid-reserved-for-too-large": {
			isExpectedFailure: true,
			oldClaim:          validAllocatedClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				for i := 0; i < core.ResourceClaimReservedForMaxSize+1; i++ {
					claim.Status.ReservedFor = append(claim.Status.ReservedFor,
						core.ResourceClaimUserReference{
							Resource: "pods",
							Name:     fmt.Sprintf("foo-%d", i),
							UID:      "1",
						})
				}
				return claim
			},
		},
		"invalid-reserved-for-duplicate": {
			isExpectedFailure: true,
			oldClaim:          validAllocatedClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				for i := 0; i < 2; i++ {
					claim.Status.ReservedFor = append(claim.Status.ReservedFor,
						core.ResourceClaimUserReference{
							Resource: "pods",
							Name:     "foo",
							UID:      "1",
						})
				}
				return claim
			},
		},
		"invalid-reserved-for-not-shared": {
			isExpectedFailure: true,
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				claim.Status.Allocation.SharedResource = false
				return claim
			}(),
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				for i := 0; i < 2; i++ {
					claim.Status.ReservedFor = append(claim.Status.ReservedFor,
						core.ResourceClaimUserReference{
							Resource: "pods",
							Name:     fmt.Sprintf("foo-%d", i),
							UID:      "1",
						})
				}
				return claim
			},
		},
		"invalid-reserved-for-no-allocation": {
			isExpectedFailure: true,
			oldClaim:          validClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				claim.Status.ReservedFor = []core.ResourceClaimUserReference{
					{
						Resource: "pods",
						Name:     "foo",
						UID:      "1",
					},
				}
				return claim
			},
		},
		"invalid-reserved-for-no-resource": {
			isExpectedFailure: true,
			oldClaim:          validAllocatedClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.ReservedFor = []core.ResourceClaimUserReference{
					{
						Name: "foo",
						UID:  "1",
					},
				}
				return claim
			},
		},
		"invalid-reserved-for-no-name": {
			isExpectedFailure: true,
			oldClaim:          validAllocatedClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.ReservedFor = []core.ResourceClaimUserReference{
					{
						Resource: "pods",
						UID:      "1",
					},
				}
				return claim
			},
		},
		"invalid-reserved-for-no-uid": {
			isExpectedFailure: true,
			oldClaim:          validAllocatedClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.ReservedFor = []core.ResourceClaimUserReference{
					{
						Resource: "pods",
						Name:     "foo",
					},
				}
				return claim
			},
		},
		"invalid-reserved-deleted": {
			isExpectedFailure: true,
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				var deletionTimestamp metav1.Time
				claim.DeletionTimestamp = &deletionTimestamp
				return claim
			}(),
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.ReservedFor = []core.ResourceClaimUserReference{
					{
						Resource: "pods",
						Name:     "foo",
						UID:      "1",
					},
				}
				return claim
			},
		},
		"invalid-reserved-deallocation-requested": {
			isExpectedFailure: true,
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				claim.Status.DeallocationRequested = true
				return claim
			}(),
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.ReservedFor = []core.ResourceClaimUserReference{
					{
						Resource: "pods",
						Name:     "foo",
						UID:      "1",
					},
				}
				return claim
			},
		},
		"add-deallocation-requested": {
			isExpectedFailure: false,
			oldClaim:          validAllocatedClaim,
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DeallocationRequested = true
				return claim
			},
		},
		"invalid-deallocation-requested-removal": {
			isExpectedFailure: true,
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				claim.Status.DeallocationRequested = true
				return claim
			}(),
			newClaim: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DeallocationRequested = false
				return claim
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldClaim.ResourceVersion = "1"
			errs := ValidateResourceClaimStatusUpdate(scenario.newClaim(scenario.oldClaim.DeepCopy()), scenario.oldClaim)
			if len(errs) == 0 && scenario.isExpectedFailure {
				t.Errorf("Unexpected success")
			}
			if len(errs) > 0 && !scenario.isExpectedFailure {
				t.Errorf("Unexpected failure: %+v", errs)
			}
		})
	}
}
