/*
Copyright 2017,2020 The Kubernetes Authors.

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
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	kcache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/kubernetes/pkg/volume/util/operationexecutor"
	"k8s.io/kubernetes/pkg/volume/util/volumepathhandler"
)

const (
	// number of default volume expansion workers
	defaultWorkerCount = 10
)

// EphemeralController creates PVCs for ephemeral inline volumes in a pod spec.
type EphemeralController interface {
	Run(stopCh <-chan struct{})
}

type ephemeralController struct {
	// kubeClient is the kube API client used by volumehost to communicate with
	// the API server.
	kubeClient clientset.Interface

	// pvcLister is the shared PVC lister used to fetch and store PVC
	// objects from the API server. It is shared with other controllers and
	// therefore the PVC objects in its store should be treated as immutable.
	pvcLister  corelisters.PersistentVolumeClaimLister
	pvcsSynced kcache.InformerSynced

	// same for pods
	podLister corelisters.PodLister
	podSynced kcache.InformerSynced

	// recorder is used to record events in the API server
	recorder record.EventRecorder

	operationGenerator operationexecutor.OperationGenerator

	queue workqueue.RateLimitingInterface
}

// NewEphemeralController ephemerals the pvs
func NewEphemeralController(
	kubeClient clientset.Interface,
	podInformer coreinformers.PodInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer) (EphemeralController, error) {

	ec := &ephemeralController{
		kubeClient: kubeClient,
		podLister:  podInformer.Lister(),
		podSynced:  podInformer.Informer().HasSynced,
		pvcLister:  pvcInformer.Lister(),
		pvcsSynced: pvcInformer.Informer().HasSynced,
		queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ephemeral_volume"),
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	ec.recorder = eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "ephemeral_volume"})
	blkutil := volumepathhandler.NewBlockVolumePathHandler()

	ec.operationGenerator = operationexecutor.NewOperationGenerator(
		kubeClient,
		nil,
		ec.recorder,
		false,
		blkutil)

	podInformer.Informer().AddEventHandler(kcache.ResourceEventHandlerFuncs{
		AddFunc: ec.enqueuePod,
	})

	return ec, nil
}

func (ec *ephemeralController) enqueuePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return
	}

	// Ignore pods which are already getting deleted.
	if pod.DeletionTimestamp != nil {
		return
	}

	for _, vol := range pod.Spec.Volumes {
		if vol.Ephemeral != nil {
			// It has at least one ephemeral inline volume, work on it.
			key, err := kcache.DeletionHandlingMetaNamespaceKeyFunc(pod)
			if err != nil {
				runtime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", pod, err))
				return
			}
			ec.queue.Add(key)
			break
		}
	}
}

func (ec *ephemeralController) processNextWorkItem() bool {
	key, shutdown := ec.queue.Get()
	if shutdown {
		return false
	}
	defer ec.queue.Done(key)

	err := ec.syncHandler(key.(string))
	if err == nil {
		ec.queue.Forget(key)
		return true
	}

	// TODO: emit errors as pod events

	runtime.HandleError(fmt.Errorf("%v failed with : %v", key, err))
	ec.queue.AddRateLimited(key)

	return true
}

// syncHandler performs actual expansion of volume. If an error is returned
// from this function - PVC will be requeued for resizing.
func (ec *ephemeralController) syncHandler(key string) error {
	namespace, name, err := kcache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	pod, err := ec.podLister.Pods(namespace).Get(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		// TODO: structured logging, per-instance logger (for testing)
		klog.V(5).Infof("Error getting pod %q (uid: %q) from informer : %v", pod.Name, pod.UID, err)
		return err
	}

	for _, vol := range pod.Spec.Volumes {
		ephemeral := vol.Ephemeral
		if ephemeral == nil {
			continue
		}

		pvcName := pod.Name + "-" + vol.Name
		pvc, err := ec.pvcLister.PersistentVolumeClaims(pod.Namespace).Get(pvcName)
		if pvc != nil {
			if metav1.IsControlledBy(pvc, pod) {
				// Already created, nothing more to do.
				continue
			}
			return fmt.Errorf("PVC %q (uid: %q) was not created for the pod %s",
				util.GetPersistentVolumeClaimQualifiedName(pvc), pvc.UID, pod.Name)
		}

		// Create the PVC with pod as owner.
		isTrue := true
		pvc = &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvcName,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "v1",
						Kind:               "Pod",
						Name:               pod.Name,
						UID:                pod.UID,
						Controller:         &isTrue,
						BlockOwnerDeletion: &isTrue,
					},
				},
				// TODO: decide whether we want to copy labels
			},
			Spec: *ephemeral.VolumeClaim,
		}
		// TODO: replace stopCh with context and use that here.
		_, err = ec.kubeClient.CoreV1().PersistentVolumeClaims(pod.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create PVC %q for pod %s: %v", pvcName, pod.Name, err)
		}
	}

	return nil
}

// TODO make concurrency configurable (workers/threadiness argument). previously, nestedpendingoperations spawned unlimited goroutines
func (ec *ephemeralController) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer ec.queue.ShutDown()

	klog.Infof("Starting ephemeral volume controller")
	defer klog.Infof("Shutting down ephemeral volume controller")

	if !cache.WaitForNamedCacheSync("ephemeral", stopCh, ec.podSynced, ec.pvcsSynced) {
		return
	}

	for i := 0; i < defaultWorkerCount; i++ {
		go wait.Until(ec.runWorker, time.Second, stopCh)
	}

	<-stopCh
}

func (ec *ephemeralController) runWorker() {
	for ec.processNextWorkItem() {
	}
}

func podKey(pod *v1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}
