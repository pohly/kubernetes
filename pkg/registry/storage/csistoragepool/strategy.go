/*
Copyright 2019 The Kubernetes Authors.

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

package csistoragepool

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/apis/storage"
	"k8s.io/kubernetes/pkg/apis/storage/validation"
)

// csiStoragePoolStrategy implements behavior for CSIStoragePool objects
type csiStoragePoolStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// Strategy is the default logic that applies when creating and updating
// CSIStoragePool objects via the REST API.
var Strategy = csiStoragePoolStrategy{legacyscheme.Scheme, names.SimpleNameGenerator}

func (csiStoragePoolStrategy) NamespaceScoped() bool {
	return false
}

// PrepareForCreate is currently a NOP.
func (csiStoragePoolStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
}

func (csiStoragePoolStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	csiStoragePool := obj.(*storage.CSIStoragePool)

	errs := validation.ValidateCSIStoragePool(csiStoragePool)
	errs = append(errs, validation.ValidateCSIStoragePool(csiStoragePool)...)

	return errs
}

// Canonicalize normalizes the object after validation.
func (csiStoragePoolStrategy) Canonicalize(obj runtime.Object) {
}

func (csiStoragePoolStrategy) AllowCreateOnUpdate() bool {
	return false
}

// PrepareForUpdate is currently a NOP.
func (csiStoragePoolStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
}

func (csiStoragePoolStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	newCSIStoragePoolObj := obj.(*storage.CSIStoragePool)
	oldCSIStoragePoolObj := old.(*storage.CSIStoragePool)
	errorList := validation.ValidateCSIStoragePool(newCSIStoragePoolObj)
	return append(errorList, validation.ValidateCSIStoragePoolUpdate(newCSIStoragePoolObj, oldCSIStoragePoolObj)...)
}

func (csiStoragePoolStrategy) AllowUnconditionalUpdate() bool {
	return false
}
