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

package cache

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/ktesting"
	_ "k8s.io/klog/v2/ktesting/init"
)

// persistentVolume is a simplified version of v1.PersistentVolume.
type persistentVolume struct {
	metav1.ObjectMeta

	storageClassName string
	claimRef         string
}

func makePV(name, storageClassName, resourceVersion string) *persistentVolume {
	return &persistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: resourceVersion,
		},
		storageClassName: storageClassName,
	}
}

type pvAssumeCache struct {
	AssumeCache[*persistentVolume]
}

func (c pvAssumeCache) ListPVs(storageClassName string) []*persistentVolume {
	return c.List(&persistentVolume{
		storageClassName: storageClassName,
	})
}

func pvStorageClassIndexFunc(pv *persistentVolume) ([]string, error) {
	return []string{pv.storageClassName}, nil
}

func newPVAssumeCache(logger klog.Logger, informer AssumeCacheInformer) pvAssumeCache {
	return pvAssumeCache{NewAssumeCache[*persistentVolume](logger, informer, "persistentVolume", "storageclass", pvStorageClassIndexFunc)}
}

// fakeAssumeCacheInformer makes it possible to add/update/delete objects directly.
type fakeAssumeCacheInformer struct {
	eventHandlers []cache.ResourceEventHandler
}

func (fi *fakeAssumeCacheInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	fi.eventHandlers = append(fi.eventHandlers, handler)
	return nil, nil
}

func (fi *fakeAssumeCacheInformer) add(obj interface{}) {
	for _, handler := range fi.eventHandlers {
		handler.OnAdd(obj)
	}
}

func (fi *fakeAssumeCacheInformer) update(oldObj, newObj interface{}) {
	for _, handler := range fi.eventHandlers {
		handler.OnUpdate(oldObj, newObj)
	}
}

func (fi *fakeAssumeCacheInformer) delete(obj interface{}) {
	for _, handler := range fi.eventHandlers {
		handler.OnDelete(obj)
	}
}

func newTestPVAssumeCache(logger klog.Logger) (pvAssumeCache, *fakeAssumeCacheInformer) {
	internalCache := &fakeAssumeCacheInformer{}
	cache := newPVAssumeCache(logger, internalCache)
	return cache, internalCache
}

func verifyListPVs(t *testing.T, cache pvAssumeCache, expectedPVs map[string]*persistentVolume, storageClassName string) {
	pvList := cache.ListPVs(storageClassName)
	if len(pvList) != len(expectedPVs) {
		t.Errorf("ListPVs() returned %v PVs, expected %v", len(pvList), len(expectedPVs))
	}
	for _, pv := range pvList {
		expectedPV, ok := expectedPVs[pv.Name]
		if !ok {
			t.Errorf("ListPVs() returned unexpected PV %q", pv.Name)
		}
		if expectedPV != pv {
			t.Errorf("ListPVs() returned PV %p, expected %p", pv, expectedPV)
		}
	}
}

func verifyPV(cache pvAssumeCache, name string, expectedPV *persistentVolume) error {
	pv, err := cache.Get(name)
	if err != nil {
		return err
	}
	if pv != expectedPV {
		return fmt.Errorf("GetPV() returned %p, expected %p", pv, expectedPV)
	}
	return nil
}

func TestAssumePV(t *testing.T) {
	scenarios := map[string]struct {
		cachedPV      *persistentVolume
		oldPV         *persistentVolume
		newPV         *persistentVolume
		shouldSucceed bool
	}{
		"success-same-version": {
			oldPV:         makePV("pv1", "", "5"),
			newPV:         makePV("pv1", "", "5"),
			shouldSucceed: true,
		},
		"success-storageclass-same-version": {
			oldPV:         makePV("pv1", "class1", "5"),
			newPV:         makePV("pv1", "class1", "5"),
			shouldSucceed: true,
		},
		"success-new-higher-version": {
			oldPV:         makePV("pv1", "", "5"),
			newPV:         makePV("pv1", "", "6"),
			shouldSucceed: true,
		},
		"success-same-higher-version": {
			cachedPV:      makePV("pv1", "", "6"),
			oldPV:         makePV("pv1", "", "5"),
			newPV:         makePV("pv1", "", "6"),
			shouldSucceed: true,
		},
		"fail-old-not-found": {
			oldPV:         makePV("pv2", "", "5"),
			newPV:         makePV("pv1", "", "5"),
			shouldSucceed: false,
		},
		"fail-different-version": {
			oldPV:         makePV("pv1", "", "2"),
			cachedPV:      makePV("pv1", "", "3"),
			newPV:         makePV("pv1", "", "4"),
			shouldSucceed: false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			logger, _ := ktesting.NewTestContext(t)
			cache := newPVAssumeCache(logger, nil)
			internalCache, ok := cache.AssumeCache.(*assumeCache[*persistentVolume])
			if !ok {
				t.Fatalf("Failed to get internal cache")
			}

			// Add oldPV to cache or, if set, some other PV.
			cachedPV := scenario.cachedPV
			if cachedPV == nil {
				cachedPV = scenario.oldPV
			}
			internalCache.add(cachedPV)
			if err := verifyPV(cache, cachedPV.Name, cachedPV); err != nil {
				t.Fatalf("Failed to GetPV() after initial update: %v", err)
			}

			// Assume newPV
			err := cache.Assume(logger, scenario.oldPV, scenario.newPV)
			if scenario.shouldSucceed && err != nil {
				t.Errorf("Assume() returned error %v", err)
			}
			if !scenario.shouldSucceed && err == nil {
				t.Errorf("Assume() returned success but expected error")
			}

			// Check that GetPV returns correct PV
			expectedPV := scenario.newPV
			if !scenario.shouldSucceed {
				expectedPV = cachedPV
			}
			if err := verifyPV(cache, scenario.oldPV.Name, expectedPV); err != nil {
				t.Errorf("Failed to GetPV() after initial update: %v", err)
			}
		})
	}
}

