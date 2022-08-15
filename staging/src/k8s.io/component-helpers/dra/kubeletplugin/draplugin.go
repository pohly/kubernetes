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

package kubeletplugin

import (
	"fmt"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1alpha1"
)

type DRAPlugin interface {
	// Stop ensures that all spawned goroutines are stopped and frees
	// resources.
	Stop()

	// This unexported method ensures that we can modify the interface
	// without causing an API break of the package
	// (https://pkg.go.dev/golang.org/x/exp/apidiff#section-readme).
	internal()
}

// draPlugin combines the kubelet registration service and the DRA node plugin
// service.
type draPlugin struct {
	registrar *nodeRegistrar
	plugin    *grpcServer
}

// Start sets up two gRPC servers (one for registration, one for the DRA node
// client).
func Start(logger klog.Logger, driverName, endpoint, draAddress, pluginRegistrationPath string, nodeServer drapbv1.NodeServer) (result DRAPlugin, finalErr error) {
	d := &draPlugin{}

	// Run the node plugin gRPC server first to ensure that it is ready.
	plugin, err := startGRPCServer(klog.LoggerWithName(logger, "dra"), endpoint, func(grpcServer *grpc.Server) {
		drapbv1.RegisterNodeServer(grpcServer, nodeServer)
	})
	if err != nil {
		return nil, fmt.Errorf("start node client: %v", err)
	}
	d.plugin = plugin
	defer func() {
		// Clean up if we didn't finish succcessfully.
		if finalErr != nil {
			plugin.stop()
		}
	}()

	// Now make it available to kubelet.
	registrar, err := startRegistrar(klog.LoggerWithName(logger, "registrar"), driverName, draAddress, pluginRegistrationPath)
	if err != nil {
		return nil, fmt.Errorf("start registrar: %v", err)
	}
	d.registrar = registrar

	return d, nil
}

func (d *draPlugin) Stop() {
	if d == nil {
		return
	}
	d.registrar.stop()
	d.plugin.stop()
}

func (d *draPlugin) internal() {}
