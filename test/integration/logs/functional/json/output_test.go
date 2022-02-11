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

package json

import (
	"io"
	"testing"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"

	"k8s.io/component-base/config"
	logsjson "k8s.io/component-base/logs/json"
	"k8s.io/klog/v2/test"
)

func init() {
	test.InitKlog()
}

// This is how JSON output looks like when the JSON logger gets called directly,
// without going through klog.
var expectedOutputMappingDirect = map[string]string{
	`I output.go:<LINE>] "test" akey="<&>"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"akey":"<&>"}
`,

	`I output.go:<LINE>] "test" basekey1="basevar1" basekey2="(MISSING)" akey="avalue" akey2="(MISSING)"
`: `{"caller":"test/output.go:<WITH-VALUES>","msg":"odd number of arguments passed as key-value pairs for logging","ignored key":"basekey2"}
{"caller":"test/output.go:<LINE>","msg":"odd number of arguments passed as key-value pairs for logging","basekey1":"basevar1","ignored key":"akey2"}
{"caller":"test/output.go:<LINE>","msg":"test","basekey1":"basevar1","v":0,"akey":"avalue"}
`,

	`E output.go:<LINE>] "test" err="whoops"
`: `{"caller":"test/output.go:<LINE>","msg":"test","err":"whoops"}
`,

	`I output.go:<LINE>] "helper" akey="avalue"
`: `{"caller":"test/output.go:<LINE>","msg":"helper","v":0,"akey":"avalue"}
`,

	`I output.go:<LINE>] "hello/world: test" akey="avalue"
`: `{"logger":"hello.world","caller":"test/output.go:<LINE>","msg":"test","v":0,"akey":"avalue"}
`,

	`I output.go:<LINE>] "test" X="y" duration="1m0s" A="b"
`: `{"caller":"test/output.go:<LINE>","msg":"test","duration":"1h0m0s","X":"y","v":0,"duration":"1m0s","A":"b"}
`,

	`I output.go:<LINE>] "test" keyWithoutValue="(MISSING)"
I output.go:<LINE>] "test" keyWithoutValue="(MISSING)" anotherKeyWithoutValue="(MISSING)"
I output.go:<LINE>] "test" keyWithoutValue="(MISSING)"
`: `{"caller":"test/output.go:<WITH-VALUES>","msg":"odd number of arguments passed as key-value pairs for logging","ignored key":"keyWithoutValue"}
{"caller":"test/output.go:<WITH-VALUES-2>","msg":"odd number of arguments passed as key-value pairs for logging","ignored key":"anotherKeyWithoutValue"}
{"caller":"test/output.go:<LINE>","msg":"test","v":0}
{"caller":"test/output.go:<LINE>","msg":"test","v":0}
{"caller":"test/output.go:<LINE>","msg":"test","v":0}
`,

	`I output.go:<LINE>] "test" akey9="avalue9" akey8="avalue8" akey1="avalue1" akey5="avalue5" akey4="avalue4"
`: `{"caller":"test/output.go:<LINE>","msg":"test","akey9":"avalue9","akey8":"avalue8","akey1":"avalue1","v":0,"akey5":"avalue5","akey4":"avalue4"}
`,

	`I output.go:<LINE>] "test"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0}
`,

	`I output.go:<LINE>] "\"quoted\"" key="\"quoted value\""
`: `{"caller":"test/output.go:<LINE>","msg":"\"quoted\"","v":0,"key":"\"quoted value\""}
`,

	`I output.go:<LINE>] "test" err="whoops"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"err":"whoops"}
`,

	`I output.go:<LINE>] "test" pod="kube-system/pod-1"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"pod":{"name":"pod-1","namespace":"kube-system"}}
`,

	`I output.go:<LINE>] "test" pods=[kube-system/pod-1 kube-system/pod-2]
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"pods":[{"name":"pod-1","namespace":"kube-system"},{"name":"pod-2","namespace":"kube-system"}]}
`,

	`I output.go:<LINE>] "test" akey="avalue"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"akey":"avalue"}
`,

	`I output.go:<LINE>] "me: test" akey="avalue"
`: `{"logger":"me","caller":"test/output.go:<LINE>","msg":"test","v":0,"akey":"avalue"}
`,

	`I output.go:<LINE>] "test" akey="avalue2"
`: `{"caller":"test/output.go:<LINE>","msg":"test","akey":"avalue","v":0,"akey":"avalue2"}
`,

	`I output.go:<LINE>] "test" akey="avalue" akey2="(MISSING)"
`: `{"caller":"test/output.go:<LINE>","msg":"odd number of arguments passed as key-value pairs for logging","ignored key":"akey2"}
{"caller":"test/output.go:<LINE>","msg":"test","v":0,"akey":"avalue"}
`,

	`I output.go:<LINE>] "you see me"
`: `{"caller":"test/output.go:<LINE>","msg":"you see me","v":9}
`,

	`I output.go:<LINE>] "test" firstKey=1
I output.go:<LINE>] "test" firstKey=1 secondKey=2
I output.go:<LINE>] "test" firstKey=1
I output.go:<LINE>] "test" firstKey=1 secondKey=3
`: `{"caller":"test/output.go:<LINE>","msg":"test","firstKey":1,"v":0}
{"caller":"test/output.go:<LINE>","msg":"test","firstKey":1,"secondKey":2,"v":0}
{"caller":"test/output.go:<LINE>","msg":"test","firstKey":1,"v":0}
{"caller":"test/output.go:<LINE>","msg":"test","firstKey":1,"secondKey":3,"v":0}
`,

	// klog.Info
	`I output.go:<LINE>] "helloworld\n"
`: `{"caller":"test/output.go:<LINE>","msg":"helloworld\n","v":0}
`,

	// klog.Infoln
	`I output.go:<LINE>] "hello world\n"
`: `{"caller":"test/output.go:<LINE>","msg":"hello world\n","v":0}
`,

	// klog.Error
	`E output.go:<LINE>] "helloworld\n"
`: `{"caller":"test/output.go:<LINE>","msg":"helloworld\n"}
`,

	// klog.Errorln
	`E output.go:<LINE>] "hello world\n"
`: `{"caller":"test/output.go:<LINE>","msg":"hello world\n"}
`,

	// klog.ErrorS
	`E output.go:<LINE>] "world" err="hello"
`: `{"caller":"test/output.go:<LINE>","msg":"world","err":"hello"}
`,

	// klog.InfoS
	`I output.go:<LINE>] "hello" what="world"
`: `{"caller":"test/output.go:<LINE>","msg":"hello","v":0,"what":"world"}
`,

	// klog.V(1).Info
	`I output.go:<LINE>] "hellooneworld\n"
`: `{"caller":"test/output.go:<LINE>","msg":"hellooneworld\n","v":1}
`,

	// klog.V(1).Infoln
	`I output.go:<LINE>] "hello one world\n"
`: `{"caller":"test/output.go:<LINE>","msg":"hello one world\n","v":1}
`,

	// klog.V(1).ErrorS
	`E output.go:<LINE>] "one world" err="hello"
`: `{"caller":"test/output.go:<LINE>","msg":"one world","err":"hello"}
`,

	// klog.V(1).InfoS
	`I output.go:<LINE>] "hello" what="one world"
`: `{"caller":"test/output.go:<LINE>","msg":"hello","v":1,"what":"one world"}
`,
}

