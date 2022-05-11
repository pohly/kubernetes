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

package pod_test

import (
	"testing"
	"time"

	"github.com/onsi/ginkgo"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/output"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
)

// The line number of the following code is checked in TestFailureOutput below.
// Be careful when moving it around or changing the import statements above.
// Here are some intentionally blank lines that can be removed to compensate
// for future additional import statements.
//
//
//
//
//
//
//
//
//
//
//

func runTests(t *testing.T, reporter ginkgo.Reporter) {
	// This source code line will be part of the stack dump comparison.
	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "Pod Suite", []ginkgo.Reporter{reporter})
}

var _ = ginkgo.Describe("pod", func() {
	ginkgo.It("not found", func() {
		framework.ExpectNoError(e2epod.WaitTimeoutForPodRunningInNamespace(clientSet, "no-such-pod", "default", 5*time.Second), "wait for pod running")
	})

	ginkgo.It("not running", func() {
		framework.ExpectNoError(e2epod.WaitTimeoutForPodRunningInNamespace(clientSet, podName, podNamespace, 5*time.Second), "wait for pod running")
	})
})

var (
	podName      = "pending-pod"
	podNamespace = "default"
	clientSet    = fake.NewSimpleClientset(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: podNamespace}})
)

func TestFailureOutput(t *testing.T) {
	// Sorted by name!
	expected := output.SuiteResults{
		output.TestResult{
			Name: "[Top Level] pod not found",
			Output: `INFO: Waiting up to 5s for pod "no-such-pod" in namespace "default" to be "running"
INFO: Ignoring NotFound error while getting pod default/no-such-pod
INFO: Ignoring NotFound error while getting pod default/no-such-pod
INFO: Ignoring NotFound error while getting pod default/no-such-pod
INFO: Ignoring NotFound error while getting pod default/no-such-pod
INFO: Timed out while waiting for pod default/no-such-pod to be running. Last observed as:     <*v1.Pod>: nil
FAIL: wait for pod running
Unexpected error:
    <*fmt.wrapError>: {
        msg: "error while waiting for pod default/no-such-pod to be running: pods \"no-such-pod\" not found",
        err: {
            ErrStatus: {
                TypeMeta: {Kind: "", APIVersion: ""},
                ListMeta: {
                    SelfLink: "",
                    ResourceVersion: "",
                    Continue: "",
                    RemainingItemCount: nil,
                },
                Status: "Failure",
                Message: "pods \"no-such-pod\" not found",
                Reason: "NotFound",
                Details: {Name: "no-such-pod", Group: "", Kind: "pods", UID: "", Causes: nil, RetryAfterSeconds: 0},
                Code: 404,
            },
        },
    }
    error while waiting for pod default/no-such-pod to be running: pods "no-such-pod" not found
occurred

Full Stack Trace
k8s.io/kubernetes/test/e2e/framework/pod_test.glob..func1.1()
	wait_test.go:56
k8s.io/kubernetes/test/e2e/framework/pod_test.runTests()
	wait_test.go:51

`,
			Failure: `wait for pod running
Unexpected error:
    <*fmt.wrapError>: {
        msg: "error while waiting for pod default/no-such-pod to be running: pods \"no-such-pod\" not found",
        err: {
            ErrStatus: {
                TypeMeta: {Kind: "", APIVersion: ""},
                ListMeta: {
                    SelfLink: "",
                    ResourceVersion: "",
                    Continue: "",
                    RemainingItemCount: nil,
                },
                Status: "Failure",
                Message: "pods \"no-such-pod\" not found",
                Reason: "NotFound",
                Details: {Name: "no-such-pod", Group: "", Kind: "pods", UID: "", Causes: nil, RetryAfterSeconds: 0},
                Code: 404,
            },
        },
    }
    error while waiting for pod default/no-such-pod to be running: pods "no-such-pod" not found
occurred`,
			Stack: `k8s.io/kubernetes/test/e2e/framework/pod_test.glob..func1.1()
	wait_test.go:56
k8s.io/kubernetes/test/e2e/framework/pod_test.runTests()
	wait_test.go:51
`,
		},
		output.TestResult{
			Name: "[Top Level] pod not running",
			Output: `INFO: Waiting up to 5s for pod "pending-pod" in namespace "default" to be "running"
INFO: Pod "pending-pod": Phase="", Reason="", readiness=false. Elapsed: <elapsed>
INFO: Pod "pending-pod": Phase="", Reason="", readiness=false. Elapsed: <elapsed>
INFO: Pod "pending-pod": Phase="", Reason="", readiness=false. Elapsed: <elapsed>
INFO: Pod "pending-pod": Phase="", Reason="", readiness=false. Elapsed: <elapsed>
INFO: Timed out while waiting for pod default/pending-pod to be running. Last observed as:     <*v1.Pod>: {
        TypeMeta: {Kind: "", APIVersion: ""},
        ObjectMeta: {
            Name: "pending-pod",
            GenerateName: "",
            Namespace: "default",
            SelfLink: "",
            UID: "",
            ResourceVersion: "",
            Generation: 0,
            CreationTimestamp: {
                Time: 0001-01-01T00:00:00Z,
            },
            DeletionTimestamp: nil,
            DeletionGracePeriodSeconds: nil,
            Labels: nil,
            Annotations: nil,
            OwnerReferences: nil,
            Finalizers: nil,
            ManagedFields: nil,
        },
        Spec: {
            Volumes: nil,
            InitContainers: nil,
            Containers: nil,
            EphemeralContainers: nil,
            RestartPolicy: "",
            TerminationGracePeriodSeconds: nil,
            ActiveDeadlineSeconds: nil,
            DNSPolicy: "",
            NodeSelector: nil,
            ServiceAccountName: "",
            DeprecatedServiceAccount: "",
            AutomountServiceAccountToken: nil,
            NodeName: "",
            HostNetwork: false,
            HostPID: false,
            HostIPC: false,
            ShareProcessNamespace: nil,
            SecurityContext: nil,
            ImagePullSecrets: nil,
            Hostname: "",
            Subdomain: "",
            Affinity: nil,
            SchedulerName: "",
            Tolerations: nil,
            HostAliases: nil,
            PriorityClassName: "",
            Priority: nil,
            DNSConfig: nil,
            ReadinessGates: nil,
            RuntimeClassName: nil,
            EnableServiceLinks: nil,
            PreemptionPolicy: nil,
            Overhead: nil,
            TopologySpreadConstraints: nil,
            SetHostnameAsFQDN: nil,
            OS: nil,
        },
        Status: {
            Phase: "",
            Conditions: nil,
            Message: "",
            Reason: "",
            NominatedNodeName: "",
            HostIP: "",
            PodIP: "",
            PodIPs: nil,
            StartTime: nil,
            InitContainerStatuses: nil,
            ContainerStatuses: nil,
            QOSClass: "",
            EphemeralContainerStatuses: nil,
        },
    }
FAIL: wait for pod running
Unexpected error:
    <*pod.timeoutError>: {
        msg: "timed out while waiting for pod default/pending-pod to be running",
    }
    timed out while waiting for pod default/pending-pod to be running
occurred

Full Stack Trace
k8s.io/kubernetes/test/e2e/framework/pod_test.glob..func1.2()
	wait_test.go:60
k8s.io/kubernetes/test/e2e/framework/pod_test.runTests()
	wait_test.go:51

`,
			Failure: `wait for pod running
Unexpected error:
    <*pod.timeoutError>: {
        msg: "timed out while waiting for pod default/pending-pod to be running",
    }
    timed out while waiting for pod default/pending-pod to be running
occurred`,
			Stack: `k8s.io/kubernetes/test/e2e/framework/pod_test.glob..func1.2()
	wait_test.go:60
k8s.io/kubernetes/test/e2e/framework/pod_test.runTests()
	wait_test.go:51
`,
		},
	}

	output.TestGinkgoOutput(t, runTests, expected)
}
