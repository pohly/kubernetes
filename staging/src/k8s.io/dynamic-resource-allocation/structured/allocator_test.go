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

package structured

import (
	"errors"
	"flag"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1alpha3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2/ktesting"
	"sigs.k8s.io/yaml"
)

func init() {
	ktesting.DefaultConfig.AddFlags(flag.CommandLine)
}

func TestAllocator(t *testing.T) {
	// Class with DeviceClassSpec.SuitableNodes selector.
	driverA := "driver-a"
	driverAClass := &resourceapi.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: driverA,
		},
		Spec: resourceapi.DeviceClassSpec{
			Selectors: []resourceapi.DeviceSelector{
				{
					CEL: &resourceapi.CELDeviceSelector{
						Expression: `device.driver == "driver-a"`,
					},
				},
			},
			SuitableNodes: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "region",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"west", "east"},
							},
						},
					},
				},
			},
		},
	}

	driverAClassWithConfig := &resourceapi.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: driverA,
		},
		Spec: resourceapi.DeviceClassSpec{
			Config: []resourceapi.DeviceClassConfiguration{
				{
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: &resourceapi.OpaqueDeviceConfiguration{
							Driver: driverA,
							Parameters: runtime.RawExtension{
								Raw: []byte(`{"classAttribute":"classAttributeValue"}`),
							},
						},
					},
				},
			},
			Selectors: []resourceapi.DeviceSelector{
				{
					CEL: &resourceapi.CELDeviceSelector{
						Expression: `device.driver == "driver-a"`,
					},
				},
			},
			SuitableNodes: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "region",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"west", "east"},
							},
						},
					},
				},
			},
		},
	}

	// Class without DeviceClassSpec.SuitableNodes selector.
	driverB := "driver-b"
	driverBClass := &resourceapi.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: driverB,
		},
		Spec: resourceapi.DeviceClassSpec{
			Selectors: []resourceapi.DeviceSelector{
				{
					CEL: &resourceapi.CELDeviceSelector{
						Expression: `device.driver == "driver-b"`,
					},
				},
			},
		},
	}
	// device1 := "device-1"
	// device2 := "device-2"
	// driverVersion := "driverVersion"
	// memory := "memory"
	// oneGig := resource.NewQuantity(1024*1024, resource.BinarySI)
	// numa := "numa"
	// numa1 := ptr.To(int64(1))
	// numa2 := ptr.To(int64(2))
	node1 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Labels: map[string]string{
				"region": "west",
			},
		},
	}
	node2 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-2",
			Labels: map[string]string{
				"region": "east",
			},
		},
	}
	// regionWest := v1.NodeSelector{
	// 	NodeSelectorTerms: []v1.NodeSelectorTerm{{
	// 		MatchExpressions: []v1.NodeSelectorRequirement{{
	// 			Key:      "region",
	// 			Operator: v1.NodeSelectorOpIn,
	// 			Values:   []string{"west"},
	// 		}},
	// 	}},
	// }
	// TODO: use Go to define objects or YAML?
	//
	// Go is very verbose, but surfaces errors at compile time.
	// YAML is more compact and supports global search/replace
	// to produce new objects that are derived from others.
	//
	// node1Slice := &resourceapi.ResourceSlice{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name: "node1Slice",
	// 	},
	// 	Spec: resourceapi.ResourceSliceSpec{
	// 		NodeName: node1.Name,
	// 		Driver:   driverA,
	// 		Pool: resourceapi.ResourcePool{
	// 			Name:               node1.Name,
	// 			ResourceSliceCount: 1,
	// 		},
	// 		Devices: []resourceapi.Device{
	// 			{
	// 				Name: device1,
	// 				Attributes: []resourceapi.DeviceAttribute{
	// 					{
	// 						Name:         driverVersion,
	// 						VersionValue: ptr.To("1.0.0"),
	// 					},
	// 					{
	// 						Name:     numa,
	// 						IntValue: numa1,
	// 					},
	// 				},
	// 				Capacities: []resourceapi.DeviceCapacity{
	// 					{
	// 						Name:     memory,
	// 						Quantity: oneGig,
	// 					},
	// 				},
	// 			},
	// 		},
	// 	},
	// }

	// Slices

	node1slice := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-1-slice
