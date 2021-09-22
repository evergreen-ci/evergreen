package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
)

// APITest contains the data to be returned whenever a test is used in the
// API.
type APITest struct {
	Id              *string    `json:"test_id"`
	TaskId          *string    `json:"task_id"`
	Execution       int        `json:"execution"`
	GroupId         *string    `json:"group_id,omitempty"`
	Status          *string    `json:"status"`
	BaseStatus      *string    `json:"base_status,omitempty"`
	TestFile        *string    `json:"test_file"`
	DisplayTestName *string    `json:"display_test_name,omitempty"`
	Logs            TestLogs   `json:"logs"`
	ExitCode        int        `json:"exit_code"`
	StartTime       *time.Time `json:"start_time"`
	EndTime         *time.Time `json:"end_time"`
	Duration        float64    `json:"duration"`
	LogTestName     *string    `json:"log_test_name,omitempty"`
}

// TestLogs is a struct for storing the information about logs that will be
// written out as part of an APITest.
type TestLogs struct {
	URL        *string `json:"url"`
	URLRaw     *string `json:"url_raw"`
	URLLobster *string `json:"url_lobster,omitempty"`
	LineNum    int     `json:"line_num"`
	LogId      *string `json:"log_id,omitempty"`

	// TODO: (EVG-15418): Remove once spruce is updated.
	HTMLDisplayURL *string `json:"url_raw_display"`
	RawDisplayURL  *string `json:"url_html_display"`
}

func (at *APITest) BuildFromService(st interface{}) error {
	switch v := st.(type) {
	case *testresult.TestResult:
		at.Id = utility.ToStringPtr(v.ID.Hex())
		at.Execution = v.Execution
		if v.GroupID != "" {
			at.GroupId = utility.ToStringPtr(v.GroupID)
		}
		at.Status = utility.ToStringPtr(v.Status)
		at.TestFile = utility.ToStringPtr(v.TestFile)
		if v.DisplayTestName != "" {
			at.DisplayTestName = utility.ToStringPtr(v.DisplayTestName)
		}
		at.ExitCode = v.ExitCode
		startTime := utility.FromPythonTime(v.StartTime)
		endTime := utility.FromPythonTime(v.EndTime)
		at.Duration = v.EndTime - v.StartTime
		at.StartTime = ToTimePtr(startTime)
		at.EndTime = ToTimePtr(endTime)

		tr := task.ConvertToOld(v)
		at.Logs = TestLogs{
			URL:        utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerHTML)),
			URLRaw:     utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerRaw)),
			URLLobster: utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerLobster)),
			LineNum:    v.LineNum,

			// TODO: (EVG-15418) Remove after spruce is updated.
			HTMLDisplayURL: utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerHTML)),
			RawDisplayURL:  utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerRaw)),
		}
		if v.LogID != "" {
			at.Logs.LogId = utility.ToStringPtr(v.LogID)
		}

	case *apimodels.CedarTestResult:
		at.Id = utility.ToStringPtr(v.TestName)
		at.Execution = v.Execution
		if v.GroupID != "" {
			at.GroupId = utility.ToStringPtr(v.GroupID)
		}
		at.Status = utility.ToStringPtr(v.Status)
		at.TestFile = utility.ToStringPtr(v.TestName)
		if v.DisplayTestName != "" {
			at.DisplayTestName = utility.ToStringPtr(v.DisplayTestName)
		}
		at.StartTime = utility.ToTimePtr(v.Start)
		at.EndTime = utility.ToTimePtr(v.End)
		at.Duration = v.End.Sub(v.Start).Seconds()
		if v.LogTestName != "" {
			at.LogTestName = utility.ToStringPtr(v.LogTestName)
		}

		tr := task.ConvertCedarTestResult(*v)
		at.Logs = TestLogs{
			URL:        utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerHTML)),
			URLRaw:     utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerRaw)),
			URLLobster: utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerLobster)),
			LineNum:    v.LineNum,

			// TODO: (EVG-15418) Remove after spruce is updated.
			HTMLDisplayURL: utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerHTML)),
			RawDisplayURL:  utility.ToStringPtr(tr.GetLogURL(evergreen.LogViewerRaw)),
		}

	case string:
		at.TaskId = utility.ToStringPtr(v)
	default:
		return fmt.Errorf("incorrect type '%v' when creating APITest", v)
	}

	return nil
}

func (at *APITest) ToService() (interface{}, error) {
	catcher := grip.NewBasicCatcher()
	start, err := FromTimePtr(at.StartTime)
	catcher.Add(err)
	end, err := FromTimePtr(at.EndTime)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return &testresult.TestResult{
		Status:          utility.FromStringPtr(at.Status),
		TestFile:        utility.FromStringPtr(at.TestFile),
		DisplayTestName: utility.FromStringPtr(at.DisplayTestName),
		URL:             utility.FromStringPtr(at.Logs.URL),
		URLRaw:          utility.FromStringPtr(at.Logs.URLRaw),
		LogID:           utility.FromStringPtr(at.Logs.LogId),
		LineNum:         at.Logs.LineNum,
		ExitCode:        at.ExitCode,
		StartTime:       utility.ToPythonTime(start),
		EndTime:         utility.ToPythonTime(end),
		GroupID:         utility.FromStringPtr(at.GroupId),
	}, nil
}
