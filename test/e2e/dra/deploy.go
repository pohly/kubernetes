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

package dra

import (
	"bytes"
	"context"
	"errors"
	"net"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"google.golang.org/grpc"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/component-helpers/dra/kubeletplugin"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/test/e2e/dra/test-driver/app"
	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2ereplicaset "k8s.io/kubernetes/test/e2e/framework/replicaset"
	e2eskipper "k8s.io/kubernetes/test/e2e/framework/skipper"
	"k8s.io/kubernetes/test/e2e/storage/drivers/proxy"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

const (
	NodePrepareResourceMethod   = "/v1alpha1.Node/NodePrepareResource"
	NodeUnprepareResourceMethod = "/v1alpha1.Node/NodeUnprepareResource"
)

// NewDriver sets up controller (as client of the cluster) and
// kubelet plugin (via proxy) before the test runs. It cleans
// up after the test.
//
// This must be called at least one level lower in the Gingko
// hierarchy than the creationg of the framework instance,
// otherwise f.ClientSet would already be cleared by the time
// that our AfterEach callback gets invoked.
func NewDriver(f *framework.Framework, minNodes, maxNodes int) *Driver {
	d := &Driver{
		f:          f,
		minNodes:   minNodes,
		maxNodes:   maxNodes,
		fail:       map[MethodInstance]bool{},
		callCounts: map[MethodInstance]int64{},
	}

	ginkgo.BeforeEach(func() {
		d.SetUp()
	})

	ginkgo.AfterEach(func() {
		d.TearDown()
	})

	return d
}

type MethodInstance struct {
	Hostname   string
	FullMethod string
}

type Driver struct {
	f                  *framework.Framework
	ctx                context.Context
	cleanup            []func() // executed first-in-first-out
	wg                 sync.WaitGroup
	minNodes, maxNodes int

	Name  string
	Hosts map[string]*app.ExamplePlugin

	mutex      sync.Mutex
	fail       map[MethodInstance]bool
	callCounts map[MethodInstance]int64
}

func (d *Driver) SetUp() {
	d.Hosts = map[string]*app.ExamplePlugin{}
	d.Name = d.f.UniqueName + ".k8s.io"

	ctx, cancel := context.WithCancel(context.Background())
	d.ctx = ctx
	d.cleanup = append(d.cleanup, cancel)

	// The controller is easy: we simply connect to the API server. It
	// would be slightly nicer if we had a way to wait for all goroutines, but
	// SharedInformerFactory has no API for that. At least we can wait
	// for our own goroutine to stop once the context gets cancelled.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		app.RunController(d.ctx, d.f.ClientSet, d.Name, 1 /* workers */)
	}()

	// The kubelet plugin is harder. We deploy the builtin manifest
	// after patching in the driver name and all nodes on which we
	// want the plugin to run.
	//
	// Only a subset of the nodes are picked to avoid causing
	// unnecessary load on a big cluster.
	nodes, err := e2enode.GetBoundedReadySchedulableNodes(d.f.ClientSet, d.maxNodes)
	framework.ExpectNoError(err, "get nodes")
	numNodes := int32(len(nodes.Items))
	if int(numNodes) < d.minNodes {
		e2eskipper.Skipf("%d ready nodes required, only have %d", numNodes, d.minNodes)
	}

	manifests := []string{
		// The code below matches the content of this manifest (ports,
		// container names, etc.).
		"test/e2e/testing-manifests/dra/dra-test-driver-proxy.yaml",
	}
	instanceKey := "app.kubernetes.io/instance"
	rsName := ""
	draAddr := path.Join(framework.TestContext.KubeletRootDir, "plugins", d.Name+".sock")
	undeploy, err := utils.CreateFromManifests(d.f, d.f.Namespace, func(item interface{}) error {
		switch item := item.(type) {
		case *appsv1.ReplicaSet:
			rsName = item.Name
			item.Spec.Replicas = &numNodes
			item.Spec.Selector.MatchLabels[instanceKey] = d.Name
			item.Spec.Template.Labels[instanceKey] = d.Name
			item.Spec.Template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchLabels[instanceKey] = d.Name
			item.Spec.Template.Spec.Volumes[0].HostPath.Path = path.Join(framework.TestContext.KubeletRootDir, "plugins")
			item.Spec.Template.Spec.Volumes[2].HostPath.Path = path.Join(framework.TestContext.KubeletRootDir, "plugins_registry")
			item.Spec.Template.Spec.Containers[0].Args = append(item.Spec.Template.Spec.Containers[0].Args, "--endpoint=/plugins_registry/"+d.Name+"-reg.sock")
			item.Spec.Template.Spec.Containers[1].Args = append(item.Spec.Template.Spec.Containers[1].Args, "--endpoint=/dra/"+d.Name+".sock")
		}
		return nil
	}, manifests...)
	framework.ExpectNoError(err, "deploy kubelet plugin replicaset")
	d.cleanup = append(d.cleanup, undeploy)

	rs, err := d.f.ClientSet.AppsV1().ReplicaSets(d.f.Namespace.Name).Get(ctx, rsName, metav1.GetOptions{})
	framework.ExpectNoError(err, "get replicaset")

	// Wait for all pods to be running.
	if err := e2ereplicaset.WaitForReplicaSetTargetAvailableReplicas(d.f.ClientSet, rs, numNodes); err != nil {
		framework.ExpectNoError(err, "all kubelet plugin proxies running")
	}
	requirement, err := labels.NewRequirement(instanceKey, selection.Equals, []string{d.Name})
	framework.ExpectNoError(err, "create label selector requirement")
	selector := labels.NewSelector().Add(*requirement)
	pods, err := d.f.ClientSet.CoreV1().Pods(d.f.Namespace.Name).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	framework.ExpectNoError(err, "list proxy pods")
	gomega.Expect(numNodes).To(gomega.Equal(int32(len(pods.Items))), "number of proxy pods")

	// Run registar and plugin for each of the pods.
	for _, pod := range pods.Items {
		hostname := pod.Spec.NodeName
		logger := klog.Background().WithName("kubelet plugin").WithValues("node", pod.Spec.NodeName, "pod", klog.KObj(&pod))
		plugin, err := app.StartPlugin(logger, "/cdi", d.Name,
			app.FileOperations{
				Create: func(name string, content []byte) error {
					return d.createFile(&pod, name, content)
				},
				Remove: func(name string) error {
					return d.removeFile(&pod, name)
				},
			},
			kubeletplugin.GRPCVerbosity(0),
			kubeletplugin.GRPCInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				return d.interceptor(hostname, ctx, req, info, handler)
			}),
			kubeletplugin.PluginListener(listen(ctx, d.f, pod.Name, "plugin", 9001)),
			kubeletplugin.RegistrarListener(listen(ctx, d.f, pod.Name, "registrar", 9000)),
			kubeletplugin.KubeletPluginSocketPath(draAddr),
		)
		framework.ExpectNoError(err, "start kubelet plugin for node %s", pod.Spec.NodeName)
		d.cleanup = append(d.cleanup, func() {
			// Depends on cancel being called first.
			plugin.Stop()
		})
		d.Hosts[hostname] = plugin
	}

	// Wait for registration.
	gomega.Eventually(func(g gomega.Gomega) {
		var notRegistered []string
		for hostname, plugin := range d.Hosts {
			if !plugin.IsRegistered() {
				notRegistered = append(notRegistered, hostname)
			}
		}
		sort.Strings(notRegistered)
		g.Expect(notRegistered).To(gomega.BeEmpty(), "hosts where the plugin has not been registered yet")
	}).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Succeed())
}

