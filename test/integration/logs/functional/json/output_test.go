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
			ExpectedOutputMapping: test.ZaprOutputMappingDirect(),
		})
	})

	t.Run("klog-backend", func(t *testing.T) {
		test.Output(t, test.OutputConfig{
			NewLogger:             newLogger,
			AsBackend:             true,
			ExpectedOutputMapping: test.ZaprOutputMappingIndirect(),
		})
	})
}
