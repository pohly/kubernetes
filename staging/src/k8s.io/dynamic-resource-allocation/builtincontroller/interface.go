/*
Copyright 2023 The Kubernetes Authors.

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

package builtincontroller

import (
	"context"

	v1 "k8s.io/api/core/v1"
	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	dracache "k8s.io/dynamic-resource-allocation/cache"
)

const (
	// Finalizer is the finalizer that gets set for claims
	// which were allocated through a builtin controller.
	Finalizer = "numeric.dra.k8s.io/delete-protection"
)

// NamedController is included in all interfaces. It provides
// a name for the controller for logging.
type NamedController interface {
	// ControllerName returns a name that ideally should be unique among
	// all active controllers. Only used for logging, so this is not a hard
	// requirement.
	ControllerName() string
}

// Controller is used to register a controller during program initialization.
type Controller interface {
	NamedController

	// Activate will get called during program startup to prepare the controller for usage.
	// The caller will start and sync the informer factory before using the controller.
	Activate(ctx context.Context, client kubernetes.Interface, informerFactory informers.SharedInformerFactory, genericInformerFactory *dracache.GenericListerFactory) (ActiveController, error)
}

// ActiveController is a controller which is ready for usage. It encapsulates
// a certain cluster state and can make changes to it. Events which change
// that state include:
//   - resource events (claim added, updated, deleted)
//   - allocation and deallocation
//
// Resource events are received in additional goroutines, so the implementation of the
// ActiveController must be thread-safe.
type ActiveController interface {
	NamedController

	// ClaimAllocated gets called for claims which were oberved to be allocated
	// in a resource event handler (i.e. new or updated claims). The controller
	// must update is resource tracking accordingly if (and only if) the
	// claim was not known to be already allocated and the controller is the
	// one which is handling the claim.
	ClaimAllocated(ctx context.Context, claim *resourcev1alpha2.ResourceClaim)

	// ClaimDeallocated is the inverse of ClaimAllocated. Besides being
	// called more than once per claim, it might also get called when the
	// caller is not entirely sure about the claim, for example in case of
	// a delete where the last state is unknown. Therefore the controller
	// has to check whether it knows about resources allocated for the
	// claim. It cannot rely on the allocation result in the claim for
	// that.
	ClaimDeallocated(ctx context.Context, claim *resourcev1alpha2.ResourceClaim)

	// Snapshot tells the controller to capture the current cluster
	// state in a new ActiveController instance. Cluster
	// events in the parent instance must not affect the child
	// instance. The child instance will not receive cluster events.
	Snapshot() ActiveController

	// HandlesClaim returns a non-nil claim controller if the claim, its
	// class, its parameters and/or status indicate that the controller is
	// able to handle claim allocation (if the claim is not allocated) or
	// deallocation (otherwise).
	//
	// It returns an error if the claim cannot be handled by any
	// controller, for example because it is not allocated and its resource
	// class is missing.
	HandlesClaim(ctx context.Context, claim *resourcev1alpha2.ResourceClaim, class *resourcev1alpha2.ResourceClass) (ClaimController, error)
}

// ClaimController is a controller which got instantiated for a particular claim.
type ClaimController interface {
	NamedController

	// NodeIsSuitable checks whether a claim could be allocated for
	// a pod such that it will be available on the node.
	NodeIsSuitable(ctx context.Context, pod *v1.Pod, node *v1.Node) (bool, error)

	// Allocate must adapt the cluster state as if the claim
	// had been allocated for use on the selected node and return
	// the result for the claim. It must not modify the claim,
	// that will be done by the caller.
	//
	// The tracking of available resources in the parent ActiveController
	// must reflect that the claim is now allocated. The cluster
	// event for "claim updated with allocation" may be received there
	// later.
	//
	// Deallocate might get called after Allocate if the caller needs
	// to roll back.
	Allocate(ctx context.Context, nodeName string) (driverName string, allocation *resourcev1alpha2.AllocationResult, err error)

	// Deallocate must adapt the cluster state as if the claim
	// had been deallocated. It must not modify the claim,
	// that will be done by the caller.
	//
	// As with Allocated, tracking of resources must be updated and
	// be prepared for a "claim updated without allocation" event.
	//
	// This must not fail because it might be called in the error
	// path after Allocation where handling another error isn't possible.
	Deallocate(ctx context.Context)
}
