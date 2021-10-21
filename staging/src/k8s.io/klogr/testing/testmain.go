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

package testing

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr"
)

// DefaultOptions is the global default logging configuration
// for a unit test. It is used by NewTestContext and TestMainWithLogging.
var DefaultOptions = NewOptions()

// NewTestContext returns a logger and context for use in a unit test case or
// benchmark. The tl parameter can be a testing.T or testing.B pointer that
// will receive all log output. TestMainWithLogging can be used to enable
// command line flags that modify the configuration of that log output.
func NewTestContext(tl TL) (logr.Logger, context.Context) {
	logger := NewLogger(tl, DefaultOptions)
	ctx := logr.NewContext(context.Background(), logger)
	return logger, ctx

}

// TestMainWithLogging can be used as a test's TestMain implementation.
// It adds, parses and validates logging command line options before
// running tests.
func TestMainWithLogging(m *testing.M) {
	DefaultOptions.AddFlags(flag.CommandLine)
	flag.Parse()
	if err := DefaultOptions.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
	}
	os.Exit(m.Run())
}
