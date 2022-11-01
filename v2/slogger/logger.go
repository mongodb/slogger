// Copyright 2013 - 2016 MongoDB, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package slogger

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

var loggerConfigLock sync.RWMutex

type Log struct {
	Prefix     string
	Level      Level
	ErrorCode  ErrorCode
	Filename   string
	FuncName   string
	Line       int
	Timestamp  time.Time
	MessageFmt string
	Args       []interface{}
	Context    *Context
}

func SimpleLog(prefix string, level Level, errorCode ErrorCode, callerSkip int, messageFmt string, args ...interface{}) *Log {
	return SimpleLogStrippingDirs(prefix, level, errorCode, callerSkip, -1, messageFmt, args...)
}

func SimpleLogStrippingDirs(prefix string, level Level, errorCode ErrorCode, callerSkip int, numDirsToKeep int, messageFmt string, args ...interface{}) *Log {
	pc, file, line, ok := runtime.Caller(callerSkip)
	funcName := ""

	if ok {
		funcName = baseFuncNameForPC(pc)
	} else {
		file = "UNKNOWN_FILE"
		line = -1
	}

	if numDirsToKeep >= 0 {
		file = stripDirectories(file, numDirsToKeep)
	}

	return &Log{
		Prefix:     prefix,
		Level:      level,
		ErrorCode:  errorCode,
		Filename:   file,
		FuncName:   funcName,
		Line:       line,
		Timestamp:  time.Now(),
		MessageFmt: messageFmt,
		Args:       args,
	}
}

// MaxLogSize values below this threshold are effectively ignored
const MinimumMaxLogSizeThreshold = 100

var maxLogSize = -1 // -1 means no truncation

func getMaxLogSize() int {
	loggerConfigLock.RLock()
	defer loggerConfigLock.RUnlock()
	return maxLogSize
}

func SetMaxLogSize(size int) {
	loggerConfigLock.Lock()
	defer loggerConfigLock.Unlock()
	maxLogSize = size
}
func getSizeInKb(n int) string {
	return fmt.Sprintf("%.1f", float64(n)/1024)
}

func getTruncatedMessage(old string) string {
	new := old
	lineLen := len(old)
	mls := getMaxLogSize()
	if mls > MinimumMaxLogSizeThreshold && lineLen > mls+4 {
		new = fmt.Sprintf("%s...%s (warning: log line attempted (%vk) over max size (%vk), printing beginning and end)", old[0:mls], old[lineLen+3-MinimumMaxLogSizeThreshold:], getSizeInKb(lineLen), getSizeInKb(mls))
	}
	return new
}

func (self *Log) Message() string {
	return getTruncatedMessage(fmt.Sprintf(self.MessageFmt, self.Args...))
}

type Logger struct {
	Prefix       string
	Appenders    []Appender
	StripDirs    int
	TurboFilters []TurboFilter
}

// Log a message and a level to a logger instance. This returns a
// pointer to a Log and a slice of errors that were gathered from every
// Appender (nil errors included), or nil and an empty error slice if
// any turbo filter condition was not satisfied causing an early exit.
func (self *Logger) Logf(level Level, messageFmt string, args ...interface{}) (*Log, []error) {
	return self.logf(level, NoErrorCode, messageFmt, nil, args...)
}

func (self *Logger) LogfWithContext(level Level, messageFmt string, context *Context, args ...interface{}) (*Log, []error) {
	return self.logf(level, NoErrorCode, messageFmt, context, args...)
}

func (self *Logger) LogfWithErrorCodeAndContext(level Level, errorCode ErrorCode, messageFmt string, context *Context, args ...interface{}) (*Log, []error) {
	return self.logf(level, errorCode, messageFmt, context, args...)
}

// Log and return a formatted error string.
// Example:
//
// if whatIsExpected != whatIsReturned {
//     return slogger.Errorf(slogger.WARN, "Unexpected return value. Expected: %v Received: %v",
//         whatIsExpected, whatIsReturned)
// }5
//

