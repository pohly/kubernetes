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
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	cdiapi "github.com/container-orchestrated-devices/container-device-interface/pkg/cdi"
	specs "github.com/container-orchestrated-devices/container-device-interface/specs-go"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubernetes/pkg/kubelet/apis/dra/v1alpha1"
	"k8s.io/kubernetes/test/integration/cdi/example-driver/kubeletplugin"
	plugin "k8s.io/kubernetes/test/integration/cdi/example-driver/kubeletplugin"
)

type examplePlugin struct {
	logger klog.Logger
	d      plugin.DRAPlugin

	cdiDir     string
	driverName string
}

var _ drapbv1.NodeServer = &examplePlugin{}

// runPlugin sets up the plugin and waits for connections from kubelet.
func runPlugin(logger klog.Logger, cdiDir, driverName, endpoint, draAddress, pluginRegistrationPath string) error {
	plugin := examplePlugin{
		logger:     logger,
		cdiDir:     cdiDir,
		driverName: driverName,
	}
	if err := plugin.start(driverName, endpoint, draAddress, pluginRegistrationPath); err != nil {
		return fmt.Errorf("start example plugin: %v", err)
	}
	// TODO: graceful termination via signal. Needs to be implemented in the caller.
	select {}
	plugin.stop()
	return nil
}

// getJSONFilePath returns the absolute path where CDI file is/should be.
func (ex *examplePlugin) getJSONFilePath(claimUID string) string {
	return filepath.Join(ex.cdiDir, fmt.Sprintf("%s-%s.json", ex.driverName, claimUID))
}

// start sets up the servers that are necessary for a DRA kubelet plugin.
func (ex *examplePlugin) start(driverName, endpoint, draAddress, pluginRegistrationPath string) error {
	// Ensure that directories exist, creating them if necessary. We want
	// to know early if there is a setup problem that would prevent
	// creating those directories.
	if err := os.MkdirAll(ex.cdiDir, os.FileMode(0750)); err != nil {
		return fmt.Errorf("create CDI directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(endpoint), 0750); err != nil {
		return fmt.Errorf("create socket directory: %v", err)
	}

	d, err := kubeletplugin.Start(ex.logger, driverName, endpoint, draAddress, pluginRegistrationPath, ex)
	if err != nil {
		return fmt.Errorf("start kubelet plugin: %v", err)
	}
	ex.d = d

	return nil
}

// stop ensures that all servers are stopped and resources freed.
func (ex *examplePlugin) stop() {
	ex.d.Stop()
}

// NodePrepareResource ensures that the CDI file for the claim exists. It uses
// a deterministic name to simplify NodeUnprepareResource (no need to remember
// or discover the name) and idempotency (when called again, the file simply
// gets written again).
func (ex *examplePlugin) NodePrepareResource(ctx context.Context, req *drapbv1.NodePrepareResourceRequest) (*drapbv1.NodePrepareResourceResponse, error) {
	logger := klog.FromContext(ctx)
	deviceName := "claim-" + req.ClaimUid
	kind := ex.driverName + "/test"
	filePath := ex.getJSONFilePath(req.ClaimUid)

	// Determine environment variables.
	var env map[string]string
	if err := json.Unmarshal([]byte(req.ResourceHandle), &env); err != nil {
		return nil, fmt.Errorf("unmarshal resource handle: %v", err)
	}

	// CDI wants env variables as set of strings.
	envs := []string{}
	for key, val := range env {
		envs = append(envs, key+"="+val)
	}

	spec := specs.Spec{
		Version: "0.2.0",
		Kind:    kind,
		// At least one device is required and its entry must have more
		// than just the name.
		Devices: []specs.Device{
			{
				Name: deviceName,
				ContainerEdits: specs.ContainerEdits{
					Env: envs,
				},
			},
		},
	}
	cdiSpec, err := cdiapi.NewSpec(&spec, filePath, 1)
	if err != nil {
		return nil, fmt.Errorf("create spec: %v", err)
	}

	buffer, err := json.Marshal(*cdiSpec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %v", err)
	}

	if err := os.WriteFile(filePath, buffer, os.FileMode(0644)); err != nil {
		return nil, fmt.Errorf("failed to write CDI file %v", err)
	}

	dev := cdiSpec.GetDevice(deviceName).GetQualifiedName()
	resp := &drapbv1.NodePrepareResourceResponse{CdiDevice: []string{dev}}

	logger.V(3).Info("CDI file created", "path", filePath, "device", dev)
	return resp, nil
}

// NodeUnprepareResource removes the CDI file created by
// NodePrepareResource. It's idempotent, therefore it is not an error when that
// file is already gone.
func (ex *examplePlugin) NodeUnprepareResource(ctx context.Context, req *drapbv1.NodeUnprepareResourceRequest) (*drapbv1.NodeUnprepareResourceResponse, error) {
	logger := klog.FromContext(ctx)

	filePath := ex.getJSONFilePath(req.ClaimUid)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error removing CDI file: %v", err)
	}
	logger.V(3).Info("CDI file removed", "path", filePath)
	return &drapbv1.NodeUnprepareResourceResponse{}, nil
}
