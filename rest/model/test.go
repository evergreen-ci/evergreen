package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// APITest contains the data to be returned whenever a test is used in the
// API.
type APITest struct {
	ID *string `json:"test_id"`
	// Identifier of the task this test is a part of
	TaskID    *string `json:"task_id"`
	Execution int     `json:"execution"`
	// Execution status of the test
	Status     *string `json:"status"`
	BaseStatus *string `json:"base_status,omitempty"`
	// Name of the test file that this test was run in
	TestFile *string `json:"test_file"`
	GroupID  *string `json:"group_id,omitempty"`
	// Object containing information about the logs for this test
	Logs TestLogs `json:"logs"`
	// Time that this test began execution
	StartTime *time.Time `json:"start_time"`
	// Time that this test stopped execution
	EndTime  *time.Time `json:"end_time"`
	Duration float64    `json:"duration"`
	// The exit code of the process that ran this test
	ExitCode int `json:"-"`
}

// TestLogs is a struct for storing the information about logs that will be
// written out as part of an APITest.
type TestLogs struct {
	// URL where the log can be fetched
	URL *string `json:"url"`
	// URL of the unprocessed version of the logs file for this test
	URLRaw     *string `json:"url_raw"`
	URLParsley *string `json:"url_parsley,omitempty"`
	// Line number in the log file corresponding to information about this test
	LineNum int `json:"line_num"`
	// Test name as represented in the logging backend
	TestName      *string `json:"log_test_name"`
	RenderingType *string `json:"rendering_type"`
	Version       int32   `json:"version"`
}

func (at *APITest) BuildFromService(st any) error {
	env := evergreen.GetEnvironment()

	switch v := st.(type) {
	case *testresult.TestResult:
		at.ID = utility.ToStringPtr(v.TestName)
		at.Execution = v.Execution
		if v.GroupID != "" {
			at.GroupID = utility.ToStringPtr(v.GroupID)
		}
		at.Status = utility.ToStringPtr(v.Status)
		if v.BaseStatus != "" {
			at.BaseStatus = utility.ToStringPtr(v.BaseStatus)
		}
		at.StartTime = utility.ToTimePtr(v.TestStartTime)
		at.EndTime = utility.ToTimePtr(v.TestEndTime)
		at.Duration = v.Duration().Seconds()

		at.TestFile = utility.ToStringPtr(v.GetDisplayTestName())
		at.Logs = TestLogs{
			URL:      utility.ToStringPtr(v.GetLogURL(env, evergreen.LogViewerHTML)),
			URLRaw:   utility.ToStringPtr(v.GetLogURL(env, evergreen.LogViewerRaw)),
			LineNum:  v.LineNum,
			TestName: utility.ToStringPtr(v.GetLogTestName()),
		}
		if parsleyURL := v.GetLogURL(env, evergreen.LogViewerParsley); parsleyURL != "" {
			at.Logs.URLParsley = utility.ToStringPtr(parsleyURL)
		}
		if v.LogInfo != nil {
			at.Logs.RenderingType = v.LogInfo.RenderingType
			if at.Logs.RenderingType == nil {
				at.Logs.RenderingType = v.LogInfo.RenderingTypeCedar
			}
			at.Logs.Version = v.LogInfo.Version
			at.Logs.LineNum = int(v.LogInfo.LineNum)
			if at.Logs.LineNum == 0 {
				at.Logs.LineNum = int(v.LogInfo.LineNumCedar)
			}
		}
	case string:
		at.TaskID = utility.ToStringPtr(v)
	default:
		return errors.Errorf("programmatic error: expected test result but got type %T", st)
	}

	return nil
}

func (at *APITest) ToService() (any, error) {
	// It is not valid translate an APITest object to a TestResult object
	// due to data loss.
	return nil, errors.New("not implemented")
}
