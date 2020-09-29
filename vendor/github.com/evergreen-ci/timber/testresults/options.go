package testresults

import (
	"time"

	"github.com/evergreen-ci/timber/internal"
	"github.com/golang/protobuf/ptypes"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// CreateOptions represent options to create a new test results record.
type CreateOptions struct {
	Project     string `bson:"project" json:"project" yaml:"project"`
	Version     string `bson:"version" json:"version" yaml:"version"`
	Variant     string `bson:"variant" json:"variant" yaml:"variant"`
	TaskID      string `bson:"task_id" json:"task_id" yaml:"task_id"`
	TaskName    string `bson:"task_name" json:"task_name" yaml:"task_name"`
	Execution   int32  `bson:"execution" json:"execution" yaml:"execution"`
	RequestType string `bson:"request_type" json:"request_type" yaml:"request_type"`
	Mainline    bool   `bson:"mainline" json:"mainline" yaml:"mainline"`
}

func (opts CreateOptions) export() *internal.TestResultsInfo {
	return &internal.TestResultsInfo{
		Project:     opts.Project,
		Version:     opts.Version,
		Variant:     opts.Variant,
		TaskName:    opts.TaskName,
		TaskId:      opts.TaskID,
		Execution:   opts.Execution,
		RequestType: opts.RequestType,
		Mainline:    opts.Mainline,
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
func (r Results) export() (*internal.TestResults, error) {
	var results []*internal.TestResult
	for _, res := range r.Results {
		exported, err := res.export()
		if err != nil {
			return nil, errors.Wrap(err, "converting test result")
		}
		results = append(results, exported)
	}
	return &internal.TestResults{
		TestResultsRecordId: r.ID,
		Results:             results,
	}, nil
}

// Result represents a single test result.
type Result struct {
	Name        string
	Trial       int32
	Status      string
	LineNum     int32
	TaskCreated time.Time
	TestStarted time.Time
	TestEnded   time.Time
}

// export converts a Result into the equivalent protobuf TestResult.
func (r Result) export() (*internal.TestResult, error) {
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
	return &internal.TestResult{
		TestName:       r.Name,
		Trial:          r.Trial,
		Status:         r.Status,
		LineNum:        r.LineNum,
		TaskCreateTime: created,
		TestStartTime:  started,
		TestEndTime:    ended,
	}, nil
}
