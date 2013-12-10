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

// Package rolling_file_appender provides a slogger Appender that
// supports log rotation.

package rolling_file_appender

import (
	"fmt"
	"github.com/tolsen/slogger/v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

type RollingFileAppender struct {
	MaxFileSize     int64
	MaxRotatedLogs  int
	file            *os.File
	absPath         string
	curFileSize     int64
	headerGenerator func() []string
}

// New creates a new RollingFileAppender.  filename is path to the
// file to log to.  It can be a relative path (with respect to the
// current working directory) or an absolute path.  maxFileSize is the
// approximate file size that will be allowed before the log file is
// rotated.  Rotated log files will have suffix of the form
// .YYYY-MM-DDTHH-MM-SS or .YYYY-MM-DDTHH-MM-SS-N (where N is an
// incrementing serial number used to resolve conflicts) appended to
// them.  Set maxFileSize to a non-positive number if you wish there
// to be no limit.  maxRotatedLogs specifies the maximum number of
// rotated logs allowed before old logs are deleted.  If
// rotateIfExists is set to true and a log file with the same filename
// already exists, then the current one will be rotated.  If
// rotateIfExists is set to true and a log file with the same filename
// already exists, then the current log file will be appended to.  If
// a log file with the same filename does not exist, then a new log
// file is created regardless of the value of rotateIfExists.  As
// RotatingFileAppender is asynchronous, an errHandler can be provided
// that will be called when an error occurs.  It can set to nil if you
// do not want to provide one.  The return value headerGenerator, if
// not nil, is logged at the beginning of every log file.
//
// Note that after creating a RollingFileAppender with New(), you will
// probably want to defer a call to RollingFileAppender's Close() (or
// at least Flush()).  This ensures that in case of program exit
// (normal or panicking) that any pending logs are logged.
func New(filename string, maxFileSize int64, maxRotatedLogs int, rotateIfExists bool, headerGenerator func() []string) (*RollingFileAppender, error) {
	if headerGenerator == nil {
		headerGenerator = func() []string {
			return []string{}
		}
	}

	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	appender := &RollingFileAppender{
		MaxFileSize:     maxFileSize,
		MaxRotatedLogs:  maxRotatedLogs,
		absPath:         absPath,
		headerGenerator: headerGenerator,
	}

	fileInfo, err := os.Stat(absPath)
	if err == nil && rotateIfExists { // err == nil means file exists
		return appender, appender.rotate()
	} else {
		// we're either creating a new log file or appending to the current one
		appender.file, err = os.OpenFile(
			absPath,
			os.O_WRONLY|os.O_APPEND|os.O_CREATE,
			0666,
		)
		if err != nil {
			return nil, err
		}

		if fileInfo != nil {
			appender.curFileSize = fileInfo.Size()
		}

		return appender, appender.logHeader()
	}
}

func (self *RollingFileAppender) Append(log *slogger.Log) error {
	n, err := self.appendSansSizeTracking(log)
	self.curFileSize += int64(n)

	if err != nil {
		return err
	}

	if self.MaxFileSize > 0 && self.curFileSize > self.MaxFileSize {
		return self.rotate()
	}

	return nil
}

func (self *RollingFileAppender) Close() error {
	err := self.Flush()
	if err != nil {
		return err
	}
	return self.file.Close()
}

func (self *RollingFileAppender) Flush() error {
	return self.file.Sync()
}

func rotatedFilename(baseFilename string, t time.Time, serial int) string {
	filename := fmt.Sprintf(
		"%s.%d-%02d-%02dT%02d-%02d-%02d",
		baseFilename,
		t.Year(),
		t.Month(),
		t.Day(),
		t.Hour(),
		t.Minute(),
		t.Second(),
	)

	if serial > 0 {
		filename = fmt.Sprintf("%s-%d", filename, serial)
	}

	return filename
}

func (self *RollingFileAppender) appendSansSizeTracking(log *slogger.Log) (bytesWritten int, err error) {
	if self.file == nil {
		return 0, NoFileError{}
	}

	msg := slogger.FormatLog(log)
	bytesWritten, err = self.file.WriteString(msg)

	if err != nil {
		err = WriteError{self.absPath, err}
	}

	return
}

func (self *RollingFileAppender) logHeader() error {
	header := self.headerGenerator()
	for _, line := range header {

		log := &slogger.Log{
			Prefix:     "header",
			Level:      slogger.INFO,
			Filename:   "",
			Line:       0,
			Timestamp:  time.Now(),
			MessageFmt: line,
			Args:       []interface{}{},
		}

		// do not count header as part of size towards rotation in
		// order to prevent infinite rotation when max size is smaller
		// than header
		_, err := self.appendSansSizeTracking(log)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *RollingFileAppender) removeMaxRotatedLogs() error {
	rotationTimes, err := self.rotationTimeSlice()

	if err != nil {
		return MinorRotationError{err}
	}

	// return if we're under the limit
	if len(rotationTimes) <= self.MaxRotatedLogs {
		return nil
	}

	// otherwise remove enough of the oldest logfiles to bring us
	// under the limit
	sort.Sort(rotationTimes)
	for _, rotationTime := range rotationTimes[self.MaxRotatedLogs:] {
		if err = os.Remove(rotationTime.Filename); err != nil {
			return MinorRotationError{err}
		}
	}
	return nil
}

const MAX_ROTATE_SERIAL_NUM = 1000000000

func (self *RollingFileAppender) renameLogFile(oldFilename string) error {
	now := time.Now()

	var newFilename string
	var err error

	for serial := 0; err == nil; serial++ { // err == nil means file exists
		if serial > MAX_ROTATE_SERIAL_NUM {
			return RenameError{
				oldFilename,
				newFilename,
				fmt.Errorf("Reached max serial number: %d", MAX_ROTATE_SERIAL_NUM),
			}
		}
		newFilename = rotatedFilename(self.absPath, now, serial)
		_, err = os.Stat(newFilename)
	}

	err = os.Rename(oldFilename, newFilename)

	if err != nil {
		return RenameError{oldFilename, newFilename, err}
	}
	return nil
}

func (self *RollingFileAppender) rotate() error {
	// rename old log
	if err := self.renameLogFile(self.absPath); err != nil {
		return err
	}

	// close current log if we have one open
	if self.file != nil {
		if err := self.file.Close(); err != nil {
			return CloseError{self.absPath, err}
		}
	}
	self.curFileSize = 0

	// create new log
	file, err := os.Create(self.absPath)
	if err != nil {
		self.file = nil
		return OpenError{self.absPath, err}
	}
	self.file = file
	self.logHeader()

	// remove really old logs
	self.removeMaxRotatedLogs()

	return nil
}

func (self *RollingFileAppender) rotationTimeSlice() (RotationTimeSlice, error) {
	candidateFilenames, err := filepath.Glob(self.absPath + ".*")

	if err != nil {
		return nil, err
	}

	rotationTimes := make(RotationTimeSlice, 0, len(candidateFilenames))

	for _, candidateFilename := range candidateFilenames {
		rotationTime, err := extractRotationTimeFromFilename(candidateFilename)
		if err == nil {
			rotationTimes = append(rotationTimes, rotationTime)
		}
	}

	return rotationTimes, nil
}

type CloseError struct {
	Filename string
	Err      error
}

func (self CloseError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to close %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsCloseError(err error) bool {
	_, ok := err.(CloseError)
	return ok
}

type MinorRotationError struct {
	Err error
}

func (self MinorRotationError) Error() string {
	return ("rolling_file_appender: minor error while rotating logs: " + self.Err.Error())
}

func IsMinorRotationError(err error) bool {
	_, ok := err.(MinorRotationError)
	return ok
}

type NoFileError struct{}

func (NoFileError) Error() string {
	return "rolling_file_appender: No log file to write to"
}

func IsNoFileError(err error) bool {
	_, ok := err.(NoFileError)
	return ok
}

type OpenError struct {
	Filename string
	Err      error
}

func (self OpenError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to open %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsOpenError(err error) bool {
	_, ok := err.(OpenError)
	return ok
}

type RenameError struct {
	OldFilename string
	NewFilename string
	Err         error
}

func (self RenameError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to rename %s to %s: %s",
		self.OldFilename,
		self.NewFilename,
		self.Err.Error(),
	)
}

func IsRenameError(err error) bool {
	_, ok := err.(RenameError)
	return ok
}

type WriteError struct {
	Filename string
	Err      error
}

func (self WriteError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to write to %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsWriteError(err error) bool {
	_, ok := err.(WriteError)
	return ok
}

type RotationTime struct {
	Time     time.Time
	Serial   int
	Filename string
}

type RotationTimeSlice [](*RotationTime)

func (self RotationTimeSlice) Len() int {
	return len(self)
}

func (self RotationTimeSlice) Less(i, j int) bool {
	if self[i].Time == self[j].Time {
		return self[i].Serial < self[j].Serial
	}

	return self[i].Time.Before(self[j].Time)
}

func (self RotationTimeSlice) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}

var rotatedTimeRegExp = regexp.MustCompile(`\.(\d+-\d\d-\d\dT\d\d-\d\d-\d\d)(-(\d+))?$`)

func extractRotationTimeFromFilename(filename string) (*RotationTime, error) {
	match := rotatedTimeRegExp.FindStringSubmatch(filename)

	if match == nil {
		return nil, fmt.Errorf("Filename does not match rotation time format: %s", filename)
	}

	rotatedTime, err := time.Parse("2006-01-02T15-04-05", match[1])
	if err != nil {
		return nil, fmt.Errorf(
			"Time %s in filename %s did not parse: %v",
			match[1],
			filename,
			err,
		)
	}

	var serial int
	if match[3] != "" {
		serial, err = strconv.Atoi(match[3])

		if err != nil {
			return nil, fmt.Errorf(
				"Could not parse serial number in filename %s: %v",
				filename,
				err,
			)
		}
	}

	return &RotationTime{rotatedTime, serial, filename}, nil
}
