package testresult

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
)

// TaskTestResults represents a set of test results from an Evergreen task run.
type TaskTestResults struct {
	Stats   TaskTestResultsStats `json:"stats"`
	Results []TestResult         `json:"results"`
}

// TaskTestResultsStats represents basic statistics of a set of test results
// from an Evergreen task run.
type TaskTestResultsStats struct {
	TotalCount    int  `json:"total_count"`
	FailedCount   int  `json:"failed_count"`
	FilteredCount *int `json:"filtered_count"`
}

// TestResult represents a single test result from an Evergreen task run.
type TestResult struct {
	TaskID          string `json:"task_id"`
	Execution       int    `json:"execution"`
	TestName        string `json:"test_name"`
	DisplayTestName string `json:"display_test_name"`
	GroupID         string `json:"group_id"`
	Status          string `json:"status"`
	BaseStatus      string `json:"base_status"`
	LogTestName     string `json:"log_test_name"`
	LogURL          string `json:"log_url"`
	RawLogURL       string `json:"raw_log_url"`
	LineNum         int    `json:"line_num"`
	// TODO: Keep these time.Time or convert them to floats?
	Start time.Time `json:"test_start_time"`
	End   time.Time `json:"test_end_time"`
}

// GetLogTestName returns the name of the test in the logging backend. This is
// used for test logs where the name of the test in the logging service may
// differ from that in the test results service.
func (tr TestResult) GetLogTestName() string {
	if tr.LogTestName != "" {
		return tr.LogTestName
	}

	return tr.TestName
}

// GetDisplayTestName returns the name of the test that should be displayed in
// the UI. In most cases, this will just be TestName.
func (tr TestResult) GetDisplayTestName() string {
	if tr.DisplayTestName != "" {
		return tr.DisplayTestName
	}

	return tr.TestName
}

// GetLogURL returns the external or internal log URL for this test result.
//
// It is not advisable to set URL or URLRaw with the output of this function as
// those fields are reserved for external logs and used to determine URL
// generation for other log viewers.
func (tr TestResult) GetLogURL(env evergreen.Environment, viewer evergreen.LogViewer) string {
	root := env.Settings().ApiUrl
	parsleyURL := env.Settings().Ui.ParsleyUrl
	deprecatedLobsterURLs := []string{"https://logkeeper.mongodb.org", "https://logkeeper2.build.10gen.cc"}

	switch viewer {
	case evergreen.LogViewerHTML:
		// Return an empty string for logkeeper URLS.
		if tr.LogURL != "" {
			for _, url := range deprecatedLobsterURLs {
				if strings.Contains(tr.LogURL, url) {
					return ""
				}
			}
			// Some test results may have internal URLs that are
			// missing the root.
			if err := util.CheckURL(tr.LogURL); err != nil {
				return root + tr.LogURL
			}

			return tr.LogURL
		}

		return fmt.Sprintf("%s/test_log/%s/%d?test_name=%s&group_id=%s#L%d",
			root,
			url.PathEscape(tr.TaskID),
			tr.Execution,
			url.QueryEscape(tr.GetLogTestName()),
			url.QueryEscape(tr.GroupID),
			tr.LineNum,
		)
	case evergreen.LogViewerLobster:
		// Evergreen-hosted lobster does not support external logs.
		if tr.LogURL != "" || tr.RawLogURL != "" {
			for _, url := range deprecatedLobsterURLs {
				if strings.Contains(tr.LogURL, url) {
					return strings.Replace(tr.LogURL, url, root+"/lobster", 1)
				}
			}
			return ""

		}

		return fmt.Sprintf("%s/lobster/evergreen/test/%s/%d/%s/%s#shareLine=%d",
			root,
			url.PathEscape(tr.TaskID),
			tr.Execution,
			url.QueryEscape(tr.GetLogTestName()),
			url.QueryEscape(tr.GroupID),
			tr.LineNum,
		)
	case evergreen.LogViewerParsley:
		if parsleyURL == "" {
			return ""
		}

		for _, url := range deprecatedLobsterURLs {
			if strings.Contains(tr.LogURL, url) {
				updatedResmokeParsleyURL := strings.Replace(tr.LogURL, fmt.Sprintf("%s/build", url), parsleyURL+"/resmoke", 1)
				return fmt.Sprintf("%s?selectedLine=%d", updatedResmokeParsleyURL, tr.LineNum)
			}
		}

		return fmt.Sprintf("%s/test/%s/%d/%s?selectedLine=%d", parsleyURL, tr.TaskID, tr.Execution, tr.GetLogTestName(), tr.LineNum)
	default:
		if tr.RawLogURL != "" {
			// Some test results may have internal URLs that are
			// missing the root.
			if err := util.CheckURL(tr.RawLogURL); err != nil {
				return root + tr.RawLogURL
			}

			return tr.RawLogURL
		}

		return fmt.Sprintf("%s/test_log/%s/%d?test_name=%s&group_id=%s&text=true",
			root,
			url.PathEscape(tr.TaskID),
			tr.Execution,
			url.QueryEscape(tr.GetLogTestName()),
			url.QueryEscape(tr.GroupID),
		)
	}
}

// TaskTestResultsFailedSample represents a sample of failed test names from
// an Evergreen task run.
type TaskTestResultsFailedSample struct {
	TaskID                  string   `json:"task_id"`
	Execution               int      `json:"execution"`
	MatchingFailedTestNames []string `json:"matching_failed_test_names"`
	TotalFailedNames        int      `json:"total_failed_names"`
}

func GetTaskTestResults(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, filterOpts FilterOptions) (TaskTestResults, error) {
	svc, err := getService(env, taskOpts.ResultsService)
	if err != nil {
		return TaskTestResults{}, err
	}

	return svc.GetTaskTestResults(ctx, taskOpts, filterOpts)
}

func GetTaskTestResultsStats(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions) (TaskTestResultsStats, error) {
	svc, err := getService(env, taskOpts.ResultsService)
	if err != nil {
		return TaskTestResultsStats{}, err
	}

	return svc.GetTaskTestResultsStats(ctx, taskOpts)
}

func GetFailedTestSamples(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions, regexFilters []string) ([]TaskTestResultsFailedSample, error) {
	var servicesToTasks map[string][]TaskOptions
	for _, task := range taskOpts {
		if tasks, ok := servicesToTasks[task.ResultsService]; ok {
			servicesToTasks[task.ResultsService] = append(tasks, task)
		} else {
			servicesToTasks[task.ResultsService] = []TaskOptions{task}
		}
	}

	var allSamples []TaskTestResultsFailedSample
	for service, tasks := range servicesToTasks {
		svc, err := getService(env, service)
		if err != nil {
			return nil, err
		}

		samples, err := svc.GetFailedTestSamples(ctx, tasks, regexFilters)
		if err != nil {
			return nil, err
		}
		allSamples = append(allSamples, samples...)
	}

	return allSamples, nil
}

// TaskOptions represents the task-level information required to fetch test
// results from an Evergreen test run.
type TaskOptions struct {
	TaskID         string
	Execution      int
	DisplayTask    bool
	ResultsService string
}

// FilterOptions represents the filtering arguments for fetching test results.
type FilterOptions struct {
	TestName     string
	Statuses     []string
	GroupID      string
	SortBy       string
	SortOrderDSC bool
	BaseTaskID   string
	Limit        int
	// TODO: should this be a string?
	Page int
}

// Valid sort by keys.
const (
	SortByStart      = "start"
	SortByDuration   = "duration"
	SortByTestName   = "test_name"
	SortByStatus     = "status"
	SortByBaseStatus = "base_status"
)
