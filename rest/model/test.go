package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
)

// APITest contains the data to be returned whenever a test is used in the
// API.
type APITest struct {
	Id         *string    `json:"test_id"`
	TaskId     *string    `json:"task_id"`
	Status     *string    `json:"status"`
	BaseStatus *string    `json:"base_status"`
	TestFile   *string    `json:"test_file"`
	Logs       TestLogs   `json:"logs"`
	ExitCode   int        `json:"exit_code"`
	StartTime  *time.Time `json:"start_time"`
	EndTime    *time.Time `json:"end_time"`
	Duration   float64    `json:"duration"`
}

// TestLogs is a struct for storing the information about logs that will
// be written out as part of an APITest.
type TestLogs struct {
	URL            *string `json:"url"`
	LineNum        int     `json:"line_num"`
	URLRaw         *string `json:"url_raw"`
	LogId          *string `json:"log_id"`
	RawDisplayURL  *string `json:"url_raw_display"`
	HTMLDisplayURL *string `json:"url_html_display"`
}

func (at *APITest) BuildFromService(st interface{}) error {
	switch v := st.(type) {
	case *testresult.TestResult:
		at.Status = utility.ToStringPtr(v.Status)
		at.TestFile = utility.ToStringPtr(v.TestFile)
		at.ExitCode = v.ExitCode
		at.Id = utility.ToStringPtr(v.ID.Hex())

		startTime := utility.FromPythonTime(v.StartTime)
		endTime := utility.FromPythonTime(v.EndTime)
		at.Duration = v.EndTime - v.StartTime
		at.StartTime = ToTimePtr(startTime)
		at.EndTime = ToTimePtr(endTime)

		at.Logs = TestLogs{
			URL:     utility.ToStringPtr(v.URL),
			URLRaw:  utility.ToStringPtr(v.URLRaw),
			LogId:   utility.ToStringPtr(v.LogID),
			LineNum: v.LineNum,
		}

		isEmptyLogID := v.LogID == ""
		isEmptyURL := v.URL == ""
		isEmptyURLRaw := v.URLRaw == ""

		if !isEmptyURL {
			at.Logs.HTMLDisplayURL = at.Logs.URL
		} else if isEmptyLogID {
			at.Logs.HTMLDisplayURL = nil
		} else {
			dispString := fmt.Sprintf("/test_log/%s#L%d", *at.Logs.LogId, at.Logs.LineNum)
			at.Logs.HTMLDisplayURL = &dispString
		}

		if !isEmptyURLRaw {
			at.Logs.RawDisplayURL = at.Logs.URLRaw
		} else if isEmptyLogID {
			at.Logs.RawDisplayURL = nil
		} else {
			dispString := fmt.Sprintf("/test_log/%s?raw=1", *at.Logs.LogId)
			at.Logs.RawDisplayURL = &dispString
		}
	case *apimodels.CedarTestResult:
		at.Status = utility.ToStringPtr(v.Status)
		at.TestFile = utility.ToStringPtr(v.TestName)
		at.StartTime = utility.ToTimePtr(v.Start)
		at.EndTime = utility.ToTimePtr(v.End)
		duration := v.End.Sub(v.Start)
		at.Duration = float64(duration)
		at.Logs = TestLogs{
			LineNum: v.LineNum,
		}

		// Need to generate a consistent id for test results
		testResultID := fmt.Sprintf("ceder_test_%s_%d_%s_%s_%s", v.TaskID, v.Execution, v.TestName, v.Start, v.LogURL)
		at.Id = utility.ToStringPtr(testResultID)
	case string:
		at.TaskId = utility.ToStringPtr(v)
	default:
		return fmt.Errorf("Incorrect type when creating APITest")
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
		Status:    utility.FromStringPtr(at.Status),
		TestFile:  utility.FromStringPtr(at.TestFile),
		URL:       utility.FromStringPtr(at.Logs.URL),
		URLRaw:    utility.FromStringPtr(at.Logs.URLRaw),
		LogID:     utility.FromStringPtr(at.Logs.LogId),
		LineNum:   at.Logs.LineNum,
		ExitCode:  at.ExitCode,
		StartTime: utility.ToPythonTime(start),
		EndTime:   utility.ToPythonTime(end),
	}, nil
}
