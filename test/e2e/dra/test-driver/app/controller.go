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

// Package app does all of the work necessary to configure and run a
// Kubernetes app process.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"k8s.io/component-helpers/dra/controller"
)

type Resources struct {
	NodeLocal      bool
	Nodes          []string
	MaxAllocations int
	Shared         bool
}

func RunController(ctx context.Context, clientset kubernetes.Interface, driverName string, workers int, resources Resources) {
	driver := &exampleDriver{
		clientset: clientset,
		resources: resources,
		allocated: make(map[types.UID]string),
	}
	informerFactory := informers.NewSharedInformerFactory(clientset, 0 /* resync period */)
	ctrl := controller.New(ctx, driverName, driver, clientset, informerFactory)
	informerFactory.Start(ctx.Done())
	ctrl.Run(workers)
}

type exampleDriver struct {
	clientset kubernetes.Interface
	resources Resources

	// mutex must be locked at the gRPC call level.
	mutex sync.Mutex
	// allocated maps claim.UID to the node (if network-attached) or empty (if not).
	allocated map[types.UID]string
}
type parameters struct {
	EnvVars  map[string]string
	NodeName string
}

var _ controller.Driver = &exampleDriver{}

func (d *exampleDriver) countAllocations(node string) int {
	total := 0
	for _, n := range d.allocated {
		if n == node {
			total++
		}
	}
	return total
}

func (d *exampleDriver) GetClassParameters(ctx context.Context, class *corev1.ResourceClass) (interface{}, error) {
	// TODO: read config map, but from what namespace?!
	var env map[string]string
	return env, nil
}

func (d *exampleDriver) GetClaimParameters(ctx context.Context, claim *corev1.ResourceClaim, class *corev1.ResourceClass, classParameters interface{}) (interface{}, error) {
	if claim.Spec.Parameters != nil {
		if claim.Spec.Parameters.APIVersion != "v1" ||
			claim.Spec.Parameters.Kind != "ConfigMap" {
			return nil, fmt.Errorf("claim parameters are only supported in APIVersion v1, Kind ConfigMap, got: %v", claim.Spec.Parameters)
		}
		return d.readParametersFromConfigMap(ctx, claim.Namespace, claim.Spec.Parameters.Name)
	}
	return nil, nil
}

func (d *exampleDriver) readParametersFromConfigMap(ctx context.Context, namespace, name string) (map[string]string, error) {
	configMap, err := d.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get config map: %v", err)
	}
	return configMap.Data, nil
}

// Allocate simply copies parameters as JSON map into ResourceHandle.
func (d *exampleDriver) Allocate(ctx context.Context, claim *corev1.ResourceClaim, claimParameters interface{}, class *corev1.ResourceClass, classParameters interface{}, selectedNode string) (result *corev1.AllocationResult, err error) {
	logger := klog.FromContext(ctx).WithName("Allocate").WithValues("claim", klog.KObj(claim), "uid", claim.UID)
	defer func() {
		logger.Info("done", "result", prettyPrint(result), "err", err)
	}()

	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Already allocated? Then we don't need to count it again.
	node, ok := d.allocated[claim.UID]
	if ok {
		// Idempotent result - kind of. We don't check whether
		// the parameters changed in the meantime. A real
		// driver would have to do that.
		logger.Info("already allocated")
	} else {
		if d.resources.NodeLocal {
			node = selectedNode
			if node == "" {
				// If none has been selected because we do immediate allocation,
				// then we need to pick one ourselves.
				var viableNodes []string
				for _, n := range d.resources.Nodes {
					if d.resources.MaxAllocations == 0 ||
						d.countAllocations(n) < d.resources.MaxAllocations {
						viableNodes = append(viableNodes, n)
					}
				}
				if len(viableNodes) == 0 {
					return nil, errors.New("resources exhausted on all nodes")
				}
				// Pick randomly. We could also prefer the one with the least
				// number of allocations (even spreading) or the most (packing).
				node = viableNodes[rand.Intn(len(viableNodes))]
			} else if d.resources.MaxAllocations > 0 &&
				d.countAllocations(node) >= d.resources.MaxAllocations {
				return nil, fmt.Errorf("resources exhausted on node %q", node)
			}
		} else {
			if d.resources.MaxAllocations > 0 &&
				len(d.allocated) >= d.resources.MaxAllocations {
				return nil, errors.New("resources exhausted in the cluster")
			}
		}
	}

	allocation := &corev1.AllocationResult{
		SharedResource: d.resources.Shared,
	}
	p := parameters{
		EnvVars:  make(map[string]string),
		NodeName: node,
	}
	toEnvVars("user", claimParameters.(map[string]string), p.EnvVars)
	toEnvVars("admin", classParameters.(map[string]string), p.EnvVars)
	data, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode parameters: %v", err)
	}
	allocation.ResourceHandle = string(data)
	if node != "" {
		allocation.AvailableOnNodes = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "kubernetes.io/hostname",
							Operator: corev1.NodeSelectorOpIn,
							Values: []string{
								node,
							},
						},
					},
				},
			},
		}
	}
	d.allocated[claim.UID] = node
	return allocation, nil
}

