/*
Copyright 2014 The Kubernetes Authors.

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

// The kubelet binary is responsible for maintaining a set of containers on a particular host VM.
// It syncs data from both configuration file(s) as well as from a quorum of etcd servers.
// It then queries Docker to see what is currently running.  It synchronizes the configuration data,
// with the running set of containers by starting or stopping Docker containers.
package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/kubernetes/cmd/kubelet/app"
	_ "k8s.io/kubernetes/pkg/client/metrics/prometheus" // for client metric registration
	_ "k8s.io/kubernetes/pkg/version/prometheus"        // for version metric registration

	jaegercfg "github.com/uber/jaeger-client-go/config"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	// Usage of Jaeger is configured via the usual environment variables, like
	// JAEGER_AGENT_HOST (https://github.com/jaegertracing/jaeger-client-go#environment-variables).
	// But in contrast to the default behavior, tracing with Jaeger must
	// be enabled explicitly by setting JAEGER_ENABLED to true.
	envEnabled := "JAEGER_ENABLED"
	if e := os.Getenv(envEnabled); e != "" {
		value, err := strconv.ParseBool(e)
                if err != nil {
			fmt.Fprintf(os.Stderr, "cannot parse \n", err)
			os.Exit(1)
		}
		if value {
			cfg, err := jaegercfg.FromEnv()
			if err != nil {
				// parsing errors might happen here, such as when we get a string where we expect a number
				fmt.Fprintf(os.Stderr, "Could not parse Jaeger env vars: %s", err.Error())
				os.Exit(1)
			}
			// Initialize tracer singleton.
			closer, err := cfg.InitGlobalTracer("kubelet")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Could not initialize jaeger tracer: %s", err.Error())
				os.Exit(1)
			}
			defer closer.Close()
		}
        }

	command := app.NewKubeletCommand()
	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
