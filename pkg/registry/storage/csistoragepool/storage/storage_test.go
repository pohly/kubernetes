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
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistrytest "k8s.io/apiserver/pkg/registry/generic/testing"
	etcd3testing "k8s.io/apiserver/pkg/storage/etcd3/testing"
	coreapi "k8s.io/kubernetes/pkg/apis/core"
	storageapi "k8s.io/kubernetes/pkg/apis/storage"
	_ "k8s.io/kubernetes/pkg/apis/storage/install"
	"k8s.io/kubernetes/pkg/registry/registrytest"
)

func newStorage(t *testing.T) (*REST, *etcd3testing.EtcdTestServer) {
	etcdStorage, server := registrytest.NewEtcdStorageForResource(t, storageapi.SchemeGroupVersion.WithResource("csistoragepools").GroupResource())
	restOptions := generic.RESTOptions{
		StorageConfig:           etcdStorage,
		Decorator:               generic.UndecoratedStorage,
		DeleteCollectionWorkers: 1,
		ResourcePrefix:          "csistoragepools",
	}
	csiStoragePoolStorage, err := NewStorage(restOptions)
	if err != nil {
		t.Fatalf("unexpected error from REST storage: %v", err)
	}
	return csiStoragePoolStorage.CSIStoragePool, server
}

func validNewCSIStoragePool(name string) *storageapi.CSIStoragePool {
	selector := coreapi.NodeSelector{
		NodeSelectorTerms: []coreapi.NodeSelectorTerm{
			{
				MatchExpressions: []coreapi.NodeSelectorRequirement{
					{
						Key:      "kubernetes.io/hostname",
						Operator: "In",
						Values:   []string{"node-a"},
					},
				},
			},
		},
	}
	capacity := resource.MustParse("1Gi")
	return &storageapi.CSIStoragePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: storageapi.CSIStoragePoolSpec{
			DriverName: "csi.example.com",
		},
		Status: storageapi.CSIStoragePoolStatus{
			NodeTopology: &selector,
			Classes: []storageapi.CSIStoragePoolByClass{
				{
					StorageClassName: "some-storage-class",
					Capacity:         &capacity,
				},
			},
		},
	}
}

func TestCreate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	csiStoragePool := validNewCSIStoragePool("foo")
	csiStoragePool.ObjectMeta = metav1.ObjectMeta{GenerateName: "foo"}
	test.TestCreate(
		// valid
		csiStoragePool,
		// invalid
		&storageapi.CSIStoragePool{
			ObjectMeta: metav1.ObjectMeta{Name: "*BadName!"},
		},
	)
}

func TestUpdate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()

	test.TestUpdate(
		// valid
		validNewCSIStoragePool("foo"),
		// updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*storageapi.CSIStoragePool)
			object.Labels = map[string]string{"a": "b"}
			return object
		},
		//invalid update
		func(obj runtime.Object) runtime.Object {
			object := obj.(*storageapi.CSIStoragePool)
			object.Name = "!@#$%"
			return object
		},
	)
}

func TestDelete(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope().ReturnDeletedObject()
	test.TestDelete(validNewCSIStoragePool("foo"))
}

func TestGet(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	test.TestGet(validNewCSIStoragePool("foo"))
}

func TestList(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	test.TestList(validNewCSIStoragePool("foo"))
}

func TestWatch(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := genericregistrytest.New(t, storage.Store).ClusterScope()
	test.TestWatch(
		validNewCSIStoragePool("foo"),
		// matching labels
		[]labels.Set{},
		// not matching labels
		[]labels.Set{
			{"foo": "bar"},
		},
		// matching fields
		[]fields.Set{
			{"metadata.name": "foo"},
		},
		// not matching fields
		[]fields.Set{
			{"metadata.name": "bar"},
		},
	)
}
