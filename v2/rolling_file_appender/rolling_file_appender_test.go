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
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"github.com/tolsen/slogger/v2"
	. "github.com/tolsen/slogger/v2/test_util"
)

const rfaTestLogDir = "log"
const rfaTestLogFilename = "logger_rfa_test.log"
const rfaTestLogPath = rfaTestLogDir + "/" + rfaTestLogFilename

func TestLog(test *testing.T) {
	defer teardown()
	_, logger := setup(test, 1000, 10, false)

	_, errs := logger.Logf(slogger.WARN, "This is a log message")
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())

	assertCurrentLogContains(test, "This is a log message")
}

func TestNoRotation(test *testing.T) {
	defer teardown()

	_, logger := setup(test, 1000, 10, false)

	_, errs := logger.Logf(slogger.WARN, "This is under 1,000 characters and should not cause a log rotation")
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())

	assertNumLogFiles(test, 1)
}

func TestNoRotation2(test *testing.T) {
	defer teardown()

	_, logger := setup(test, -1, 10, false)

	_, errs := logger.Logf(slogger.WARN, "This should not cause a log rotation")
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())

	assertNumLogFiles(test, 1)
}

func TestOldLogRemoval(test *testing.T) {
	defer teardown()

	_, logger := setup(test, 10, 2, false)

	_, errs := logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())
	assertNumLogFiles(test, 2)

	_, errs = logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")	
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())
	assertNumLogFiles(test, 3)

	_, errs = logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())
	assertNumLogFiles(test, 3)
}

func TestPreRotation(test *testing.T) {
	createLogDir(test)
	
	file, err := os.Create(rfaTestLogPath)
	if err != nil {
		test.Fatalf("Failed to create empty logfile: %v", err)
	}

	err = file.Close()
	if err != nil {
		test.Fatalf("Failed to close logfile: %v", err)
	}

	_, logger := newAppenderAndLogger(test, 1000, 2, true)
	AssertNoErrors(test, logger.Flush())
	assertNumLogFiles(test, 2)
}

func TestRotation(test *testing.T) {
	defer teardown()

	_, logger := setup(test, 10, 10, false)

	_, errs := logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())

	assertNumLogFiles(test, 2)
}

func assertCurrentLogContains(test *testing.T, expected string) {
	actual := readCurrentLog(test)

	if !strings.Contains(actual, expected) {
		test.Errorf("Log contains: \n%s\ninstead of\n%s", actual, expected)
	}
}

func assertNumLogFiles(test *testing.T, expected_n int) {
	actual_n, err := numLogFiles()
	if err != nil {
		test.Fatal("Could not get numLogFiles")
	}

	if expected_n != actual_n {
		test.Errorf(
			"Expected number of log files to be %d, not %d",
			expected_n,
			actual_n,
		)
	}
}

func createLogDir(test *testing.T) {
	os.RemoveAll(rfaTestLogDir)
	err := os.Mkdir(rfaTestLogDir, 0777)

	if err != nil {
		test.Fatal("setup() failed to create directory: " + rfaTestLogDir)
	}
}

func newAppenderAndLogger(test *testing.T, maxFileSize int64, maxRotatedLogs int, rotateIfExists bool) (appender *RollingFileAppender, logger *slogger.Logger) {
	appender, err := New(
		rfaTestLogPath,
		maxFileSize,
		maxRotatedLogs,
		rotateIfExists,
		func() []string {
			return []string{"This is a header", "more header"}
		},
	)

	if err != nil {
		test.Fatal("NewRollingFileAppender() failed: " + err.Error())
	}
	
	logger = &slogger.Logger{
		Prefix: "rfa",
		Appenders: []slogger.Appender{appender},
	}
	
	return
}
	

func numLogFiles() (int, error) {
	cwd, err := os.Open(rfaTestLogDir)
	if err != nil {
		return -1, err
	}
	defer cwd.Close()

	var filenames []string
	filenames, err = cwd.Readdirnames(-1)
	if err != nil {
		return -1, err
	}

	return len(filenames), nil
}

func readCurrentLog(test *testing.T) string {
	bytes, err := ioutil.ReadFile(rfaTestLogPath)
	if err != nil {
		test.Fatal("Could not read log file")
	}

	return string(bytes)
}

func setup(test *testing.T, maxFileSize int64, maxRotatedLogs int, rotateIfExists bool) (appender *RollingFileAppender, logger *slogger.Logger) {
	createLogDir(test)
	
	return newAppenderAndLogger(test, maxFileSize, maxRotatedLogs, rotateIfExists)
}

func teardown() {
	os.RemoveAll(rfaTestLogDir)
}	
