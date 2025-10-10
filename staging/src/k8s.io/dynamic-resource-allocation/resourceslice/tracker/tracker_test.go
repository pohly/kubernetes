/*
Copyright 2025 The Kubernetes Authors.

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

package tracker

import (
	stdcmp "cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	resourcealphaapi "k8s.io/api/resource/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	draapi "k8s.io/dynamic-resource-allocation/api"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/ktesting"
	_ "k8s.io/klog/v2/ktesting/init"
	"k8s.io/utils/ptr"
)

type handlerEventType string

const (
	handlerEventAdd    handlerEventType = "add"
	handlerEventUpdate handlerEventType = "update"
	handlerEventDelete handlerEventType = "delete"
)

type handlerEvent struct {
	event  handlerEventType
	oldObj *draapi.ResourceSlice
	newObj *draapi.ResourceSlice
}

func add[T any](obj *T) [2]*T {
	return [2]*T{nil, obj}
}

func remove[T any](obj *T) [2]*T {
	return [2]*T{obj, nil}
}

func update[T any](oldObj, newObj *T) [2]*T {
	return [2]*T{oldObj, newObj}
}

func runInputEvents(tCtx *testContext, events []any, permutation []int) {
	for _, i := range permutation {
		event, name := lookupEvent(events, i)
		stepCtx := tCtx.withLoggerName(fmt.Sprintf("event #%s", name))
		applyEventPair(stepCtx, event)
	}
}

// lookupEvent is the opposite of flatten: it takes an index
// after flattening and maps it back to the event in the original
// event hierarchy. To do so, it descends into the second level where necessary.
func lookupEvent(events []any, index int) (any, string) {
	numEvents := 0
	for i := range events {
		if e, ok := events[i].([]any); ok {
			for j := range e {
				if numEvents == index {
					return e[j], fmt.Sprintf("%d/%d", i, j)
				}
				numEvents++
			}
		} else {
			if numEvents == index {
				return events[i], fmt.Sprintf("%d", i)
			}
			numEvents++
		}
	}
	panic(fmt.Sprintf("invalid event index #%d", index))
}

func applyEventPair(tCtx *testContext, event any) {
	switch pair := event.(type) {
	case [2]*draapi.ResourceSlice:
		store := tCtx.resourceSlices.GetStore()
		switch {
		case pair[0] != nil && pair[1] != nil:
			err := store.Update(pair[1])
			require.NoError(tCtx, err)
			tCtx.resourceSliceUpdate(tCtx.Context)(pair[0], pair[1])
		case pair[0] != nil:
			err := store.Delete(pair[0])
			require.NoError(tCtx, err)
			tCtx.resourceSliceDelete(tCtx.Context)(pair[0])
		default:
			err := store.Add(pair[1])
			require.NoError(tCtx, err)
			tCtx.resourceSliceAdd(tCtx.Context)(pair[1])
		}
	case [2]*resourcealphaapi.DeviceTaintRule:
		store := tCtx.deviceTaints.GetStore()
		switch {
		case pair[0] != nil && pair[1] != nil:
			err := store.Update(pair[1])
			require.NoError(tCtx, err)
			tCtx.deviceTaintUpdate(tCtx.Context)(pair[0], pair[1])
		case pair[0] != nil:
			err := store.Delete(pair[0])
			require.NoError(tCtx, err)
			tCtx.deviceTaintDelete(tCtx.Context)(pair[0])
		default:
			err := store.Add(pair[1])
			require.NoError(tCtx, err)
			tCtx.deviceTaintAdd(tCtx.Context)(pair[1])
		}
	case [2]*resourceapi.DeviceClass:
		store := tCtx.deviceClasses.GetStore()
		switch {
		case pair[0] != nil && pair[1] != nil:
			err := store.Update(pair[1])
			require.NoError(tCtx, err)
			tCtx.deviceClassUpdate(tCtx.Context)(pair[0], pair[1])
		case pair[0] != nil:
			err := store.Delete(pair[0])
			require.NoError(tCtx, err)
			tCtx.deviceClassDelete(tCtx.Context)(pair[0])
		default:
			err := store.Add(pair[1])
			require.NoError(tCtx, err)
			tCtx.deviceClassAdd(tCtx.Context)(pair[1])
		}
	}
}

type testContext struct {
	*testing.T
	context.Context
	*Tracker
	*fake.Clientset
}

func (t *testContext) withLoggerName(name string) *testContext {
	logger := klog.FromContext(t.Context)
	logger = klog.LoggerWithName(logger, name)
	t = &testContext{
		T:         t.T,
		Context:   klog.NewContext(t.Context, logger),
		Tracker:   t.Tracker,
		Clientset: t.Clientset,
	}
	return t
}

// m contains parameters for taintMeta.
type m map[struct {
	deviceName string
	taintIndex int
}]struct {
	id   int
	rule *resourcealphaapi.DeviceTaintRule
}

var (
	// Alias to save typing.
	u = draapi.MakeUniqueString

	now, _      = time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
	driver1     = "driver1.example.com"
	driver2     = "driver2.example.com"
	pool1       = "pool-1"
	pool2       = "pool-2"
	device0Name = "device-0"
	device1Name = "device-1"
	device2Name = "device-2"

	deviceClass1 = &resourceapi.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "device-class-1"},
		Spec: resourceapi.DeviceClassSpec{
			Selectors: []resourceapi.DeviceSelector{
				{
					CEL: &resourceapi.CELDeviceSelector{
						Expression: `device.driver == "` + driver1 + `"`,
					},
				},
			},
		},
	}

	sliceWithDevices = func(slice *draapi.ResourceSlice, devices []draapi.Device) *draapi.ResourceSlice {
		slice = slice.DeepCopy()
		slice.Spec.Devices = devices
		return slice
	}
	sliceWithLabels = func(slice *draapi.ResourceSlice, labels map[string]string) *draapi.ResourceSlice {
		slice = slice.DeepCopy()
		slice.Labels = labels
		return slice
	}
	slice1NoDevices = &draapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "s1",
		},
		Spec: draapi.ResourceSliceSpec{
			Driver: u(driver1),
			Pool: draapi.ResourcePool{
				Name: u(pool1),
			},
		},
	}
	slice2NoDevices = &draapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "s2",
		},
		Spec: draapi.ResourceSliceSpec{
			Driver: u(driver2),
			Pool: draapi.ResourcePool{
				Name: u(pool2),
			},
		},
	}
	unchangedSlice = &draapi.ResourceSlice{ObjectMeta: metav1.ObjectMeta{Name: "no-change"}}

	deviceWithName = func(device draapi.Device, name string) draapi.Device {
		device.Name = u(name)
		return device
	}
	deviceWithTaints = func(device draapi.Device, taints []resourceapi.DeviceTaint) draapi.Device {
		device = *device.DeepCopy()
		for _, taint := range taints {
			device.Taints = append(device.Taints, draapi.TrackedDeviceTaint{DeviceTaint: taint})
		}
		return device
	}
	emptyDevice = draapi.Device{}
	device0     = deviceWithName(emptyDevice, device0Name)
	device1     = deviceWithName(emptyDevice, device1Name)
	device2     = deviceWithName(emptyDevice, device2Name)

	deviceTaint1 = resourceapi.DeviceTaint{
		Key:       "example.com/taint",
		Value:     "tainted",
		Effect:    resourceapi.DeviceTaintEffectNoExecute,
		TimeAdded: &metav1.Time{Time: now},
	}
	deviceTaint2 = resourceapi.DeviceTaint{
		Key:       "example.com/taint2",
		Value:     "tainted2",
		Effect:    resourceapi.DeviceTaintEffectNoExecute,
		TimeAdded: &metav1.Time{Time: now},
	}
	deviceTaints   = []resourceapi.DeviceTaint{deviceTaint1}
	device1Tainted = deviceWithTaints(device1, deviceTaints)
	device2Tainted = deviceWithTaints(device2, deviceTaints)
	devices        = []draapi.Device{device1}
	threeDevices   = []draapi.Device{
		device0,
		device1,
		device2,
	}
	threeDevicesOneTainted = []draapi.Device{
		device0,
		device1Tainted,
		device2,
	}
	devices2        = []draapi.Device{device2}
	taintedDevices  = []draapi.Device{device1Tainted}
	taintedDevices2 = []draapi.Device{device2Tainted}

	existingDeviceTaints   = []resourceapi.DeviceTaint{deviceTaint2}
	existingDevice1Tainted = deviceWithTaints(device1, existingDeviceTaints)
	existingTaintedDevices = []draapi.Device{existingDevice1Tainted}
	mergedDeviceTaints     = []resourceapi.DeviceTaint{deviceTaint2, deviceTaint1}
	mergedDevice1Tainted   = deviceWithTaints(device1, mergedDeviceTaints)
	mergedTaintedDevices   = []draapi.Device{mergedDevice1Tainted}

	// Tainted slices contain devices with taints, but without ID and rule.
	// Test cases have to specify those separately through taintMeta.
	slice1               = sliceWithDevices(slice1NoDevices, devices)
	slice1Tainted        = sliceWithDevices(slice1NoDevices, taintedDevices)
	slice1AlreadyTainted = sliceWithDevices(slice1NoDevices, existingTaintedDevices)
	slice1MergedTaints   = sliceWithDevices(slice1NoDevices, mergedTaintedDevices)
	slice1Labels         = sliceWithLabels(slice1, map[string]string{"foo": "bar"})
	slice2               = sliceWithDevices(slice2NoDevices, devices2)
	slice2Tainted        = sliceWithDevices(slice2NoDevices, taintedDevices2)

	taintMeta = func(slice *draapi.ResourceSlice, meta m) *draapi.ResourceSlice {
		slice = slice.DeepCopy()
	nextMeta:
		for k, v := range meta {
			for i, device := range slice.Spec.Devices {
				if device.Name.String() == k.deviceName {
					if k.taintIndex >= len(device.Taints) {
						panic(fmt.Sprintf("device taint %#v does not exist in slice", k))
					}
					slice.Spec.Devices[i].Taints[k.taintIndex].ID = draapi.DeviceTaintID(v.id)
					slice.Spec.Devices[i].Taints[k.taintIndex].Rule = v.rule
					continue nextMeta
				}
			}
			panic(fmt.Sprintf("device %s does not exist in slice", k.deviceName))
		}
		return slice
	}

	alphaDeviceTaint = func(taint resourceapi.DeviceTaint) resourcealphaapi.DeviceTaint {
		return resourcealphaapi.DeviceTaint{
			Key:       taint.Key,
			Value:     taint.Value,
			Effect:    resourcealphaapi.DeviceTaintEffect(taint.Effect),
			TimeAdded: taint.TimeAdded,
		}
	}
	taintAllDevicesRule = &resourcealphaapi.DeviceTaintRule{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rule",
		},
		Spec: resourcealphaapi.DeviceTaintRuleSpec{
			Taint: alphaDeviceTaint(deviceTaint1),
		},
	}
	taintPoolDevicesRule = func(rule *resourcealphaapi.DeviceTaintRule, pool string) *resourcealphaapi.DeviceTaintRule {
		rule = rule.DeepCopy()
		rule.Spec.DeviceSelector = &resourcealphaapi.DeviceTaintSelector{
			Pool: &pool,
		}
		return rule
	}
	taintDriverDevicesRule = func(rule *resourcealphaapi.DeviceTaintRule, driver string) *resourcealphaapi.DeviceTaintRule {
		rule = rule.DeepCopy()
		rule.Spec.DeviceSelector = &resourcealphaapi.DeviceTaintSelector{
			Driver: &driver,
		}
		return rule
	}
	taintNamedDevicesRule = func(rule *resourcealphaapi.DeviceTaintRule, name string) *resourcealphaapi.DeviceTaintRule {
		rule = rule.DeepCopy()
		rule.Spec.DeviceSelector = &resourcealphaapi.DeviceTaintSelector{
			Device: &name,
		}
		return rule
	}
	taintCELSelectedDevicesRule = func(rule *resourcealphaapi.DeviceTaintRule, exprs ...string) *resourcealphaapi.DeviceTaintRule {
		rule = rule.DeepCopy()
		var selectors []resourcealphaapi.DeviceSelector
		for _, expr := range exprs {
			selectors = append(selectors, resourcealphaapi.DeviceSelector{
				CEL: &resourcealphaapi.CELDeviceSelector{
					Expression: expr,
				},
			})
		}
		rule.Spec.DeviceSelector = &resourcealphaapi.DeviceTaintSelector{
			Selectors: selectors,
		}
		return rule
	}
	taintDeviceClassRule = func(rule *resourcealphaapi.DeviceTaintRule, deviceClassName string) *resourcealphaapi.DeviceTaintRule {
		rule = rule.DeepCopy()
		rule.Spec.DeviceSelector = &resourcealphaapi.DeviceTaintSelector{
			DeviceClassName: &deviceClassName,
		}
		return rule
	}
	taintPool1DevicesRule             = taintPoolDevicesRule(taintAllDevicesRule, pool1)
	taintPool2DevicesRule             = taintPoolDevicesRule(taintAllDevicesRule, pool2)
	taintDriver1DevicesRule           = taintDriverDevicesRule(taintAllDevicesRule, driver1)
	taintDevice1Rule                  = taintNamedDevicesRule(taintAllDevicesRule, device1Name)
	taintDriver1DevicesCELRule        = taintCELSelectedDevicesRule(taintAllDevicesRule, `device.driver == "`+driver1+`"`)
	taintNoDevicesCELRule             = taintCELSelectedDevicesRule(taintAllDevicesRule, `true`, `false`, `true`)
	taintNoDevicesCELRuntimeErrorRule = taintCELSelectedDevicesRule(taintAllDevicesRule, `device.attributes["test.example.com"].deviceAttr`)
	taintNoDevicesInvalidCELRule      = taintCELSelectedDevicesRule(taintAllDevicesRule, `invalid`)
	taintDeviceClass1Rule             = taintDeviceClassRule(taintAllDevicesRule, deviceClass1.Name)
	taintDeviceAllCriteria            = taintDeviceClassRule(
		taintDriverDevicesRule(
			taintPoolDevicesRule(
				taintNamedDevicesRule(
					taintCELSelectedDevicesRule(
						taintAllDevicesRule,
						`true`,
					),
					device1Name,
				),
				pool1,
			),
			driver1,
		),
		deviceClass1.Name,
	)
)

func TestListPatchedResourceSlices(t *testing.T) {
	type test struct {
		// events contains pairs of old and new objects which will
		// be passed to event handler methods.
		// Objects can be slices, device taint rules, and device
		// classes.
		// [add], [remove], and [update] can be used to produce
		// such pairs.
		//
		// Alternatively, it can also contain a list of such pairs.
		// Those will be applied in the order in which they appear
		// in each event entry, but not necessarily in consecutive
		// order. Other events may be placed in between as long as
		// the order in those nested lists is preserved.
		events                []any
		expectedPatchedSlices []*draapi.ResourceSlice
		// The exact events that are emitted for a sequence of events is
		// highly dependent on the order in which those events are received.
		// We punt on determining a set of validation criteria for every
		// possible sequence and only check them against the first
		// permutation: the order in which the events are defined.
		//
		// The same applies to taint IDs: for example, if a taint rule
		// first applies to one slice, then to another after an update
		// ("update-patch" test case), then depending on the order
		// a taint gets added only once ot twice, leading to different IDs.
		// Therefore taint meta data as specified below is ignored except
		// for the original order of events.
		expectedHandlerEvents []handlerEvent
		expectEvents          func(t *assert.CollectT, events *v1.EventList)
		expectUnhandledErrors func(t *testing.T, errs []error)
	}
	tests := map[string]test{
		"add-slices-no-patches": {
			events: []any{
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				slice1,
				slice2,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1},
				{event: handlerEventAdd, newObj: slice2},
			},
		},
		"update-slices-no-patches": {
			events: []any{
				[]any{
					add(slice1NoDevices),
					update(slice1NoDevices, slice1),
				},
				[]any{
					add(slice2NoDevices),
					update(slice2NoDevices, slice2),
				},
				add(unchangedSlice),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				slice1,
				slice2,
				unchangedSlice,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1NoDevices},
				{event: handlerEventUpdate, oldObj: slice1NoDevices, newObj: slice1},
				{event: handlerEventAdd, newObj: slice2NoDevices},
				{event: handlerEventUpdate, oldObj: slice2NoDevices, newObj: slice2},
				{event: handlerEventAdd, newObj: unchangedSlice},
			},
		},
		"update-slice-labels": {
			events: []any{
				[]any{
					add(slice1),
					update(slice1, slice1Labels),
				},
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				slice1Labels,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1},
				{event: handlerEventUpdate, oldObj: slice1, newObj: slice1Labels},
			},
		},
		"delete-slices": {
			events: []any{
				[]any{add(slice1), remove(slice1)},
				[]any{add(slice2), remove(slice2)},
				add(unchangedSlice),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				unchangedSlice,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1},
				{event: handlerEventDelete, oldObj: slice1},
				{event: handlerEventAdd, newObj: slice2},
				{event: handlerEventDelete, oldObj: slice2},
				{event: handlerEventAdd, newObj: unchangedSlice},
			},
		},
		"patch-all-slices": {
			events: []any{
				add(slice1),
				add(taintAllDevicesRule),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintAllDevicesRule}}),
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1},
				{event: handlerEventUpdate, oldObj: slice1, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintAllDevicesRule}})},
			},
		},
		"update-patch": {
			events: []any{
				[]any{
					add(taintPool1DevicesRule),
					update(taintPool1DevicesRule, taintPool2DevicesRule),
				},
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				slice1,
				taintMeta(slice2Tainted, m{{device2Name, 0}: {1, taintPool2DevicesRule}}),
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1},
				{event: handlerEventAdd, newObj: taintMeta(slice2Tainted, m{{device2Name, 0}: {1, taintPool2DevicesRule}})},
			},
		},
		"merge-taints": {
			events: []any{
				add(taintAllDevicesRule),
				add(slice1AlreadyTainted),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1MergedTaints, m{{device1Name, 0}: {0, nil} /* from pool */, {device1Name, 1}: {1, taintAllDevicesRule}}),
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1MergedTaints, m{{device1Name, 0}: {0, nil} /* from pool */, {device1Name, 1}: {1, taintAllDevicesRule}})},
			},
		},
		// TODO:
		// - different pool generations in parallel
		// - different pools
		// - multiple slices with taints
		// - multiple taints per slice
		// - multiple taints per device

		"add-taint-for-driver": {
			events: []any{
				add(taintDriver1DevicesRule),
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDriver1DevicesRule}}),
				slice2,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDriver1DevicesRule}})},
				{event: handlerEventAdd, newObj: slice2},
			},
		},
		"add-taint-for-pool": {
			events: []any{
				add(taintPool1DevicesRule),
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintPool1DevicesRule}}),
				slice2,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintPool1DevicesRule}})},
				{event: handlerEventAdd, newObj: slice2},
			},
		},
		"add-taint-for-device": {
			events: []any{
				add(taintDevice1Rule),
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDevice1Rule}}),
				slice2,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDevice1Rule}})},
				{event: handlerEventAdd, newObj: slice2},
			},
		},
		"add-attribute-for-selector": {
			events: []any{
				add(taintDriver1DevicesCELRule),
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDriver1DevicesCELRule}}),
				slice2,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDriver1DevicesCELRule}})},
				{event: handlerEventAdd, newObj: slice2},
			},
		},
		"selector-does-not-match": {
			events: []any{
				add(taintNoDevicesCELRule),
				add(slice1),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				slice1,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1},
			},
		},
		"runtime-CEL-errors-skip-devices": {
			events: []any{
				add(taintNoDevicesCELRuntimeErrorRule),
				add(slice1),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				slice1,
			},
			expectEvents: func(t *assert.CollectT, events *v1.EventList) {
				if !assert.Len(t, events.Items, 1) {
					return
				}
				assert.Equal(t, taintNoDevicesCELRuntimeErrorRule.Name, events.Items[0].InvolvedObject.Name)
				assert.Equal(t, "CELRuntimeError", events.Items[0].Reason)
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: slice1},
			},
		},
		"invalid-CEL-expression-throws-error": {
			events: []any{
				[]any{
					add(taintNoDevicesInvalidCELRule),
					add(slice1),
				},
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{},
			expectUnhandledErrors: func(t *testing.T, errs []error) {
				if !assert.Len(t, errs, 1) {
					return
				}
				assert.ErrorContains(t, errs[0], "CEL compile error")
			},
		},
		"add-taint-for-device-class": {
			events: []any{
				add(deviceClass1),
				add(taintDeviceClass1Rule),
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDeviceClass1Rule}}),
				slice2,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDeviceClass1Rule}})},
				{event: handlerEventAdd, newObj: slice2},
			},
		},
		"filter-all-criteria": {
			events: []any{
				add(deviceClass1),
				add(taintDeviceAllCriteria),
				add(slice1),
				add(slice2),
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDeviceAllCriteria}}),
				slice2,
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDeviceAllCriteria}})},
				{event: handlerEventAdd, newObj: slice2},
			},
		},
		"update-patched-slice": {
			events: []any{
				add(taintDevice1Rule),
				[]any{
					add(slice1),
					update(slice1, sliceWithDevices(slice1, threeDevices)),
				},
				[]any{
					add(sliceWithDevices(slice2, threeDevices)),
					update(sliceWithDevices(slice2, threeDevices), sliceWithDevices(slice2, devices)),
				},
			},
			expectedPatchedSlices: []*draapi.ResourceSlice{
				taintMeta(sliceWithDevices(slice1, threeDevicesOneTainted), m{{device1Name, 0}: {1, taintDevice1Rule}}),
				taintMeta(sliceWithDevices(slice2, taintedDevices), m{{device1Name, 0}: {2, taintDevice1Rule}}),
			},
			expectedHandlerEvents: []handlerEvent{
				{event: handlerEventAdd, newObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDevice1Rule}})},
				{event: handlerEventUpdate, oldObj: taintMeta(slice1Tainted, m{{device1Name, 0}: {1, taintDevice1Rule}}), newObj: taintMeta(sliceWithDevices(slice1, threeDevicesOneTainted), m{{device1Name, 0}: {1, taintDevice1Rule}})},
				{event: handlerEventAdd, newObj: taintMeta(sliceWithDevices(slice2, threeDevicesOneTainted), m{{device1Name, 0}: {2, taintDevice1Rule}})},
				{event: handlerEventUpdate, oldObj: taintMeta(sliceWithDevices(slice2, threeDevicesOneTainted), m{{device1Name, 0}: {2, taintDevice1Rule}}), newObj: taintMeta(sliceWithDevices(slice2, taintedDevices), m{{device1Name, 0}: {2, taintDevice1Rule}})},
			},
		},
	}

	setup := func(t *testing.T) *testContext {
		_, ctx := ktesting.NewTestContext(t)

		kubeClient := fake.NewSimpleClientset()
		informerFactory := informers.NewSharedInformerFactoryWithOptions(kubeClient, 10*time.Minute)

		opts := Options{
			EnableDeviceTaints: true,
			SliceInformer:      draapi.NewResourceSliceInformer(informerFactory),
			TaintInformer:      informerFactory.Resource().V1alpha3().DeviceTaintRules(),
			ClassInformer:      informerFactory.Resource().V1().DeviceClasses(),
			KubeClient:         kubeClient,
		}
		tracker, err := newTracker(ctx, opts)
		require.NoError(t, err)

		return &testContext{
			T:         t,
			Context:   ctx,
			Tracker:   tracker,
			Clientset: kubeClient,
		}
	}

	testHandlers := func(tCtx *testContext, test test, permutation []int) {
		isPermutated := false
		for i, j := range permutation {
			if i != j {
				isPermutated = true
				break
			}
		}

		var handlerEvents []handlerEvent
		handler := cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				handlerEvents = append(handlerEvents, handlerEvent{event: handlerEventAdd, newObj: obj.(*draapi.ResourceSlice)})
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				handlerEvents = append(handlerEvents, handlerEvent{event: handlerEventUpdate, oldObj: oldObj.(*draapi.ResourceSlice), newObj: newObj.(*draapi.ResourceSlice)})
			},
			DeleteFunc: func(obj interface{}) {
				handlerEvents = append(handlerEvents, handlerEvent{event: handlerEventDelete, oldObj: obj.(*draapi.ResourceSlice)})
			},
		}
		_, _ = tCtx.AddEventHandler(handler)

		var unhandledErrors []error
		tCtx.handleError = func(_ context.Context, err error, _ string, _ ...any) {
			unhandledErrors = append(unhandledErrors, err)
		}

		runInputEvents(tCtx, test.events, permutation)

		if !isPermutated {
			assert.Equal(tCtx, test.expectedHandlerEvents, handlerEvents)
		}

		expectUnhandledErrors := test.expectUnhandledErrors
		if expectUnhandledErrors == nil {
			expectUnhandledErrors = func(t *testing.T, errs []error) {
				assert.Empty(t, errs)
			}
		}
		expectUnhandledErrors(tCtx.T, unhandledErrors)

		// Check ResourceSlices
		patchedResourceSlices, err := tCtx.ListPatchedResourceSlices()
		require.NoError(tCtx, err, "list patched resource slices")
		sortResourceSlicesFunc := func(s1, s2 *draapi.ResourceSlice) int {
			return stdcmp.Compare(s1.Name, s2.Name)
		}
		expectedPatchedSlices := slices.Clone(test.expectedPatchedSlices)
		slices.SortFunc(expectedPatchedSlices, sortResourceSlicesFunc)
		slices.SortFunc(patchedResourceSlices, sortResourceSlicesFunc)
		if isPermutated {
			expectedPatchedSlices = trimDeviceTaintMeta(expectedPatchedSlices)
			patchedResourceSlices = trimDeviceTaintMeta(patchedResourceSlices)
		}
		assert.Equal(tCtx, expectedPatchedSlices, patchedResourceSlices)
		expectEvents := test.expectEvents
		if expectEvents == nil {
			expectEvents = func(t *assert.CollectT, events *v1.EventList) {
				assert.Empty(t, events.Items)
			}
		}
		// Events are generated asynchronously. While shutting down the event recorder will flush all
		// pending events, it is not possible to determine when exactly that flush is complete.
		// TODO (pohly): use synctest.Wait instead, ideally with ktesting
		assert.EventuallyWithT(
			tCtx,
			func(t *assert.CollectT) {
				events, err := tCtx.CoreV1().Events("").List(tCtx.Context, metav1.ListOptions{})
				require.NoError(t, err, "list events")
				expectEvents(t, events)
			},
			1*time.Second,
			10*time.Millisecond,
			"did not observe expected events",
		)
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// flatten does one level of flattening of events, counting all events.
			// It also returns a slice of pairs of indices representing ranges which were
			// flattened (= came from the second level) and which therefore must
			// remain in that order.
			flatten := func(events []any) (int, [][2]int) {
				numEvents := 0
				var ranges [][2]int
				for _, e := range events {
					switch e := e.(type) {
					case []any:
						ranges = append(ranges, [2]int{numEvents, numEvents + len(e)})
						numEvents += len(e)
					default:
						numEvents++
					}
				}
				return numEvents, ranges
			}
			numEvents, constraints := flatten(tc.events)

			if len(tc.events) <= 1 {
				// No permutations possible.
				var permutation []int
				for i := 0; i < numEvents; i++ {
					permutation = append(permutation, i)
				}
				tContext := setup(t)
				testHandlers(tContext, tc, permutation)
				return
			}

			permutation := make([]int, numEvents)
			var permutate func(depth int)
			permutate = func(depth int) {
				if depth >= numEvents {
					// Define a sub-test which runs the current permutation of events.
					name := strings.Trim(fmt.Sprintf("%v", permutation), "[]")
					t.Run(name, func(t *testing.T) {
						tContext := setup(t)
						// No need to clone the slice, we don't run in parallel.
						testHandlers(tContext, tc, permutation)
					})
					return
				}
			nexti:
				for i := range numEvents {
					if slices.Contains(permutation[0:depth], i) {
						// Already taken.
						continue
					}
					for _, constraint := range constraints {
						if i < constraint[0] || i > constraint[1] {
							continue
						}
						for j := i + 1; j < constraint[1]; j++ {
							if slices.Contains(permutation[0:depth], j) {
								// Invalid permutation, would change order
								// of sub-events.
								continue nexti
							}
						}
					}

					// Pick it for the current position in permutation,
					// continue with next position.
					permutation[depth] = i
					permutate(depth + 1)
				}
			}
			permutate(0)
		})
	}
}

