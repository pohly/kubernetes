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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDuration(t *testing.T) {
	testcases := map[string]struct {
		s                  string
		setError           string
		unmarshalJSONError string
		duration           Duration
		str                string
	}{
		"seconds": {
			s:        `"123s"`,
			duration: Duration{Duration: 123 * time.Second},
			str:      `2m3s`,
		},
		"minutes": {
			s:        `"10m"`,
			duration: Duration{Duration: 10 * time.Minute},
			str:      `10m0s`,
		},
		"fraction": {
			s:        `"1.5m"`,
			duration: Duration{Duration: 90 * time.Second},
			str:      `1m30s`,
		},
		"invalid": {
			s:                  `"xyz"`,
			setError:           `time: invalid duration "xyz"`,
			unmarshalJSONError: `time: invalid duration "xyz"`,
		},
		"int": {
			s:        `123`,
			setError: `time: missing unit in duration "123"`,
			duration: Duration{Duration: 123 * time.Second},
			str:      `2m3s`,
		},
		"float": {
			s:        `1.5`,
			setError: `time: missing unit in duration "1.5"`,
			duration: Duration{Duration: 1500 * time.Millisecond},
			str:      `1.5s`,
		},
		"NaN": {
			s:                  `NaN`,
			setError:           `time: invalid duration "NaN"`,
			unmarshalJSONError: `need a number, got NaN`,
		},
		"Inf": {
			s:                  `Inf`,
			setError:           `time: invalid duration "Inf"`,
			unmarshalJSONError: `need a number, got +Inf`,
		},
		"-Inf": {
			s:                  `-Inf`,
			setError:           `time: invalid duration "-Inf"`,
			unmarshalJSONError: `need a number, got -Inf`,
		},
		"null": {
			s:        `null`,
			setError: `time: invalid duration "null"`,
		},
		"overflow": {
			s:                  `1e100`,
			setError:           `time: unknown unit "e" in duration "1e100"`,
			unmarshalJSONError: `1e100 out of valid range for int64 nanoseconds`,
		},
		"underflow": {
			s:                  `-1e100`,
			setError:           `time: unknown unit "e" in duration "-1e100"`,
			unmarshalJSONError: `-1e100 out of valid range for int64 nanoseconds`,
		},
		"no-string": {
			s:                  `"1s`,
			setError:           `time: invalid duration "\"1s"`,
			unmarshalJSONError: `strconv.ParseFloat: parsing "\"1s": invalid syntax`,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Run("Set", func(t *testing.T) {
				var d Duration
				s := tc.s
				l := len(s)
				if len(s) >= 2 && s[0] == '"' && s[l-1] == '"' {
					s = s[1 : l-1]
				}
				err := d.Set(s)
				errStr := ""
				if err != nil {
					errStr = err.Error()
				}
				if assert.Equal(t, tc.setError, errStr) && errStr == "" {
					assert.Equal(t, tc.duration, d)
				}
			})

			t.Run("String", func(t *testing.T) {
				s := tc.duration.String()
				expect := tc.str
				if tc.duration.Duration == 0 {
					expect = "0s"
				}
				assert.Equal(t, expect, s)
			})

			t.Run("UnmarshalJSON", func(t *testing.T) {
				var d Duration
				err := d.UnmarshalJSON([]byte(tc.s))
				errStr := ""
				if err != nil {
					errStr = err.Error()
				}
				if assert.Equal(t, tc.unmarshalJSONError, errStr) && errStr == "" {
					assert.Equal(t, tc.duration, d)
				}
			})

			t.Run("MarshalJSON", func(t *testing.T) {
				buffer, err := tc.duration.MarshalJSON()
				if assert.NoError(t, err) {
					expect := tc.str
					if tc.duration.Duration == 0 {
						expect = "0s"
					}
					assert.Equal(t, `"`+expect+`"`, string(buffer))
				}
			})
		})
	}
}