// When using the JSON logger as backend for klog, some test cases are
// different:
// - WithName gets added to the message by TestJSONOutput.
// - WithValues are part of the normal key/value parameters,
//   which puts them after "v".
// - There are no separate warnings about WithValue parameters.
// - zap uses . as separator instead of / between WithName values.
// - zap drops keys with missing values, here we get "(MISSING)".
// - zap does not de-duplicate key/value pairs, here klog does that
//   for it.
var expectedOutputMappingBackend = map[string]string{
	`I output.go:<LINE>] "hello/world: test" akey="avalue"
`: `{"caller":"test/output.go:<LINE>","msg":"hello/world: test","v":0,"akey":"avalue"}
`,

	`I output.go:<LINE>] "me: test" akey="avalue"
`: `{"caller":"test/output.go:<LINE>","msg":"me: test","v":0,"akey":"avalue"}
`,

	`I output.go:<LINE>] "test" basekey1="basevar1" basekey2="(MISSING)" akey="avalue" akey2="(MISSING)"
`: `{"caller":"test/output.go:<LINE>","msg":"odd number of arguments passed as key-value pairs for logging","ignored key":"akey2"}
{"caller":"test/output.go:<LINE>","msg":"test","v":0,"basekey1":"basevar1","basekey2":"(MISSING)","akey":"avalue"}
`,

	`I output.go:<LINE>] "test" keyWithoutValue="(MISSING)"
I output.go:<LINE>] "test" keyWithoutValue="(MISSING)" anotherKeyWithoutValue="(MISSING)"
I output.go:<LINE>] "test" keyWithoutValue="(MISSING)"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"keyWithoutValue":"(MISSING)"}
{"caller":"test/output.go:<LINE>","msg":"test","v":0,"keyWithoutValue":"(MISSING)","anotherKeyWithoutValue":"(MISSING)"}
{"caller":"test/output.go:<LINE>","msg":"test","v":0,"keyWithoutValue":"(MISSING)"}
`,

	`I output.go:<LINE>] "test" akey9="avalue9" akey8="avalue8" akey1="avalue1" akey5="avalue5" akey4="avalue4"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"akey9":"avalue9","akey8":"avalue8","akey1":"avalue1","akey5":"avalue5","akey4":"avalue4"}
`,

	`I output.go:<LINE>] "test" akey="avalue2"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"akey":"avalue2"}
`,

	`I output.go:<LINE>] "test" X="y" duration="1m0s" A="b"
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"X":"y","duration":"1m0s","A":"b"}
`,

	`I output.go:<LINE>] "test" firstKey=1
I output.go:<LINE>] "test" firstKey=1 secondKey=2
I output.go:<LINE>] "test" firstKey=1
I output.go:<LINE>] "test" firstKey=1 secondKey=3
`: `{"caller":"test/output.go:<LINE>","msg":"test","v":0,"firstKey":1}
{"caller":"test/output.go:<LINE>","msg":"test","v":0,"firstKey":1,"secondKey":2}
{"caller":"test/output.go:<LINE>","msg":"test","v":0,"firstKey":1}
{"caller":"test/output.go:<LINE>","msg":"test","v":0,"firstKey":1,"secondKey":3}
`,
}

// TestJsonOutput tests the JSON logger, directly and as backend for klog.
func TestJSONOutput(t *testing.T) {
	newLogger := func(out io.Writer, v int, vmodule string) logr.Logger {
		logger, _ := logsjson.NewJSONLogger(config.VerbosityLevel(v), logsjson.AddNopSync(out), nil,
			&zapcore.EncoderConfig{
				MessageKey:     "msg",
				CallerKey:      "caller",
				NameKey:        "logger",
				EncodeDuration: zapcore.StringDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			})
		return logger
	}

	t.Run("direct", func(t *testing.T) {
		test.Output(t, test.OutputConfig{
			NewLogger:             newLogger,
			ExpectedOutputMapping: expectedOutputMappingDirect,
		})
	})

	t.Run("klog-backend", func(t *testing.T) {
		mapping := map[string]string{}
		copyMap(expectedOutputMappingDirect, mapping)
		copyMap(expectedOutputMappingBackend, mapping)
		test.Output(t, test.OutputConfig{
			NewLogger:             newLogger,
			AsBackend:             true,
			ExpectedOutputMapping: mapping,
		})
	})
}

func copyMap(from, to map[string]string) {
	for key, value := range from {
		to[key] = value
	}
}
