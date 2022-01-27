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

package v1

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Supported output formats.
const (
	// DefaultLogFormat is the traditional klog output format.
	DefaultLogFormat = "text"

	// JSONLogFormat emits each log message as a JSON struct.
	JSONLogFormat = "json"
)

// The alpha or beta level of structs is the highest stability level of any field
// inside it. Feature gates will get checked during LoggingConfiguration.ValidateAndApply.

// LoggingConfiguration contains logging options.
type LoggingConfiguration struct {
	// Format Flag specifies the structure of log messages.
	// default value of format is `text`
	Format string `json:"format,omitempty"`
	// Maximum number of nanoseconds (i.e. 1s = 1000000000) between log
	// flushes. Ignored if the selected logging backend writes log
	// messages without buffering.
	//
	// Deprecated: use FlushFrequencyDuration instead
	FlushFrequency *time.Duration `json:"flushFrequency,omitempty"`
	// Maximum amount of time between log flushes. Ignored if the selected
	// logging backend writes log messages without buffering.
	FlushFrequencyDuration metav1.Duration `json:"flushFrequencyDuration"`
	// Verbosity is the threshold that determines which log messages are
	// logged. Default is zero which logs only the most important
	// messages. Higher values enable additional messages. Error messages
	// are always logged.
	Verbosity VerbosityLevel `json:"verbosity"`
	// VModule overrides the verbosity threshold for individual files.
	// Only supported for "text" log format.
	VModule VModuleConfiguration `json:"vmodule,omitempty"`
	// [Alpha] Options holds additional parameters that are specific
	// to the different logging formats. Only the options for the selected
	// format get used, but all of them get validated.
	Options FormatOptions `json:"options,omitempty"`
}

// FormatOptions contains options for the different logging formats.
type FormatOptions struct {
	// [Alpha] JSON contains options for logging format "json".
	JSON JSONOptions `json:"json,omitempty"`
}

// JSONOptions contains options for logging format "json".
type JSONOptions struct {
	// [Alpha] SplitStream redirects error messages to stderr while
	// info messages go to stdout, with buffering. The default is to write
	// both to stdout, without buffering.
	SplitStream bool `json:"splitStream,omitempty"`
	// [Alpha] InfoBufferSize sets the size of the info stream when
	// using split streams. The default is zero, which disables buffering.
	InfoBufferSize resource.QuantityValue `json:"infoBufferSize,omitempty"`
}

// VModuleConfiguration is a collection of individual file names or patterns
// and the corresponding verbosity threshold.
type VModuleConfiguration []VModuleItem

var _ pflag.Value = &VModuleConfiguration{}

// VModuleItem defines verbosity for one or more files which match a certain
// glob pattern.
type VModuleItem struct {
	// FilePattern is a base file name (i.e. minus the ".go" suffix and
	// directory) or a "glob" pattern for such a name. It must not contain
	// comma and equal signs because those are separators for the
	// corresponding klog command line argument.
	FilePattern string `json:"filePattern"`
	// Verbosity is the threshold for log messages emitted inside files
	// that match the pattern.
	Verbosity VerbosityLevel `json:"verbosity"`
}

// String returns the -vmodule parameter (comma-separated list of pattern=N).
func (vmodule *VModuleConfiguration) String() string {
	var patterns []string
	for _, item := range *vmodule {
		patterns = append(patterns, fmt.Sprintf("%s=%d", item.FilePattern, item.Verbosity))
	}
	return strings.Join(patterns, ",")
}

// Set parses the -vmodule parameter (comma-separated list of pattern=N).
func (vmodule *VModuleConfiguration) Set(value string) error {
	// This code mirrors https://github.com/kubernetes/klog/blob/9ad246211af1ed84621ee94a26fcce0038b69cd1/klog.go#L287-L313

	for _, pat := range strings.Split(value, ",") {
		if len(pat) == 0 {
			// Empty strings such as from a trailing comma can be ignored.
			continue
		}
		patLev := strings.Split(pat, "=")
		if len(patLev) != 2 || len(patLev[0]) == 0 || len(patLev[1]) == 0 {
			return fmt.Errorf("%q does not have the pattern=N format", pat)
		}
		pattern := patLev[0]
		// 31 instead of 32 to ensure that it also fits into int32.
		v, err := strconv.ParseUint(patLev[1], 10, 31)
		if err != nil {
			return fmt.Errorf("parsing verbosity in %q: %v", pat, err)
		}
		*vmodule = append(*vmodule, VModuleItem{FilePattern: pattern, Verbosity: VerbosityLevel(v)})
	}
	return nil
}

func (vmodule *VModuleConfiguration) Type() string {
	return "pattern=N,..."
}

// VerbosityLevel represents a klog or logr verbosity threshold.
type VerbosityLevel uint32

var _ pflag.Value = new(VerbosityLevel)

func (l *VerbosityLevel) String() string {
	return strconv.FormatInt(int64(*l), 10)
}

func (l *VerbosityLevel) Get() interface{} {
	return *l
}

func (l *VerbosityLevel) Set(value string) error {
	// Limited to int32 for compatibility with klog.
	v, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return err
	}
	*l = VerbosityLevel(v)
	return nil
}

func (l *VerbosityLevel) Type() string {
	return "Level"
}
