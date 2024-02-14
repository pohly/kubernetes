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

package validation

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/resource/structured/namedresources"
)

func testResources(instances map[string][]namedresources.Attribute) *namedresources.Resources {
	resources := &namedresources.Resources{}
	for name, attributes := range instances {
		resources.Instances = append(resources.Instances,
			namedresources.Instance{
				Name:       name,
				Attributes: attributes,
			},
		)
	}
	sort.Slice(resources.Instances, func(i, j int) bool { return resources.Instances[i].Name < resources.Instances[j].Name })
	return resources
}

func TestValidateResources(t *testing.T) {
	// goodName := "foo"
	// badName := "!@#$%^"

	scenarios := map[string]struct {
		resources    *namedresources.Resources
		wantFailures field.ErrorList
	}{
		"empty": {
			resources: testResources(nil),
		},
		// TODO: more tests
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			errs := ValidateResources(scenario.resources, nil)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}

func TestValidateSelector(t *testing.T) {
	scenarios := map[string]struct {
		selector     string
		wantFailures field.ErrorList
	}{
		"okay": {
			selector: "true",
		},
		"empty": {
			selector:     "",
			wantFailures: field.ErrorList{field.Required(nil, "")},
		},
		"undefined": {
			selector:     "nosuchvar",
			wantFailures: field.ErrorList{field.Invalid(nil, "nosuchvar", "compilation failed: ERROR: <input>:1:1: undeclared reference to 'nosuchvar' (in container '')\n | nosuchvar\n | ^")},
		},
		"wrong-type": {
			selector:     "1",
			wantFailures: field.ErrorList{field.Invalid(nil, "1", "must evaluate to bool")},
		},
		"quantity": {
			selector: `attributes.quantity["name"].isGreaterThan(quantity("0"))`,
		},
		"bool": {
			selector: `attributes.bool["name"]`,
		},
		"int": {
			selector: `attributes.int["name"] > 0`,
		},
		"intslice": {
			selector: `attributes.intslice["name"].isSorted()`,
		},
		"string": {
			selector: `attributes.string["name"] == "fish"`,
		},
		"stringslice": {
			selector: `attributes.stringslice["name"].isSorted()`,
		},
		// TODO: more tests
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			// At the moment, there's no difference between stored and new expressions.
			// This uses the stricter validation.
			opts := Options{
				StoredExpressions: true,
			}
			errs := validateSelector(opts, scenario.selector, nil)
			assert.Equal(t, scenario.wantFailures, errs)
		})
	}
}
