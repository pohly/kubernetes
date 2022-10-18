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
	"sync"
	"time"

	"github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/resourceclaim"
	"k8s.io/klog/v2"
	dra "k8s.io/kubernetes/pkg/kubelet/cm/dra/plugin"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// ActivePodsFunc is a function that returns a list of pods to reconcile.
type ActivePodsFunc func() []*v1.Pod

// ManagerImpl is the structure in charge of managing DRA resource Plugins.
type ManagerImpl struct {
	sync.Mutex

	// resources contains resources referenced by pod containers
	resources *claimedResources

	// KubeClient reference
	kubeClient clientset.Interface

	// activePods is a method for listing active pods on the node
	// so the NodeUnprepareResource API can be properly called
	// for the forcibly deleted pods (with a grace period 0)
	activePods ActivePodsFunc

	// reconcilePeriod is the duration between calls to reconcileState.
	reconcilePeriod time.Duration
}

// NewManagerImpl creates a new manager.
func NewManagerImpl(kubeClient clientset.Interface, reconcilePeriod time.Duration) (*ManagerImpl, error) {
	klog.V(2).InfoS("Creating DRA manager")

	manager := &ManagerImpl{
		resources:       newClaimedResources(),
		kubeClient:      kubeClient,
		reconcilePeriod: reconcilePeriod,
	}

	// The activePods field is populated with real implementations in manager.Init()
	// Before that, initializes it to perform no-op operation.
	manager.activePods = func() []*v1.Pod { return []*v1.Pod{} }

	return manager, nil
}

// Start populates activePods field with a real implementation
// and starts reconciler function
func (m *ManagerImpl) Start(activePods ActivePodsFunc) error {
	m.activePods = activePods

	// Periodically call m.reconcileState() to continue to keep the DRA
	// resource cache up to date.
	go wait.Until(func() { m.reconcileState() }, m.reconcilePeriod, wait.NeverStop)

	return nil
}

// reconcileState finds inactive pods that are still in the DRA Manager cache
// and un-prepares resources for them
func (m *ManagerImpl) reconcileState() {
	activePods := m.activePods()
	inactivePodUIDs := m.resources.allPodUIDs()
	for _, pod := range activePods {
		inactivePodUIDs.Delete(string(pod.UID))
	}
	if len(inactivePodUIDs) <= 0 {
		return
	}

	klog.V(3).InfoS("Inactive pod UIDs", "podUIDs", inactivePodUIDs)

	for _, res := range m.resources.resources {
		for _, podUID := range inactivePodUIDs.List() {
			if res.hasPodUID(podUID) {
				klog.V(3).InfoS("Calling UnprepareResource", "pod UID", podUID, "claim name", res.claimName)
				if err := m.unprepareResource(res, podUID); err != nil {
					klog.V(3).InfoS("UnprepareResource failed", "pod UID", podUID, "claim name", res.claimName, "error", err)
					continue
				}
			}
		}
	}
}

// Generate container annotations using CDI UpdateAnnotations API
func generateCDIAnnotation(claimUID types.UID, driverName string, cdiDevices []string) ([]kubecontainer.Annotation, error) {
	const maxKeyLen = 63 // max length of the CDI annotation key
	deviceID := string(claimUID)
	if len(deviceID) > maxKeyLen-len(driverName)-1 {
		deviceID = deviceID[:maxKeyLen-len(driverName)-1]
	}
	annotations, err := cdi.UpdateAnnotations(map[string]string{}, driverName, deviceID, cdiDevices)
	if err != nil {
		return nil, err
	}
	kubeAnnotations := []kubecontainer.Annotation{}
	for key, value := range annotations {
		kubeAnnotations = append(kubeAnnotations, kubecontainer.Annotation{Name: key, Value: value})
	}
	return kubeAnnotations, nil
}

