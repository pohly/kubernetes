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

package cel

import (
	"strings"
	"testing"

	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/klog/v2/ktesting"
	"k8s.io/utils/ptr"
)

func TestCompile(t *testing.T) {
	for name, scenario := range map[string]struct {
		expression         string
		driver             string
		attributes         map[resourceapi.QualifiedName]resourceapi.DeviceAttribute
		capacity           map[resourceapi.QualifiedName]resourceapi.DeviceCapacity
		expectCompileError string
		expectMatchError   string
		expectMatch        bool

		// There's no good way to verify that the cost of an expression
		// really is what it should be other than eye-balling it. The
		// cost should not change in the future unless the expression
		// gets changed, so this provides some protection against
		// regressions.
		expectCost uint64
	}{
		"true": {
			expression:  "true",
			expectMatch: true,
		},
		"false": {
			expression:  "false",
			expectMatch: false,
		},
		"syntax-error": {
			expression:         "?!",
			expectCompileError: "Syntax error",
		},
		"type-error": {
			expression:         `1`,
			expectCompileError: "must evaluate to bool or the unknown type, not int",
		},
		"runtime-error-lookup-identifier": {
			expression:       `device.attributes["no-such-domain"].noSuchAttr.isGreaterThan(quantity("0"))`,
			expectMatchError: "no such key: noSuchAttr",
			expectCost:       6,
		},
		"runtime-error-lookup-map": {
			expression:       `device.attributes["no-such-domain"]["noSuchAttr"].isGreaterThan(quantity("0"))`,
			expectMatchError: "no such key: noSuchAttr",
			expectCost:       6,
		},
		"domain-check-negative": {
			expression:  `"no-such-domain" in device.attributes`,
			expectMatch: false,
			expectCost:  3,
		},
		"domain-check-positive": {
			expression:  `"dra.example.com" in device.attributes`,
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"dra.example.com/something": {BoolValue: ptr.To(true)}},
			expectMatch: true,
			expectCost:  3,
		},
		"empty-driver-name": {
			expression:  `device.driver == ""`,
			expectMatch: true,
			expectCost:  2,
		},
		"real-driver-name": {
			expression:  `device.driver == "dra.example.com"`,
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  4,
		},
		"driver-name-qualifier": {
			expression:  `device.attributes["dra.example.com"].name`,
			driver:      "dra.example.com",
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"name": {BoolValue: ptr.To(true)}},
			expectMatch: true,
			expectCost:  4,
		},
		"driver-name-qualifier-map-lookup": {
			expression:  `device.attributes["dra.example.com"]["name"]`,
			driver:      "dra.example.com",
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"name": {BoolValue: ptr.To(true)}},
			expectMatch: true,
			expectCost:  4,
		},
		"bind": {
			expression:  `cel.bind(dra, device.attributes["dra.example.com"], dra.name)`,
			driver:      "dra.example.com",
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"name": {BoolValue: ptr.To(true)}},
			expectMatch: true,
			expectCost:  15,
		},
		"qualified-attribute-name": {
			expression:  `device.attributes["other.example.com"].name`,
			driver:      "dra.example.com",
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"other.example.com/name": {BoolValue: ptr.To(true)}},
			expectMatch: true,
			expectCost:  4,
		},
		"bool": {
			expression:  `device.attributes["dra.example.com"].name`,
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"name": {BoolValue: ptr.To(true)}},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  4,
		},
		"int": {
			expression:  `device.attributes["dra.example.com"].name > 0`,
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"name": {IntValue: ptr.To(int64(1))}},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  5,
		},
		"string": {
			expression:  `device.attributes["dra.example.com"].name == "fish"`,
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"name": {StringValue: ptr.To("fish")}},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  5,
		},
		"version": {
			expression:  `device.attributes["dra.example.com"].name.isGreaterThan(semver("0.0.1"))`,
			attributes:  map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{"name": {VersionValue: ptr.To("1.0.0")}},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  6,
		},
		"quantity": {
			expression:  `device.capacity["dra.example.com"].name.isGreaterThan(quantity("1Ki"))`,
			capacity:    map[resourceapi.QualifiedName]resourceapi.DeviceCapacity{"name": {Quantity: resource.MustParse("1Mi")}},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  6,
		},
		"check-positive": {
			expression:  `"name" in device.capacity["dra.example.com"] && device.capacity["dra.example.com"].name.isGreaterThan(quantity("1Ki"))`,
			capacity:    map[resourceapi.QualifiedName]resourceapi.DeviceCapacity{"name": {Quantity: resource.MustParse("1Mi")}},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  10,
		},
		"check-negative": {
			expression:  `!("name" in device.capacity["dra.example.com"]) || device.capacity["dra.example.com"].name.isGreaterThan(quantity("1Ki"))`,
			expectMatch: true,
			expectCost:  11,
		},
		"all": {
			expression: `
device.capacity["dra.example.com"].quantity.isGreaterThan(quantity("1Ki")) &&
device.attributes["dra.example.com"].bool &&
device.attributes["dra.example.com"].int > 0 &&
device.attributes["dra.example.com"].string == "fish" &&
device.attributes["dra.example.com"].version.isGreaterThan(semver("0.0.1")) &&
device.capacity["dra.example.com"]["quantity"].isGreaterThan(quantity("1Ki")) &&
device.attributes["dra.example.com"]["bool"] &&
device.attributes["dra.example.com"]["int"] > 0 &&
device.attributes["dra.example.com"]["string"] == "fish" &&
device.attributes["dra.example.com"]["version"].isGreaterThan(semver("0.0.1"))
`,
			attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
				"bool":    {BoolValue: ptr.To(true)},
				"int":     {IntValue: ptr.To(int64(1))},
				"string":  {StringValue: ptr.To("fish")},
				"version": {VersionValue: ptr.To("1.0.0")},
			},
			capacity: map[resourceapi.QualifiedName]resourceapi.DeviceCapacity{
				"quantity": {Quantity: resource.MustParse("1Mi")},
			},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  52,
		},
		"many": {
			expression: `device.attributes["dra.example.com"].a && device.attributes["dra.example.com"].b && device.attributes["dra.example.com"].c`,
			attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
				"a": {BoolValue: ptr.To(true)},
				"b": {BoolValue: ptr.To(true)},
				"c": {BoolValue: ptr.To(true)},
			},
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  12,
		},
		"expensive": {
			// The worst-case is based on the maximum number of
			// attributes and the maximum attribute name length.
			expression:  `device.attributes["dra.example.com"].map(s, s.lowerAscii()).map(s, s.size()).sum() == 0`,
			driver:      "dra.example.com",
			expectMatch: true,
			expectCost:  18446744073709551615, // Exceeds limit!
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, ctx := ktesting.NewTestContext(t)
			result := GetCompiler().CompileCELExpression(scenario.expression, environment.StoredExpressions)
			if scenario.expectCompileError != "" && result.Error == nil {
				t.Fatalf("expected compile error %q, got none", scenario.expectCompileError)
			}
			if result.Error != nil {
				if scenario.expectCompileError == "" {
					t.Fatalf("unexpected compile error: %v", result.Error)
				}
				if !strings.Contains(result.Error.Error(), scenario.expectCompileError) {
					t.Fatalf("expected compile error to contain %q, but got instead: %v", scenario.expectCompileError, result.Error)
				}
				return
			}
			if expect, actual := scenario.expectCost, result.MaxCost; expect != actual {
				t.Errorf("expected CEL cost %d, got %d instead", expect, actual)
			}

			match, err := result.DeviceMatches(ctx, Device{Attributes: scenario.attributes, Capacity: scenario.capacity, Driver: scenario.driver})
			if err != nil {
				if scenario.expectMatchError == "" {
					t.Fatalf("unexpected evaluation error: %v", err)
				}
				if !strings.Contains(err.Error(), scenario.expectMatchError) {
					t.Fatalf("expected evaluation error to contain %q, but got instead: %v", scenario.expectMatchError, err)
				}
				return
			}
			if match != scenario.expectMatch {
				t.Fatalf("expected result %v, got %v", scenario.expectMatch, match)
			}
		})
	}
}
