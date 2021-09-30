package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// APITest contains the data to be returned whenever a test is used in the
// API.
type APITest struct {
	ID         *string `json:"test_id"`
	TaskID     *string `json:"task_id"`
	Execution  int     `json:"execution"`
	Status     *string `json:"status"`
	BaseStatus *string `json:"base_status,omitempty"`
	TestFile   *string `json:"test_file"`
	// TODO: (EVG-15379) Remove this field once Spruce dependency is gone.
	DisplayTestName *string    `json:"display_test_name"`
	GroupID         *string    `json:"group_id,omitempty"`
	Logs            TestLogs   `json:"logs"`
	ExitCode        int        `json:"exit_code"`
	StartTime       *time.Time `json:"start_time"`
	EndTime         *time.Time `json:"end_time"`
	Duration        float64    `json:"duration"`
}

// TestLogs is a struct for storing the information about logs that will be
// written out as part of an APITest.
type TestLogs struct {
	URL        *string `json:"url"`
	URLRaw     *string `json:"url_raw"`
	URLLobster *string `json:"url_lobster,omitempty"`
	LineNum    int     `json:"line_num"`
	LogID      *string `json:"log_id,omitempty"`
}

func (at *APITest) BuildFromService(st interface{}) error {
	switch v := st.(type) {
	case *testresult.TestResult:
		at.ID = utility.ToStringPtr(v.ID.Hex())
		at.Execution = v.Execution
		if v.GroupID != "" {
			at.GroupID = utility.ToStringPtr(v.GroupID)
		}
		at.Status = utility.ToStringPtr(v.Status)
		at.ExitCode = v.ExitCode
		startTime := utility.FromPythonTime(v.StartTime)
		endTime := utility.FromPythonTime(v.EndTime)
		at.Duration = v.EndTime - v.StartTime
		at.StartTime = ToTimePtr(startTime)
		at.EndTime = ToTimePtr(endTime)

		tr := task.ConvertToOld(v)
		at.TestFile = utility.ToStringPtr(tr.GetDisplayTestName())
		at.Logs = TestLogs{
			URL:     utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerHTML)),
			URLRaw:  utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerRaw)),
			LineNum: v.LineNum,
		}
		if lobsterURL := tr.GetLogURL(evergreen.LogViewerLobster); lobsterURL != "" {
			at.Logs.URLLobster = utility.ToStringPtr(lobsterURL)
		}
		if v.LogID != "" {
			at.Logs.LogID = utility.ToStringPtr(v.LogID)
		}
	case *apimodels.CedarTestResult:
		at.ID = utility.ToStringPtr(v.TestName)
		at.Execution = v.Execution
		if v.GroupID != "" {
			at.GroupID = utility.ToStringPtr(v.GroupID)
		}
		at.Status = utility.ToStringPtr(v.Status)
		if v.BaseStatus != "" {
			at.BaseStatus = utility.ToStringPtr(v.BaseStatus)
		}
		at.StartTime = utility.ToTimePtr(v.Start)
		at.EndTime = utility.ToTimePtr(v.End)
		at.Duration = v.End.Sub(v.Start).Seconds()

		tr := task.ConvertCedarTestResult(*v)
		at.TestFile = utility.ToStringPtr(tr.GetDisplayTestName())
		at.Logs = TestLogs{
			URL:     utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerHTML)),
			URLRaw:  utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerRaw)),
			LineNum: v.LineNum,
		}
		if lobsterURL := tr.GetLogURL(evergreen.LogViewerLobster); lobsterURL != "" {
			at.Logs.URLLobster = utility.ToStringPtr(lobsterURL)
		}
	case string:
		at.TaskID = utility.ToStringPtr(v)
	default:
		return fmt.Errorf("incorrect type '%v' when creating APITest", v)
	}

	return nil
}

func (at *APITest) ToService() (interface{}, error) {
	// It is not valid translate an APITest object to a TestResult object
	// due to data loss.
	return nil, errors.New("not implemented")
}
