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

package dra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"

	v1 "k8s.io/api/core/v1"
	resourcealphaapi "k8s.io/api/resource/v1alpha3"
	resourceapi "k8s.io/api/resource/v1beta1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	resourceapiac "k8s.io/client-go/applyconfigurations/resource/v1beta1"
	"k8s.io/component-base/featuregate"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	"k8s.io/klog/v2"
	kubeschedulerconfigv1 "k8s.io/kube-scheduler/config/v1"
	kubeapiservertesting "k8s.io/kubernetes/cmd/kube-apiserver/app/testing"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	kubeschedulerscheme "k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	"k8s.io/kubernetes/test/integration/framework"
	"k8s.io/kubernetes/test/integration/util"
	"k8s.io/kubernetes/test/utils/format"
	"k8s.io/kubernetes/test/utils/ktesting"
	"k8s.io/utils/ptr"
)

var (
	// For more test data see pkg/scheduler/framework/plugin/dynamicresources/dynamicresources_test.go.

	podName          = "my-pod"
	namespace        = "default"
	resourceName     = "my-resource"
	className        = "my-resource-class"
	claimName        = podName + "-" + resourceName
	podWithClaimName = st.MakePod().Name(podName).Namespace(namespace).
				Container("my-container").
				PodResourceClaims(v1.PodResourceClaim{Name: resourceName, ResourceClaimName: &claimName}).
				Obj()
	class = &resourceapi.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: className,
		},
	}
	claim = st.MakeResourceClaim().
		Name(claimName).
		Namespace(namespace).
		Request(className).
		Obj()
	claimPrioritizedList = st.MakeResourceClaim().
				Name(claimName).
				Namespace(namespace).
				RequestWithPrioritizedList(className).
				Obj()
)

// createTestNamespace creates a namespace with a name that is derived from the
// current test name:
// - Non-alpha-numeric characters replaced by hyphen.
// - Truncated in the middle to make it short enough for GenerateName.
// - Hyphen plus random suffix added by the apiserver.
func createTestNamespace(tCtx ktesting.TContext, labels map[string]string) string {
	tCtx.Helper()
	name := regexp.MustCompile(`[^[:alnum:]_-]`).ReplaceAllString(tCtx.Name(), "-")
	name = strings.ToLower(name)
	if len(name) > 63 {
		name = name[:30] + "--" + name[len(name)-30:]
	}
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: name + "-"}}
	ns.Labels = labels
	ns, err := tCtx.Client().CoreV1().Namespaces().Create(tCtx, ns, metav1.CreateOptions{})
	tCtx.ExpectNoError(err, "create test namespace")
	tCtx.CleanupCtx(func(tCtx ktesting.TContext) {
		tCtx.ExpectNoError(tCtx.Client().CoreV1().Namespaces().Delete(tCtx, ns.Name, metav1.DeleteOptions{}), "delete test namespace")
	})
	return ns.Name
}

