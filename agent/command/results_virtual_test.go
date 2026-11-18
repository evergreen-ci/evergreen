package command

import (
	"os"
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

	testResultsCreatedAt := time.Now().UTC().Round(time.Millisecond)
	stats, err := ProcessVirtualTaskTestResultFiles(ctx, conf, logger, info, time.Now(), VirtualTaskTestResultsInput{
		Files:     []VirtualTaskTestResultFileGroup{{Type: virtualTestResultTypeNative, Files: []string{"results.json"}}},
		CreatedAt: testResultsCreatedAt,
	})
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 2, stats.Stats.TotalCount)
	assert.Equal(t, 1, stats.Stats.FailedCount)
	assert.Equal(t, []string{"test2"}, stats.FailedSample)
	assert.True(t, testResultsCreatedAt.Equal(stats.CreatedAt))

	bucket, err := conf.Task.TaskOutputInfo.TestResults.GetBucket(ctx, conf.TaskOutput)
	require.NoError(t, err)
	r, err := bucket.Get(ctx, testresult.PartitionKey(testResultsCreatedAt, info.Project, info.ID()))
	require.NoError(t, err, "the test results should be uploaded to the expected partition")
	require.NoError(t, r.Close())
}

func TestProcessVirtualTaskTestResultFilesWithLogDirShouldUploadReferencedLogs(t *testing.T) {
	ctx := t.Context()
	comm := client.NewMock("url")
	conf := &internal.TaskConfig{
		Task: task.Task{
			Id:             "runner",
			Project:        "project",
			TaskOutputInfo: agentutil.InitializeTaskOutput(t),
		},
		WorkDir: t.TempDir(),
	}
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, logger.Close()) })

	info := testresult.TestResultsInfo{
		Project:   "project",
		TaskID:    "virtual",
		Execution: 0,
	}
	logDir := filepath.Join(conf.WorkDir, "virtual_logs")
	require.NoError(t, os.MkdirAll(filepath.Join(logDir, "job0"), 0777))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "job0", "a.log"), []byte("a log\n"), 0777))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "job0", "b.log"), []byte("b log\n"), 0777))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "job0", "global.log"), []byte("global log\n"), 0777))
	globalLog := "job0/global.log"
	require.NoError(t, utility.WriteJSONFile(filepath.Join(conf.WorkDir, "results.json"), nativeTestResults{Results: []nativeTestResult{
		{TestFile: "a", Status: evergreen.TestSucceededStatus, LogInfo: &testresult.TestLogInfo{LogName: "job0/a.log", LogsToMerge: []*string{&globalLog}}},
		{TestFile: "b", Status: evergreen.TestSucceededStatus, LogInfo: &testresult.TestLogInfo{LogName: "job0/b.log", LogsToMerge: []*string{&globalLog}}},
	}}))

	_, err = ProcessVirtualTaskTestResultFiles(ctx, conf, logger, info, time.Now(), VirtualTaskTestResultsInput{
		Files:  []VirtualTaskTestResultFileGroup{{Type: virtualTestResultTypeNative, Files: []string{"results.json"}}},
		LogDir: "virtual_logs",
	})
	require.NoError(t, err)

	vTask := task.Task{Id: info.TaskID, Project: info.Project, TaskOutputInfo: conf.Task.TaskOutputInfo}
	for logPath, expected := range map[string]string{
		"job0/a.log":      "a log",
		"job0/b.log":      "b log",
		"job0/global.log": "global log",
	} {
		it, err := vTask.GetTestLogs(ctx, task.TestLogGetOptions{LogPaths: []string{logPath}})
		require.NoError(t, err)
		var lines []string
		for it.Next() {
			lines = append(lines, it.Item().Data)
		}
		assert.NoError(t, it.Close())
		require.NoError(t, it.Err())
		assert.Equal(t, []string{expected}, lines, "log '%s' should be uploaded under the virtual task", logPath)
	}
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

	_, _, err = parseVirtualTaskTestResultFileGroup(ctx, parseOptionsForTask(conf), logger, VirtualTaskTestResultFileGroup{
		Type:  "bogus",
		Files: []string{"results.json"},
	})
	assert.ErrorContains(t, err, "unrecognized test result type")
}
