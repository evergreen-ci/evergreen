package apimodels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	if !strings.HasPrefix(opts.BaseURL, "http") {
		opts.BaseURL = fmt.Sprintf("https://%s", opts.BaseURL)
	}
	return testresults.GetOptions{
		Cedar: timber.GetOptions{
			BaseURL: opts.BaseURL,
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
func GetCedarTestResults(ctx context.Context, opts GetCedarTestResultsOptions) (*CedarTestResults, int, error) {
	data, status, err := testresults.Get(ctx, opts.convert())
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting test results from Cedar")
	}

	testResults := &CedarTestResults{}
	if status != http.StatusOK {
		return testResults, status, nil
	}

	return testResults, status, errors.Wrap(json.Unmarshal(data, testResults), "unmarshaling test results from Cedar")
}

// GetCedarTestResultsWithStatusError  makes a request to Cedar for a task's
// test results. An error is returned if Cedar sends a non-200 HTTP status.
func GetCedarTestResultsWithStatusError(ctx context.Context, opts GetCedarTestResultsOptions) (*CedarTestResults, error) {
	testResults, status, err := GetCedarTestResults(ctx, opts)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, errors.Errorf("getting test results from Cedar returned HTTP status '%d'", status)
	}

	return testResults, nil
}

// GetMultiPageCedarTestResults makes a request to Cedar for a task's test
// results and returns an io.ReadCloser that will continue fetching and reading
// subsequent pages of test results if paginated.
func GetMultiPageCedarTestResults(ctx context.Context, opts GetCedarTestResultsOptions) (io.ReadCloser, int, error) {
	r, status, err := testresults.GetWithPaginatedReadCloser(ctx, opts.convert())
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting test results from Cedar")
	}

	return r, status, nil
}

// GetCedarTestResultsStats makes a request to Cedar for a task's test results
// stats. This route ignores filtering, sorting, and pagination query
// parameters.
func GetCedarTestResultsStats(ctx context.Context, opts GetCedarTestResultsOptions) (*CedarTestResultsStats, int, error) {
	timberOpts := opts.convert()
	timberOpts.Stats = true
	data, status, err := testresults.Get(ctx, timberOpts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting test results stats from Cedar")
	}

	stats := &CedarTestResultsStats{}
	if status != http.StatusOK {
		return stats, status, nil
	}

	return stats, status, errors.Wrap(json.Unmarshal(data, stats), "unmarshaling test results stats from Cedar")
}

// GetCedarTestResultsStatsWithStatusError makes a request to Cedar for a
// task's test results stats. This route ignores filtering, sorting, and
// pagination query parameters. An error is returned if Cedar sends a non-200
// HTTP status.
func GetCedarTestResultsStatsWithStatusError(ctx context.Context, opts GetCedarTestResultsOptions) (*CedarTestResultsStats, error) {
	stats, status, err := GetCedarTestResultsStats(ctx, opts)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, errors.Errorf("getting test results stats from Cedar returned HTTP status '%d'", status)
	}

	return stats, nil
}

// GetCedarTestResultsFailedSample makes a request to Cedar for a task's failed
// test result sample. This route ignores filtering, sorting, and pagination
// query parameters.
func GetCedarTestResultsFailedSample(ctx context.Context, opts GetCedarTestResultsOptions) ([]string, int, error) {
	timberOpts := opts.convert()
	timberOpts.FailedSample = true
	data, status, err := testresults.Get(ctx, timberOpts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting failed test result sample from Cedar")
	}

	sample := []string{}
	if status != http.StatusOK {
		return sample, status, nil
	}

	return sample, status, errors.Wrap(json.Unmarshal(data, &sample), "unmarshaling failed test result sample from Cedar")
}

// GetCedarTestResultsFailedSampleWithStatusError makes a request to Cedar for
// a task's failed test result sample. This route ignores filtering, sorting,
// and pagination query parameters. An error is returned if Cedar sends a
// non-200 HTTP status.
func GetCedarTestResultsFailedSampleWithStatusError(ctx context.Context, opts GetCedarTestResultsOptions) ([]string, error) {
	sample, status, err := GetCedarTestResultsFailedSample(ctx, opts)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, errors.Errorf("getting failed test result sample from Cedar returned HTTP status '%d'", status)
	}

	return sample, nil
}

// CedarFailedTestResultsSample is a sample of test names for a given task and execution.
type CedarFailedTestResultsSample struct {
	TaskID                  *string  `json:"task_id"`
	Execution               int      `json:"execution"`
	MatchingFailedTestNames []string `json:"matching_failed_test_names"`
	TotalFailedNames        int      `json:"total_failed_names"`
}

// GetCedarFailedTestResultsSampleOptions represents the arguments for fetching
// test names for failed tests via Cedar.
type GetCedarFailedTestResultsSampleOptions struct {
	BaseURL       string
	SampleOptions CedarFailedTestSampleOptions
}

// CedarFailedTestSampleOptions specifies the tasks to get the sample for
// and regexes to filter the test names by.
type CedarFailedTestSampleOptions struct {
	Tasks        []CedarTaskInfo
	RegexFilters []string
}

// CedarTaskInfo specifies a set of test results to find.
type CedarTaskInfo struct {
	TaskID      string
	Execution   int
	DisplayTask bool
}

func (opts GetCedarFailedTestResultsSampleOptions) convert() testresults.GetFailedSampleOptions {
	return testresults.GetFailedSampleOptions{
		Cedar: timber.GetOptions{
			BaseURL: fmt.Sprintf("https://%s", opts.BaseURL),
		},
		SampleOptions: opts.SampleOptions.convert(),
	}
}

func (opts *CedarFailedTestSampleOptions) convert() testresults.FailedTestSampleOptions {
	tasks := make([]testresults.TaskInfo, 0, len(opts.Tasks))
	for _, t := range opts.Tasks {
		tasks = append(tasks, t.convert())
	}

	return testresults.FailedTestSampleOptions{
		Tasks:        tasks,
		RegexFilters: opts.RegexFilters,
	}
}

func (info *CedarTaskInfo) convert() testresults.TaskInfo {
	return testresults.TaskInfo{
		TaskID:      info.TaskID,
		Execution:   info.Execution,
		DisplayTask: info.DisplayTask,
	}
}

// GetCedarFilteredFailedSamples makes a request to Cedar for failed
// test result samples for the specified tasks.
func GetCedarFilteredFailedSamples(ctx context.Context, opts GetCedarFailedTestResultsSampleOptions) ([]CedarFailedTestResultsSample, error) {
	data, err := testresults.GetFailedSamples(ctx, opts.convert())
	if err != nil {
		return nil, errors.Wrap(err, "getting failed test result samples from Cedar")
	}

	samples := []CedarFailedTestResultsSample{}
	if err = json.Unmarshal(data, &samples); err != nil {
		return nil, errors.Wrap(err, "unmarshaling failed test result samples from Cedar")
	}

	return samples, nil
}

// DisplayTaskInfo represents information about a display task necessary for
// creating a cedar test result.
type DisplayTaskInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
