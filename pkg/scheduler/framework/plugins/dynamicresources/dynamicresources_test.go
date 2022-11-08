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

package dynamicresources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	resourcev1alpha1 "k8s.io/api/resource/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	resourcev1alpha1ac "k8s.io/client-go/applyconfigurations/resource/v1alpha1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	cgotesting "k8s.io/client-go/testing"
	"k8s.io/klog/v2/ktesting"
	_ "k8s.io/klog/v2/ktesting/init"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

var (
	podName       = "my-pod"
	podUID        = types.UID("1234")
	resourceName  = "my-resource"
	resourceName2 = resourceName + "-2"
	claimName     = podName + "-" + resourceName
	claimName2    = podName + "-" + resourceName + "-2"
	className     = "my-resource-class"
	namespace     = "default"
	templateName  = "my-template"

	resourceClass = toResourceClass(resourcev1alpha1ac.ResourceClass(className))

	podWithClaimName = toPod(corev1ac.Pod(podName, namespace).
				WithUID(podUID).
				WithSpec(corev1ac.PodSpec().
					WithResourceClaims(corev1ac.PodResourceClaim().
						WithName(resourceName).
						WithSource(corev1ac.ClaimSource().
							WithResourceClaimName(claimName)))))
	otherPodWithClaimName = toPod(corev1ac.Pod(podName, namespace).
				WithUID(podUID + "-II").
				WithSpec(corev1ac.PodSpec().
					WithResourceClaims(corev1ac.PodResourceClaim().
						WithName(resourceName).
						WithSource(corev1ac.ClaimSource().
							WithResourceClaimName(claimName)))))
	podWithClaimTemplate = toPod(corev1ac.Pod(podName, namespace).
				WithUID(podUID).
				WithSpec(corev1ac.PodSpec().
					WithResourceClaims(corev1ac.PodResourceClaim().
						WithName(resourceName).
						WithSource(corev1ac.ClaimSource().
							WithResourceClaimTemplateName(templateName)))))
	podWithTwoClaimNames = toPod(corev1ac.Pod(podName, namespace).
				WithUID(podUID).
				WithSpec(corev1ac.PodSpec().
					WithResourceClaims(
				corev1ac.PodResourceClaim().
					WithName(resourceName).
					WithSource(corev1ac.ClaimSource().
						WithResourceClaimName(claimName)),
				corev1ac.PodResourceClaim().
					WithName(resourceName2).
					WithSource(corev1ac.ClaimSource().
						WithResourceClaimName(claimName2)))))

	workerNode = toNode(corev1ac.Node("worker").
			WithLabels(map[string]string{"nodename": "worker"}))

	pendingImmediateClaim = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName, namespace).
				WithSpec(resourcev1alpha1ac.ResourceClaimSpec().
					WithAllocationMode(resourcev1alpha1.AllocationModeImmediate).
					WithResourceClassName(className)))
	pendingDelayedClaim = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName, namespace).
				WithSpec(resourcev1alpha1ac.ResourceClaimSpec().
					WithAllocationMode(resourcev1alpha1.AllocationModeWaitForFirstConsumer).
					WithResourceClassName(className)))
	pendingDelayedClaim2 = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName2, namespace).
				WithSpec(resourcev1alpha1ac.ResourceClaimSpec().
					WithAllocationMode(resourcev1alpha1.AllocationModeWaitForFirstConsumer).
					WithResourceClassName(className)))
	deallocatingClaim = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName, namespace).
				WithStatus(resourcev1alpha1ac.ResourceClaimStatus().
					WithAllocation(resourcev1alpha1ac.AllocationResult()).
					WithDeallocationRequested(true)))
	inUseClaim = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName, namespace).
			WithStatus(resourcev1alpha1ac.ResourceClaimStatus().
				WithAllocation(resourcev1alpha1ac.AllocationResult()).
				WithReservedFor(resourcev1alpha1ac.ResourceClaimConsumerReference().
					WithUID(podUID))))
	allocatedClaim = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName, namespace).
			WithOwnerReferences(metav1ac.OwnerReference().
				WithUID(podUID).
				WithController(true)).
			WithStatus(resourcev1alpha1ac.ResourceClaimStatus().
				WithAllocation(resourcev1alpha1ac.AllocationResult())))
	allocatedClaimWithWrongTopology = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName, namespace).
					WithOwnerReferences(metav1ac.OwnerReference().
						WithUID(podUID).
						WithController(true)).
					WithStatus(resourcev1alpha1ac.ResourceClaimStatus().
						WithAllocation(resourcev1alpha1ac.AllocationResult().
							WithAvailableOnNodes(corev1ac.NodeSelector().
								WithNodeSelectorTerms(corev1ac.NodeSelectorTerm().
									WithMatchExpressions(corev1ac.NodeSelectorRequirement().
										WithKey("no-such-label").
										WithOperator(v1.NodeSelectorOpIn).
										WithValues("no-such-value")))))))
	allocatedClaimWithGoodTopology = toResourceClaim(resourcev1alpha1ac.ResourceClaim(claimName, namespace).
					WithOwnerReferences(metav1ac.OwnerReference().
						WithUID(podUID).
						WithController(true)).
					WithStatus(resourcev1alpha1ac.ResourceClaimStatus().
						WithAllocation(resourcev1alpha1ac.AllocationResult().
							WithAvailableOnNodes(corev1ac.NodeSelector().
								WithNodeSelectorTerms(corev1ac.NodeSelectorTerm().
									WithMatchExpressions(corev1ac.NodeSelectorRequirement().
										WithKey("nodename").
										WithOperator(v1.NodeSelectorOpIn).
										WithValues("worker")))))))
	otherClaim = toResourceClaim(resourcev1alpha1ac.ResourceClaim("not-my-claim", namespace))

	schedulingSelectedPotential = toPodScheduling(resourcev1alpha1ac.PodScheduling(podName, namespace).
					WithOwnerReferences(metav1ac.OwnerReference().
						WithAPIVersion("v1").
						WithController(true).
						WithKind("Pod").
						WithName(podName).
						WithUID(podUID)).
					WithSpec(resourcev1alpha1ac.PodSchedulingSpec().
						WithSelectedNode(workerNode.Name).
						WithPotentialNodes(workerNode.Name)))
	schedulingPotential = toPodScheduling(resourcev1alpha1ac.PodScheduling(podName, namespace).
				WithOwnerReferences(metav1ac.OwnerReference().
					WithAPIVersion("v1").
					WithController(true).
					WithKind("Pod").
					WithName(podName).
					WithUID(podUID)).
				WithSpec(resourcev1alpha1ac.PodSchedulingSpec().
					WithPotentialNodes(workerNode.Name)))
	schedulingInfo = toPodScheduling(resourcev1alpha1ac.PodScheduling(podName, namespace).
			WithOwnerReferences(metav1ac.OwnerReference().
				WithAPIVersion("v1").
				WithController(true).
				WithKind("Pod").
				WithName(podName).
				WithUID(podUID)).
			WithSpec(resourcev1alpha1ac.PodSchedulingSpec().
				WithPotentialNodes(workerNode.Name)).
			WithStatus(resourcev1alpha1ac.PodSchedulingStatus().
				WithResourceClaims(resourcev1alpha1ac.ResourceClaimSchedulingStatus().
					WithName(resourceName), resourcev1alpha1ac.ResourceClaimSchedulingStatus().
					WithName(resourceName2))))
)

