/*
Copyright 2022 The Kubernetes Authors.

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

package cel

import (
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
)

func newStringIteratorForMap[T any](m map[string]T) traits.Iterator {
	strings := make([]types.String, 0, len(m))
	for key := range m {
		strings = append(strings, types.String(key))
	}
	return &stringIterator{
		Int:     types.Int(-1),
		strings: strings,
	}
}

// stringIterator implements [traits.Iterator] for a list of strings.
type stringIterator struct {
	types.Int
	strings []types.String
}

var _ traits.Iterator = &stringIterator{}

func (i *stringIterator) HasNext() ref.Val {
	return types.Bool(i.Int+1 < types.Int(len(i.strings)))
}

func (i *stringIterator) Next() ref.Val {
	i.Int++
	return i.strings[i.Int]
}
