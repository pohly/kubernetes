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

package proxy

import (
	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
)

// New returns a logr.Logger writes log messages via klog's structured logging
// functions.
func New() logr.Logger {
	l := klogger{}
	return logr.New(&l)
}

type klogger struct {
	level     int
	callDepth int
	prefix    string
	values    []interface{}
}

func (l *klogger) Init(info logr.RuntimeInfo) {
	l.callDepth += info.CallDepth
}

func (l klogger) Enabled(level int) bool {
	return klog.V(klog.Level(level)).Enabled()
}

func (l klogger) Info(level int, msg string, kvList ...interface{}) {
	if l.prefix != "" {
		msg = l.prefix + ": " + msg
	}
	// We know that the log message is enabled (gets checked by
	// logr.Logger) and klog doesn't log the verbosity, so we can emit as a
	// normal Info message.
	klog.InfoSDepth(l.callDepth+1, msg, l.mergeKVs(kvList)...)
}

func (l klogger) Error(err error, msg string, kvList ...interface{}) {
	if l.prefix != "" {
		msg = l.prefix + ": " + msg
	}
	klog.ErrorSDepth(l.callDepth+1, err, msg, l.mergeKVs(kvList)...)
}

func (l klogger) mergeKVs(kvList []interface{}) []interface{} {
	if len(l.values) == 0 {
		return kvList
	}

	// To filter out key/value pairs in l.values that get superseded by
	// key/value pairs in kvList, first collect all keys in the later.
	haveKey := map[interface{}]bool{}
	for i := 0; i < len(kvList); i += 2 {
		haveKey[kvList[i]] = true
	}

	// We know the maximum length. Might be shorter after filtering out
	// duplicates.
	kv := make([]interface{}, 0, len(l.values)+len(kvList))

	// Copy those values that are not superseeded.
	for i := 0; i < len(l.values); i += 2 {
		key := l.values[i]
		if !haveKey[key] {
			kv = append(kv, key, l.values[i+1])
		}
	}

	// Duplicates in the parameters are not filtered out. That would cause
	// additional runtime overhead and can be avoided by code review and/or
	// static code analysis.
	kv = append(kv, kvList...)

	return kv
}

// WithName returns a new logr.Logger with the specified name appended.  klogr
// uses '/' characters to separate name elements.  Callers should not pass '/'
// in the provided name string, but this library does not actually enforce that.
func (l klogger) WithName(name string) logr.LogSink {
	if len(l.prefix) > 0 {
		l.prefix = l.prefix + "/"
	}
	l.prefix += name
	return &l
}

func (l klogger) WithValues(kvList ...interface{}) logr.LogSink {
	l.values = l.mergeKVs(kvList)
	if len(l.values)%2 != 0 {
		l.values = append(l.values, "(MISSING)")
	}
	return &l
}

func (l klogger) WithCallDepth(depth int) logr.LogSink {
	l.callDepth += depth
	return &l
}

var _ logr.LogSink = &klogger{}
var _ logr.CallDepthLogSink = &klogger{}