// result defines the expected outcome of some operation. It covers return
// values, the plugin state and the state of the world (= objects).
type result struct {
	returnValue interface{}
	status      *framework.Status

	// changes contains corev1ac apply configuration instances that contain
	// the expected changes to existing objects. This way, new fields and
	// can be described concisely. Removal of fields is not needed.
	changes []any

	// added contains objects created by the operation.
	added []metav1.Object

	// removed contains objects deleted by the operation.
	removed []metav1.Object
}

type operation int

const (
	prefilterOp operation = iota
	filterOp
	prescoreOp
	reserveOp
	unreserveOp
	postbindOp
)

type perNodeResult map[string]result

type want map[operation]any

func TestPreFilter(t *testing.T) {
	var nilPreFilterResult *framework.PreFilterResult

	testcases := map[string]struct {
		nodes       []*v1.Node // default if unset is workerNode
		pod         *v1.Pod
		claims      []*resourcev1alpha1.ResourceClaim
		classes     []*resourcev1alpha1.ResourceClass
		schedulings []*resourcev1alpha1.PodScheduling

		want want
	}{
		"empty": {
			pod: toPod(corev1ac.Pod("foo", "default")),
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
			},
		},
		"claim-reference": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{allocatedClaim, otherClaim},
			want: want{
				prefilterOp: result{

					returnValue: nilPreFilterResult,
				},
			},
		},
		"claim-template": {
			pod:    podWithClaimTemplate,
			claims: []*resourcev1alpha1.ResourceClaim{allocatedClaim, otherClaim},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
			},
		},
		"missing-claim": {
			pod: podWithClaimTemplate,
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
					status:      framework.NewStatus(framework.UnschedulableAndUnresolvable, `waiting for dynamic resource controller to create the resourceclaim "my-pod-my-resource"`),
				},
			},
		},
		"waiting-for-immediate-allocation": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{pendingImmediateClaim},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
					status:      framework.NewStatus(framework.UnschedulableAndUnresolvable, `unallocated immediate resourceclaim`),
				},
			},
		},
		"waiting-for-deallocation": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{deallocatingClaim},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
					status:      framework.NewStatus(framework.UnschedulableAndUnresolvable, `resourceclaim must be reallocated`),
				},
			},
		},
		"delayed-allocation-missing-class": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{pendingDelayedClaim},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
				filterOp: perNodeResult{
					workerNode.Name: {
						status: framework.AsStatus(fmt.Errorf(`look up resource class: resourceclass.resource.k8s.io "%s" not found`, className)),
					},
				},
			},
		},
		"delayed-allocation-scheduling-select-immediately": {
			// Create the PodScheduling object, ask for information
			// and select a node.
			pod:     podWithClaimName,
			claims:  []*resourcev1alpha1.ResourceClaim{pendingDelayedClaim},
			classes: []*resourcev1alpha1.ResourceClass{resourceClass},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
				reserveOp: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `waiting for resource driver to allocate resource`),
					added:  []metav1.Object{schedulingSelectedPotential},
				},
			},
		},
		"delayed-allocation-scheduling-ask": {
			// Create the PodScheduling object, ask for
			// information, but do not select a node because
			// there are multiple claims.
			pod:     podWithTwoClaimNames,
			claims:  []*resourcev1alpha1.ResourceClaim{pendingDelayedClaim, pendingDelayedClaim2},
			classes: []*resourcev1alpha1.ResourceClass{resourceClass},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
				reserveOp: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `waiting for resource driver to provide information`),
					added:  []metav1.Object{schedulingPotential},
				},
			},
		},
		"delayed-allocation-scheduling-finish": {
			// Use the populated PodScheduling object to select a
			// node.
			pod:         podWithClaimName,
			claims:      []*resourcev1alpha1.ResourceClaim{pendingDelayedClaim},
			schedulings: []*resourcev1alpha1.PodScheduling{schedulingInfo},
			classes:     []*resourcev1alpha1.ResourceClass{resourceClass},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
				reserveOp: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `waiting for resource driver to allocate resource`),
					changes: []any{
						resourcev1alpha1ac.PodScheduling(podName, namespace).WithSpec(resourcev1alpha1ac.PodSchedulingSpec().WithSelectedNode(workerNode.Name)),
					},
				},
			},
		},
		"delayed-allocation-scheduling-completed": {
			// Remove PodScheduling object once the pod is scheduled.
			pod:         podWithClaimName,
			claims:      []*resourcev1alpha1.ResourceClaim{allocatedClaim},
			schedulings: []*resourcev1alpha1.PodScheduling{schedulingInfo},
			classes:     []*resourcev1alpha1.ResourceClass{resourceClass},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
				reserveOp: result{
					changes: []any{
						resourcev1alpha1ac.ResourceClaim(claimName, namespace).WithStatus(resourcev1alpha1ac.ResourceClaimStatus().WithReservedFor(resourcev1alpha1ac.ResourceClaimConsumerReference().WithResource("pods").WithName(podName).WithUID(podUID))),
					},
				},
				postbindOp: result{
					removed: []metav1.Object{schedulingInfo},
				},
			},
		},
		"in-use-by-other": {
			pod:    otherPodWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{inUseClaim},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
					status:      framework.NewStatus(framework.UnschedulableAndUnresolvable, `resourceclaim in use`),
				},
			},
		},
		"wrong-topology": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{allocatedClaimWithWrongTopology},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
				filterOp: perNodeResult{
					workerNode.Name: {
						status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `resourceclaim not available on the node`),
					},
				},
			},
		},
		"good-topology": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{allocatedClaimWithGoodTopology},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
				reserveOp: result{
					changes: []any{
						resourcev1alpha1ac.ResourceClaim(claimName, namespace).WithStatus(resourcev1alpha1ac.ResourceClaimStatus().WithReservedFor(resourcev1alpha1ac.ResourceClaimConsumerReference().WithResource("pods").WithName(podName).WithUID(podUID))),
					},
				},
			},
		},
		"reserved-okay": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{inUseClaim},
			want: want{
				prefilterOp: result{
					returnValue: nilPreFilterResult,
				},
			},
		},
	}

	for name, testcase := range testcases {
		// We can run in parallel because logging is per-test.
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			nodes := testcase.nodes
			if nodes == nil {
				nodes = []*v1.Node{workerNode}
			}
			tc := setup(t, nodes, testcase.claims, testcase.classes, testcase.schedulings)

			initialObjects := tc.listAll(t)
			result, status := tc.p.PreFilter(tc.ctx, tc.state, testcase.pod)
			t.Run("prefilter", func(t *testing.T) {
				tc.verify(t, overallResult(testcase.want[prefilterOp]), initialObjects, result, status)
			})
			unschedulable := status.Code() != framework.Success

			var potentialNodes []*v1.Node
			if !unschedulable {
				for _, nodeInfo := range tc.nodeInfos {
					initialObjects = tc.listAll(t)
					status := tc.p.Filter(tc.ctx, tc.state, testcase.pod, nodeInfo)
					nodeName := nodeInfo.Node().Name
					t.Run(fmt.Sprintf("filter/%s", nodeInfo.Node().Name), func(t *testing.T) {
						tc.verify(t, nodeResult(testcase.want[filterOp], nodeName), initialObjects, nil, status)
					})
					if status.Code() != framework.Success {
						unschedulable = true
					} else {
						potentialNodes = append(potentialNodes, nodeInfo.Node())
					}
				}
			}

			if !unschedulable && len(potentialNodes) > 0 {
				initialObjects = tc.listAll(t)
				status := tc.p.PreScore(tc.ctx, tc.state, testcase.pod, potentialNodes)
				t.Run("prescore", func(t *testing.T) {
					tc.verify(t, overallResult(testcase.want[prescoreOp]), initialObjects, nil, status)
				})
				if status.Code() != framework.Success {
					unschedulable = true
				}
			}

			var selectedNode *v1.Node
			if !unschedulable && len(potentialNodes) > 0 {
				selectedNode = potentialNodes[0]

				initialObjects = tc.listAll(t)
				status := tc.p.Reserve(tc.ctx, tc.state, testcase.pod, selectedNode.Name)
				t.Run("reserve", func(t *testing.T) {
					tc.verify(t, overallResult(testcase.want[reserveOp]), initialObjects, nil, status)
				})
				if status.Code() != framework.Success {
					unschedulable = true
				}
			}

			if selectedNode != nil {
				if unschedulable {
					initialObjects = tc.listAll(t)
					tc.p.Unreserve(tc.ctx, tc.state, testcase.pod, selectedNode.Name)
					t.Run("unreserve", func(t *testing.T) {
						tc.verify(t, overallResult(testcase.want[unreserveOp]), initialObjects, nil, status)
					})
				} else {
					initialObjects = tc.listAll(t)
					tc.p.PostBind(tc.ctx, tc.state, testcase.pod, selectedNode.Name)
					t.Run("postbind", func(t *testing.T) {
						tc.verify(t, overallResult(testcase.want[postbindOp]), initialObjects, nil, status)
					})
				}
			}
		})
	}
}

