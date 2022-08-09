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
	"context"
	"fmt"

	"k8s.io/klog/v2"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
)

// registrationServer is a sample plugin to work with plugin watcher.
type registrationServer struct {
	driverName string
	endpoint   string
	version    []string
}

// newRegistrationServer returns an initialized registrationServer instance.
func newRegistrationServer(driverName string, endpoint string, versions []string) registerapi.RegistrationServer {
	return &registrationServer{
		driverName: driverName,
		endpoint:   endpoint,
		version:    versions,
	}
}

var _ registerapi.RegistrationServer = registrationServer{}

// GetInfo is the RPC invoked by plugin watcher.
func (e registrationServer) GetInfo(ctx context.Context, req *registerapi.InfoRequest) (*registerapi.PluginInfo, error) {
	klog.Infof("Received GetInfo call: %+v", req)

	return &registerapi.PluginInfo{
		Type:              registerapi.DRAPlugin,
		Name:              e.driverName,
		Endpoint:          e.endpoint,
		SupportedVersions: e.version,
	}, nil
}

// NotifyRegistrationStatus is the RPC invoked by plugin watcher.
func (e registrationServer) NotifyRegistrationStatus(ctx context.Context, status *registerapi.RegistrationStatus) (*registerapi.RegistrationStatusResponse, error) {
	klog.Infof("Received NotifyRegistrationStatus call: %+v", status)
	if !status.PluginRegistered {
		return nil, fmt.Errorf("failed registration process: %+v", status.Error)
	}

	return &registerapi.RegistrationStatusResponse{}, nil
}
