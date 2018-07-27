/*
Copyright 2018 The Kubernetes Authors.

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

package viperconfig

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	viperFileNotFound = "Unsupported Config Type \"\""
)

// Viper is the name of a configuration file, in a format
// supported by Viper (https://github.com/spf13/viper#what-is-viper).
//
// Example: Create a file 'e2e.json' with the following:
// 	"Cadvisor":{
// 		"MaxRetries":"6"
// 	}
// Then invoke e2e with '--viper-config e2e' with 'e2e.json'
// in the current directory or '--viper-config /tmp/e2e.json'.
var viperConfig string

func init() {
	flag.StringVar(&viperConfig, "viper-config", "", "The name of the viper config i.e. 'e2e' will read values from 'e2e.json' locally.  All e2e parameters can also be configured in such a file. May contain a path and may or may not contain the file suffix.")
}

// ViperizeFlags checks whether a configuration file was specified, reads it, and updates
// the configuration variables accordingly. Must be called after framework.HandleFlags()
// and before framework.AfterReadingAllFlags().
func ViperizeFlags() {
	if viperConfig != "" {
		viper.SetConfigName(filepath.Base(viperConfig))
		viper.AddConfigPath(filepath.Dir(viperConfig))
		if err := viper.ReadInConfig(); err != nil {
			// If the user specified a file suffix, the Viper won't
			// find the file because it always appends its known set
			// of file suffices. Therefore try once more without
			// suffix.
			ext := filepath.Ext(viperConfig)
			if ext != "" && err.Error() == viperFileNotFound {
				viper.SetConfigName(filepath.Base(viperConfig[0 : len(viperConfig)-len(ext)]))
				err = viper.ReadInConfig()
			}
			if err != nil {
				// If parsing a config was requested, then it
				// must succeed. This catches syntax errors
				// and "file not found". Unfortunately error
				// messages are sometimes hard to understand,
				// so try to help the user a bit.
				switch err.Error() {
				case viperFileNotFound:
					panic(fmt.Sprintf("Viper config file %s not found or not using a supported file format", viperConfig))
				default:
					panic(fmt.Sprintf("Error parsing Viper config file %s: %s", viperConfig, err))
				}
			}
		}

		// Expose all command line flags also as Viper keys.
		pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
		pflag.Parse()
		viper.BindPFlags(pflag.CommandLine)

		// Update all flag values with values found via
		// Viper. We do this ourselves instead of calling
		// viper.Unmarshal(&TestContext) because that would
		// only update values in the main context and miss
		// values stored in the context of individual tests.
		viperUnmarshal()
	}
}

// viperUnmarshall updates all command line flags with the corresponding values found
// via Viper, regardless whether the flag value is stored in TestContext, some other
// context or a local variable.
func viperUnmarshal() {
	flag.VisitAll(func(f *flag.Flag) {
		if viper.IsSet(f.Name) {
			// In contrast to viper.Unmarshal(), values
			// that have the wrong type (for example, a
			// list instead of a plain string) will not
			// trigger an error here. This could be fixed
			// by checking the type ourselves, but
			// probably isn't worth the effort.
			//
			// "%v" correctly turns bool, int, strings into
			// the representation expected by flag, so those
			// can be used in config files. Plain strings
			// always work there, just as on the command line.
			value := viper.Get(f.Name)
			str := fmt.Sprintf("%v", value)
			if err := f.Value.Set(str); err != nil {
				panic(err)
			}
		}
	})
}
