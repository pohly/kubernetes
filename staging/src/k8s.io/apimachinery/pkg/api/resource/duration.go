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

package resource

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/spf13/pflag"
)

// Duration is a wrapper around time.Duration.
// It provides convenient marshaling/unmarshaling in JSON and YAML
// and implements the pflag.Value interface.
//
// The serialization format is the same as for time.ParseDurations:
//
// A duration string is a possibly signed sequence of decimal numbers, each
// with optional fraction and a unit suffix, such as "300ms", "-1.5h" or
// "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
//
// In addition, float values in JSON or YAML are also accepted and treated as
// number of seconds. This makes Duration a suitable replacement for fields
// that previously accepted seconds as an integer.
//
// +protobuf=true
// +protobuf.options.marshal=false
// +protobuf.options.(gogoproto.goproto_stringer)=false
// +k8s:openapi-gen=true
type Duration struct {
	time.Duration `json:"duration"`
}

func (d Duration) Type() string {
	return "duration"
}

func (d *Duration) Set(s string) error {
	duration, err := time.ParseDuration(s)
	if err != nil {
		// Already contains the invalid value, so no need to wrap the
		// error here.
		return err
	}
	d.Duration = duration
	return nil
}

var _ flag.Value = &Duration{}
var _ pflag.Value = &Duration{}

// MarshalJSON always represents the duration as a string
// with suffix.
func (d *Duration) MarshalJSON() ([]byte, error) {
	s := d.String()
	out := make([]byte, len(s)+2)
	out[0], out[len(out)-1] = '"', '"'
	copy(out[1:], s)
	return out, nil
}

// UnmarshalJSON accepts strings and parses those with
// time.ParseDuration or a float and treats that value
// as seconds.
func (d *Duration) UnmarshalJSON(value []byte) error {
	l := len(value)

	if l == 4 && bytes.Equal(value, []byte("null")) {
		d.Duration = 0
		return nil
	}

	if l >= 2 && value[0] == '"' && value[l-1] == '"' {
		return d.Set(string(value[1 : l-1]))
	}

	seconds, err := strconv.ParseFloat(string(value), 64)
	if err != nil {
		return err
	}
	nanoseconds := math.Round(seconds * 1e9)
	if math.IsInf(nanoseconds, 0) || math.IsNaN(nanoseconds) {
		return fmt.Errorf("need a number, got %f", nanoseconds)
	}
	if nanoseconds > math.MaxInt64 ||
		nanoseconds < math.MinInt64 {
		return fmt.Errorf("%s out of valid range for int64 nanoseconds", string(value))
	}
	d.Duration = time.Duration(nanoseconds)
	return nil
}

var _ json.Marshaler = &Duration{}
