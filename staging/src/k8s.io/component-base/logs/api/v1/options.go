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
	"flag"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/api/resource"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/wait"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/featuregate"
)

var (
	logrFlush func()

	// LogFlushFreqDefault is the default for the corresponding command line
	// parameter.
	LogFlushFreqDefault = 5 * time.Second
)

const (
	// LogFlushFreqFlagName is the name of the command line parameter.
	// Depending on how flags get added, it is either a stand-alone
	// value (logs.AddFlags) or part of LoggingConfiguration.
	LogFlushFreqFlagName = "log-flush-frequency"
)

// FlushLogs flushes the logger that may have been instantiated by
// LoggingConfiguration.Apply and the global klog.
func FlushLogs() {
	klog.Flush()
	if logrFlush != nil {
		logrFlush()
	}
}

// NewLoggingConfiguration returns a struct holding the default logging configuration.
func NewLoggingConfiguration() *LoggingConfiguration {
	c := LoggingConfiguration{}
	c.SetRecommendedLoggingConfiguration()
	return &c
}

// ValidateAndApply combines validation and application of the logging configuration.
// This should be invoked as early as possible because then the rest of the program
// startup (including validation of other options) will already run with the final
// logging configuration.
func (c *LoggingConfiguration) ValidateAndApply(featureGate featuregate.FeatureGate) error {
	errs := c.validate(featureGate)
	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}
	c.apply()
	return nil
}

// validate verifies if any unsupported flag is set for non-default logging
// format. It's meant to be used when LoggingConfiguration is used as a
// stand-alone struct with command line flags.
func (c *LoggingConfiguration) validate(featureGate featuregate.FeatureGate) []error {
	errs := c.ValidateAsField(featureGate, nil)
	if len(errs) != 0 {
		return errs.ToAggregate().Errors()
	}
	return nil
}

// ValidateAsField is a variant of Validate that is meant to be used when
// LoggingConfiguration is embedded inside a larger configuration struct.
func (c *LoggingConfiguration) ValidateAsField(featureGate featuregate.FeatureGate, fldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}
	if c.Format != DefaultLogFormat {
		// WordSepNormalizeFunc is just a guess. Commands should use it,
		// but we cannot know for sure.
		allFlags := unsupportedLoggingFlags(cliflag.WordSepNormalizeFunc)
		for _, f := range allFlags {
			if f.DefValue != f.Value.String() {
				errs = append(errs, field.Invalid(fldPath.Child("format"), c.Format, fmt.Sprintf("Non-default format doesn't honor flag: %s", f.Name)))
			}
		}
	}
	factory, err := logRegistry.get(c.Format)
	if err != nil {
		errs = append(errs, field.Invalid(fldPath.Child("format"), c.Format, "Unsupported log format"))
	} else if factory != nil {
		feature := factory.Feature()
		if feature != "" && !featureGate.Enabled(feature) {
			// Look up alpha/beta from actual feature. Should exist, but the API doesn't guarantee it.
			status := "experimental"
			if featureSpec, ok := featureGate.DeepCopy().GetAll()[feature]; ok {
				status = string(featureSpec.PreRelease)
			}
			errs = append(errs, field.Forbidden(fldPath.Child("format"), fmt.Sprintf("Log format %s is %s and disabled, see %s feature", c.Format, status, feature)))
		}
	}

	// The type in our struct is uint32, but klog only accepts positive int32.
	if c.Verbosity > math.MaxInt32 {
		errs = append(errs, field.Invalid(fldPath.Child("verbosity"), c.Verbosity, fmt.Sprintf("Must be <= %d", math.MaxInt32)))
	}
	vmoduleFldPath := fldPath.Child("vmodule")
	if len(c.VModule) > 0 && c.Format != "" && c.Format != "text" {
		errs = append(errs, field.Forbidden(vmoduleFldPath, "Only supported for text log format"))
	}
	for i, item := range c.VModule {
		if item.FilePattern == "" {
			errs = append(errs, field.Required(vmoduleFldPath.Index(i), "File pattern must not be empty"))
		}
		if strings.ContainsAny(item.FilePattern, "=,") {
			errs = append(errs, field.Invalid(vmoduleFldPath.Index(i), item.FilePattern, "File pattern must not contain equal sign or comma"))
		}
		if item.Verbosity > math.MaxInt32 {
			errs = append(errs, field.Invalid(vmoduleFldPath.Index(i), item.Verbosity, fmt.Sprintf("Must be <= %d", math.MaxInt32)))
		}
	}

	errs = append(errs, c.validateFormatOptions(featureGate, fldPath.Child("options"))...)
	return errs
}

func (c *LoggingConfiguration) validateFormatOptions(featureGate featuregate.FeatureGate, fldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}
	errs = append(errs, c.validateJSONOptions(featureGate, fldPath.Child("json"))...)
	return errs
}

func (c *LoggingConfiguration) validateJSONOptions(featureGate featuregate.FeatureGate, fldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}
	if !featureGate.Enabled(LoggingAlphaOptions) {
		if c.Options.JSON.SplitStream {
			errs = append(errs, field.Forbidden(fldPath.Child("splitStream"), fmt.Sprintf("Feature %s is disabled", LoggingAlphaOptions)))
		}
		if c.Options.JSON.InfoBufferSize.Value() != 0 {
			errs = append(errs, field.Forbidden(fldPath.Child("infoBufferSize"), fmt.Sprintf("Feature %s is disabled", LoggingAlphaOptions)))
		}
	}
	return errs
}

