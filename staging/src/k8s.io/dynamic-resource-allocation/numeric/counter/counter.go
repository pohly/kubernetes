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

package counter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/dynamic-resource-allocation/apis/counter"
	counterv1alpha1 "k8s.io/dynamic-resource-allocation/apis/counter/v1alpha1"
	"k8s.io/dynamic-resource-allocation/builtincontroller"
	dracache "k8s.io/dynamic-resource-allocation/cache"
	counterinternal "k8s.io/dynamic-resource-allocation/internal/apis/counter"
	"k8s.io/dynamic-resource-allocation/internal/nilmap"
	"k8s.io/dynamic-resource-allocation/labels"
	"k8s.io/dynamic-resource-allocation/numeric/counter/internal"
	"k8s.io/klog/v2"
)

var _ builtincontroller.Controller = CounterController{}

var scheme = runtime.NewScheme()

var decoder *json.Serializer

func init() {
	counter.Install(scheme)
	decoder = json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, json.SerializerOptions{})
}

type CounterController struct {
}

func (c CounterController) ControllerName() string {
	return counterv1alpha1.GroupName
}

func (c CounterController) Activate(ctx context.Context, client kubernetes.Interface, informerFactory informers.SharedInformerFactory, genericListerFactory *dracache.GenericListerFactory) (finalC builtincontroller.ActiveController, finalErr error) {
	ac := &activeCounterController{
		genericListerFactory: genericListerFactory,
		state:                &internal.State{},
	}

	logger := klog.FromContext(ctx)
	logger = klog.LoggerWithName(logger, c.ControllerName())

	capacityInformer := informerFactory.Resource().V1alpha2().NodeResourceCapacities().Informer()
	capacityRegistration, err := capacityInformer.AddEventHandler(dracache.NewResourceEventHandlerFuncs(
		logger,
		ac.nodeResourceCapacityAdded,
		ac.nodeResourceCapacityUpdated,
		ac.nodeResourceCapacityRemoved,
	))
	if err != nil {
		return nil, fmt.Errorf("register node capacity event handlers: %v", err)
	}
	defer func() {
		r := recover()
		if finalErr != nil || r != nil {
			_ = capacityInformer.RemoveEventHandler(capacityRegistration)
		}
		if r != nil {
			panic(r)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = capacityInformer.RemoveEventHandler(capacityRegistration)
	}()

	// We need to monitor resource classes. For any class which declares that it
	// is handled by the counter controller, we need to start watching the parameter
	// type listed for the class and for claims using the class.
	classInformer := informerFactory.Resource().V1alpha2().ResourceClasses().Informer()
	classRegistration, err := classInformer.AddEventHandler(dracache.NewResourceEventHandlerFuncs(
		logger,
		ac.classAdded,
		ac.classUpdated,
		ac.classRemoved,
	))
	if err != nil {
		return nil, fmt.Errorf("register resource class event handlers: %v", err)
	}
	defer func() {
		r := recover()
		if finalErr != nil || r != nil {
			_ = classInformer.RemoveEventHandler(classRegistration)
		}
		if r != nil {
			panic(r)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = classInformer.RemoveEventHandler(classRegistration)

		// Also cancel all dynamically created informers.
		ac.mutex.Lock()
		defer ac.mutex.Unlock()
		for _, cInformers := range ac.informersPerClass {
			logger.V(3).Info("Cleaning up, stopping informers", "classParameterType", cInformers.classParameterGK, "claimParameterTypes", nilmap.Keys(cInformers.claimParameterTypes))
			cInformers.cleanup()
		}
		ac.informersPerClass = nil
	}()

	return ac, nil
}

var _ builtincontroller.ActiveController = &activeCounterController{}

type activeCounterController struct {
	CounterController
	genericListerFactory *dracache.GenericListerFactory

	mutex sync.Mutex
	state *internal.State

	// informersPerClass tracks classes for which informers were activated
	// and which parameters may be needed for them and their claims. Only
	// classes listed here are handled by this controller.
	informersPerClass map[types.UID]classInformers
}

// Before snapshotting, listers contains live (= updated by informers)
// listers for class or claim parameters. After snapshotting, the
// listers use a fixed store.
type classInformers struct {
	cancelFuncs []func()

	classParameterGK     schema.GroupKind
	classParameterLister cache.GenericLister
	claimParameterTypes  map[schema.GroupKind]claimParameterType
}

type claimParameterType struct {
	cache.GenericLister
	fieldPath []string
	shareable bool
}

func (ci *classInformers) cleanup() {
	for _, cancel := range ci.cancelFuncs {
		cancel()
	}
	ci.classParameterLister = nil
	ci.claimParameterTypes = nil
}

func (ci *classInformers) add(logger klog.Logger, factory *dracache.GenericListerFactory, class *resourcev1alpha2.ResourceClass, gk schema.GroupKind, fieldPath string, isClassParameterType bool, shareable bool) {
	ctx, cancel := context.WithCancel(context.Background())
	name := "ClaimParameterType"
	if isClassParameterType {
		name = "ClassParameterType"
	}
	ctx = klog.NewContext(ctx, klog.LoggerWithName(klog.LoggerWithValues(logger, "class", klog.KObj(class), "type", gk), name))
	logger.V(3).Info("Referencing parameter type", "class", klog.KObj(class), "type", gk)
	lister := factory.ForType(ctx, gk)
	ci.cancelFuncs = append(ci.cancelFuncs, cancel)

	if isClassParameterType {
		ci.classParameterGK = gk
		ci.classParameterLister = lister
	} else {
		nilmap.Insert(&ci.claimParameterTypes, gk, claimParameterType{
			GenericLister: lister,
			fieldPath:     strings.Split(fieldPath, "."),
			shareable:     shareable,
		})
	}
}

func (c *activeCounterController) nodeResourceCapacityAdded(logger klog.Logger, nodeResourceCapacity *resourcev1alpha2.NodeResourceCapacity) {
	c.nodeResourceCapacityAddedOrUpdated(logger, nodeResourceCapacity)
}

func (c *activeCounterController) nodeResourceCapacityUpdated(logger klog.Logger, oldNodeResourceCapacity, newNodeResourceCapacity *resourcev1alpha2.NodeResourceCapacity) {
	c.nodeResourceCapacityAddedOrUpdated(logger, newNodeResourceCapacity)
}

type instance struct {
	id                       string
	counterinternal.Capacity // Empty if deleted or unknown.
}

func (c *activeCounterController) nodeResourceCapacityRemoved(logger klog.Logger, nodeResourceCapacity *resourcev1alpha2.NodeResourceCapacity) {
	// Passing just the IDs with no data clears the corresponding entries.
	instances := make([]instance, len(nodeResourceCapacity.Instances))
	for i, instance := range nodeResourceCapacity.Instances {
		instances[i].id = instance.ID
	}
	c.updateCapacity(logger, "State updated after node capacity removal", nodeResourceCapacity, instances)

}

func (c *activeCounterController) nodeResourceCapacityAddedOrUpdated(logger klog.Logger, nodeResourceCapacity *resourcev1alpha2.NodeResourceCapacity) {
	instances := parseResourceInstances(logger, nodeResourceCapacity)
	c.updateCapacity(logger, "State updated after node capacity changed", nodeResourceCapacity, instances)
}

func (c *activeCounterController) updateCapacity(logger klog.Logger, what string, nodeResourceCapacity *resourcev1alpha2.NodeResourceCapacity, instances []instance) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	loggerV := logger.V(5)
	if loggerV.Enabled() {
		oldState := c.state.DeepCopy()
		defer func() {
			loggerV.Info(what, "state", c.state, "diff", cmp.Diff(oldState, c.state))
		}()
	}

	// Update capacity information. This must must create maps as needed
	// and prune unused entries at all levels to prevent memory leaks (like
	// keeping entries for obsolete nodes). This is done by:
	// - walking down into the maps as defined by node name, driver name, uid,
	// - update the leaf,
	// - and then walk back up.
	nodeResources := c.state.PerNode[nodeResourceCapacity.NodeName]
	driverResources := nodeResources.PerDriver[nodeResourceCapacity.DriverName]
	for _, fromInstance := range instances {
		instance := driverResources.PerInstance[fromInstance.id]

		// Usually the node capacity should not go away while it is in use. But
		// this is something to be checked elsewhere. Here we allow it and
		// keep entries with more resources allocated than available.
		if fromInstance.Count == 0 && instance.Allocated == 0 {
			nilmap.Delete(&driverResources.PerInstance, fromInstance.id)
		} else {
			// Only store information that is relevant.
			instance.Name = fromInstance.Name
			instance.Labels = fromInstance.Labels
			instance.Capacity = fromInstance.Count

			nilmap.Insert(&driverResources.PerInstance, fromInstance.id, instance)
		}
	}
	if len(driverResources.PerInstance) == 0 {
		nilmap.Delete(&nodeResources.PerDriver, nodeResourceCapacity.DriverName)
	} else {
		nilmap.Insert(&nodeResources.PerDriver, nodeResourceCapacity.DriverName, driverResources)
	}
	if len(nodeResources.PerDriver) == 0 {
		nilmap.Delete(&c.state.PerNode, nodeResourceCapacity.NodeName)
	} else {
		nilmap.Insert(&c.state.PerNode, nodeResourceCapacity.NodeName, nodeResources)
	}
}

func parseResourceInstances(logger klog.Logger, in *resourcev1alpha2.NodeResourceCapacity) []instance {
	instances := make([]instance, len(in.Instances))
	for i, fromInstance := range in.Instances {
		// We always have the ID, the capacity only if it is using the counter model.
		instances[i].id = fromInstance.ID

		// Logging always includes the same key/value pairs. Because in most cases
		// (no error, low log level) nothing gets logged, it doesn't make sense
		// to use klog.LoggerWithValues.

		// Unknown types are normal because each model receives all
		// NodeResourceCapacity objects, including those with capacity
		// handled by other models.
		if fromInstance.Kind != "Capacity" {
			logger.V(6).Info("Ignoring unsupported node capacity kind", "nodeResourceCapacity", klog.KObj(in), "node", klog.KRef("", in.NodeName), "driver", in.DriverName, "instanceID", fromInstance.ID, "kind", fromInstance.Kind)
			continue
		}
		gv, err := schema.ParseGroupVersion(fromInstance.APIVersion)
		if err != nil {
			// Should not happen for valid NodeResourceCapacity.
			logger.Error(err, "Parsing apiVersion", "nodeResourceCapacity", klog.KObj(in))
			continue
		}
		if gv.Group != counterv1alpha1.GroupName {
			logger.V(6).Info("Ignoring unsupported node capacity group", "nodeResourceCapacity", klog.KObj(in), "node", klog.KRef("", in.NodeName), "driver", in.DriverName, "instanceID", fromInstance.ID, "groupVersion", gv)
			continue
		}

		// The version doesn't matter as long as the scheme can convert
		// it to the internal type. It's passed into Decode in case that
		// the data itself doesn't have it (which is okay!).
		defaultGvk := gv.WithKind(fromInstance.Kind)
		actual, gvk, err := decoder.Decode(fromInstance.Data.Raw, &defaultGvk, nil)
		if err != nil {
			logger.Error(err, "Decoding node capacity failed", "nodeResourceCapacity", klog.KObj(in), "node", klog.KRef("", in.NodeName), "driver", in.DriverName, "instanceID", fromInstance.ID, "gvk", gvk)
			continue
		}
		if err := scheme.Convert(actual, &instances[i].Capacity, nil); err != nil {
			logger.Error(err, "Converting node capacity failed", "nodeResourceCapacity", klog.KObj(in), "node", klog.KRef("", in.NodeName), "driver", in.DriverName, "instanceID", fromInstance.ID)
		}
		logger.V(5).Info("Got node resource capacity", "nodeResourceCapacity", klog.KObj(in), "node", klog.KRef("", in.NodeName), "driver", in.DriverName, "instanceID", fromInstance.ID, "capacity", &instances[i].Capacity)
	}
	return instances
}

func (c *activeCounterController) ClaimAllocated(ctx context.Context, claim *resourcev1alpha2.ResourceClaim) {
	logger := klog.FromContext(ctx)

	if claim.Status.Allocation == nil /* Should have been checked by caller. */ ||
		len(claim.Status.Allocation.ResourceHandles) != 1 /* If allocated by this controller, it has one entry. */ {
		// Not a claim handled by this controller, ignore it.
		logger.V(5).Info("Ignoring unknown foreign allocation", "claim", klog.KObj(claim))
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if nilmap.Contains(c.state.Claims, claim.UID) {
		return
	}

	allocationResult := new(counterv1alpha1.AllocationResult)
	actual, gvk, err := decoder.Decode([]byte(claim.Status.Allocation.ResourceHandles[0].Data), nil, allocationResult)
	switch {
	case runtime.IsNotRegisteredError(err):
		logger.V(6).Info("Ignoring unknown resource handle data", "data", claim.Status.Allocation.ResourceHandles[0].Data, "claim", klog.KObj(claim), "err", err)
	case err != nil:
		logger.Error(err, "Decoding resource handle data failed", "claim", klog.KObj(claim))
	case actual != allocationResult:
		logger.Error(nil, "Invalid resource handle data, got unsupported object", "gvk", gvk, "obj", actual, "claim", klog.KObj(claim))
	default:
		loggerV := logger.V(5)
		if loggerV.Enabled() {
			oldState := c.state.DeepCopy()
			defer func() {
				loggerV.Info("State updated after new allocated claim encountered", "state", c.state, "diff", cmp.Diff(oldState, c.state))
			}()
		}

		nodeResources := c.state.PerNode[allocationResult.NodeName]
		driverResources := nodeResources.PerDriver[allocationResult.DriverName]
		instance := driverResources.PerInstance[allocationResult.InstanceID]
		instance.Allocated += allocationResult.Count
		nilmap.Insert(&driverResources.PerInstance, allocationResult.InstanceID, instance)
		nilmap.Insert(&nodeResources.PerDriver, allocationResult.DriverName, driverResources)
		nilmap.Insert(&c.state.PerNode, allocationResult.NodeName, nodeResources)
		claimResources := internal.ClaimResources{
			Name:       claim.Name,
			Namespace:  claim.Namespace,
			NodeName:   allocationResult.NodeName,
			DriverName: allocationResult.DriverName,
			InstanceID: allocationResult.InstanceID,
			Count:      allocationResult.Count,
		}
		nilmap.Insert(&c.state.Claims, claim.UID, claimResources)
	}
}

func (c *activeCounterController) ClaimDeallocated(ctx context.Context, claim *resourcev1alpha2.ResourceClaim) {
	c.deallocate(ctx, claim, "State updated after new deallocated claim encountered")
}

func (c *activeCounterController) deallocate(ctx context.Context, claim *resourcev1alpha2.ResourceClaim, msg string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	claimResources, ok := c.state.Claims[claim.UID]
	if !ok {
		return
	}

	logger := klog.FromContext(ctx)
	loggerV := logger.V(5)
	if loggerV.Enabled() {
		oldState := c.state.DeepCopy()
		defer func() {
			loggerV.Info(msg, "state", c.state, "diff", cmp.Diff(oldState, c.state))
		}()
	}

	// Reduce allocated capacity. This may lead to empty leafs and thus
	// pruning.
	nodeResources := c.state.PerNode[claimResources.NodeName]
	driverResources := nodeResources.PerDriver[claimResources.DriverName]
	instance := driverResources.PerInstance[claimResources.InstanceID]
	instance.Allocated -= claimResources.Count
	if instance.Allocated == 0 && instance.Capacity == 0 {
		nilmap.Delete(&driverResources.PerInstance, claimResources.InstanceID)
	} else {
		nilmap.Insert(&driverResources.PerInstance, claimResources.InstanceID, instance)
	}
	if len(driverResources.PerInstance) == 0 {
		nilmap.Delete(&nodeResources.PerDriver, claimResources.DriverName)
	} else {
		nilmap.Insert(&nodeResources.PerDriver, claimResources.DriverName, driverResources)
	}
	if len(nodeResources.PerDriver) == 0 {
		nilmap.Delete(&c.state.PerNode, claimResources.NodeName)
	} else {
		nilmap.Insert(&c.state.PerNode, claimResources.NodeName, nodeResources)
	}

	delete(c.state.Claims, claim.UID)
}

func (c *activeCounterController) classAdded(logger klog.Logger, class *resourcev1alpha2.ResourceClass) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	logger = klog.LoggerWithName(logger, "ClassAdded")
	c.classAddedAlreadyLocked(logger, class)
}

func (c *activeCounterController) classUpdated(logger klog.Logger, oldClass, newClass *resourcev1alpha2.ResourceClass) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// First remove all old informers, recreate them, and then clean up.
	// The informer factory takes care of reusing informers that continue
	// to be used.
	logger = klog.LoggerWithName(logger, "ClassUpdated")
	cInformers, ok := c.informersPerClass[newClass.UID]
	delete(c.informersPerClass, newClass.UID)
	c.classAddedAlreadyLocked(logger, newClass)
	if ok {
		logger.V(3).Info("Dropping reference to out-dated informers", "class", klog.KObj(newClass), "classParameterType", cInformers.classParameterGK, "claimParameterTypes", nilmap.Keys(cInformers.claimParameterTypes))
		cInformers.cleanup()
	}
}

