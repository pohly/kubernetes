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

	"github.com/go-logr/logr"
	"k8s.io/klogr/proxy"
)

// defLogger is intentionally not protected by a mutex. If used as intended, it
// will get written once before being read and thus doesn't mutex locking.
var defLogger Logger = newDefaultLogger()

var defLoggerWasSet = false

// SetDefaultLogger may only be called once per program run. Setting it
// multiple times would not have the intended effect because the new logger
// would not be used by code which already retrieved the default earlier.
//
// Therefore this also should be called before any code starts to use logging.
// Such code will get a valid logger to avoid crashes, but that logger will add
// "INIT-LOG-FIXME" as prefix to all output to indicate that SetDefaultLogger
// should have been called first.
func SetDefaultLogger(log Logger) {
	if defLoggerWasSet {
		panic("the global default logger was already set and cannot be changed again")
	}
	defLogger = log
	defLoggerWasSet = true
}

// FromContext retrieves a logger set by the caller or, if not set,
// falls back to the program's default logger.
func FromContext(ctx context.Context) Logger {
	if logger, err := logr.FromContext(ctx); err == nil {
		return logger
	}

	return defLogger
}

// DefaultLogger can be used as a last resort by code that has no means of
// receiving a logger from its caller. FromContext or an explicit logger
// parameter should be used instead.
//
// DefaultLogger and therefore FromContext should not be called before the
// program had a chance to initialize logging because the builtin default is
// not using the intended final logging configuration.
//
// This function may get deprecated at some point when enough code has been
// converted to accepting a logger from the caller and direct access to the
// default logger is not needed anymore.
func DefaultLogger() Logger {
	return defLogger
}

// InitLogPrefix gets insert into all log messages that were emitted through
// the initial default logger. Such log calls should be fixed so that they get
// triggered after logging is initialized with SetDefaultLogger.
const InitLogPrefix = "INIT-LOG-FIXME"

func newDefaultLogger() Logger {
	return proxy.New().WithName(InitLogPrefix)
}
