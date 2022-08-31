/*
Copyright 2017 The Kubernetes Authors.

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

package volumebinding

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	storagehelpers "k8s.io/component-helpers/storage/volume"
)

// PVAssumeCache is a AssumeCache for PersistentVolume objects
type PVAssumeCache interface {
	cache.AssumeCache[*v1.PersistentVolume]

	ListPVs(storageClassName string) []*v1.PersistentVolume
}

type pvAssumeCache struct {
	cache.AssumeCache[*v1.PersistentVolume]
}

func pvStorageClassIndexFunc(obj interface{}) ([]string, error) {
	if pv, ok := obj.(*v1.PersistentVolume); ok {
		return []string{storagehelpers.GetPersistentVolumeClass(pv)}, nil
	}
	return []string{""}, fmt.Errorf("object is not a v1.PersistentVolume: %v", obj)
}

// NewPVAssumeCache creates a PV assume cache.
func NewPVAssumeCache(informer cache.AssumeCacheInformer) PVAssumeCache {
	return pvAssumeCache{cache.NewAssumeCache[*v1.PersistentVolume](informer, "v1.PersistentVolume", "storageclass", pvStorageClassIndexFunc)}
}

func (c pvAssumeCache) ListPVs(storageClassName string) []*v1.PersistentVolume {
	return c.List(&v1.PersistentVolume{
		Spec: v1.PersistentVolumeSpec{
			StorageClassName: storageClassName,
		},
	})
}

// PVCAssumeCache is a AssumeCache for PersistentVolumeClaim objects
type PVCAssumeCache cache.AssumeCache[*v1.PersistentVolumeClaim]

// NewPVCAssumeCache creates a PVC assume cache.
func NewPVCAssumeCache(informer cache.AssumeCacheInformer) PVCAssumeCache {
	return cache.NewAssumeCache[*v1.PersistentVolumeClaim](informer, "v1.PersistentVolumeClaim", "", nil)
}
