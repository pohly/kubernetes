/*
Copyright 2021 The Kubernetes Authors.

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

// Package example shows how to initialize per-test log output.
package example

import (
	"context"
	"testing"

	"k8s.io/klog/v2"
	"k8s.io/klog/v2/ktesting"

	// Add flags for testing logger.
	_ "k8s.io/klog/v2/ktesting/init"

	// Add flags for klog.
	_ "k8s.io/component-base/logs/testinit"
)

func TestContext(t *testing.T) {
	logger, ctx := ktesting.NewTestContext(t)
	logger.V(5).Info("The default log level during tests is 5.")
	doSomething(ctx, "TestContext")
}

func TestNoContext(t *testing.T) {
	// The output will go through klog and only be visible when running the
	// test with -v=5.
	doSomething(context.Background(), "TestNoContext")
}

func doSomething(ctx context.Context, prefix string) {
	logger := klog.FromContext(ctx)
	logger.V(5).WithName(prefix).Info("This output is done through a logger retrieved via klog.FromContext.")
}