func TestRestorePV(t *testing.T) {
	logger, _ := ktesting.NewTestContext(t)
	cache := newPVAssumeCache(logger, nil)
	internalCache, ok := cache.AssumeCache.(*assumeCache[*persistentVolume])
	if !ok {
		t.Fatalf("Failed to get internal cache")
	}

	oldPV := makePV("pv1", "", "5")
	newPV := makePV("pv1", "", "5")

	// Restore PV that doesn't exist
	cache.Restore(logger, "nothing")

	// Add oldPV to cache
	internalCache.add(oldPV)
	if err := verifyPV(cache, oldPV.Name, oldPV); err != nil {
		t.Fatalf("Failed to GetPV() after initial update: %v", err)
	}

	// Restore PV
	cache.Restore(logger, oldPV.Name)
	if err := verifyPV(cache, oldPV.Name, oldPV); err != nil {
		t.Fatalf("Failed to GetPV() after initial restore: %v", err)
	}

	// Assume newPV
	if err := cache.Assume(logger, oldPV, newPV); err != nil {
		t.Fatalf("Assume() returned error %v", err)
	}
	if err := verifyPV(cache, oldPV.Name, newPV); err != nil {
		t.Fatalf("Failed to GetPV() after Assume: %v", err)
	}

	// Restore PV
	cache.Restore(logger, oldPV.Name)
	if err := verifyPV(cache, oldPV.Name, oldPV); err != nil {
		t.Fatalf("Failed to GetPV() after restore: %v", err)
	}
}

func TestBasicPVCache(t *testing.T) {
	logger, _ := ktesting.NewTestContext(t)
	cache := newPVAssumeCache(logger, nil)
	internalCache, ok := cache.AssumeCache.(*assumeCache[*persistentVolume])
	if !ok {
		t.Fatalf("Failed to get internal cache")
	}

	// Get object that doesn't exist
	pv, err := cache.Get("nothere")
	if err == nil {
		t.Errorf("Get() returned unexpected success")
	}
	if pv != nil {
		t.Errorf("Get() returned unexpected PV %+v", pv)
	}

	// Add a bunch of PVs
	pvs := map[string]*persistentVolume{}
	for i := 0; i < 10; i++ {
		pv := makePV(fmt.Sprintf("test-pv%v", i), "", "1")
		pvs[pv.Name] = pv
		internalCache.add(pv)
	}

	// List them
	verifyListPVs(t, cache, pvs, "")

	// Update a PV
	updatedPV := makePV("test-pv3", "", "2")
	pvs[updatedPV.Name] = updatedPV
	internalCache.update(nil, updatedPV)

	// List them
	verifyListPVs(t, cache, pvs, "")

	// Delete a PV
	deletedPV := pvs["test-pv7"]
	delete(pvs, deletedPV.Name)
	internalCache.delete(deletedPV)

	// List them
	verifyListPVs(t, cache, pvs, "")
}

func TestPVCacheWithStorageClasses(t *testing.T) {
	logger, _ := ktesting.NewTestContext(t)
	cache := newPVAssumeCache(logger, nil)
	internalCache, ok := cache.AssumeCache.(*assumeCache[*persistentVolume])
	if !ok {
		t.Fatalf("Failed to get internal cache")
	}

	// Add a bunch of PVs
	pvs1 := map[string]*persistentVolume{}
	for i := 0; i < 10; i++ {
		pv := makePV(fmt.Sprintf("test-pv%v", i), "class1", "1")
		pvs1[pv.Name] = pv
		internalCache.add(pv)
	}

	// Add a bunch of PVs
	pvs2 := map[string]*persistentVolume{}
	for i := 0; i < 10; i++ {
		pv := makePV(fmt.Sprintf("test2-pv%v", i), "class2", "1")
		pvs2[pv.Name] = pv
		internalCache.add(pv)
	}

	// List them
	verifyListPVs(t, cache, pvs1, "class1")
	verifyListPVs(t, cache, pvs2, "class2")

	// Update a PV
	updatedPV := makePV("test-pv3", "class1", "2")
	pvs1[updatedPV.Name] = updatedPV
	internalCache.update(nil, updatedPV)

	// List them
	verifyListPVs(t, cache, pvs1, "class1")
	verifyListPVs(t, cache, pvs2, "class2")

	// Delete a PV
	deletedPV := pvs1["test-pv7"]
	delete(pvs1, deletedPV.Name)
	internalCache.delete(deletedPV)

	// List them
	verifyListPVs(t, cache, pvs1, "class1")
	verifyListPVs(t, cache, pvs2, "class2")
}

