/*
Copyright 2024 The Kubernetes Authors.

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

package incubating

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	apiservercel "k8s.io/apiserver/pkg/cel"
	draapi "k8s.io/dynamic-resource-allocation/api"
	"k8s.io/klog/v2"
)

// distinctAttributeConstraint compares an attribute value across devices.
// All devices must share the same value. When the set of devices is
// empty, any device that has the attribute can be added. After that,
// only matching devices can be added.
//
// We don't need to track *which* devices are part of the set, only
// how many.
type distinctAttributeConstraint struct {
	logger                         klog.Logger // Includes name and attribute name, so no need to repeat in log messages.
	requestNames                   sets.Set[string]
	attributeDomain, attributeName draapi.UniqueString

	attributes []any
}

func (m *distinctAttributeConstraint) add(requestName, subRequestName string, device *draapi.Device, deviceID DeviceID) bool {
	if m.requestNames.Len() > 0 && !m.matches(requestName, subRequestName) {
		// Device not affected by constraint.
		return true
	}

	attribute := device.Attributes.Lookup(m.attributeDomain, m.attributeName)
	if attribute == nil {
		// Doesn't have the attribute.
		m.logger.V(7).Info("Constraint not satisfied, attribute not set")
		return false
	}

	if len(m.attributes) == 0 {
		// The first device can always get picked.
		m.attributes = append(m.attributes, attribute)
		m.logger.V(7).Info("First attribute added")
		return true
	}

	if !m.matchesAttribute(attribute) {
		m.logger.V(7).Info("Constraint not satisfied, has some duplicated attributes")
		return false
	}
	m.attributes = append(m.attributes, attribute)
	m.logger.V(7).Info("Constraint satisfied by device", "device", deviceID, "numDevices", len(m.attributes))
	return true

}

func (m *distinctAttributeConstraint) remove(requestName, subRequestName string, device *draapi.Device, deviceID DeviceID) {
	if m.requestNames.Len() > 0 && !m.matches(requestName, subRequestName) {
		// Device not affected by constraint.
		return
	}
	// This keeps the same backing store, so we don't need to reallocate.
	m.attributes = m.attributes[:len(m.attributes)-1]
	m.logger.V(7).Info("Device removed from constraint set", "device", deviceID, "numDevices", len(m.attributes))
}

func (m *distinctAttributeConstraint) matches(requestName, subRequestName string) bool {
	if subRequestName == "" {
		return m.requestNames.Has(requestName)
	} else {
		fullSubRequestName := fmt.Sprintf("%s/%s", requestName, subRequestName)
		return m.requestNames.Has(requestName) || m.requestNames.Has(fullSubRequestName)
	}
}

func (m *distinctAttributeConstraint) matchesAttribute(attribute any) bool {
	// Different types are distinct. We only need to compare equal types.
	switch candidate := attribute.(type) {
	case string:
		for _, existing := range m.attributes {
			existing, ok := existing.(string)
			if ok && existing == candidate {
				m.logger.V(7).Info("Value not distinct", "existing", existing)
				return false
			}
		}
	case int64:
		for _, existing := range m.attributes {
			existing, ok := existing.(int64)
			if ok && existing == candidate {
				m.logger.V(7).Info("Value not distinct", "existing", existing)
				return false
			}
		}
	case bool:
		for _, existing := range m.attributes {
			existing, ok := existing.(bool)
			if ok && existing == candidate {
				m.logger.V(7).Info("Value not distinct", "existing", existing)
				return false
			}
		}
	case apiservercel.Semver:
		for _, existing := range m.attributes {
			existing, ok := existing.(apiservercel.Semver)
			if ok && existing.Version.Equals(candidate.Version) {
				m.logger.V(7).Info("Value not distinct", "existing", existing)
				return false
			}
		}
	default:
		// Unknown value type, cannot match.
		m.logger.V(7).Info("Distinct attribute type unknown", "candidate", candidate)
		return false
	}
	// All distinct
	return true
}