// AddFlags adds command line flags for the configuration.
func (c *LoggingConfiguration) AddFlags(fs *pflag.FlagSet) {
	// The help text is generated assuming that flags will eventually use
	// hyphens, even if currently no normalization function is set for the
	// flag set yet.
	unsupportedFlags := strings.Join(unsupportedLoggingFlagNames(cliflag.WordSepNormalizeFunc), ", ")
	formats := fmt.Sprintf(`"%s"`, strings.Join(logRegistry.list(), `", "`))
	fs.StringVar(&c.Format, "logging-format", c.Format, fmt.Sprintf("Sets the log format. Permitted formats: %s.\nNon-default formats don't honor these flags: %s.\nNon-default choices are currently alpha and subject to change without warning.", formats, unsupportedFlags))
	// No new log formats should be added after generation is of flag options
	logRegistry.freeze()

	fs.DurationVar(&c.FlushFrequency, LogFlushFreqFlagName, c.FlushFrequency, "Maximum number of seconds between log flushes")
	fs.VarP(&c.Verbosity, "v", "v", "number for the log level verbosity")
	fs.Var(&c.VModule, "vmodule", "comma-separated list of pattern=N settings for file-filtered logging (only works for text log format)")

	// JSON options. We only register them if "json" is a valid format. The
	// config file API however always has them.
	if _, err := logRegistry.get("json"); err == nil {
		fs.BoolVar(&c.Options.JSON.SplitStream, "log-json-split-stream", false, "[Alpha] In JSON format, write error messages to stderr and info messages to stdout. The default is to write a single stream to stdout.")
		fs.Var(&c.Options.JSON.InfoBufferSize, "log-json-info-buffer-size", "[Alpha] In JSON format with split output streams, the info messages can be buffered for a while to increase performance. The default value of zero bytes disables buffering. The size can be specified as number of bytes (512), multiples of 1000 (1K), multiples of 1024 (2Ki), or powers of those (3M, 4G, 5Mi, 6Gi).")
	}
}

// apply initializes logging with the selected configuration.
func (c *LoggingConfiguration) apply() {
	// if log format not exists, use nil loggr
	factory, _ := logRegistry.get(c.Format)
	if factory == nil {
		klog.ClearLogger()
	} else {
		log, flush := factory.Create(c.Options)
		klog.SetLogger(log)
		logrFlush = flush
	}
	if err := loggingFlags.Lookup("v").Value.Set(c.Verbosity.String()); err != nil {
		panic(fmt.Errorf("internal error while setting klog verbosity: %v", err))
	}
	if err := loggingFlags.Lookup("vmodule").Value.Set(c.VModule.String()); err != nil {
		panic(fmt.Errorf("internal error while setting klog vmodule: %v", err))
	}
	go wait.Forever(FlushLogs, c.FlushFrequency)
}

// SetRecommendedLoggingConfiguration sets the default logging configuration
// for fields that are unset.
//
// Consumers who embed LoggingConfiguration in their own configuration structs
// may set custom defaults and then should call this function to add the
// global defaults.
func (c *LoggingConfiguration) SetRecommendedLoggingConfiguration() {
	if c.Format == "" {
		c.Format = "text"
	}
	if c.FlushFrequency == 0 {
		c.FlushFrequency = 5 * time.Second
	}
	var empty resource.QuantityValue
	if c.Options.JSON.InfoBufferSize == empty {
		c.Options.JSON.InfoBufferSize = resource.QuantityValue{
			// This is similar, but not quite the same as a default
			// constructed instance.
			Quantity: *resource.NewQuantity(0, resource.DecimalSI),
		}
		// This sets the unexported Quantity.s which will be compared
		// by reflect.DeepEqual in some tests.
		_ = c.Options.JSON.InfoBufferSize.String()
	}
}

// loggingFlags captures the state of the logging flags, in particular their default value
// before flag parsing. It is used by unsupportedLoggingFlags.
var loggingFlags pflag.FlagSet

func init() {
	var fs flag.FlagSet
	klog.InitFlags(&fs)
	loggingFlags.AddGoFlagSet(&fs)
}

// List of logs (k8s.io/klog + k8s.io/component-base/logs) flags supported by all logging formats
var supportedLogsFlags = map[string]struct{}{
	"v": {},
	// TODO: support vmodule after 1.19 Alpha
}

// unsupportedLoggingFlags lists unsupported logging flags. The normalize
// function is optional.
func unsupportedLoggingFlags(normalizeFunc func(f *pflag.FlagSet, name string) pflag.NormalizedName) []*pflag.Flag {
	// k8s.io/component-base/logs and klog flags
	pfs := &pflag.FlagSet{}
	loggingFlags.VisitAll(func(flag *pflag.Flag) {
		if _, found := supportedLogsFlags[flag.Name]; !found {
			// Normalization changes flag.Name, so make a copy.
			clone := *flag
			pfs.AddFlag(&clone)
		}
	})

	// Apply normalization.
	pfs.SetNormalizeFunc(normalizeFunc)

	var allFlags []*pflag.Flag
	pfs.VisitAll(func(flag *pflag.Flag) {
		allFlags = append(allFlags, flag)
	})
	return allFlags
}

// unsupportedLoggingFlagNames lists unsupported logging flags by name, with
// optional normalization and sorted.
func unsupportedLoggingFlagNames(normalizeFunc func(f *pflag.FlagSet, name string) pflag.NormalizedName) []string {
	unsupportedFlags := unsupportedLoggingFlags(normalizeFunc)
	names := make([]string, 0, len(unsupportedFlags))
	for _, f := range unsupportedFlags {
		names = append(names, "--"+f.Name)
	}
	sort.Strings(names)
	return names
}