func TestAssumeUpdatePVCache(t *testing.T) {
	logger, _ := ktesting.NewTestContext(t)
	cache := newPVAssumeCache(logger, nil)
	internalCache, ok := cache.AssumeCache.(*assumeCache[*persistentVolume])
	if !ok {
		t.Fatalf("Failed to get internal cache")
	}

	pvName := "test-pv0"

	// Add a PV
	pv := makePV(pvName, "", "1")
	internalCache.add(pv)
	if err := verifyPV(cache, pvName, pv); err != nil {
		t.Fatalf("failed to get PV: %v", err)
	}

	// Assume PV
	newPV := *pv
	newPV.claimRef = "test-claim"
	if err := cache.Assume(logger, pv, &newPV); err != nil {
		t.Fatalf("failed to assume PV: %v", err)
	}
	if err := verifyPV(cache, pvName, &newPV); err != nil {
		t.Fatalf("failed to get PV after assume: %v", err)
	}

	// Add old PV, overwrites the new one.
	internalCache.add(pv)
	if err := verifyPV(cache, pvName, pv); err != nil {
		t.Fatalf("failed to get PV after old PV added: %v", err)
	}
}

func TestAssumeEventHandler(t *testing.T) {
	logger, _ := ktesting.NewTestContext(t)
	informer := &fakeAssumeCacheInformer{}
	c := newPVAssumeCache(logger, informer)

	pv := makePV("test-pv0", "", "1")
	pvUpdate := makePV("test-pv0", "test-claim", "2")

	counts := map[string]int{}
	c.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				counts["added"]++

				if p, ok := obj.(*persistentVolume); !ok || p != pv {
					t.Errorf("Expected %v, got %v", pv, obj)
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				counts["updated"]++

				if p, ok := oldObj.(*persistentVolume); !ok || p != pv {
					t.Errorf("Expected %v, got %v", pv, oldObj)
				}
				if p, ok := newObj.(*persistentVolume); !ok || p != pvUpdate {
					t.Errorf("Expected %v, got %v", pv, newObj)
				}

			},
			DeleteFunc: func(obj interface{}) {
				counts["deleted"]++

				if p, ok := obj.(*persistentVolume); !ok || p != pvUpdate {
					t.Errorf("Expected %v, got %v", pv, obj)
				}
			},
		},
	)

	informer.add(pv)
	informer.update(pv, pvUpdate)
	informer.delete(pvUpdate)
	for name, count := range counts {
		if count != 1 {
			t.Errorf("Expected 1 %s event, got %d", name, count)
		}
	}
}

var result interface{}

// BenchmarkAssumePVInformerInput simulates receiving a constant stream of
// add/update/delete input from the informer.
func BenchmarkAssumePVInformerInput(b *testing.B) {
	_, internalCache := newTestPVAssumeCache(logr.Discard())
	pvName := "test-pv0"
	pv := makePV(pvName, "", "1")
	pvUpdate := makePV(pvName, "", "2")

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		internalCache.add(pv)
		internalCache.update(pv, pvUpdate)
		internalCache.delete(pvUpdate)
	}
}

// BenchmarkAssumePVGet simulates retrieving the same assumed PV repeatedly.
func BenchmarkAssumePVGet(b *testing.B) {
	logger := logr.Discard()
	cache, internalCache := newTestPVAssumeCache(logger)
	pvName := "test-pv0"
	pv := makePV(pvName, "", "1")
	pvUpdate := makePV(pvName, "", "2")
	internalCache.add(pv)
	if err := cache.Assume(logger, pv, pvUpdate); err != nil {
		b.Fatalf("unexpected cache.Assume error: %v", err)
	}

	var r interface{}
	var err error

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		r, err = cache.Get(pvName)
		if err != nil {
			b.Fatalf("unexpected Get error: %v", err)
		}
	}
	result = r
}

// BenchmarkAssumePVRestore simulates assuming and restoring a PV.
func BenchmarkAssumePVRestore(b *testing.B) {
	logger := logr.Discard()
	cache, internalCache := newTestPVAssumeCache(logger)
	pvName := "test-pv0"
	pv := makePV(pvName, "", "1")
	pvUpdate := makePV(pvName, "", "2")
	internalCache.add(pv)
	if err := cache.Assume(logger, pv, pvUpdate); err != nil {
		b.Fatalf("unexpected cache.Assume error: %v", err)
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if err := cache.Assume(logger, pv, pvUpdate); err != nil {
			b.Fatalf("unexpected cache.Assume error: %v", err)
		}
		cache.Restore(logger, pvName)
	}
}
