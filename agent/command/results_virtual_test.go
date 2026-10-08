package command

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/internal/testutil"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOptionsForVirtualTask(t *testing.T) {
	conf := &internal.TaskConfig{
		Task:    task.Task{Id: "runner", Execution: 0},
		WorkDir: "workdir",
	}
	info := testresult.TestResultsInfo{
		TaskID:    "virtual",
		Execution: 2,
	}

	opts := parseOptionsForVirtualTask(conf, info)

	assert.Equal(t, "workdir", opts.workDir)
	assert.Equal(t, "virtual", opts.taskID)
	assert.Equal(t, 2, opts.taskExecution)
}

func TestProcessVirtualTaskTestResultFiles(t *testing.T) {
	ctx := t.Context()
	comm := client.NewMock("url")
	conf := &internal.TaskConfig{
		Task: task.Task{
			Id:             "runner",
			Secret:         "secret",
			Project:        "project",
			Version:        "version",
			BuildVariant:   "bv",
			Execution:      0,
			Requester:      evergreen.PatchVersionRequester,
			TaskOutputInfo: agentutil.InitializeTaskOutput(t),
		},
		WorkDir: t.TempDir(),
	}
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, logger.Close()) })

	info := testresult.TestResultsInfo{
		Project:   "project",
		Version:   "version",
		Variant:   "bv",
		TaskID:    "virtual",
		TaskName:  "virtual_name",
		Execution: 0,
		Requester: evergreen.PatchVersionRequester,
	}
	require.NoError(t, utility.WriteJSONFile(filepath.Join(conf.WorkDir, "results.json"), nativeTestResults{Results: []nativeTestResult{
		{TestFile: "test1", Status: evergreen.TestSucceededStatus},
		{TestFile: "test2", Status: evergreen.TestFailedStatus, LogRaw: "some log"},
	}}))

	createdAt := time.Now().UTC().Round(time.Millisecond)
	stats, err := processVirtualTaskTestResultFiles(ctx, conf, logger, info, time.Now(), createdAt, []virtualTaskTestResultFile{
		{Type: virtualTestResultTypeNative, Files: []string{"results.json"}},
	})
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 2, stats.Stats.TotalCount)
	assert.Equal(t, 1, stats.Stats.FailedCount)
	assert.Equal(t, []string{"test2"}, stats.FailedSample)
	assert.True(t, createdAt.Equal(stats.CreatedAt))

	bucket, err := conf.Task.TaskOutputInfo.TestResults.GetBucket(ctx, conf.TaskOutput)
	require.NoError(t, err)
	r, err := bucket.Get(ctx, testresult.PartitionKey(createdAt, info.Project, info.ID()))
	require.NoError(t, err, "the test results should be uploaded to the expected partition")
	require.NoError(t, r.Close())
}

func TestParseVirtualTaskTestResultFileUnknownTypeShouldError(t *testing.T) {
	ctx := t.Context()
	comm := client.NewMock("url")
	conf := &internal.TaskConfig{
		Task:    task.Task{Id: "runner"},
		WorkDir: t.TempDir(),
	}
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, logger.Close()) })

	_, _, err = parseVirtualTaskTestResultFile(ctx, parseOptionsForTask(conf), logger, virtualTaskTestResultFile{
		Type:  "bogus",
		Files: []string{"results.json"},
	})
	assert.ErrorContains(t, err, "unrecognized test result type")
}
