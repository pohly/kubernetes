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
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/tools/cache"
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

var TriggerFunc = map[string]storage.IndexerFunc{
	"nodeName": nodeNameTriggerFunc,
	// https://github.com/kubernetes/kubernetes/blob/3aa8c59fec0bf339e67ca80ea7905c817baeca85/staging/src/k8s.io/apiserver/pkg/storage/cacher/cacher.go#L346-L350
	// "driverName": driverNameTriggerFunc,
}

func nodeNameTriggerFunc(obj runtime.Object) string {
	return obj.(*resource.NodeResourceCapacity).NodeName
}

// func driverNameTriggerFunc(obj runtime.Object) string {
// 	return obj.(*resource.NodeResourceCapacity).DriverName
// }

// Indexers returns the indexers for NodeResourceCapacity.
func Indexers() *cache.Indexers {
	return &cache.Indexers{
		storage.FieldIndex("nodeName"): nodeNameIndexFunc,
		// storage.FieldIndex("driverName"): driverNameIndexFunc,
	}
}

func nodeNameIndexFunc(obj interface{}) ([]string, error) {
	capacity, ok := obj.(*resource.NodeResourceCapacity)
	if !ok {
		return nil, fmt.Errorf("not a NodeResourceCapacity")
	}
	return []string{capacity.NodeName}, nil
}

// func driverNameIndexFunc(obj interface{}) ([]string, error) {
// 	capacity, ok := obj.(*resource.NodeResourceCapacity)
// 	if !ok {
// 		return nil, fmt.Errorf("not a NodeResourceCapacity")
// 	}
// 	return []string{capacity.DriverName}, nil
// }

// GetAttrs returns labels and fields of a given object for filtering purposes.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	capacity, ok := obj.(*resource.NodeResourceCapacity)
	if !ok {
		return nil, nil, fmt.Errorf("not a NodeResourceCapacity")
	}
	return labels.Set(capacity.ObjectMeta.Labels), toSelectableFields(capacity), nil
}

// Match returns a generic matcher for a given label and field selector.
func Match(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:       label,
		Field:       field,
		GetAttrs:    GetAttrs,
		IndexFields: []string{"nodeName" /*, "driverName" */},
	}
}

// toSelectableFields returns a field set that represents the object
// TODO: fields are not labels, and the validation rules for them do not apply.
func toSelectableFields(capacity *resource.NodeResourceCapacity) fields.Set {
	// The purpose of allocation with a given number of elements is to reduce
	// amount of allocations needed to create the fields.Set. If you add any
	// field here or the number of object-meta related fields changes, this should
	// be adjusted.
	fields := make(fields.Set, 3)
	fields["nodeName"] = capacity.NodeName
	fields["driverName"] = capacity.DriverName

	// Adds one field.
	return generic.AddObjectMetaFieldsSet(fields, &capacity.ObjectMeta, false)
}