spec:
  nodeName: worker-1
  driver: driver-a
  pool:
    name: worker-1
    resourceSliceCount: 2
    generation: 2
  devices:
  - name: device-1
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
        stringAttribute:
          string: stringAttributeValue
        boolAttribute:
          bool: true
      capacity:
        memory: 1Gi # small
  - name: device-2
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
        stringAttribute:
          string: stringAttributeValue
        boolAttribute:
          bool: true
      capacity:
        memory: 2Gi # large
`)

	// Second slice on the same node, with different device names
	// and attributes.
	node1slice2 := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-1-2-slice
spec:
  nodeName: worker-1
  driver: driver-a
  pool:
    name: worker-1
    generation: 2 # recent pool generation with 2 slices and 4 devices
    resourceSliceCount: 1
  devices:
  - name: device-5
    basic:
      attributes:
        driverVersion:
          version: 2.0.0
        numa:
          int: 2
        stringAttribute:
          string: stringAttributeValue2
        boolAttribute:
          bool: false
      capacity:
        memory: 1Gi # small
  - name: device-6
    basic:
      attributes:
        driverVersion:
          version: 2.0.0
        numa:
          int: 2
        stringAttribute:
          string: stringAttributeValue2
        boolAttribute:
          bool: false
      capacity:
        memory: 2Gi # large
`)

	node1ObsoleteSlice := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-1-obsolete-slice
spec:
  nodeName: worker-1
  driver: driver-a
  pool:
    name: worker-1
    generation: 1
    resourceSliceCount: 1
  devices:
  - name: device-1
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 1Gi # small
`)

	node1IncompletePoolSlice := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-1-incomplete-slice
spec:
  nodeName: worker-1
  driver: driver-a
  pool:
    name: worker-1
    generation: 2
    resourceSliceCount: 2
  devices:
  - name: device-1
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 1Gi # small
`)

	networkAttachedSliceWest := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-1-slice
spec:
  driver: driver-a
  pool:
    name: worker-1
    resourceSliceCount: 2
    generation: 2
  nodeSelector:
    nodeSelectorTerms:
    - matchExpressions:
      - key: region
        operator: In
        values: [west]
  devices:
  - name: device-1
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 1Gi # small
  - name: device-2
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 2Gi # large
`)

	networkAttachedSliceEast := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-2-slice
spec:
  driver: driver-b
  pool:
    name: worker-2
    resourceSliceCount: 2
    generation: 2
  nodeSelector:
    nodeSelectorTerms:
    - matchExpressions:
      - key: region
        operator: In
        values: [east]
  devices:
  - name: device-1
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 1Gi # small
  - name: device-2
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 2Gi # large
`)

	node1DriverBslice := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-1-driverB-slice
spec:
  nodeName: worker-1
  driver: driver-b
  pool:
    name: worker-1
    generation: 1
    resourceSliceCount: 1
  devices:
  - name: device-1
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 1Gi # small
  - name: device-2
    basic:
      attributes:
        driverVersion:
          version: 1.0.0
        numa:
          int: 1
      capacity:
        memory: 2Gi # large
`)

	node2slice := unmarshal[resourceapi.ResourceSlice](t, `
metadata:
  name: worker-2-slice
spec:
  nodeName: worker-2
  driver: driver-a
  pool:
    name: worker-2
    resourceSliceCount: 1
  devices:
  - name: device-3
    basic:
      attributes:
        driverVersion:
          version: 2.0.0
        numa:
          int: 1
      capacity:
        memory: 1Gi # small
  - name: device-4
    basic:
      attributes:
        driverVersion:
          version: 2.0.0
        numa:
          int: 1
      capacity:
        memory: 2Gi # large
`)

	// Claims

	simpleClaim := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: driver-a
`)

	simpleClaimDriverB := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: driver-b
`)

	simpleClaimWithConfig := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: driver-a
    config:
    - opaque:
        driver: driver-a
        parameters: {"deviceAttribute":"deviceAttributeValue"}
`)

	simpleClaimWithUnknownClassName := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: unknown-class
