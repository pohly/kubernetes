package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	resourceapi "k8s.io/api/resource/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

func main() {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf("create out-of-cluster client configuration: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("create client: %v", err)
	}

	for numDevices := 0; numDevices <= 200; numDevices += 10 {
		for numAttributes := 0; numAttributes <= 50; numAttributes += 10 {
			for _, attributeNameLen := range []int{10, 96} {
				measureResourceSlice(clientset, numDevices, numAttributes, attributeNameLen)
			}
		}
	}

	measureResourceSlice(clientset, 128, 32, 96)
}

func measureResourceSlice(clientset kubernetes.Interface, numDevices, numAttributes, attributeNameLen int) {
	slice := generateResourceSlice(numDevices, numAttributes, attributeNameLen)
	slice, err := clientset.ResourceV1alpha2().ResourceSlices().Create(context.Background(), slice, metav1.CreateOptions{})
	if err != nil {
		klog.Infof("%d/%d: %v", numDevices, numAttributes, err)
		return
	}
	total := slice.Size()
	managedFields := 0
	for _, managed := range slice.ManagedFields {
		managedFields += managed.Size()
	}
	klog.Infof("%d/%d/%d: total %d, managed fields %d (%d%%)", numDevices, numAttributes, attributeNameLen, total, managedFields, managedFields*100/total)
}

func generateResourceSlice(numDevices, numAttributes, attributeNameLen int) *resourceapi.ResourceSlice {
	slice := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("rs-%04d-%04d-%04d", numDevices, numAttributes, attributeNameLen),
		},
		NodeName:   "worker",
		DriverName: "dra.example.com",
		ResourceModel: resourceapi.ResourceModel{
			NamedResources: &resourceapi.NamedResourcesResources{
				Instances: make([]resourceapi.NamedResourcesInstance, numDevices),
			},
		},
	}
	for i := 0; i < numDevices; i++ {
		slice.NamedResources.Instances[i].Name = fmt.Sprintf("device-%04d", i)
		slice.NamedResources.Instances[i].Attributes = make([]resourceapi.NamedResourcesAttribute, numAttributes)
		for e := 0; e < numAttributes; e++ {
			slice.NamedResources.Instances[i].Attributes[e].Name = strings.Repeat("a", attributeNameLen-5) + "-" + fmt.Sprintf("%04d", e)
			slice.NamedResources.Instances[i].Attributes[e].IntValue = ptr.To(int64(e))
		}
	}

	return slice
}
