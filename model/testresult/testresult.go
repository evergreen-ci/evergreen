package testresult

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
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

// TestResultsInfo describes information unique to a single task execution.
type TestResultsInfo struct {
	Project         string `bson:"project" json:"project" yaml:"project"`
	Version         string `bson:"version" json:"version" yaml:"version"`
	Variant         string `bson:"variant" json:"variant" yaml:"variant"`
	TaskName        string `bson:"task_name" json:"task_name" yaml:"task_name"`
	DisplayTaskName string `bson:"display_task_name,omitempty" json:"display_task_name" yaml:"display_task_name"`
	TaskID          string `bson:"task_id" json:"task_id" yaml:"task_id"`
	DisplayTaskID   string `bson:"display_task_id,omitempty" json:"display_task_id" yaml:"display_task_id"`
	Execution       int    `bson:"execution" json:"execution" yaml:"execution"`
	Requester       string `bson:"request_type" json:"request_type" yaml:"request_type"`
	Mainline        bool   `bson:"mainline" json:"mainline" yaml:"mainline"`
	Schema          int    `bson:"schema" json:"schema" yaml:"schema"`
}

// TestResult represents a single test result from an Evergreen task run.
type TestResult struct {
	TaskID          string       `json:"task_id" bson:"task_id"`
	Execution       int          `json:"execution" bson:"execution"`
	TestName        string       `json:"test_name" bson:"test_name"`
	GroupID         string       `json:"group_id" bson:"group_id"`
	DisplayTestName string       `json:"display_test_name" bson:"display_test_name"`
	Status          string       `json:"status" bson:"status"`
	BaseStatus      string       `json:"base_status" bson:"base_status"`
	LogInfo         *TestLogInfo `json:"log_info" bson:"log_info"`
	TestStartTime   time.Time    `json:"test_start_time" bson:"test_start_time"`
	TaskCreateTime  time.Time    `json:"task_create_time" bson:"task_create_time"`
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
	LogName       string    `json:"log_name" bson:"log_name" parquet:"name=logname"`
	LogsToMerge   []*string `json:"logs_to_merge" bson:"logs_to_merge" parquet:"name=logstomerge"`
	LineNum       int32     `json:"line_num" bson:"line_num" parquet:"name=linenum"`
	RenderingType *string   `json:"rendering_type" bson:"rendering_type" parquet:"name=renderingtype"`
	Version       int32     `json:"version" bson:"version"`
	// TODO: DEVPROD-19170 remove these fields
	// The following are deprecated fields used for tasks that wrote test results
	// through Cedar. These tasks wrote using custom parquet tags that included underscores.
	// Tasks that wrote test results directly to Evergreen will use the tags above that don't
	// contain any underscores, since when the evergreen test result service first launched
	// the fields above didn't contain parquet tags and hence defaulted to tag names with no underscores
	// because of how the parquet-go library automatically resolves tag names. Both fields are now
	// required to support backwards compatibility.
	LogNameCedar       string    `parquet:"name=log_name"`
	LogsToMergeCedar   []*string `parquet:"name=logs_to_merge"`
	LineNumCedar       int32     `parquet:"name=line_num"`
	RenderingTypeCedar *string   `parquet:"name=rendering_type"`
}

// ParquetTestResults describes a set of test results from a task execution to
// be stored in Apache Parquet format.
type ParquetTestResults struct {
	Version         string              `parquet:"name=version"`
	Variant         string              `parquet:"name=variant"`
	TaskName        string              `parquet:"name=task_name"`
	DisplayTaskName *string             `parquet:"name=display_task_name"`
	TaskID          string              `parquet:"name=task_id"`
	DisplayTaskID   *string             `parquet:"name=display_task_id"`
	Execution       int32               `parquet:"name=execution"`
	Requester       string              `parquet:"name=request_type"`
	CreatedAt       time.Time           `parquet:"name=created_at, timeunit=MILLIS"`
	Results         []ParquetTestResult `parquet:"name=results"`
}

// ParquetTestResult describes a single test result to be stored in Apache
// Parquet file format.
type ParquetTestResult struct {
	TestName        string       `parquet:"name=test_name"`
	DisplayTestName *string      `parquet:"name=display_test_name"`
	GroupID         *string      `parquet:"name=group_id"`
	Status          string       `parquet:"name=status"`
	LogInfo         *TestLogInfo `parquet:"name=log_info"`
	TaskCreateTime  time.Time    `parquet:"name=task_create_time, timeunit=MILLIS"`
	TestStartTime   time.Time    `parquet:"name=test_start_time, timeunit=MILLIS"`
	TestEndTime     time.Time    `parquet:"name=test_end_time, timeunit=MILLIS"`

	// Legacy test log fields.
	LogTestName *string `parquet:"name=log_test_name"`
	LogURL      *string `parquet:"name=log_url"`
	RawLogURL   *string `parquet:"name=raw_log_url"`
	LineNum     *int32  `parquet:"name=line_num"`
}

