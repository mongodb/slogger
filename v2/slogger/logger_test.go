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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestLevels(test *testing.T) {
	levelType := fmt.Sprintf("%T", OFF)
	if levelType != "slogger.Level" {
		test.Errorf("Bad Level type. Expected: `slogger.Level` Received: `%v`", levelType)
	}
}

func TestFormat(test *testing.T) {
	log := Log{
		Prefix:     "agent.OplogTail",
		Level:      INFO,
		Filename:   "oplog.go",
		FuncName:   "TailOplog",
		Line:       88,
		MessageFmt: "Tail started on RsId: `backup_test`",
	}

	expected := "[0001/01/01 00:00:00.000] [agent.OplogTail.info] [oplog.go:TailOplog:88] Tail started on RsId: `backup_test`\n"
	received := FormatLog(&log)
	if received != expected {
		test.Errorf("Improperly formatted log. Received: `%v`", received)
	}
}

func TestLog(test *testing.T) {
	const logFilename = "logger_test.output"
	logfile, err := os.Create(logFilename)
	if err != nil {
		test.Fatal("Cannot create `logger_test.output` file.")
	}
	defer os.Remove(logFilename)

	logger := &Logger{
		Prefix:    "agent.OplogTail",
		Appenders: []Appender{&FileAppender{logfile}},
	}

	const logMessage = "Please disregard the imminent warning. This is just a test."
	logger.Logf(WARN, logMessage)
	fileOutputBytes, err := ioutil.ReadFile(logFilename)
	if err != nil {
		test.Fatal("Could not read entire file contents")
	}

	fileOutput := string(fileOutputBytes)
	if strings.Contains(fileOutput, "logger_test.go") == false {
		test.Fatalf("Incorrect filename. Expected: `%v` Full log: `%v`", logFilename, fileOutput)
	}

	if strings.Contains(fileOutput, logMessage) == false {
		test.Fatalf("Incorrect message. Expected: `%v` Full log: `%v`", logMessage, fileOutput)
	}

	if !strings.Contains(fileOutput, "TestLog") {
		test.Fatalf("Incorrect function name. Expected `TestLog` Full log: `%v`", fileOutput)
	}
}

type countingAppender struct {
	count int
}

func (self *countingAppender) Append(log *Log) error {
	self.count++
	return nil
}

func (self *countingAppender) Flush() error {
	return nil
}

func TestFilter(test *testing.T) {
	counter := &countingAppender{}
	logger := &Logger{
		Prefix:    "agent.OplogTail",
		Appenders: []Appender{LevelFilter(WARN, counter)},
	}

	logger.Logf(INFO, "%d", 0)
	logger.Logf(WARN, "%d", 1)
	logger.Logf(ERROR, "%d", 2)
	logger.Logf(DEBUG, "%d", 3)

	if counter.count != 2 {
		test.Errorf("Expected two logs to pass through the filter to the appender. Received: %d",
			counter.count)
	}
}

func TestFilterOff(test *testing.T) {
	counter := &countingAppender{}
	logger := &Logger{
		Prefix:    "agent.OplogTail",
		Appenders: []Appender{LevelFilter(OFF, counter)},
	}

	logger.Logf(INFO, "%d", 0)
	logger.Logf(WARN, "%d", 1)
	logger.Logf(ERROR, "%d", 2)
	logger.Logf(DEBUG, "%d", 3)
	logger.Logf(FATAL, "%d", 4)

	if counter.count != 0 {
		test.Errorf("Expected no logs to pass through the filter to the appender. Received: %d",
			counter.count)
	}
}

func TestStacktrace(test *testing.T) {
	stacktrace := NewStackError("").Stacktrace
	if match, _ := regexp.MatchString("^at v2/slogger/logger_test.go:\\d+", stacktrace[0]); match == false {
		test.Errorf("Stacktrace level 0 did not match. Received: %v", stacktrace[0])
	}
}

