package command

import (
	"context"
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

// contextReader wraps an io.Reader and checks context cancellation periodically.
// It checks context every checkInterval bytes read.
type contextReader struct {
	ctx           context.Context
	reader        io.Reader
	checkInterval int
	bytesRead     int
}

// newContextReader creates a reader that checks context every checkInterval bytes.
func newContextReader(ctx context.Context, reader io.Reader, checkInterval int) *contextReader {
	return &contextReader{
		ctx:           ctx,
		reader:        reader,
		checkInterval: checkInterval,
	}
}

func (r *contextReader) Read(p []byte) (n int, err error) {
	// Check context before reading.
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	n, err = r.reader.Read(p)
	r.bytesRead += n

	// Check context periodically based on bytes read.
	if r.bytesRead >= r.checkInterval {
		r.bytesRead = 0
		if ctxErr := r.ctx.Err(); ctxErr != nil {
			return n, ctxErr
		}
	}

	return n, err
}

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

// contextCheckInterval is the number of bytes between context cancellation checks
// during file reading. 64KB provides a good balance between responsiveness and overhead.
const contextCheckInterval = 64 * 1024

func parseXMLResults(ctx context.Context, reader io.Reader) ([]testSuite, error) {
	// Wrap reader with context-aware reader to allow cancellation during I/O.
	ctxReader := newContextReader(ctx, reader, contextCheckInterval)

	// Use xml.Decoder for streaming parse with context checks between elements.
	decoder := xml.NewDecoder(ctxReader)

	var suites []testSuite

	// Find the root element and parse accordingly.
	// The JUnit XML schema allows for <testsuite> or <testsuites> to be the root.
	// https://github.com/windyroad/JUnit-Schema/blob/master/JUnit.xsd
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "context cancelled during XML parsing")
		}

		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "reading XML token")
		}

		startElem, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		switch startElem.Name.Local {
		case "testsuites":
			// Parse testsuites container - decode child testsuite elements one by one.
			suites, err = parseTestSuitesStreaming(ctx, decoder)
			if err != nil {
				return nil, errors.Wrap(err, "parsing testsuites element")
			}
		case "testsuite":
			// Single testsuite as root - decode it directly.
			var suite testSuite
			if err := decoder.DecodeElement(&suite, &startElem); err != nil {
				return nil, errors.Wrap(err, "parsing testsuite element")
			}
			suites = append(suites, suite)
		}
	}

	return suites, nil
}

// parseTestSuitesStreaming parses testsuite elements from within a testsuites container,
// checking context between each suite for cancellation.
func parseTestSuitesStreaming(ctx context.Context, decoder *xml.Decoder) ([]testSuite, error) {
	var suites []testSuite

	for {
		// Check context between parsing each test suite.
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "context cancelled while parsing test suites")
		}

		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "reading XML token")
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "testsuite" {
				var suite testSuite
				if err := decoder.DecodeElement(&suite, &elem); err != nil {
					return nil, errors.Wrap(err, "parsing testsuite element")
				}
				suites = append(suites, suite)
			}
		case xml.EndElement:
			if elem.Name.Local == "testsuites" {
				return suites, nil
			}
		}
	}

	return suites, nil
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
		// When sending test logs we need to use a
		// unique string since there may be duplicate file
		// names if there are duplicate test names.
		log.Name = utility.RandomString()
		log.Task = conf.Task.Id
		log.TaskExecution = conf.Task.Execution
		res.LogInfo = &testresult.TestLogInfo{LogName: log.Name}
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
