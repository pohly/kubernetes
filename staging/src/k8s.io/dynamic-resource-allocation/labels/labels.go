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

// Package labels implements the resource selection via labels for the numeric
// models. The APIs use [metav1.LabelSelector] as data type with some
// additional operators defined in
// [k8s.io/dynamic-resource-allocation/apis/meta/v1]. [Compile] checks a
// [metav1.LabelSelector] and returns a [Matcher] which then can be used
// to check labels efficiently.
package labels

import (
	"fmt"
	"slices"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	drametav1 "k8s.io/dynamic-resource-allocation/apis/meta/v1"
)

func Compile(s *metav1.LabelSelector) (*Matcher, error) {
	// This is similar to https://github.com/kubernetes/apimachinery/blob/3c8c1f22dc332a8a6b51033f6a7c0566b885d290/pkg/apis/meta/v1/helpers.go#L31-L72
	if s == nil {
		// Match nothing.
		return nil, nil
	}
	if len(s.MatchLabels)+len(s.MatchExpressions) == 0 {
		// Match everything.
		return &Matcher{}, nil
	}
	requirements := make([]requirement, 0, len(s.MatchLabels)+len(s.MatchExpressions))
	for k, v := range s.MatchLabels {
		r := requirement{op: metav1.LabelSelectorOpIn, key: k, strings: []string{v}}
		requirements = append(requirements, r)
	}
	fldPath := field.NewPath("matchExpressions")
	for i, expr := range s.MatchExpressions {
		fldIdx := fldPath.Index(i)
		fldValues := fldIdx.Child("values")

		r := requirement{
			op:  expr.Operator,
			key: expr.Key,
		}
		switch expr.Operator {
		case drametav1.LabelSelectorOpIn, drametav1.LabelSelectorOpNotIn:
			r.strings = expr.Values
		case drametav1.LabelSelectorOpExists, drametav1.LabelSelectorOpDoesNotExist:
			if len(expr.Values) != 0 {
				return nil, fmt.Errorf("%s: must be empty, have %q", fldValues, expr.Values)
			}
		case drametav1.LabelSelectorOpVersionGE, drametav1.LabelSelectorOpVersionGT, drametav1.LabelSelectorOpVersionLE, drametav1.LabelSelectorOpVersionLT, drametav1.LabelSelectorOpVersionEQ, drametav1.LabelSelectorOpVersionNE:
			if len(expr.Values) != 1 {
				return nil, fmt.Errorf("%s: must have exactly one entry, have %q", fldValues, expr.Values)
			}
			version, err := semver.ParseTolerant(expr.Values[0])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fldValues, err)
			}
			r.version = version
		case drametav1.LabelSelectorOpGE, drametav1.LabelSelectorOpGT, drametav1.LabelSelectorOpLE, drametav1.LabelSelectorOpLT, drametav1.LabelSelectorOpEQ, drametav1.LabelSelectorOpNE:
			if len(expr.Values) != 1 {
				return nil, fmt.Errorf("%s: must have exactly one entry, have %q", fldValues, expr.Values)
			}
			quantity, err := resource.ParseQuantity(expr.Values[0])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fldValues, err)
			}
			r.quantity = cmpQuantity{Quantity: quantity}
		default:
			return nil, fmt.Errorf("%s: %q is not a valid label selector operator", fldIdx.Child("operator"), expr.Operator)
		}
		requirements = append(requirements, r)
	}
	return &Matcher{
		requirements: requirements,
	}, nil
}

// Matcher implements [*metav1.LabelSelector] matching against a set of labels.
// The nil Matcher matches nothing. The empty Matcher matches everything.
type Matcher struct {
	requirements []requirement
}

func (m *Matcher) Matches(labels map[string]string) (bool, error) {
	if m == nil {
		// Nil matcher matches nothing.
		return false, nil
	}

	// All requirements must match.
	for _, r := range m.requirements {
		match, err := r.Matches(labels)
		if !match || err != nil {
			return match, err
		}
	}

	return true, nil
}

