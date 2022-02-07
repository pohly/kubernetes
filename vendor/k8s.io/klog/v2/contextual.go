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

package klog

import (
	"context"

	"github.com/go-logr/logr"
)

// This file provides the implementation of
// https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/1602-structured-logging
//
// SetLogger and ClearLogger were originally added to klog.go and got moved
// here. Contextual logging adds a way to retrieve a Logger for direct logging
// without the logging calls in klog.go.
//
// The global variables are expected to be modified only during sequential
// parts of a program (init, serial tests) and therefore are not protected by
// mutex locking.

var (
	// contextualLoggingEnabled controls whether contextual logging is
	// active. Disabling it may have some small performance benefit.
	contextualLoggingEnabled = true

	// globalLogger is the global Logger chosen by users of klog, nil if
	// none is available.
	globalLogger *Logger

	// klogLogger is used as fallback for logging through the normal klog code
	// when no Logger is set.
	klogLogger logr.Logger = logr.New(&klogger{})
)

// SetLogger sets a Logger implementation that will be used in two different
// ways:
// - as backing implementation of the traditional klog log calls
// - for direct log calls afer retrieving it with FromContext, TODO, or
//   Background
//
// If set, all log lines will be suppressed from the regular Output, and
// redirected to the logr implementation.
// Use as:
//   ...
//   klog.SetLogger(zapr.NewLogger(zapLog))
//
// To remove a backing logr implemention, use ClearLogger. Setting an
// empty logger with SetLogger(logr.Logger{}) does not work.
//
// Modifying the logger is not thread-safe and should be done while no other
// goroutines invoke log calls, usually during program initialization.
func SetLogger(logger logr.Logger) {
	globalLogger = &logger
}

// ClearLogger removes a backing Logger implementation if one was set earlier
// with SetLogger.
//
// Modifying the logger is not thread-safe and should be done while no other
// goroutines invoke log calls, usually during program initialization.
func ClearLogger() {
	globalLogger = nil
}

// EnableContextualLogging controls whether contextual logging is enabled.
// By default it is enabled. When disabled, FromContext avoids looking up
// the logger in the context and always returns the global logger.
// LoggerWithValues, LoggerWithName, and NewContext become no-ops
// and return their input logger respectively context. This may be useful
// to avoid the additional overhead for contextual logging.
//
// This must be called during initialization before goroutines are started.
//
// Experimental
//
// Notice: This function is EXPERIMENTAL and may be changed or removed in a
// later release.
func EnableContextualLogging(enabled bool) {
	contextualLoggingEnabled = enabled
}

// FromContext retrieves a logger set by the caller or, if not set,
// falls back to the program's global logger (a Logger instance or klog
// itself).
//
// Experimental
//
// Notice: This function is EXPERIMENTAL and may be changed or removed in a
// later release.
func FromContext(ctx context.Context) Logger {
	if contextualLoggingEnabled {
		if logger, err := logr.FromContext(ctx); err == nil {
			return logger
		}
	}

	return Background()
}

// TODO can be used as a last resort by code that has no means of
// receiving a logger from its caller. FromContext or an explicit logger
// parameter should be used instead.
//
// Experimental
//
// Notice: This function is EXPERIMENTAL and may be changed or removed in a
// later release.
func TODO() Logger {
	return Background()
}

// Background retrieves the fallback logger. It should not be called before
// that logger was initialized by the program and not by code that should
// better receive a logger via its parameters. TODO can be used as a temporary
// solution for such code.
//
// Experimental
//
// Notice: This function is EXPERIMENTAL and may be changed or removed in a
// later release.
func Background() Logger {
	logging.mu.Lock()
	defer logging.mu.Unlock()
	if globalLogger != nil {
		return *globalLogger
	}

	return klogLogger
}

// LoggerWithValues returns logger.WithValues(...kv) when
// contextual logging is enabled, otherwise the logger.
//
// Experimental
//
// Notice: This function is EXPERIMENTAL and may be changed or removed in a
// later release.
func LoggerWithValues(logger Logger, kv ...interface{}) Logger {
	if contextualLoggingEnabled {
		return logger.WithValues(kv...)
	}
	return logger
}

// LoggerWithName returns logger.WithName(name) when contextual logging is
// enabled, otherwise the logger.
//
// Experimental
//
// Notice: This function is EXPERIMENTAL and may be changed or removed in a
// later release.
func LoggerWithName(logger Logger, name string) Logger {
	if contextualLoggingEnabled {
		return logger.WithName(name)
	}
	return logger
}

// NewContext returns logr.NewContext(ctx, logger) when
// contextual logging is enabled, otherwise ctx.
//
// Experimental
//
// Notice: This function is EXPERIMENTAL and may be changed or removed in a
// later release.
func NewContext(ctx context.Context, logger Logger) context.Context {
	if contextualLoggingEnabled {
		return logr.NewContext(ctx, logger)
	}
	return ctx
}
