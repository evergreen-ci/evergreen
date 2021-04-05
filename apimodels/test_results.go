package apimodels

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/pkg/errors"
)

// CedarTestResult represents the expected test result format returned from
// cedar.
type CedarTestResult struct {
	TaskID          string    `json:"task_id"`
	Execution       int       `json:"execution"`
	TestName        string    `json:"test_name"`
	DisplayTestName string    `json:"display_test_name"`
	GroupID         string    `json:"group_id"`
	Status          string    `json:"status"`
	LogTestName     string    `json:"log_test_name"`
	LineNum         int       `json:"line_num"`
	Start           time.Time `json:"test_start_time"`
	End             time.Time `json:"test_end_time"`
}

// GetCedarTestResultsOptions represents the arguments passed into the
// GetCedarTestResults function.
type GetCedarTestResultsOptions struct {
	BaseURL       string `json:"-"`
	TaskID        string `json:"-"`
	DisplayTaskID string `json:"-"`
	TestName      string `json:"-"`
	Execution     int    `json:"-"`
}

// GetCedarTestResults makes request to cedar for a task's test results.
func GetCedarTestResults(ctx context.Context, opts GetCedarTestResultsOptions) ([]CedarTestResult, error) {
	getOpts := testresults.TestResultsGetOptions{
		CedarOpts: timber.GetOptions{
			BaseURL: fmt.Sprintf("https://%s", opts.BaseURL),
		},
		TaskID:        opts.TaskID,
		DisplayTaskID: opts.DisplayTaskID,
		TestName:      opts.TestName,
		Execution:     opts.Execution,
	}
	data, err := testresults.GetTestResults(ctx, getOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test results for from cedar")
	}

	testResults := []CedarTestResult{}
	if opts.TestName != "" {
		testResult := CedarTestResult{}
		if err = json.Unmarshal(data, &testResult); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal test result for from cedar")
		}
		testResults = append(testResults, testResult)
	} else {
		if err = json.Unmarshal(data, &testResults); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal test results for from cedar")
		}
	}

	return testResults, nil
}

// DisplayTaskInfo represents information about a display task necessary for
// creating a cedar test result.
type DisplayTaskInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