func (c *activeCounterController) classAddedAlreadyLocked(logger klog.Logger, class *resourcev1alpha2.ResourceClass) {
	cInformers, ok := c.informersPerClass[class.UID]
	if ok {
		// Shouldn't happen, but let's be careful and clean up if it does.
		logger.V(3).Info("Dropping reference to old informers", "class", klog.KObj(class), "classParameterType", cInformers.classParameterGK, "claimParameterTypes", nilmap.Keys(cInformers.claimParameterTypes))
		cInformers.cleanup()
	}
	delete(c.informersPerClass, class.UID)
	if len(class.NumericParameters) == 0 {
		// Nothing to do for the class.
		return
	}

	cInformers = classInformers{}
	if class.ParametersRef != nil {
		gk := schema.GroupKind{Group: class.ParametersRef.APIGroup, Kind: class.ParametersRef.Kind}
		cInformers.add(logger, c.genericListerFactory, class, gk, "", true, false)
	}

	for _, parameterType := range class.NumericParameters {
		gk := schema.GroupKind{Group: parameterType.APIGroup, Kind: parameterType.Kind}
		cInformers.add(logger, c.genericListerFactory, class, gk, parameterType.FieldPath, false, parameterType.Shareable)
	}

	nilmap.Insert(&c.informersPerClass, class.UID, cInformers)
}

