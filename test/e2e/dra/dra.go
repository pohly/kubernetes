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

package dra

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	// podStartTimeout is how long to wait for the pod to be started.
	podStartTimeout = 5 * time.Minute
)

var _ = ginkgo.Describe("[sig-node] DRA [Feature:DynamicResourceAllocation]", func() {
	f := framework.NewDefaultFramework("dra")
	ctx := context.Background()

	// The driver containers have to run with sufficient privileges to
	// modify /var/lib/kubelet/plugins.
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.Context("kubelet", func() {
		driver := NewDriver(f, 1, 1 /* nodes */) // All tests get their own driver instance.
		b := newBuilder(f, driver)

		ginkgo.It("registers plugin", func() {
			ginkgo.By("the driver is running")
		})

		// This test does not pass at the moment because kubelet doesn't retry.
		ginkgo.It("must retry NodePrepareResource", func() {
			// We have exactly one host.
			m := MethodInstance{driver.Hostnames()[0], NodePrepareResourceMethod}

			driver.Fail(m, true)

			ginkgo.By("waiting for container startup to fail")
			parameters := b.resourceClaimParameters()
			pod := b.podInline(corev1.AllocationModeWaitForFirstConsumer)

			b.create(ctx, parameters, pod)

			framework.By("wait for NodePrepareResource call", func() {
				gomega.Eventually(func() error {
					if driver.CallCount(m) == 0 {
						return errors.New("NodePrepareResource not called yet")
					}
					return nil
				}).WithTimeout(podStartTimeout).Should(gomega.Succeed())
			})

			ginkgo.By("allowing container startup to succeed")
			callCount := driver.CallCount(m)
			driver.Fail(m, false)
			err := e2epod.WaitForPodNameRunningInNamespace(f.ClientSet, pod.Name, pod.Namespace)
			framework.ExpectNoError(err, "start pod with inline resource claim")
			if driver.CallCount(m) == callCount {
				framework.Fail("NodePrepareResource should have been called again")
			}
		})
	})

	ginkgo.Context("cluster", func() {
		driver := NewDriver(f, 1, 4 /* nodes */)
		b := newBuilder(f, driver)

		// claimTests tries out several different combinations of pods with
		// claims, both inline and external.
		claimTests := func(allocationMode corev1.AllocationMode) {
			ginkgo.It("supports simple pod referencing inline resource claim", func() {
				parameters := b.resourceClaimParameters()
				pod := b.podInline(allocationMode)
				b.create(ctx, parameters, pod)

				b.testPod(f.ClientSet, pod)
			})

			ginkgo.It("supports inline claim referenced by multiple containers", func() {
				parameters := b.resourceClaimParameters()
				pod := b.podInlineMultiple(allocationMode)
				b.create(ctx, parameters, pod)

				b.testPod(f.ClientSet, pod)
			})

			ginkgo.It("supports simple pod referencing external resource claim", func() {
				parameters := b.resourceClaimParameters()
				pod := b.podExternal()
				b.create(ctx, parameters, b.externalClaim(allocationMode), pod)

				b.testPod(f.ClientSet, pod)
			})

			ginkgo.It("supports external claim referenced by multiple pods", func() {
				parameters := b.resourceClaimParameters()
				pod1 := b.podExternal()
				pod2 := b.podExternal()
				pod3 := b.podExternal()
				claim := b.externalClaim(allocationMode)
				b.create(ctx, parameters, claim, pod1, pod2, pod3)

				for _, pod := range []*corev1.Pod{pod1, pod2, pod3} {
					b.testPod(f.ClientSet, pod)
				}
			})

			ginkgo.It("supports external claim referenced by multiple containers of multiple pods", func() {
				parameters := b.resourceClaimParameters()
				pod1 := b.podExternalMultiple()
				pod2 := b.podExternalMultiple()
				pod3 := b.podExternalMultiple()
				claim := b.externalClaim(allocationMode)
				b.create(ctx, parameters, claim, pod1, pod2, pod3)

				for _, pod := range []*corev1.Pod{pod1, pod2, pod3} {
					b.testPod(f.ClientSet, pod)
				}
			})
		}

		ginkgo.Context("with delayed allocation", func() {
			claimTests(corev1.AllocationModeWaitForFirstConsumer)
		})

		ginkgo.Context("with immediate allocation", func() {
			claimTests(corev1.AllocationModeImmediate)
		})

	})
})

// builder contains a running counter to make objects unique within thir
// namespace.
type builder struct {
	f      *framework.Framework
	driver *Driver

	podCounter                     int
	resourceClaimParametersCounter int
}

// className returns the default resource class name.
func (b *builder) className() string {
	return b.f.UniqueName + "-class"
}

