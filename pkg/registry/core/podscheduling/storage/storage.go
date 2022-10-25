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
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/printers"
	printersinternal "k8s.io/kubernetes/pkg/printers/internalversion"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"
	"k8s.io/kubernetes/pkg/registry/core/podscheduling"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

// REST implements a RESTStorage for PodSchedulings.
type REST struct {
	*genericregistry.Store
}

// NewREST returns a RESTStorage object that will work against PodSchedulings.
func NewREST(optsGetter generic.RESTOptionsGetter) (*REST, *StatusREST, error) {
	store := &genericregistry.Store{
		NewFunc:                  func() runtime.Object { return &core.PodScheduling{} },
		NewListFunc:              func() runtime.Object { return &core.PodSchedulingList{} },
		PredicateFunc:            podscheduling.Match,
		DefaultQualifiedResource: core.Resource("podschedulings"),

		CreateStrategy:      podscheduling.Strategy,
		UpdateStrategy:      podscheduling.Strategy,
		DeleteStrategy:      podscheduling.Strategy,
		ReturnDeletedObject: true,
		ResetFieldsStrategy: podscheduling.Strategy,

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(printersinternal.AddHandlers)},
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: podscheduling.GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, nil, err
	}

	statusStore := *store
	statusStore.UpdateStrategy = podscheduling.StatusStrategy
	statusStore.ResetFieldsStrategy = podscheduling.StatusStrategy

	rest := &REST{store}
	store.Decorator = rest.defaultOnRead

	return rest, &StatusREST{store: &statusStore}, nil
}

// defaultOnRead sets interlinked fields that were not previously set on read.
// We can't do this in the normal defaulting path because that same logic
// applies on Get, Create, and Update, but we need to distinguish between them.
//
// This will be called on both PodScheduling and PodSchedulingList types.
func (r *REST) defaultOnRead(obj runtime.Object) {
	switch s := obj.(type) {
	case *core.PodScheduling:
		r.defaultOnReadClaim(s)
	case *core.PodSchedulingList:
		r.defaultOnReadClaimList(s)
	default:
		// This was not an object we can default.  This is not an error, as the
		// caching layer can pass through here, too.
	}
}

// defaultOnReadClaimList defaults a PodSchedulingList.
func (r *REST) defaultOnReadClaimList(list *core.PodSchedulingList) {
	if list == nil {
		return
	}

	for i := range list.Items {
		r.defaultOnReadClaim(&list.Items[i])
	}
}

// defaultOnRead defaults a single PodScheduling.
func (r *REST) defaultOnReadClaim(claim *core.PodScheduling) {
	if claim == nil {
		return
	}

	// TODO: can this and the code above be removed?
}

// StatusREST implements the REST endpoint for changing the status of a PodScheduling.
type StatusREST struct {
	store *genericregistry.Store
}

// New creates a new PodScheduling object.
func (r *StatusREST) New() runtime.Object {
	return &core.PodScheduling{}
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
