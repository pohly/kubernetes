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
	"flag"
)

// Options is a subset of the LoggingConfiguration that can be applied to
// logging during unit testing.
type Options struct {
	// Verbosity is the threshold for log messages. Only messages
	// with a verbosity >= this threshold get captured.
	Verbosity int
}

// NewOptions returns a configuration with recommended
// defaults.
func NewOptions() Options {
	return Options{
		// For "the steps leading up to errors and warnings" and "troubleshooting"
		// https://github.com/kubernetes/community/blob/9406b4352fe2d5810cb21cc3cb059ce5886de157/contributors/devel/sig-instrumentation/logging.md#logging-conventions
		Verbosity: 5,
	}
}

// AddFlags needs to be called by TestMain to add logging command line options.
func (o *Options) AddFlags(fs *flag.FlagSet) {
	fs.IntVar(&o.Verbosity, "v", o.Verbosity, "number for the log level verbosity")
}

// Validate should be called after command line parsing to check that the
// selected options are usable.
func (o *Options) Validate() error {
	return nil
}
