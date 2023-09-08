package testresult

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// TaskTestResults represents a set of test results. These results may come
// from a single task run or multiple related task runs, such as execution
// tasks within a display task.
type TaskTestResults struct {
	Stats   TaskTestResultsStats `json:"stats"`
	Results []TestResult         `json:"results"`
}

// TaskTestResultsStats represents basic statistics of a set of test results.
type TaskTestResultsStats struct {
	TotalCount    int  `json:"total_count" bson:"total_count"`
	FailedCount   int  `json:"failed_count" bson:"failed_count"`
	FilteredCount *int `json:"filtered_count" bson:"-"`
}

// TestResult represents a single test result from an Evergreen task run.
type TestResult struct {
	TaskID          string       `json:"task_id" bson:"task_id"`
	Execution       int          `json:"execution" bson:"execution"`
	TestName        string       `json:"test_name" bson:"test_name"`
	DisplayTestName string       `json:"display_test_name" bson:"display_test_name"`
	GroupID         string       `json:"group_id" bson:"group_id"`
	Status          string       `json:"status" bson:"status"`
	BaseStatus      string       `json:"base_status" bson:"base_status"`
	LogInfo         *TestLogInfo `json:"log_info" bson:"log_info"`
	TestStartTime   time.Time    `json:"test_start_time" bson:"test_start_time"`
	TestEndTime     time.Time    `json:"test_end_time" bson:"test_end_time"`

	// Legacy test log fields.
	LogTestName string `json:"log_test_name" bson:"log_test_name"`
	LogURL      string `json:"log_url" bson:"log_url"`
	RawLogURL   string `json:"raw_log_url" bson:"raw_log_url"`
	LineNum     int    `json:"line_num" bson:"line_num"`
}

// TestLogInfo describes a metadata for a test result's log stored using
// Evergreen logging.
type TestLogInfo struct {
	LogName       string    `json:"log_name" bson:"log_name"`
	LogsToMerge   []*string `json:"logs_to_merge" bson:"logs_to_merge"`
	LineNum       int32     `json:"line_num" bson:"line_num"`
	RenderingType *string   `json:"rendering_type" bson:"rendering_type"`
	Version       int32     `json:"version" bson:"version"`
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

// Duration returns the duration of the test.
func (tr TestResult) Duration() time.Duration {
	return tr.TestEndTime.Sub(tr.TestStartTime)
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
				return fmt.Sprintf("%s?shareLine=%d", updatedResmokeParsleyURL, tr.LineNum)
			}
		}

		return fmt.Sprintf("%s/test/%s/%d/%s?shareLine=%d", parsleyURL, url.PathEscape(tr.TaskID), tr.Execution, url.QueryEscape(tr.TestName), tr.LineNum)
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

// GetMergedTaskTestResults returns the merged test results filtered, sorted,
// and paginated as specified by the optional filter options for the given
// tasks. This function requires that all specified tasks have persisted their
// results using the same test results service.
func GetMergedTaskTestResults(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions, filterOpts *FilterOptions) (TaskTestResults, error) {
	if len(taskOpts) == 0 {
		return TaskTestResults{}, errors.New("must specify task options")
	}

	svc, err := getServiceImpl(env, taskOpts[0].ResultsService)
	if err != nil {
		return TaskTestResults{}, err
	}

	return svc.GetMergedTaskTestResults(ctx, taskOpts, filterOpts)
}

// GetMergedTaskTestResultsStats returns the aggregated statistics of the test
// results for the given tasks.
func GetMergedTaskTestResultsStats(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions) (TaskTestResultsStats, error) {
	if len(taskOpts) == 0 {
		return TaskTestResultsStats{}, errors.New("must specify task options")
	}

	var allStats TaskTestResultsStats
	for service, tasks := range groupTasksByService(taskOpts) {
		svc, err := getServiceImpl(env, service)
		if err != nil {
			return TaskTestResultsStats{}, err
		}

		stats, err := svc.GetMergedTaskTestResultsStats(ctx, tasks)
		if err != nil {
			return TaskTestResultsStats{}, err
		}

		allStats.TotalCount += stats.TotalCount
		allStats.FailedCount += stats.FailedCount
	}

	return allStats, nil
}

// GetMergedFailedTestSample returns a sample of test names (up to 10) that
// failed in the given tasks.
func GetMergedFailedTestSample(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions) ([]string, error) {
	if len(taskOpts) == 0 {
		return nil, errors.New("must specify task options")
	}

	var allSamples []string
	for service, tasks := range groupTasksByService(taskOpts) {
		svc, err := getServiceImpl(env, service)
		if err != nil {
			return nil, err
		}

		samples, err := svc.GetMergedFailedTestSample(ctx, tasks)
		if err != nil {
			return nil, err
		}

		allSamples = append(allSamples, samples...)
		if len(allSamples) >= 10 {
			allSamples = allSamples[0:10]
			break
		}
	}

	return allSamples, nil
}

// GetFailedTestSamples returns failed test samples filtered as specified by
// the optional regex filters for each task specified.
func GetFailedTestSamples(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions, regexFilters []string) ([]TaskTestResultsFailedSample, error) {
	if len(taskOpts) == 0 {
		return nil, errors.New("must specify task options")
	}

	var allSamples []TaskTestResultsFailedSample
	for service, tasks := range groupTasksByService(taskOpts) {
		svc, err := getServiceImpl(env, service)
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

func groupTasksByService(taskOpts []TaskOptions) map[string][]TaskOptions {
	servicesToTasks := map[string][]TaskOptions{}
	for _, task := range taskOpts {
		servicesToTasks[task.ResultsService] = append(servicesToTasks[task.ResultsService], task)
	}

	return servicesToTasks
}

// TaskOptions represents the task-level information required to fetch test
// results from an Evergreen test run.
type TaskOptions struct {
	TaskID         string
	Execution      int
	ResultsService string
}

// SortBy describes the properties by which to sort a set of test results.
type SortBy struct {
	Key      string
	OrderDSC bool
}

// Valid sort by keys.
const (
	SortByStartKey      = "start"
	SortByDurationKey   = "duration"
	SortByTestNameKey   = "test_name"
	SortByStatusKey     = "status"
	SortByBaseStatusKey = "base_status"
)

// FilterOptions represents the filtering arguments for fetching test results.
type FilterOptions struct {
	TestName            string
	ExcludeDisplayNames bool
	Statuses            []string
	GroupID             string
	Sort                []SortBy
	Limit               int
	Page                int
	BaseTasks           []TaskOptions
}
