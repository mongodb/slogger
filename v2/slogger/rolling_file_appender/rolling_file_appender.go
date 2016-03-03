// Copyright 2013, 2014 MongoDB, Inc.
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
	"github.com/mongodb/slogger/v2/slogger"

	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type RollingFileAppender struct {
	maxFileSize          int64
	maxDuration          time.Duration
	maxRotatedLogs       int
	file                 *os.File
	absPath              string
	curFileSize          int64
	headerGenerator      func() []string
	stringWriterCallback func(*os.File) slogger.StringWriter
	lock                 sync.Mutex
	state                *state
}

// New creates a new RollingFileAppender.
//
// filename is path to the file to log to.  It can be a relative path
// (with respect to the current working directory) or an absolute
// path.
//
// maxFileSize is the approximate file size that will be allowed
// before the log file is rotated.  Rotated log files will have suffix
// of the form .YYYY-MM-DDTHH-MM-SS or .YYYY-MM-DDTHH-MM-SS-N (where N
// is an incrementing serial number used to resolve conflicts)
// appended to them.  Set maxFileSize to a non-positive number if you
// wish there to be no limit.
//
// maxDuration is how long to wait before rotating the log file.  Set
// to 0 if you do not want log rotation to be time-based.
//
// maxRotatedLogs specifies the maximum number of rotated logs allowed
// before old logs are deleted.  Set to a non-positive number if you
// do not want old log files to be deleted.
//
// If rotateIfExists is set to true and a log file with the same
// filename already exists, then the current one will be rotated.  If
// rotateIfExists is set to false and a log file with the same
// filename already exists, then the current log file will be appended
// to.  If a log file with the same filename does not exist, then a
// new log file is created regardless of the value of rotateIfExists.
//
// As RotatingFileAppender might be wrapped by an AsyncAppender, an
// errHandler can be provided that will be called when an error
// occurs.  It can set to nil if you do not want to provide one.
//
// The return value headerGenerator, if not nil, is logged at the
// beginning of every log file.
//
// Note that after creating a RollingFileAppender with New(), you will
// probably want to defer a call to RollingFileAppender's Close() (or
// at least Flush()).  This ensures that in case of program exit
// (normal or panicking) that any pending logs are logged.
func New(filename string, maxFileSize int64, maxDuration time.Duration, maxRotatedLogs int, rotateIfExists bool, headerGenerator func() []string) (*RollingFileAppender, error) {
	return NewWithStringWriter(filename, maxFileSize, maxDuration, maxRotatedLogs, rotateIfExists, headerGenerator, nil)
}

func NewWithStringWriter(filename string, maxFileSize int64, maxDuration time.Duration, maxRotatedLogs int, rotateIfExists bool, headerGenerator func() []string, stringWriterCallback func(*os.File) slogger.StringWriter) (*RollingFileAppender, error) {
	if headerGenerator == nil {
		headerGenerator = func() []string {
			return []string{}
		}
	}
	if stringWriterCallback == nil {
		stringWriterCallback = func(f *os.File) slogger.StringWriter {
			return f
		}
	}

	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	appender := &RollingFileAppender{
		maxFileSize:          maxFileSize,
		maxDuration:          maxDuration,
		maxRotatedLogs:       maxRotatedLogs,
		absPath:              absPath,
		headerGenerator:      headerGenerator,
		stringWriterCallback: stringWriterCallback,
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

		stateExistsVar, err := stateExists(appender.statePath())
		if err != nil {
			appender.file.Close()
			return nil, err
		}

		if stateExistsVar {
			if err = appender.loadState(); err != nil {
				return nil, err
			}
		} else {
			if err = appender.stampStartTime(); err != nil {
				appender.file.Close()
				return nil, err
			}
		}

		return appender, appender.logHeader()
	}
}

func (self *RollingFileAppender) Append(log *slogger.Log) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	n, err := self.appendSansSizeTracking(log)
	self.curFileSize += int64(n)

	if err != nil {
		return err
	}

	if (self.maxFileSize > 0 && self.curFileSize > self.maxFileSize) ||
		(self.maxDuration > 0 &&
			self.state != nil &&
			time.Since(self.state.LogStartTime) > self.maxDuration) {
		return self.rotate()
	}

	return nil
}

func (self *RollingFileAppender) Close() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	err := self.file.Sync()
	if err != nil {
		return err
	}
	return self.file.Close()
}

func (self *RollingFileAppender) Flush() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	return self.file.Sync()
}

func (self *RollingFileAppender) Rotate() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	return self.rotate()
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
	bytesWritten, err = self.stringWriterCallback(self.file).WriteString(msg)

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
	if self.maxRotatedLogs <= 0 {
		return nil
	}

	rotationTimes, err := self.rotationTimeSlice()

	if err != nil {
		return MinorRotationError{err}
	}

	numLogsToDelete := len(rotationTimes) - self.maxRotatedLogs

	// return if we're under the limit
	if numLogsToDelete <= 0 {
		return nil
	}

	// otherwise remove enough of the oldest logfiles to bring us
	// under the limit
	sort.Sort(rotationTimes)
	for _, rotationTime := range rotationTimes[:numLogsToDelete] {
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
	// close current log if we have one open
	if self.file != nil {
		if err := self.file.Close(); err != nil {
			return CloseError{self.absPath, err}
		}
	}
	self.curFileSize = 0

	// rename old log
	if err := self.renameLogFile(self.absPath); err != nil {
		return err
	}

	// create new log
	file, err := os.Create(self.absPath)
	if err != nil {
		self.file = nil
		return OpenError{self.absPath, err}
	}
	self.file = file
	self.logHeader()

	// stamp start time
	if err = self.stampStartTime(); err != nil {
		return err
	}

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

func (self *RollingFileAppender) loadState() error {
	state, err := readState(self.statePath())
	if err != nil {
		return err
	}

	self.state = state
	return nil
}

func (self *RollingFileAppender) stampStartTime() error {
	state := newState(time.Now())
	if err := state.write(self.statePath()); err != nil {
		return err
	}
	self.state = state
	return nil
}
