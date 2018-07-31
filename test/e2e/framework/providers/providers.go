/*
Copyright 2016 The Kubernetes Authors.

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

package providers

import (
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/providers/aws"
	"k8s.io/kubernetes/test/e2e/framework/providers/azure"
	"k8s.io/kubernetes/test/e2e/framework/providers/gce"
	"k8s.io/kubernetes/test/e2e/framework/providers/kubemark"
)

// SetupProviderConfig validates and sets up TestContext.CloudConfig based on framework.TestContext.Provider.
//
// This could be made more modular such that providers can be picked individually by a test suite
// author, but in practice that doesn't gain anything in terms of code dependency reduction because
// k8s.io/kubernetes/pkg/cloudprovider itself already pulls in most of the dependencies.
func SetupProviderConfig() error {
	var err error
	switch framework.TestContext.Provider {
	case "":
		framework.Logf("The --provider flag is not set.  Treating as a conformance test.  Some tests may not be run.")

	case "kubemark":
		framework.TestContext.CloudConfig.Provider, err = kubemark.NewProvider()
	case "gce", "gke":
		framework.TestContext.CloudConfig.Provider, err = gce.NewProvider()
	case "aws":
		framework.TestContext.CloudConfig.Provider, err = aws.NewProvider()
	case "azure":
		framework.TestContext.CloudConfig.Provider, err = azure.NewProvider()
	}

	return err
}
