package task

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsContextError(t *testing.T) {
	for name, tc := range map[string]struct {
		err      error
		expected bool
	}{
		"DeadlineExceeded":           {context.DeadlineExceeded, true},
		"Canceled":                   {context.Canceled, true},
		"WrappedDeadlineExceeded":    {errors.Wrap(context.DeadlineExceeded, "wrap"), true},
		"WrappedCanceled":            {errors.Wrap(context.Canceled, "wrap"), true},
		"MessageContainsDeadline":    {errors.New("operation error S3: CopyObject, context deadline exceeded"), true},
		"MessageContainsCanceled":    {errors.New("request canceled"), true},
		"Nil":                        {nil, false},
		"OtherError":                 {errors.New("some other error"), false},
		"MessageContainsDeadlineSub": {errors.New("context deadline exceeded"), true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isContextError(tc.err))
		})
	}
}

func TestRevertBucketConfigToSource(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	t.Run("NilTaskOutputInfoIsNoop", func(t *testing.T) {
		tsk := &Task{Id: "task-nil-output"}
		require.NoError(t, db.Insert(t.Context(), Collection, tsk))

		sourceCfg := evergreen.BucketConfig{Name: "source-bucket", Type: evergreen.BucketTypeLocal}
		require.NoError(t, tsk.RevertBucketConfigToSource(t.Context(), sourceCfg))

		dbTask, err := FindOneId(t.Context(), tsk.Id)
		require.NoError(t, err)
		require.NotNil(t, dbTask)
		assert.Nil(t, dbTask.TaskOutputInfo)
	})
}

func TestRevertBucketConfigToSourceIfLogsExist(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	makeBucketCfg := func(t *testing.T) evergreen.BucketConfig {
		return evergreen.BucketConfig{
			Name: t.TempDir(),
			Type: evergreen.BucketTypeLocal,
		}
	}

	makeTask := func(t *testing.T, id string, bucketCfg evergreen.BucketConfig) *Task {
		tsk := &Task{
			Id:      id,
			Project: "proj",
			Status:  evergreen.TaskFailed,
			TaskOutputInfo: &TaskOutput{
				TaskLogs: TaskLogOutput{Version: 1, BucketConfig: bucketCfg},
				TestLogs: TestLogOutput{Version: 1, BucketConfig: bucketCfg},
			},
		}
		require.NoError(t, db.Insert(t.Context(), Collection, tsk))
		return tsk
	}

	writeLogChunk := func(t *testing.T, bucketPath string, tsk *Task) {
		bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: bucketPath, UseSlash: true})
		require.NoError(t, err)
		output, ok := tsk.GetTaskOutputSafe()
		require.True(t, ok)
		svc := log.NewLogServiceV0(bucket)
		logName := getLogName(*tsk, TaskLogTypeTask, output.TaskLogs.ID())
		_, _, err = svc.Append(t.Context(), logName, 1, []log.LogLine{
			{Priority: level.Info, Timestamp: time.Now().UnixNano(), Data: "test log line"},
		})
		require.NoError(t, err)
	}

	t.Run("FilesExistInSourceBucketRevertsConfig", func(t *testing.T) {
		sourceCfg := makeBucketCfg(t)
		failedCfg := makeBucketCfg(t)
		tsk := makeTask(t, "task-files-exist", failedCfg)
		writeLogChunk(t, sourceCfg.Name, tsk)

		reverted, err := tsk.RevertBucketConfigToSourceIfLogsExist(t.Context(), sourceCfg)
		require.NoError(t, err)
		assert.True(t, reverted)

		dbTask, err := FindOneId(t.Context(), tsk.Id)
		require.NoError(t, err)
		require.NotNil(t, dbTask)
		assert.Equal(t, sourceCfg.Name, dbTask.TaskOutputInfo.TaskLogs.BucketConfig.Name)
		assert.Equal(t, sourceCfg.Name, dbTask.TaskOutputInfo.TestLogs.BucketConfig.Name)
	})

	t.Run("NoFilesInSourceBucketDoesNotRevert", func(t *testing.T) {
		sourceCfg := makeBucketCfg(t)
		failedCfg := makeBucketCfg(t)
		tsk := makeTask(t, "task-no-files", failedCfg)

		reverted, err := tsk.RevertBucketConfigToSourceIfLogsExist(t.Context(), sourceCfg)
		require.NoError(t, err)
		assert.False(t, reverted)

		dbTask, err := FindOneId(t.Context(), tsk.Id)
		require.NoError(t, err)
		require.NotNil(t, dbTask)
		assert.Equal(t, failedCfg.Name, dbTask.TaskOutputInfo.TaskLogs.BucketConfig.Name)
		assert.Equal(t, failedCfg.Name, dbTask.TaskOutputInfo.TestLogs.BucketConfig.Name)
	})

	t.Run("BucketCheckErrorReturnsFalseAndError", func(t *testing.T) {
		invalidCfg := evergreen.BucketConfig{Type: "invalid-bucket-type", Name: "irrelevant"}
		failedCfg := makeBucketCfg(t)
		tsk := makeTask(t, "task-bucket-err", failedCfg)

		reverted, err := tsk.RevertBucketConfigToSourceIfLogsExist(t.Context(), invalidCfg)
		require.Error(t, err)
		assert.False(t, reverted)

		dbTask, err := FindOneId(t.Context(), tsk.Id)
		require.NoError(t, err)
		require.NotNil(t, dbTask)
		assert.Equal(t, failedCfg.Name, dbTask.TaskOutputInfo.TaskLogs.BucketConfig.Name)
	})

	t.Run("DisplayOnlyTaskIsNoop", func(t *testing.T) {
		sourceCfg := makeBucketCfg(t)
		tsk := &Task{Id: "task-display-only", DisplayOnly: true}
		require.NoError(t, db.Insert(t.Context(), Collection, tsk))

		reverted, err := tsk.RevertBucketConfigToSourceIfLogsExist(t.Context(), sourceCfg)
		require.NoError(t, err)
		assert.False(t, reverted)
	})
}

func TestAllObjectsExistInBucket(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	bucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: tmpDir, UseSlash: true})
	require.NoError(t, err)

	require.NoError(t, bucket.Put(ctx, "key1", strings.NewReader("data1")))
	require.NoError(t, bucket.Put(ctx, "key2", strings.NewReader("data2")))

	for name, tc := range map[string]struct {
		keys     []string
		expected bool
	}{
		"AllExist":     {[]string{"key1", "key2"}, true},
		"OneExists":    {[]string{"key1"}, true},
		"EmptyKeys":    {[]string{}, true},
		"MissingKey":   {[]string{"key1", "key2", "key3"}, false},
		"AllMissing":   {[]string{"key3", "key4"}, false},
		"FirstMissing": {[]string{"key3", "key1"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, allObjectsExistInBucket(ctx, bucket, tc.keys))
		})
	}
}
