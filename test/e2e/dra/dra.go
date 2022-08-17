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
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"
)

var _ = ginkgo.Describe("[sig-node] DRA [Feature:DynamicResourceAllocation]", func() {
	f := framework.NewDefaultFramework("dra")

	// The driver containers have to run with sufficient privileges to
	// modify /var/lib/kubelet/plugins.
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	// Additional nesting is needed to ensure that the framework and driver
	// instances get created and deleted in the right order (see
	// https://github.com/onsi/ginkgo/issues/1022).
	ginkgo.Context("", func() {
		var (
			ctx    = context.Background()
			driver = NewDriver(f, 1, 1 /* nodes */) // All tests get their own driver instance.
			b      = &builder{f: f, driver: driver}
		)

		ginkgo.Context("", func() {
			ginkgo.BeforeEach(func() {
				b.create(ctx, b.class())
			})

			ginkgo.AfterEach(func() {
				err := f.ClientSet.CoreV1().ResourceClasses().Delete(ctx, b.className(), metav1.DeleteOptions{})
				framework.ExpectNoError(err, "delete resource class")

				// Before we allow the namespace and all objects in it
				// do be deleted by the framework, we must ensure that
				// test pods and the claims that they use are
				// deleted. Otherwise the driver might get deleted
				// first, in which case deleting the claims won't work
				// anymore.
				gomega.Eventually(func(g gomega.Gomega) {
					pods, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).List(ctx, metav1.ListOptions{})
					g.Expect(err).NotTo(gomega.HaveOccurred(), "list pods")
					for _, pod := range pods.Items {
						if pod.DeletionTimestamp != nil ||
							pod.Labels["app.kubernetes.io/part-of"] == "dra-test-driver" {
							continue
						}
						err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(ctx, pod.Name, metav1.DeleteOptions{})
						g.Expect(err).NotTo(gomega.HaveOccurred(), "delete pod")
					}

					claims, err := f.ClientSet.CoreV1().ResourceClaims(f.Namespace.Name).List(ctx, metav1.ListOptions{})
					for _, claim := range claims.Items {
						if claim.DeletionTimestamp != nil {
							continue
						}
						err := f.ClientSet.CoreV1().ResourceClaims(f.Namespace.Name).Delete(ctx, claim.Name, metav1.DeleteOptions{})
						g.Expect(err).NotTo(gomega.HaveOccurred(), "delete claim")
					}

					g.Expect(claims.Items).To(gomega.BeEmpty(), "all resource claims removed")
				}).WithTimeout(5*time.Minute).WithPolling(time.Second).Should(gomega.Succeed(), "clean up")
			})

			ginkgo.Describe("kubelet", func() {
				ginkgo.It("registers plugin", func() {
					// If we got here, the driver is running.
				})

				ginkgo.It("supports simple pod", func() {
					parameters := b.resourceClaimParameters()
					pod := b.podInline()
					b.create(ctx, parameters, pod)

					err := e2epod.WaitForPodNameRunningInNamespace(f.ClientSet, pod.Name, pod.Namespace)
					framework.ExpectNoError(err, "start pod with inline resource claim")

					log, err := e2epod.GetPodLogs(f.ClientSet, pod.Namespace, pod.Name, pod.Spec.Containers[0].Name)
					var envStr string
					for key, value := range b.resourceClaimParametersEnv() {
						envStr = fmt.Sprintf("\nuser_%s=%s\n", key, value)
						break
					}
					gomega.Expect(log).To(gomega.ContainSubstring(envStr), "container env variables")
				})
			})
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
		DriverName: b.driver.Name,
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
	pod.ObjectMeta.GenerateName = ""
	b.podCounter++
	pod.ObjectMeta.Name = fmt.Sprintf("tester-%d", b.podCounter)
	return pod
}

// makePodInline adds an inline resource claim with default class name and parameters.
func (b *builder) podInline() *corev1.Pod {
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
					},
				},
			},
		},
	}
	return pod
}

// create takes a bunch of objects and calls their Create function.
func (b *builder) create(ctx context.Context, objs ...interface{}) {
	for _, obj := range objs {
		var err error
		switch obj := obj.(type) {
		case *corev1.ResourceClass:
			_, err = b.f.ClientSet.CoreV1().ResourceClasses().Create(ctx, obj, metav1.CreateOptions{})
		case *corev1.Pod:
			_, err = b.f.ClientSet.CoreV1().Pods(b.f.Namespace.Name).Create(ctx, obj, metav1.CreateOptions{})
		case *corev1.ConfigMap:
			_, err = b.f.ClientSet.CoreV1().ConfigMaps(b.f.Namespace.Name).Create(ctx, obj, metav1.CreateOptions{})
		default:
			framework.Fail(fmt.Sprintf("internal error, unsupported type %T", obj), 1)
		}
		framework.ExpectNoErrorWithOffset(1, err, "create %T", obj)
	}
}
