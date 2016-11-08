package slogger

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
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
		Line:       88,
		messageFmt: "Tail started on RsId: `backup_test`",
	}

	expected := "[0001/01/01 00:00:00] [agent.OplogTail.info] [oplog.go:88] Tail started on RsId: `backup_test`\n"
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
		test.Fatal("Incorrect filename. Expected: `%v` Full log: `%v`", logFilename, fileOutput)
	}

	if strings.Contains(fileOutput, logMessage) == false {
		test.Fatal("Incorrect message. Expected: `%v` Full log: `%v`", logMessage, fileOutput)
	}
}

func TestCopy(test *testing.T) {
	CapLogCache(10)

	logger := &Logger{
		Prefix:    "agent.OplogTail",
		Appenders: []Appender{},
	}

	logger.Logf(INFO, "0")
	logger.Logf(INFO, "1")
	logger.Logf(DEBUG, "2")
	logger.Logf(DEBUG, "3")
	logger.Logf(WARN, "4")
	logger.Logf(DEBUG, "5")
	logger.Logf(INFO, "6")

	for idx, log := range Cache.Copy() {
		expected := fmt.Sprintf("%d", idx)
		if expected != log.Message() {
			test.Errorf("Mismatch message. Idx: %d expected: `%v`", idx, expected)
		}

		//fmt.Printf("#%d: %v", idx, FormatLog(log))
	}

	CapLogCache(5)
	logger.Logf(INFO, "0")
	logger.Logf(INFO, "1")
	logger.Logf(DEBUG, "2")
	logger.Logf(DEBUG, "3")
	logger.Logf(WARN, "4")
	logger.Logf(DEBUG, "5")
	logger.Logf(INFO, "6")

	for idx, log := range Cache.Copy() {
		expected := fmt.Sprintf("%d", idx+2)
		if expected != log.Message() {
			test.Errorf("Mismatch message. Idx: %d expected: `%v`", idx, expected)
		}

		//fmt.Printf("#%d: %v", idx, FormatLog(log))
	}
}

type countingAppender struct {
	count int
}

func (self *countingAppender) Append(log *Log) error {
	self.count++
	return nil
}

func TestFilter(test *testing.T) {
	CapLogCache(10)

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

	cache := Cache.Copy()
	if len(cache) != 4 {
		test.Errorf("Expected all logs to be cached. Received: %d", len(cache))
	}
}

func TestStacktrace(test *testing.T) {
	// slogger/logger_test.go:129
	// testing/testing.go:346
	// runtime/proc.c:1214

	stacktrace := NewStackError("").Stacktrace
	if match, _ := regexp.MatchString("^at slogger/v1/logger_test.go:\\d+", stacktrace[0]); match == false {
		test.Errorf("Stacktrace level 0 did not match. Received: %v", stacktrace[0])
	}

	if match, _ := regexp.MatchString("^at pkg/testing/testing.go:\\d+", stacktrace[1]); match == false {
		test.Errorf("Stacktrace level 1 did not match. Received: %v", stacktrace[1])
	}

	if match, _ := regexp.MatchString("^at pkg/runtime/proc.c:\\d+", stacktrace[2]); match == false {
		test.Errorf("Stacktrace level 2 did not match. Received: %v", stacktrace[2])
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

	if match, _ := regexp.MatchString("slogger/v1/logger_test.go:\\d+", str); match == false {
		test.Errorf("Expected to see output for `v1/logger_test.go`. Received:\n%v", str)
	}

	match, err := regexp.MatchString("slogger/v1/logger.go:\\d+", str)
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
