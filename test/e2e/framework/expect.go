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

package framework

import (
	"fmt"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
)

// NewGomega is a wrapper around gomega.NewGomega which sets up Gomega so that
// the failure is recorded locally as an error. This enables the use of Gomega
// in a function which is expected to return an error:
//
//     gErr := NewGomega()
//     gErr.Expect(...).To(...)
//     if g := NewGomega(), g.Expect(...).To(...); g.Failed() {
//         return g
//     }
//
// This error can get wrapped to provide additional context for the
// failure. The test then should use ExpectNoError to turn a non-nilr
// error into a failure.
func NewGomega() GomegaInstance {
	g := gomegaInstance{}
	g.Gomega = gomega.NewGomega(g.failure)
	return g
}

type GomegaInstance interface {
	gomega.Gomega
	error
	Failed() bool
}

type gomegaInstance struct {
	gomega.Gomega
	failureMsg *string
}

var _ GomegaInstance = gomegaInstance{}

func (g *gomegaInstance) failure(msg string, callerSkip ...int) {
	g.failureMsg = &msg
}

func (g gomegaInstance) Failed() bool {
	return g.failureMsg != nil
}

func (g gomegaInstance) Is(target error) bool {
	return target == ErrFailure
}

func (g gomegaInstance) Error() string {
	if g.failureMsg == nil {
		return ""
	}
	return *g.failureMsg
}

// FailureError is an error where the error string is meant to be
// passed to ginkgo.Fail directly, i.e. adding some prefix
// like "unexpected error" is not necessary. It is also not
// necessary to dump the error struct.
type FailureError string

func (f FailureError) Error() string {
	return string(f)
}

// ErrFailure is an emtpy error that can be wrapped to
// indicate that an error is a FailureError:
//
//    return fmt.Errorf("some problem%w", ErrFailure)
//    ...
//    err := someOperation()
//    if errors.is(err, ErrFailure) {
//        ...
//    }
var ErrFailure error = FailureError("")

// ExpectEqual expects the specified two are the same, otherwise an exception raises
func ExpectEqual(actual interface{}, extra interface{}, explain ...interface{}) {
	gomega.ExpectWithOffset(1, actual).To(gomega.Equal(extra), explain...)
}

// ExpectNotEqual expects the specified two are not the same, otherwise an exception raises
func ExpectNotEqual(actual interface{}, extra interface{}, explain ...interface{}) {
	gomega.ExpectWithOffset(1, actual).NotTo(gomega.Equal(extra), explain...)
}

// ExpectError expects an error happens, otherwise an exception raises
func ExpectError(err error, explain ...interface{}) {
	gomega.ExpectWithOffset(1, err).To(gomega.HaveOccurred(), explain...)
}

// ExpectNoError checks if "err" is set, and if so, fails assertion while logging the error.
func ExpectNoError(err error, explain ...interface{}) {
	ExpectNoErrorWithOffset(1, err, explain...)
}

// ExpectNoErrorWithOffset checks if "err" is set, and if so, fails assertion while logging the error at "offset" levels above its caller
// (for example, for call chain f -> g -> ExpectNoErrorWithOffset(1, ...) error would be logged for "f").
func ExpectNoErrorWithOffset(offset int, err error, explain ...interface{}) {
	if err == nil {
		return
	}

	// Errors usually contain unexported fields. We have to use
	// a formatter here which can print those.
	prefix := ""
	if len(explain) > 0 {
		if str, ok := explain[0].(string); ok {
			prefix = fmt.Sprintf(str, explain[1:]...) + ": "
		} else {
			prefix = fmt.Sprintf("unexpected explain arguments, need format string: %v", explain)
		}
	}

	// This intentionally doesn't use gomega.Expect. Instead we take
	// full control over what information is presented where:
	// - The complete error object is logged because it may contain
	//   additional information that isn't included in its error
	//   string.
	// - It is not included in the failure message because
	//   it might make the failure message very large and/or
	//   cause error aggregation to work less well: two
	//   failures at the same code line might not be matched in
	//   https://go.k8s.io/triage because the error details are too
	//   different.
	Logf("Unexpected error: %s\n%s", prefix, format.Object(err, 1))
	Fail(prefix+err.Error(), 1+offset)
}

// ExpectConsistOf expects actual contains precisely the extra elements.  The ordering of the elements does not matter.
func ExpectConsistOf(actual interface{}, extra interface{}, explain ...interface{}) {
	gomega.ExpectWithOffset(1, actual).To(gomega.ConsistOf(extra), explain...)
}

// ExpectHaveKey expects the actual map has the key in the keyset
func ExpectHaveKey(actual interface{}, key interface{}, explain ...interface{}) {
	gomega.ExpectWithOffset(1, actual).To(gomega.HaveKey(key), explain...)
}

// ExpectEmpty expects actual is empty
func ExpectEmpty(actual interface{}, explain ...interface{}) {
	gomega.ExpectWithOffset(1, actual).To(gomega.BeEmpty(), explain...)
}
