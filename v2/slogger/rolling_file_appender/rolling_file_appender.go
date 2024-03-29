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

	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RollingFileAppender struct {
	// These fields should not need to change
	maxFileSize          int64
	maxDuration          time.Duration
	maxRotatedLogs       int
	compressRotatedLogs  bool
	maxUncompressedLogs  int
	absPath              string
	headerGenerator      func() []string
	stringWriterCallback func(*os.File) slogger.StringWriter

	lock sync.Mutex

	// These fields can change and the lock should be held when
	// reading or writing to them after construction of the
	// RollingFileAppender struct
	file        *os.File
	curFileSize int64

	// state holds "state" that is written to disk in a hidden state
	// file.  Not all "state" needs to go in here.  For example, the
	// current file size can be determined by a stat system call on
	// the file.  This state pointer should always be non-nil.  The
	// lock should also be held when reading or writing to state.
	state *state
}

type rollingFileAppenderBuilder struct {
	filename             string
	maxFileSize          int64
	maxDuration          time.Duration
	maxRotatedLogs       int
	rotateIfExists       bool
	compressRotatedLogs  bool
	maxUncompressedLogs  int
	headerGenerator      func() []string
	stringWriterCallback func(*os.File) slogger.StringWriter
}

// NewBuilder returns a new rollingFileAppenderBuilder. You can directly
// call Build() to create a new RollingFileAppender, or configure
// additional options first.
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
// If both maxFileSize and maxDuration are set than the log file will
// be rotated whenever either threshold is met.  The duration used to
// determine whether a log file should be rotated (that is, the
// duration compared to maxDuration) is reset regardless of why the
// log was rotated previously.
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
// Note that after building a RollingFileAppender with Build(), you will
// probably want to defer a call to RollingFileAppender's Close() (or
// at least Flush()).  This ensures that in case of program exit
// (normal or panicking) that any pending logs are logged.
func NewBuilder(filename string, maxFileSize int64, maxDuration time.Duration, maxRotatedLogs int, rotateIfExists bool, headerGenerator func() []string) *rollingFileAppenderBuilder {
	return &rollingFileAppenderBuilder{
		filename:             filename,
		maxFileSize:          maxFileSize,
		maxDuration:          maxDuration,
		maxRotatedLogs:       maxRotatedLogs,
		rotateIfExists:       rotateIfExists,
		compressRotatedLogs:  false,
		maxUncompressedLogs:  0,
		headerGenerator:      headerGenerator,
		stringWriterCallback: nil,
	}
}

func (b *rollingFileAppenderBuilder) WithLogCompression(maxUncompressedLogs int) *rollingFileAppenderBuilder {
	b.compressRotatedLogs = true
	b.maxUncompressedLogs = maxUncompressedLogs
	return b
}

func (b *rollingFileAppenderBuilder) WithStringWriter(stringWriterCallback func(*os.File) slogger.StringWriter) *rollingFileAppenderBuilder {
	b.stringWriterCallback = stringWriterCallback
	return b
}

func (b *rollingFileAppenderBuilder) Build() (*RollingFileAppender, error) {
	if b.headerGenerator == nil {
		b.headerGenerator = func() []string {
			return []string{}
		}
	}
	if b.stringWriterCallback == nil {
		b.stringWriterCallback = func(f *os.File) slogger.StringWriter {
			return f
		}
	}

	absPath, err := filepath.Abs(b.filename)
	if err != nil {
		return nil, err
	}

	appender := &RollingFileAppender{
		maxFileSize:          b.maxFileSize,
		maxDuration:          b.maxDuration,
		maxRotatedLogs:       b.maxRotatedLogs,
		compressRotatedLogs:  b.compressRotatedLogs,
		maxUncompressedLogs:  b.maxUncompressedLogs,
		absPath:              absPath,
		headerGenerator:      b.headerGenerator,
		stringWriterCallback: b.stringWriterCallback,
	}

	fileInfo, err := os.Stat(absPath)
	if err == nil && b.rotateIfExists { // err == nil means file exists
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
				appender.file.Close()
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

// New creates a new RollingFileAppender.
//
// This is deprecated in favor of calling NewBuilder().Build()
func New(filename string, maxFileSize int64, maxDuration time.Duration, maxRotatedLogs int, rotateIfExists bool, headerGenerator func() []string) (*RollingFileAppender, error) {
	return NewBuilder(filename, maxFileSize, maxDuration, maxRotatedLogs, rotateIfExists, headerGenerator).Build()
}

func NewWithStringWriter(filename string, maxFileSize int64, maxDuration time.Duration, maxRotatedLogs int, rotateIfExists bool, headerGenerator func() []string, stringWriterCallback func(*os.File) slogger.StringWriter) (*RollingFileAppender, error) {
	return NewBuilder(filename, maxFileSize, maxDuration, maxRotatedLogs, rotateIfExists, headerGenerator).WithStringWriter(stringWriterCallback).Build()
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

	if err := self.file.Sync(); err != nil {
		return err
	}

	if err := self.file.Close(); err != nil {
		return &CloseError{self.absPath, err}
	}

	return nil
}

func (self *RollingFileAppender) Flush() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	if err := self.file.Sync(); err != nil {
		return &SyncError{self.absPath, err}
	}

	return nil
}

func (self *RollingFileAppender) Rotate() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	return self.rotate()
}

