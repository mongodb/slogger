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

package rolling_file_appender

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sort"
	"time"
	"github.com/tolsen/slogger/v2"
)

// Do not set this to zero or deadlocks might occur
const APPEND_CHANNEL_SIZE = 4096

type RollingFileAppender struct {
	MaxFileSize int64
	MaxRotatedLogs int
	file *os.File
	absPath string
	curFileSize int64
	appendCh chan *slogger.Log
	syncCh chan (chan bool)
	errHandler func(error)
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
func New(filename string, maxFileSize int64, maxRotatedLogs int, rotateIfExists bool, errHandler func(error), headerGenerator func() []string) (*RollingFileAppender, error) {
	if errHandler == nil {
		errHandler = func(err error) { }
	}

	if headerGenerator == nil {
		headerGenerator = func() []string {
			return []string{}
		}
	}

	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	appender := &RollingFileAppender {
		MaxFileSize: maxFileSize,
		MaxRotatedLogs: maxRotatedLogs,
		absPath: absPath,
		appendCh: make(chan *slogger.Log, APPEND_CHANNEL_SIZE),
		syncCh: make(chan (chan bool)),
		errHandler: errHandler,
		headerGenerator: headerGenerator,
	}

	
	fileInfo, err := os.Stat(absPath)  
	if err == nil && rotateIfExists {  // err == nil means file exists
		appender.rotate()
	} else {
		// we're either creating a new log file or appending to the current one
		appender.file, err = os.OpenFile(
			absPath,
			os.O_WRONLY | os.O_APPEND | os.O_CREATE,
			0666,
		)
		if err != nil {
			return nil, err
		}

		if fileInfo != nil {
			appender.curFileSize = fileInfo.Size()
		}
		
		appender.logHeader()
	}

	go appender.listenForAppends()
	return appender, nil 
}

func (self RollingFileAppender) Append(log *slogger.Log) error {
	select {
	case self.appendCh <- log:
		// nothing else to do
	default:
		// channel is full. log a warning
		self.appendCh <- fullWarningLog()
		self.appendCh <- log
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
	replyCh := make(chan bool)
	self.syncCh <- replyCh
	for !(<- replyCh) {
		self.syncCh <- replyCh
	}
	return nil
}

// These are commented out until I determine as to whether they are thread-safe -Tim

// func (self RollingFileAppender) SetErrHandler(errHandler func(error)) {
// 	self.errHandler = errHandler
// }

// func (self RollingFileAppender) SetHeaderGenerator(headerGenerator func() string) {
// 	self.headerGenerator = headerGenerator
// 	self.logHeader()
// }

func fullWarningLog() *slogger.Log {
	return internalWarningLog(
		"appendCh is full. You may want to increase APPEND_CHANNEL_SIZE (currently %d).",
		APPEND_CHANNEL_SIZE,
	)
}

func internalWarningLog(messageFmt string, args ...interface{}) *slogger.Log {
	return simpleLog("RollingFileAppender", slogger.WARN, 3, messageFmt, args)
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

func simpleLog(prefix string, level slogger.Level, callerSkip int, messageFmt string, args []interface{}) *slogger.Log {
	_, file, line, ok := runtime.Caller(callerSkip)
	if !ok {
		file = "UNKNOWN_FILE"
		line = -1
	}
	
	return &slogger.Log {
		Prefix: prefix,
		Level: level,
		Filename: file,
		Line: line,
		Timestamp: time.Now(),
		MessageFmt: messageFmt,
		Args: args,
	}
}

// listenForAppends consumes appendCh and syncCh.  It consumes Logs
// coming down the appendCh, flushing to disk when necessary and the
// appendCh is empty.  It will reply to syncCh messages (via the given
// syncReplyCh) after flushing (or if nothing has ever been logged),
// increasing the chance that it will be able to reply true.
func (self *RollingFileAppender) listenForAppends() {
	needsSync := false
	for {
		if needsSync {
			select {
			case log := <- self.appendCh:
				self.reallyAppend(log, true)
			default:
				self.file.Sync()
				needsSync = false
			}
		} else {
			select {
			case log := <- self.appendCh:
				self.reallyAppend(log, true)
				needsSync = true
			case syncReplyCh := <- self.syncCh:
				syncReplyCh <- (len(self.appendCh) <= 0)
			}
		}
	}
}

func (self *RollingFileAppender) logHeader() {
	header := self.headerGenerator()
	for _, line := range header {
		log := simpleLog("header", slogger.INFO, 3, line, []interface{}{})

		// do not count header as part of size towards rotation in
		// order to prevent infinite rotation when max size is smaller
		// than header
		self.reallyAppend(log, false)
	}
}

func (self *RollingFileAppender) reallyAppend(log *slogger.Log, trackSize bool) {
	if self.file == nil {
		self.errHandler(NoFileError{})
		return
	}
	
	msg := slogger.FormatLog(log)

	n, err := self.file.WriteString(msg)

	if err != nil {
		self.errHandler(WriteError{self.absPath, err})
		return
	}

	if trackSize && self.MaxFileSize > 0 {
		self.curFileSize += int64(n)

		if self.curFileSize > self.MaxFileSize {
			self.rotate()
		}
	}
	return
}

func (self *RollingFileAppender) removeMaxRotatedLogs() {
	rotationTimes, err := self.rotationTimeSlice()

	if err != nil {
		self.errHandler(MinorRotationError{err})
		return
	}

	// return if we're under the limit
	if len(rotationTimes) <= self.MaxRotatedLogs {
		return
	}

	// otherwise remove enough of the oldest logfiles to bring us
	// under the limit
	sort.Sort(rotationTimes)
	for _, rotationTime := range rotationTimes[self.MaxRotatedLogs:] {
		err = os.Remove(rotationTime.Filename)
		if err != nil {
			self.errHandler(MinorRotationError{err})
			return
		}
	}
	return
}

const MAX_ROTATE_SERIAL_NUM = 1000000000

func (self *RollingFileAppender) renameLogFile(oldFilename string) (ok bool) {
	now := time.Now()

	var newFilename string
	var err error
	
	for serial := 0; err == nil; serial++ {  // err == nil means file exists
		if serial > MAX_ROTATE_SERIAL_NUM {
			self.errHandler(
				RenameError{
					oldFilename,
					newFilename,
					fmt.Errorf("Reached max serial number: %d", MAX_ROTATE_SERIAL_NUM),
				},
			)
		}
		newFilename = rotatedFilename(self.absPath, now, serial)
		_, err = os.Stat(newFilename) 
	}
		
	err = os.Rename(oldFilename, newFilename)

	
	if err != nil {
		self.errHandler(RenameError{oldFilename, newFilename, err})
		file, err := os.OpenFile(
			oldFilename,
			os.O_WRONLY | os.O_APPEND,
			0666,
		)

		if err == nil {
			self.file = file
		} else {
			self.curFileSize = 0
			self.file = nil
			self.errHandler(OpenError{oldFilename, err})
		}
		return false
	}
	self.curFileSize = 0
	return true
}


func (self *RollingFileAppender) rotate() {
	// close current log if we have one open
	if self.file != nil {
		if err := self.file.Close(); err != nil {
			self.errHandler(CloseError{self.absPath, err})
		}
	}

	// rename old log
	if !self.renameLogFile(self.absPath) {
		return
	}

	// remove really old logs
	self.removeMaxRotatedLogs()

	// create new log
	file, err := os.Create(self.absPath)
	if err != nil {
		self.file = nil
		self.errHandler(OpenError{self.absPath, err})
		return
	}

	self.file = file
	self.logHeader()
	return
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
	Err error
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
	return("rolling_file_appender: minor error while rotating logs: " + self.Err.Error())
}

func IsMinorRotationError(err error) bool {
	_, ok := err.(MinorRotationError)
	return ok
}

type NoFileError struct {}

func (NoFileError) Error() string {
	return "rolling_file_appender: No log file to write to"
}

func IsNoFileError(err error) bool {
	_, ok := err.(NoFileError)
	return ok
}

type OpenError struct {
	Filename string
	Err error
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
	Err error
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
	Err error
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
	Time time.Time
	Serial int
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
		
				
	
	
			
	