type testContext struct {
	ctx       context.Context
	client    *fake.Clientset
	p         *dynamicResources
	nodeInfos []*framework.NodeInfo
	state     *framework.CycleState
}

func overallResult(in any) result {
	if in == nil {
		return result{}
	}
	out, ok := in.(result)
	if !ok {
		panic(fmt.Sprintf("internal error, expected result value, got %T: %v", in, in))
	}
	return out
}

func nodeResult(in any, nodeName string) result {
	if in == nil {
		return result{}
	}
	out, ok := in.(perNodeResult)
	if !ok {
		panic(fmt.Sprintf("internal error, expected perNodeResult value, got %T: %v", in, in))
	}
	return out[nodeName]
}

func (tc *testContext) verify(t *testing.T, expected result, initialObjects []metav1.Object, result interface{}, status *framework.Status) {
	t.Helper()
	assert.Equal(t, expected.returnValue, result)
	assert.Equal(t, expected.status, status)
	objects := tc.listAll(t)
	wantObjects := update(t, initialObjects, expected.changes)
	for _, add := range expected.added {
		wantObjects = append(wantObjects, add)
	}
	for _, remove := range expected.removed {
		for i, obj := range wantObjects {
			// This is a bit relaxed (no GVR comparison, no UID
			// comparison) to simplify writing the test cases.
			if obj.GetName() == remove.GetName() && obj.GetNamespace() == remove.GetNamespace() {
				wantObjects = append(wantObjects[0:i], wantObjects[i+1:]...)
				break
			}
		}
	}
	sortObjects(wantObjects)
	stripObjects(wantObjects)
	stripObjects(objects)
	assert.Equal(t, wantObjects, objects)
}

