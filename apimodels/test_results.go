package apimodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/pkg/errors"
)

// Valid sort by keys.
const (
	CedarTestResultsSortByStart      = testresults.SortByStart
	CedarTestResultsSortByDuration   = testresults.SortByDuration
	CedarTestResultsSortByTestName   = testresults.SortByTestName
	CedarTestResultsSortByStatus     = testresults.SortByStatus
	CedarTestResultsSortByBaseStatus = testresults.SortByBaseStatus
)

// CedarTestResults represents the expected test results format returned from
// Cedar.
type CedarTestResults struct {
	Stats   CedarTestResultsStats `json:"stats"`
	Results []CedarTestResult     `json:"results"`
}

// CedarTestResultsStats represents the expected test results stats format
// returned from Cedar.
type CedarTestResultsStats struct {
	TotalCount    int  `json:"total_count"`
	FailedCount   int  `json:"failed_count"`
	FilteredCount *int `json:"filtered_count"`
}

// CedarTestResult represents the expected test result format returned from
// Cedar.
type CedarTestResult struct {
	TaskID          string    `json:"task_id"`
	Execution       int       `json:"execution"`
	TestName        string    `json:"test_name"`
	DisplayTestName string    `json:"display_test_name"`
	GroupID         string    `json:"group_id"`
	Status          string    `json:"status"`
	BaseStatus      string    `json:"base_status"`
	LogTestName     string    `json:"log_test_name"`
	LogURL          string    `json:"log_url"`
	RawLogURL       string    `json:"raw_log_url"`
	LineNum         int       `json:"line_num"`
	Start           time.Time `json:"test_start_time"`
	End             time.Time `json:"test_end_time"`
}

// GetCedarTestResultsOptions represents the arguments for fetching test
// results and related information via Cedar.
type GetCedarTestResultsOptions struct {
	BaseURL string `json:"-"`
	TaskID  string `json:"-"`

	// General query parameters.
	Execution   *int `json:"-"`
	DisplayTask bool `json:"-"`

	// Filter, sortring, and pagination query parameters specific to
	// fetching test results.
	TestName     string   `json:"-"`
	Statuses     []string `json:"-"`
	GroupID      string   `json:"-"`
	SortBy       string   `json:"-"`
	SortOrderDSC bool     `json:"-"`
	BaseTaskID   string   `json:"-"`
	Limit        int      `json:"-"`
	Page         int      `json:"-"`
}

func (opts GetCedarTestResultsOptions) convert() testresults.GetOptions {
	return testresults.GetOptions{
		Cedar: timber.GetOptions{
			BaseURL: fmt.Sprintf("https://%s", opts.BaseURL),
		},
		TaskID:       opts.TaskID,
		Execution:    opts.Execution,
		DisplayTask:  opts.DisplayTask,
		TestName:     opts.TestName,
		Statuses:     opts.Statuses,
		GroupID:      opts.GroupID,
		SortBy:       opts.SortBy,
		SortOrderDSC: opts.SortOrderDSC,
		BaseTaskID:   opts.BaseTaskID,
		Limit:        opts.Limit,
		Page:         opts.Page,
	}
}

// GetCedarTestResults makes a request to Cedar for a task's test results.
func GetCedarTestResults(ctx context.Context, opts GetCedarTestResultsOptions) (*CedarTestResults, error) {
	data, err := testresults.Get(ctx, opts.convert())
	if err != nil {
		return nil, errors.Wrap(err, "getting test results from Cedar")
	}

	testResults := &CedarTestResults{}
	if err = json.Unmarshal(data, testResults); err != nil {
		return nil, errors.Wrap(err, "unmarshaling test results from Cedar")
	}

	return testResults, nil
}

// GetMultiPageCedarTestResults makes a request to Cedar for a task's test
// results and returns an io.ReadCloser that will continue fetching and reading
// subsequent pages of test results if paginated.
func GetMultiPageCedarTestResults(ctx context.Context, opts GetCedarTestResultsOptions) (io.ReadCloser, error) {
	r, err := testresults.GetWithPaginatedReadCloser(ctx, opts.convert())
	if err != nil {
		return nil, errors.Wrap(err, "getting test results from Cedar")
	}

	return r, nil
}

// GetCedarTestResultsStats makes a request to Cedar for a task's test results
// stats. This route ignores filtering, sorting, and pagination query
// parameters.
func GetCedarTestResultsStats(ctx context.Context, opts GetCedarTestResultsOptions) (*CedarTestResultsStats, error) {
	timberOpts := opts.convert()
	timberOpts.Stats = true
	data, err := testresults.Get(ctx, opts.convert())
	if err != nil {
		return nil, errors.Wrap(err, "getting test results stats from Cedar")
	}

	stats := &CedarTestResultsStats{}
	if err = json.Unmarshal(data, stats); err != nil {
		return nil, errors.Wrap(err, "unmarshaling test results stats from cedar")
	}

	return stats, nil
}

// GetCedarTestResultsFailedSample makes a request to Cedar for a task's failed
// test result sample. This route ignores filtering, sorting, and pagination
// query parameters.
func GetCedarTestResultsFailedSample(ctx context.Context, opts GetCedarTestResultsOptions) ([]string, error) {
	timberOpts := opts.convert()
	timberOpts.FailedSample = true
	data, err := testresults.Get(ctx, opts.convert())
	if err != nil {
		return nil, errors.Wrap(err, "getting failed test result sample from Cedar")
	}

	sample := []string{}
	if err = json.Unmarshal(data, &sample); err != nil {
		return nil, errors.Wrap(err, "unmarshaling failed test result sample from Cedar")
	}

	return sample, nil
}

// DisplayTaskInfo represents information about a display task necessary for
// creating a cedar test result.
type DisplayTaskInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