`)

	simpleClaimWithEmptyClassName := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: ""
`)

	allocatedSimpleClaim := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-a
    pool: worker-1
    device: device-1
`)

	allocatedSimpleClaimWithClassConfig := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-a
    pool: worker-1
    device: device-1
  config:
  - source: FromClass
    opaque:
      driver: driver-a
      parameters:
        classAttribute: classAttributeValue
`)

	allocatedSimpleClaimWithClaimConfig := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-a
    pool: worker-1
    device: device-1
  config:
  - source: FromClaim
    opaque:
      driver: driver-a
      parameters:
        deviceAttribute: deviceAttributeValue
`)

	allocatedSimpleClaimDriverB := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-b
    pool: worker-2
    device: device-1
`)

	allocatedSimpleClaimDriverBWorker1 := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-b
    pool: worker-1
    device: device-1
`)

	allocatedSimpleClaimNode2 := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-a
    pool: worker-2
    device: device-3
`)

	allocated2DeviceClaim := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
    - request: req-0
      driver: driver-a
      pool: worker-1
      device: device-5
    - request: req-0
      driver: driver-a
      pool: worker-1
      device: device-6
`)

	allocatedClaim := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 2
      deviceClassName: driver-a
status:
  allocation:
    devices:
      results:
      - request: req-0
        driver: driver-a
        pool: worker-1
        device: device-1
      - request: req-1
        driver: driver-a
        pool: worker-1
        device: device-2
`)

	twoDeviceClaim := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: driver-a
      selectors:
      - cel:
          # small
          expression: device.capacity["driver-a"].memory.compareTo(quantity("1Gi")) >= 0
    - name: req-1
      countMode: Exact
      count: 1
      deviceClassName: driver-a
      selectors:
      - cel:
          # large
          expression: device.capacity["driver-a"].memory.compareTo(quantity("2Gi")) >= 0
`)

	twoDeviceClaimWithConstraint := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: driver-a
    - name: req-1
      countMode: Exact
      count: 1
      deviceClassName: driver-a
    constraints:
    - matchAttribute: numa
    - matchAttribute: driverVersion
    - matchAttribute: stringAttribute
    - matchAttribute: boolAttribute
`)

	claimWithConstraintNotMatchingIntAttribute := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 3
      deviceClassName: driver-a
    constraints:
    - matchAttribute: numa
`)

	claimWithConstraintNotMatchingVersionAttribute := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 3
      deviceClassName: driver-a
    constraints:
    - matchAttribute: driverVersion	
`)

	claimWithConstraintNotMatchingStringAttribute := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 3
      deviceClassName: driver-a
    constraints:
    - matchAttribute: stringAttribute	
`)

	claimWithConstraintNotMatchingBoolAttribute := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 3
      deviceClassName: driver-a
    constraints:
    - matchAttribute: boolAttribute
`)

	fourDeviceClaim := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 2
      deviceClassName: driver-a
      selectors:
      - cel:
          # small
          expression: device.capacity["driver-a"].memory.compareTo(quantity("1Gi")) >= 0
    - name: req-1
      countMode: Exact
      count: 2
      deviceClassName: driver-a
      selectors:
      - cel:
          # large
          expression: device.capacity["driver-a"].memory.compareTo(quantity("2Gi")) >= 0
`)

	allDeviceClaim := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: All
      deviceClassName: driver-a
      selectors:
      - cel:
          # small
          expression: device.capacity["driver-a"].memory.compareTo(quantity("1Gi")) >= 0
`)

	simpleClaimWithConstraintNonExistentAttribute := unmarshal[resourceapi.ResourceClaim](t, `
metadata:
  name: claim
spec:
  devices:
    requests:
    - name: req-0
      countMode: Exact
      count: 1
      deviceClassName: driver-a
    constraints:
    - matchAttribute: NonExistentAttribute
`)

	allocatedTwoDeviceClaim := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-a
    pool: worker-1
    device: device-1
  - request: req-1
    driver: driver-a
    pool: worker-1
    device: device-2
