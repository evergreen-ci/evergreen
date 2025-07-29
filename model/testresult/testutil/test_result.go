package testutil

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
)

func MakeAppendTestResultMetadataReq(ctx context.Context, results []testresult.TestResult, id string) (context.Context, []string, int, int, testresult.DbTaskTestResults) {
	failedCount := 0
	failedTests := []string{}
	for _, result := range results {
		if result.Status == evergreen.TestFailedStatus {
			failedCount++
			failedTests = append(failedTests, result.GetDisplayTestName())
		}
	}
	tr := testresult.DbTaskTestResults{
		ID: id,
		Info: testresult.TestResultsInfo{
			TaskID:    results[0].TaskID,
			Execution: results[0].Execution,
		},
		Results: results,
	}
	return ctx, failedTests, failedCount, len(results), tr
}
