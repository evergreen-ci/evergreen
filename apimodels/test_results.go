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
	TaskID    string    `json:"task_id"`
	Execution int       `json:"execution"`
	TestName  string    `json:"test_name"`
	Status    string    `json:"status"`
	LogURL    string    `json:"log_url"`
	LineNum   int       `json:"line_num"`
	Start     time.Time `json:"test_start_time"`
	End       time.Time `json:"test_end_time"`
}

// GetCedarTestResultsOptions represents the arguments passed into the
// GetCedarTestResults function.
type GetCedarTestResultsOptions struct {
	BaseURL   string
	TaskID    string
	TestName  string
	Execution int
}

// GetCedarTestResults makes request to cedar for a task's test results.
func GetCedarTestResults(ctx context.Context, opts GetCedarTestResultsOptions) ([]CedarTestResult, error) {
	getOpts := timber.GetOptions{
		BaseURL:   fmt.Sprintf("https://%s", opts.BaseURL),
		TaskID:    opts.TaskID,
		TestName:  opts.TestName,
		Execution: opts.Execution,
	}
	data, err := testresults.GetTestResults(ctx, getOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get test results for '%s' from cedar", opts.TaskID)
	}

	testResults := []CedarTestResult{}
	if err = json.Unmarshal(data, &testResults); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal test results for '%s' from cedar", opts.TaskID)
	}

	return testResults, nil
}
