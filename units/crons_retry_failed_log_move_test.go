package units

import (
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
	sourceBucketCfg := evergreen.BucketConfig{Name: sourceBucket, Type: evergreen.BucketTypeS3}
	failedBucketCfg := evergreen.BucketConfig{Name: failedBucket, Type: evergreen.BucketTypeS3}

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
					BucketConfig: sourceBucketCfg,
				},
			},
		}
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

	for tName, tCase := range map[string]func(t *testing.T, env *mock.Environment){
		"EnqueuesWeeklyRetryJobForEvergreenFailedTask": func(t *testing.T, env *mock.Environment) {
			env.EvergreenSettings.Buckets.LogBucketFailedTasks = failedBucketCfg
			tsk := retryCandidate("evergreen-retry-1", tempRetryFailedLogMoveProject, time.Now())
			require.NoError(t, tsk.Insert(ctx))
			require.NoError(t, run(env))

			ts := utility.RoundPartOfMinute(0).Format(TSFormat)
			expectID := NewMoveLogsToFailedBucketJob(env, tsk.Id, ts, sourceBucketCfg, MoveLogsTriggerWeeklyRetry).ID()
			j, ok := env.RemoteQueue().Get(ctx, expectID)
			require.True(t, ok)
			moveJob, ok := j.(*moveLogsToFailedBucketJob)
			require.True(t, ok)
			assert.Equal(t, tsk.Id, moveJob.TaskID)
			assert.Equal(t, MoveLogsTriggerWeeklyRetry, moveJob.Trigger)
			assert.Equal(t, sourceBucket, moveJob.SourceBucketCfg.Name)
		},
		"SkipsFailedTaskInOtherProject": func(t *testing.T, env *mock.Environment) {
			env.EvergreenSettings.Buckets.LogBucketFailedTasks = failedBucketCfg
			tsk := retryCandidate("other-proj-task", "some-other-project", time.Now())
			require.NoError(t, tsk.Insert(ctx))
			require.NoError(t, run(env))

			ts := utility.RoundPartOfMinute(0).Format(TSFormat)
			expectID := NewMoveLogsToFailedBucketJob(env, tsk.Id, ts, sourceBucketCfg, MoveLogsTriggerWeeklyRetry).ID()
			_, ok := env.RemoteQueue().Get(ctx, expectID)
			assert.False(t, ok)
		},
		"SkipsTaskWhoseLogsAreAlreadyInFailedBucket": func(t *testing.T, env *mock.Environment) {
			env.EvergreenSettings.Buckets.LogBucketFailedTasks = failedBucketCfg
			tsk := retryCandidate("already-moved", tempRetryFailedLogMoveProject, time.Now())
			tsk.TaskOutputInfo.TaskLogs.BucketConfig = failedBucketCfg
			require.NoError(t, tsk.Insert(ctx))
			require.NoError(t, run(env))

			ts := utility.RoundPartOfMinute(0).Format(TSFormat)
			expectID := NewMoveLogsToFailedBucketJob(env, tsk.Id, ts, sourceBucketCfg, MoveLogsTriggerWeeklyRetry).ID()
			_, ok := env.RemoteQueue().Get(ctx, expectID)
			assert.False(t, ok)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			env := setup(t)
			tCase(t, env)
		})
	}
}
