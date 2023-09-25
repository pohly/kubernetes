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

// Package init installs a simulation plugin for a test driver with
// resources as defined by the resources.json file in this directory.
// That same file can be can be passed to the test driver binary.
package init

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"k8s.io/dynamic-resource-allocation/simulation"
	"k8s.io/kubernetes/test/e2e/dra/test-driver/app"
	"k8s.io/kubernetes/test/e2e/dra/test-driver/simulation-plugin"
)

//go:embed resources.json
var resourcesJSON []byte

func init() {
	var resources app.Resources
	decoder := json.NewDecoder(bytes.NewBuffer(resourcesJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resources); err != nil {
		panic(fmt.Sprintf("internal error, resources.json does not parse: %v", err))
	}
	plugin := simulationplugin.TestPlugin{Resources: &resources}
	simulation.Registry.Add(simulation.PluginName(resources.DriverName), plugin)
}
