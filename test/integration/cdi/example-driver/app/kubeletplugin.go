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
	plugin "k8s.io/kubernetes/test/integration/cdi/example-driver/kubeletplugin"
)

const (
	deviceName = "exampledevice"
	cdiVersion = "0.2.0"
)

type examplePlugin struct {
	cdiDir                 string
	driverName             string
	draAddress             string
	pluginRegistrationPath string
}

// newExamplePlugin returns an initialized examplePlugin instance.
func newExamplePlugin(cdiDir, driverName, draAddress, pluginRegistrationPath string) (*examplePlugin, error) {
	return &examplePlugin{
		cdiDir:                 cdiDir,
		driverName:             driverName,
		draAddress:             draAddress,
		pluginRegistrationPath: pluginRegistrationPath,
	}, nil
}

// getJsonFilePath returns the absolute path where json file is/should be.
func (ex *examplePlugin) getJsonFilePath(claimUID string) string {
	jsonfileName := fmt.Sprintf("%s-%s.json", ex.driverName, claimUID)
	return filepath.Join(ex.cdiDir, jsonfileName)
}

// startPlugin starts servers that are necessary for plugins and registers the plugin with kubelet.
func (ex examplePlugin) startPlugin() error {
	klog.Infof("Starting kubelet plugin")

	// create CDI directory if not exists
	if err := os.MkdirAll(ex.cdiDir, os.FileMode(0750)); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// create DRA driver socket directory if not exists.
	if err := os.MkdirAll(filepath.Dir(ex.draAddress), 0750); err != nil {
		return fmt.Errorf("failed to create DRA driver socket directory: %v", err)
	}

	// Run a grpc server for the driver, that listens on draAddress
	if err := plugin.StartNonblockingGrpcServer(ex.draAddress, &ex); err != nil {
		return fmt.Errorf("failed to start a nonblocking grpc server: %v", err)
	}

	// Run a registration server and registers plugin with kubelet
	if err := plugin.StartRegistrar(ex.driverName, ex.draAddress, ex.pluginRegistrationPath); err != nil {
		return fmt.Errorf("failed to start registrar: %v", err)
	}

	return nil

}

func (ex *examplePlugin) NodePrepareResource(ctx context.Context, req *drapbv1.NodePrepareResourceRequest) (*drapbv1.NodePrepareResourceResponse, error) {

	kind := ex.driverName + "/" + deviceName

	klog.Infof("NodePrepareResource is called: request: %+v", req)

	klog.Infof("Creating CDI File")
	// make resourceHandle in the form of map[string]string
	var decodedResourceHandle map[string]string
	if err := json.Unmarshal([]byte(req.ResourceHandle), &decodedResourceHandle); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resourceHandle: %v", err)
	}

	// create ContainerEdits in device spec and ContainerEdits
	envs := []string{}
	for key, val := range decodedResourceHandle {
		envs = append(envs, key+"="+val)
	}
	// create spec
	spec := specs.Spec{
		Version: cdiVersion,
		Kind:    kind,
		Devices: []specs.Device{{
			Name: deviceName,
			ContainerEdits: specs.ContainerEdits{
				Env: envs,
			},
		}},
		ContainerEdits: specs.ContainerEdits{},
	}
	cdiSpec, err := cdiapi.NewSpec(&spec, ex.getJsonFilePath(req.ClaimUid), 1)
	if err != nil {
		return nil, fmt.Errorf("failed to create new spec: %v", err)
	}
	klog.V(5).InfoS("CDI spec is validated")

	// create bytes from spec, which would be written into the json file
	jsonBytes, err := json.Marshal(*cdiSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CDI spec: %v", err)
	}

	// create json file
	filePath := ex.getJsonFilePath(req.ClaimUid)
	if err := os.WriteFile(filePath, jsonBytes, os.FileMode(0644)); err != nil {
		return nil, fmt.Errorf("failed to write CDI file %v", err)
	}
	klog.Infof("CDI file is created at " + filePath)

	dev := cdiSpec.GetDevice(deviceName).GetQualifiedName()
	resp := &drapbv1.NodePrepareResourceResponse{CdiDevice: []string{dev}}
	klog.Infof("NodePrepareResource is successfuly finished, response: %+v", resp)
	return resp, nil
}

func (ex *examplePlugin) NodeUnprepareResource(ctx context.Context, req *drapbv1.NodeUnprepareResourceRequest) (*drapbv1.NodeUnprepareResourceResponse, error) {
	klog.Infof("NodeUnprepareResource is called: request: %+v", req)
	filePath := ex.getJsonFilePath(req.ClaimUid)
	if err := os.Remove(filePath); err != nil {
		return nil, fmt.Errorf("error removing CDI file: %v", err)
	}
	klog.Infof("CDI file at %s is removed" + filePath)
	return &drapbv1.NodeUnprepareResourceResponse{}, nil
}
