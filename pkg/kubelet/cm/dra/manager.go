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

	"github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/component-helpers/dra/resourceclaim"
	"k8s.io/klog/v2"
	dra "k8s.io/kubernetes/pkg/kubelet/cm/dra/plugin"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// ManagerImpl is the structure in charge of managing DRA resource Plugins.
type ManagerImpl struct {
	sync.Mutex

	// resources contains resources referenced by pod containers
	resources *claimedResources

	// KubeClient reference
	kubeClient clientset.Interface
}

// NewManagerImpl creates a new manager.
func NewManagerImpl(kubeClient clientset.Interface) (*ManagerImpl, error) {
	klog.V(2).InfoS("Creating DRA manager")

	manager := &ManagerImpl{
		resources:  newClaimedResources(),
		kubeClient: kubeClient,
	}

	return manager, nil
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
			klog.V(3).Infof("Processing resource claim %s, pod %s", claimName, pod.Name)

			if resource := m.resources.get(claimName, pod.Namespace); resource != nil {
				// resource is already prepared, add pod UID to it
				resource.addPodUID(pod.UID)
				continue
			}

			// Query claim object from the API server
			resourceClaim, err := m.kubeClient.CoreV1().ResourceClaims(pod.Namespace).Get(context.TODO(), claimName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to fetch ResourceClaim %s referenced by pod %s: %+v", claimName, pod.Name, err)
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
			klog.V(3).Infof("NodePrepareResource: response: %+v", response)

			annotations, err := generateCDIAnnotation(resourceClaim.UID, driverName, response.CdiDevice)
			if err != nil {
				return fmt.Errorf("failed to generate container annotations, err: %+v", err)
			}

			// Cache prepared resource
			m.resources.add(
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
			klog.V(3).Infof("claim %s: add resource annotations: %+v", resource.annotations)
			annotations = append(annotations, resource.annotations...)
		}
	}
	return &DRAContainerInfo{Annotations: annotations}, nil
}

func (m *ManagerImpl) UnprepareResources(pod *v1.Pod) error {
	m.Lock()
	defer m.Unlock()

	// Call NodeUnprepareResource RPC for every resource claim referenced by the pod
	for _, podResourceClaim := range pod.Spec.ResourceClaims {
		claimName := resourceclaim.Name(pod, &podResourceClaim)
		resource := m.resources.get(claimName, pod.Namespace)
		if resource == nil {
			return fmt.Errorf("failed to get resource for namespace %s, claim %s", pod.Namespace, claimName)
		}

		if !resource.hasPodUID(pod.UID) {
			// skip calling NodeUnprepareResource if pod is not cached
			continue
		}

		// Delete pod UID from the cache
		resource.deletePodUID(pod.UID)

		if len(resource.podUIDs) > 0 {
			// skip calling NodeUnprepareResource if this is not the latest pod
			// that uses the resource
			continue
		}

		// Call NodeUnprepareResource only for the latest pod that refers the claim
		client, err := dra.NewDRAPluginClient(resource.driverName)
		if err != nil || client == nil {
			return fmt.Errorf("failed to get DRA Plugin client for plugin name %s, err=%+v", resource.driverName, err)
		}
		response, err := client.NodeUnprepareResource(context.Background(), resource.namespace, resource.claimUID, resource.claimName, resource.cdiDevice)
		if err != nil {
			return fmt.Errorf("NodeUnprepareResource failed, pod: %s, claim UID: %s, claim name: %s, CDI devices: %s, err: %+v", pod.Name, resource.claimUID, resource.claimName, resource.cdiDevice, err)
		}
		klog.V(3).Infof("NodeUnprepareResource: response: %+v", response)
		// delete resource from the cache
		m.resources.delete(resource.claimName, pod.Namespace)
	}

	return nil
}