func TestDRA(t *testing.T) {
	// Each sub-test brings up the API server in a certain
	// configuration. These sub-tests must run sequentially because they
	// change the global DefaultFeatureGate. For each configuration,
	// multiple tests can run in parallel as long as they are careful
	// about what they create.
	for name, tc := range map[string]struct {
		apis     map[schema.GroupVersion]bool
		features map[featuregate.Feature]bool
		f        func(tCtx ktesting.TContext)
	}{
		"default": {
			f: func(tCtx ktesting.TContext) {
				tCtx.Run("Pod", func(tCtx ktesting.TContext) { testPod(tCtx, false) })
				tCtx.Run("APIDisabled", testAPIDisabled)
			},
		},
		"GA": {
			// TODO (https://github.com/kubernetes/kubernetes/issues/131903): remove enabling the beta when promoting to GA.
			apis: map[schema.GroupVersion]bool{
				resourceapi.SchemeGroupVersion:     true,
				resourcev1beta2.SchemeGroupVersion: true,
			},
			features: map[featuregate.Feature]bool{
				features.DynamicResourceAllocation: true,
				// TODO: replace specific list with AllBeta once DRA is not beta.
				features.DRAResourceClaimDeviceStatus: false,
				// featuregate.Feature("AllBeta"):     false,
			},
			f: func(tCtx ktesting.TContext) {
				tCtx.Run("AdminAccess", func(tCtx ktesting.TContext) { testAdminAccess(tCtx, false) })
				tCtx.Run("PrioritizedList", func(tCtx ktesting.TContext) { testPrioritizedList(tCtx, false) })
				tCtx.Run("Pod", func(tCtx ktesting.TContext) { testPod(tCtx, true) })
				tCtx.Run("PublishResourceSlices", func(tCtx ktesting.TContext) {
					testPublishResourceSlices(tCtx, features.DRADeviceTaints, features.DRAPartitionableDevices)
				})
				tCtx.Run("ResourceClaimDeviceStatus", func(tCtx ktesting.TContext) { testResourceClaimDeviceStatus(tCtx, false) })
			},
		},
		"core": {
			apis: map[schema.GroupVersion]bool{
				resourceapi.SchemeGroupVersion:     true,
				resourcev1beta2.SchemeGroupVersion: true,
			},
			features: map[featuregate.Feature]bool{features.DynamicResourceAllocation: true},
			f: func(tCtx ktesting.TContext) {
				tCtx.Run("AdminAccess", func(tCtx ktesting.TContext) { testAdminAccess(tCtx, false) })
				tCtx.Run("PrioritizedList", func(tCtx ktesting.TContext) { testPrioritizedList(tCtx, false) })
				tCtx.Run("Pod", func(tCtx ktesting.TContext) { testPod(tCtx, true) })
				tCtx.Run("PublishResourceSlices", func(tCtx ktesting.TContext) {
					testPublishResourceSlices(tCtx, features.DRADeviceTaints, features.DRAPartitionableDevices)
				})
				tCtx.Run("ResourceClaimDeviceStatus", func(tCtx ktesting.TContext) { testResourceClaimDeviceStatus(tCtx, true) })
			},
		},
		"v1beta1": {
			apis: map[schema.GroupVersion]bool{
				resourceapi.SchemeGroupVersion: true,
			},
			features: map[featuregate.Feature]bool{features.DynamicResourceAllocation: true},
			f: func(tCtx ktesting.TContext) {
				tCtx.Run("PublishResourceSlices", func(tCtx ktesting.TContext) {
					testPublishResourceSlices(tCtx, features.DRADeviceTaints, features.DRAPartitionableDevices)
				})
			},
		},
		"v1beta2": {
			apis: map[schema.GroupVersion]bool{
				resourcev1beta2.SchemeGroupVersion: true,
			},
			features: map[featuregate.Feature]bool{features.DynamicResourceAllocation: true},
			f: func(tCtx ktesting.TContext) {
				tCtx.Run("PublishResourceSlices", func(tCtx ktesting.TContext) {
					testPublishResourceSlices(tCtx, features.DRADeviceTaints, features.DRAPartitionableDevices)
				})
			},
		},
		"all": {
			apis: map[schema.GroupVersion]bool{
				resourceapi.SchemeGroupVersion:      true,
				resourcev1beta2.SchemeGroupVersion:  true,
				resourcealphaapi.SchemeGroupVersion: true,
			},
			features: map[featuregate.Feature]bool{
				features.DynamicResourceAllocation: true,
				// Additional DRA feature gates go here,
				// in alphabetical order,
				// as needed by tests for them.
				features.DRAAdminAccess:          true,
				features.DRADeviceTaints:         true,
				features.DRAPartitionableDevices: true,
				features.DRAPrioritizedList:      true,
			},
			f: func(tCtx ktesting.TContext) {
				tCtx.Run("AdminAccess", func(tCtx ktesting.TContext) { testAdminAccess(tCtx, true) })
				tCtx.Run("Convert", testConvert)
				tCtx.Run("PrioritizedList", func(tCtx ktesting.TContext) { testPrioritizedList(tCtx, true) })
				tCtx.Run("PublishResourceSlices", func(tCtx ktesting.TContext) { testPublishResourceSlices(tCtx) })
				tCtx.Run("ResourceClaimDeviceStatus", func(tCtx ktesting.TContext) { testResourceClaimDeviceStatus(tCtx, true) })
				tCtx.Run("DeviceStatusWithConsumableCapacity", testResourceClaimDeviceStatusWithConsumableCapacity)
				tCtx.Run("MaxResourceSlice", testMaxResourceSlice)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			tCtx := ktesting.Init(t)
			var entries []string
			for key, value := range tc.features {
				entries = append(entries, fmt.Sprintf("%s=%t", key, value))
			}
			for key, value := range tc.apis {
				entries = append(entries, fmt.Sprintf("%s=%t", key, value))
			}
			sort.Strings(entries)
			t.Logf("Config: %s", strings.Join(entries, ","))

			for key, value := range tc.features {
				featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, key, value)
			}

			etcdOptions := framework.SharedEtcd()
			apiServerOptions := kubeapiservertesting.NewDefaultTestServerOptions()
			apiServerFlags := framework.DefaultTestServerFlags()
			var runtimeConfigs []string
			for key, value := range tc.apis {
				runtimeConfigs = append(runtimeConfigs, fmt.Sprintf("%s=%t", key, value))
			}
			apiServerFlags = append(apiServerFlags, "--runtime-config="+strings.Join(runtimeConfigs, ","))
			server := kubeapiservertesting.StartTestServerOrDie(t, apiServerOptions, apiServerFlags, etcdOptions)
			tCtx.Cleanup(server.TearDownFn)
			tCtx = ktesting.WithRESTConfig(tCtx, server.ClientConfig)
			tCtx = ktesting.WithValue(tCtx, etcdKey, etcd{client: server.EtcdClient, storagePrefix: server.EtcdStoragePrefix})

			tc.f(tCtx)
		})
	}
}

