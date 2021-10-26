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
	"context"
	"os"

	"github.com/go-logr/logr"

	"k8s.io/klogr/logger"
)

var (
	// fallbackLogger is intentionally not protected by a mutex. If used as intended, it
	// will get written once before being read and thus doesn't mutex locking.
	fallbackLogger       Logger = newFallbackLogger()
	fallbackLoggerWasSet        = false

	contextualLoggingEnabled = true
)

// InitLogPrefix gets insert into all log messages that were emitted through
// the initial fallback logger. Such log calls should be fixed so that they get
// triggered after logging is initialized with SetFallbackLogger.
const InitLogPrefix = "INIT-LOG-FIXME"

// SetFallbackLogger may only be called once per program run. Setting it
// multiple times would not have the intended effect because the new logger
// would not be used by code which already retrieved the fallback logger
// earlier.
//
// Therefore this also should be called before any code starts to use logging.
// Such code will get a valid logger to avoid crashes, but that logger will add
// "INIT-LOG-FIXME" as prefix to all output to indicate that SetFallbackLogger
// should have been called first.
func SetFallbackLogger(log Logger) {
	if fallbackLoggerWasSet {
		panic("the global logger was already set and cannot be changed again")
	}
	fallbackLogger = log
	fallbackLoggerWasSet = true
}

// EnableContextualLogging controls whether contextual logging is enabled.
// By default it is enabled. When disabled, FromContext avoids looking up
// the logger in the context and always returns the fallback logger.
// LoggerWithValues, LoggerWithName, and NewContext become no-ops
// and return their input logger respectively context. This may be useful
// to avoid the additional overhead for contextual logging.
//
// Like SetFallbackLogger this must be called during initialization before
// goroutines are started.
func EnableContextualLogging(enabled bool) {
	contextualLoggingEnabled = enabled
}

// FromContext retrieves a logger set by the caller or, if not set,
// falls back to the program's fallback logger.
func FromContext(ctx context.Context) Logger {
	if contextualLoggingEnabled {
		if logger, err := logr.FromContext(ctx); err == nil {
			return logger
		}
	}

	return fallbackLogger
}

// TODO can be used as a last resort by code that has no means of
// receiving a logger from its caller. FromContext or an explicit logger
// parameter should be used instead.
//
// This function may get deprecated at some point when enough code has been
// converted to accepting a logger from the caller and direct access to the
// fallback logger is not needed anymore.
func TODO() Logger {
	return fallbackLogger
}

// Background retrieves the fallback logger. It should not be called before
// that logger was initialized by the program and not by code that should
// better receive a logger via its parameters. TODO can be used as a temporary
// solution for such code.
func Background() Logger {
	return fallbackLogger
}

// Reset restores the initial state of this package. Should only be used in
// unit tests.
func Reset() {
	fallbackLogger = newFallbackLogger()
	fallbackLoggerWasSet = false
}

func newFallbackLogger() Logger {
	return logger.New(logger.Options{
		Output: os.Stderr,
	}).WithName(InitLogPrefix)
}
