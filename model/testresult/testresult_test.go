package testresult

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := testutil.TestConfig()
	require.NoError(t, db.Clear(evergreen.ConfigCollection))
	settings.Ui.ParsleyUrl = "https://parsley.mongodb.org"
	require.NoError(t, evergreen.UpdateConfig(ctx, settings))

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

	url := test1.GetLogURL(evergreen.GetEnvironment(), evergreen.LogViewerRaw)
	assert.Equal(t, url, "https://localhost:8443/rest/v2/tasks/task1/build/TestLogs/test1?execution=0&print_time=true")
	url = test2.GetLogURL(evergreen.GetEnvironment(), evergreen.LogViewerHTML)
	assert.Equal(t, url, "https://localhost:8443/test_log/task1/0?test_name=test2#L0")
	url = test3.GetLogURL(evergreen.GetEnvironment(), evergreen.LogViewerParsley)
	assert.Equal(t, url, "https://localhost:4173/test/task1/0/test3?shareLine=0")
}
