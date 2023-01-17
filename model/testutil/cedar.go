package testutil

import (
	"encoding/json"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// SetMockCedarTestResults adds test results to the response sequence of the
// given mock Cedar, taking care of converting the results to the correct API
// model.
func SetMockCedarTestResults(cedarHandler *mock.CedarHandler, results []task.TestResult, filteredCount *int) error {
	cedarResults := apimodels.CedarTestResults{
		Stats: apimodels.CedarTestResultsStats{
			TotalCount:    len(results),
			FilteredCount: filteredCount,
		},
		Results: make([]apimodels.CedarTestResult, len(results)),
	}
	for i, result := range results {
		cedarResults.Results[i] = apimodels.CedarTestResult{
			TaskID:          result.TaskID,
			Execution:       result.Execution,
			TestName:        result.TestFile,
			DisplayTestName: result.DisplayTestName,
			GroupID:         result.GroupID,
			LogTestName:     result.LogTestName,
			LogURL:          result.URL,
			RawLogURL:       result.URLRaw,
			LineNum:         result.LineNum,
			Start:           time.Unix(int64(result.StartTime), 0),
			End:             time.Unix(int64(result.EndTime), 0),
			Status:          result.Status,
		}
		if result.Status == evergreen.TestFailedStatus {
			cedarResults.Stats.FailedCount++
		}
	}

	response, err := json.Marshal(&cedarResults)
	if err != nil {
		return errors.Wrap(err, "marshalling Cedar test results into JSON")
	}
	cedarHandler.Responses = append(cedarHandler.Responses, response)

	return nil
}
