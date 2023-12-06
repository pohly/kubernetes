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

package apiserver

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/test/integration/framework"
)

// Tests that the apiserver returns "not found" consistently when multiple
// clients update and delete the same object concurrently.
func TestDeleteConflicts(t *testing.T) {
	ctx, clientSet, _, tearDownFn := setup(t)
	defer tearDownFn()

	ns := framework.CreateNamespaceOrDie(clientSet, "status-code", t)
	defer framework.DeleteNamespaceOrDie(clientSet, ns, t)

	numOfObjects := 100
	numOfOperationsPerObject := 100
	successes := int32(0)
	notFounds := int32(0)

	for i := 0; i < numOfObjects; i++ {
		// Create the objects we're going to conflict on.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		}
		secret, err := clientSet.CoreV1().Secrets(ns.Name).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("FAILURE: error creating secret #%d: %v", i, err)
		}

		wg := sync.WaitGroup{}

		// Mix several different patch operations with a single delete in the middle.
		// Each patch operation succeeds if the object still exists. "Not found" is
		// a valid response. Everything else isn't.
		for e := 0; e < numOfOperationsPerObject; e++ {
			wg.Add(1)
			go func(i int, e int) {
				defer wg.Done()

				if e == numOfOperationsPerObject/2 {
					// Halfway through, delete now.
					if err := clientSet.CoreV1().Secrets(ns.Name).Delete(ctx, secret.Name, metav1.DeleteOptions{}); err != nil {
						t.Errorf("FAILURE: error deleting #%d: %v", i, err)
					}
					return
				}

				// This patch includes the UID to ensure that the right object instance
				// gets patched. This cannot fail in this test because the secret
				// only gets created once, but it may be relevant elsewhere.
				patch := fmt.Sprintf(`{"metadata":{"uid": %q, "labels":{"label-%d":"%d"}}}`, secret.UID, e, i)
				_, err := clientSet.CoreV1().Secrets(secret.Namespace).Patch(ctx, secret.Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
				switch {
				case err == nil:
					atomic.AddInt32(&successes, 1)
				case apierrors.IsNotFound(err):
					atomic.AddInt32(&notFounds, 1)
				default:
					t.Errorf("FAILURE: error patching secret #%d during operation #%d: %v", i, e, err)
				}
			}(i, e)
		}

		wg.Wait()
	}

	if actual, expected := successes+notFounds, int32(numOfObjects*(numOfOperationsPerObject-1)); actual < expected {
		t.Errorf("FAILURE: Expected all %d patch operations to succeed or fail with 'not found', but %d failed for some other reason.", expected, actual)
	} else {
		t.Logf("Got %d patches with the expected result.", expected)
	}

	// Verify that the test really covers both scenarios. This is racy, so
	// it might have to be removed.
	if successes == 0 {
		t.Error("FAILURE: Expected at least one successful patch, got none.")
	} else if notFounds == 0 {
		t.Error("FAILURE: Expected at lease one 'not found', got none.")
	} else {
		t.Logf("Got a mixture of %d successful patches and %d 'not found'.", successes, notFounds)
	}
}