// setGVK is implemented by metav1.TypeMeta and thus all API objects, in
// contrast to metav1.Type, which is not (?!) implemented.
type setGVK interface {
	SetGroupVersionKind(gvk schema.GroupVersionKind)
}

// stripObjects removes certain fields (Kind, APIVersion, etc.) which are not
// important and might not be set.
func stripObjects(objects []metav1.Object) {
	for _, obj := range objects {
		obj.SetResourceVersion("")
		obj.SetUID("")
		if objType, ok := obj.(setGVK); ok {
			objType.SetGroupVersionKind(schema.GroupVersionKind{})
		}
	}
}

func (tc *testContext) listAll(t *testing.T) (objects []metav1.Object) {
	t.Helper()
	claims, err := tc.client.ResourceV1alpha1().ResourceClaims("").List(tc.ctx, metav1.ListOptions{})
	require.NoError(t, err, "list claims")
	for _, claim := range claims.Items {
		objects = append(objects, &claim)
	}
	schedulings, err := tc.client.ResourceV1alpha1().PodSchedulings("").List(tc.ctx, metav1.ListOptions{})
	require.NoError(t, err, "list pod scheduling")
	for _, scheduling := range schedulings.Items {
		objects = append(objects, &scheduling)
	}

	sortObjects(objects)
	return
}

