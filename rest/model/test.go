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
	ID         *string    `json:"test_id"`
	TaskID     *string    `json:"task_id"`
	Execution  int        `json:"execution"`
	Status     *string    `json:"status"`
	BaseStatus *string    `json:"base_status,omitempty"`
	TestFile   *string    `json:"test_file"`
	GroupID    *string    `json:"group_id,omitempty"`
	Logs       TestLogs   `json:"logs"`
	ExitCode   int        `json:"exit_code"`
	StartTime  *time.Time `json:"start_time"`
	EndTime    *time.Time `json:"end_time"`
	Duration   float64    `json:"duration"`

	env evergreen.Environment
}

// TestLogs is a struct for storing the information about logs that will be
// written out as part of an APITest.
type TestLogs struct {
	URL        *string `json:"url"`
	URLRaw     *string `json:"url_raw"`
	URLLobster *string `json:"url_lobster,omitempty"`
	URLParsley *string `json:"url_parsley,omitempty"`
	LineNum    int     `json:"line_num"`
}

func (at *APITest) BuildFromService(st interface{}) error {
	if at.env == nil {
		at.env = evergreen.GetEnvironment()
	}

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
		at.StartTime = utility.ToTimePtr(v.Start)
		at.EndTime = utility.ToTimePtr(v.End)
		at.Duration = v.End.Sub(v.Start).Seconds() // TODO: Do we want this in seconds or ms?

		at.TestFile = utility.ToStringPtr(v.GetDisplayTestName())
		at.Logs = TestLogs{
			URL:     utility.ToStringPtr(v.GetLogURL(at.env, evergreen.LogViewerHTML)),
			URLRaw:  utility.ToStringPtr(v.GetLogURL(at.env, evergreen.LogViewerRaw)),
			LineNum: v.LineNum,
		}
		if lobsterURL := v.GetLogURL(at.env, evergreen.LogViewerLobster); lobsterURL != "" {
			at.Logs.URLLobster = utility.ToStringPtr(lobsterURL)
		}
		if parsleyURL := v.GetLogURL(at.env, evergreen.LogViewerParsley); parsleyURL != "" {
			at.Logs.URLParsley = utility.ToStringPtr(parsleyURL)
		}

	case string:
		at.TaskID = utility.ToStringPtr(v)
	default:
		return errors.Errorf("programmatic error: expected test result but got type %T", st)
	}

	return nil
}

func (at *APITest) ToService() (interface{}, error) {
	// It is not valid translate an APITest object to a TestResult object
	// due to data loss.
	return nil, errors.New("not implemented")
}
