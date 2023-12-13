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

package noderesourcecapacity

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/apis/resource"
	"k8s.io/kubernetes/pkg/apis/resource/validation"
)

// nodeResourceCapacityStrategy implements behavior for NodeResourceCapacity objects
type nodeResourceCapacityStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

var Strategy = nodeResourceCapacityStrategy{legacyscheme.Scheme, names.SimpleNameGenerator}

func (nodeResourceCapacityStrategy) NamespaceScoped() bool {
	return false
}

func (nodeResourceCapacityStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
}

func (nodeResourceCapacityStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	nodeResourceCapacity := obj.(*resource.NodeResourceCapacity)
	return validation.ValidateNodeResourceCapacity(nodeResourceCapacity)
}

func (nodeResourceCapacityStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

func (nodeResourceCapacityStrategy) Canonicalize(obj runtime.Object) {
}

func (nodeResourceCapacityStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (nodeResourceCapacityStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
}

func (nodeResourceCapacityStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateNodeResourceCapacityUpdate(obj.(*resource.NodeResourceCapacity), old.(*resource.NodeResourceCapacity))
}

func (nodeResourceCapacityStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}

func (nodeResourceCapacityStrategy) AllowUnconditionalUpdate() bool {
	return true
}
