package gotest

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	PASS = "PASS"
	FAIL = "FAIL"
	SKIP = "SKIP"

	// Match the start prefix and save the group of non-space characters
	// following the word "RUN"
	StartRegexString = `=== RUN (\S+)`

	// Match the end prefix, save PASS/FAIL/SKIP, save the decimal value
	// for number of seconds
	EndRegexString = `--- (PASS|SKIP|FAIL): (\S+) \(([0-9.]+[ ]*s)`
)

var startRegex = regexp.MustCompile(StartRegexString)
var endRegex = regexp.MustCompile(EndRegexString)

// Parser is an interface for parsing go test output, producing
// test logs and test results
type Parser interface {
	// Parse takes a reader for test output, and reads
	// until the reader is exhausted. Any parsing erros
	// are returned.
	Parse(io.Reader) error

	// Logs returns an array of strings, each entry a line in
	// the test output.
	Logs() []string

	// Results returns an array of test results. Parse() must be called
	// before this.
	Results() []TestResult
}

// This test result implementation maps more idiomatically to Go's test output
// than the TestResult type in the model package. Results are converted to the
// model type before being sent to the server.
type TestResult struct {
	// The name of the test
	Name string
	// The name of the test suite the test is a part of.
	// Currently, for this plugin, this is the name of the package
	// being tested, prefixed with a unique number to avoid
	// collisions when packages have the same name
	SuiteName string
	// The result status of the test
	Status string
	// How long the test took to run
	RunTime time.Duration
	// Number representing the starting log line number of the test
	// in the test's logged output
	StartLine int
	// Number representing the last line of the test in log output
	EndLine int

	// Can be set to mark the id of the server-side log that this
	// results corresponds to
	LogId string
}

// VanillaParser parses tests following regular go test output format.
// This should cover regular go tests as well as those written with the
// popular testing package "goconvey". The package"GoCheck" hides most
// test output, so it might be nice to add support for that at some
// point by building another parser.
type VanillaParser struct {
	Suite   string
	logs    []string
	results []TestResult
}

// Logs returns an array of logs captured during test execution.
func (self *VanillaParser) Logs() []string {
	return self.logs
}

// Results returns an array of test results parsed during test execution.
func (self *VanillaParser) Results() []TestResult {
	return self.results
}

// Parse reads in a test's output and stores the results and logs.
func (self *VanillaParser) Parse(testOutput io.Reader) error {
	curTest := TestResult{}
	testScanner := bufio.NewScanner(testOutput)

	// main parse loop
	for testScanner.Scan() {
		// handle errors first
		if err := testScanner.Err(); err != nil {
			return fmt.Errorf("error reading test output: %v", err)
		}

		// logs are appended at the start of the loop, allowing
		// len(self.logs) to represent the current line number [1...]
		logLine := testScanner.Text()
		self.logs = append(self.logs, logLine)

		// This is gross, and could all go away with the resolution of
		// https://code.google.com/p/go/issues/detail?id=2981
		switch {
		case startRegex.MatchString(logLine):
			newTestName, err := startInfoFromLogLine(logLine)
			if err != nil {
				return fmt.Errorf("error parsing start line '%v': %v", logLine, err)
			}
			// sanity check that we aren't already parsing a test
			if curTest.Name != "" {
				return fmt.Errorf("never read end line of test %v", curTest.Name)
			}
			curTest = TestResult{
				Name:      newTestName,
				SuiteName: self.Suite,
				StartLine: len(self.logs),
			}
		case endRegex.MatchString(logLine):
			name, status, duration, err := endInfoFromLogLine(logLine)
			if err != nil {
				return fmt.Errorf("error parsing end line '%v': %v", logLine, err)
			}
			// sanity check on test name
			if name != curTest.Name {
				return fmt.Errorf(
					"name of test end line (%v) does not match start line (%v)",
					name,
					curTest.Name,
				)
			}
			curTest.Status = status
			curTest.RunTime = duration
			curTest.EndLine = len(self.logs)
			self.results = append(self.results, curTest)
			curTest = TestResult{}
		}
	}

	return nil
}

// startInfoFromLogLine gets the test name from a log line
// indicating the start of a test. Returns test name
// and an error if one occurs.
func startInfoFromLogLine(line string) (string, error) {
	matches := startRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		// futureproofing -- this can't happen as long as we
		// check Match() before calling startInfoFromLogLine
		return "", fmt.Errorf(
			"unable to match start line regular expression on line: %v", line)
	}
	return matches[1], nil
}

// endInfoFromLogLine gets the test name, result status, and Duration
// from a log line. Returns those matched elements, as well as any error
// in regex or duration parsing.
func endInfoFromLogLine(line string) (string, string, time.Duration, error) {
	matches := endRegex.FindStringSubmatch(line)
	if len(matches) < 4 {
		// this block should never be reached if we call endRegex.Match()
		// before entering this function
		return "", "", 0, fmt.Errorf(
			"unable to match end line regular expression on line: %v", line)
	}
	status := matches[1]
	name := matches[2]
	duration, err := time.ParseDuration(strings.Replace(matches[3], " ", "", -1))
	if err != nil {
		return "", "", 0, fmt.Errorf("error parsing test runtime: %v", err)
	}
	return name, status, duration, nil
}