func TestStripDirs(test *testing.T) {
	input := "/home/user/filename.go"
	expect := "filename.go"
	if stripDirectories(input, 0) != expect {
		test.Errorf("stripDirectories(\"%v\"); Expected: %v Received: %v",
			input, expect, stripDirectories(input, 0))
	}

	if stripDirectories(input, 1) != "user/filename.go" {
		test.Errorf("stripDirectories(\"%v\"); Expected: %v Received: %v",
			input, expect, stripDirectories(input, 1))
	}

	if stripDirectories(input, 2) != "home/user/filename.go" {
		test.Errorf("stripDirectories(\"%v\"); Expected: %v Received: %v",
			input, expect, stripDirectories(input, 2))
	}

	if stripDirectories(input, 3) != "home/user/filename.go" {
		test.Errorf("stripDirectories(\"%v\"); Expected: %v Received: %v",
			input, expect, stripDirectories(input, 3))
	}
}

func TestStackError(test *testing.T) {
	testErr := NewStackError("This is just a test")
	str := testErr.Error()
	if strings.HasPrefix(str, "This is just a test\n") == false {
		test.Errorf("Expected output to start with the message. Received:\n%v", str)
	}

	if match, _ := regexp.MatchString("v2/slogger/logger_test.go:\\d+", str); match == false {
		test.Errorf("Expected to see output for `v2/logger_test.go`. Received:\n%v", str)
	}

	match, err := regexp.MatchString("slogger/v2/logger.go:\\d+", str)
	if err != nil {
		test.Errorf("Error matching: %v", err)
	}

	if match == true {
		test.Errorf("The stacktrace should have no output from slogger/logger.go. Received:\n%v", str)
	}
}

func assertZero(number int) error {
	if number < 0 {
		return NewStackError("Number is expected to be zero. Was negative: %d", number)
	}

	if -number < 0 {
		return NewStackError("Number is expected to be zero. Was positive: %d", number)
	}

	return nil
}

func addZero(number, zero int, logger *Logger) (int, error) {
	if err := assertZero(zero); err != nil {
		return 0, err
	}

	return number + zero, nil
}

func TestStacktracing(test *testing.T) {
	logBuffer := new(bytes.Buffer)
	logger := &Logger{
		Prefix:    "slogger.logger_test",
		Appenders: []Appender{NewStringAppender(logBuffer)},
	}

	_, err := addZero(6, 0, logger)
	if err != nil {
		logger.Stackf(WARN, err, "Had an illegal argument to addZero. %d", 0)
	}
	logOutput, _ := ioutil.ReadAll(logBuffer)
	if len(logOutput) > 0 {
		test.Errorf("Did not expect any log messages from this first call.")
	}

	_, err = addZero(5, 2, logger)
	if err != nil {
		logger.Stackf(WARN, err, "Had an illegal argument to addZero. %d", 2)
	}
	logOutput, _ = ioutil.ReadAll(logBuffer)
	if len(logOutput) == 0 {
		test.Errorf("Expected a log message when adding 2.")
	}

	_, err = addZero(-8, -4, logger)
	if err != nil {
		logger.Stackf(WARN, err, "Had an illegal argument to addZero. %d", -4)
	}
	logOutput, _ = ioutil.ReadAll(logBuffer)
	if len(logOutput) == 0 {
		test.Errorf("Expected a log message when adding -4.")
	}
}

func TestContext(t *testing.T) {
	ctxt := NewContext()
	ctxt.Add("foo", "bar")
	ctxt.Add("biz", "baz")

	if ctxt.Len() != 2 {
		t.Fatalf("Expected len of ctxt (%v) to be 2, but was %d", ctxt, ctxt.Len())
	}

	assertUnorderedStringSlicesEqual(t, ctxt.Keys(), []string{"foo", "biz"})

	ctxt.Remove("biz")
	if ctxt.Len() != 1 {
		t.Fatalf("Expected len of ctxt (%v) to be 1, but was %d", ctxt, ctxt.Len())
	}
	assertUnorderedStringSlicesEqual(t, ctxt.Keys(), []string{"foo"})
	val, found := ctxt.Get("foo")

	if !found {
		t.Fatalf("Expected \"foo\" to be present in ctxt. ctxt: %v", ctxt)
	}

	if val != "bar" {
		t.Fatalf("Expected ctxt.Get(\"foo\") == \"bar\" but was == \"%v\"", val)
	}

	_, found = ctxt.Get("biz")
	if found {
		t.Fatalf("Expected \"biz\" to not be in ctxt.  ctxt: %v", ctxt)
	}
}