func (c *activeCounterController) classRemoved(logger klog.Logger, class *resourcev1alpha2.ResourceClass) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cInformers, ok := c.informersPerClass[class.UID]
	if !ok {
		return
	}
	logger = klog.LoggerWithName(logger, "ClassRemoved")
	logger.V(3).Info("Stopping informers", "class", klog.KObj(class), "classParameterType", cInformers.classParameterGK, "claimParameterTypes", nilmap.Keys(cInformers.claimParameterTypes))
	cInformers.cleanup()
	delete(c.informersPerClass, class.UID)
}

// Snapshot is implemented by doing a deep copy of the state and snapshotting
// all active stores.
func (c *activeCounterController) Snapshot() builtincontroller.ActiveController {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// TODO (?): snapshot claim and class listers
	return &activeCounterController{
		state: c.state.DeepCopy(),
	}
}

func (c *activeCounterController) HandlesClaim(ctx context.Context, claim *resourcev1alpha2.ResourceClaim, class *resourcev1alpha2.ResourceClass) (builtincontroller.ClaimController, error) {
	logger := klog.FromContext(ctx)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if claim.Status.Allocation != nil {
		// We should have seen this claim in ClaimAllocated or recorded its allocation.
		// If not, then we are not responsible for it.
		claimResources, ok := c.state.Claims[claim.UID]
		if !ok {
			logger.V(6).Info("Claim not handled because the claim is allocated and unknown, so it must be from someone else", "controller", c.ControllerName(), "claim", klog.KObj(claim))
			return nil, nil
		}
		return &claimCounterController{
			activeCounterController: c,
			claim:                   claim,
			claimResources:          &claimResources,
		}, nil
	}

	// Support for the class gets checked when it is first seen in the
	// informer callback. If we haven't seen it yet, then we return nil
	// incorrectly here. That is okay: the caller has to detect that some
	// builtin controller should have handled the claim and needs to retry
	// later.
	cInformers, ok := c.informersPerClass[class.UID]
	if !ok {
		return nil, nil
	}

	// For allocating, we need class and claim parameters. It's an error if
	// we should handle the claim (according to the class) and don't have
	// them. Because this might be called by the cluster autoscaler and for
	// performance reasons, we rely entirely on caches.
	var classParameters runtime.Object
	if class.ParametersRef != nil {
		gk := schema.GroupKind{Group: class.ParametersRef.APIGroup, Kind: class.ParametersRef.Kind}
		if gk != cInformers.classParameterGK {
			return nil, fmt.Errorf("class %q references a class parameter type %q, expected %q", klog.KObj(class), gk, cInformers.classParameterGK)
		}
		lister := cInformers.classParameterLister
		var err error
		if class.ParametersRef.Namespace != "" {
			lister := lister.ByNamespace(class.ParametersRef.Namespace)
			classParameters, err = lister.Get(class.ParametersRef.Name)
		} else {
			classParameters, err = lister.Get(class.ParametersRef.Name)
		}
		if err != nil {
			return nil, fmt.Errorf("could not retrieve class parameter %q of type %q for class %q: %v", klog.KRef(class.ParametersRef.Namespace, class.ParametersRef.Name), gk, klog.KObj(class), err)
		}
	}

	if claim.Spec.ParametersRef == nil {
		return nil, fmt.Errorf("claim %q has no parameter reference", klog.KObj(claim))
	}
	gk := schema.GroupKind{Group: claim.Spec.ParametersRef.APIGroup, Kind: claim.Spec.ParametersRef.Kind}
	claimParameterType, ok := cInformers.claimParameterTypes[gk]
	if !ok {
		return nil, fmt.Errorf("claim %q references a claim parameter type %q which is not among the numeric parameter types for class %q (%q)", klog.KObj(claim), gk, klog.KObj(class), nilmap.Keys(cInformers.claimParameterTypes))
	}
	claimParameters, err := claimParameterType.ByNamespace(claim.Namespace).Get(claim.Spec.ParametersRef.Name)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve claim parameter %q of type %q for claim %q: %q", klog.KRef(claim.Namespace, claim.Spec.ParametersRef.Name), gk, klog.KObj(claim), err)
	}

	// A class might list multiple alternative types. Now that we have the actual object,
	// we need to check whether it contains parameters that are handled by this controller.
	claimParametersObj, ok := claimParameters.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("internal error, expected unstructured.Unstructured as claim parameter object, got %T", claimParameters)
	}
	parameters, ok, err := unstructured.NestedMap(claimParametersObj.Object, claimParameterType.fieldPath...)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve numeric parameters from claim %q at field %q: %v", klog.KRef(claim.Namespace, claim.Spec.ParametersRef.Name), strings.Join(claimParameterType.fieldPath, "."), err)
	}
	if !ok {
		return nil, fmt.Errorf("claim %q does not have numeric parameter at field %q", klog.KRef(claim.Namespace, claim.Spec.ParametersRef.Name), strings.Join(claimParameterType.fieldPath, "."))
	}
	numericParametersObj := unstructured.Unstructured{Object: parameters}
	gvk := numericParametersObj.GetObjectKind().GroupVersionKind()
	if gvk.Group != counterv1alpha1.SchemeGroupVersion.Group ||
		gvk.Kind != "Parameters" {
		// Some unsupported type.
		logger.V(6).Info("Claim not handled because the embedded parameter type is unknown", "controller", c.ControllerName(), "claim", klog.KObj(claim), "parameterType", gvk)
		return nil, nil
	}
	buffer, err := numericParametersObj.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("encoding %q from claim %q failed: %v", gvk, klog.KRef(claim.Namespace, claim.Spec.ParametersRef.Name), err)
	}
	var numericClaimParameters counterv1alpha1.Parameters
	actual, actualGVK, err := decoder.Decode(buffer, nil, &numericClaimParameters)
	if err != nil {
		return nil, fmt.Errorf("decoding of %q from claim %q failed: %v", gvk, klog.KObj(claim), err)
	}
	if actual != &numericClaimParameters {
		return nil, fmt.Errorf("claim %q: internal error: expected claim parameter type %q, got got unsupported object of type %q", klog.KObj(claim), gvk, actualGVK)
	}
	labelMatcher, err := labels.Compile(&numericClaimParameters.Selector)
	if err != nil {
		// This can happen if the user populated the claim parameters
		// with some invalid label selector. The error message must be
		// informative to tell them exactly what was wrong where.
		return nil, fmt.Errorf("claim %q: parameters %q: %s: %v", klog.KObj(claim), claim.Spec.ParametersRef.Name, claimParameterType.fieldPath, err)
	}

	// Now we have enough information to check where the claim might be allocated.
	return &claimCounterController{
		activeCounterController: c,
		claim:                   claim,

		driverName:             class.DriverName,
		shareable:              claimParameterType.shareable,
		classParameters:        classParameters,
		claimParameters:        claimParameters,
		numericClaimParameters: numericClaimParameters,
		labelMatcher:           labelMatcher,
	}, nil
}

