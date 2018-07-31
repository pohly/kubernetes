/*
Copyright 20158 The Kubernetes Authors.

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

package framework

import (
	"fmt"

	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// ProviderInterface contains the implementation for certain
// provider-specific functionality.
type ProviderInterface interface {
	FrameworkBeforeEach(f *Framework)
	FrameworkAfterEach(f *Framework)

	ResizeGroup(group string, size int32) error
	GetGroupNodes(group string) ([]string, error)
	GroupSize(group string) (int, error)

	EnsureLoadBalancerResourcesDeleted(ip, portRange string) error

	CreatePD(zone string) (string, error)
	DeletePD(pdName string) error
	CreatePVSource(zone, diskName string) (*v1.PersistentVolumeSource, error)
	DeletePVSource(pvSource *v1.PersistentVolumeSource) error

	CleanupServiceResources(c clientset.Interface, loadBalancerName, region, zone string)

	LoadBalancerSrcRanges() []string
}

// NullProvider is the default implementation of the ProviderInterface
// which doesn't do anything.
type NullProvider struct{}

func (n NullProvider) FrameworkBeforeEach(f *Framework) {}
func (n NullProvider) FrameworkAfterEach(f *Framework)  {}

func (n NullProvider) ResizeGroup(string, int32) error {
	return fmt.Errorf("Provider does not support InstanceGroups")
}
func (n NullProvider) GetGroupNodes(group string) ([]string, error) {
	return nil, fmt.Errorf("provider does not support InstanceGroups")
}
func (n NullProvider) GroupSize(group string) (int, error) {
	return -1, fmt.Errorf("provider does not support InstanceGroups")
}

func (n NullProvider) EnsureLoadBalancerResourcesDeleted(ip, portRange string) error {
	return nil
}

func (n NullProvider) CreatePD(zone string) (string, error) {
	return "", fmt.Errorf("provider does not support volume creation")
}
func (n NullProvider) DeletePD(pdName string) error {
	return fmt.Errorf("provider does not support volume deletion")
}
func (n NullProvider) CreatePVSource(zone, diskName string) (*v1.PersistentVolumeSource, error) {
	return nil, fmt.Errorf("Provider not supported")
}
func (n NullProvider) DeletePVSource(pvSource *v1.PersistentVolumeSource) error {
	return fmt.Errorf("Provider not supported")
}

func (n NullProvider) CleanupServiceResources(c clientset.Interface, loadBalancerName, region, zone string) {
}

func (n NullProvider) LoadBalancerSrcRanges() []string {
	return nil
}

var _ ProviderInterface = NullProvider{}