type ErrorWithCode struct {
	ErrCode ErrorCode
	Err     error
}

func (e ErrorWithCode) Error() string {
	return e.Err.Error()
}

func (e ErrorWithCode) Unwrap() error {
	return e.Err
}

func (self *Logger) Errorf(level Level, messageFmt string, args ...interface{}) error {
	return self.ErrorfWithContext(level, messageFmt, nil, args...)
}

func (self *Logger) ErrorfWithContext(level Level, messageFmt string, context *Context, args ...interface{}) error {
	return self.ErrorfWithErrorCodeAndContext(level, NoErrorCode, messageFmt, context, args...)
}

func (self *Logger) ErrorfWithErrorCodeAndContext(level Level, errorCode ErrorCode, messageFmt string, context *Context, args ...interface{}) error {
	log, _ := self.logf(level, errorCode, messageFmt, context, args...)
	return ErrorWithCode{errorCode, errors.New(log.Message())}
}

func (self *Logger) Flush() (errors []error) {
	for _, appender := range self.Appenders {
		if err := appender.Flush(); err != nil {
			errors = append(errors, err)
		}
	}
	return
}

// Stackf is designed to work in tandem with `NewStackError`. This
// function is similar to `Logf`, but takes a `stackErr`
// parameter. `stackErr` is expected to be of type StackError, but does
// not have to be.
func (self *Logger) Stackf(level Level, stackErr error, messageFmt string, args ...interface{}) (*Log, []error) {
	return self.StackfWithContext(level, stackErr, messageFmt, nil, args...)
}

func (self *Logger) StackfWithContext(level Level, stackErr error, messageFmt string, context *Context, args ...interface{}) (*Log, []error) {
	return self.StackfWithErrorCodeAndContext(level, NoErrorCode, stackErr, messageFmt, context, args...)
}

func (self *Logger) StackfWithErrorCodeAndContext(level Level, errorCode ErrorCode, stackErr error, messageFmt string, context *Context, args ...interface{}) (*Log, []error) {
	messageFmt = fmt.Sprintf("%v\n%v", messageFmt, stackErr.Error())
	return self.logf(level, errorCode, messageFmt, context, args...)
}

var ignoredFileNames = []string{"logger.go"}

// Add a file to the list of file names that slogger will skip when it identifies the source
// of a message.  This is useful if you have a logging library built on top of slogger.
// If you IgnoreThisFilenameToo(...) on the files of that library, logging messages
// will be marked as coming from your code that calls your library, rather than from your library.
func IgnoreThisFilenameToo(fn string) {
	loggerConfigLock.Lock()
	defer loggerConfigLock.Unlock()
	ignoredFileNames = append(ignoredFileNames, fn)
}

func getIgnoredFileNames() []string {
	loggerConfigLock.RLock()
	defer loggerConfigLock.RUnlock()
	return ignoredFileNames
}

func baseFuncNameForPC(pc uintptr) string {
	fullFuncName := runtime.FuncForPC(pc).Name()

	// strip github.com/mongodb/slogger/v2slogger.BaseFuncNameForPC down to BaseFuncNameForPC
	periodIndex := strings.LastIndex(fullFuncName, ".")
	if periodIndex >= 0 {
		return fullFuncName[(periodIndex + 1):]
	}

	// no period present.  Just return the full function name
	return fullFuncName
}

func containsAnyIgnoredFilename(s string) bool {
	for _, ign := range getIgnoredFileNames() {
		if strings.Contains(s, ign) {
			return true
		}
	}
	return false
}

/*
DO NOT MAKE FUNCTION PUBLIC! Keeping this private lets us skip the stack frames for
this function and the function in this package that calls `nonSloggerCaller()`, therefore
allowing us to set the skip=2 in the loop below.
*/
func nonSloggerCaller() (pc uintptr, file string, line int, ok bool) {
	for skip := 2; skip < 100; skip++ {
		pc, file, line, ok := runtime.Caller(skip)
		if !ok || !containsAnyIgnoredFilename(file) {
			return pc, file, line, ok
		}
	}
	return 0, "", 0, false
}

