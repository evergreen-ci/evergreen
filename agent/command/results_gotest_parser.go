package command

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/pkg/errors"
)

const (
	PASS = "PASS"
	FAIL = "FAIL"
	SKIP = "SKIP"
)

var (
	// Match the start prefix and save the group of non-space characters following the word "RUN"
	startRegex = regexp.MustCompile(`=== RUN\s+(\S+)`)

	// Match the end prefix, save PASS/FAIL/SKIP, save the decimal value for number of seconds
	endRegex = regexp.MustCompile(`--- (PASS|SKIP|FAIL): (\S+) \(-*([0-9\.m]+[ ]*s)`)

	// Match the start prefix and save the group of non-space characters following the word "RUN"
	gocheckStartRegex = regexp.MustCompile(`START: .*.go:[0-9]+: (\S+)`)

	// Match the end prefix, save PASS/FAIL/SKIP, save the decimal value for number of seconds
	gocheckEndRegex = regexp.MustCompile(`(PASS|SKIP|FAIL): .*.go:[0-9]+: (\S+)\s*([0-9\.m]+[ ]*s)?`)

	// Match the failing status prefix for go build.
	goTestFailedStatusRegex = regexp.MustCompile(`FAIL\s+(\S+)\s+\[build failed\]`)
)

// This test result implementation maps more idiomatically to Go's test output
// than the TestResult type in the model package. Results are converted to the
// model type before being sent to the server.
type goTestResult struct {
	// The name of the test
	Name string
	// The result status of the test
	Status string
	// How long the test took to run
	RunTime time.Duration
	// Number representing the starting log line number of the test
	// in the test's logged output
	StartLine int
	// Number representing the last line of the test in log output
	EndLine int
}

// ToModelTestResults converts the implementation of LocalTestResults native
// to the goTest plugin to the implementation used by MCI tasks.
func ToModelTestResults(results []*goTestResult, suiteName string) []testresult.TestResult {
	var modelResults []testresult.TestResult
	for _, res := range results {
		// Start and end are times that we don't know, we must
		// calculate them here based on the result runtime.
		start := time.Now()
		end := start.Add(res.RunTime)

		var status string
		switch res.Status {
		// As long as we use a regex, it should be impossible to get an
		// incorrect status code.
		case PASS:
			status = evergreen.TestSucceededStatus
		case SKIP:
			status = evergreen.TestSkippedStatus
		case FAIL:
			status = evergreen.TestFailedStatus
		}
		convertedResult := testresult.TestResult{
			TestName:      res.Name,
			Status:        status,
			TestStartTime: start,
			TestEndTime:   end,
			LogInfo: &testresult.TestLogInfo{
				LogName: suiteName,
				LineNum: int32(res.StartLine - 1),
			},
		}
		modelResults = append(modelResults, convertedResult)
	}

	return modelResults
}

// goTestParser parses tests following go test output format.
// This should cover regular go tests as well as those written with the
// popular testing packages goconvey and gocheck.
type goTestParser struct {
	logs []string
	// map for storing tests during parsing. this is an array to handle multiple
	// executions of the same test in the same log
	tests map[string][]*goTestResult
	order []*goTestResult
}

// Logs returns an array of logs captured during test execution.
func (vp *goTestParser) Logs() []string {
	return vp.logs
}

// Results returns an array of test results parsed during test execution.
func (vp *goTestParser) Results() []*goTestResult {
	return vp.order
}

// Parse reads in a test's output and stores the results and logs.
func (vp *goTestParser) Parse(testOutput io.Reader) error {
	testScanner := bufio.NewScanner(testOutput)
	vp.tests = map[string][]*goTestResult{}
	for testScanner.Scan() {
		// logs are appended at the start of the loop, allowing
		// len(vp.logs) to represent the current line number [1...]
		logLine := testScanner.Text()
		vp.logs = append(vp.logs, logLine)
		if err := vp.handleLine(logLine); err != nil {
			return errors.WithStack(err)
		}
	}
	return errors.Wrap(testScanner.Err(), "reading test output")
}

// handleLine attempts to parse and store any test updates from the given line.
func (vp *goTestParser) handleLine(line string) error {
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
	case goTestFailedStatusRegex.MatchString(line):
		return vp.handleFailedBuild(line)
	}
	return nil
}

// handleEnd gets the end data from an ending line and stores it.
func (vp *goTestParser) handleEnd(line string, rgx *regexp.Regexp) error {
	name, status, duration, err := endInfoFromLogLine(line, rgx)
	if err != nil {
		return errors.Wrapf(err, "parsing end line '%s'", line)
	}
	tAry, ok := vp.tests[name]
	if !ok || tAry == nil {
		// if there's no existing test, just stub one out
		t := vp.newTestResult(name)
		tAry = []*goTestResult{t}
		vp.order = append(vp.order, t)
	}
	tAry[len(tAry)-1].Status = status
	tAry[len(tAry)-1].RunTime = duration
	tAry[len(tAry)-1].EndLine = len(vp.logs)
	vp.tests[name] = tAry

	return nil
}

// handleStart gets the data from a start line and stores it.
func (vp *goTestParser) handleStart(line string, rgx *regexp.Regexp, defaultFail bool) error {
	name, err := startInfoFromLogLine(line, rgx)
	if err != nil {
		return errors.Wrapf(err, "parsing start line '%s'", line)
	}
	t := vp.newTestResult(name)

	// tasks should start out failed unless they're marked
	// passing/skipped, although gocheck can't support this
	if defaultFail {
		t.Status = FAIL
	} else {
		t.Status = PASS
	}

	tAry, ok := vp.tests[name]
	if !ok || tAry == nil {
		tAry = []*goTestResult{t}
	} else {
		tAry = append(tAry, t)
	}
	vp.tests[name] = tAry
	vp.order = append(vp.order, t)

	return nil
}

func (vp *goTestParser) handleFailedBuild(line string) error {
	path, err := pathNameFromLogLine(line)
	if err != nil {
		return errors.Wrapf(err, "parsing build failure line '%s'", line)
	}
	t := vp.newTestResult("[build failed] " + path)
	t.Status = FAIL
	t.EndLine = len(vp.logs)
	vp.order = append(vp.order, t)
	return nil
}

// newTestResult populates a test result type with the given
// test name and current line number.
func (vp *goTestParser) newTestResult(name string) *goTestResult {
	return &goTestResult{
		Name:      name,
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
		return "", errors.Errorf("unable to match start line regular expression on line '%s'", line)
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
		return "", "", 0, errors.Errorf("unable to match end line regular expression on line '%s'", line)
	}
	status := matches[1]
	name := matches[2]
	var duration time.Duration
	if matches[3] != "" {
		var err error
		duration, err = time.ParseDuration(strings.Replace(matches[3], " ", "", -1))
		if err != nil {
			return "", "", 0, errors.Wrap(err, "parsing test runtime duration")
		}
	}
	return name, status, duration, nil
}

// pathNameFromLogLine returns the test name from a go test log line.
func pathNameFromLogLine(line string) (string, error) {
	matches := goTestFailedStatusRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return "", errors.Errorf("unable to match build line to regular expression on line '%s'", line)
	}
	return matches[1], nil
}
