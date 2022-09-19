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
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// resource contains resource attributes required
// to prepare and unprepare the resource
type resource struct {
	// name of the DRA driver
	driverName string

	// claimUID is an UID of the resource claim
	claimUID types.UID

	// claimName is a name of the resource claim
	claimName string

	// namespace is a claim namespace
	namespace string

	// podUIDs is a map of pod UIDs that reference a resource
	podUIDs map[types.UID]bool

	// cdiDevice is a list of CDI devices returned by the
	// GRPC API call NodePrepareResource
	cdiDevice []string

	// annotations is a list of container annotations associated with
	// a prepared resource
	annotations []kubecontainer.Annotation
}

// claimedResources is cache of processed resources keyed by namespace + claim name
type claimedResources struct {
	sync.RWMutex
	resources map[string]*resource
}

// newClaimedResources is a function that returns object of podResources
func newClaimedResources() *claimedResources {
	return &claimedResources{
		resources: make(map[string]*resource),
	}
}

func (cres *claimedResources) add(claim, namespace string, res *resource) error {
	cres.Lock()
	defer cres.Unlock()

	key := claim + namespace
	if _, ok := cres.resources[key]; ok {
		return fmt.Errorf("claim %s, namespace %s already cached", claim, namespace)
	}
	cres.resources[claim+namespace] = res
	return nil
}

func (cres *claimedResources) addPodUID(claimName, namespace string, podUID types.UID) error {
	cres.Lock()
	defer cres.Unlock()

	resource := cres.resources[claimName+namespace]
	if resource == nil {
		return fmt.Errorf("claim %s, namespace %s is not cached", claimName, namespace)
	}

	if _, ok := resource.podUIDs[podUID]; ok {
		return fmt.Errorf("pod uid %s is already cached for the claim %s, namespace %s", podUID, claimName, namespace)
	}

	resource.podUIDs[podUID] = true

	return nil
}

func (cres *claimedResources) get(claimName, namespace string) *resource {
	cres.RLock()
	defer cres.RUnlock()

	return cres.resources[claimName+namespace]
}

func (cres *claimedResources) delete(claimName, namespace string) {
	cres.Lock()
	defer cres.Unlock()

	delete(cres.resources, claimName+namespace)
}

func (cres *claimedResources) deletePodUIDs(podUIDs []types.UID) {
	cres.Lock()
	defer cres.Unlock()

	for _, resource := range cres.resources {
		for _, podUID := range podUIDs {
			delete(resource.podUIDs, podUID)
		}
	}
}