func (self *Logger) logf(level Level, errorCode ErrorCode, messageFmt string, context *Context, args ...interface{}) (*Log, []error) {
	var errors []error

	for _, filter := range self.TurboFilters {
		if filter(level, messageFmt, args) == false {
			return nil, errors
		}
	}

	pc, file, line, ok := nonSloggerCaller()
	if ok == false {
		return nil, []error{fmt.Errorf("Failed to find the calling method.")}
	}

	file = stripDirectories(file, self.StripDirs)
	log := &Log{
		Prefix:     self.Prefix,
		Level:      level,
		ErrorCode:  errorCode,
		Filename:   file,
		FuncName:   baseFuncNameForPC(pc),
		Line:       line,
		Timestamp:  time.Now(),
		MessageFmt: messageFmt,
		Args:       args,
		Context:    context,
	}

	for _, appender := range self.Appenders {
		if err := appender.Append(log); err != nil {
			error := fmt.Errorf("Error appending. Appender: %T Error: %v", appender, err)
			errors = append(errors, error)
		}
	}

	return log, errors
}

type Level uint8

// The level is in an order such that the expressions
// `level < WARN`, `level >= INFO` have intuitive meaning.
const (
	TRACE Level = iota
	DEBUG
	INFO
	WARN
	ERROR
	FATAL
	OFF
	topLevel
)

var strToLevel map[string]Level

var levelToStr []string

func init() {
	strToLevel = map[string]Level{
		"off":   OFF,
		"trace": TRACE,
		"debug": DEBUG,
		"info":  INFO,
		"warn":  WARN,
		"error": ERROR,
		"fatal": FATAL,
	}

	levelToStr = make([]string, len(strToLevel))
	for str, level := range strToLevel {
		levelToStr[uint8(level)] = str
	}
}

func NewLevel(levelStr string) (Level, error) {
	level, ok := strToLevel[strings.ToLower(levelStr)]

	if !ok {
		err := UnknownLevelError{levelStr}
		return OFF, err
	}

	return level, nil
}

func (self Level) String() string {
	return self.Type()
}

func (self Level) Type() string {
	if self >= topLevel {
		return "off?"
	}

	return levelToStr[uint8(self)]
}

type ErrorCode uint8

const NoErrorCode = 0

func stacktrace() []string {
	ret := make([]string, 0, 2)
	for skip := 2; true; skip++ {
		_, file, line, ok := runtime.Caller(skip)
		if ok == false {
			break
		}

		ret = append(ret, fmt.Sprintf("at %s:%d", stripDirectories(file, 2), line))
	}

	return ret
}

type StackError struct {
	Message    string
	Stacktrace []string
}

func NewStackError(messageFmt string, args ...interface{}) *StackError {
	return &StackError{
		Message:    fmt.Sprintf(messageFmt, args...),
		Stacktrace: stacktrace(),
	}
}

func (self *StackError) Error() string {
	return fmt.Sprintf("%s\n\t%s", self.Message, strings.Join(self.Stacktrace, "\n\t"))
}

func stripDirectories(filepath string, toKeep int) string {
	var idxCutoff int

	// Look for forward slashes ('/') regardless of OS because the Go
	// runtime uses forward slashes regardless of OS
	if idxCutoff = strings.LastIndex(filepath, string('/')); idxCutoff == -1 {
		return filepath
	}

	for dirToKeep := 0; dirToKeep < toKeep; dirToKeep++ {
		switch idx := strings.LastIndex(filepath[:idxCutoff], string('/')); idx {
		case -1:
			break
		default:
			idxCutoff = idx
		}
	}

	return filepath[idxCutoff+1:]
}

type UnknownLevelError struct {
	levelStr string
}

func (self UnknownLevelError) Error() string {
	return fmt.Sprintf("Unknown level: %s", self.levelStr)
}
