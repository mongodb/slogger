package rolling_file_appender

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	headerGenerator func() string
}

// Set maxFileSize to < 0 for unlimited file size (no rotation)
func New(filename string, maxFileSize int64, maxRotatedLogs int, rotateIfExists bool, errHandler func(error), headerGenerator func() string) (*RollingFileAppender, error) {
	if errHandler == nil {
		errHandler = func(err error) { }
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

	if rotateIfExists {
		_, err = os.Stat(absPath)
		
		if err == nil {
			// file exists.  rotate it.
			// rotate() will create the new logfile and logHeader as well
			appender.rotate()
		} else {
			// does not exist
			appender.file, err = os.OpenFile(
				absPath,
				os.O_WRONLY | os.O_CREATE | os.O_EXCL,
				0666,
			)

			if err != nil {
				return nil, err
			}

			appender.logHeader()
		}

	} else { // !rotateIfExists
		appender.file, err = os.OpenFile(
			absPath,
			os.O_WRONLY | os.O_APPEND | os.O_CREATE,
			0666,
		)
		if err != nil {
			return nil, err
		}

		fileInfo, err := appender.file.Stat()
		if err != nil {
			return nil, err
		}

		appender.curFileSize = fileInfo.Size()
		appender.logHeader()
	}

	go appender.listenForAppends()
	return appender, nil 
}

func (self *RollingFileAppender) Append(log *slogger.Log) error {
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
	self.waitUntilEmpty()
	return self.file.Close()
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
		[]interface{}{APPEND_CHANNEL_SIZE},
	)
}

func internalWarningLog(messageFmt string, args []interface{}) *slogger.Log {
	return simpleLog("RollingFileAppender", slogger.WARN, 3, messageFmt, args)
}

func newRotatedFilename(baseFilename string, inc int) string {
	now := time.Now()
	now = now.Add(time.Duration(inc) * time.Second)
	return rotatedFilename(baseFilename, now)
}

func rotatedFilename(baseFilename string, t time.Time) string {
	return fmt.Sprintf("%s.%d-%02d-%02dT%02d-%02d-%02d",
		baseFilename,
		t.Year(),
		t.Month(),
		t.Day(),
		t.Hour(),
		t.Minute(),
		t.Second())
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
	if self.headerGenerator != nil {
		header := self.headerGenerator()
		log := simpleLog("header", slogger.INFO, 3, header, []interface{}{})

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

var maxTime = time.Unix(math.MaxInt64 / 2, 0) // divide by 2 to avoid this bug: https://code.google.com/p/go/issues/detail?id=6210

func (self *RollingFileAppender) removeMaxRotatedLogs() {
	timeStrs, err := self.rotatedTimeStrs()

	if err != nil {
		self.errHandler(MinorRotationError{err})
		return
	}

	timeStrsLen := len(timeStrs)
	// return if we're under the limit
	if timeStrsLen <= self.MaxRotatedLogs {
		return
	}

	// find oldest Time
	var oldestTime time.Time = maxTime
	for _, timeStr := range timeStrs {
		rotatedTime, err := time.Parse("2006-01-02T15-04-05", timeStr)

		if err == nil && rotatedTime.Before(oldestTime) {
			oldestTime = rotatedTime
		}
	}

	// remove file with oldest Time
	oldestFilename := rotatedFilename(self.absPath, oldestTime)
	err = os.Remove(oldestFilename)
	if err != nil {
		self.errHandler(MinorRotationError{err})
		return
	}

	// return if successful removal would have put us under the limit
	if timeStrsLen <= (self.MaxRotatedLogs + 1) {
		return
	}

	// Now we are in a weird case where we were over the limit by more
	// than one.  Rather than complicate the above code to find the N
	// oldest times we will just recursively call ourselves, but only
	// if we have made any progess to avoid going into an infinite
	// loop.

	// check if we've made progress
	timeStrs, err = self.rotatedTimeStrs()
	if err != nil {
		self.errHandler(MinorRotationError{err})
		return
	}
	if len(timeStrs) >= timeStrsLen {
		return
	}

	// recursively call ourself if there's more to do
	if len(timeStrs) > self.MaxRotatedLogs {
		self.removeMaxRotatedLogs()
		return // explicit return added in hopes of TCO
	}

	return
}

func (self *RollingFileAppender) renameLogFile(oldFilename string, inc int) (ok bool) {
	newFilename := newRotatedFilename(self.absPath, inc)
	_, err := os.Stat(newFilename) // check if newFilename already exists
	if err == nil {
		// exists! try incrementing by 1 second
		return self.renameLogFile(oldFilename, inc + 1)
	}
		
	err = os.Rename(oldFilename, newFilename)

	
	if err != nil {
		self.errHandler(RenameError{oldFilename, newFilename, err})
		file, err := os.OpenFile(oldFilename, os.O_RDWR, 0666)

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
	if !self.renameLogFile(self.absPath, 0) {
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

var rotatedTimeRegExp = regexp.MustCompile(`\.(\d+-\d\d-\d\dT\d\d-\d\d-\d\d)$`)

func (self *RollingFileAppender) rotatedTimeStrs() ([]string, error) {
	logDirname := filepath.Dir(self.absPath)
	logDir, err := os.Open(logDirname)
	if err != nil {
		return nil, err
	}
	defer logDir.Close()

	var filenames []string
	filenames, err = logDir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	logTimeStrs := make([]string, 0, len(filenames))
	for _, filename := range filenames {
		match := rotatedTimeRegExp.FindStringSubmatch(filename)
		if match != nil {
			logTimeStrs = append(logTimeStrs, match[1])
		}
	}

	return logTimeStrs, nil
}

func (self *RollingFileAppender) waitUntilEmpty() {
	replyCh := make(chan bool)
	self.syncCh <- replyCh
	for !(<- replyCh) {
		self.syncCh <- replyCh
	}
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
