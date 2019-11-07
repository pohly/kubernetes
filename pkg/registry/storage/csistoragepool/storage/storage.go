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

package storage

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	storageapi "k8s.io/kubernetes/pkg/apis/storage"
	"k8s.io/kubernetes/pkg/registry/storage/csistoragepool"
)

// CSIStoragePoolStorage includes storage for CSIStoragePools and all subresources
type CSIStoragePoolStorage struct {
	CSIStoragePool *REST
}

// REST object that will work for CSIStoragePools
type REST struct {
	*genericregistry.Store
}

// NewStorage returns a RESTStorage object that will work against CSIStoragePools
func NewStorage(optsGetter generic.RESTOptionsGetter) (*CSIStoragePoolStorage, error) {
	store := &genericregistry.Store{
		NewFunc:                  func() runtime.Object { return &storageapi.CSIStoragePool{} },
		NewListFunc:              func() runtime.Object { return &storageapi.CSIStoragePoolList{} },
		DefaultQualifiedResource: storageapi.Resource("csistoragepools"),

		CreateStrategy:      csistoragepool.Strategy,
		UpdateStrategy:      csistoragepool.Strategy,
		DeleteStrategy:      csistoragepool.Strategy,
		ReturnDeletedObject: true,
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, err
	}

	return &CSIStoragePoolStorage{
		CSIStoragePool: &REST{store},
	}, nil
}
