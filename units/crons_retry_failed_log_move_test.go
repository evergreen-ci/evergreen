package units

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopulateRetryFailedLogMoveJobs(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)

	const failedBucket = "failed-task-logs"
	const sourceBucket = "regular-task-logs"

	retryCandidate := func(id, project string, finish time.Time) task.Task {
		return task.Task{
			Id:          id,
			Status:      evergreen.TaskFailed,
			FinishTime:  finish,
			DisplayOnly: false,
			Project:     project,
			TaskOutputInfo: &task.TaskOutput{
				TaskLogs: task.TaskLogOutput{
					Version:      task.TestResultServiceEvergreen,
					BucketConfig: evergreen.BucketConfig{Name: sourceBucket, Type: evergreen.BucketTypeS3},
				},
			},
		}
	}

	jobID := func(taskID string) string {
		ts := utility.RoundPartOfMinute(0).Format(TSFormat)
		return fmt.Sprintf("%s.%s.%s", moveLogsToFailedBucketJobName, taskID, ts)
	}

	setup := func(t *testing.T) *mock.Environment {
		require.NoError(t, db.ClearCollections(task.Collection))
		t.Cleanup(func() { assert.NoError(t, db.ClearCollections(task.Collection)) })
		env := &mock.Environment{}
		require.NoError(t, env.Configure(ctx))
		return env
	}

	run := func(env *mock.Environment) error {
		return PopulateRetryFailedLogMoveJobs(env)(ctx, env.RemoteQueue())
	}

	t.Run("EnqueuesWeeklyRetryJobForEvergreenFailedTask", func(t *testing.T) {
		env := setup(t)
		env.EvergreenSettings.Buckets.LogBucketFailedTasks = evergreen.BucketConfig{
			Name: failedBucket, Type: evergreen.BucketTypeS3,
		}
		tsk := retryCandidate("evergreen-retry-1", tempRetryFailedLogMoveProject, time.Now())
		require.NoError(t, tsk.Insert(ctx))
		require.NoError(t, run(env))

		j, ok := env.RemoteQueue().Get(ctx, jobID(tsk.Id))
		require.True(t, ok)
		moveJob := j.(*moveLogsToFailedBucketJob)
		assert.Equal(t, tsk.Id, moveJob.TaskID)
		assert.Equal(t, MoveLogsTriggerWeeklyRetry, moveJob.Trigger)
		assert.Equal(t, sourceBucket, moveJob.SourceBucketCfg.Name)
	})

	t.Run("SkipsFailedTaskInOtherProject", func(t *testing.T) {
		env := setup(t)
		env.EvergreenSettings.Buckets.LogBucketFailedTasks = evergreen.BucketConfig{
			Name: failedBucket, Type: evergreen.BucketTypeS3,
		}
		tsk := retryCandidate("other-proj-task", "some-other-project", time.Now())
		require.NoError(t, tsk.Insert(ctx))
		require.NoError(t, run(env))

		_, ok := env.RemoteQueue().Get(ctx, jobID(tsk.Id))
		assert.False(t, ok)
	})

	t.Run("SkipsTaskWhoseLogsAreAlreadyInFailedBucket", func(t *testing.T) {
		env := setup(t)
		env.EvergreenSettings.Buckets.LogBucketFailedTasks = evergreen.BucketConfig{
			Name: failedBucket, Type: evergreen.BucketTypeS3,
		}
		tsk := retryCandidate("already-moved", tempRetryFailedLogMoveProject, time.Now())
		tsk.TaskOutputInfo.TaskLogs.BucketConfig = evergreen.BucketConfig{Name: failedBucket, Type: evergreen.BucketTypeS3}
		require.NoError(t, tsk.Insert(ctx))
		require.NoError(t, run(env))

		_, ok := env.RemoteQueue().Get(ctx, jobID(tsk.Id))
		assert.False(t, ok)
	})
}
