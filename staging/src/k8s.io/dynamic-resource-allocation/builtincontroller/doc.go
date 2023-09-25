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

// Package simulation defines how vendors can extend the scheduler plugin for
// dynamic resource allocation so that it can accurately model the effect of
// allocating claims inside a cluster. This is needed for the Cluster
// AutoScaler.
//
// Vendor extensions currently have be built into a custom Cluster AutoScaler
// binary. Extensions mechanisms that work without recompiling the Cluster
// AutoScaler will be added in the future.
//
// The easiest way to add one or more extensions is to create a drasimulation.go
// file alongside main.go which imports vendor packages:
//
//     package main
//     import (
//          _ "example.com/driverA/simulation-plugin/init"
//          _ "example.com/driverB/simulation-plugin/init"
//     )
//
// These "init" packages must have an init function which registers the
// vendor's plugin:
//
//     package init
//     import (
//          "k8s.io/dynamic-resource-allocation/simulation"
//          "example.com/driverA/simulation-plugin"
//     )
//
//     func init() {
//         simulation.Registry.Add(simulationplugin.Plugin{})
//     }
//
// Without a vendor extension, the scheduler plugin will assume that a claim
// can be allocated so that it will be available on any node in the cluster.
// The effect is that Cluster AutoScaler will .... ?
package builtincontroller
