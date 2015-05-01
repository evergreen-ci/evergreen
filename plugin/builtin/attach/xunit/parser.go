package xunit

import (
	"encoding/xml"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"io"
	"strings"
	"time"
)

type XUnitResults []TestSuite

type TestSuite struct {
	Errors    int        `xml:"errors,attr"`
	Failures  int        `xml:"failures,attr"`
	Skip      int        `xml:"skip,attr"`
	Name      string     `xml:"name,attr"`
	TestCases []TestCase `xml:"testcase"`
	SysOut    string     `xml:"system-out"`
	SysErr    string     `xml:"system-err"`
}

type TestCase struct {
	Name      string          `xml:"name,attr"`
	Time      float64         `xml:"time,attr"`
	ClassName string          `xml:"classname,attr"`
	Failure   *FailureDetails `xml:"failure"`
	Error     *FailureDetails `xml:"error"`
	Skipped   *FailureDetails `xml:"skipped"`
}

type FailureDetails struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

func ParseXMLResults(reader io.Reader) (XUnitResults, error) {
	results := XUnitResults{}
	if err := xml.NewDecoder(reader).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

// ToModelTestResultAndLog converts an xunit test case into an
// mci model.TestResult and model.TestLog. Logs are only
// generated if the test case did not succeed (this is part of
// the xunit xml file design)
func (tc TestCase) ToModelTestResultAndLog(task *model.Task) (model.TestResult, *model.TestLog) {

	res := model.TestResult{}
	var log *model.TestLog

	if tc.ClassName != "" {
		res.TestFile = fmt.Sprintf("%v.%v", tc.ClassName, tc.Name)
	} else {
		res.TestFile = tc.Name
	}
	// replace spaces with dashes
	res.TestFile = strings.Replace(res.TestFile, " ", "-", -1)

	res.StartTime = float64(time.Now().Unix())
	res.EndTime = float64(res.StartTime + tc.Time)

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
		log.Name = res.TestFile
		log.Task = task.Id
		log.TaskExecution = task.Execution

		// update the URL of the result to the expected log URL
		res.URL = log.URL()
	}

	return res, log
}

func (fd FailureDetails) toBasicTestLog(fdType string) *model.TestLog {
	log := model.TestLog{
		Lines: []string{fmt.Sprintf("%v: %v (%v)", fdType, fd.Message, fd.Type)},
	}
	logLines := strings.Split(strings.TrimSpace(fd.Content), "\n")
	log.Lines = append(log.Lines, logLines...)
	return &log
}
