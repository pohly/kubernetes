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
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	cgotesting "k8s.io/client-go/testing"
	"k8s.io/klog/v2/ktesting"
	_ "k8s.io/klog/v2/ktesting/init"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
)

var (
	podKind = v1.SchemeGroupVersion.WithKind("Pod")

	podName       = "my-pod"
	podUID        = "1234"
	resourceName  = "my-resource"
	resourceName2 = resourceName + "-2"
	claimName     = podName + "-" + resourceName
	claimName2    = podName + "-" + resourceName + "-2"
	className     = "my-resource-class"
	namespace     = "default"

	resourceClass = &resourcev1alpha1.ResourceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: className,
		},
		DriverName: "some-driver",
	}

	podWithClaimName = st.MakePod().Name(podName).Namespace(namespace).
				UID(podUID).
				PodResourceClaims(v1.PodResourceClaim{Name: resourceName, Source: v1.ClaimSource{ResourceClaimName: &claimName}}).
				Obj()
	otherPodWithClaimName = st.MakePod().Name(podName).Namespace(namespace).
				UID(podUID + "-II").
				PodResourceClaims(v1.PodResourceClaim{Name: resourceName, Source: v1.ClaimSource{ResourceClaimName: &claimName}}).
				Obj()
	podWithClaimTemplate = st.MakePod().Name(podName).Namespace(namespace).
				UID(podUID).
				PodResourceClaims(v1.PodResourceClaim{Name: resourceName, Source: v1.ClaimSource{ResourceClaimTemplateName: &claimName}}).
				Obj()
	podWithTwoClaimNames = st.MakePod().Name(podName).Namespace(namespace).
				UID(podUID).
				PodResourceClaims(v1.PodResourceClaim{Name: resourceName, Source: v1.ClaimSource{ResourceClaimName: &claimName}}).
				PodResourceClaims(v1.PodResourceClaim{Name: resourceName2, Source: v1.ClaimSource{ResourceClaimName: &claimName2}}).
				Obj()

	workerNode = &st.MakeNode().Name("worker").Label("nodename", "worker").Node

	claim = st.MakeResourceClaim().
		Name(claimName).
		Namespace(namespace).
		ResourceClassName(className).
		Obj()
	pendingImmediateClaim = st.FromResourceClaim(claim).
				AllocationMode(resourcev1alpha1.AllocationModeImmediate).
				Obj()
	pendingDelayedClaim = st.FromResourceClaim(claim).
				AllocationMode(resourcev1alpha1.AllocationModeWaitForFirstConsumer).
				Obj()
	pendingDelayedClaim2 = st.FromResourceClaim(pendingDelayedClaim).
				Name(claimName2).
				Obj()
	deallocatingClaim = st.FromResourceClaim(pendingImmediateClaim).
				Allocation(&resourcev1alpha1.AllocationResult{}).
				DeallocationRequested(true).
				Obj()
	inUseClaim = st.FromResourceClaim(pendingImmediateClaim).
			Allocation(&resourcev1alpha1.AllocationResult{}).
			ReservedFor(resourcev1alpha1.ResourceClaimConsumerReference{UID: types.UID(podUID)}).
			Obj()
	allocatedClaim = st.FromResourceClaim(pendingDelayedClaim).
			OwnerReference(podName, podUID, podKind).
			Allocation(&resourcev1alpha1.AllocationResult{}).
			Obj()
	allocatedClaimWithWrongTopology = st.FromResourceClaim(allocatedClaim).
					Allocation(&resourcev1alpha1.AllocationResult{AvailableOnNodes: st.MakeNodeSelector().In("no-such-label", []string{"no-such-value"}).Obj()}).
					Obj()
	allocatedClaimWithGoodTopology = st.FromResourceClaim(allocatedClaim).
					Allocation(&resourcev1alpha1.AllocationResult{AvailableOnNodes: st.MakeNodeSelector().In("nodename", []string{"worker"}).Obj()}).
					Obj()
	otherClaim = st.MakeResourceClaim().
			Name("not-my-claim").
			Namespace(namespace).
			ResourceClassName(className).
			Obj()

	scheduling = st.MakePodScheduling().Name(podName).Namespace(namespace).
			OwnerReference(podName, podUID, podKind).
			Obj()
	schedulingPotential = st.FromPodScheduling(scheduling).
				PotentialNodes(workerNode.Name).
				Obj()
	schedulingSelectedPotential = st.FromPodScheduling(schedulingPotential).
					SelectedNode(workerNode.Name).
					Obj()
	schedulingInfo = st.FromPodScheduling(schedulingPotential).
			ResourceClaims(resourcev1alpha1.ResourceClaimSchedulingStatus{Name: resourceName},
			resourcev1alpha1.ResourceClaimSchedulingStatus{Name: resourceName2}).
		Obj()
)

