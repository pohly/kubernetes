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

package klogr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"k8s.io/klogr/logger"
)

// TestOutput ensures that direct calls into klog and indirect calls via the
// proxy logger produce the same output.
func TestOutput(t *testing.T) {
	tests := map[string]struct {
		withHelper     bool
		withNames      []string
		withValues     []interface{}
		v              int
		text           string
		values         []interface{}
		err            error
		expectedOutput string
	}{
		"log with values": {
			text:   "test",
			values: []interface{}{"akey", "avalue"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" akey="avalue"
`,
		},
		"call depth": {
			text:       "helper",
			withHelper: true,
			values:     []interface{}{"akey", "avalue"},
			expectedOutput: `I klogr_test.go:<LINE>] "helper" akey="avalue"
`,
		},
		"verbosity": {
			text: "you don't see me",
			v:    11,
		},
		"log with name and values": {
			withNames: []string{"me"},
			text:      "test",
			values:    []interface{}{"akey", "avalue"},
			expectedOutput: `I klogr_test.go:<LINE>] "me: test" akey="avalue"
`,
		},
		"log with multiple names and values": {
			withNames: []string{"hello", "world"},
			text:      "test",
			values:    []interface{}{"akey", "avalue"},
			expectedOutput: `I klogr_test.go:<LINE>] "hello/world: test" akey="avalue"
`,
		},
		"print duplicate keys in arguments": {
			text:   "test",
			values: []interface{}{"akey", "avalue", "akey", "avalue2"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" akey="avalue" akey="avalue2"
`,
		},
		"override single value": {
			withValues: []interface{}{"akey", "avalue"},
			text:       "test",
			values:     []interface{}{"akey", "avalue2"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" akey="avalue2"
`,
		},
		"override WithValues": {
			withValues: []interface{}{"duration", time.Hour, "X", "y"},
			text:       "test",
			values:     []interface{}{"duration", time.Minute, "A", "b"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" X="y" duration="1m0s" A="b"
`,
		},
		"duplicates": {
			text:   "test",
			values: []interface{}{"duration", time.Minute, "A", "b", "duration", time.Hour},
			expectedOutput: `I klogr_test.go:<LINE>] "test" duration="1m0s" A="b" duration="1h0m0s"
`,
		},
		"preserve order of key/value pairs": {
			withValues: []interface{}{"akey9", "avalue9", "akey8", "avalue8", "akey1", "avalue1"},
			text:       "test",
			values:     []interface{}{"akey5", "avalue5", "akey4", "avalue4"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" akey9="avalue9" akey8="avalue8" akey1="avalue1" akey5="avalue5" akey4="avalue4"
`,
		},
		"handle odd-numbers of KVs": {
			text:   "test",
			values: []interface{}{"akey", "avalue", "akey2"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" akey="avalue" akey2="(MISSING)"
`,
		},
		"html characters": {
			text:   "test",
			values: []interface{}{"akey", "<&>"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" akey="<&>"
`,
		},
		"quotation": {
			text:   `"quoted"`,
			values: []interface{}{"key", `"quoted value"`},
			expectedOutput: `I klogr_test.go:<LINE>] "\"quoted\"" key="\"quoted value\""
`,
		},
		"handle odd-numbers of KVs in both log values and Info args": {
			withValues: []interface{}{"basekey1", "basevar1", "basekey2"},
			text:       "test",
			values:     []interface{}{"akey", "avalue", "akey2"},
			expectedOutput: `I klogr_test.go:<LINE>] "test" basekey1="basevar1" basekey2="(MISSING)" akey="avalue" akey2="(MISSING)"
`,
		},
		"KObj": {
			text:   "test",
			values: []interface{}{"pod", KObj(&kmeta{Name: "pod-1", Namespace: "kube-system"})},
			expectedOutput: `I klogr_test.go:<LINE>] "test" pod="kube-system/pod-1"
`,
		},
		"KObjs": {
			text: "test",
			values: []interface{}{"pods",
				KObjs([]interface{}{
					&kmeta{Name: "pod-1", Namespace: "kube-system"},
					&kmeta{Name: "pod-2", Namespace: "kube-system"},
				})},
			expectedOutput: `I klogr_test.go:<LINE>] "test" pods=[kube-system/pod-1 kube-system/pod-2]
`,
		},
		"regular error types as value": {
			text:   "test",
			values: []interface{}{"err", errors.New("whoops")},
			expectedOutput: `I klogr_test.go:<LINE>] "test" err="whoops"
`,
		},
		"ignore MarshalJSON": {
			text:   "test",
			values: []interface{}{"err", &customErrorJSON{"whoops"}},
			expectedOutput: `I klogr_test.go:<LINE>] "test" err="whoops"
`,
		},
		"regular error types when using logr.Error": {
			text: "test",
			err:  errors.New("whoops"),
			expectedOutput: `E klogr_test.go:<LINE>] "test" err="whoops"
`,
		},
	}
	for n, test := range tests {
		t.Run(n, func(t *testing.T) {
			printWithLogger := func(logger logr.Logger) {
				for _, name := range test.withNames {
					logger = logger.WithName(name)
				}
				logger = logger.WithValues(test.withValues...)
				if test.withHelper {
					loggerHelper(logger, test.text, test.values)
				} else if test.err != nil {
					logger.Error(test.err, test.text, test.values...)
				} else {
					logger.V(test.v).Info(test.text, test.values...)
				}
			}
			_, _, printWithLoggerLine, _ := runtime.Caller(0)

			testOutput := func(t *testing.T, expectedLine int, print func(buffer *bytes.Buffer)) {
				var tmpWriteBuffer bytes.Buffer
				print(&tmpWriteBuffer)

				actual := tmpWriteBuffer.String()
				// Strip varying header.
				re := `^(.).... ..:..:......... ....... klogr_test.go`
				actual = regexp.MustCompile(re).ReplaceAllString(actual, `${1} klogr_test.go`)

				// Inject expected line. This matches the `if test.err != nil` check above.
				if test.withHelper {
					expectedLine -= 7
				} else if test.err != nil {
					expectedLine -= 5
				} else {
					expectedLine -= 3
				}
				expected := test.expectedOutput
				expected = strings.ReplaceAll(expected, "<LINE>", fmt.Sprintf("%d", expectedLine))
				if actual != expected {
					t.Errorf("Output mismatch. Expected:\n%s\nActual:\n%s\n", expected, actual)
				}
			}

			t.Run("logger", func(t *testing.T) {
				testOutput(t, printWithLoggerLine, func(buffer *bytes.Buffer) {
					printWithLogger(logger.New(
						logger.Options{
							Output: buffer,
							V:      10,
						}))
				})
			})
		})
	}
}

type kmeta struct {
	Name, Namespace string
}

func (k kmeta) GetName() string {
	return k.Name
}

func (k kmeta) GetNamespace() string {
	return k.Namespace
}

var _ KMetadata = kmeta{}

type customErrorJSON struct {
	s string
}

var _ error = &customErrorJSON{}
var _ json.Marshaler = &customErrorJSON{}

func (e *customErrorJSON) Error() string {
	return e.s
}

func (e *customErrorJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToUpper(e.s))
}

func loggerHelper(logger logr.Logger, msg string, kv []interface{}) {
	logger = logger.WithCallDepth(1)
	logger.Info(msg, kv...)
}
