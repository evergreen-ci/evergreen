package testresult

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

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
