/*
Copyright 2025 The Kubernetes Authors.

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

package devicetainteviction

import (
	"sync"
	"testing"
	"time"

	resourcealpha "k8s.io/api/resource/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/test/utils/ktesting"
)

// TestSynctest runs the same test twice, once with, once without synctest.
func TestSynctest(t *testing.T) {
	featuregatetesting.SetFeatureGatesDuringTest(t, utilfeature.DefaultFeatureGate,
		featuregatetesting.FeatureOverrides{
			features.DRADeviceTaints:     true,
			features.DRADeviceTaintRules: true,
		},
	)

	tCtx := ktesting.Init(t)
	tCtx.Run("normal", testSynctest)
	tCtx.SyncTest("synctest", testSynctest)
}

func testSynctest(tCtx ktesting.TContext) {
	var wg sync.WaitGroup
	defer func() {
		tCtx.Log("Waiting for goroutine termination...")
		tCtx.Cancel("time to stop")
		wg.Wait()
	}()

	rule := ruleNone.DeepCopy()
	rule.Spec.Taint.Effect = resourcealpha.DeviceTaintEffectNoExecute
	claim := inUseClaim.DeepCopy()
	fakeClientset := fake.NewClientset(podWithClaimName, claim, rule)
	blockDelete := make(chan struct{})
	deleteWaiting := make(chan struct{})
	fakeClientset.PrependReactor("delete", "pods", func(action core.Action) (bool, runtime.Object, error) {
		tCtx.Log("Delaying pod deletion...")
		close(deleteWaiting)
		<-blockDelete
		tCtx.Log("Proceeding with pod deletion...")
		return false, nil, nil
	})
	tCtx = tCtx.WithClients(nil, nil, fakeClientset, nil, nil)
	controller := newTestController(tCtx)
	wg.Go(func() {
		tCtx.AssertNoError(controller.Run(tCtx, 1))
	})

	// Need to move forward in time past the delay(s)
	tCtx.Log("Waiting for deletion...")
	<-deleteWaiting
	tCtx.Log("Sleeping...")
	time.Sleep(3 * time.Second)
	tCtx.Log("Unblocking...")
	close(blockDelete)
}