`)

	allocatedFourDeviceClaim := unmarshal[resourceapi.AllocationResult](t, `
devices:
  results:
  - request: req-0
    driver: driver-a
    pool: worker-1
    device: device-1
  - request: req-0
    driver: driver-a
    pool: worker-1
    device: device-5
  - request: req-1
    driver: driver-a
    pool: worker-1
    device: device-2
  - request: req-1
    driver: driver-a
    pool: worker-1
    device: device-6
`)

	testcases := map[string]struct {
		claimsToAllocate []*resourceapi.ResourceClaim
		allocatedClaims  []*resourceapi.ResourceClaim
		classes          []*resourceapi.DeviceClass
		slices           []*resourceapi.ResourceSlice
		node             *v1.Node

		expectResults []any
		expectError   types.GomegaMatcher // can be used to check for no error or match specific error types
	}{

		"empty": {},
		"simple": {
			claimsToAllocate: objects(simpleClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1slice, node2slice),
			node:             node1,

			expectResults: []any{allocatedSimpleClaim},
		},
		"other-node": {
			claimsToAllocate: objects(simpleClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1slice, node2slice),
			node:             node2,

			expectResults: []any{allocatedSimpleClaimNode2},
		},
		"small-and-large": {
			claimsToAllocate: objects(twoDeviceClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1slice, node2slice),
			node:             node1,

			expectResults: []any{allocatedTwoDeviceClaim},
		},
		"small-and-large-backtrack": {
			claimsToAllocate: objects(twoDeviceClaim),
			classes:          objects(driverAClass),
			// Reversing the order in which the devices are listed causes the "large" device to
			// be allocated for the "small" request, leaving the "large" request unsatisfied.
			// The initial decision needs to be undone before a solution is found.
			slices: objects(func() *resourceapi.ResourceSlice {
				slice := node1slice.DeepCopy()
				slice.Spec.Devices[0], slice.Spec.Devices[1] = slice.Spec.Devices[1], slice.Spec.Devices[0]
				return slice
			}()),
			node: node1,

			expectResults: []any{allocatedTwoDeviceClaim},
		},
		"devices-split-across-different-slices": {
			claimsToAllocate: objects(fourDeviceClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1slice, node1slice2),
			node:             node1,

			expectResults: []any{allocatedFourDeviceClaim},
		},
		"obsolete-slice": {
			claimsToAllocate: objects(simpleClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1ObsoleteSlice, node1slice),
			node:             node1,

			expectResults: []any{allocatedSimpleClaim},
		},
		"no-slices": {
			claimsToAllocate: objects(simpleClaim),
			classes:          objects(driverAClass),
			slices:           nil,
			node:             node1,

			expectResults: nil,
		},
		"not-enough-suitable-devices": {
			claimsToAllocate: objects(fourDeviceClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1slice),
			node:             node1,

			expectResults: nil,
		},
		"no-classes": {
			claimsToAllocate: objects(simpleClaim),
			classes:          nil,
			slices:           objects(node1slice),
			node:             node1,

			expectResults: nil,
			expectError:   gomega.MatchError(gomega.ContainSubstring("could not retrieve device class driver-a")),
		},
		"unknown-class": {
			claimsToAllocate: objects(simpleClaimWithUnknownClassName),
			classes:          objects(driverAClass, driverBClass),
			slices:           objects(node1slice, node1DriverBslice),
			node:             node1,

			expectResults: nil,
			expectError:   gomega.MatchError(gomega.ContainSubstring("could not retrieve device class unknown-class")),
		},
		"empty-class": {
			claimsToAllocate: objects(simpleClaimWithEmptyClassName),
			classes:          objects(driverAClass, driverBClass),
			slices:           objects(node1slice, node1DriverBslice),
			node:             node1,

			expectResults: nil,
			expectError:   gomega.MatchError(gomega.ContainSubstring("claim claim, request req-0: unsupported request type")),
		},
		"no-claims-to-allocate": {
			claimsToAllocate: nil,
			classes:          objects(driverAClass),
			slices:           objects(node1slice),
			node:             node1,

			expectResults: nil,
		},
		"all-devices": {
			claimsToAllocate: objects(allDeviceClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1slice2),
			node:             node1,

			expectResults: []any{allocated2DeviceClaim},
		},
		"all-devices-of-the-incomplete-pool": {
			claimsToAllocate: objects(allDeviceClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1IncompletePoolSlice),
			node:             node1,

			expectResults: nil,
			expectError:   gomega.MatchError(gomega.ContainSubstring("claim claim, request req-0: asks for all devices, but resource pool driver-a/worker-1 is currently being updated")),
		},
		"network-attached-device-with-class.SuitableNodes": {
			claimsToAllocate: objects(simpleClaim),
			classes:          objects(driverAClass),
			slices:           objects(networkAttachedSliceWest),
			node:             node1,

			expectResults: []any{allocatedSimpleClaim},
		},
		"network-attached-device-without-class.SuitableNodes": {
			claimsToAllocate: objects(simpleClaimDriverB),
			classes:          objects(driverBClass),
			slices:           objects(networkAttachedSliceEast),
			node:             node2,

			expectResults: []any{allocatedSimpleClaimDriverB},
		},
		"unsuccessful-allocation-network-attached-device-with-class.SuitableNodes": {
			claimsToAllocate: objects(simpleClaim),
			classes:          objects(driverAClass),
			slices:           objects(networkAttachedSliceEast),
			node:             node1,

			expectResults: nil,
		},
		"unsuccessful-allocation-network-attached-device-without-class.SuitableNodes": {
			claimsToAllocate: objects(simpleClaimDriverB),
			classes:          objects(driverBClass),
			slices:           objects(networkAttachedSliceWest),
			node:             node2,

			expectResults: nil,
		},
		"several-different-drivers": {
			claimsToAllocate: objects(simpleClaim, simpleClaimDriverB),
			classes:          objects(driverAClass, driverBClass),
			slices:           objects(node1slice, node1DriverBslice),
			node:             node1,

			expectResults: []any{allocatedSimpleClaim, allocatedSimpleClaimDriverBWorker1},
		},
		"already-allocated-devices": {
			claimsToAllocate: objects(simpleClaim),
			allocatedClaims:  objects(allocatedClaim),
			classes:          objects(driverAClass),
			slices:           objects(node1slice),
			node:             node1,

			expectResults: nil,
		},
		"with-constraint": {
			claimsToAllocate: objects(twoDeviceClaimWithConstraint),
			classes:          objects(driverAClass),
			slices:           objects(node1slice),
			node:             node1,

			expectResults: []any{allocatedTwoDeviceClaim},
		},
		"with-constraint-non-existent-attribute": {
			claimsToAllocate: objects(simpleClaimWithConstraintNonExistentAttribute),
			classes:          objects(driverAClass),
			slices:           objects(node1slice),
			node:             node1,

			expectResults: nil,
		},
		"with-constraint-not-matching-int-attribute": {
			claimsToAllocate: objects(claimWithConstraintNotMatchingIntAttribute),
			classes:          objects(driverAClass, driverBClass),
			slices:           objects(node1slice, node1slice2),
			node:             node1,

			expectResults: nil,
		},
		"with-constraint-not-matching-version-attribute": {
			claimsToAllocate: objects(claimWithConstraintNotMatchingVersionAttribute),
			classes:          objects(driverAClass, driverBClass),
			slices:           objects(node1slice, node1slice2),
			node:             node1,

			expectResults: nil,
		},
		"with-constraint-not-matching-string-attribute": {
			claimsToAllocate: objects(claimWithConstraintNotMatchingStringAttribute),
			classes:          objects(driverAClass, driverBClass),
			slices:           objects(node1slice, node1slice2),
			node:             node1,

			expectResults: nil,
		},
		"with-constraint-not-matching-bool-attribute": {
			claimsToAllocate: objects(claimWithConstraintNotMatchingBoolAttribute),
			classes:          objects(driverAClass, driverBClass),
			slices:           objects(node1slice, node1slice2),
			node:             node1,

			expectResults: nil,
		},
		"with-class-device-config": {
			claimsToAllocate: objects(simpleClaim),
			classes:          objects(driverAClassWithConfig),
			slices:           objects(node1slice),
			node:             node1,

			expectResults: []any{allocatedSimpleClaimWithClassConfig},
		},
		"with-claim-device-config": {
			claimsToAllocate: objects(simpleClaimWithConfig),
			classes:          objects(driverAClass),
			slices:           objects(node1slice),
			node:             node1,

			expectResults: []any{allocatedSimpleClaimWithClaimConfig},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			_, ctx := ktesting.NewTestContext(t)
			g := gomega.NewWithT(t)

			// Listing objects is deterministic and returns them in the same
			// order as in the test case. That makes the allocation result
			// also deterministic.
			var allocated, toAllocate claimLister
			var classLister informerLister[resourceapi.DeviceClass]
			var sliceLister informerLister[resourceapi.ResourceSlice]
			for _, claim := range tc.claimsToAllocate {
				toAllocate.claims = append(toAllocate.claims, claim.DeepCopy())
			}
			for _, claim := range tc.allocatedClaims {
				allocated.claims = append(allocated.claims, claim.DeepCopy())
			}
			for _, slice := range tc.slices {
				sliceLister.objs = append(sliceLister.objs, slice.DeepCopy())
			}
			for _, class := range tc.classes {
				classLister.objs = append(classLister.objs, class.DeepCopy())
			}

			allocator, err := NewAllocator(ctx, toAllocate.claims, allocated, classLister, sliceLister)
			g.Expect(err).ToNot(gomega.HaveOccurred())

			results, err := allocator.Allocate(ctx, tc.node)
			matchError := tc.expectError
			if matchError == nil {
				matchError = gomega.Not(gomega.HaveOccurred())
			}
			g.Expect(err).To(matchError)
			g.Expect(results).To(gomega.HaveExactElements(tc.expectResults...))

			// Objects that the allocator had access to should not have been modified.
			g.Expect(toAllocate.claims).To(gomega.HaveExactElements(tc.claimsToAllocate))
			g.Expect(allocated.claims).To(gomega.HaveExactElements(tc.allocatedClaims))
			g.Expect(sliceLister.objs).To(gomega.ConsistOf(tc.slices))
			g.Expect(classLister.objs).To(gomega.ConsistOf(tc.classes))
		})
	}
}

type claimLister struct {
	claims []*resourceapi.ResourceClaim
	err    error
}

func (l claimLister) ListAllAllocated() ([]*resourceapi.ResourceClaim, error) {
	return l.claims, l.err
}

type informerLister[T any] struct {
	objs []*T
	err  error
}

func (l informerLister[T]) List(selector labels.Selector) (ret []*T, err error) {
	if selector.String() != labels.Everything().String() {
		return nil, errors.New("labels selector not implemented")
	}
	return l.objs, l.err
}

func (l informerLister[T]) Get(name string) (*T, error) {
	for _, obj := range l.objs {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		if accessor.GetName() == name {
			return obj, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, "not found")
}

func unmarshal[T any](t *testing.T, data string) *T {
	t.Helper()

	var obj T
	err := yaml.UnmarshalStrict([]byte(data), &obj)
	gomega.NewWithT(t).Expect(err).NotTo(gomega.HaveOccurred(), "parse YAML:\n"+data)
	return &obj
}

// func marshal[T any](t *testing.T, obj *T) string {
// 	t.Helper()

// 	data, err := yaml.Marshal(obj)
// 	gomega.NewWithT(t).Expect(err).NotTo(gomega.HaveOccurred(), "create YAML")
// 	return string(data)
// }

// func replace[T any](t *testing.T, obj *T, pairs ...string) *T {
// 	t.Helper()

// 	data := marshal(t, obj)
// 	for i := 0; i < len(pairs); i += 2 {
// 		data = strings.ReplaceAll(data, pairs[i], pairs[i+1])
// 	}
// 	return unmarshal[T](t, data)
// }

func objects[T any](objs ...T) []T {
	return objs
}