func sortObjects(objects []metav1.Object) {
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].GetNamespace() < objects[j].GetNamespace() {
			return true
		}
		return objects[i].GetName() < objects[j].GetName()
	})
}

type namespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type object struct {
	Metadata namespacedName `json:"metadata"`
}

func update[T metav1.Object](t *testing.T, objects []T, updates []any) []T {
	var updated []T

	// Convert all updates from apply configuration to the corresponding
	// JSON. By decoding the JSON into namespacedName we can figure out
	// what the update is for.
	jsonUpdates := make(map[namespacedName][]byte)
	for _, update := range updates {
		data, err := json.Marshal(update)
		require.NoError(t, err)
		var o object
		require.NoError(t, json.Unmarshal(data, &o))
		jsonUpdates[o.Metadata] = data
	}

	for _, object := range objects {
		// We can get this through the common interface.
		id := namespacedName{
			Name:      object.GetName(),
			Namespace: object.GetNamespace(),
		}

		if data, ok := jsonUpdates[id]; ok {
			// Updates are not proper server-side-apply - here we just overwrite fields.
			require.NoError(t, json.Unmarshal(data, object))
		}
		updated = append(updated, object)
	}
	return updated
}

func setup(t *testing.T, nodes []*v1.Node, claims []*resourcev1alpha1.ResourceClaim, classes []*resourcev1alpha1.ResourceClass, schedulings []*resourcev1alpha1.PodScheduling) (result *testContext) {
	t.Helper()

	tc := &testContext{}
	_, ctx := ktesting.NewTestContext(t)
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	tc.ctx = ctx

	tc.client = fake.NewSimpleClientset()
	reactor := createReactor(tc.client.Tracker())
	tc.client.PrependReactor("*", "*", reactor)

	informerFactory := informers.NewSharedInformerFactory(tc.client, 0)

	opts := []runtime.Option{
		runtime.WithClientSet(tc.client),
		runtime.WithInformerFactory(informerFactory),
	}
	fh, err := runtime.NewFramework(nil, nil, tc.ctx.Done(), opts...)
	if err != nil {
		t.Fatal(err)
	}

	pl, err := New(nil, fh, feature.Features{EnableDynamicResourceAllocation: true})
	if err != nil {
		t.Fatal(err)
	}
	tc.p = pl.(*dynamicResources)

	// The tests use the API to create the objects because then reactors
	// get triggered.
	for _, claim := range claims {
		_, err := tc.client.ResourceV1alpha1().ResourceClaims(claim.Namespace).Create(tc.ctx, claim, metav1.CreateOptions{})
		require.NoError(t, err, "create resource claim")
	}
	for _, class := range classes {
		_, err := tc.client.ResourceV1alpha1().ResourceClasses().Create(tc.ctx, class, metav1.CreateOptions{})
		require.NoError(t, err, "create resource class")
	}
	for _, scheduling := range schedulings {
		_, err := tc.client.ResourceV1alpha1().PodSchedulings(scheduling.Namespace).Create(tc.ctx, scheduling, metav1.CreateOptions{})
		require.NoError(t, err, "create pod scheduling")
	}

	informerFactory.Start(tc.ctx.Done())
	t.Cleanup(func() {
		cancel()
		informerFactory.Shutdown()
	})

	informerFactory.WaitForCacheSync(tc.ctx.Done())

	for _, node := range nodes {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(node)
		tc.nodeInfos = append(tc.nodeInfos, nodeInfo)
	}
	tc.state = framework.NewCycleState()

	return tc
}

