package rolling_file_appender

import (
	"fmt"
	"os"
	"testing"
	"time"
	".."
)

const rfaTestLogDir = "log"
const rfaTestLogFilename = "logger_rfa_test.log"

func TestRotation(test *testing.T) {
	defer teardown()

	appender, logger := setup(test, 10, 10)
	
	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()

	assertNumLogFiles(test, 2)
}

func TestNoRotation(test *testing.T) {
	defer teardown()

	appender, logger := setup(test, 1000, 10)
	
	logger.Logf(slogger.WARN, "This is under 1,000 characters and should not cause a log rotation")
	appender.waitUntilEmpty()

	assertNumLogFiles(test, 1)
}

func TestOldLogRemoval(test *testing.T) {
	defer teardown()

	appender, logger := setup(test, 10, 2)

	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()
	assertNumLogFiles(test, 2)

	time.Sleep(time.Second)
	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()
	assertNumLogFiles(test, 3)

	time.Sleep(time.Second)
	logger.Logf(slogger.WARN, "This is more than 10 characters and should cause a log rotation")
	appender.waitUntilEmpty()
	assertNumLogFiles(test, 3)
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
	
func setup(test *testing.T, maxFileSize uint64, maxRotatedLogs int) (appender *RollingFileAppender, logger *slogger.Logger) {
	os.RemoveAll(rfaTestLogDir)
	err := os.Mkdir(rfaTestLogDir, 0755)

	if err != nil {
		test.Fatal("setup() failed to create directory: " + rfaTestLogDir)
	}
	
	appender, err = New(
		(rfaTestLogDir + "/" + rfaTestLogFilename),
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