// result defines the expected outcome of some operation. It covers
// operation's status and the state of the world (= objects).
type result struct {
	status *framework.Status

	// changes contains a mapping of name to an update function for
	// the corresponding object. These functions apply exactly the expected
	// changes to a copy of the object as it existed before the operation.
	changes changeMapping

	// added contains objects created by the operation.
	added []metav1.Object

	// removed contains objects deleted by the operation.
	removed []metav1.Object
}

type changeMapping map[string]func(metav1.Object) metav1.Object
type perNodeResult map[string]result

func (p perNodeResult) forNode(nodeName string) result {
	if p == nil {
		return result{}
	}
	return p[nodeName]
}

type want struct {
	preFilterResult *framework.PreFilterResult
	prefilter       result
	filter          perNodeResult
	prescore        result
	reserve         result
	unreserve       result
	postbind        result
}

// prepare contains changes for objects in the API server.
// Those changes are applied before running the steps. This can
// be used to simulate concurrent changes by some other entities
// like a resource driver.
type prepare struct {
	filter    changeMapping
	prescore  changeMapping
	reserve   changeMapping
	unreserve changeMapping
	postbind  changeMapping
}

func TestPlugin(t *testing.T) {
	testcases := map[string]struct {
		nodes       []*v1.Node // default if unset is workerNode
		pod         *v1.Pod
		claims      []*resourcev1alpha1.ResourceClaim
		classes     []*resourcev1alpha1.ResourceClass
		schedulings []*resourcev1alpha1.PodScheduling

		want want
	}{
		"empty": {
			pod: st.MakePod().Name("foo").Namespace("default").Obj(),
		},
		"claim-reference": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{allocatedClaim, otherClaim},
		},
		"claim-template": {
			pod:    podWithClaimTemplate,
			claims: []*resourcev1alpha1.ResourceClaim{allocatedClaim, otherClaim},
		},
		"missing-claim": {
			pod: podWithClaimTemplate,
			want: want{
				prefilter: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `waiting for dynamic resource controller to create the resourceclaim "my-pod-my-resource"`),
				},
			},
		},
		"waiting-for-immediate-allocation": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{pendingImmediateClaim},
			want: want{
				prefilter: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `unallocated immediate resourceclaim`),
				},
			},
		},
		"waiting-for-deallocation": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{deallocatingClaim},
			want: want{
				prefilter: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `resourceclaim must be reallocated`),
				},
			},
		},
		"delayed-allocation-missing-class": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{pendingDelayedClaim},
			want: want{
				filter: perNodeResult{
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
				reserve: result{
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
				reserve: result{
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
				reserve: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `waiting for resource driver to allocate resource`),
					changes: changeMapping{
						podName: func(in metav1.Object) metav1.Object {
							scheduling, ok := in.(*resourcev1alpha1.PodScheduling)
							if !ok {
								return in
							}
							return st.FromPodScheduling(scheduling).
								SelectedNode(workerNode.Name).
								Obj()
						},
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
				reserve: result{
					changes: changeMapping{
						claimName: func(in metav1.Object) metav1.Object {
							claim, ok := in.(*resourcev1alpha1.ResourceClaim)
							if !ok {
								return in
							}
							return st.FromResourceClaim(claim).
								ReservedFor(resourcev1alpha1.ResourceClaimConsumerReference{Resource: "pods", Name: podName, UID: types.UID(podUID)}).
								Obj()
						},
					},
				},
				postbind: result{
					removed: []metav1.Object{schedulingInfo},
				},
			},
		},
		"in-use-by-other": {
			pod:    otherPodWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{inUseClaim},
			want: want{
				prefilter: result{
					status: framework.NewStatus(framework.UnschedulableAndUnresolvable, `resourceclaim in use`),
				},
			},
		},
		"wrong-topology": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{allocatedClaimWithWrongTopology},
			want: want{
				filter: perNodeResult{
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
				reserve: result{
					changes: changeMapping{
						claimName: func(in metav1.Object) metav1.Object {
							claim, ok := in.(*resourcev1alpha1.ResourceClaim)
							if !ok {
								return in
							}
							return st.FromResourceClaim(claim).
								ReservedFor(resourcev1alpha1.ResourceClaimConsumerReference{Resource: "pods", Name: podName, UID: types.UID(podUID)}).
								Obj()
						},
					},
				},
			},
		},
		"reserved-okay": {
			pod:    podWithClaimName,
			claims: []*resourcev1alpha1.ResourceClaim{inUseClaim},
		},
	}

	for name, tc := range testcases {
		// We can run in parallel because logging is per-test.
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			nodes := tc.nodes
			if nodes == nil {
				nodes = []*v1.Node{workerNode}
			}
			testCtx := setup(t, nodes, tc.claims, tc.classes, tc.schedulings)

			initialObjects := testCtx.listAll(t)
			result, status := testCtx.p.PreFilter(testCtx.ctx, testCtx.state, tc.pod)
			t.Run("prefilter", func(t *testing.T) {
				assert.Equal(t, tc.want.preFilterResult, result)
				testCtx.verify(t, tc.want.prefilter, initialObjects, result, status)
			})
			unschedulable := status.Code() != framework.Success

			var potentialNodes []*v1.Node
			if !unschedulable {
				for _, nodeInfo := range testCtx.nodeInfos {
					initialObjects = testCtx.listAll(t)
					status := testCtx.p.Filter(testCtx.ctx, testCtx.state, tc.pod, nodeInfo)
					nodeName := nodeInfo.Node().Name
					t.Run(fmt.Sprintf("filter/%s", nodeInfo.Node().Name), func(t *testing.T) {
						testCtx.verify(t, tc.want.filter.forNode(nodeName), initialObjects, nil, status)
					})
					if status.Code() != framework.Success {
						unschedulable = true
					} else {
						potentialNodes = append(potentialNodes, nodeInfo.Node())
					}
				}
			}

			if !unschedulable && len(potentialNodes) > 0 {
				initialObjects = testCtx.listAll(t)
				status := testCtx.p.PreScore(testCtx.ctx, testCtx.state, tc.pod, potentialNodes)
				t.Run("prescore", func(t *testing.T) {
					testCtx.verify(t, tc.want.prescore, initialObjects, nil, status)
				})
				if status.Code() != framework.Success {
					unschedulable = true
				}
			}

			var selectedNode *v1.Node
			if !unschedulable && len(potentialNodes) > 0 {
				selectedNode = potentialNodes[0]

				initialObjects = testCtx.listAll(t)
				status := testCtx.p.Reserve(testCtx.ctx, testCtx.state, tc.pod, selectedNode.Name)
				t.Run("reserve", func(t *testing.T) {
					testCtx.verify(t, tc.want.reserve, initialObjects, nil, status)
				})
				if status.Code() != framework.Success {
					unschedulable = true
				}
			}

			if selectedNode != nil {
				if unschedulable {
					initialObjects = testCtx.listAll(t)
					testCtx.p.Unreserve(testCtx.ctx, testCtx.state, tc.pod, selectedNode.Name)
					t.Run("unreserve", func(t *testing.T) {
						testCtx.verify(t, tc.want.unreserve, initialObjects, nil, status)
					})
				} else {
					initialObjects = testCtx.listAll(t)
					testCtx.p.PostBind(testCtx.ctx, testCtx.state, tc.pod, selectedNode.Name)
					t.Run("postbind", func(t *testing.T) {
						testCtx.verify(t, tc.want.postbind, initialObjects, nil, status)
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

func (tc *testContext) verify(t *testing.T, expected result, initialObjects []metav1.Object, result interface{}, status *framework.Status) {
	t.Helper()
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

// update walks through all existing objects, finds the corresponding update
// function based on name and kind, and replaces those objects that have an
// update function. The rest is left unchanged.
func update(t *testing.T, objects []metav1.Object, updates changeMapping) []metav1.Object {
	var updated []metav1.Object

	for _, obj := range objects {
		name := obj.GetName()
		updater, ok := updates[name]
		if ok {
			obj = updater(obj)
		}
		updated = append(updated, obj)
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
		// Need to cancel before waiting for the shutdown.
		cancel()
		// Now we can wait for all goroutines to stop.
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
			obj.SetUID(types.UID(fmt.Sprintf("UID-%d", uidCounter)))
			uidCounter++
		case "update":
			uid := obj.GetUID()
			if uid == "" {
				return true, nil, errors.New("UID must be set on update")
			}
		}
		return false, nil, nil
	}
}