// createReactor implements the logic required for the UID and ResourceVersion fields to work when using
// the fake client. Add it with client.PrependReactor to your fake client.
func createReactor(tracker cgotesting.ObjectTracker) func(action cgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
	var uidCounter int
	var resourceVersionCounter int
	var mutex sync.Mutex

	return func(action cgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		createAction, ok := action.(cgotesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		obj, ok := createAction.GetObject().(metav1.Object)
		if !ok {
			return false, nil, nil
		}

		mutex.Lock()
		defer mutex.Unlock()
		switch action.GetVerb() {
		case "create":
			if obj.GetUID() != "" {
				return true, nil, errors.New("UID must not be set on create")
			}
			if obj.GetResourceVersion() != "" {
				return true, nil, errors.New("ResourceVersion must not be set on create")
			}
			obj.SetUID(types.UID(fmt.Sprintf("UID-%d", uidCounter)))
			uidCounter++
			obj.SetResourceVersion(fmt.Sprintf("REV-%d", resourceVersionCounter))
			resourceVersionCounter++
		case "update":
			uid := obj.GetUID()
			resourceVersion := obj.GetResourceVersion()
			if uid == "" {
				return true, nil, errors.New("UID must be set on update")
			}
			if resourceVersion == "" {
				return true, nil, errors.New("ResourceVersion must be set on update")
			}

			oldObj, err := tracker.Get(action.GetResource(), obj.GetNamespace(), obj.GetName())
			if err != nil {
				return true, nil, err
			}
			oldObjMeta, ok := oldObj.(metav1.Object)
			if !ok {
				return true, nil, errors.New("internal error: unexpected old object type")
			}
			if oldObjMeta.GetResourceVersion() != resourceVersion {
				return true, nil, errors.New("ResourceVersion must match the object that gets updated")
			}

			obj.SetResourceVersion(fmt.Sprintf("REV-%d", resourceVersionCounter))
			resourceVersionCounter++
		}
		return false, nil, nil
	}
}
