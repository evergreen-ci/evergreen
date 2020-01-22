package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
)

// APITest contains the data to be returned whenever a test is used in the
// API.
type APITest struct {
	Id        *string   `json:"test_id"`
	TaskId    *string   `json:"task_id"`
	Status    *string   `json:"status"`
	TestFile  *string   `json:"test_file"`
	Logs      TestLogs  `json:"logs"`
	ExitCode  int       `json:"exit_code"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// TestLogs is a struct for storing the information about logs that will
// be written out as part of an APITest.
type TestLogs struct {
	URL     *string `json:"url"`
	LineNum int     `json:"line_num"`
	URLRaw  *string `json:"url_raw"`
	LogId   *string `json:"log_id"`
}

func (at *APITest) BuildFromService(st interface{}) error {
	switch v := st.(type) {
	case *testresult.TestResult:
		at.Status = ToStringPtr(v.Status)
		fmt.Println("_____________________________________________________________________-")
		fmt.Println(v.TestFile)
		at.TestFile = ToStringPtr(v.TestFile)
		at.ExitCode = v.ExitCode
		at.Id = ToStringPtr(v.ID.Hex())

		startTime := util.FromPythonTime(v.StartTime)
		endTime := util.FromPythonTime(v.EndTime)

		at.StartTime = startTime
		at.EndTime = endTime

		at.Logs = TestLogs{
			URL:     ToStringPtr(v.URL),
			URLRaw:  ToStringPtr(v.URLRaw),
			LogId:   ToStringPtr(v.LogID),
			LineNum: v.LineNum,
		}
	case string:
		at.TaskId = ToStringPtr(v)
	default:
		return fmt.Errorf("Incorrect type when creating APITest")
	}
	return nil
}

func (at *APITest) ToService() (interface{}, error) {
	return &testresult.TestResult{
		Status:    FromStringPtr(at.Status),
		TestFile:  FromStringPtr(at.TestFile),
		URL:       FromStringPtr(at.Logs.URL),
		URLRaw:    FromStringPtr(at.Logs.URLRaw),
		LogID:     FromStringPtr(at.Logs.LogId),
		LineNum:   at.Logs.LineNum,
		ExitCode:  at.ExitCode,
		StartTime: util.ToPythonTime(at.StartTime),
		EndTime:   util.ToPythonTime(at.EndTime),
	}, nil
}
