/*
Copyright 2019,2020 The Kubernetes Authors.

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

package ephemeral

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/kubernetes/pkg/controller"
	controllervolumetesting "k8s.io/kubernetes/pkg/controller/volume/attachdetach/testing"
)

func TestSyncHandler(t *testing.T) {
	tests := []struct {
		name     string
		podKey   string
		pod      *v1.Pod
		hasError bool
	}{
		{
			name: "create PVC",
		},
	}

	for _, tc := range tests {
		test := tc
		t.Run(test.name, func(t *testing.T) {
			fakeKubeClient := controllervolumetesting.CreateTestClient()
			informerFactory := informers.NewSharedInformerFactory(fakeKubeClient, controller.NoResyncPeriodFunc())
			podInformer := informerFactory.Core().V1().Pods()
			pvcInformer := informerFactory.Core().V1().PersistentVolumeClaims()

			ephc, err := NewEphemeralController(fakeKubeClient, podInformer, pvcInformer)
			if err != nil {
				t.Fatalf("error creating ephemeral controller : %v", err)
			}
			ephController, _ := ephc.(*ephemeralController)

			err = ephController.syncHandler(test.podKey)
			if err != nil && !test.hasError {
				t.Fatalf("unexpected error while running handler : %v", err)
			}
			if err == nil && test.hasError {
				t.Fatalf("unexpected success")
			}
		})
	}
}

func getFakePersistentVolumeClaim(pvcName, volumeName, scName string, uid types.UID) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: "default", UID: uid},
		Spec:       v1.PersistentVolumeClaimSpec{},
	}
	if volumeName != "" {
		pvc.Spec.VolumeName = volumeName
	}

	if scName != "" {
		pvc.Spec.StorageClassName = &scName
	}
	return pvc
}
