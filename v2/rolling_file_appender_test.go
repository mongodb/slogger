package slogger

import (
//	"fmt"
	"os"
	"strings"
	"testing"
)

const rfaTestLogFilename = "logger_rfa_test.log"

func TestRollingFileAppenderLog(test *testing.T) {
	
	logfile, err := os.Create(rfaTestLogFilename)
	if err != nil {
		test.Fatal("Cannot create `logger_rfa_test.output` file.")
	}
	defer os.Remove(rfaTestLogFilename)

	appender := NewRollingFileAppender(
		logfile,
		100,
		func(err error) {
			test.Fatal("Error during logging: " + err.Error())
		},
		func() string {
			return "This is a header"
		},
	)
	
	logger := &Logger{
		Prefix: "rfa",
		Appenders: []Appender{appender},
	}


	beforeRotatedLogFilenames, err := rotatedLogFilenames()
	if err != nil {
		test.Fatal("Could not get rotatedLogFilenames: " + err.Error())
	}
	
	logger.Logf(WARN, "This is more than 10 characters and should cause a log rotation")
	appender.Sync()

	afterRotatedLogFilenames, err := rotatedLogFilenames()
	if err != nil {
		test.Fatal("Could not get rotatedLogFilenames: " + err.Error())
	}

	if len(afterRotatedLogFilenames) != len(beforeRotatedLogFilenames) + 1 {
		test.Errorf(
			"Number of rotate logs did not increase by 1.  Before: %d.  After: %d",
			len(beforeRotatedLogFilenames), len(afterRotatedLogFilenames))
	}

	newLogFilename := getNewLogFilename(beforeRotatedLogFilenames, afterRotatedLogFilenames)
	defer os.Remove(newLogFilename)
}

func getNewLogFilename(beforeRotatedLogFilenames []string, afterRotatedLogFilenames []string) string {
	var newLogFilename string

	for _, newLogFilename = range afterRotatedLogFilenames {
		found := false
		for _, beforeLogfilename := range beforeRotatedLogFilenames {
			if beforeLogfilename == newLogFilename {
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return newLogFilename;
}

func rotatedLogFilenames() ([]string, error) {
	const rotatedLogPrefix = rfaTestLogFilename + "."
	cwd, err := os.Open(".")
	if err != nil {
		return nil, err
	}
	defer cwd.Close()

	var filenames []string
	filenames, err = cwd.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	logFilenames := make([]string, 0, len(filenames))
	for _, filename := range filenames {
		if strings.HasPrefix(filename, rotatedLogPrefix) {
			logFilenames = append(logFilenames, filename)
		}
	}

	return logFilenames, nil
}
	
