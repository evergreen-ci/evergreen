package testresult

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestDbTaskTestResultsQuarantinedTestsBSONRoundTrip(t *testing.T) {
	original := DbTaskTestResults{
		ID:                    "task_results",
		QuarantinedTestsCount: 2,
		QuarantinedTests: []QuarantinedTest{
			{TestName: "test_0", DisplayTestName: "Display test 0"},
			{TestName: "test_1"},
		},
	}

	data, err := bson.Marshal(original)
	require.NoError(t, err)

	var raw bson.M
	require.NoError(t, bson.Unmarshal(data, &raw))
	assert.Contains(t, raw, "quarantined_tests_count")
	assert.Contains(t, raw, "quarantined_tests")
	rawTests, ok := raw["quarantined_tests"].(bson.A)
	require.True(t, ok)
	require.NotEmpty(t, rawTests)
	firstRawTest, ok := rawTests[0].(bson.M)
	require.True(t, ok)
	assert.Contains(t, firstRawTest, "test_name")
	assert.Contains(t, firstRawTest, "display_test_name")

	var roundTrip DbTaskTestResults
	require.NoError(t, bson.Unmarshal(data, &roundTrip))
	assert.Equal(t, original.QuarantinedTestsCount, roundTrip.QuarantinedTestsCount)
	assert.Equal(t, original.QuarantinedTests, roundTrip.QuarantinedTests)
}

func TestGetLogURL(t *testing.T) {
	evergreenBaseURL := "https://request.evergreen.example.com"
	parsleyURL := "https://parsley.mongodb.org"

	test1 := TestResult{
		TaskID:   "task1",
		TestName: "test1",
		LogURL:   "http://www.something.com/absolute",
		Status:   evergreen.TestSucceededStatus,
	}
	test2 := TestResult{
		TaskID:   "task1",
		TestName: "test2",
		Status:   evergreen.TestFailedStatus,
	}
	test3 := TestResult{
		TaskID:   "task1",
		TestName: "test3",
		Status:   evergreen.TestFailedStatus,
	}

	url := test1.GetLogURL(evergreenBaseURL, parsleyURL, evergreen.LogViewerRaw)
	assert.Equal(t, "https://request.evergreen.example.com/rest/v2/tasks/task1/build/TestLogs/test1?execution=0&print_time=true", url)
	url = test2.GetLogURL(evergreenBaseURL, parsleyURL, evergreen.LogViewerHTML)
	assert.Equal(t, "https://request.evergreen.example.com/test_log/task1/0?test_name=test2#L0", url)
	url = test3.GetLogURL(evergreenBaseURL, parsleyURL, evergreen.LogViewerParsley)
	assert.Equal(t, "https://parsley.mongodb.org/test/task1/0/test3?shareLine=0", url)
}
