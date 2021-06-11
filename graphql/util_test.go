package graphql

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
)

func init() {
	testutil.Setup()
}

func TestFilterSortAndPaginateCedarTestResults(t *testing.T) {
	var testResults = []apimodels.CedarTestResult{
		apimodels.CedarTestResult{
			TestName: "A test",
			Status:   "Pass",
			Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
			End:      time.Date(1996, time.August, 31, 12, 5, 12, 0, time.UTC),
		},
		apimodels.CedarTestResult{
			TestName:        "B test",
			DisplayTestName: "Display",
			Status:          "Fail",
			Start:           time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
			End:             time.Date(1996, time.August, 31, 12, 5, 16, 0, time.UTC),
		},
		apimodels.CedarTestResult{
			TestName: "C test",
			Status:   "Fail",
			Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
			End:      time.Date(1996, time.August, 31, 12, 5, 15, 0, time.UTC),
		},
		apimodels.CedarTestResult{
			TestName: "D test",
			Status:   "Pass",
			Start:    time.Date(1996, time.August, 31, 12, 5, 10, 0, time.UTC),
			End:      time.Date(1996, time.August, 31, 12, 5, 11, 0, time.UTC),
		},
	}

	for _, test := range []struct {
		name            string
		testName        string
		statuses        []string
		sortBy          string
		sortDir         int
		pageParam       int
		limitParam      int
		expectedResults []apimodels.CedarTestResult
		expectedCount   int
	}{
		{
			name:            "NoParams",
			expectedResults: testResults,
			expectedCount:   len(testResults),
		},
		{
			name:            "TestName",
			testName:        "A test",
			expectedResults: testResults[0:1],
			expectedCount:   1,
		},
		{
			name:            "DisplayTestName",
			testName:        "Display",
			expectedResults: testResults[1:2],
			expectedCount:   1,
		},
		{
			name:            "StatusFilter",
			statuses:        []string{"Fail"},
			expectedResults: testResults[1:3],
			expectedCount:   2,
		},
		{
			name:   "SortByDuration",
			sortBy: "duration",
			expectedResults: []apimodels.CedarTestResult{
				testResults[1],
				testResults[2],
				testResults[0],
				testResults[3],
			},
			expectedCount: 4,
		},
		{
			name:   "SortByTestName",
			sortBy: testresult.TestFileKey,
			expectedResults: []apimodels.CedarTestResult{
				testResults[3],
				testResults[2],
				testResults[1],
				testResults[0],
			},
			expectedCount: 4,
		},
		{
			name:    "SortByStatus",
			sortBy:  testresult.StatusKey,
			sortDir: 1,
			expectedResults: []apimodels.CedarTestResult{
				testResults[1],
				testResults[2],
				testResults[0],
				testResults[3],
			},
			expectedCount: 4,
		},
		{
			name:            "Limit",
			limitParam:      3,
			expectedResults: testResults[0:3],
			expectedCount:   4,
		},
		{
			name:            "LimitAndPage",
			limitParam:      3,
			pageParam:       1,
			expectedResults: testResults[3:],
			expectedCount:   4,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			results, count := FilterSortAndPaginateCedarTestResults(
				testResults,
				test.testName,
				test.statuses,
				test.sortBy,
				"",
				test.sortDir,
				test.pageParam,
				test.limitParam,
			)
			assert.Equal(t, test.expectedResults, results)
			assert.Equal(t, test.expectedCount, count)
		})
	}
}

func TestAddDisplayTasksToPatchReq(t *testing.T) {
	p := model.Project{
		BuildVariants: []model.BuildVariant{
			{
				Name: "bv",
				DisplayTasks: []patch.DisplayTask{
					{Name: "dt1", ExecTasks: []string{"1", "2"}},
					{Name: "dt2", ExecTasks: []string{"3", "4"}},
				}},
		},
	}
	req := PatchVariantsTasksRequest{
		VariantsTasks: []patch.VariantTasks{
			{Variant: "bv", Tasks: []string{"t1", "dt1", "dt2"}},
		},
	}
	addDisplayTasksToPatchReq(&req, p)
	assert.Len(t, req.VariantsTasks[0].Tasks, 1)
	assert.Equal(t, "t1", req.VariantsTasks[0].Tasks[0])
	assert.Len(t, req.VariantsTasks[0].DisplayTasks, 2)
}
