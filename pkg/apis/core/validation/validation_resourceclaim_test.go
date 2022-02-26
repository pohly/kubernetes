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

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/utils/pointer"
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
	validMode := core.AllocationModeImmediate
	invalidMode := core.AllocationMode("invalid")
	goodName := "foo"
	badName := "!@#$%^"
	goodNS := "ns"
	goodClaimSpec := core.ResourceClaimSpec{
		ResourceClassName: goodName,
		AllocationMode:    validMode,
	}
	now := metav1.Now()
	badValue := "spaces not allowed"

	scenarios := map[string]struct {
		claim        *core.ResourceClaim
		wantFailures field.ErrorList
	}{
		"good-claim": {
			claim: testResourceClaim(goodName, goodNS, goodClaimSpec),
		},
		"missing-name": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("metadata", "name"), "name or generateName is required")},
			claim:        testResourceClaim("", goodNS, goodClaimSpec),
		},
		"bad-name": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "name"), badName, "a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')")},
			claim:        testResourceClaim(badName, goodNS, goodClaimSpec),
		},
		"missing-namespace": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("metadata", "namespace"), "")},
			claim:        testResourceClaim(goodName, "", goodClaimSpec),
		},
		"generate-name": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.GenerateName = "pvc-"
				return claim
			}(),
		},
		"uid": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.UID = "ac051fac-2ead-46d9-b8b4-4e0fbeb7455d"
				return claim
			}(),
		},
		"resource-version": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.ResourceVersion = "1"
				return claim
			}(),
		},
		"generation": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Generation = 100
				return claim
			}(),
		},
		"creation-timestamp": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.CreationTimestamp = now
				return claim
			}(),
		},
		"deletion-grace-period-seconds": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.DeletionGracePeriodSeconds = pointer.Int64(10)
				return claim
			}(),
		},
		"owner-references": {
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
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Finalizers = []string{
					"example.com/foo",
				}
				return claim
			}(),
		},
		"managed-fields": {
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
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "labels"), badValue, "a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')")},
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Labels = map[string]string{
					"hello-world": badValue,
				}
				return claim
			}(),
		},
		"good-annotations": {
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Annotations = map[string]string{
					"foo": "bar",
				}
				return claim
			}(),
		},
		"bad-annotations": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("metadata", "annotations"), badName, "name part must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyName',  or 'my.name',  or '123-abc', regex used for validation is '([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]')")},
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Annotations = map[string]string{
					badName: "hello world",
				}
				return claim
			}(),
		},
		"bad-classname": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("spec", "resourceClassName"), badName, "a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')")},
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.ResourceClassName = badName
				return claim
			}(),
		},
		"bad-mode": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("spec", "allocationMode"), invalidMode, "unknown mode")},
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.AllocationMode = invalidMode
				return claim
			}(),
		},
		"good-parameters": {
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
			wantFailures: field.ErrorList{field.Required(field.NewPath("spec", "parametersRef", "kind"), "")},
			claim: func() *core.ResourceClaim {
				claim := testResourceClaim(goodName, goodNS, goodClaimSpec)
				claim.Spec.ParametersRef = &core.ResourceClaimParametersReference{
					Name: "bar",
				}
				return claim
			}(),
		},
		"missing-parameters-name": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("spec", "parametersRef", "name"), "")},
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
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}