func (r ParquetTestResults) ConvertToTestResultSlice() []TestResult {
	results := make([]TestResult, len(r.Results))
	for i := range r.Results {
		results[i] = TestResult{
			TaskID:          r.TaskID,
			Execution:       int(r.Execution),
			TestName:        r.Results[i].TestName,
			DisplayTestName: utility.FromStringPtr(r.Results[i].DisplayTestName),
			GroupID:         utility.FromStringPtr(r.Results[i].GroupID),
			Status:          r.Results[i].Status,
			LogInfo:         r.Results[i].LogInfo,
			LogTestName:     utility.FromStringPtr(r.Results[i].LogTestName),
			LogURL:          utility.FromStringPtr(r.Results[i].LogURL),
			RawLogURL:       utility.FromStringPtr(r.Results[i].RawLogURL),
			LineNum:         int(utility.FromInt32Ptr(r.Results[i].LineNum)),
			TestStartTime:   r.Results[i].TestStartTime,
			TaskCreateTime:  r.Results[i].TaskCreateTime,
			TestEndTime:     r.Results[i].TestEndTime,
		}
	}

	return results
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
	root := env.Settings().Api.URL
	parsleyURL := env.Settings().Ui.ParsleyUrl
	deprecatedLogkeeperURLs := []string{"https://logkeeper.mongodb.org", "https://logkeeper2.build.10gen.cc"}

	switch viewer {
	case evergreen.LogViewerHTML:
		// Return an empty string for logkeeper URLS.
		if tr.LogURL != "" {
			for _, url := range deprecatedLogkeeperURLs {
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

		return fmt.Sprintf("%s/test_log/%s/%d?test_name=%s#L%d",
			root,
			url.PathEscape(tr.TaskID),
			tr.Execution,
			url.QueryEscape(tr.GetLogTestName()),
			tr.getLineNum(),
		)
	case evergreen.LogViewerParsley:
		if parsleyURL == "" {
			return ""
		}

		for _, url := range deprecatedLogkeeperURLs {
			if strings.Contains(tr.LogURL, url) {
				updatedResmokeParsleyURL := strings.Replace(tr.LogURL, fmt.Sprintf("%s/build", url), parsleyURL+"/resmoke", 1)
				return fmt.Sprintf("%s?shareLine=%d", updatedResmokeParsleyURL, tr.getLineNum())
			}
		}

		return fmt.Sprintf("%s/test/%s/%d/%s?shareLine=%d", parsleyURL, url.PathEscape(tr.TaskID), tr.Execution, url.QueryEscape(tr.TestName), tr.getLineNum())
	default:
		if tr.RawLogURL != "" {
			// Some test results may have internal URLs that are
			// missing the root.
			if err := util.CheckURL(tr.RawLogURL); err != nil {
				return root + tr.RawLogURL
			}

			return tr.RawLogURL
		}

		printTime := true
		var logsToMerge string
		if tr.LogInfo != nil {
			if utility.FromStringPtr(tr.LogInfo.RenderingType) == evergreen.ResmokeRenderingType || utility.FromStringPtr(tr.LogInfo.RenderingTypeCedar) == evergreen.ResmokeRenderingType {
				printTime = false
			}

			logPathsToMerge := tr.LogInfo.LogsToMerge
			if len(logPathsToMerge) == 0 {
				logPathsToMerge = tr.LogInfo.LogsToMergeCedar
			}
			for _, logPath := range logPathsToMerge {
				logsToMerge += fmt.Sprintf("&logs_to_merge=%s", url.QueryEscape(*logPath))
			}
		}

		return fmt.Sprintf("%s/rest/v2/tasks/%s/build/TestLogs/%s?execution=%d&print_time=%v%s",
			root,
			url.PathEscape(tr.TaskID),
			url.QueryEscape(tr.GetLogTestName()),
			tr.Execution,
			printTime,
			logsToMerge,
		)
	}
}

// GetLogTestName returns the name of the test in the logging backend. This is
// used for test logs where the name of the test in the logging service may
// differ from that in the test results service.
func (tr TestResult) GetLogTestName() string {
	if tr.LogInfo != nil {
		if tr.LogInfo.LogName != "" {
			return tr.LogInfo.LogName
		}
		if tr.LogInfo.LogNameCedar != "" {
			return tr.LogInfo.LogNameCedar
		}
	}
	if tr.LogTestName != "" {
		return tr.LogTestName
	}

	return tr.TestName
}

func (tr TestResult) getLineNum() int {
	if tr.LogInfo != nil {
		if int(tr.LogInfo.LineNumCedar) > 0 {
			return int(tr.LogInfo.LineNumCedar)
		}
		return int(tr.LogInfo.LineNum)
	}

	return tr.LineNum
}

// TaskTestResultsFailedSample represents a sample of failed test names from
// an Evergreen task run.
type TaskTestResultsFailedSample struct {
	TaskID                  string   `json:"task_id"`
	Execution               int      `json:"execution"`
	MatchingFailedTestNames []string `json:"matching_failed_test_names"`
	TotalFailedNames        int      `json:"total_failed_names"`
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