func getEtcd(tCtx ktesting.TContext) etcd {
	return tCtx.Value(etcdKey).(etcd)
}

type etcd struct {
	client        *clientv3.Client
	storagePrefix string
}

func (e etcd) put(tCtx ktesting.TContext, resource, namespace, name, data string) {
	tCtx.Helper()
	key := path.Join("/", e.storagePrefix, resource, namespace, name)
	_, err := e.client.Put(tCtx, key, data)
	tCtx.ExpectNoError(err, fmt.Sprintf("etcd PUT %s", key))
}

type etcdKeyType struct{}

var etcdKey etcdKeyType

func startScheduler(tCtx ktesting.TContext) {
	// Run scheduler with default configuration.
	tCtx.Log("Scheduler starting...")
	schedulerCtx := klog.NewContext(tCtx, klog.LoggerWithName(tCtx.Logger(), "scheduler"))
	schedulerCtx, cancel := context.WithCancelCause(schedulerCtx)
	_, informerFactory := util.StartScheduler(schedulerCtx, tCtx.Client(), tCtx.RESTConfig(), newDefaultSchedulerComponentConfig(tCtx), nil)
	// Stop clients of the apiserver before stopping the apiserver itself,
	// otherwise it delays its shutdown.
	tCtx.Cleanup(informerFactory.Shutdown)
	tCtx.Cleanup(func() {
		tCtx.Log("Stoping scheduler...")
		cancel(errors.New("test is done"))
	})
}

func newDefaultSchedulerComponentConfig(tCtx ktesting.TContext) *config.KubeSchedulerConfiguration {
	gvk := kubeschedulerconfigv1.SchemeGroupVersion.WithKind("KubeSchedulerConfiguration")
	cfg := config.KubeSchedulerConfiguration{}
	_, _, err := kubeschedulerscheme.Codecs.UniversalDecoder().Decode(nil, &gvk, &cfg)
	tCtx.ExpectNoError(err, "decode default scheduler configuration")
	return &cfg
}

// testPod creates a pod with a resource claim reference and then checks
// whether that field is or isn't getting dropped.
func testPod(tCtx ktesting.TContext, draEnabled bool) {
	tCtx.Parallel()
	namespace := createTestNamespace(tCtx, nil)
	podWithClaimName := podWithClaimName.DeepCopy()
	podWithClaimName.Namespace = namespace
	pod, err := tCtx.Client().CoreV1().Pods(namespace).Create(tCtx, podWithClaimName, metav1.CreateOptions{})
	tCtx.ExpectNoError(err, "create pod")
	if draEnabled {
		assert.NotEmpty(tCtx, pod.Spec.ResourceClaims, "should store resource claims in pod spec")
	} else {
		assert.Empty(tCtx, pod.Spec.ResourceClaims, "should drop resource claims from pod spec")
	}
}

// testAPIDisabled checks that the resource.k8s.io API is disabled.
func testAPIDisabled(tCtx ktesting.TContext) {
	tCtx.Parallel()
	_, err := tCtx.Client().ResourceV1beta1().ResourceClaims(claim.Namespace).Create(tCtx, claim, metav1.CreateOptions{})
	if !apierrors.IsNotFound(err) {
		tCtx.Fatalf("expected 'resource not found' error, got %v", err)
	}
}

// testConvert creates a claim using a one API version and reads it with another.
func testConvert(tCtx ktesting.TContext) {
	tCtx.Parallel()
	namespace := createTestNamespace(tCtx, nil)
	claim := claim.DeepCopy()
	claim.Namespace = namespace
	claim, err := tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).Create(tCtx, claim, metav1.CreateOptions{})
	tCtx.ExpectNoError(err, "create claim")
	claimAlpha, err := tCtx.Client().ResourceV1alpha3().ResourceClaims(namespace).Get(tCtx, claim.Name, metav1.GetOptions{})
	tCtx.ExpectNoError(err, "get claim")
	// We could check more fields, but there are unit tests which cover this better.
	assert.Equal(tCtx, claim.Name, claimAlpha.Name, "claim name")
}