type requirement struct {
	op       drametav1.LabelSelectorOperator
	key      string
	strings  []string
	version  semver.Version
	quantity cmpQuantity
}

func (r requirement) Matches(labels map[string]string) (bool, error) {
	// This is similar to https://github.com/kubernetes/apimachinery/blob/02a41040d88da08de6765573ae2b1a51f424e1ca/pkg/labels/selector.go#L212-L267

	switch r.op {
	case drametav1.LabelSelectorOpIn:
		value, ok := labels[r.key]
		if !ok {
			return false, nil
		}
		return slices.Contains(r.strings, value), nil
	case drametav1.LabelSelectorOpNotIn:
		value, ok := labels[r.key]
		if !ok {
			return false, nil
		}
		return !slices.Contains(r.strings, value), nil
	case drametav1.LabelSelectorOpExists:
		_, ok := labels[r.key]
		return ok, nil
	case drametav1.LabelSelectorOpDoesNotExist:
		_, ok := labels[r.key]
		return !ok, nil
	case drametav1.LabelSelectorOpVersionGE:
		return matchVersion(labels, r.key, r.version.GE)
	case drametav1.LabelSelectorOpVersionGT:
		return matchVersion(labels, r.key, r.version.GT)
	case drametav1.LabelSelectorOpVersionLE:
		return matchVersion(labels, r.key, r.version.LE)
	case drametav1.LabelSelectorOpVersionLT:
		return matchVersion(labels, r.key, r.version.LT)
	case drametav1.LabelSelectorOpVersionEQ:
		return matchVersion(labels, r.key, r.version.EQ)
	case drametav1.LabelSelectorOpVersionNE:
		return matchVersion(labels, r.key, r.version.NE)
	case drametav1.LabelSelectorOpGE:
		return matchQuantity(labels, r.key, r.quantity.GE)
	case drametav1.LabelSelectorOpGT:
		return matchQuantity(labels, r.key, r.quantity.GT)
	case drametav1.LabelSelectorOpLE:
		return matchQuantity(labels, r.key, r.quantity.LE)
	case drametav1.LabelSelectorOpLT:
		return matchQuantity(labels, r.key, r.quantity.LT)
	case drametav1.LabelSelectorOpEQ:
		return matchQuantity(labels, r.key, r.quantity.EQ)
	case drametav1.LabelSelectorOpNE:
		return matchQuantity(labels, r.key, r.quantity.NE)
	}
	return true, nil
}

func matchVersion(labels map[string]string, key string, op func(v semver.Version) bool) (bool, error) {
	value, ok := labels[key]
	if !ok {
		return false, nil
	}
	version, err := semver.ParseTolerant(value)
	if err != nil {
		return false, fmt.Errorf("parsing label %q as semantic version: %w", key, err)
	}
	return op(version), nil
}

func matchQuantity(labels map[string]string, key string, op func(q resource.Quantity) bool) (bool, error) {
	value, ok := labels[key]
	if !ok {
		return false, nil
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		return false, fmt.Errorf("parsing label %q as quantity: %w", key, err)
	}
	return op(quantity), nil
}

// quantity adds more comparison methods to resource.Quantity.
type cmpQuantity struct {
	resource.Quantity
}

func (q cmpQuantity) GE(o resource.Quantity) bool {
	return q.Cmp(o) >= 0
}

func (q cmpQuantity) GT(o resource.Quantity) bool {
	return q.Cmp(o) > 0
}

func (q cmpQuantity) LT(o resource.Quantity) bool {
	return q.Cmp(o) < 0
}

func (q cmpQuantity) LE(o resource.Quantity) bool {
	return q.Cmp(o) <= 0
}

func (q cmpQuantity) EQ(o resource.Quantity) bool {
	return q.Cmp(o) == 0
}

func (q cmpQuantity) NE(o resource.Quantity) bool {
	return q.Cmp(o) != 0
}