func TestValidateResourceClaimUpdate(t *testing.T) {
	name := "valid"
	parameters := &core.ResourceClaimParametersReference{
		Kind: "foo",
		Name: "bar",
	}
	validClaim := testResourceClaim("foo", "ns", core.ResourceClaimSpec{
		ResourceClassName: name,
		AllocationMode:    core.AllocationModeImmediate,
		ParametersRef:     parameters,
	})

	scenarios := map[string]struct {
		oldClaim     *core.ResourceClaim
		update       func(claim *core.ResourceClaim) *core.ResourceClaim
		wantFailures field.ErrorList
	}{
		"valid-no-op-update": {
			oldClaim: validClaim,
			update:   func(claim *core.ResourceClaim) *core.ResourceClaim { return claim },
		},
		"invalid-update-class": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("spec"), func() core.ResourceClaimSpec {
				spec := validClaim.Spec.DeepCopy()
				spec.ResourceClassName += "2"
				return *spec
			}(), "field is immutable")},
			oldClaim: validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Spec.ResourceClassName += "2"
				return claim
			},
		},
		"invalid-update-remove-parameters": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("spec"), func() core.ResourceClaimSpec {
				spec := validClaim.Spec.DeepCopy()
				spec.ParametersRef = nil
				return *spec
			}(), "field is immutable")},
			oldClaim: validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Spec.ParametersRef = nil
				return claim
			},
		},
		"invalid-update-mode": {
			wantFailures: field.ErrorList{field.Invalid(field.NewPath("spec"), func() core.ResourceClaimSpec {
				spec := validClaim.Spec.DeepCopy()
				spec.AllocationMode = core.AllocationModeWaitForFirstConsumer
				return *spec
			}(), "field is immutable")},
			oldClaim: validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Spec.AllocationMode = core.AllocationModeWaitForFirstConsumer
				return claim
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldClaim.ResourceVersion = "1"
			errs := ValidateResourceClaimUpdate(scenario.update(scenario.oldClaim.DeepCopy()), scenario.oldClaim)
			assert.Equal(t, scenario.wantFailures, errs)
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
			ResourceHandle:    strings.Repeat(" ", core.ResourceHandleMaxSize),
			ShareableResource: true,
		},
	}

	scenarios := map[string]struct {
		oldClaim     *core.ResourceClaim
		update       func(claim *core.ResourceClaim) *core.ResourceClaim
		wantFailures field.ErrorList
	}{
		"valid-no-op-update": {
			oldClaim: validClaim,
			update:   func(claim *core.ResourceClaim) *core.ResourceClaim { return claim },
		},
		"add-driver": {
			oldClaim: validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				return claim
			},
		},
		"invalid-add-allocation": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("status", "driverName"), "must be set when a claim is allocated")},
			oldClaim:     validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				// DriverName must also get set here!
				claim.Status.Allocation = &core.AllocationResult{}
				return claim
			},
		},
		"valid-add-allocation": {
			oldClaim: validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				claim.Status.Allocation = &core.AllocationResult{
					ResourceHandle: strings.Repeat(" ", core.ResourceHandleMaxSize),
				}
				return claim
			},
		},
		"invalid-allocation-handle": {
			wantFailures: field.ErrorList{field.TooLongMaxLength(field.NewPath("status", "allocation", "resourceHandle"), core.ResourceHandleMaxSize+1, core.ResourceHandleMaxSize)},
			oldClaim:     validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DriverName = "valid"
				claim.Status.Allocation = &core.AllocationResult{
					ResourceHandle: strings.Repeat(" ", core.ResourceHandleMaxSize+1),
				}
				return claim
			},
		},
		"invalid-node-selector": {
			wantFailures: field.ErrorList{field.Required(field.NewPath("status", "allocation", "availableOnNodes", "nodeSelectorTerms"), "must have at least one node selector term")},
			oldClaim:     validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			oldClaim: validAllocatedClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			oldClaim: validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.TooLongMaxLength(field.NewPath("status", "reservedFor"), core.ResourceClaimReservedForMaxSize+1, core.ResourceClaimReservedForMaxSize)},
			oldClaim:     validAllocatedClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Duplicate(field.NewPath("status", "reservedFor").Index(1), core.ResourceClaimUserReference{
				Resource: "pods",
				Name:     "foo",
				UID:      "1",
			})},
			oldClaim: validAllocatedClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Forbidden(field.NewPath("status", "reservedFor"), "the claim cannot be reserved more than once")},
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				claim.Status.Allocation.ShareableResource = false
				return claim
			}(),
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Forbidden(field.NewPath("status", "reservedFor"), "only allocated claims can get reserved")},
			oldClaim:     validClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Required(field.NewPath("status", "reservedFor").Index(0).Child("resource"), "")},
			oldClaim:     validAllocatedClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Required(field.NewPath("status", "reservedFor").Index(0).Child("name"), "")},
			oldClaim:     validAllocatedClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Required(field.NewPath("status", "reservedFor").Index(0).Child("uid"), "")},
			oldClaim:     validAllocatedClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Forbidden(field.NewPath("status", "reservedFor"), "new entries may not get added while the claim is meant to be deallocated")},
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				var deletionTimestamp metav1.Time
				claim.DeletionTimestamp = &deletionTimestamp
				return claim
			}(),
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			wantFailures: field.ErrorList{field.Forbidden(field.NewPath("status", "reservedFor"), "new entries may not get added while the claim is meant to be deallocated")},
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				claim.Status.DeallocationRequested = true
				return claim
			}(),
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
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
			oldClaim: validAllocatedClaim,
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DeallocationRequested = true
				return claim
			},
		},
		"invalid-deallocation-requested-removal": {
			wantFailures: field.ErrorList{field.Forbidden(field.NewPath("status", "deallocationRequested"), "deallocation cannot be canceled because it may already have started")},
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				claim.Status.DeallocationRequested = true
				return claim
			}(),
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DeallocationRequested = false
				return claim
			},
		},
		"invalid-deallocation-requested-in-use": {
			wantFailures: field.ErrorList{field.Forbidden(field.NewPath("status", "deallocationRequested"), "deallocation cannot be requested for claims which are in use")},
			oldClaim: func() *core.ResourceClaim {
				claim := validAllocatedClaim.DeepCopy()
				claim.Status.ReservedFor = []core.ResourceClaimUserReference{
					{
						Resource: "pods",
						Name:     "foo",
						UID:      "1",
					},
				}
				return claim
			}(),
			update: func(claim *core.ResourceClaim) *core.ResourceClaim {
				claim.Status.DeallocationRequested = true
				return claim
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.oldClaim.ResourceVersion = "1"
			errs := ValidateResourceClaimStatusUpdate(scenario.update(scenario.oldClaim.DeepCopy()), scenario.oldClaim)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}
