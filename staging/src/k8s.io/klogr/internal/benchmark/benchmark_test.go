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

package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// BenchmarkRecursion measures the overhead of adding calling a function
// recursively with just the depth parameter.
func BenchmarkRecursion(b *testing.B) {
	for depth := 10; depth <= 100000; depth *= 10 {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				recurse(depth)
			}
		})
	}
}

//go:noinline
func recurse(depth int) {
	if depth == 0 {
		logr.Discard().Info("hello world")
		return
	}
	recurse(depth - 1)
}

// BenchmarkRecursionWithLogger measures the overhead of adding a logr.Logger
// parameter.
func BenchmarkRecursionWithLogger(b *testing.B) {
	logger := logr.Discard()

	for depth := 10; depth <= 100000; depth *= 10 {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				recurseWithLogger(logger, depth)
			}
		})
	}
}

//go:noinline
func recurseWithLogger(logger logr.Logger, depth int) {
	if depth == 0 {
		logger.Info("hello world")
		return
	}
	recurseWithLogger(logger, depth-1)
}

// BenchmarkRecursionWithContext measures the overhead of adding a context
// parameter.
func BenchmarkRecursionWithContext(b *testing.B) {
	logger := logr.Discard()
	ctx := logr.NewContext(context.Background(), logger)

	for depth := 10; depth <= 100000; depth *= 10 {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				recurseWithContext(ctx, depth)
			}
		})
	}
}

//go:noinline
func recurseWithContext(ctx context.Context, depth int) {
	if depth == 0 {
		logger := logr.FromContextOrDiscard(ctx)
		logger.Info("hello world")
		return
	}
	recurseWithContext(ctx, depth-1)
}

// BenchmarkNestedContext benchmarks how quickly a function
// can be called that creates a new context at each call.
func BenchmarkNestedContext(b *testing.B) {
	ctx := context.Background()

	for depth := 10; depth <= 1000; depth *= 10 {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nestedContext(ctx, depth)
			}
		})
	}
}

//go:noinline
func nestedContext(ctx context.Context, depth int) {
	if depth == 0 {
		logr.Discard().Info("hello world")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	nestedContext(ctx, depth-1)
}

// BenchmarkNestedContext benchmarks how quickly a function
// can be called that creates a new context at each call and
// retrieves a logger at the end.
func BenchmarkNestedContextWithLogger(b *testing.B) {
	logger := logr.Discard()
	ctx := logr.NewContext(context.Background(), logger)

	for depth := 10; depth <= 1000; depth *= 10 {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nestedContextWithLogger(ctx, depth)
			}
		})
	}
}

//go:noinline
func nestedContextWithLogger(ctx context.Context, depth int) {
	if depth == 0 {
		logger := logr.FromContextOrDiscard(ctx)
		logger.Info("hello world")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	nestedContextWithLogger(ctx, depth-1)
}

var logger logr.Logger

// BenchmarkFromContextWithTimeouts benchmarks how quickly a value can be
// looked up in a context of varying nesting, with timeouts added at each
// level.
func BenchmarkFromContextWithTimeouts(b *testing.B) {
	for depth := 1; depth <= 10000; depth *= 10 {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			ctx := context.Background()
			for i := 1; i < depth; i++ {
				ctx2, cancel := context.WithTimeout(ctx, time.Hour)
				defer cancel()
				ctx = ctx2
			}
			for i := 0; i < b.N; i++ {
				logger = logr.FromContextOrDiscard(ctx)
			}
		})
	}
}

// BenchmarkFromContextWithValues benchmarks how quickly a value can be looked
// up in a context of varying nesting, with a new value added at each level.
func BenchmarkFromContextWithValues(b *testing.B) {
	for depth := 1; depth <= 10000; depth *= 10 {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			ctx := context.Background()
			for i := 1; i < depth; i++ {
				ctx = context.WithValue(ctx, 1, 2)
			}
			for i := 0; i < b.N; i++ {
				logger = logr.FromContextOrDiscard(ctx)
			}
		})
	}
}
