/*
Copyright 2018 The Kubernetes Authors.

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

package testsuites

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	"k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

// StorageClassTest represents parameters to be used by provisioning tests.
// Not all parameters are used by all tests.
type StorageClassTest struct {
	Name             string
	CloudProviders   []string
	Provisioner      string
	StorageClassName string
	Parameters       map[string]string
	DelayBinding     bool
	ClaimSize        string
	ExpectedSize     string
	PvCheck          func(claim *v1.PersistentVolumeClaim, volume *v1.PersistentVolume)
	VolumeMode       *v1.PersistentVolumeMode
}

type provisioningTestSuite struct {
	tsInfo TestSuiteInfo
}

var _ TestSuite = &provisioningTestSuite{}

// InitProvisioningTestSuite returns provisioningTestSuite that implements TestSuite interface
func InitProvisioningTestSuite() TestSuite {
	return &provisioningTestSuite{
		tsInfo: TestSuiteInfo{
			name: "provisioning",
			testPatterns: []testpatterns.TestPattern{
				testpatterns.DefaultFsDynamicPV,
			},
		},
	}
}

func (p *provisioningTestSuite) getTestSuiteInfo() TestSuiteInfo {
	return p.tsInfo
}

func (p *provisioningTestSuite) skipUnsupportedTest(pattern testpatterns.TestPattern, driver TestDriver) {
}

func createProvisioningTestInput(driver TestDriver, pattern testpatterns.TestPattern) (provisioningTestResource, provisioningTestInput) {
	// Setup test resource for driver and testpattern
	resource := provisioningTestResource{}
	resource.setupResource(driver, pattern)

	input := provisioningTestInput{
		testCase: StorageClassTest{
			ClaimSize:    resource.claimSize,
			ExpectedSize: resource.claimSize,
		},
		cs:       driver.GetDriverInfo().Config.Framework.ClientSet,
		pvc:      resource.pvc,
		sc:       resource.sc,
		dInfo:    driver.GetDriverInfo(),
		nodeName: driver.GetDriverInfo().Config.ClientNodeName,
	}

	return resource, input
}

func (p *provisioningTestSuite) execTest(driver TestDriver, pattern testpatterns.TestPattern) {
	Context(getTestNameStr(p, pattern), func() {
		var (
			resource     provisioningTestResource
			input        provisioningTestInput
			needsCleanup bool
		)

		BeforeEach(func() {
			needsCleanup = false
			// Skip unsupported tests to avoid unnecessary resource initialization
			skipUnsupportedTest(p, driver, pattern)
			needsCleanup = true

			// Create test input
			resource, input = createProvisioningTestInput(driver, pattern)
		})

		AfterEach(func() {
			if needsCleanup {
				resource.cleanupResource(driver, pattern)
			}
		})

		// Ginkgo's "Global Shared Behaviors" require arguments for a shared function
		// to be a single struct and to be passed as a pointer.
		// Please see https://onsi.github.io/ginkgo/#global-shared-behaviors for details.
		testProvisioning(&input)
	})
}

type provisioningTestResource struct {
	driver TestDriver

	claimSize string
	sc        *storage.StorageClass
	pvc       *v1.PersistentVolumeClaim
}

var _ TestResource = &provisioningTestResource{}

func (p *provisioningTestResource) setupResource(driver TestDriver, pattern testpatterns.TestPattern) {
	// Setup provisioningTest resource
	switch pattern.VolType {
	case testpatterns.DynamicPV:
		if dDriver, ok := driver.(DynamicPVTestDriver); ok {
			p.sc = dDriver.GetDynamicProvisionStorageClass("")
			if p.sc == nil {
				framework.Skipf("Driver %q does not define Dynamic Provision StorageClass - skipping", driver.GetDriverInfo().Name)
			}
			p.driver = driver
			p.claimSize = dDriver.GetClaimSize()
			p.pvc = getClaim(p.claimSize, driver.GetDriverInfo().Config.Framework.Namespace.Name)
			p.pvc.Spec.StorageClassName = &p.sc.Name
			framework.Logf("In creating storage class object and pvc object for driver - sc: %v, pvc: %v", p.sc, p.pvc)
		}
	default:
		framework.Failf("Dynamic Provision test doesn't support: %s", pattern.VolType)
	}
}

func (p *provisioningTestResource) cleanupResource(driver TestDriver, pattern testpatterns.TestPattern) {
}

type provisioningTestInput struct {
	testCase StorageClassTest
	cs       clientset.Interface
	pvc      *v1.PersistentVolumeClaim
	sc       *storage.StorageClass
	dInfo    *DriverInfo
	nodeName string
}

func testProvisioning(input *provisioningTestInput) {
	// common checker for most of the test cases below
	pvcheck := func(claim *v1.PersistentVolumeClaim, volume *v1.PersistentVolume) {
		PVWriteReadCheck(input.cs, claim, volume, NodeSelection{Name: input.nodeName})
	}

	It("should provision storage with defaults", func() {
		input.testCase.PvCheck = pvcheck
		TestDynamicProvisioning(input.testCase, input.cs, input.pvc, input.sc)
	})

	It("should provision storage with mount options", func() {
		if input.dInfo.SupportedMountOption == nil {
			framework.Skipf("Driver %q does not define supported mount option - skipping", input.dInfo.Name)
		}

		input.sc.MountOptions = input.dInfo.SupportedMountOption.Union(input.dInfo.RequiredMountOption).List()
		input.testCase.PvCheck = pvcheck
		TestDynamicProvisioning(input.testCase, input.cs, input.pvc, input.sc)
	})

	It("should create and delete block persistent volumes", func() {
		if !input.dInfo.Capabilities[CapBlock] {
			framework.Skipf("Driver %q does not support BlockVolume - skipping", input.dInfo.Name)
		}
		block := v1.PersistentVolumeBlock
		input.testCase.VolumeMode = &block
		input.pvc.Spec.VolumeMode = &block
		TestDynamicProvisioning(input.testCase, input.cs, input.pvc, input.sc)
	})
}

// TestDynamicProvisioning tests dynamic provisioning with specified StorageClassTest and storageClass
func TestDynamicProvisioning(t StorageClassTest, client clientset.Interface, claim *v1.PersistentVolumeClaim, class *storage.StorageClass) *v1.PersistentVolume {
	var err error
	if class != nil {
		By("creating a StorageClass " + class.Name)
		class, err = client.StorageV1().StorageClasses().Create(class)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			framework.Logf("deleting storage class %s", class.Name)
			framework.ExpectNoError(client.StorageV1().StorageClasses().Delete(class.Name, nil))
		}()
	}

	By("creating a claim")
	claim, err = client.CoreV1().PersistentVolumeClaims(claim.Namespace).Create(claim)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		framework.Logf("deleting claim %q/%q", claim.Namespace, claim.Name)
		// typically this claim has already been deleted
		err = client.CoreV1().PersistentVolumeClaims(claim.Namespace).Delete(claim.Name, nil)
		if err != nil && !apierrs.IsNotFound(err) {
			framework.Failf("Error deleting claim %q. Error: %v", claim.Name, err)
		}
	}()
	err = framework.WaitForPersistentVolumeClaimPhase(v1.ClaimBound, client, claim.Namespace, claim.Name, framework.Poll, framework.ClaimProvisionTimeout)
	Expect(err).NotTo(HaveOccurred())

	By("checking the claim")
	// Get new copy of the claim
	claim, err = client.CoreV1().PersistentVolumeClaims(claim.Namespace).Get(claim.Name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Get the bound PV
	pv, err := client.CoreV1().PersistentVolumes().Get(claim.Spec.VolumeName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Check sizes
	expectedCapacity := resource.MustParse(t.ExpectedSize)
	pvCapacity := pv.Spec.Capacity[v1.ResourceName(v1.ResourceStorage)]
	Expect(pvCapacity.Value()).To(Equal(expectedCapacity.Value()), "pvCapacity is not equal to expectedCapacity")

	requestedCapacity := resource.MustParse(t.ClaimSize)
	claimCapacity := claim.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	Expect(claimCapacity.Value()).To(Equal(requestedCapacity.Value()), "claimCapacity is not equal to requestedCapacity")

	// Check PV properties
	By("checking the PV")
	expectedAccessModes := []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}
	Expect(pv.Spec.AccessModes).To(Equal(expectedAccessModes))
	Expect(pv.Spec.ClaimRef.Name).To(Equal(claim.ObjectMeta.Name))
	Expect(pv.Spec.ClaimRef.Namespace).To(Equal(claim.ObjectMeta.Namespace))
	if class == nil {
		Expect(pv.Spec.PersistentVolumeReclaimPolicy).To(Equal(v1.PersistentVolumeReclaimDelete))
	} else {
		Expect(pv.Spec.PersistentVolumeReclaimPolicy).To(Equal(*class.ReclaimPolicy))
		Expect(pv.Spec.MountOptions).To(Equal(class.MountOptions))
	}
	if t.VolumeMode != nil {
		Expect(pv.Spec.VolumeMode).NotTo(BeNil())
		Expect(*pv.Spec.VolumeMode).To(Equal(*t.VolumeMode))
	}

	// Run the checker
	if t.PvCheck != nil {
		t.PvCheck(claim, pv)
	}

	By(fmt.Sprintf("deleting claim %q/%q", claim.Namespace, claim.Name))
	framework.ExpectNoError(client.CoreV1().PersistentVolumeClaims(claim.Namespace).Delete(claim.Name, nil))

	// Wait for the PV to get deleted if reclaim policy is Delete. (If it's
	// Retain, there's no use waiting because the PV won't be auto-deleted and
	// it's expected for the caller to do it.) Technically, the first few delete
	// attempts may fail, as the volume is still attached to a node because
	// kubelet is slowly cleaning up the previous pod, however it should succeed
	// in a couple of minutes. Wait 20 minutes to recover from random cloud
	// hiccups.
	if pv.Spec.PersistentVolumeReclaimPolicy == v1.PersistentVolumeReclaimDelete {
		By(fmt.Sprintf("deleting the claim's PV %q", pv.Name))
		framework.ExpectNoError(framework.WaitForPersistentVolumeDeleted(client, pv.Name, 5*time.Second, 20*time.Minute))
	}

	return pv
}

// PVWriteReadCheck checks that a PV retains data.
//
// It starts two pods:
// - The first writes 'hello word' to the /mnt/test (= the volume).
// - The second one runs grep 'hello world' on /mnt/test.
// If both succeed, Kubernetes actually allocated something that is
// persistent across pods.
//
// This is a common test that can be called from a StorageClassTest.PvCheck.
func PVWriteReadCheck(client clientset.Interface, claim *v1.PersistentVolumeClaim, volume *v1.PersistentVolume, node NodeSelection) {
	By(fmt.Sprintf("checking the created volume is writable and has the PV's mount options on node %+v", node))
	command := "echo 'hello world' > /mnt/test/data"
	// We give the first pod the secondary responsibility of checking the volume has
	// been mounted with the PV's mount options, if the PV was provisioned with any
	for _, option := range volume.Spec.MountOptions {
		// Get entry, get mount options at 6th word, replace brackets with commas
		command += fmt.Sprintf(" && ( mount | grep 'on /mnt/test' | awk '{print $6}' | sed 's/^(/,/; s/)$/,/' | grep -q ,%s, )", option)
	}
	command += " || (mount | grep 'on /mnt/test'; false)"
	RunInPodWithVolume(client, claim.Namespace, claim.Name, "pvc-volume-tester-writer", command, node)

	By(fmt.Sprintf("checking the created volume is readable and retains data on the same node %+v", node))
	command = "grep 'hello world' /mnt/test/data"
	RunInPodWithVolume(client, claim.Namespace, claim.Name, "pvc-volume-tester-reader", command, node)
}

func TestBindingWaitForFirstConsumer(t StorageClassTest, client clientset.Interface, claim *v1.PersistentVolumeClaim, class *storage.StorageClass, nodeSelector map[string]string, expectUnschedulable bool) (*v1.PersistentVolume, *v1.Node) {
	pvs, node := TestBindingWaitForFirstConsumerMultiPVC(t, client, []*v1.PersistentVolumeClaim{claim}, class, nodeSelector, expectUnschedulable)
	if pvs == nil {
		return nil, node
	}
	return pvs[0], node
}

func TestBindingWaitForFirstConsumerMultiPVC(t StorageClassTest, client clientset.Interface, claims []*v1.PersistentVolumeClaim, class *storage.StorageClass, nodeSelector map[string]string, expectUnschedulable bool) ([]*v1.PersistentVolume, *v1.Node) {
	var err error
	Expect(len(claims)).ToNot(Equal(0))
	namespace := claims[0].Namespace

	By("creating a storage class " + class.Name)
	class, err = client.StorageV1().StorageClasses().Create(class)
	Expect(err).NotTo(HaveOccurred())
	defer deleteStorageClass(client, class.Name)

	By("creating claims")
	var claimNames []string
	var createdClaims []*v1.PersistentVolumeClaim
	for _, claim := range claims {
		c, err := client.CoreV1().PersistentVolumeClaims(claim.Namespace).Create(claim)
		claimNames = append(claimNames, c.Name)
		createdClaims = append(createdClaims, c)
		Expect(err).NotTo(HaveOccurred())
	}
	defer func() {
		var errors map[string]error
		for _, claim := range createdClaims {
			err := framework.DeletePersistentVolumeClaim(client, claim.Name, claim.Namespace)
			if err != nil {
				errors[claim.Name] = err
			}
		}
		if len(errors) > 0 {
			for claimName, err := range errors {
				framework.Logf("Failed to delete PVC: %s due to error: %v", claimName, err)
			}
		}
	}()

	// Wait for ClaimProvisionTimeout (across all PVCs in parallel) and make sure the phase did not become Bound i.e. the Wait errors out
	By("checking the claims are in pending state")
	err = framework.WaitForPersistentVolumeClaimsPhase(v1.ClaimBound, client, namespace, claimNames, 2*time.Second /* Poll */, framework.ClaimProvisionShortTimeout, true)
	Expect(err).To(HaveOccurred())
	verifyPVCsPending(client, createdClaims)

	By("creating a pod referring to the claims")
	// Create a pod referring to the claim and wait for it to get to running
	var pod *v1.Pod
	if expectUnschedulable {
		pod, err = framework.CreateUnschedulablePod(client, namespace, nodeSelector, createdClaims, true /* isPrivileged */, "" /* command */)
	} else {
		pod, err = framework.CreatePod(client, namespace, nil /* nodeSelector */, createdClaims, true /* isPrivileged */, "" /* command */)
	}
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		framework.DeletePodOrFail(client, pod.Namespace, pod.Name)
		framework.WaitForPodToDisappear(client, pod.Namespace, pod.Name, labels.Everything(), framework.Poll, framework.PodDeleteTimeout)
	}()
	if expectUnschedulable {
		// Verify that no claims are provisioned.
		verifyPVCsPending(client, createdClaims)
		return nil, nil
	}

	// collect node details
	node, err := client.CoreV1().Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("re-checking the claims to see they binded")
	var pvs []*v1.PersistentVolume
	for _, claim := range createdClaims {
		// Get new copy of the claim
		claim, err = client.CoreV1().PersistentVolumeClaims(claim.Namespace).Get(claim.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		// make sure claim did bind
		err = framework.WaitForPersistentVolumeClaimPhase(v1.ClaimBound, client, claim.Namespace, claim.Name, framework.Poll, framework.ClaimProvisionTimeout)
		Expect(err).NotTo(HaveOccurred())

		pv, err := client.CoreV1().PersistentVolumes().Get(claim.Spec.VolumeName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		pvs = append(pvs, pv)
	}
	Expect(len(pvs)).To(Equal(len(createdClaims)))
	return pvs, node
}

// NodeSelection specifies where to run a pod, using a combination of fixed node name,
// node selector and/or affinity.
type NodeSelection struct {
	Name     string
	Selector map[string]string
	Affinity *v1.Affinity
}

// RunInPodWithVolume runs a command in a pod with given claim mounted to /mnt directory.
// It starts, checks, collects output and stops it.
func RunInPodWithVolume(c clientset.Interface, ns, claimName, podName, command string, node NodeSelection) {
	pod := StartInPodWithVolume(c, ns, claimName, podName, command, node)
	defer StopPod(c, pod)
	framework.ExpectNoError(framework.WaitForPodSuccessInNamespaceSlow(c, pod.Name, pod.Namespace))
}

// StartInPodWithVolume starts a command in a pod with given claim mounted to /mnt directory
// The caller is responsible for checking the pod and deleting it.
func StartInPodWithVolume(c clientset.Interface, ns, claimName, podName, command string, node NodeSelection) *v1.Pod {
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: podName + "-",
		},
		Spec: v1.PodSpec{
			NodeName:     node.Name,
			NodeSelector: node.Selector,
			Affinity:     node.Affinity,
			Containers: []v1.Container{
				{
					Name:    "volume-tester",
					Image:   imageutils.GetE2EImage(imageutils.BusyBox),
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", command},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "my-volume",
							MountPath: "/mnt/test",
						},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
			Volumes: []v1.Volume{
				{
					Name: "my-volume",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: claimName,
							ReadOnly:  false,
						},
					},
				},
			},
		},
	}

	pod, err := c.CoreV1().Pods(ns).Create(pod)
	framework.ExpectNoError(err, "Failed to create pod: %v", err)
	return pod
}

// StopPod first tries to log the output of the pod's container, then deletes the pod.
func StopPod(c clientset.Interface, pod *v1.Pod) {
	if pod == nil {
		return
	}
	body, err := c.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{}).Do().Raw()
	if err != nil {
		framework.Logf("Error getting logs for pod %s: %v", pod.Name, err)
	} else {
		framework.Logf("Pod %s has the following logs: %s", pod.Name, body)
	}
	framework.DeletePodOrFail(c, pod.Namespace, pod.Name)
}

func verifyPVCsPending(client clientset.Interface, pvcs []*v1.PersistentVolumeClaim) {
	for _, claim := range pvcs {
		// Get new copy of the claim
		claim, err := client.CoreV1().PersistentVolumeClaims(claim.Namespace).Get(claim.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(claim.Status.Phase).To(Equal(v1.ClaimPending))
	}
}
