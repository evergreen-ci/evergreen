package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
)

// APITest contains the data to be returned whenever a test is used in the
// API.
type APITest struct {
	TaskId    APIString `json:"task_id"`
	Status    APIString `json:"status"`
	TestFile  APIString `json:"test_file"`
	Logs      TestLogs  `json:"logs"`
	ExitCode  int       `json:"exit_code"`
	StartTime APITime   `json:"start_time"`
	EndTime   APITime   `json:"end_time"`
}

// TestLogs is a struct for storing the information about logs that will
// be written out as part of an APITest.
type TestLogs struct {
	URL     APIString `json:"url"`
	LineNum int       `json:"line_num"`
	URLRaw  APIString `json:"url_raw"`
	LogId   APIString `json:"log_id"`
}

func (at *APITest) BuildFromService(st interface{}) error {
	switch v := st.(type) {
	case *task.TestResult:
		at.Status = APIString(v.Status)
		at.TestFile = APIString(v.TestFile)
		at.ExitCode = v.ExitCode

		startTime := util.FromPythonTime(v.StartTime)
		endTime := util.FromPythonTime(v.EndTime)

		at.StartTime = NewTime(startTime)
		at.EndTime = NewTime(endTime)

		at.Logs = TestLogs{
			URL:     APIString(v.URL),
			URLRaw:  APIString(v.URLRaw),
			LogId:   APIString(v.LogId),
			LineNum: v.LineNum,
		}
	case string:
		at.TaskId = APIString(v)
	default:
		return fmt.Errorf("Incorrect type when creating APITest")
	}
	return nil
}

func (at *APITest) ToService() (interface{}, error) {
	return &task.TestResult{
		Status:    string(at.Status),
		TestFile:  string(at.TestFile),
		URL:       string(at.Logs.URL),
		URLRaw:    string(at.Logs.URLRaw),
		LogId:     string(at.Logs.LogId),
		LineNum:   at.Logs.LineNum,
		ExitCode:  at.ExitCode,
		StartTime: util.ToPythonTime(time.Time(at.StartTime)),
		EndTime:   util.ToPythonTime(time.Time(at.EndTime)),
	}, nil
}
