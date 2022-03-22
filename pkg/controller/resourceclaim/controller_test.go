/*
Copyright 2020 The Kubernetes Authors.

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

package resourceclaim

import (
	"context"
	"errors"
	"sort"
	"testing"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/component-base/metrics/testutil"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller"
	ephemeralvolumemetrics "k8s.io/kubernetes/pkg/controller/resourceclaim/metrics"

	"github.com/stretchr/testify/assert"
)

var (
	testPodName          = "test-pod"
	testNamespace        = "my-namespace"
	testPodUID           = types.UID("uidpod1")
	otherNamespace       = "not-my-namespace"
	podResourceClaimName = "acme-resource"

	testPod             = makePod(testPodName, testNamespace, testPodUID)
	testPodWithResource = makePod(testPodName, testNamespace, testPodUID, *makePodResourceClaim(podResourceClaimName))
	otherTestPod        = makePod(testPodName+"-II", testNamespace, testPodUID+"-II")
	testClaim           = makeClaim(testPodName+"-"+podResourceClaimName, testNamespace, makeOwnerReference(testPodWithResource, true))
	testClaimReserved   = func() *v1.ResourceClaim {
		claim := makeClaim(testPodName+"-"+podResourceClaimName, testNamespace, makeOwnerReference(testPodWithResource, true))
		claim.Status.ReservedFor = append(claim.Status.ReservedFor,
			v1.ResourceClaimUserReference{
				Resource: "pods",
				Name:     testPodWithResource.Name,
				UID:      testPodWithResource.UID,
			},
		)
		return claim
	}()
	testClaimReservedTwice = func() *v1.ResourceClaim {
		claim := testClaimReserved.DeepCopy()
		claim.Status.ReservedFor = append(claim.Status.ReservedFor,
			v1.ResourceClaimUserReference{
				Resource: "pods",
				Name:     otherTestPod.Name,
				UID:      otherTestPod.UID,
			},
		)
		return claim
	}()
	conflictingClaim    = makeClaim(testPodName+"-"+podResourceClaimName, testNamespace, nil)
	otherNamespaceClaim = makeClaim(testPodName+"-"+podResourceClaimName, otherNamespace, nil)
)

func init() {
	klog.InitFlags(nil)
}

func TestSyncHandler(t *testing.T) {
	tests := []struct {
		name            string
		key             string
		claims          []*v1.ResourceClaim
		pods            []*v1.Pod
		expectedClaims  []v1.ResourceClaim
		expectedError   bool
		expectedMetrics expectedMetrics
	}{
		{
			name:            "create",
			pods:            []*v1.Pod{testPodWithResource},
			key:             podKey(testPodWithResource),
			expectedClaims:  []v1.ResourceClaim{*testClaim},
			expectedMetrics: expectedMetrics{1, 0},
		},
		{
			name:            "nop",
			pods:            []*v1.Pod{testPodWithResource},
			key:             podKey(testPodWithResource),
			claims:          []*v1.ResourceClaim{testClaim},
			expectedClaims:  []v1.ResourceClaim{*testClaim},
			expectedMetrics: expectedMetrics{0, 0},
		},
		{
			name: "no-such-pod",
			key:  podKey(testPodWithResource),
		},
		{
			name: "pod-deleted",
			pods: func() []*v1.Pod {
				deleted := metav1.Now()
				pods := []*v1.Pod{testPodWithResource.DeepCopy()}
				pods[0].DeletionTimestamp = &deleted
				return pods
			}(),
			key: podKey(testPodWithResource),
		},
		{
			name: "no-volumes",
			pods: []*v1.Pod{testPod},
			key:  podKey(testPod),
		},
		{
			name:            "create-with-other-claim",
			pods:            []*v1.Pod{testPodWithResource},
			key:             podKey(testPodWithResource),
			claims:          []*v1.ResourceClaim{otherNamespaceClaim},
			expectedClaims:  []v1.ResourceClaim{*otherNamespaceClaim, *testClaim},
			expectedMetrics: expectedMetrics{1, 0},
		},
		{
			name:           "wrong-claim-owner",
			pods:           []*v1.Pod{testPodWithResource},
			key:            podKey(testPodWithResource),
			claims:         []*v1.ResourceClaim{conflictingClaim},
			expectedClaims: []v1.ResourceClaim{*conflictingClaim},
			expectedError:  true,
		},
		{
			name:            "create-conflict",
			pods:            []*v1.Pod{testPodWithResource},
			key:             podKey(testPodWithResource),
			expectedMetrics: expectedMetrics{1, 1},
			expectedError:   true,
		},
		{
			name:            "stay-reserved",
			pods:            []*v1.Pod{testPodWithResource},
			key:             claimKey(testClaimReserved),
			claims:          []*v1.ResourceClaim{testClaimReserved},
			expectedClaims:  []v1.ResourceClaim{*testClaimReserved},
			expectedMetrics: expectedMetrics{0, 0},
		},
		{
			name:            "clear-reserved",
			pods:            []*v1.Pod{},
			key:             claimKey(testClaimReserved),
			claims:          []*v1.ResourceClaim{testClaimReserved},
			expectedClaims:  []v1.ResourceClaim{*testClaim},
			expectedMetrics: expectedMetrics{0, 0},
		},
		{
			name:            "remove-reserved",
			pods:            []*v1.Pod{testPod},
			key:             claimKey(testClaimReservedTwice),
			claims:          []*v1.ResourceClaim{testClaimReservedTwice},
			expectedClaims:  []v1.ResourceClaim{*testClaimReserved},
			expectedMetrics: expectedMetrics{0, 0},
		},
	}

	for _, tc := range tests {
		// Run sequentially because of global logging and global metrics.
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var objects []runtime.Object
			for _, pod := range tc.pods {
				objects = append(objects, pod)
			}
			for _, claim := range tc.claims {
				objects = append(objects, claim)
			}

			fakeKubeClient := createTestClient(objects...)
			if tc.expectedMetrics.numFailures > 0 {
				fakeKubeClient.PrependReactor("create", "resourceclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, apierrors.NewConflict(action.GetResource().GroupResource(), "fake name", errors.New("fake conflict"))
				})
			}
			setupMetrics()
			informerFactory := informers.NewSharedInformerFactory(fakeKubeClient, controller.NoResyncPeriodFunc())
			podInformer := informerFactory.Core().V1().Pods()
			claimInformer := informerFactory.Core().V1().ResourceClaims()

			c, err := NewController(fakeKubeClient, podInformer, claimInformer)
			if err != nil {
				t.Fatalf("error creating ephemeral controller : %v", err)
			}
			ec, _ := c.(*resourceClaimController)

			// Ensure informers are up-to-date.
			go informerFactory.Start(ctx.Done())
			defer func() {
				cancel()
				informerFactory.Shutdown()
			}()
			informerFactory.WaitForCacheSync(ctx.Done())
			cache.WaitForCacheSync(ctx.Done(), podInformer.Informer().HasSynced, claimInformer.Informer().HasSynced)

			err = ec.syncHandler(context.TODO(), tc.key)
			if err != nil && !tc.expectedError {
				t.Fatalf("unexpected error while running handler: %v", err)
			}
			if err == nil && tc.expectedError {
				t.Fatalf("unexpected success")
			}

			claims, err := fakeKubeClient.CoreV1().ResourceClaims("").List(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("unexpected error while listing claims: %v", err)
			}
			assert.Equal(t, normalizeClaims(tc.expectedClaims), normalizeClaims(claims.Items))
			expectMetrics(t, tc.expectedMetrics)
		})
	}
}

func makeClaim(name, namespace string, owner *metav1.OwnerReference) *v1.ResourceClaim {
	claim := &v1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if owner != nil {
		claim.OwnerReferences = []metav1.OwnerReference{*owner}
	}

	return claim
}

func makePodResourceClaim(name string) *v1.PodResourceClaim {
	return &v1.PodResourceClaim{
		Name: name,
		Source: v1.ClaimSource{
			Template: &v1.ResourceClaimTemplate{},
		},
	}
}

func makePod(name, namespace string, uid types.UID, podClaims ...v1.PodResourceClaim) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: uid},
		Spec: v1.PodSpec{
			ResourceClaims: podClaims,
		},
	}

	return pod
}

func podKey(pod *v1.Pod) string {
	return podKeyPrefix + pod.Namespace + "/" + pod.Name
}

func claimKey(claim *v1.ResourceClaim) string {
	return claimKeyPrefix + claim.Namespace + "/" + claim.Name
}

func makeOwnerReference(pod *v1.Pod, isController bool) *metav1.OwnerReference {
	isTrue := true
	return &metav1.OwnerReference{
		APIVersion:         "v1",
		Kind:               "Pod",
		Name:               pod.Name,
		UID:                pod.UID,
		Controller:         &isController,
		BlockOwnerDeletion: &isTrue,
	}
}

func normalizeClaims(claims []v1.ResourceClaim) []v1.ResourceClaim {
	sort.Slice(claims, func(i, j int) bool {
		return claims[i].Namespace < claims[j].Namespace ||
			claims[i].Name < claims[j].Name
	})
	for i := range claims {
		if len(claims[i].Status.ReservedFor) == 0 {
			claims[i].Status.ReservedFor = nil
		}
	}
	return claims
}

func createTestClient(objects ...runtime.Object) *fake.Clientset {
	fakeClient := fake.NewSimpleClientset(objects...)
	return fakeClient
}

// Metrics helpers

type expectedMetrics struct {
	numCreated  int
	numFailures int
}

func expectMetrics(t *testing.T, em expectedMetrics) {
	t.Helper()

	actualCreated, err := testutil.GetCounterMetricValue(ephemeralvolumemetrics.ResourceClaimCreateAttempts)
	handleErr(t, err, "ResourceClaimCreate")
	if actualCreated != float64(em.numCreated) {
		t.Errorf("Expected claims to be created %d, got %v", em.numCreated, actualCreated)
	}
	actualConflicts, err := testutil.GetCounterMetricValue(ephemeralvolumemetrics.ResourceClaimCreateFailures)
	handleErr(t, err, "ResourceClaimCreate/Conflict")
	if actualConflicts != float64(em.numFailures) {
		t.Errorf("Expected claims to have conflicts %d, got %v", em.numFailures, actualConflicts)
	}
}

func handleErr(t *testing.T, err error, metricName string) {
	if err != nil {
		t.Errorf("Failed to get %s value, err: %v", metricName, err)
	}
}

func setupMetrics() {
	ephemeralvolumemetrics.RegisterMetrics()
	ephemeralvolumemetrics.ResourceClaimCreateAttempts.Reset()
	ephemeralvolumemetrics.ResourceClaimCreateFailures.Reset()
}
