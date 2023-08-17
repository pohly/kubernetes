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

package storage

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/apis/resource"
	"k8s.io/kubernetes/pkg/apis/resource/v1alpha2"
	"k8s.io/kubernetes/pkg/printers"
	printersinternal "k8s.io/kubernetes/pkg/printers/internalversion"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"
	"k8s.io/kubernetes/pkg/registry/resource/podschedulingcontext"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

// REST implements a RESTStorage for PodSchedulingContext.
type REST struct {
	*genericregistry.Store
}

func (r *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return r.Store.Update(ctx, name, objInfo, createValidation, updateValidation, true, options)
}

func (r *REST) SkipManagedFields() bool {
	return true
}

func (r *REST) ApplyPatchToCurrentObject(requestContext context.Context, currentObject runtime.Object, patch []byte, patchType types.PatchType) (runtime.Object, error) {
	// Below we need to decode. Only input for server-side-apply is
	// supported because it clearly indicates what the client wants to
	// update.
	if patchType != types.ApplyPatchType {
		return nil, rest.PatchNotHandledErr
	}

	// There's only one kind of object in the storage, check anyway.
	podScheduling, ok := currentObject.(*resource.PodSchedulingContext)
	if !ok {
		return nil, rest.PatchNotHandledErr
	}

	// Decode directly into a type which has exactly those
	// fields which the code below knows to handle. If any
	// other field is present, we fall back to normal apply
	// handling.
	patchObj := struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata"`
		Spec              struct {
			SelectedNode   string      `json:"selectedNode"`
			PotentialNodes sets.String `json:"potentialNodes"`
		} `json:"spec"`
	}{}
	if err := yaml.UnmarshalStrict(patch, &patchObj); err != nil {
		return nil, rest.PatchNotHandledErr
	}

	// k8s.io/apimachinery/pkg/util/managedfields/internal/versioncheck.go:51
	if patchObj.Kind != "PodSchedulingContext" ||
		patchObj.APIVersion != v1alpha2.SchemeGroupVersion.String() {
		return nil, rest.PatchNotHandledErr
	}
	// ObjectMeta was validated earlier, otherwise we wouldn't have a matching
	// current object.

	// Update values.
	podScheduling.Spec.SelectedNode = patchObj.Spec.SelectedNode
	podScheduling.Spec.PotentialNodes = patchObj.Spec.PotentialNodes
	return podScheduling, nil
}

var _ rest.ManagedFieldsSkipper = &REST{}
var _ rest.PatchHandler = &REST{}

// NewREST returns a RESTStorage object that will work against PodSchedulingContext.
func NewREST(optsGetter generic.RESTOptionsGetter) (*REST, *StatusREST, error) {
	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return &resource.PodSchedulingContext{} },
		NewListFunc:               func() runtime.Object { return &resource.PodSchedulingContextList{} },
		PredicateFunc:             podschedulingcontext.Match,
		DefaultQualifiedResource:  resource.Resource("podschedulingcontexts"),
		SingularQualifiedResource: resource.Resource("podschedulingcontext"),

		CreateStrategy:      podschedulingcontext.Strategy,
		UpdateStrategy:      podschedulingcontext.Strategy,
		DeleteStrategy:      podschedulingcontext.Strategy,
		ReturnDeletedObject: true,
		ResetFieldsStrategy: podschedulingcontext.Strategy,

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(printersinternal.AddHandlers)},
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: podschedulingcontext.GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, nil, err
	}

	statusStore := *store
	statusStore.UpdateStrategy = podschedulingcontext.StatusStrategy
	statusStore.ResetFieldsStrategy = podschedulingcontext.StatusStrategy

	rest := &REST{store}

	return rest, &StatusREST{store: &statusStore}, nil
}

// StatusREST implements the REST endpoint for changing the status of a PodSchedulingContext.
type StatusREST struct {
	store *genericregistry.Store
}

// New creates a new PodSchedulingContext object.
func (r *StatusREST) New() runtime.Object {
	return &resource.PodSchedulingContext{}
}

func (r *StatusREST) Destroy() {
	// Given that underlying store is shared with REST,
	// we don't destroy it here explicitly.
}

// Get retrieves the object from the storage. It is required to support Patch.
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

// Update alters the status subset of an object.
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// We are explicitly setting forceAllowCreate to false in the call to the underlying storage because
	// subresources should never allow create on update.
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}

// GetResetFields implements rest.ResetFieldsStrategy
func (r *StatusREST) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return r.store.GetResetFields()
}

func (r *StatusREST) SkipManagedFields() bool {
	return true
}

func (r *StatusREST) ApplyPatchToCurrentObject(requestContext context.Context, currentObject runtime.Object, patch []byte, patchType types.PatchType) (runtime.Object, error) {
	// Below we need to decode. Only input for server-side-apply is
	// supported because it clearly indicates what the client wants to
	// update.
	if patchType != types.ApplyPatchType {
		return nil, rest.PatchNotHandledErr
	}

	// There's only one kind of object in the storage, check anyway.
	podScheduling, ok := currentObject.(*resource.PodSchedulingContext)
	if !ok {
		return nil, rest.PatchNotHandledErr
	}

	// Decode directly into a type which has exactly those
	// fields which the code below knows to handle. If any
	// other field is present, we fall back to normal apply
	// handling.
	patchObj := struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata"`
		Status            struct {
			ResourceClaims []struct {
				Name            string      `json:"name"`
				UnsuitableNodes sets.String `json:"unsuitableNodes"`
			} `json:"resourceClaims"`
		} `json:"status"`
	}{}
	if err := yaml.UnmarshalStrict(patch, &patchObj); err != nil {
		return nil, rest.PatchNotHandledErr
	}

	if patchObj.Kind != "PodSchedulingContext" ||
		patchObj.APIVersion != v1alpha2.SchemeGroupVersion.String() {
		return nil, rest.PatchNotHandledErr
	}
	// ObjectMeta was validated earlier, otherwise we wouldn't have a matching
	// current object.

	// Update values.
	for _, claimStatus := range patchObj.Status.ResourceClaims {
		if existingClaimStatus := findClaimStatus(podScheduling.Status.ResourceClaims, claimStatus.Name); existingClaimStatus != nil {
			existingClaimStatus.UnsuitableNodes = claimStatus.UnsuitableNodes
			continue
		}
		podScheduling.Status.ResourceClaims = append(podScheduling.Status.ResourceClaims,
			resource.ResourceClaimSchedulingStatus{
				Name:            claimStatus.Name,
				UnsuitableNodes: claimStatus.UnsuitableNodes,
			})
	}
	return podScheduling, nil
}

func findClaimStatus(claimStatuses []resource.ResourceClaimSchedulingStatus, name string) *resource.ResourceClaimSchedulingStatus {
	for i := range claimStatuses {
		if claimStatuses[i].Name == name {
			return &claimStatuses[i]
		}
	}
	return nil
}

var _ rest.ManagedFieldsSkipper = &StatusREST{}
var _ rest.PatchHandler = &StatusREST{}