// Useful for manual log rotation.  For example, logrotated may rename
// the log file and then ask us to reopen it.  Before reopening it we
// will be writing to the renamed log file.  After reopening we will
// be writing to a new log file with the original name.
func (self *RollingFileAppender) Reopen() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	// close current log if we have one open
	if self.file != nil {
		if err := self.file.Sync(); err != nil {
			return &SyncError{self.absPath, err}
		}

		if err := self.file.Close(); err != nil {
			return &CloseError{self.absPath, err}
		}
	}

	fileInfo, err := os.Stat(self.absPath)
	if err == nil { // file exists
		self.curFileSize = fileInfo.Size()
	} else { // file does not exist
		self.curFileSize = 0
	}

	file, err := os.OpenFile(self.absPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666) // umask applies to perms
	if err != nil {
		self.file = nil
		return &OpenError{self.absPath, err}
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
		return 0, &NoFileError{}
	}
	f := slogger.GetFormatLogFunc()
	msg := f(log)
	bytesWritten, err = self.stringWriterCallback(self.file).WriteString(msg)

	if err != nil {
		err = &WriteError{self.absPath, err}
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
		return &MinorRotationError{err}
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
			return &MinorRotationError{err}
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
			return &RenameError{
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
		return &RenameError{oldFilename, newFilename, err}
	}
	return nil
}

func (self *RollingFileAppender) compressMaxUncompressedLogs() error {
	if self.maxUncompressedLogs < 0 {
		return nil
	}

	rotationTimes, err := self.rotationTimeSlice()

	if err != nil {
		return &MinorRotationError{err}
	}

	uncompressedRotationTimes := make(RotationTimeSlice, 0, len(rotationTimes))
	for _, v := range rotationTimes {
		if !strings.HasSuffix(v.Filename, ".gz") {
			uncompressedRotationTimes = append(uncompressedRotationTimes, v)
		}
	}

	numLogsToCompress := len(uncompressedRotationTimes) - self.maxUncompressedLogs
	if numLogsToCompress <= 0 {
		return nil
	}

	sort.Sort(uncompressedRotationTimes)
	for _, rotationTime := range uncompressedRotationTimes[:numLogsToCompress] {
		if err = self.compressLogFile(rotationTime.Filename); err != nil {
			return &MinorRotationError{err}
		}
	}
	return nil
}

func (self *RollingFileAppender) compressLogFile(logpath string) error {
	f, err := os.Open(logpath)
	if err != nil {
		return fmt.Errorf("error trying to open %v, %v", logpath, err)
	}
	defer f.Close()

	info, err := os.Stat(logpath)
	if err != nil {
		return fmt.Errorf("error trying to stat %v, %v", logpath, err)
	}
	compressedF, err := os.Create(logpath + ".gz")
	defer compressedF.Close()
	if err != nil {
		return fmt.Errorf("error trying to create %v, %v", compressedF, err)
	}

	gzipWriter := gzip.NewWriter(compressedF)
	defer gzipWriter.Close()
	gzipWriter.ModTime = info.ModTime()

	if _, err := io.Copy(gzipWriter, f); err != nil {
		return fmt.Errorf("error compressing %v, %v", logpath, err)
	}

	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("error closing gzipWriter, %v", err)
	}

	if err := compressedF.Close(); err != nil {
		return fmt.Errorf("error closing %v, %v", compressedF, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing %v, %v", logpath, err)
	}

	if err := os.Chtimes(compressedF.Name(), time.Now(), info.ModTime()); err != nil {
		return fmt.Errorf("error updating ModTime for %v, %v", compressedF.Name(), err)
	}

	if err := os.Remove(logpath); err != nil {
		return fmt.Errorf("error removing old log file %v, %v", logpath, err)
	}

	return nil
}

func (self *RollingFileAppender) rotate() error {
	// close current log if we have one open
	if self.file != nil {
		if err := self.file.Close(); err != nil {
			return &CloseError{self.absPath, err}
		}
	}
	self.curFileSize = 0

	// rename old log
	err := self.renameLogFile(self.absPath)
	if err != nil {
		return err
	}

	// create new log
	file, err := os.Create(self.absPath)
	if err != nil {
		self.file = nil
		return &OpenError{self.absPath, err}
	}
	self.file = file
	self.logHeader()

	// stamp start time
	if err = self.stampStartTime(); err != nil {
		return err
	}

	if self.compressRotatedLogs {
		if err = self.compressMaxUncompressedLogs(); err != nil {
			return err
		}
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