// testAdminAccess creates a claim with AdminAccess and then checks
// whether that field is or isn't getting dropped.
// when the AdminAccess feature is enabled, it also checks that the field
// is only allowed to be used in namespace with the Resource Admin Access label
func testAdminAccess(tCtx ktesting.TContext, adminAccessEnabled bool) {
	namespace := createTestNamespace(tCtx, nil)
	claim1 := claim.DeepCopy()
	claim1.Namespace = namespace
	claim1.Spec.Devices.Requests[0].AdminAccess = ptr.To(true)
	// create claim with AdminAccess in non-admin namespace
	_, err := tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).Create(tCtx, claim1, metav1.CreateOptions{})
	if adminAccessEnabled {
		if err != nil {
			// should result in validation error
			assert.ErrorContains(tCtx, err, "admin access to devices requires the `resource.k8s.io/admin-access: true` label on the containing namespace", "the error message should have contained the expected error message")
			return
		} else {
			tCtx.Fatal("expected validation error(s), got none")
		}

		// create claim with AdminAccess in admin namespace
		adminNS := createTestNamespace(tCtx, map[string]string{"resource.k8s.io/admin-access": "true"})
		claim2 := claim.DeepCopy()
		claim2.Namespace = adminNS
		claim2.Name = "claim2"
		claim2.Spec.Devices.Requests[0].AdminAccess = ptr.To(true)
		claim2, err := tCtx.Client().ResourceV1beta1().ResourceClaims(adminNS).Create(tCtx, claim2, metav1.CreateOptions{})
		tCtx.ExpectNoError(err, "create claim")
		if !ptr.Deref(claim2.Spec.Devices.Requests[0].AdminAccess, true) {
			tCtx.Fatalf("should store AdminAccess in ResourceClaim %v", claim2)
		}
	} else {
		if claim.Spec.Devices.Requests[0].AdminAccess != nil {
			tCtx.Fatal("should drop AdminAccess in ResourceClaim")
		}
	}
}

func testPrioritizedList(tCtx ktesting.TContext, enabled bool) {
	tCtx.Parallel()
	_, err := tCtx.Client().ResourceV1beta1().DeviceClasses().Create(tCtx, class, metav1.CreateOptions{})
	tCtx.ExpectNoError(err, "create class")
	namespace := createTestNamespace(tCtx, nil)
	claim := claimPrioritizedList.DeepCopy()
	claim.Namespace = namespace
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).Create(tCtx, claim, metav1.CreateOptions{})

	if !enabled {
		require.Error(tCtx, err, "claim should have become invalid after dropping FirstAvailable")
		return
	}

	require.NotEmpty(tCtx, claim.Spec.Devices.Requests[0].FirstAvailable, "should store FirstAvailable")
	tCtx.Run("scheduler", func(tCtx ktesting.TContext) {
		startScheduler(tCtx)

		// The fake cluster configuration is not complete enough to actually schedule pods.
		// That is covered over in test/integration/scheduler_perf.
		// Here we only test that we get to the point where it notices that, without failing
		// during PreFilter because of FirstAvailable.
		pod := podWithClaimName.DeepCopy()
		pod.Namespace = namespace
		_, err := tCtx.Client().CoreV1().Pods(namespace).Create(tCtx, pod, metav1.CreateOptions{})
		tCtx.ExpectNoError(err, "create pod")
		schedulingAttempted := gomega.HaveField("Status.Conditions", gomega.ContainElement(
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Type":    gomega.Equal(v1.PodScheduled),
				"Status":  gomega.Equal(v1.ConditionFalse),
				"Reason":  gomega.Equal("Unschedulable"),
				"Message": gomega.Equal("no nodes available to schedule pods"),
			}),
		))
		ktesting.Eventually(tCtx, func(tCtx ktesting.TContext) *v1.Pod {
			pod, err := tCtx.Client().CoreV1().Pods(namespace).Get(tCtx, pod.Name, metav1.GetOptions{})
			tCtx.ExpectNoError(err, "get pod")
			return pod
		}).WithTimeout(time.Minute).WithPolling(time.Second).Should(schedulingAttempted)
	})
}