func trimDeviceTaintMeta(slices []*draapi.ResourceSlice) []*draapi.ResourceSlice {
	var trimmedSlices []*draapi.ResourceSlice
	for _, slice := range slices {
		slice = slice.DeepCopy()
		for i, device := range slice.Spec.Devices {
			for j := range device.Taints {
				slice.Spec.Devices[i].Taints[j].ID = 0
			}
		}
		trimmedSlices = append(trimmedSlices, slice)
	}
	return trimmedSlices
}

func BenchmarkEventHandlers(b *testing.B) {
	now := time.Now()
	benchmarks := map[string]struct {
		resourceSlices []*draapi.ResourceSlice
		taintRules     []*resourcealphaapi.DeviceTaintRule
		loop           func(ctx context.Context, b *testing.B, tracker *Tracker, resourceSlices []*draapi.ResourceSlice, taintRules []*resourcealphaapi.DeviceTaintRule, i int)
	}{
		"resource-slice-add-no-taint-rules": {
			resourceSlices: func() []*draapi.ResourceSlice {
				resourceSlices := make([]*draapi.ResourceSlice, 1000)
				for i := range resourceSlices {
					resourceSlices[i] = &draapi.ResourceSlice{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-" + strconv.Itoa(i),
						},
						Spec: draapi.ResourceSliceSpec{
							Devices: slices.Repeat([]draapi.Device{}, 64),
						},
					}
				}
				return resourceSlices
			}(),
			loop: func(ctx context.Context, b *testing.B, tracker *Tracker, resourceSlices []*draapi.ResourceSlice, _ []*resourcealphaapi.DeviceTaintRule, i int) {
				tracker.resourceSliceAdd(ctx)(resourceSlices[i%len(resourceSlices)])
			},
		},
		"one-patch-to-many-slices-add-taint-rule": {
			resourceSlices: func() []*draapi.ResourceSlice {
				resourceSlices := make([]*draapi.ResourceSlice, 500)
				for i := range resourceSlices {
					resourceSlices[i] = &draapi.ResourceSlice{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-" + strconv.Itoa(i),
						},
						Spec: draapi.ResourceSliceSpec{
							Devices: slices.Repeat([]draapi.Device{{}}, 64),
						},
					}
				}
				return resourceSlices
			}(),
			taintRules: []*resourcealphaapi.DeviceTaintRule{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "taintRule",
					},
					Spec: resourcealphaapi.DeviceTaintRuleSpec{
						DeviceSelector: nil, // all slices
						Taint: resourcealphaapi.DeviceTaint{
							Key:       "example.com/taint",
							Value:     "tainted",
							Effect:    resourcealphaapi.DeviceTaintEffectNoExecute,
							TimeAdded: &metav1.Time{Time: now},
						},
					},
				},
			},
			loop: func(ctx context.Context, b *testing.B, tracker *Tracker, resourceSlices []*draapi.ResourceSlice, taintRules []*resourcealphaapi.DeviceTaintRule, i int) {
				tracker.deviceTaintAdd(ctx)(taintRules[i%len(taintRules)])
			},
		},
		"one-patch-to-many-slices-add-slice": {
			resourceSlices: func() []*draapi.ResourceSlice {
				resourceSlices := make([]*draapi.ResourceSlice, 500)
				for i := range resourceSlices {
					resourceSlices[i] = &draapi.ResourceSlice{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-" + strconv.Itoa(i),
						},
						Spec: draapi.ResourceSliceSpec{
							Devices: slices.Repeat([]draapi.Device{{}}, 64),
						},
					}
				}
				return resourceSlices
			}(),
			taintRules: []*resourcealphaapi.DeviceTaintRule{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "taintRule",
					},
					Spec: resourcealphaapi.DeviceTaintRuleSpec{
						DeviceSelector: nil, // all slices
						Taint: resourcealphaapi.DeviceTaint{
							Key:       "example.com/taint",
							Value:     "tainted",
							Effect:    resourcealphaapi.DeviceTaintEffectNoExecute,
							TimeAdded: &metav1.Time{Time: now},
						},
					},
				},
			},
			loop: func(ctx context.Context, b *testing.B, tracker *Tracker, resourceSlices []*draapi.ResourceSlice, _ []*resourcealphaapi.DeviceTaintRule, i int) {
				tracker.resourceSliceAdd(ctx)(resourceSlices[i%len(resourceSlices)])
			},
		},
		"one-patched-device-among-many-slices-add-taint-rule": {
			resourceSlices: func() []*draapi.ResourceSlice {
				nSlices := 500
				nDevices := 64
				resourceSlices := make([]*draapi.ResourceSlice, nSlices)
				for i := range resourceSlices {
					resourceSlices[i] = &draapi.ResourceSlice{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-" + strconv.Itoa(i),
						},
						Spec: draapi.ResourceSliceSpec{
							Pool: draapi.ResourcePool{
								Name: u("pool-" + strconv.Itoa(i)),
							},
							Devices: func() []draapi.Device {
								devices := make([]draapi.Device, nDevices)
								for j := range devices {
									devices[j] = draapi.Device{
										Name: u("device-" + strconv.Itoa(j)),
									}
								}
								return devices
							}(),
						},
					}
				}
				resourceSlices[nSlices/2].Spec.Devices[nDevices/2].Name = u("patchme")
				return resourceSlices
			}(),
			taintRules: []*resourcealphaapi.DeviceTaintRule{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "taintRule",
					},
					Spec: resourcealphaapi.DeviceTaintRuleSpec{
						DeviceSelector: &resourcealphaapi.DeviceTaintSelector{
							Device: ptr.To("patchme"),
						},
						Taint: resourcealphaapi.DeviceTaint{
							Key:       "example.com/taint",
							Value:     "tainted",
							Effect:    resourcealphaapi.DeviceTaintEffectNoExecute,
							TimeAdded: &metav1.Time{Time: now},
						},
					},
				},
			},
			loop: func(ctx context.Context, b *testing.B, tracker *Tracker, resourceSlices []*draapi.ResourceSlice, taintRules []*resourcealphaapi.DeviceTaintRule, i int) {
				tracker.deviceTaintAdd(ctx)(taintRules[i%len(taintRules)])
			},
		},
		"one-patched-device-among-many-slices-add-slice": {
			resourceSlices: func() []*draapi.ResourceSlice {
				resourceSlices := make([]*draapi.ResourceSlice, 500)
				for i := range resourceSlices {
					resourceSlices[i] = &draapi.ResourceSlice{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-" + strconv.Itoa(i),
						},
						Spec: draapi.ResourceSliceSpec{
							Pool: draapi.ResourcePool{
								Name: u("pool-" + strconv.Itoa(i)),
							},
							Devices: func() []draapi.Device {
								nDevices := 64
								devices := slices.Repeat([]draapi.Device{{}}, nDevices)
								devices[nDevices/2].Name = u("patchme")
								return devices
							}(),
						},
					}
				}
				return resourceSlices
			}(),
			taintRules: []*resourcealphaapi.DeviceTaintRule{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "patch",
					},
					Spec: resourcealphaapi.DeviceTaintRuleSpec{
						DeviceSelector: &resourcealphaapi.DeviceTaintSelector{
							Pool:   ptr.To("pool-250"),
							Device: ptr.To("patchme"),
						},
						Taint: resourcealphaapi.DeviceTaint{
							Key:       "example.com/taint",
							Value:     "tainted",
							Effect:    resourcealphaapi.DeviceTaintEffectNoExecute,
							TimeAdded: &metav1.Time{Time: now},
						},
					},
				},
			},
			loop: func(ctx context.Context, b *testing.B, tracker *Tracker, resourceSlices []*draapi.ResourceSlice, patches []*resourcealphaapi.DeviceTaintRule, i int) {
				tracker.resourceSliceAdd(ctx)(resourceSlices[250]) // the slice affected by the patch
			},
		},
		"one-patch-for-each-of-many-slices-add-taint-rule": {
			resourceSlices: func() []*draapi.ResourceSlice {
				resourceSlices := make([]*draapi.ResourceSlice, 500)
				for i := range resourceSlices {
					resourceSlices[i] = &draapi.ResourceSlice{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-" + strconv.Itoa(i),
						},
						Spec: draapi.ResourceSliceSpec{
							Pool: draapi.ResourcePool{
								Name: u("pool-" + strconv.Itoa(i)),
							},
							Devices: slices.Repeat([]draapi.Device{{}}, 64),
						},
					}
				}
				return resourceSlices
			}(),
			taintRules: func() []*resourcealphaapi.DeviceTaintRule {
				patches := make([]*resourcealphaapi.DeviceTaintRule, 500)
				for i := range patches {
					patches[i] = &resourcealphaapi.DeviceTaintRule{
						ObjectMeta: metav1.ObjectMeta{
							Name: "taint-rule-" + strconv.Itoa(i),
						},
						Spec: resourcealphaapi.DeviceTaintRuleSpec{
							DeviceSelector: &resourcealphaapi.DeviceTaintSelector{
								Pool: ptr.To("pool-" + strconv.Itoa(i)),
							},
							Taint: resourcealphaapi.DeviceTaint{
								Key:       "example.com/taint",
								Value:     "tainted",
								Effect:    resourcealphaapi.DeviceTaintEffectNoExecute,
								TimeAdded: &metav1.Time{Time: now},
							},
						},
					}
				}
				return patches
			}(),
			loop: func(ctx context.Context, b *testing.B, tracker *Tracker, resourceSlices []*draapi.ResourceSlice, taintRules []*resourcealphaapi.DeviceTaintRule, i int) {
				tracker.deviceTaintAdd(ctx)(taintRules[i%len(taintRules)])
			},
		},
	}

	newBenchTracker := func(ctx context.Context) *Tracker {
		kubeClient := fake.NewSimpleClientset()
		informerFactory := informers.NewSharedInformerFactoryWithOptions(kubeClient, 10*time.Minute)
		opts := Options{
			EnableDeviceTaints: true,
			SliceInformer:      draapi.NewResourceSliceInformer(informerFactory),
			TaintInformer:      informerFactory.Resource().V1alpha3().DeviceTaintRules(),
			ClassInformer:      informerFactory.Resource().V1().DeviceClasses(),
			KubeClient:         kubeClient,
		}
		tracker, err := newTracker(ctx, opts)
		require.NoError(b, err)
		tracker.handleError = func(_ context.Context, err error, _ string, _ ...any) {
			b.Error("unexpected unhandled error:", err)
		}
		return tracker
	}

	for name, benchmark := range benchmarks {
		b.Run(name, func(b *testing.B) {
			logger, ctx := ktesting.NewTestContext(b)
			ctx = klog.NewContext(ctx, logger.V(2))
			tracker := newBenchTracker(ctx)

			for _, slice := range benchmark.resourceSlices {
				err := tracker.resourceSlices.GetIndexer().Add(slice)
				require.NoError(b, err)
			}

			for _, taintRule := range benchmark.taintRules {
				err := tracker.deviceTaints.GetIndexer().Add(taintRule)
				require.NoError(b, err)
			}

			b.ResetTimer()
			for i := range b.N {
				benchmark.loop(ctx, b, tracker, benchmark.resourceSlices, benchmark.taintRules, i)
			}
		})
	}
}
