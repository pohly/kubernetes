/*
Copyright 2019 The Kubernetes Authors.

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

package framework

import (
	"fmt"

	// TODO: Remove the following imports (ref: https://github.com/kubernetes/kubernetes/issues/81245)
	e2eginkgowrapper "k8s.io/kubernetes/test/e2e/framework/ginkgowrapper"
	"k8s.io/kubernetes/test/e2e/framework/internal/log"
)

// Logf logs the info.
func Logf(format string, args ...interface{}) {
	log.Msg("INFO", format, args...)
}

// Failf logs the fail info, including a stack trace that starts with the direct caller.
// Code that logs a fixed string should use Fail instead. To skip additional one additional
// stack frame in a helper function, use Fail(fmt.Sprintf(...), 1) or Fail("some string", 1).
func Failf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	skip := 1
	e2eginkgowrapper.Fail(msg, skip)
	panic("unreachable")
}

// Fail is a replacement for ginkgo.Fail which logs the problem as it occurs
// together with a stack trace and then calls ginkgowrapper.Fail.
func Fail(msg string, callerSkip ...int) {
	skip := 1
	if len(callerSkip) > 0 {
		skip += callerSkip[0]
	}
	e2eginkgowrapper.Fail(msg, skip)
}