func testPublishResourceSlices(tCtx ktesting.TContext, disabledFeatures ...featuregate.Feature) {
	tCtx.Parallel()

	tCtx = ktesting.WithTimeout(tCtx, 30*time.Second, "test timed out")
	driverName := "dra.example.com"
	poolName := "global"
	resources := &resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			poolName: {
				Slices: []resourceslice.Slice{
					{
						Devices: []resourcev1beta2.Device{
							{
								Name: "device-simple",
							},
						},
					},
					{
						SharedCounters: []resourcev1beta2.CounterSet{{
							Name: "gpu-0",
							Counters: map[string]resourcev1beta2.Counter{
								"mem": {Value: resource.MustParse("1")},
							},
						}},
						Devices: []resourcev1beta2.Device{
							{
								Name: "device-tainted-default",
								Taints: []resourcev1beta2.DeviceTaint{{
									Key:    "dra.example.com/taint",
									Value:  "taint-value",
									Effect: resourcev1beta2.DeviceTaintEffectNoExecute,
									// TimeAdded is added by apiserver.
								}},
							},
							{
								Name: "device-tainted-time-added",
								Taints: []resourcev1beta2.DeviceTaint{{
									Key:       "dra.example.com/taint",
									Value:     "taint-value",
									Effect:    resourcev1beta2.DeviceTaintEffectNoExecute,
									TimeAdded: ptr.To(metav1.Now()),
								}},
							},
							{
								Name: "gpu",
								ConsumesCounters: []resourcev1beta2.DeviceCounterConsumption{{
									CounterSet: "gpu-0",
									Counters: map[string]resourcev1beta2.Counter{
										"mem": {Value: resource.MustParse("1")},
									},
								}},
							},
						},
					},
				},
			},
		},
	}
	var gotDroppedFieldError atomic.Bool
	var gotValidationError atomic.Bool
	var validationErrorsOkay atomic.Bool
	opts := resourceslice.Options{
		DriverName: driverName,
		KubeClient: tCtx.Client(),
		SyncDelay:  ptr.To(0 * time.Second),
		Resources:  resources,
		ErrorHandler: func(ctx context.Context, err error, msg string) {
			klog.FromContext(ctx).Info("ErrorHandler called", "err", err, "msg", msg)
			if !validationErrorsOkay.Load() && len(disabledFeatures) == 0 {
				assert.NoError(tCtx, err, msg)
				return
			}

			var droppedFields *resourceslice.DroppedFieldsError
			if errors.As(err, &droppedFields) {
				var disabled []string
				for _, feature := range disabledFeatures {
					disabled = append(disabled, string(feature))
				}
				assert.ErrorContains(tCtx, err, fmt.Sprintf("pool %q, slice #1: some fields were dropped by the apiserver, probably because these features are disabled: %s", poolName, strings.Join(disabled, " ")))
				gotDroppedFieldError.Store(true)
			} else if validationErrorsOkay.Load() && apierrors.IsInvalid(err) {
				gotValidationError.Store(true)
			} else {
				tCtx.Errorf("unexpected error: %v", err)
			}
		},
	}
	controller, err := resourceslice.StartController(tCtx, opts)
	tCtx.ExpectNoError(err, "start controller")
	defer controller.Stop()

	// Two create calls should be all that are needed.
	expectedStats := resourceslice.Stats{
		NumCreates: 2,
	}
	getStats := func(tCtx ktesting.TContext) resourceslice.Stats {
		return controller.GetStats()
	}
	ktesting.Eventually(tCtx, getStats).WithTimeout(10 * time.Second).Should(gomega.Equal(expectedStats))

	// No further changes necessary.
	ktesting.Consistently(tCtx, getStats).WithTimeout(10 * time.Second).Should(gomega.Equal(expectedStats))

	if len(disabledFeatures) > 0 && !gotDroppedFieldError.Load() {
		tCtx.Error("expected dropped fields error, got none")
	}

	// Now switch to one invalid slice.
	pool := resources.Pools[poolName]
	pool.Slices = pool.Slices[:1]
	pool.Slices[0].Devices[0].Attributes = map[resourcev1beta2.QualifiedName]resourcev1beta2.DeviceAttribute{"empty": {}}
	resources.Pools[poolName] = pool
	validationErrorsOkay.Store(true)
	controller.Update(resources)
	ktesting.Eventually(tCtx, getStats).WithTimeout(10*time.Second).Should(gomega.HaveField("NumDeletes", gomega.BeNumerically(">=", int64(1))), "Slice should have been removed.")
	ktesting.Eventually(tCtx, func(tCtx ktesting.TContext) bool {
		return gotValidationError.Load()
	}).WithTimeout(10 * time.Second).Should(gomega.BeTrueBecause("Should have gotten another error because the slice is invalid."))

}

