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

// Package nilmap provides some generic helper functions for working with
// maps that are intended to be nil when not in use.
package nilmap

// Insert adds a new key/value pair, creating the map if it is nil.
func Insert[K comparable, V interface{}](m *map[K]V, k K, v V) {
	if *m == nil {
		*m = make(map[K]V)
	}
	(*m)[k] = v
}

// Contains checks whether a map contains a certain key.
func Contains[K comparable, V interface{}](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

// Delete removes a key amd if that makes the map empty, sets it to nil.
func Delete[K comparable, V interface{}](m *map[K]V, k K) {
	delete(*m, k)
	if len(*m) == 0 {
		*m = nil
	}
}

// Keys returns a slice with all keys of the map.
func Keys[K comparable, V interface{}](m map[K]V) []K {
	ks := make([]K, 0, len(m))
	for key := range m {
		ks = append(ks, key)
	}
	return ks
}