type claimCounterController struct {
	*activeCounterController

	claim                            *resourcev1alpha2.ResourceClaim
	claimResources                   *internal.ClaimResources
	driverName                       string
	shareable                        bool
	classParameters, claimParameters runtime.Object
	numericClaimParameters           counterv1alpha1.Parameters
	labelMatcher                     *labels.Matcher
}

var _ builtincontroller.ClaimController = &claimCounterController{}

func (c *claimCounterController) NodeIsSuitable(ctx context.Context, pod *v1.Pod, node *v1.Node) (bool, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, instance := range c.state.PerNode[node.Name].PerDriver[c.driverName].PerInstance {
		if instance.Allocated+c.numericClaimParameters.Count <= instance.Capacity {
			matches, err := c.labelMatcher.Matches(instance.Labels)
			return matches, err
		}
	}
	return false, nil
}

func (c *claimCounterController) Allocate(ctx context.Context, nodeName string) (string, *resourcev1alpha2.AllocationResult, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	logger := klog.FromContext(ctx)
	loggerV := logger.V(5)
	if loggerV.Enabled() {
		oldState := c.state.DeepCopy()
		defer func() {
			loggerV.Info("State updated after allocating claim", "state", c.state, "claim", klog.KObj(c.claim), "diff", cmp.Diff(oldState, c.state))
		}()
	}

	count := c.numericClaimParameters.Count
	nodeResources := c.state.PerNode[nodeName]
	driverResources := nodeResources.PerDriver[c.driverName]
	for instanceID, instance := range driverResources.PerInstance {
		if instance.Allocated+count <= instance.Capacity {
			result := counterv1alpha1.AllocationResult{
				ClassParameters: runtime.RawExtension{Object: c.classParameters},
				ClaimParameters: runtime.RawExtension{Object: c.claimParameters},
				NodeName:        nodeName,
				DriverName:      c.driverName,
				InstanceID:      instanceID,
				Count:           count,
			}
			var buffer bytes.Buffer
			if err := decoder.Encode(&result, &buffer); err != nil {
				return "", nil, fmt.Errorf("failed to encode counter allocation result: %v", err)
			}
			instance.Allocated += count
			driverResources.PerInstance[instanceID] = instance
			c.claimResources = &internal.ClaimResources{
				Name:       c.claim.Name,
				Namespace:  c.claim.Namespace,
				NodeName:   nodeName,
				DriverName: c.driverName,
				InstanceID: instanceID,
				Count:      count,
			}
			nilmap.Insert(&c.state.Claims, c.claim.UID, *c.claimResources)

			return c.driverName, &resourcev1alpha2.AllocationResult{
				ResourceHandles: []resourcev1alpha2.ResourceHandle{{
					DriverName: c.driverName,
					Data:       buffer.String(),
				}},
				AvailableOnNodes: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{nodeName},
								},
							},
						},
					},
				},
				Shareable: c.shareable,
			}, nil
		}
	}

	// This should have been checked before, but perhaps someone else was faster or
	// the capacity changed.
	return "", nil, errors.New("capacity exhausted")
}

func (c *claimCounterController) Deallocate(ctx context.Context) {
	c.deallocate(ctx, c.claim, "State updated after claim was deallocated")
}
