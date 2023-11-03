package command

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type customFloat float64

func (cf *customFloat) UnmarshalXMLAttr(attr xml.Attr) error {
	s := strings.Replace(attr.Value, ",", "", -1)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*cf = customFloat(f)
	return nil
}

type testSuites struct {
	Suites   []testSuite `xml:"testsuite"`
	Errors   int         `xml:"errors,attr"`
	Failures int         `xml:"failures,attr"`
	Skip     int         `xml:"skip,attr"`
	Name     string      `xml:"name,attr"`
	Time     customFloat `xml:"time,attr"`
	Tests    int         `xml:"tests,attr"`
}

type testSuite struct {
	Errors       int             `xml:"errors,attr"`
	Failures     int             `xml:"failures,attr"`
	Skip         int             `xml:"skip,attr"`
	Name         string          `xml:"name,attr"`
	Tests        int             `xml:"tests,attr"`
	TestCases    []testCase      `xml:"testcase"`
	Time         customFloat     `xml:"time,attr"`
	Error        *failureDetails `xml:"error"`
	SysOut       string          `xml:"system-out"`
	SysErr       string          `xml:"system-err"`
	NestedSuites *testSuite      `xml:"testsuite"`
}

type testCase struct {
	Name      string          `xml:"name,attr"`
	Time      customFloat     `xml:"time,attr"`
	ClassName string          `xml:"classname,attr"`
	Failure   *failureDetails `xml:"failure"`
	Error     *failureDetails `xml:"error"`
	SysOut    string          `xml:"system-out"`
	SysErr    string          `xml:"system-err"`
	Skipped   *failureDetails `xml:"skipped"`
}

type failureDetails struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

func parseXMLResults(reader io.Reader) ([]testSuite, error) {
	results := testSuites{}
	fileData, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, "reading results file")
	}
	// need to try to unmarshal into 2 different structs since the JUnit XML schema
	// allows for <testsuite> or <testsuites> to be the root
	// https://github.com/windyroad/JUnit-Schema/blob/master/JUnit.xsd
	if err = xml.Unmarshal(fileData, &results); err != nil {
		return nil, errors.Wrap(err, "unmarshalling XML test suite")
	}
	if len(results.Suites) == 0 {
		if err = xml.Unmarshal(fileData, &results.Suites); err != nil {
			return nil, errors.Wrap(err, "unmarshalling XML test suites")
		}
	}
	return results.Suites, nil
}

// toModelTestResultAndLog converts an XUnit test case into a test result and
// test log. Logs are only generated if the test case did not succeed (this is
// part of the XUnit XML file design).
func (tc testCase) toModelTestResultAndLog(conf *internal.TaskConfig, logger client.LoggerProducer) (testresult.TestResult, *testlog.TestLog) {

	res := testresult.TestResult{}
	var log *testlog.TestLog

	if tc.ClassName != "" {
		res.TestName = fmt.Sprintf("%v.%v", tc.ClassName, tc.Name)
	} else {
		res.TestName = tc.Name
	}
	// Replace spaces, dashes, etc. with underscores.
	res.TestName = util.CleanForPath(res.TestName)

	if math.IsNaN(float64(tc.Time)) {
		logger.Task().Errorf("Test '%s' time was NaN, its calculated duration will be incorrect", res.TestName)
		tc.Time = 0
	}
	// Passing 0 as the sign will check for Inf as well as -Inf.
	if math.IsInf(float64(tc.Time), 0) {
		logger.Task().Errorf("Test '%s' time was Inf, its calculated duration will be incorrect", res.TestName)
		tc.Time = 0
	}

	res.TestStartTime = time.Now()
	res.TestEndTime = res.TestStartTime.Add(time.Duration(float64(tc.Time) * float64(time.Second)))

	// The presence of the Failure, Error, or Skipped fields is used to
	// indicate an unsuccessful test case. Logs can only be generated in
	// in failure cases, because XUnit results only include messages if
	// they did *not* succeed.
	switch {
	case tc.Failure != nil:
		res.Status = evergreen.TestFailedStatus
		log = tc.Failure.toBasicTestLog("FAILURE")
	case tc.Error != nil:
		res.Status = evergreen.TestFailedStatus
		log = tc.Error.toBasicTestLog("ERROR")
	case tc.Skipped != nil:
		res.Status = evergreen.TestSkippedStatus
	default:
		res.Status = evergreen.TestSucceededStatus
	}

	if systemLogs := constructSystemLogs(tc.SysOut, tc.SysErr); len(systemLogs) > 0 {
		if log == nil {
			log = &testlog.TestLog{}
		}
		log.Lines = append(log.Lines, systemLogs...)
	}

	if log != nil {
		// When sending test logs to Cedar we need to use a
		// unique string since there may be duplicate file
		// names if there are duplicate test names.
		log.Name = utility.RandomString()
		log.Task = conf.Task.Id
		log.TaskExecution = conf.Task.Execution
		res.LogTestName = log.Name
	}

	return res, log
}

func (fd failureDetails) toBasicTestLog(fdType string) *testlog.TestLog {
	log := testlog.TestLog{
		Lines: []string{fmt.Sprintf("%v: %v (%v)", fdType, fd.Message, fd.Type)},
	}
	logLines := strings.Split(strings.TrimSpace(fd.Content), "\n")
	log.Lines = append(log.Lines, logLines...)
	return &log
}
