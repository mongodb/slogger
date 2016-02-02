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

package async_appender

import (
	"bytes"
	"fmt"
	"github.com/tolsen/slogger/v2/slogger"
	. "github.com/tolsen/slogger/v2/slogger/test_util"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestLog(test *testing.T) {
	appender, logger := setup(test)

	_, errs := logger.Logf(slogger.WARN, "This is a log message")
	AssertNoErrors(test, errs)
	AssertNoErrors(test, logger.Flush())

	assertCurrentLogContains(test, "This is a log message", appender)
}

func TestConcurrentLog(test *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	appender, logger := setup(test)

	wg := &sync.WaitGroup{}
	// Have 10 goroutines log 1000 lines each
	for i := 0; i < 10; i++ {
		prefix := fmt.Sprint("GO", i)
		wg.Add(1)
		go logSomeLines(test, wg, logger, prefix, 1000)
	}

	wg.Wait()

	AssertNoErrors(test, logger.Flush())

	// Now check that each goroutine logged in order

	stringAppender, ok := appender.Appender.(*slogger.StringAppender)
	if !ok {
		test.Fatal("sub Appender should be a *StringAppender: %v", appender.Appender)
	}

	tracker := make([]int, 10)
	for i := 0; i < 10; i++ {
		tracker[i] = -1
	}
	lineRegexp := regexp.MustCompile(`GO(\d+) (\d+)\n$`)
	eof := false
	for !eof {
		line, err := stringAppender.ReadString('\n')
		if err == io.EOF {
			eof = true
		} else if err != nil {
			test.Fatal("Failure while reading log: " + err.Error())
		}

		matches := lineRegexp.FindAllStringSubmatch(line, 1)

		if matches == nil {
			continue
		}

		match := matches[0]
		go_n, err := strconv.Atoi(match[1])
		if err != nil {
			test.Fatalf("Failure to parse %s as int: %s", match[1], err.Error())
		}
		seq, err := strconv.Atoi(match[2])
		if err != nil {
			test.Fatalf("Failure to parse %s as int: %s", match[2], err.Error())
		}

		if tracker[go_n] != seq-1 {
			test.Fatalf(
				"Logged out of order?  Received seq %d for go %d when last seq was %d",
				seq,
				go_n,
				tracker[go_n],
			)
		}

		tracker[go_n] = seq
	}

	for i := 0; i < 10; i++ {
		if tracker[i] != 999 {
			test.Fatalf(
				"Last received seq for go %d was %d",
				i,
				tracker[1],
			)
		}
	}
}

func assertCurrentLogContains(test *testing.T, expected string, appender *AsyncAppender) {
	stringAppender, ok := appender.Appender.(*slogger.StringAppender)
	if !ok {
		test.Fatal("sub Appender should be a *StringAppender: %v", appender.Appender)
	}
	actual := stringAppender.String()

	if !strings.Contains(actual, expected) {
		test.Errorf("Log contains: \n%s\ninstead of\n%s", actual, expected)
	}
}

func logSomeLines(test *testing.T, waitGroup *sync.WaitGroup, logger *slogger.Logger, prefix string, numLines int) {
	for i := 0; i < numLines; i++ {
		_, errs := logger.Logf(slogger.WARN, "%s %d", prefix, i)
		if len(errs) != 0 {
			fmt.Printf("ERRORS WHILE LOGGING: %v\n", errs)
		}
		AssertNoErrors(test, errs)
	}
	waitGroup.Done()
}

func newAppenderAndLogger(test *testing.T) (appender *AsyncAppender, logger *slogger.Logger) {
	appender = New(
		slogger.NewStringAppender(new(bytes.Buffer)),
		4096,
		func(err error) {
			msg := "Error during logging: " + err.Error()
			fmt.Fprintln(os.Stderr, msg+"\n(Test may deadlock)")
			test.Fatal(msg)
		},
	)

	logger = &slogger.Logger{
		Prefix:    "rfa",
		Appenders: []slogger.Appender{appender},
	}

	return
}

func setup(test *testing.T) (appender *AsyncAppender, logger *slogger.Logger) {
	return newAppenderAndLogger(test)
}
