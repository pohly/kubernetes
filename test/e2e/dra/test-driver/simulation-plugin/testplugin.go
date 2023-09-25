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

package simulationplugin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	v1 "k8s.io/api/core/v1"
	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	resourcev1alpha2listers "k8s.io/client-go/listers/resource/v1alpha2"
	"k8s.io/dynamic-resource-allocation/simulation"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/test/e2e/dra/test-driver/app"
)

var _ simulation.Plugin = TestPlugin{}

type TestPlugin struct {
	Resources *app.Resources
}

func (p TestPlugin) Activate(ctx context.Context, client kubernetes.Interface, informerFactory informers.SharedInformerFactory) (simulation.ActivePlugin, error) {
	pa := activeTestPlugin{
		TestPlugin: p,
		// When simulation starts, we need to know which claims have already been allocated.
		claimLister: informerFactory.Resource().V1alpha2().ResourceClaims().Lister(),
	}

	return pa, nil
}

var _ simulation.ActivePlugin = activeTestPlugin{}

type activeTestPlugin struct {
	TestPlugin
	claimLister resourcev1alpha2listers.ResourceClaimLister
}

func (p activeTestPlugin) Start(ctx context.Context) (simulation.StartedPlugin, error) {
	ps := startedTestPlugin{
		activeTestPlugin: p,
		claimsPerNode:    make(map[string]int),
	}
	claims, err := p.claimLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing claims: %v", err)
	}
	for _, claim := range claims {
		if claim.Status.Allocation != nil &&
			claim.Status.DriverName == p.Resources.DriverName {
			if !p.Resources.NodeLocal {
				// Network-attached, no need to check where the claim is really allocated.
				ps.claimsPerNode[""]++
				continue
			}
			if len(claim.Status.Allocation.AvailableOnNodes.NodeSelectorTerms) != 1 {
				return nil, fmt.Errorf("expected one node selector term for claim %s, got %+v", klog.KObj(claim), claim.Status.Allocation.AvailableOnNodes)
			}
			selectorTerm := claim.Status.Allocation.AvailableOnNodes.NodeSelectorTerms[0]
			if len(selectorTerm.MatchExpressions) != 1 {
				return nil, fmt.Errorf("expected one match expression for claim %s, got %+v", klog.KObj(claim), selectorTerm)
			}
			matchExpression := selectorTerm.MatchExpressions[0]
			if matchExpression.Key != "kubernetes.io/hostname" ||
				matchExpression.Operator != v1.NodeSelectorOpIn ||
				len(matchExpression.Values) != 1 {
				return nil, fmt.Errorf("expected a match expression for claim %s with kubernetes.io/hostname as key for one node, got %+v", klog.KObj(claim), matchExpression)
			}
			ps.claimsPerNode[matchExpression.Values[0]]++
		}
	}
	return ps, nil
}

var _ simulation.StartedPlugin = startedTestPlugin{}

type startedTestPlugin struct {
	activeTestPlugin
	// claimsPerNode counts how many claims are currently allocated for a node (non-empty key)
	// or allocated for the entire cluster (empty key).
	claimsPerNode map[string]int
}

func (p startedTestPlugin) Clone() simulation.StartedPlugin {
	clone := p
	clone.claimsPerNode = make(map[string]int)
	for node, count := range p.claimsPerNode {
		clone.claimsPerNode[node] = count
	}
	return clone
}

func (p startedTestPlugin) NodeIsSuitable(ctx context.Context, pod *v1.Pod, claim *resourcev1alpha2.ResourceClaim, node *v1.Node) (bool, error) {
	// Available on that node?
	if len(p.Resources.NodeLabels) > 0 {
		selector := labels.SelectorFromSet(p.Resources.NodeLabels)
		if !selector.Matches(labels.Set(node.Labels)) {
			return false, nil
		}
	}
	if len(p.Resources.Nodes) > 0 {
		if !slices.Contains(p.Resources.Nodes, node.Name) {
			return false, nil
		}
	}

	// Can still be allocated?
	if p.Resources.MaxAllocations == 0 {
		// Unlimited.
		return true, nil
	}
	if p.Resources.NodeLocal {
		// Still space on the node?
		return p.claimsPerNode[node.Name] < p.Resources.MaxAllocations, nil
	}
	return p.claimsPerNode[""] < p.Resources.MaxAllocations, nil
}

func (p startedTestPlugin) Allocate(ctx context.Context, claim *resourcev1alpha2.ResourceClaim, node *v1.Node) (*resourcev1alpha2.AllocationResult, error) {
	nodeName := node.Name
	if !p.Resources.NodeLocal {
		nodeName = ""
	}

	if p.Resources.MaxAllocations > 0 &&
		p.claimsPerNode[nodeName] >= p.Resources.MaxAllocations {
		return nil, fmt.Errorf("already %d claims allocated for node %q", p.claimsPerNode[nodeName], nodeName)
	}

	p.claimsPerNode[nodeName]++
	allocation := p.Resources.NewAllocation(nodeName, nil)
	return allocation, nil
}

func (p startedTestPlugin) Deallocate(ctx context.Context, claim *resourcev1alpha2.ResourceClaim) error {
	if !p.Resources.NodeLocal {
		p.claimsPerNode[""]--
		return nil
	}

	// For node-local resources we need to determine the node name based on
	// the allocation.
	if claim.Status.Allocation == nil {
		return errors.New("cannot deallocate an unallocated claim")
	}
	if claim.Status.Allocation.AvailableOnNodes == nil {
		return errors.New("claim should have node selector")
	}
	selector := claim.Status.Allocation.AvailableOnNodes
	if len(selector.NodeSelectorTerms) != 1 {
		return errors.New("claim's node selector should have exactly one term")
	}
	term := selector.NodeSelectorTerms[0]
	if len(term.MatchExpressions) != 1 {
		return errors.New("claim's node selector should have exactly one match expression")
	}
	expr := term.MatchExpressions[0]
	if expr.Key != "kubernetes.io/hostname" ||
		expr.Operator != v1.NodeSelectorOpIn {
		return errors.New("claim's node selector should have hostname check")
	}
	if len(expr.Values) != 1 {
		return errors.New("claim's match expression should have exactly one value")
	}
	nodeName := expr.Values[0]
	p.claimsPerNode[nodeName]--
	return nil
}
