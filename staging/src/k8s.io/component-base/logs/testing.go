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

package logs

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/pflag"
)

// RunTests can be used inside a `TestMain` of some unit test to set up logging
// flags, activate logging, and then run the tests. It never returns.
func RunTests(m *testing.M) {
	// TODO: increase the default log level, but how much?
	o := NewOptions()
	var fs pflag.FlagSet
	o.AddFlags(&fs)
	// https://github.com/spf13/pflag/pull/330 would make this nicer.
	fs.VisitAll(func(f *pflag.Flag) {
		usage := f.Usage
		if f.Deprecated != "" {
			usage = fmt.Sprintf("%s (DEPRECATED: %s)", usage, f.Deprecated)
		}
		flag.CommandLine.Var(f.Value, f.Name, usage)
	})
	flag.Parse()
	if err := o.ValidateAndApply(); err != nil {
		fmt.Fprintf(os.Stderr, "logging configuration failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
