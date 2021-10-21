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

package serialize

import (
	"bytes"
	"fmt"
)

// The missing value gets printed with quotation marks.
// This would avoid that, but then not be consistent with klog anymore:
// type MissingType string
// const MissingValue MissingType = "(MISSING)"

const MissingValue = "(MISSING)"

// KVListFormat serializes all key/value pairs into the provided buffer.
// A space gets inserted before the first pair and between each pair.
func KVListFormat(b *bytes.Buffer, keysAndValues []interface{}) {
	for i := 0; i < len(keysAndValues); i += 2 {
		var v interface{}
		k := keysAndValues[i]
		if i+1 < len(keysAndValues) {
			v = keysAndValues[i+1]
		} else {
			v = MissingValue
		}
		b.WriteByte(' ')

		switch v.(type) {
		case string, error:
			b.WriteString(fmt.Sprintf("%s=%q", k, v))
		case []byte:
			b.WriteString(fmt.Sprintf("%s=%+q", k, v))
		default:
			if _, ok := v.(fmt.Stringer); ok {
				b.WriteString(fmt.Sprintf("%s=%q", k, v))
			} else {
				b.WriteString(fmt.Sprintf("%s=%+v", k, v))
			}
		}
	}
}