func TestTruncation(t *testing.T) {
	const logFilename = "logger_test.output"
	logfile, err := os.Create(logFilename)
	if err != nil {
		t.Fatal("Cannot create `logger_test.output` file.")
	}
	defer os.Remove(logFilename)

	logger := &Logger{
		Prefix:    "dummy.Dummy",
		Appenders: []Appender{&FileAppender{logfile}},
	}

	check := func(message, expected string) {
		logger.Logf(WARN, message)
		fileOutputBytes, err := ioutil.ReadFile(logFilename)
		if err != nil {
			t.Fatal("Could not read entire file contents")
		}
		fileOut := string(fileOutputBytes)
		lines := strings.Split(fileOut, "\n")
		line := lines[len(lines)-2]
		if !strings.Contains(line, expected) {
			t.Fatalf("expected line to be %s but was %s", expected, line)
		}
	}
	// no truncation
	msg := "Please disregard the imminent warning. This is just a test. Please disregard the imminent warning. This is just a test. Please disregard the imminent warning. This is just a test. This is just a test. Please disregard the imminent warning. This is just a test. Please disregard the imminent warning. This is just a test. This is just a test. Please disregard the imminent warning. This is just a test. Please disregard the imminent warning. This is just a test."
	check(msg, msg)

	// set threshold below 100 (the minimum) - no truncation
	SetMaxLogSize(9)
	check(msg, msg)

	SetMaxLogSize(0)
	check(msg, msg)

	// set threshold to 110
	SetMaxLogSize(110)
	check(msg, "This is jus...mminent warning")

}

func assertLoggingOccurred(t *testing.T, logBuffer *bytes.Buffer, logit func()) {
	origOutput, _ := ioutil.ReadAll(logBuffer)
	origBufSize := len(origOutput)
	logit()
	newOutput, _ := ioutil.ReadAll(logBuffer)
	newBufSize := len(newOutput)

	if newBufSize <= origBufSize {
		t.Errorf("Logging should have occurred")
	}
}

// this modifies the arguments!
func assertUnorderedStringSlicesEqual(t *testing.T, slice1 []string, slice2 []string) {
	if len(slice1) != len(slice2) {
		t.Errorf("Expected slices to be equal! slice1: %v ; slice2: %v", slice1, slice2)
		return
	}

	sort.StringSlice(slice1).Sort()
	sort.StringSlice(slice2).Sort()

	for i, str := range slice1 {
		if str != slice2[i] {
			t.Errorf("Expected slices to be equal! slice1: %v ; slice2: %v", slice1, slice2)
			return
		}
	}
}

func denyLoggingOccurred(t *testing.T, logBuffer *bytes.Buffer, logit func()) {
	origOutput, _ := ioutil.ReadAll(logBuffer)
	origBufSize := len(origOutput)
	logit()
	newOutput, _ := ioutil.ReadAll(logBuffer)
	newBufSize := len(newOutput)

	if newBufSize != origBufSize {
		t.Errorf("Logging should not have occurred")
	}
}

func logHelloMongoDB(logger *Logger) {
	logger.logf(WARN, NoErrorCode, "Hello MongoDB", nil)
}

func logHelloWorld(logger *Logger) {
	logger.logf(WARN, NoErrorCode, "Hello World", nil)
}

func TestErrorWrapping(t *testing.T) {
	wrappedErr := ErrorWithCode{
		ErrCode: NoErrorCode,
		Err:     customError{},
	}

	cause := errors.Unwrap(wrappedErr)
	if cause == nil {
		t.Error("error cause should not be nil")
		return
	}
	if cause.Error() != "foo" {
		t.Errorf("cause should be 'foo' but was %s", cause.Error())
		return
	}
	if _, isRightType := cause.(customError); !isRightType {
		t.Error("error cause is not the right type")
	}
}

type customError struct{}

func (e customError) Error() string {
	return "foo"
}
