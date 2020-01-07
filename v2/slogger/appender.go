// Copyright 2013 - 2015 MongoDB, Inc.
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
	"bytes"
	"fmt"
	"os"
)

type Appender interface {
	Append(log *Log) error
	Flush() error
}

var formatLogFunc = FormatLog

func GetFormatLogFunc() func(log *Log) string {
	loggerConfigLock.RLock()
	defer loggerConfigLock.RUnlock()
	return formatLogFunc
}

func SetFormatLogFunc(f func(log *Log) string) {
	loggerConfigLock.Lock()
	defer loggerConfigLock.Unlock()
	formatLogFunc = f

}

func formatLog(log *Log, timePart string) string {

	errorCodeStr := ""
	if log.ErrorCode != NoErrorCode {
		errorCodeStr += fmt.Sprintf("[%v] ", log.ErrorCode)
	}

	return fmt.Sprintf("%v [%v.%v] [%v:%v:%d] %v%v\n",
		timePart, log.Prefix, log.Level.Type(),
		log.Filename, log.FuncName, log.Line,
		errorCodeStr,
		log.Message())
}

func convertOffsetToString(offset int) string {
	var sign string
	if offset > 0 {
		sign = "+"
	} else {
		sign = "-"
	}
	hoursOffset := float32(offset) / 3600.0
	var leadingZero string
	if hoursOffset > -9 && hoursOffset < 9 {
		leadingZero = "0"
	}
	return fmt.Sprintf("%s%s%.0f", sign, leadingZero, hoursOffset*100.0)
}

func FormatLogWithTimezone(log *Log) string {
	year, month, day := log.Timestamp.Date()
	hour, min, sec := log.Timestamp.Clock()
	millisec := log.Timestamp.Nanosecond() / 1000000
	_, offset := log.Timestamp.Zone() // offset in seconds

	return formatLog(log, fmt.Sprintf("[%.4d-%.2d-%.2dT%.2d:%.2d:%.2d.%.3d%s]",
		year, month, day,
		hour, min, sec,
		millisec,
		convertOffsetToString(offset)),
	)
}

func FormatLog(log *Log) string {
	year, month, day := log.Timestamp.Date()
	hour, min, sec := log.Timestamp.Clock()
	millisec := log.Timestamp.Nanosecond() / 1000000

	return formatLog(log, fmt.Sprintf("[%.4d/%.2d/%.2d %.2d:%.2d:%.2d.%.3d]",
		year, month, day,
		hour, min, sec,
		millisec,
	))
}

type StringWriter interface {
	WriteString(s string) (ret int, err error)
	Sync() error
}

type FileAppender struct {
	StringWriter
}

func (self FileAppender) Append(log *Log) error {
	f := GetFormatLogFunc()
	_, err := self.WriteString(f(log))
	return err
}

func (self FileAppender) Flush() error {
	return self.Sync()
}

func StdOutAppender() *FileAppender {
	return &FileAppender{os.Stdout}
}

func StdErrAppender() *FileAppender {
	return &FileAppender{os.Stderr}
}

func DevNullAppender() (*FileAppender, error) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}

	return &FileAppender{devNull}, nil
}

type StringAppender struct {
	*bytes.Buffer
}

func NewStringAppender(buffer *bytes.Buffer) *StringAppender {
	return &StringAppender{buffer}
}

func (self StringAppender) Append(log *Log) error {
	f := GetFormatLogFunc()
	_, err := self.WriteString(f(log))
	return err
}

func (self StringAppender) Flush() error {
	return nil
}

// Return true if the log should be passed to the underlying
// `Appender`
type Filter func(log *Log) bool
type FilterAppender struct {
	Appender Appender
	Filter   Filter
}

func (self *FilterAppender) Append(log *Log) error {
	if self.Filter(log) == false {
		return nil
	}

	return self.Appender.Append(log)
}

func (self *FilterAppender) Flush() error {
	return self.Appender.Flush()
}

func LevelFilter(threshold Level, appender Appender) *FilterAppender {
	filterFunc := func(log *Log) bool {
		return log.Level >= threshold
	}

	return &FilterAppender{
		Appender: appender,
		Filter:   filterFunc,
	}
}
