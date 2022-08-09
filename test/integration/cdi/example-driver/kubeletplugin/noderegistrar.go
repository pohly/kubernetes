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

// Package app does all of the work necessary to configure and run a
// Kubernetes app process.
package kubeletplugin

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
)

type nodeRegistrarConfig struct {
	draDriverName          string
	draAddress             string
	pluginRegistrationPath string
}
type nodeRegistrar struct {
	config nodeRegistrarConfig
}

func buildSocketPath(cdiDriverName string, pluginRegistrationPath string) string {
	return fmt.Sprintf("%s/%s-reg.sock", pluginRegistrationPath, cdiDriverName)
}

func removeRegSocket(cdiDriverName, pluginRegistrationPath string) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	<-sigc
	socketPath := buildSocketPath(cdiDriverName, pluginRegistrationPath)
	err := os.Remove(socketPath)
	if err != nil && !os.IsNotExist(err) {
		klog.Errorf("failed to remove socket: %s with error: %+v", socketPath, err)
	}
}

func cleanupSocketFile(socketPath string) error {
	if err := os.Remove(socketPath); err == nil {
		return nil
	} else if errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return fmt.Errorf("unexpected error removing socket: %v", err)
	}
}

// newRegistrar returns an initialized nodeRegistrar instance.
func newRegistrar(config nodeRegistrarConfig) *nodeRegistrar {
	return &nodeRegistrar{
		config: config,
	}
}

// nodeRegister runs a registration server and registers plugin with kubelet.
func (nr nodeRegistrar) nodeRegister() error {
	// Run a registration server.
	registrationServer := newRegistrationServer(nr.config.draDriverName, nr.config.draAddress, []string{"1.0.0"})
	socketPath := buildSocketPath(nr.config.draDriverName, nr.config.pluginRegistrationPath)
	if err := cleanupSocketFile(socketPath); err != nil {
		return fmt.Errorf(err.Error())
	}

	var oldmask int
	if runtime.GOOS == "linux" {
		oldmask, _ = util.Umask(0077)
	}

	klog.Infof("Starting registration server at: %s\n", socketPath)
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %s with error: %+v", socketPath, err)
	}

	if runtime.GOOS == "linux" {
		util.Umask(oldmask)
	}
	grpcServer := grpc.NewServer()

	// Register kubelet plugin watcher api.
	registerapi.RegisterRegistrationServer(grpcServer, registrationServer)

	go removeRegSocket(nr.config.draDriverName, nr.config.pluginRegistrationPath)
	// Start service.
	klog.Infof("Registration server started at: %s\n", socketPath)
	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("registration server stopped serving: %v", err)
	}
	return nil
}

// StartRegistrar creates a new registrar, and run the function nodeRegister.
func StartRegistrar(driverName, draAddress, pluginRegistrationPath string) error {
	klog.Infof("Starting noderegistrar")
	registrarConfig := nodeRegistrarConfig{
		draDriverName:          driverName,
		draAddress:             draAddress,
		pluginRegistrationPath: pluginRegistrationPath,
	}
	registrar := newRegistrar(registrarConfig)
	if err := registrar.nodeRegister(); err != nil {
		return fmt.Errorf("failed to register node: %v", err)
	}

	return nil
}
