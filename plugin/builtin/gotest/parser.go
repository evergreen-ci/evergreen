package gotest

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	PASS = "PASS"
	FAIL = "FAIL"
	SKIP = "SKIP"

	// Match the start prefix and save the group of non-space characters following the word "RUN"
	StartRegexString = `=== RUN\s+(\S+)`

	// Match the end prefix, save PASS/FAIL/SKIP, save the decimal value for number of seconds
	EndRegexString = `--- (PASS|SKIP|FAIL): (\S+) \(([0-9.]+[ ]*s)`

	// Match the start prefix and save the group of non-space characters following the word "RUN"
	GocheckStartRegexString = `START: .*.go:[0-9]+: (\S+)`

	// Match the end prefix, save PASS/FAIL/SKIP, save the decimal value for number of seconds
	GocheckEndRegexString = `(PASS|SKIP|FAIL): .*.go:[0-9]+: (\S+)\s*([0-9.]+[ ]*s)?`
)

var startRegex = regexp.MustCompile(StartRegexString)
var endRegex = regexp.MustCompile(EndRegexString)
var gocheckStartRegex = regexp.MustCompile(GocheckStartRegexString)
var gocheckEndRegex = regexp.MustCompile(GocheckEndRegexString)

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
	Results() []*TestResult
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

// VanillaParser parses tests following go test output format.
// This should cover regular go tests as well as those written with the
// popular testing packages goconvey and gocheck.
type VanillaParser struct {
	Suite string
	logs  []string
	// map for storing tests during parsing
	tests map[string]*TestResult
	order []string
}

// Logs returns an array of logs captured during test execution.
func (vp *VanillaParser) Logs() []string {
	return vp.logs
}

// Results returns an array of test results parsed during test execution.
func (vp *VanillaParser) Results() []*TestResult {
	out := []*TestResult{}

	for _, name := range vp.order {
		out = append(out, vp.tests[name])
	}

	return out
}

// Parse reads in a test's output and stores the results and logs.
func (vp *VanillaParser) Parse(testOutput io.Reader) error {
	testScanner := bufio.NewScanner(testOutput)
	vp.tests = map[string]*TestResult{}
	for testScanner.Scan() {
		if err := testScanner.Err(); err != nil {
			return errors.Wrap(err, "error reading test output")
		}
		// logs are appended at the start of the loop, allowing
		// len(vp.logs) to represent the current line number [1...]
		logLine := testScanner.Text()
		vp.logs = append(vp.logs, logLine)
		if err := vp.handleLine(logLine); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// handleLine attempts to parse and store any test updates from the given line.
func (vp *VanillaParser) handleLine(line string) error {
	// This is gross, and could all go away with the resolution of
	// https://code.google.com/p/go/issues/detail?id=2981
	switch {
	case startRegex.MatchString(line):
		return vp.handleStart(line, startRegex, true)
	case gocheckStartRegex.MatchString(line):
		return vp.handleStart(line, gocheckStartRegex, false)
	case endRegex.MatchString(line):
		return vp.handleEnd(line, endRegex)
	case gocheckEndRegex.MatchString(line):
		return vp.handleEnd(line, gocheckEndRegex)
	}
	return nil
}

// handleEnd gets the end data from an ending line and stores it.
func (vp *VanillaParser) handleEnd(line string, rgx *regexp.Regexp) error {
	name, status, duration, err := endInfoFromLogLine(line, rgx)
	if err != nil {
		return errors.Wrapf(err, "error parsing end line '%s'", line)
	}
	t := vp.tests[name]
	if t == nil || t.Name == "" {
		// if there's no existing test, just stub one out
		t = vp.newTestResult(name)
		vp.order = append(vp.order, name)
		vp.tests[name] = t
	}
	t.Status = status
	t.RunTime = duration
	t.EndLine = len(vp.logs)

	return nil
}

// handleStart gets the data from a start line and stores it.
func (vp *VanillaParser) handleStart(line string, rgx *regexp.Regexp, defaultFail bool) error {
	name, err := startInfoFromLogLine(line, rgx)
	if err != nil {
		return errors.Wrapf(err, "error parsing start line '%s'", line)
	}
	t := vp.newTestResult(name)

	// tasks should start out failed unless they're marked
	// passing/skipped, although gocheck can't support this
	if defaultFail {
		t.Status = FAIL
	} else {
		t.Status = PASS
	}

	vp.tests[name] = t
	vp.order = append(vp.order, name)

	return nil
}

// newTestResult populates a test result type with the given
// test name, current suite, and current line number.
func (vp *VanillaParser) newTestResult(name string) *TestResult {
	return &TestResult{
		Name:      name,
		SuiteName: vp.Suite,
		StartLine: len(vp.logs),
	}
}

// startInfoFromLogLine gets the test name from a log line
// indicating the start of a test. Returns test name
// and an error if one occurs.
func startInfoFromLogLine(line string, rgx *regexp.Regexp) (string, error) {
	matches := rgx.FindStringSubmatch(line)
	if len(matches) < 2 {
		// futureproofing -- this can't happen as long as we
		// check Match() before calling startInfoFromLogLine
		return "", errors.Errorf(
			"unable to match start line regular expression on line: %s", line)
	}
	return matches[1], nil
}

// endInfoFromLogLine gets the test name, result status, and Duration
// from a log line. Returns those matched elements, as well as any error
// in regex or duration parsing.
func endInfoFromLogLine(line string, rgx *regexp.Regexp) (string, string, time.Duration, error) {
	matches := rgx.FindStringSubmatch(line)
	if len(matches) < 4 {
		// this block should never be reached if we call endRegex.Match()
		// before entering this function
		return "", "", 0, errors.Errorf(
			"unable to match end line regular expression on line: %s", line)
	}
	status := matches[1]
	name := matches[2]
	var duration time.Duration
	if matches[3] != "" {
		var err error
		duration, err = time.ParseDuration(strings.Replace(matches[3], " ", "", -1))
		if err != nil {
			return "", "", 0, errors.Wrap(err, "error parsing test runtime")
		}
	}
	return name, status, duration, nil
}