// prepareContainerResources attempts to prepare all of required resource
// plugin resources for the input container, issues an NodePrepareResource rpc request
// for each new resource requirement, processes their responses and updates the cached
// containerResources on success.
func (m *ManagerImpl) prepareContainerResources(pod *v1.Pod, container *v1.Container) error {
	// Process resources for each resource claim referenced by container
	for range container.Resources.Claims {
		for _, podResourceClaim := range pod.Spec.ResourceClaims {
			claimName := resourceclaim.Name(pod, &podResourceClaim)
			klog.V(3).InfoS("Processing resource", "claim", claimName, "pod", pod.Name)

			if resource := m.resources.get(claimName, pod.Namespace); resource != nil {
				// resource is already prepared, add pod UID to it
				resource.addPodUID(string(pod.UID))
				continue
			}

			// Query claim object from the API server
			resourceClaim, err := m.kubeClient.CoreV1().ResourceClaims(pod.Namespace).Get(context.TODO(), claimName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to fetch ResourceClaim %s referenced by pod %s: %+v", claimName, pod.Name, err)
			}

			// Check if pod is in the ReservedFor for the claim
			if !resourceclaim.IsReservedForPod(pod, resourceClaim) {
				return fmt.Errorf("pod %s(%s) is not allowed to use resource claim %s(%s)", pod.Name, pod.UID, podResourceClaim.Name, resourceClaim.UID)
			}

			// Call NodePrepareResource RPC
			driverName := resourceClaim.Status.DriverName
			client, err := dra.NewDRAPluginClient(driverName)
			if err != nil || client == nil {
				return fmt.Errorf("failed to get DRA Plugin client for plugin name %s, err=%+v", driverName, err)
			}

			response, err := client.NodePrepareResource(context.Background(), resourceClaim.Namespace, resourceClaim.UID, resourceClaim.Name, resourceClaim.Status.Allocation.ResourceHandle)
			if err != nil {
				return fmt.Errorf("NodePrepareResource failed, claim UID: %s, claim name: %s, resource handle: %s, err: %+v", resourceClaim.UID, resourceClaim.Name, resourceClaim.Status.Allocation.ResourceHandle, err)
			}
			klog.V(3).InfoS("NodePrepareResource succeeded", "response", response)

			annotations, err := generateCDIAnnotation(resourceClaim.UID, driverName, response.CdiDevice)
			if err != nil {
				return fmt.Errorf("failed to generate container annotations, err: %+v", err)
			}

			// Cache prepared resource
			err = m.resources.add(
				resourceClaim.Name,
				resourceClaim.Namespace,
				&resource{
					driverName:  driverName,
					claimUID:    resourceClaim.UID,
					claimName:   resourceClaim.Name,
					namespace:   resourceClaim.Namespace,
					podUIDs:     sets.NewString(string(pod.UID)),
					cdiDevice:   response.CdiDevice,
					annotations: annotations,
				})
			if err != nil {
				return fmt.Errorf("failed to cache prepared resource, claim: %s(%s), err: %+v", resourceClaim.Name, resourceClaim.UID, err)
			}
		}
	}

	return nil
}

// PrepareResources calls plugin NodePrepareResource from the registered DRA resource plugins.
func (m *ManagerImpl) PrepareResources(pod *v1.Pod, container *v1.Container) (*DRAContainerInfo, error) {
	if err := m.prepareContainerResources(pod, container); err != nil {
		return nil, err
	}
	annotations := []kubecontainer.Annotation{}
	for _, podResourceClaim := range pod.Spec.ResourceClaims {
		claimName := resourceclaim.Name(pod, &podResourceClaim)
		for _, claim := range container.Resources.Claims {
			if podResourceClaim.Name != claim {
				continue
			}
			resource := m.resources.get(claimName, pod.Namespace)
			if resource == nil {
				return nil, fmt.Errorf(fmt.Sprintf("unable to get resource for namespace: %s, claim: %s", pod.Namespace, claimName))
			}
			klog.V(3).InfoS("add resource annotations", "claim", claimName, "annotations", resource.annotations)
			annotations = append(annotations, resource.annotations...)
		}
	}
	return &DRAContainerInfo{Annotations: annotations}, nil
}

func (m *ManagerImpl) UnprepareResources(pod *v1.Pod) error {
	// Call NodeUnprepareResource RPC for every resource claim referenced by the pod
	for _, podResourceClaim := range pod.Spec.ResourceClaims {
		claimName := resourceclaim.Name(pod, &podResourceClaim)
		res := m.resources.get(claimName, pod.Namespace)
		if res == nil {
			return fmt.Errorf("failed to get resource for namespace %s, claim %s", pod.Namespace, claimName)
		}

		if !res.hasPodUID(string(pod.UID)) {
			// skip calling NodeUnprepareResource if pod is not cached
			continue
		}

		if err := m.unprepareResource(res, string(pod.UID)); err != nil {
			return err
		}
	}

	return nil
}

func (m *ManagerImpl) unprepareResource(res *resource, podUID string) error {
	// Delete pod UID from the cache
	res.deletePodUID(podUID)

	if len(res.podUIDs) <= 0 {
		// Call NodeUnprepareResource only for the latest pod that refers the claim
		client, err := dra.NewDRAPluginClient(res.driverName)
		if err != nil || client == nil {
			return fmt.Errorf("failed to get DRA Plugin client for plugin name %s, err=%+v", res.driverName, err)
		}
		response, err := client.NodeUnprepareResource(context.Background(), res.namespace, res.claimUID, res.claimName, res.cdiDevice)
		if err != nil {
			return fmt.Errorf("NodeUnprepareResource failed, podUID: %s, claim UID: %s, claim name: %s, CDI devices: %s, err: %+v", podUID, res.claimUID, res.claimName, res.cdiDevice, err)
		}
		klog.V(3).InfoS("NodeUnprepareResource succeeded", "response", response)

		// delete resource from the cache
		m.Lock()
		m.resources.delete(res.claimName, res.namespace)
		m.Unlock()
	}
	return nil
}
