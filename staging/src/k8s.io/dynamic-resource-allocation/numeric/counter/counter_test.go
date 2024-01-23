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

package counter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
	"k8s.io/dynamic-resource-allocation/numeric/counter/internal"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/ktesting"
)

func TestState(t *testing.T) {
	full := `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}, "node-b": {"perDriver": {"driver-y": {"perInstance": {"xyz": {"name": "else", "capacity": 1}}}}}}}`
	testcases := map[string]struct {
		// JSON encoding of initial state.
		state string
		// Does something with controller having that initial state.
		op func(ctb testing.TB, logger klog.Logger, c *activeCounterController)
		// New state after op.
		expect string
	}{
		"create": {
			state:  ``,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityAddedOrUpdated(logger, mustParse(tb, `{"metadata":{"name":"node-a"}, "nodeName": "node-a", "driverName":"driver-x", "resourceInstance": {"metadata":{"uid": "abc", "name": "something"}, "kind": "Capacity", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "count": 42}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"add-instance": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}, "xyz": {"name": "else", "capacity": 1}}}}}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityAddedOrUpdated(logger, mustParse(tb, `{"metadata":{"name":"node-a"}, "nodeName": "node-a", "driverName":"driver-x", "resourceInstance": {"metadata":{"uid": "xyz", "name": "else"}, "kind": "Capacity", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "count": 1}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"add-driver": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}, "driver-y": {"perInstance": {"xyz": {"name": "else", "capacity": 1}}}}}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityAddedOrUpdated(logger, mustParse(tb, `{"metadata":{"name":"node-a"}, "nodeName": "node-a", "driverName":"driver-y", "resourceInstance": {"metadata":{"uid": "xyz", "name": "else"}, "kind": "Capacity", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "count": 1}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"add-node": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}, "node-b": {"perDriver": {"driver-y": {"perInstance": {"xyz": {"name": "else", "capacity": 1}}}}}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityAddedOrUpdated(logger, mustParse(tb, `{"metadata":{"name":"node-b"}, "nodeName": "node-b", "driverName":"driver-y", "resourceInstance": {"metadata":{"uid": "xyz", "name": "else"}, "kind": "Capacity", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "count": 1}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"nop-other-node": {
			state:  full,
			expect: full,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityAddedOrUpdated(logger, mustParse(tb, `{"metadata":{"name":"node-c"}, "nodeName": "node-c", "driverName": "driver-x", "resourceInstance": {"metadata":{"uid": "abc", "name": "something"}, "kind": "Capacity", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "count": 0}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"nop-unknown-parameter": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityAddedOrUpdated(logger, mustParse(tb, `{"metadata":{"name":"node-a"}, "nodeName": "node-a", "driverName":"driver-x", "resourceInstance": {"metadata":{"uid": "xyz", "name": "else"}, "kind": "Unknown", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "foo": "bar"}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"nop-wrong-parameter": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityAddedOrUpdated(logger, mustParse(tb, `{"metadata":{"name":"node-a"}, "nodeName": "node-a", "driverName":"driver-x", "resourceInstance": {"metadata":{"uid": "xyz", "name": "else"}, "kind": "AllocationResult", "apiVersion": "counter.dra.config.k8s.io/v1alpha1"}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		// TODO: more test cases:
		// - keep entries with non-zero Allocated

		"remove-node": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			expect: ``,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityRemoved(logger, mustParse(tb, `{"metadata":{"name":"node-a"}, "nodeName": "node-a", "driverName": "driver-x", "resourceInstance": {"metadata":{"uid": "abc", "name": "something"}, "kind": "Capacity", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "count": 42}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"remove-nop": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"name": "something", "capacity": 42}}}}}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.nodeResourceCapacityRemoved(logger, mustParse(tb, `{"metadata":{"name":"node-a"}, "nodeName": "node-a", "driverName": "driver-y", "resourceInstance": {"metadata":{"uid": "abc", "name": "something"}, "kind": "Capacity", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "count": 42}}`, new(resourcev1alpha2.NodeResourceCapacity)))
			},
		},

		"add-claim": {
			state:  ``,
			expect: `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"allocated": 42}}}}}}, "claims": {"abc": {"name": "claim-a", "namespace": "namespace-a", "nodeName": "node-a", "driverName": "driver-x", "instanceID": "abc", "count": 42}}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.ClaimAllocated(klog.NewContext(context.Background(), logger), mustParse(tb, fmt.Sprintf(`{"metadata":{"name":"claim-a", "uid":"abc","namespace":"namespace-a"}, "status":{"allocation":{"resourceHandles":[{"data":%q}]}}}`, `{"kind":"AllocationResult", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "driverName": "driver-x", "nodeName": "node-a", "instanceID": "abc", "count": 42}`), new(resourcev1alpha2.ResourceClaim)))
			},
		},

		"remove-claim": {
			state:  `{"resources": {"node-a": {"perDriver": {"driver-x": {"perInstance": {"abc": {"allocated": 42}}}}}}, "claims": {"abc": {"name": "claim-a", "namespace": "namespace-a", "nodeName": "node-a", "driverName": "driver-x", "instanceID": "abc", "count": 42}}}`,
			expect: `{"claims": {}}`,
			op: func(tb testing.TB, logger klog.Logger, c *activeCounterController) {
				c.ClaimDeallocated(klog.NewContext(context.Background(), logger), mustParse(tb, fmt.Sprintf(`{"metadata":{"name":"claim-a", "uid":"abc","namespace":"namespace-a"}, "status":{"allocation":{"resourceHandles":[{"data":%q}]}}}`, `{"kind":"AllocationResult", "apiVersion": "counter.dra.config.k8s.io/v1alpha1", "driverName": "driver-x", "nodeName": "node-a", "instanceID": "abc", "count": 42}`), new(resourcev1alpha2.ResourceClaim)))
			},
		},

		// TODO: more test cases:
		// - keep entries with non-zero Count
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			logger, _ := ktesting.NewTestContext(t)
			c := &activeCounterController{
				state: mustParse(t, tc.state, new(internal.State)),
			}
			tc.op(t, logger, c)
			require.Equal(t, mustParse(t, tc.expect, new(internal.State)), c.state)
		})
	}
}

func mustParse[T interface{}](tb testing.TB, in string, out *T) *T {
	tb.Helper()
	if in == "" {
		return out
	}
	decoder := json.NewDecoder(strings.NewReader(in))
	decoder.DisallowUnknownFields()
	require.NoError(tb, decoder.Decode(out), in)
	return out
}
