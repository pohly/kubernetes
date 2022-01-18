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

package v1

import (
	"bytes"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/component-base/featuregate"
)

func TestFlags(t *testing.T) {
	c := NewLoggingConfiguration()
	fs := pflag.NewFlagSet("addflagstest", pflag.ContinueOnError)
	output := bytes.Buffer{}
	c.AddFlags(fs)
	fs.SetOutput(&output)
	fs.PrintDefaults()
	want := `      --log-flush-frequency duration   Maximum number of seconds between log flushes (default 5s)
      --logging-format string          Sets the log format. Permitted formats: "text".
                                       Non-default formats don't honor these flags: --add-dir-header, --alsologtostderr, --log-backtrace-at, --log-dir, --log-file, --log-file-max-size, --logtostderr, --one-output, --skip-headers, --skip-log-headers, --stderrthreshold, --vmodule.
                                       Non-default choices are currently alpha and subject to change without warning. (default "text")
  -v, --v Level                        number for the log level verbosity
      --vmodule pattern=N,...          comma-separated list of pattern=N settings for file-filtered logging (only works for text log format)
`
	if !assert.Equal(t, want, output.String()) {
		t.Errorf("Wrong list of flags. expect %q, got %q", want, output.String())
	}
}

func TestOptions(t *testing.T) {
	newOptions := NewLoggingConfiguration()
	testcases := []struct {
		name        string
		args        []string
		featureGate featuregate.FeatureGate
		want        *LoggingConfiguration
		errs        field.ErrorList
	}{
		{
			name: "Default log format",
			want: newOptions.DeepCopy(),
		},
		{
			name: "Text log format",
			args: []string{"--logging-format=text"},
			want: newOptions.DeepCopy(),
		},
		{
			name: "Unsupported log format",
			args: []string{"--logging-format=test"},
			want: func() *LoggingConfiguration {
				c := newOptions.DeepCopy()
				c.Format = "test"
				return c
			}(),
			errs: field.ErrorList{&field.Error{
				Type:     "FieldValueInvalid",
				Field:    "format",
				BadValue: "test",
				Detail:   "Unsupported log format",
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewLoggingConfiguration()
			fs := pflag.NewFlagSet("addflagstest", pflag.ContinueOnError)
			c.AddFlags(fs)
			fs.Parse(tc.args)
			if !assert.Equal(t, tc.want, c) {
				t.Errorf("Wrong Validate() result for %q. expect %v, got %v", tc.name, tc.want, c)
			}
			errs := c.ValidateAndApply(defaultGates)
			if !assert.ElementsMatch(t, tc.errs, errs) {
				t.Errorf("Wrong Validate() result for %q.\n expect:\t%+v\n got:\t%+v", tc.name, tc.errs, errs)
			}
		})
	}
}
