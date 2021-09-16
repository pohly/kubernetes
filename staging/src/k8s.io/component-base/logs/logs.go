/*
Copyright 2014 The Kubernetes Authors.

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

package logs

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

const logFlushFreqFlagName = "log-flush-frequency"

var (
	logFlushFreqVar *pflag.Flag

	// logFlushPeriod is the amount of time between log flushes. It might
	// change over time while log flushing has already started, for example
	// because flag parsing is done (again) later. Therefore access to it
	// has to be protected with a mutex.
	logFlushPeriod = &guardedDuration{
		duration: 5 * time.Second,
	}
)

type guardedDuration struct {
	mutex    sync.Mutex
	duration time.Duration
}

func (g *guardedDuration) String() string {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.duration.String()
}

func (g *guardedDuration) Set(value string) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		// No need to wrap, the flag package will add additional information.
		return nil
	}
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.duration = duration
	return nil
}

func (g *guardedDuration) Get() time.Duration {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.duration
}

func (g *guardedDuration) Type() string {
	return "duration"
}

var _ pflag.Value = &guardedDuration{}

func init() {
	klog.InitFlags(flag.CommandLine)
	pflag.Var(logFlushPeriod, logFlushFreqFlagName, "Maximum number of seconds between log flushes")

	// Look up the flag now for AddFlags, in case that a command replaces the global
	// pflag.CommandLine.
	logFlushFreqVar = pflag.Lookup(logFlushFreqFlagName)

}

// AddFlags registers this package's flags on arbitrary FlagSets, such that they point to the
// same value as the global flags.
func AddFlags(fs *pflag.FlagSet) {
	fs.AddFlag(logFlushFreqVar)
}

// KlogWriter serves as a bridge between the standard log package and the glog package.
type KlogWriter struct{}

// Write implements the io.Writer interface.
func (writer KlogWriter) Write(data []byte) (n int, err error) {
	klog.InfoDepth(1, string(data))
	return len(data), nil
}

// InitLogs initializes logs the way we want for kubernetes.
func InitLogs() {
	log.SetOutput(KlogWriter{})
	log.SetFlags(0)
	go flushForever()
}

// flushForever runs in a goroutine and calls FlushLogs with delays between
// flushes that are determined by logFlushPeriod. That value may change at any
// time. This makes it possible to call InitLogs before parsing flags.
func flushForever() {
	lastFlush := time.Now()
	for {
		// Sleep till it is time to flush again.
		flushNext := lastFlush.Add(logFlushPeriod.Get())
		now := time.Now()
		time.Sleep(flushNext.Sub(now))
		FlushLogs()
		lastFlush = now
	}
}

// FlushLogs flushes logs immediately.
func FlushLogs() {
	klog.Flush()
}

// NewLogger creates a new log.Logger which sends logs to klog.Info.
func NewLogger(prefix string) *log.Logger {
	return log.New(KlogWriter{}, prefix, 0)
}

// GlogSetter is a setter to set glog level.
func GlogSetter(val string) (string, error) {
	var level klog.Level
	if err := level.Set(val); err != nil {
		return "", fmt.Errorf("failed set klog.logging.verbosity %s: %v", val, err)
	}
	return fmt.Sprintf("successfully set klog.logging.verbosity to %s", val), nil
}
