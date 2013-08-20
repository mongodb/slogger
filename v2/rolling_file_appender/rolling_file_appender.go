package rolling_file_appender

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
	".."
)

// Do not set this to zero or deadlocks might occur
const ROLLING_FILE_APPENDER_CHANNEL_SIZE = 4096

type RollingFileAppender struct {
	MaxFileSize uint64
	file *os.File
	absPath string
	curFileSize uint64
	appendCh chan *slogger.Log
	syncCh chan (chan bool)
	errHandler func(error)
	headerGenerator func() string
}

func NewRollingFileAppender(filename string, maxFileSize uint64, errHandler func(error), headerGenerator func() string) (*RollingFileAppender, error) {
	if errHandler == nil {
		errHandler = func(err error) { }
	}

	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(
		absPath,
		os.O_WRONLY | os.O_APPEND | os.O_CREATE,
		0666,
	)
	if err != nil {
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	curFileSize := uint64(fileInfo.Size())
	
	appender := &RollingFileAppender {
		MaxFileSize: maxFileSize,
		file: file,
		absPath: absPath,
		curFileSize: curFileSize,
		appendCh: make(chan *slogger.Log, ROLLING_FILE_APPENDER_CHANNEL_SIZE),
		syncCh: make(chan (chan bool)),
		errHandler: errHandler,
		headerGenerator: headerGenerator,
	}

	go appender.listenForAppends()
	appender.logHeader()
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

func (self RollingFileAppender) Close() error {
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
		"appendCh is full. You may want to increase ROLLING_FILE_APPENDER_CHANNEL_SIZE (currently %d).",
		[]interface{}{ROLLING_FILE_APPENDER_CHANNEL_SIZE},
	)
}

func internalWarningLog(messageFmt string, args []interface{}) *slogger.Log {
	return simpleLog("RollingFileAppender", slogger.WARN, 3, messageFmt, args)
}

func newRotatedFilename(baseFilename string) string {
	now := time.Now()

	return fmt.Sprintf("%s.%d-%02d-%02dT%02d-%02d-%02d",
		baseFilename,
		now.Year(),
		now.Month(),
		now.Day(),
		now.Hour(),
		now.Minute(),
		now.Second())
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

func (self RollingFileAppender) listenForAppends() {
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

func (self RollingFileAppender) logHeader() {
	if self.headerGenerator != nil {
		header := self.headerGenerator()
		log := simpleLog("header", slogger.INFO, 3, header, []interface{}{})

		// do not count header as part of size towards rotation in
		// order to prevent infinite rotation when max size is smaller
		// than header
		self.reallyAppend(log, false)
	}
}

func (self RollingFileAppender) reallyAppend(log *slogger.Log, trackSize bool) {
	if self.file == nil {
		self.errHandler(errors.New("I have no logfile to write to!"))
		return
	}
	
	msg := slogger.FormatLog(log)

	n, err := self.file.WriteString(msg)

	if err != nil {
		self.errHandler(fmt.Errorf("Could not log to %s : %s", self.file.Name(), err.Error()))
		return
	}

	if trackSize {
		self.curFileSize += uint64(n)

		if self.curFileSize > self.MaxFileSize {
			self.rotate()
		}
	}
	return
}

// returns true on success, false otherwise
func (self RollingFileAppender) renameLogFile(oldFilename, newFilename string) bool {
	err := os.Rename(oldFilename, newFilename)
	if err != nil {
		self.errHandler(fmt.Errorf(
			"Error while renaming %s to %s . Will reopen. : %s",
			oldFilename, newFilename, err.Error()))

		file, err := os.OpenFile(oldFilename, os.O_RDWR, 0666)

		if err == nil {
			self.file = file
		} else {
			self.curFileSize = 0
			self.file = nil
			self.errHandler(fmt.Errorf(
				"Error while reopening %s after failing to rename. Further logging will fail: %s",
				oldFilename, err.Error()))
		}
		return false
	}
	self.curFileSize = 0
	return true
}


func (self RollingFileAppender) rotate() {
	// close current log
	if err := self.file.Close(); err != nil {
		self.errHandler(fmt.Errorf(
			"Error while closing %s : %s" , self.absPath, err.Error()))
	}

	// rename old log
	if !self.renameLogFile(self.absPath, newRotatedFilename(self.absPath)) {
		return
	}

	// create new log
	file, err := os.Create(self.absPath)

	if err != nil {
		self.file = nil
		self.errHandler(fmt.Errorf(
			"Failed to create %s . Further logging will fail. : %s",
			self.absPath, err.Error()))
		return
	}

	self.file = file
	self.logHeader()
	return
}

func (self RollingFileAppender) waitUntilEmpty() {
	replyCh := make(chan bool)
	self.syncCh <- replyCh
	for !(<- replyCh) {
		self.syncCh <- replyCh
	}
}
