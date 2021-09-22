package command

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
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
	Skipped   *failureDetails `xml:"skipped"`
}

type failureDetails struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

func parseXMLResults(reader io.Reader) ([]testSuite, error) {
	results := testSuites{}
	fileData, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read results file")
	}
	// need to try to unmarshal into 2 different structs since the JUnit XML schema
	// allows for <testsuite> or <testsuites> to be the root
	// https://github.com/windyroad/JUnit-Schema/blob/master/JUnit.xsd
	if err = xml.Unmarshal(fileData, &results); err != nil {
		return nil, err
	}
	if len(results.Suites) == 0 {
		if err = xml.Unmarshal(fileData, &results.Suites); err != nil {
			return nil, err
		}
	}
	return results.Suites, nil
}

// ToModelTestResultAndLog converts an xunit test case into an
// mci task.TestResult and model.TestLog. Logs are only
// generated if the test case did not succeed (this is part of
// the xunit xml file design)
func (tc testCase) toModelTestResultAndLog(conf *internal.TaskConfig) (task.TestResult, *model.TestLog) {

	res := task.TestResult{}
	var log *model.TestLog

	if tc.ClassName != "" {
		res.TestFile = fmt.Sprintf("%v.%v", tc.ClassName, tc.Name)
	} else {
		res.TestFile = tc.Name
	}
	// replace spaces, dashes, etc. with underscores
	res.TestFile = util.CleanForPath(res.TestFile)

	res.StartTime = float64(time.Now().Unix())
	res.EndTime = res.StartTime + float64(tc.Time)

	// the presence of the Failure, Error, or Skipped fields
	// is used to indicate an unsuccessful test case. Logs
	// can only be generated in failure cases, because xunit
	// results only include messages if they did *not* succeed.
	switch {
	case tc.Failure != nil:
		res.Status = evergreen.TestFailedStatus
		log = tc.Failure.toBasicTestLog("FAILURE")
	case tc.Error != nil:
		res.Status = evergreen.TestFailedStatus
		log = tc.Error.toBasicTestLog("ERROR")
	case tc.Skipped != nil:
		res.Status = evergreen.TestSkippedStatus
		log = tc.Skipped.toBasicTestLog("SKIPPED")
	default:
		res.Status = evergreen.TestSucceededStatus
	}

	if log != nil {
		if conf.ProjectRef.IsCedarTestResultsEnabled() {
			// When sending test logs to cedar we need to use a
			// unique string since there may be duplicate file
			// names if there are duplicate test names.
			log.Name = utility.RandomString()
		} else {
			log.Name = res.TestFile
		}
		log.Task = conf.Task.Id
		log.TaskExecution = conf.Task.Execution

		res.LogTestName = log.Name
	}

	return res, log
}

func (fd failureDetails) toBasicTestLog(fdType string) *model.TestLog {
	log := model.TestLog{
		Lines: []string{fmt.Sprintf("%v: %v (%v)", fdType, fd.Message, fd.Type)},
	}
	logLines := strings.Split(strings.TrimSpace(fd.Content), "\n")
	log.Lines = append(log.Lines, logLines...)
	return &log
}