func (d *exampleDriver) Deallocate(ctx context.Context, claim *corev1.ResourceClaim) error {
	logger := klog.FromContext(ctx).WithName("Deallocate").WithValues("claim", klog.KObj(claim), "uid", claim.UID)
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if _, ok := d.allocated[claim.UID]; !ok {
		logger.Info("already deallocated")
		return nil
	}

	logger.Info("done")
	delete(d.allocated, claim.UID)
	return nil
}

func (d *exampleDriver) UnsuitableNodes(ctx context.Context, pod *corev1.Pod, claims []*controller.ClaimAllocation, potentialNodes []string) (finalErr error) {
	logger := klog.FromContext(ctx).WithName("UnsuitableNodes").WithValues("pod", klog.KObj(pod))
	logger.Info("starting", "claim", prettyPrintSlice(claims), "potentialNodes", potentialNodes)
	defer func() {
		// UnsuitableNodes is the same for all claims.
		logger.Info("done", "unsuitableNodes", claims[0].UnsuitableNodes, "err", finalErr)
	}()
	if d.resources.MaxAllocations == 0 {
		// All nodes are suitable.
		return nil
	}
	if d.resources.NodeLocal {
		allocationsPerNode := make(map[string]int)
		for _, node := range d.resources.Nodes {
			allocationsPerNode[node] = d.countAllocations(node)
		}
		for _, claim := range claims {
			claim.UnsuitableNodes = nil
			for _, node := range potentialNodes {
				// If we have more than one claim, then a
				// single pod wants to use all of them.  That
				// can only work if a node has capacity left
				// for all of them. Also, nodes that the driver
				// doesn't run on cannot be used.
				if contains(d.resources.Nodes, node) &&
					allocationsPerNode[node]+len(claims) > d.resources.MaxAllocations {
					claim.UnsuitableNodes = append(claim.UnsuitableNodes, node)
				}
			}
		}
		return nil
	}

	allocations := d.countAllocations("")
	for _, claim := range claims {
		claim.UnsuitableNodes = nil
		for _, node := range potentialNodes {
			if contains(d.resources.Nodes, node) &&
				allocations+len(claims) > d.resources.MaxAllocations {
				claim.UnsuitableNodes = append(claim.UnsuitableNodes, node)
			}
		}
	}

	return nil
}

func toEnvVars(what string, from, to map[string]string) {
	for key, value := range from {
		to[what+"_"+strings.ToLower(key)] = value
	}
}

func contains[T comparable](list []T, value T) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}

	return false
}

func prettyPrint[T any](obj *T) interface{} {
	if obj == nil {
		return "<nil>"
	}
	return *obj
}

// prettyPrintSlice prints the values the slice points to, not the pointers.
func prettyPrintSlice[T any](slice []*T) interface{} {
	var values []interface{}
	for _, v := range slice {
		if v == nil {
			values = append(values, "<nil>")
		} else {
			values = append(values, *v)
		}
	}
	return values
}
