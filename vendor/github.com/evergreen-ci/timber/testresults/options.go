package testresults

import (
	"time"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/golang/protobuf/ptypes"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// CreateOptions represent options to create a new test results record.
type CreateOptions struct {
	Project                string   `bson:"project" json:"project" yaml:"project"`
	Version                string   `bson:"version" json:"version" yaml:"version"`
	Variant                string   `bson:"variant" json:"variant" yaml:"variant"`
	TaskID                 string   `bson:"task_id" json:"task_id" yaml:"task_id"`
	TaskName               string   `bson:"task_name" json:"task_name" yaml:"task_name"`
	DisplayTaskName        string   `bson:"display_task_name" json:"display_task_name" yaml:"display_task_name"`
	DisplayTaskID          string   `bson:"display_task_id" json:"display_task_id" yaml:"display_task_id"`
	Execution              int32    `bson:"execution" json:"execution" yaml:"execution"`
	RequestType            string   `bson:"request_type" json:"request_type" yaml:"request_type"`
	Mainline               bool     `bson:"mainline" json:"mainline" yaml:"mainline"`
	HistoricalDataIgnore   []string `bson:"historical_data_ignore" json:"historical_data_ignore" yaml:"historical_data_ignore"`
	HistoricalDataDisabled bool     `bson:"historical_data_disabled" json:"historical_data_disabled" yaml:"historical_data_disabled"`
}

func (opts CreateOptions) export() *gopb.TestResultsInfo {
	return &gopb.TestResultsInfo{
		Project:                opts.Project,
		Version:                opts.Version,
		Variant:                opts.Variant,
		TaskName:               opts.TaskName,
		TaskId:                 opts.TaskID,
		DisplayTaskName:        opts.DisplayTaskName,
		DisplayTaskId:          opts.DisplayTaskID,
		Execution:              opts.Execution,
		RequestType:            opts.RequestType,
		Mainline:               opts.Mainline,
		HistoricalDataIgnore:   opts.HistoricalDataIgnore,
		HistoricalDataDisabled: opts.HistoricalDataDisabled,
	}
}

// Results represent a set of test results.
type Results struct {
	ID      string
	Results []Result
}

func (r Results) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(r.ID == "", "must specify test result ID")
	return catcher.Resolve()
}

// export converts Results into the equivalent protobuf TestResults.
func (r Results) export() (*gopb.TestResults, error) {
	var results []*gopb.TestResult
	for _, res := range r.Results {
		exported, err := res.export()
		if err != nil {
			return nil, errors.Wrap(err, "converting test result")
		}
		results = append(results, exported)
	}
	return &gopb.TestResults{
		TestResultsRecordId: r.ID,
		Results:             results,
	}, nil
}

// Result represents a single test result.
type Result struct {
	TestName        string    `bson:"test_name" json:"test_name" yaml:"test_name"`
	DisplayTestName string    `bson:"display_test_name" json:"display_test_name" yaml:"display_test_name"`
	GroupID         string    `bson:"group_id" json:"group_id" yaml:"group_id"`
	Trial           int32     `bson:"trial" json:"trial" yaml:"trial"`
	Status          string    `bson:"status" json:"status" yaml:"status"`
	LogTestName     string    `bson:"log_test_name" json:"log_test_name" yaml:"log_test_name"`
	LogURL          string    `bson:"log_url" json:"log_url" yaml:"log_url"`
	RawLogURL       string    `bson:"raw_log_url" json:"raw_log_url" yaml:"raw_log_url"`
	LineNum         int32     `bson:"line_num" json:"line_num" yaml:"line_num"`
	TaskCreated     time.Time `bson:"task_created" json:"task_created" yaml:"task_created"`
	TestStarted     time.Time `bson:"test_started" json:"test_started" yaml:"test_started"`
	TestEnded       time.Time `bson:"test_ended" json:"test_ended" yaml:"test_ended"`
}

// export converts a Result into the equivalent protobuf TestResult.
func (r Result) export() (*gopb.TestResult, error) {
	created, err := ptypes.TimestampProto(r.TaskCreated)
	if err != nil {
		return nil, errors.Wrap(err, "converting create timestamp")
	}
	started, err := ptypes.TimestampProto(r.TestStarted)
	if err != nil {
		return nil, errors.Wrap(err, "converting start timestamp")
	}
	ended, err := ptypes.TimestampProto(r.TestEnded)
	if err != nil {
		return nil, errors.Wrap(err, "converting end timestamp")
	}
	return &gopb.TestResult{
		TestName:        r.TestName,
		DisplayTestName: r.DisplayTestName,
		GroupId:         r.GroupID,
		Trial:           r.Trial,
		Status:          r.Status,
		LogTestName:     r.LogTestName,
		LogUrl:          r.LogURL,
		RawLogUrl:       r.RawLogURL,
		LineNum:         r.LineNum,
		TaskCreateTime:  created,
		TestStartTime:   started,
		TestEndTime:     ended,
	}, nil
}
