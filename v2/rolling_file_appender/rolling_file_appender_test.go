package rolling_file_appender

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"github.com/tolsen/slogger/v2"
)

const rfaTestLogDir = "log"
const rfaTestLogFilename = "logger_rfa_test.log"
const rfaTestLogPath = rfaTestLogDir + "/" + rfaTestLogFilename

func TestLog(test *testing.T) {
	defer teardown()
	appender, logger := setup(test, 1000, 10)

	logger.Logf(slogger.WARN, "This is a log message")
	appender.waitUntilEmpty()

	assertCurrentLogContains(test, "This is a log message")
}

func TestConcurrentLog(test *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	defer teardown()
	appender, logger := setup(test, 1024 * 1024 * 1024, 10)

	// Have 10 goroutines log 1000 lines each
	for i := 0; i < 10; i++ {
		prefix := fmt.Sprint("GO", i)
		go logSomeLines(logger, prefix, 1000)
	}

	appender.waitUntilEmpty()

	// Now check that each goroutine logged in order

	file, err := os.Open(rfaTestLogPath)
	if err != nil {
		test.Fatal("Failed to open log: " + err.Error())
	}

	reader := bufio.NewReader(file)

	tracker := make([]int, 10)
	for i := 0; i < 10; i++ {
		tracker[i] = -1
	}
	lineRegexp := regexp.MustCompile(`GO(\d+) (\d+)\n$`)
	eof := false
	for !eof {
		line, err := reader.ReadString('\n')
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

		if tracker[go_n] != seq - 1 {
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

func TestNoRotation(test *testing.T) {
	defer teardown()

	appender, logger := setup(test, 1000, 10)
	
	logger.Logf(slogger.WARN, "This is under 1,000 characters and should not cause a log rotation")
	appender.waitUntilEmpty()

	assertNumLogFiles(test, 1)
}

func TestNoRotation2(test *testing.T) {
	defer teardown()

	appender, logger := setup(test, -1, 10)
	
	logger.Logf(slogger.WARN, "This should not cause a log rotation")
	appender.waitUntilEmpty()

	assertNumLogFiles(test, 1)
}

func TestOldLogRemoval(test *testing.T) {
	defer teardown()

	appender, logger := setup(test, 10, 2)

	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()
	assertNumLogFiles(test, 2)

	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()
	assertNumLogFiles(test, 3)

	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()
	assertNumLogFiles(test, 3)
}

func TestRotation(test *testing.T) {
	defer teardown()

	appender, logger := setup(test, 10, 10)
	
	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()

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

func logSomeLines(logger *slogger.Logger, prefix string, numLines int) {
	for i := 0; i < numLines; i++ {
		logger.Logf(slogger.WARN, "%s %d", prefix, i)
	}
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
	
func setup(test *testing.T, maxFileSize int64, maxRotatedLogs int) (appender *RollingFileAppender, logger *slogger.Logger) {
	os.RemoveAll(rfaTestLogDir)
	err := os.Mkdir(rfaTestLogDir, 0755)

	if err != nil {
		test.Fatal("setup() failed to create directory: " + rfaTestLogDir)
	}
	
	appender, err = New(
		rfaTestLogPath,
		maxFileSize,
		maxRotatedLogs,
		func(err error) {
			msg := "Error during logging: " + err.Error()
			fmt.Fprintln(os.Stderr, msg + "\n(Test may deadlock)")
			test.Fatal(msg)
		},
		func() string {
			return "This is a header"
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

func teardown() {
	os.RemoveAll(rfaTestLogDir)
}	