func (d *Driver) createFile(pod *corev1.Pod, name string, content []byte) error {
	buffer := bytes.NewBuffer(content)
	return d.podIO(pod).CreateFile(name, buffer)
}

func (d *Driver) removeFile(pod *corev1.Pod, name string) error {
	return d.podIO(pod).RemoveAll(name)
}

func (d *Driver) podIO(pod *corev1.Pod) proxy.PodDirIO {
	return proxy.PodDirIO{
		F:             d.f,
		Namespace:     pod.Namespace,
		PodName:       pod.Name,
		ContainerName: "plugin",
	}
}

func listen(ctx context.Context, f *framework.Framework, podName, containerName string, port int) net.Listener {
	addr := proxy.Addr{
		Namespace:     f.Namespace.Name,
		PodName:       podName,
		ContainerName: containerName,
		Port:          port,
	}
	listener, err := proxy.Listen(ctx, f.ClientSet, f.ClientConfig(), addr)
	framework.ExpectNoError(err, "listen for connections from %+v", addr)
	return listener
}

func (d *Driver) TearDown() {
	for _, c := range d.cleanup {
		c()
	}
	d.cleanup = nil
	d.wg.Wait()
}

func (d *Driver) interceptor(hostname string, ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	m := MethodInstance{hostname, info.FullMethod}
	d.callCounts[m]++
	if d.fail[m] {
		return nil, errors.New("injected error")
	}

	return handler(ctx, req)
}

func (d *Driver) Fail(m MethodInstance, injectError bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.fail[m] = injectError
}

func (d *Driver) CallCount(m MethodInstance) int64 {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.callCounts[m]
}

func (d *Driver) Hostnames() (hostnames []string) {
	for hostname := range d.Hosts {
		hostnames = append(hostnames, hostname)
	}
	sort.Strings(hostnames)
	return
}