// testResourceClaimDeviceStatus creates a ResourceClaim with an invalid device (not allocated device)
// and checks that the object is not validated (feature enabled) resp. accepted without the field (disabled).
//
// When enabled, it tries server-side-apply (SSA) with different clients. This is what DRA drivers should be using.
func testResourceClaimDeviceStatus(tCtx ktesting.TContext, enabled bool) {
	namespace := createTestNamespace(tCtx, nil)

	claim := &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: claimName,
		},
		Spec: resourceapi.ResourceClaimSpec{
			Devices: resourceapi.DeviceClaim{
				Requests: []resourceapi.DeviceRequest{
					{
						Name:            "foo",
						DeviceClassName: "foo",
					},
				},
			},
		},
	}

	claim, err := tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).Create(tCtx, claim, metav1.CreateOptions{})
	tCtx.ExpectNoError(err, "create ResourceClaim")

	deviceStatus := []resourceapi.AllocatedDeviceStatus{{
		Driver: "one",
		Pool:   "global",
		Device: "my-device",
		Data: &runtime.RawExtension{
			Raw: []byte(`{"kind": "foo", "apiVersion": "dra.example.com/v1"}`),
		},
		NetworkData: &resourceapi.NetworkDeviceData{
			InterfaceName: "net-1",
			IPs: []string{
				"10.9.8.0/24",
				"2001:db8::/64",
			},
			HardwareAddress: "ea:9f:cb:40:b1:7b",
		},
	}}
	claim.Status.Devices = deviceStatus
	updatedClaim, err := tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).UpdateStatus(tCtx, claim, metav1.UpdateOptions{})
	if !enabled {
		tCtx.ExpectNoError(err, "updating the status with an invalid AllocatedDeviceStatus should have worked because the field should have been dropped")
		require.Empty(tCtx, updatedClaim.Status.Devices, "field should have been dropped")
		return
	}

	// Tests for enabled feature follow.

	if err == nil {
		tCtx.Fatal("updating the status with an invalid AllocatedDeviceStatus should have failed and didn't")
	}

	// Add an allocation result.
	claim.Status.Allocation = &resourceapi.AllocationResult{
		Devices: resourceapi.DeviceAllocationResult{
			Results: []resourceapi.DeviceRequestAllocationResult{
				{
					Request: "foo",
					Driver:  "one",
					Pool:    "global",
					Device:  "my-device",
				},
				{
					Request: "foo",
					Driver:  "two",
					Pool:    "global",
					Device:  "another-device",
				},
				{
					Request: "foo",
					Driver:  "three",
					Pool:    "global",
					Device:  "my-device",
				},
			},
		},
	}
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).UpdateStatus(tCtx, claim, metav1.UpdateOptions{})
	tCtx.ExpectNoError(err, "add allocation result")

	// Now adding the device status should work.
	claim.Status.Devices = deviceStatus
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).UpdateStatus(tCtx, claim, metav1.UpdateOptions{})
	tCtx.ExpectNoError(err, "add device status")
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after adding device status")

	// Strip the RawExtension. SSA re-encodes it, which causes negligble differences that nonetheless break assert.Equal.
	claim.Status.Devices[0].Data = nil
	deviceStatus[0].Data = nil
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).UpdateStatus(tCtx, claim, metav1.UpdateOptions{})
	tCtx.ExpectNoError(err, "add device status")
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after stripping RawExtension")

	// Exercise SSA.
	deviceStatusAC := resourceapiac.AllocatedDeviceStatus().
		WithDriver("two").
		WithPool("global").
		WithDevice("another-device").
		WithNetworkData(resourceapiac.NetworkDeviceData().WithInterfaceName("net-2"))
	deviceStatus = append(deviceStatus, resourceapi.AllocatedDeviceStatus{
		Driver: "two",
		Pool:   "global",
		Device: "another-device",
		NetworkData: &resourceapi.NetworkDeviceData{
			InterfaceName: "net-2",
		},
	})
	claimAC := resourceapiac.ResourceClaim(claim.Name, claim.Namespace).
		WithStatus(resourceapiac.ResourceClaimStatus().WithDevices(deviceStatusAC))
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).ApplyStatus(tCtx, claimAC, metav1.ApplyOptions{
		Force:        true,
		FieldManager: "manager-1",
	})
	tCtx.ExpectNoError(err, "apply device status two")
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after applying device status two")

	deviceStatusAC = resourceapiac.AllocatedDeviceStatus().
		WithDriver("three").
		WithPool("global").
		WithDevice("my-device").
		WithNetworkData(resourceapiac.NetworkDeviceData().WithInterfaceName("net-3"))
	deviceStatus = append(deviceStatus, resourceapi.AllocatedDeviceStatus{
		Driver: "three",
		Pool:   "global",
		Device: "my-device",
		NetworkData: &resourceapi.NetworkDeviceData{
			InterfaceName: "net-3",
		},
	})
	claimAC = resourceapiac.ResourceClaim(claim.Name, claim.Namespace).
		WithStatus(resourceapiac.ResourceClaimStatus().WithDevices(deviceStatusAC))
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).ApplyStatus(tCtx, claimAC, metav1.ApplyOptions{
		Force:        true,
		FieldManager: "manager-2",
	})
	tCtx.ExpectNoError(err, "apply device status three")
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after applying device status three")
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("   ", "   ")
	tCtx.ExpectNoError(encoder.Encode(claim))
	tCtx.Logf("Final ResourceClaim:\n%s", buffer.String())

	// Update one entry, remove the other.
	deviceStatusAC = resourceapiac.AllocatedDeviceStatus().
		WithDriver("two").
		WithPool("global").
		WithDevice("another-device").
		WithNetworkData(resourceapiac.NetworkDeviceData().WithInterfaceName("yet-another-net"))
	deviceStatus[1].NetworkData.InterfaceName = "yet-another-net"
	claimAC = resourceapiac.ResourceClaim(claim.Name, claim.Namespace).
		WithStatus(resourceapiac.ResourceClaimStatus().WithDevices(deviceStatusAC))
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).ApplyStatus(tCtx, claimAC, metav1.ApplyOptions{
		Force:        true,
		FieldManager: "manager-1",
	})
	tCtx.ExpectNoError(err, "update device status two")
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after updating device status two")
	claimAC = resourceapiac.ResourceClaim(claim.Name, claim.Namespace)
	deviceStatus = deviceStatus[0:2]
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).ApplyStatus(tCtx, claimAC, metav1.ApplyOptions{
		Force:        true,
		FieldManager: "manager-2",
	})
	tCtx.ExpectNoError(err, "remove device status three")
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after removing device status three")
}

