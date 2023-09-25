/*
Copyright 2023 The Kubernetes Authors.

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

package builtincontroller

import (
	"log/slog"
	"sort"

	"github.com/go-logr/logr"
)

// Log wraps a slice of controllers so that if the returned value gets logged
// in structured logging, it will be logged as set of controller names.
func Log[T NamedController](controllers []T) interface{} {
	return namedControllers[T]{
		controllers: controllers,
	}
}

var _ logr.Marshaler = namedControllers[NamedController]{}
var _ slog.LogValuer = namedControllers[NamedController]{}

type namedControllers[T NamedController] struct {
	controllers []T
}

func (n namedControllers[T]) MarshalLog() interface{} {
	return n.listNames()
}

func (n namedControllers[T]) LogValue() slog.Value {
	return slog.AnyValue(n.listNames())
}

func (n namedControllers[T]) listNames() []string {
	names := make([]string, 0, len(n.controllers))
	for _, controller := range n.controllers {
		names = append(names, controller.ControllerName())
	}
	sort.Strings(names)
	return names
}
