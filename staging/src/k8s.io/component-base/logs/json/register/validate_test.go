/*
Copyright 2021 The Kubernetes Authors.

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

package register_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/component-base/featuregate"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	v1 "k8s.io/component-base/logs/api/v1"
)

func TestValidation(t *testing.T) {
	jsonFormat := v1.LoggingConfiguration{
		Format: "json",
	}
	testcases := map[string]struct {
		config                    v1.LoggingConfiguration
		alphaEnabled, betaEnabled bool
		expectErrors              string
	}{
		"off+off": {
			config:       jsonFormat,
			alphaEnabled: false,
			betaEnabled:  false,
			expectErrors: `format: Forbidden: Log format json is BETA and disabled, see LoggingBetaOptions feature`,
		},
		"on+off": {
			config:       jsonFormat,
			alphaEnabled: true,
			betaEnabled:  false,
			// TODO: should the JSON format be enabled when alpha is enabled?
			expectErrors: `format: Forbidden: Log format json is BETA and disabled, see LoggingBetaOptions feature`,
		},
		"off+on": {
			config:       jsonFormat,
			alphaEnabled: false,
			betaEnabled:  true,
		},
		"on+on": {
			config:       jsonFormat,
			alphaEnabled: true,
			betaEnabled:  true,
		},
	}

	for name, test := range testcases {
		t.Run(name, func(t *testing.T) {
			defer featuregatetesting.SetFeatureGateDuringTest(t, featuregate.DefaultFeatureGate, v1.LoggingAlphaOptions, test.alphaEnabled)()
			defer featuregatetesting.SetFeatureGateDuringTest(t, featuregate.DefaultFeatureGate, v1.LoggingBetaOptions, test.betaEnabled)()
			errs := test.config.ValidateAsField(nil)
			if len(errs) == 0 {
				if test.expectErrors != "" {
					t.Fatalf("did not get expected error(s): %s", test.expectErrors)
				}
			} else {
				assert.Equal(t, test.expectErrors, errs.ToAggregate().Error())
			}
		})
	}
}