// class returns the resource class that the builder's other objects
// reference.
func (b *builder) class() *corev1.ResourceClass {
	return &corev1.ResourceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: b.className(),
		},
		DriverName:    b.driver.Name,
		SuitableNodes: b.nodeSelector(),
	}
}

// nodeSelector returns a node selector that matches all nodes on which the
// kubelet plugin was deployed.
func (b *builder) nodeSelector() *corev1.NodeSelector {
	return &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      "kubernetes.io/hostname",
						Operator: corev1.NodeSelectorOpIn,
						Values:   b.driver.Hostnames(),
					},
				},
			},
		},
	}
}

// externalClaim returns external resource claim
// that test pods can reference
func (b *builder) externalClaim(allocationMode corev1.AllocationMode) *corev1.ResourceClaim {
	return &corev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "external-claim",
		},
		Spec: corev1.ResourceClaimSpec{
			ResourceClassName: b.className(),
			Parameters: &corev1.ResourceClaimParametersReference{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       b.resourceClaimParametersName(),
			},
			AllocationMode: allocationMode,
		},
	}
}

// resourceClaimParametersName returns the current ConfigMap name for resource
// claim parameters.
func (b *builder) resourceClaimParametersName() string {
	return fmt.Sprintf("resource-claim-parameters-%d", b.resourceClaimParametersCounter)
}

// resourceClaimParametersEnv returns the default env variables.
func (b *builder) resourceClaimParametersEnv() map[string]string {
	return map[string]string{
		"a": "b",
	}
}

// resourceClaimParameters returns a config map with the default env variables.
func (b *builder) resourceClaimParameters() *corev1.ConfigMap {
	b.resourceClaimParametersCounter++
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: b.f.Namespace.Name,
			Name:      b.resourceClaimParametersName(),
		},
		Data: b.resourceClaimParametersEnv(),
	}
}

// makePod returns a simple pod with no resource claims.
// The pod prints its env and waits.
func (b *builder) pod() *corev1.Pod {
	pod := e2epod.MakePod(b.f.Namespace.Name, nil, nil, false, "env && sleep 100000")
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	var zero int64
	pod.Spec.TerminationGracePeriodSeconds = &zero
	pod.ObjectMeta.GenerateName = ""
	b.podCounter++
	pod.ObjectMeta.Name = fmt.Sprintf("tester-%d", b.podCounter)
	return pod
}

// makePodInline adds an inline resource claim with default class name and parameters.
func (b *builder) podInline(allocationMode corev1.AllocationMode) *corev1.Pod {
	pod := b.pod()
	pod.Spec.Containers[0].Name = "with-resource"
	podClaimName := "my-inline-claim"
	pod.Spec.Containers[0].Resources.Claims = []string{podClaimName}
	pod.Spec.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name: podClaimName,
			Claim: corev1.ClaimSource{
				Template: &corev1.ResourceClaimTemplate{
					Spec: corev1.ResourceClaimSpec{
						ResourceClassName: b.className(),
						Parameters: &corev1.ResourceClaimParametersReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       b.resourceClaimParametersName(),
						},
						AllocationMode: allocationMode,
					},
				},
			},
		},
	}
	return pod
}

// podInlineMultiple returns a pod with inline resource claim referenced by 3 containers
func (b *builder) podInlineMultiple(allocationMode corev1.AllocationMode) *corev1.Pod {
	pod := b.podInline(allocationMode)
	pod.Spec.Containers = append(pod.Spec.Containers, *pod.Spec.Containers[0].DeepCopy(), *pod.Spec.Containers[0].DeepCopy())
	pod.Spec.Containers[1].Name = pod.Spec.Containers[1].Name + "-1"
	pod.Spec.Containers[2].Name = pod.Spec.Containers[1].Name + "-2"
	return pod
}

// podExternal adds a pod that references external resource claim with default class name and parameters.
func (b *builder) podExternal() *corev1.Pod {
	pod := b.pod()
	pod.Spec.Containers[0].Name = "with-resource"
	podClaimName := "resource-claim"
	externalClaimName := "external-claim"
	pod.Spec.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name: podClaimName,
			Claim: corev1.ClaimSource{
				ResourceClaimName: &externalClaimName,
			},
		},
	}
	pod.Spec.Containers[0].Resources.Claims = []string{podClaimName}
	return pod
}

// podShared returns a pod with 3 containers that reference external resource claim with default class name and parameters.
func (b *builder) podExternalMultiple() *corev1.Pod {
	pod := b.podExternal()
	pod.Spec.Containers = append(pod.Spec.Containers, *pod.Spec.Containers[0].DeepCopy(), *pod.Spec.Containers[0].DeepCopy())
	pod.Spec.Containers[1].Name = pod.Spec.Containers[1].Name + "-1"
	pod.Spec.Containers[2].Name = pod.Spec.Containers[1].Name + "-2"
	return pod
}

