package model

import (
	"fmt"
	"time"

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
		at.Status = ToStringPtr(v.Status)
		at.TestFile = ToStringPtr(v.TestFile)
		at.ExitCode = v.ExitCode
		at.Id = ToStringPtr(v.ID.Hex())

		startTime := utility.FromPythonTime(v.StartTime)
		endTime := utility.FromPythonTime(v.EndTime)
		at.Duration = v.EndTime - v.StartTime
		at.StartTime = ToTimePtr(startTime)
		at.EndTime = ToTimePtr(endTime)

		at.Logs = TestLogs{
			URL:     ToStringPtr(v.URL),
			URLRaw:  ToStringPtr(v.URLRaw),
			LogId:   ToStringPtr(v.LogID),
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
	case string:
		at.TaskId = ToStringPtr(v)
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
		Status:    FromStringPtr(at.Status),
		TestFile:  FromStringPtr(at.TestFile),
		URL:       FromStringPtr(at.Logs.URL),
		URLRaw:    FromStringPtr(at.Logs.URLRaw),
		LogID:     FromStringPtr(at.Logs.LogId),
		LineNum:   at.Logs.LineNum,
		ExitCode:  at.ExitCode,
		StartTime: utility.ToPythonTime(start),
		EndTime:   utility.ToPythonTime(end),
	}, nil
}
