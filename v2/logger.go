// Copyright 2013 MongoDB, Inc.
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
	"github.com/tolsen/slogger/v2/queued_set"
	"runtime"
	"strings"
	"time"
)

type Log struct {
	Prefix     string
	Level      Level
	Filename   string
	Line       int
	Timestamp  time.Time
	MessageFmt string
	Args       []interface{}
}

func SimpleLog(prefix string, level Level, callerSkip int, messageFmt string, args []interface{}) *Log {
	_, file, line, ok := runtime.Caller(callerSkip)
	if !ok {
		file = "UNKNOWN_FILE"
		line = -1
	}
	
	return &Log {
		Prefix: prefix,
		Level: level,
		Filename: file,
		Line: line,
		Timestamp: time.Now(),
		MessageFmt: messageFmt,
		Args: args,
	}
}

func (self *Log) Message() string {
	return fmt.Sprintf(self.MessageFmt, self.Args...)
}

// for use as a cache key
func (self *Log) stringWithoutTime() string {
	return fmt.Sprintf(
		"%s %v %s %d %s",
		self.Prefix,
		self.Level.Type(),
		self.Filename,
		self.Line,
		self.Message(),
	)
}	
	

type Logger struct {
	Prefix    string
	Appenders []Appender
	StripDirs int
	cache *queued_set.QueuedSet
	suppressionEnabled bool
}

// Log a message and a level to a logger instance. This returns a
// pointer to a Log and a slice of errors that were gathered from every
// Appender (nil errors included).
func (self *Logger) Logf(level Level, messageFmt string, args ...interface{}) (*Log, []error) {
	return self.logf(level, messageFmt, args...)
}

func (self *Logger) DisableLogSuppression() {
	self.suppressionEnabled = false
	return
}

func (self *Logger) EnableLogSuppression(historyCapacity int) {
	self.cache = queued_set.New(historyCapacity)
	self.suppressionEnabled = true
	return
}

// Log and return a formatted error string.
// Example:
//
// if whatIsExpected != whatIsReturned {
//     return slogger.Errorf(slogger.WARN, "Unexpected return value. Expected: %v Received: %v",
//         whatIsExpected, whatIsReturned)
// }5
//
func (self *Logger) Errorf(level Level, messageFmt string, args ...interface{}) error {
	log, _ := self.logf(level, messageFmt, args...)
	return errors.New(log.Message())
}

func (self *Logger) Flush() (errors []error) {
	for _, appender := range self.Appenders {
		if err := appender.Flush(); err != nil {
			errors = append(errors, err)
		}
	}
	return
}

func (self *Logger) IsSuppressionEnabled() bool {
	return self.suppressionEnabled
}

// Stackf is designed to work in tandem with `NewStackError`. This
// function is similar to `Logf`, but takes a `stackErr`
// parameter. `stackErr` is expected to be of type StackError, but does
// not have to be.
func (self *Logger) Stackf(level Level, stackErr error, messageFmt string, args ...interface{}) (*Log, []error) {
	messageFmt = fmt.Sprintf("%v\n%v", messageFmt, stackErr.Error())
	return self.logf(level, messageFmt, args...)
}




var ignoredFileNames = []string{"logger.go"}


// Add a file to the list of file names that slogger will skip when it identifies the source 
// of a message.  This is useful if you have a logging library built on top of slogger.
// If you IgnoreThisFilenameToo(...) on the files of that library, logging messages
// will be marked as coming from your code that calls your library, rather than from your library.
func IgnoreThisFilenameToo(fn string) {
	ignoredFileNames = append(ignoredFileNames, fn)
}

func containsAnyIgnoredFilename(s string) bool {
	for _, ign := range ignoredFileNames {
		if strings.Contains(s, ign)  {
			return true
		}
	}
	return false
}

func nonSloggerCaller() (pc uintptr, file string, line int, ok bool) {
	for skip := 0; skip < 100; skip++ {
		pc,file,line,ok := runtime.Caller(skip)
		if !ok || !containsAnyIgnoredFilename(file)  {
			return pc,file,line,ok
		}
	}
	return 0, "", 0, false
}

func (self *Logger) logf(level Level, messageFmt string, args ...interface{}) (*Log, []error) {
	var errors []error

	_, file, line, ok := nonSloggerCaller()
//	_, file, line, ok := runtime.Caller(2+offset)
	if ok == false {
		return nil, []error{fmt.Errorf("Failed to find the calling method.")}
	}

	file = stripDirectories(file, self.StripDirs)

	log := &Log{
		Prefix:     self.Prefix,
		Level:      level,
		Filename:   file,
		Line:       line,
		Timestamp:  time.Now(),
		MessageFmt: messageFmt,
		Args:       args,
	}

	if !self.suppressionEnabled || self.cache.Add(log.stringWithoutTime()) {
		for _, appender := range self.Appenders {
			if err := appender.Append(log); err != nil {
				error := fmt.Errorf("Error appending. Appender: %T Error: %v", appender, err)
				errors = append(errors, error)
			}
		}
	}

	return log, errors
}

type Level uint8

// The level is in an order such that the expressions
// `level < WARN`, `level >= INFO` have intuitive meaning.
const (
	OFF Level = iota
	DEBUG
	ROUTINE
	INFO
	WARN
	ERROR
	DOOM
	topLevel
)

var strToLevel map[string]Level

var levelToStr []string

func init() {
	strToLevel = map[string]Level {
		"off"     : OFF,
		"debug"   : DEBUG,
		"routine" : ROUTINE,
		"info"    : INFO,
		"warn"    : WARN,
		"error"   : ERROR,
		"doom"    : DOOM,
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

func (self Level) Type() string {
	if self >= topLevel {
		return "off?"
	}

	return levelToStr[uint8(self)]
}

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
	if idxCutoff = strings.LastIndex(filepath, "/"); idxCutoff == -1 {
		return filepath
	}

	for dirToKeep := 0; dirToKeep < toKeep; dirToKeep++ {
		switch idx := strings.LastIndex(filepath[:idxCutoff], "/"); idx {
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
	
