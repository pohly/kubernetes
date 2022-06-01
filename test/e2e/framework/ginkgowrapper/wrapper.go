/*
Copyright 2017 The Kubernetes Authors.

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

// Package ginkgowrapper wraps Ginkgo Fail and Skip functions to panic
// with structured data instead of a constant string.
package ginkgowrapper

import (
	"bytes"
	"regexp"
	"runtime"
	"runtime/debug"

	"github.com/onsi/ginkgo"

	"k8s.io/kubernetes/test/e2e/framework/internal/log"
)

// FailurePanic is the value that will be panicked from Fail.
type FailurePanic struct {
	Message        string // The failure message passed to Fail
	Filename       string // The filename that is the source of the failure
	Line           int    // The line number of the filename that is the source of the failure
	FullStackTrace string // A full stack trace starting at the source of the failure
}

// String makes FailurePanic look like the old Ginkgo panic when printed.
func (FailurePanic) String() string { return ginkgo.GINKGO_PANIC }

// Fail wraps ginkgo.Fail so that it panics with more useful
// information about the failure. This function will panic with a
// FailurePanic.
func Fail(message string, callerSkip ...int) {
	skip := 1
	if len(callerSkip) > 0 {
		skip += callerSkip[0]
	}

	stack := prunedStack(skip)
	_, file, line, _ := runtime.Caller(skip)
	fp := FailurePanic{
		Message:        message,
		Filename:       file,
		Line:           line,
		FullStackTrace: stack,
	}

	log.Msg("FAIL", "%s\n\nFull Stack Trace\n%s", message, stack)

	defer func() {
		e := recover()
		if e != nil {
			panic(fp)
		}
	}()

	ginkgo.Fail(log.NowStamp()+": "+message, skip)
}

// ginkgo adds a lot of test running infrastructure to the stack, so
// we filter those out
var codeFilterRE = regexp.MustCompile(`/github.com/onsi/ginkgo/`)

// prunedStack is a wrapper around debug.Stack() that removes information
// about the current goroutine and optionally skips some of the initial stack entries.
// With skip == 0, the returned stack will start with the caller of PruneStack.
// From the remaining entries it automatically filters out useless ones like
// entries coming from Ginkgo.
//
// This is a modified copy of PruneStack in https://github.com/onsi/ginkgo/blob/f90f37d87fa6b1dd9625e2b1e83c23ffae3de228/internal/codelocation/code_location.go#L25:
// - simplified API and thus renamed (calls debug.Stack() instead of taking a parameter)
// - source code filtering updated to be specific to Kubernetes
// - optimized to use bytes and in-place slice filtering from
//   https://github.com/golang/go/wiki/SliceTricks#filter-in-place
func prunedStack(skip int) string {
	fullStackTrace := debug.Stack()
	stack := bytes.Split(fullStackTrace, []byte("\n"))
	// Ensure that the even entries are the method names and the
	// the odd entries the source code information.
	if len(stack) > 0 && bytes.HasPrefix(stack[0], []byte("goroutine ")) {
		// Ignore "goroutine 29 [running]:" line.
		stack = stack[1:]
	}
	// The "+2" is for skipping over:
	// - runtime/debug.Stack()
	// - PrunedStack()
	skip += 2
	if len(stack) > 2*skip {
		stack = stack[2*skip:]
	}
	n := 0
	for i := 0; i < len(stack)/2; i++ {
		// We filter out based on the source code file name.
		if !codeFilterRE.Match([]byte(stack[i*2+1])) {
			stack[n] = stack[i*2]
			stack[n+1] = stack[i*2+1]
			n += 2
		}
	}
	stack = stack[:n]

	return string(bytes.Join(stack, []byte("\n")))
}
