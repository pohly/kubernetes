/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"k8s.io/component-base/cli"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
	"k8s.io/klogr"

	_ "k8s.io/component-base/logs/json/register"
)

func main() {
	command := NewLoggerCommand()

	// Intentionally broken: logging is not initialized yet.
	klogr.TODO().Info("Oops, I shouldn't be logging yet!")

	code := cli.Run(command)
	os.Exit(code)
}

func NewLoggerCommand() *cobra.Command {
	o := logs.NewOptions()
	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			if err := o.ValidateAndApply(); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			// Initialize contextual logging.
			logger := klogr.Background().WithName("example").WithValues("foo", "bar")
			ctx := klogr.NewContext(context.Background(), logger)

			runLogger(ctx)
		},
	}
	o.AddFlags(cmd.Flags())
	return cmd
}

func runLogger(ctx context.Context) {
	fmt.Println("This is normal output via stdout.")
	fmt.Fprintln(os.Stderr, "This is other output via stderr.")
	klog.Infof("Log using Infof, key: %s", "value")
	klog.InfoS("Log using InfoS", "key", "value")
	err := errors.New("fail")
	klog.Errorf("Log using Errorf, err: %v", err)
	klog.ErrorS(err, "Log using ErrorS")
	data := SensitiveData{Key: "secret"}
	klog.Infof("Log with sensitive key, data: %q", data)
	klog.V(1).Info("Log less important message")

	// This is the fallback that can be used if neither logger nor context
	// are available... but it's better to pass some kind of parameter.
	klogr.TODO().Info("Now the default logger is set, but using the one from the context is still better.")

	logger := klogr.FromContext(ctx)
	logger.Info("Log sensitive data through context", "data", data)

	// This intentionally uses the same key/value multiple times. Only the
	// second example could be detected via static code analysis.
	logger.WithName("myname").WithValues("duration", time.Hour).Info("runtime", "duration", time.Minute)
	logger.Info("another runtime", "duration", time.Hour, "duration", time.Minute)
}

type SensitiveData struct {
	Key string `json:"key" datapolicy:"secret-key"`
}
