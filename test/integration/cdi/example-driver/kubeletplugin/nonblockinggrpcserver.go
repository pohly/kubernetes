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
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubernetes/pkg/kubelet/apis/dra/v1alpha1"
)

// NonBlocking server
type nonBlockingGRPCServer struct {
	wg      sync.WaitGroup
	server  *grpc.Server
	cleanup func()
}

func (s *nonBlockingGRPCServer) Start(endpoint string, ns drapbv1.NodeServer) {
	s.wg.Add(1)

	s.wg.Done()
	go s.serve(endpoint, ns)
}

func (s *nonBlockingGRPCServer) Wait() {
	s.wg.Wait()
}

func (s *nonBlockingGRPCServer) serve(ep string, ns drapbv1.NodeServer) {
	listener, cleanup, err := listen(ep)
	if err != nil {
		klog.Infof("Failed to listen: %v", err)
	}

	opts := []grpc.ServerOption{}
	server := grpc.NewServer(opts...)
	s.server = server
	s.cleanup = cleanup

	if ns != nil {
		drapbv1.RegisterNodeServer(server, ns)
	}

	klog.Infof("Nonblocking grpc server started at %s\n", ep)
	server.Serve(listener)
}

func parse(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
		return "", "", fmt.Errorf("invalid endpoint: %v", ep)
	}
	// Assume everything else is a file path for a Unix Domain Socket.
	return "unix", ep, nil
}

func listen(endpoint string) (net.Listener, func(), error) {
	proto, addr, err := parse(endpoint)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {}
	if proto == "unix" {
		addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) { //nolint: vetshadow
			return nil, nil, fmt.Errorf("%s: %q", addr, err)
		}
		cleanup = func() {
			os.Remove(addr)
		}
	}

	l, err := net.Listen(proto, addr)
	return l, cleanup, err
}

func newNonBlockingGRPCServer() *nonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// StartNonblockingGrpcServer starts a grpc server for a driver, that listens on draAddress.
func StartNonblockingGrpcServer(draAddress string, driver drapbv1.NodeServer) error {
	klog.Infof("Starting nonblocking grpc server at %s\n", draAddress)
	s := newNonBlockingGRPCServer()

	s.Start(draAddress, driver)
	s.Wait()
	return nil
}