// testResourceClaimDeviceStatusWithConsumableCapacity checks the upgrade path of adding SharedUID to
// the keys of the devices +listType=map.
func testResourceClaimDeviceStatusWithConsumableCapacity(tCtx ktesting.TContext) {
	namespace := createTestNamespace(tCtx, nil)
	claimName := "test-claim"
	etcd := getEtcd(tCtx)

	// Store a ResourceClaim as it would have been stored by Kubernetes 1.32,
	// with three keys instead of four.
	data := `{
              "kind": "ResourceClaim",
              "apiVersion": "resource.k8s.io/v1beta1",
              "metadata": {
                 "name": "test-claim",
                 "namespace": "test-namespace",
                 "uid": "90715e47-024e-45eb-88fb-1cc0313e7d39",
                 "resourceVersion": "75",
                 "creationTimestamp": "2025-05-27T14:08:37Z",
                 "managedFields": [
                    {
                       "manager": "manager-1",
                       "operation": "Apply",
                       "apiVersion": "resource.k8s.io/v1beta1",
                       "time": "2025-05-27T14:08:37Z",
                       "fieldsType": "FieldsV1",
                       "fieldsV1": {
                          "f:status": {
                             "f:devices": {
                                "k:{\"device\":\"another-device\",\"driver\":\"two\",\"pool\":\"global\"}": {
                                   ".": {},
                                   "f:device": {},
                                   "f:driver": {},
                                   "f:networkData": {
                                      "f:interfaceName": {}
                                   },
                                   "f:pool": {}
                                }
                             }
                          }
                       },
                       "subresource": "status"
                    },
                    {
                       "manager": "manager-2",
                       "operation": "Apply",
                       "apiVersion": "resource.k8s.io/v1beta1",
                       "time": "2025-05-27T14:08:37Z",
                       "fieldsType": "FieldsV1",
                       "fieldsV1": {
                          "f:status": {
                             "f:devices": {
                                "k:{\"device\":\"my-device\",\"driver\":\"three\",\"pool\":\"global\"}": {
                                   ".": {},
                                   "f:device": {},
                                   "f:driver": {},
                                   "f:networkData": {
                                      "f:interfaceName": {}
                                   },
                                   "f:pool": {}
                                }
                             }
                          }
                       },
                       "subresource": "status"
                    },
                    {
                       "manager": "dra.test",
                       "operation": "Update",
                       "apiVersion": "resource.k8s.io/v1beta1",
                       "time": "2025-05-27T14:08:37Z",
                       "fieldsType": "FieldsV1",
                       "fieldsV1": {
                          "f:spec": {
                             "f:devices": {
                                "f:requests": {}
                             }
                          }
                       }
                    },
                    {
                       "manager": "dra.test",
                       "operation": "Update",
                       "apiVersion": "resource.k8s.io/v1beta1",
                       "time": "2025-05-27T14:08:37Z",
                       "fieldsType": "FieldsV1",
                       "fieldsV1": {
                          "f:status": {
                             "f:allocation": {
                                ".": {},
                                "f:devices": {
                                   ".": {},
                                   "f:results": {}
                                }
                             },
                             "f:devices": {
                                ".": {},
                                "k:{\"device\":\"my-device\",\"driver\":\"one\",\"pool\":\"global\"}": {
                                   ".": {},
                                   "f:conditions": {},
                                   "f:device": {},
                                   "f:driver": {},
                                   "f:networkData": {
                                      ".": {},
                                      "f:hardwareAddress": {},
                                      "f:interfaceName": {},
                                      "f:ips": {}
                                   },
                                   "f:pool": {}
                                }
                             }
                          }
                       },
                       "subresource": "status"
                    }
                 ]
              },
              "spec": {
                 "devices": {
                    "requests": [
                       {
                          "name": "foo",
                          "deviceClassName": "foo",
                          "allocationMode": "ExactCount",
                          "count": 1
                       }
                    ]
                 }
              },
              "status": {
                 "allocation": {
                    "devices": {
                       "results": [
                          {
                             "request": "foo",
                             "driver": "one",
                             "pool": "global",
                             "device": "my-device",
                             "adminAccess": null
                          },
                          {
                             "request": "foo",
                             "driver": "two",
                             "pool": "global",
                             "device": "another-device",
                             "adminAccess": null
                          },
                          {
                             "request": "foo",
                             "driver": "three",
                             "pool": "global",
                             "device": "my-device",
                             "adminAccess": null
                          }
                       ]
                    }
                 },
                 "devices": [
                    {
                       "driver": "one",
                       "pool": "global",
                       "device": "my-device",
                       "conditions": null,
                       "networkData": {
                          "interfaceName": "net-1",
                          "ips": [
                             "10.9.8.0/24",
                             "2001:db8::/64"
                          ],
                          "hardwareAddress": "ea:9f:cb:40:b1:7b"
                       }
                    },
                    {
                       "driver": "two",
                       "pool": "global",
                       "device": "another-device",
                       "conditions": null,
                       "networkData": {
                          "interfaceName": "net-2"
                       }
                    },
                    {
                       "driver": "three",
                       "pool": "global",
                       "device": "my-device",
                       "conditions": null,
                       "networkData": {
                          "interfaceName": "net-3"
                       }
                    }
                 ]
              }
           }
`
	data = strings.ReplaceAll(data, "test-namespace", namespace)
	data = strings.ReplaceAll(data, "\n", "")
	etcd.put(tCtx, "resourceclaims", namespace, claimName, data)

	deviceStatus := []resourceapi.AllocatedDeviceStatus{
		{
			Driver: "one",
			Pool:   "global",
			Device: "my-device",
			NetworkData: &resourceapi.NetworkDeviceData{
				InterfaceName: "net-1",
				IPs: []string{
					"10.9.8.0/24",
					"2001:db8::/64",
				},
				HardwareAddress: "ea:9f:cb:40:b1:7b",
			},
		},
		{
			Driver: "two",
			Pool:   "global",
			Device: "another-device",
			NetworkData: &resourceapi.NetworkDeviceData{
				InterfaceName: "net-2",
			},
		},
		{
			Driver: "three",
			Pool:   "global",
			Device: "my-device",
			NetworkData: &resourceapi.NetworkDeviceData{
				InterfaceName: "net-3",
			},
		},
	}
	claim, err := tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).Get(tCtx, claimName, metav1.GetOptions{})
	tCtx.ExpectNoError(err, "get pre-generated ResourceClaim")
	require.Equal(tCtx, deviceStatus, claim.Status.Devices)

	// Update one entry, remove the other.
	deviceStatusAC := resourceapiac.AllocatedDeviceStatus().
		WithDriver("two").
		WithPool("global").
		WithDevice("another-device").
		WithNetworkData(resourceapiac.NetworkDeviceData().WithInterfaceName("yet-another-net"))
	deviceStatus[1].NetworkData.InterfaceName = "yet-another-net"
	claimAC := resourceapiac.ResourceClaim(claim.Name, claim.Namespace).
		WithStatus(resourceapiac.ResourceClaimStatus().WithDevices(deviceStatusAC))
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).ApplyStatus(tCtx, claimAC, metav1.ApplyOptions{
		Force:        true,
		FieldManager: "manager-1",
	})
	tCtx.ExpectNoError(err, "update device status two")
	tCtx.Logf("Managed fields after updating device status two:\n%s", format.Object(claim.ManagedFields, 1))
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after updating device status two")
	claimAC = resourceapiac.ResourceClaim(claim.Name, claim.Namespace)
	deviceStatus = deviceStatus[0:2]
	claim, err = tCtx.Client().ResourceV1beta1().ResourceClaims(namespace).ApplyStatus(tCtx, claimAC, metav1.ApplyOptions{
		Force:        true,
		FieldManager: "manager-2",
	})
	tCtx.ExpectNoError(err, "remove device status three")
	tCtx.Logf("Managed fields after updating device status three:\n%s", format.Object(claim.ManagedFields, 1))
	require.Equal(tCtx, deviceStatus, claim.Status.Devices, "after removing device status three")
}