// create takes a bunch of objects and calls their Create function.
func (b *builder) create(ctx context.Context, objs ...klog.KMetadata) {
	for _, obj := range objs {
		ginkgo.By(fmt.Sprintf("creating %T %s", obj, obj.GetName()), func() {
			var err error
			switch obj := obj.(type) {
			case *corev1.ResourceClass:
				_, err = b.f.ClientSet.CoreV1().ResourceClasses().Create(ctx, obj, metav1.CreateOptions{})
			case *corev1.Pod:
				_, err = b.f.ClientSet.CoreV1().Pods(b.f.Namespace.Name).Create(ctx, obj, metav1.CreateOptions{})
			case *corev1.ConfigMap:
				_, err = b.f.ClientSet.CoreV1().ConfigMaps(b.f.Namespace.Name).Create(ctx, obj, metav1.CreateOptions{})
			case *corev1.ResourceClaim:
				_, err = b.f.ClientSet.CoreV1().ResourceClaims(b.f.Namespace.Name).Create(ctx, obj, metav1.CreateOptions{})
			default:
				framework.Fail(fmt.Sprintf("internal error, unsupported type %T", obj), 1)
			}
			framework.ExpectNoErrorWithOffset(1, err, "create %T", obj)
		})
	}
}

// testPod runs pod and checks if container logs contain expected environment variables
func (b *builder) testPod(clientSet kubernetes.Interface, pod *corev1.Pod) {
	err := e2epod.WaitForPodRunningInNamespace(clientSet, pod)
	framework.ExpectNoError(err, "start pod")

	for _, container := range pod.Spec.Containers {
		log, err := e2epod.GetPodLogs(clientSet, pod.Namespace, pod.Name, container.Name)
		framework.ExpectNoError(err, "get logs")
		var envStr string
		for key, value := range b.resourceClaimParametersEnv() {
			envStr = fmt.Sprintf("\nuser_%s=%s\n", key, value)
			break
		}
		gomega.Expect(log).To(gomega.ContainSubstring(envStr), "container env variables")
	}
}

func newBuilder(f *framework.Framework, driver *Driver) *builder {
	b := &builder{f: f, driver: driver}

	ginkgo.BeforeEach(b.setUp)

	return b
}

func (b *builder) setUp() {
	b.create(context.Background(), b.class())
	ginkgo.DeferCleanup(b.tearDown)
}

func (b *builder) tearDown() {
	ctx := context.Background()

	err := b.f.ClientSet.CoreV1().ResourceClasses().Delete(ctx, b.className(), metav1.DeleteOptions{})
	framework.ExpectNoError(err, "delete resource class")

	// Before we allow the namespace and all objects in it
	// do be deleted by the framework, we must ensure that
	// test pods and the claims that they use are
	// deleted. Otherwise the driver might get deleted
	// first, in which case deleting the claims won't work
	// anymore.
	framework.By("delete pods and claims", func() {
		framework.SucceedEventually(func(g gomega.Gomega) {
			pods, err := b.f.ClientSet.CoreV1().Pods(b.f.Namespace.Name).List(ctx, metav1.ListOptions{})
			var testPods []corev1.Pod
			g.Expect(err).NotTo(gomega.HaveOccurred(), "list pods")
			oneSecond := int64(1)
			for _, pod := range pods.Items {
				if pod.DeletionTimestamp != nil ||
					pod.Labels["app.kubernetes.io/part-of"] == "dra-test-driver" {
					continue
				}
				ginkgo.By(fmt.Sprintf("deleting %T %s", &pod, klog.KObj(&pod)))
				err := b.f.ClientSet.CoreV1().Pods(b.f.Namespace.Name).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &oneSecond})
				g.Expect(err).NotTo(gomega.HaveOccurred(), "delete pod")
				testPods = append(testPods, pod)
			}
			g.Expect(testPods).To(gomega.BeEmpty(), "all pods removed")

			claims, err := b.f.ClientSet.CoreV1().ResourceClaims(b.f.Namespace.Name).List(ctx, metav1.ListOptions{})
			framework.ExpectNoError(err, "get resource claims")
			for _, claim := range claims.Items {
				if claim.DeletionTimestamp != nil {
					continue
				}
				ginkgo.By(fmt.Sprintf("deleting %T %s", &claim, klog.KObj(&claim)))
				err := b.f.ClientSet.CoreV1().ResourceClaims(b.f.Namespace.Name).Delete(ctx, claim.Name, metav1.DeleteOptions{})
				g.Expect(err).NotTo(gomega.HaveOccurred(), "delete claim")
			}

			g.Expect(claims.Items).To(gomega.BeEmpty(), "all resource claims removed")
		}, []interface{}{"delete pods and claims"}, 5*time.Minute, time.Second)
	})
}
