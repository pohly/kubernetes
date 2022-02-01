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
)

// LoggerWithValues returns logger.WithValues(...kv) when
// contextual logging is enabled, otherwise the logger.
func LoggerWithValues(logger Logger, kv ...interface{}) Logger {
	if contextualLoggingEnabled {
		return logger.WithValues(kv...)
	}
	return logger
}

// LoggerWithName returns logger.WithName(name) when contextual logging is
// enabled, otherwise the logger.
func LoggerWithName(logger Logger, name string) Logger {
	if contextualLoggingEnabled {
		return logger.WithName(name)
	}
	return logger
}

// NewContext returns logr.NewContext(ctx, logger) when
// contextual logging is enabled, otherwise ctx.
func NewContext(ctx context.Context, logger Logger) context.Context {
	if contextualLoggingEnabled {
		return logr.NewContext(ctx, logger)
	}
	return ctx
}