// testMaxResourceSlice creates a ResourceSlice that is as large as possible
// and prints some information about it.
func testMaxResourceSlice(tCtx ktesting.TContext) {
	slice := NewMaxResourceSlice()
	createdSlice, err := tCtx.Client().ResourceV1beta2().ResourceSlices().Create(tCtx, slice, metav1.CreateOptions{})
	tCtx.ExpectNoError(err)
	totalSize := createdSlice.Size()
	var managedFieldsSize int
	for _, f := range createdSlice.ManagedFields {
		managedFieldsSize += f.Size()
	}
	specSize := createdSlice.Spec.Size()
	tCtx.Logf("\n\nTotal size: %s\nManagedFields size: %s (%.0f%%)\nSpec size: %s (%.0f)%%\n\nManagedFields:\n%s",
		resource.NewQuantity(int64(totalSize), resource.BinarySI),
		resource.NewQuantity(int64(managedFieldsSize), resource.BinarySI), float64(managedFieldsSize)*100/float64(totalSize),
		resource.NewQuantity(int64(specSize), resource.BinarySI), float64(specSize)*100/float64(totalSize),
		klog.Format(createdSlice.ManagedFields),
	)
	if diff := cmp.Diff(slice.Spec, createdSlice.Spec); diff != "" {
		tCtx.Errorf("ResourceSliceSpec got modified during Create (- want, + got):\n%s", diff)
	}
}
