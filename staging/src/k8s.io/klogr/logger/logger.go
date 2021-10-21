/*
Copyright 2019 The Kubernetes Authors.
Copyright 2020 Intel Coporation.

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

// Package testinglogger contains an implementation of the logr interface
// which is logging through a function like testing.TB.Log function.
// Therefore it can be used in standard Go tests and Gingko test suites
// to ensure that output is associated with the currently running test.
//
// Serialization of the structured log parameters is done in the same way
// as for klog.InfoS.
//
package logger

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/go-logr/logr"

	"k8s.io/klogr/internal/serialize"
)

// New constructs a new logger with the given options.
// At least an output stream must be configured.
func New(options Options) logr.Logger {
	if options.TimeNow == nil {
		options.TimeNow = time.Now
	}
	if options.Pid == 0 {
		options.Pid = os.Getpid()
	}
	return logr.New(&logSink{
		options: &options,
		prefix:  "",
		values:  nil,
	})
}

// Options configures the logger.
type Options struct {
	// Output receives the fully formatted log messages.
	Output io.Writer
	// V is the logging threshold.
	V int
	// VModule overrides the logging threshold for individual files.
	// TODO
	// Pid is the numeric process ID inserted at the start of each log message.
	Pid int
	// TimeNow can be used to override the default time.Now for testing purposes.
	TimeNow func() time.Time
}

type logSink struct {
	options   *Options
	prefix    string
	values    []interface{}
	callDepth int
}

func (l *logSink) Init(info logr.RuntimeInfo) {
	l.callDepth += info.CallDepth
}

func (l *logSink) Enabled(level int) bool {
	return level <= l.options.V
}

func (l *logSink) WithCallDepth(depth int) logr.LogSink {
	new := *l
	new.callDepth += depth
	return &new
}

// WithName returns a new logr.Logger with the specified name appended.  klogr
// uses '/' characters to separate name elements.  Callers should not pass '/'
// in the provided name string, but this library does not actually enforce that.
func (l *logSink) WithName(name string) logr.LogSink {
	new := *l
	if len(l.prefix) > 0 {
		new.prefix = l.prefix + "/"
	}
	new.prefix += name
	return &new
}

func (l *logSink) WithValues(kvList ...interface{}) logr.LogSink {
	new := *l
	new.values = make([]interface{}, 0, len(l.values)+len(kvList))
	new.values = append(new.values, l.values...)
	new.values = append(new.values, kvList...)
	return &new
}

func (l *logSink) Info(level int, msg string, kvList ...interface{}) {
	caller := l.unwindStack()
	buffer := l.formatHeader(infoLog, caller, msg)
	l.log(buffer, kvList)
}

func (l *logSink) Error(err error, msg string, kvList ...interface{}) {
	caller := l.unwindStack()
	buffer := l.formatHeader(errorLog, caller, msg)
	serialize.KVListFormat(&buffer.Buffer, []interface{}{"err", err})
	l.log(buffer, kvList)
}

func (l *logSink) log(buffer *buffer, kvList []interface{}) {
	serialize.KVListFormat(&buffer.Buffer, l.values)
	serialize.KVListFormat(&buffer.Buffer, kvList)
	if buffer.Bytes()[buffer.Len()-1] != '\n' {
		buffer.WriteByte('\n')
	}
	l.options.Output.Write(buffer.Bytes())
}

// severity identifies the sort of log
type severity int32

const (
	infoLog severity = iota
	errorLog
)

const severityChar = "IE"

// formatHeader formats a log header using the provided file name and line number.
func (l *logSink) formatHeader(s severity, caller caller, msg string) *buffer {
	now := l.options.TimeNow()
	if caller.line < 0 {
		caller.line = 0 // not a real line number, but acceptable to someDigits
	}
	buf := getBuffer()

	// Avoid Fprintf, for speed. The format is so simple that we can do it quickly by hand.
	// It's worth about 3X. Fprintf is hard.
	_, month, day := now.Date()
	hour, minute, second := now.Clock()
	// Lmmdd hh:mm:ss.uuuuuu threadid file:line]
	buf.tmp[0] = severityChar[s]
	buf.twoDigits(1, int(month))
	buf.twoDigits(3, day)
	buf.tmp[5] = ' '
	buf.twoDigits(6, hour)
	buf.tmp[8] = ':'
	buf.twoDigits(9, minute)
	buf.tmp[11] = ':'
	buf.twoDigits(12, second)
	buf.tmp[14] = '.'
	buf.nDigits(6, 15, now.Nanosecond()/1000, '0')
	buf.tmp[21] = ' '
	// TODO: should be TID or better, goroutine ID. But retrieving that is
	// hard: https://blog.sgmansfield.com/2015/12/goroutine-ids/
	buf.nDigits(7, 22, l.options.Pid, ' ')
	buf.tmp[29] = ' '
	buf.Write(buf.tmp[:30])
	buf.WriteString(caller.file)
	buf.tmp[0] = ':'
	n := buf.someDigits(1, caller.line)
	buf.tmp[n+1] = ']'
	buf.Write(buf.tmp[:n+2])

	if l.prefix != "" {
		msg = l.prefix + ": " + msg
	}

	if msg != "" {
		buf.WriteByte(' ')
		buf.WriteString(strconv.Quote(msg))
	}

	return buf
}

// caller represents the original call site for a log line, after considering
// logr.Logger.WithCallDepth.
type caller struct {
	// file is the basename of the file for this call site.
	file string
	// line is the line number in the file for this call site.
	line int
}

func (l *logSink) unwindStack() caller {
	// +1 for this frame, +1 for Info/Error/Enabled.
	_, file, line, ok := runtime.Caller(l.callDepth + 2)
	if !ok {
		return caller{"<unknown>", 0}
	}
	return caller{filepath.Base(file), line}
}

var _ logr.LogSink = &logSink{}
var _ logr.CallDepthLogSink = &logSink{}
