package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	moveLogsToFailedBucketJobName     = "move-logs-to-failed-bucket"
	fetchTimeout                      = 30 * time.Minute
	moveLogsToFailedBucketMaxAttempts = 3
)

// MoveLogsTriggerTaskEnd is used when the app server enqueues after a failed task ends (agent path).
const MoveLogsTriggerTaskEnd = "task_end"

// MoveLogsTriggerHourlyRetry is used by PopulateRetryFailedLogMoveJobs (hourly remote cron).
const MoveLogsTriggerHourlyRetry = "hourly_retry_failed_log_move"

func init() {
	registry.AddJobType(moveLogsToFailedBucketJobName, func() amboy.Job {
		return makeMoveLogsToFailedBucketJob()
	})
}

type moveLogsToFailedBucketJob struct {
	job.Base        `bson:"metadata" json:"metadata" yaml:"metadata"`
	TaskID          string                 `bson:"task_id"`
	SourceBucketCfg evergreen.BucketConfig `bson:"source_bucket_cfg"`
	// Trigger records why the job was enqueued.
	Trigger string        `bson:"trigger,omitempty"`
	Timeout time.Duration `bson:"timeout,omitempty"`

	env evergreen.Environment
}

func makeMoveLogsToFailedBucketJob() *moveLogsToFailedBucketJob {
	j := &moveLogsToFailedBucketJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    moveLogsToFailedBucketJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewMoveLogsToFailedBucketJob creates a job that moves a task's logs to the failed bucket.
// Pass an optional timeout as the last argument to override the default (e.g. NewMoveLogsToFailedBucketJob(env, taskID, ts, sourceCfg, 60*time.Minute)).
func NewMoveLogsToFailedBucketJob(env evergreen.Environment, taskID, ts string, sourceBucketCfg evergreen.BucketConfig, trigger string, timeout ...time.Duration) amboy.Job {
	j := makeMoveLogsToFailedBucketJob()
	j.env = env
	j.TaskID = taskID
	j.SourceBucketCfg = sourceBucketCfg
	j.Trigger = trigger
	if len(timeout) > 0 {
		j.Timeout = timeout[0]
	}
	jobID := fmt.Sprintf("%s.%s.%s", moveLogsToFailedBucketJobName, taskID, ts)
	j.SetID(jobID)
	j.SetScopes([]string{jobID})
	j.SetEnqueueAllScopes(true)
	maxTime := fetchTimeout
	if j.Timeout > 0 {
		maxTime = j.Timeout
	}
	// MaxTime tells the Amboy pool how long this job may run; without it the pool may cancel the job before the S3 move completes.
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: maxTime})
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(moveLogsToFailedBucketMaxAttempts),
	})
	return j
}

func (j *moveLogsToFailedBucketJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	grip.Info(ctx, message.Fields{
		"message": "failed_bucket_move: job started",
		"task_id": j.TaskID,
		"job":     j.ID(),
		"trigger": j.Trigger,
	})

	timeout := fetchTimeout
	if j.Timeout > 0 {
		timeout = j.Timeout
	}
	t, err := task.FindOneId(ctx, j.TaskID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding task '%s'", j.TaskID))
		return
	}
	if t == nil {
		j.AddError(errors.Errorf("task '%s' not found", j.TaskID))
		return
	}

	fetchContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// Use the source bucket config captured when the job was created rather than the one on
	// the task config because that has already been updated to the failed bucket so that future
	// logs are written there directly.
	if err := t.MoveTestAndTaskLogsToFailedBucket(fetchContext, j.env.Settings(), j.SourceBucketCfg); err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":   "moving logs to failed bucket",
			"task_id":   t.Id,
			"execution": t.Execution,
			"timeout":   timeout.String(),
			"trigger":   j.Trigger,
		}))
		j.AddRetryableError(errors.Wrap(err, "moving logs to failed bucket"))
		j.recoverIfStuck(ctx, t)
		return
	}

	grip.Info(ctx, message.Fields{
		"message":   "successfully moved logs to failed bucket",
		"task_id":   t.Id,
		"execution": t.Execution,
		"job":       j.ID(),
		"trigger":   j.Trigger,
	})
}

// recoverIfStuck attempts to revert the task's DB bucket config back to the source bucket
// when a log move has permanently failed and left the task in an inconsistent state.
//
// A task is "stuck" when its DB config already points to the failed bucket but the move
// never completed, making the logs inaccessible. This only happens after the final retry
// attempt; tasks whose DB config still points to the source bucket are not stuck because
// the hourly retry cron will find and re-enqueue them.
func (j *moveLogsToFailedBucketJob) recoverIfStuck(ctx context.Context, t *task.Task) {
	if t.TaskOutputInfo == nil {
		return
	}
	failedBucketName := j.env.Settings().Buckets.LogBucketFailedTasks.Name
	if j.RetryInfo().GetRemainingAttempts() != 0 || t.TaskOutputInfo.TaskLogs.BucketConfig.Name != failedBucketName {
		return
	}

	logFields := func(msg string) message.Fields {
		return message.Fields{
			"message":       msg,
			"task_id":       t.Id,
			"execution":     t.Execution,
			"trigger":       j.Trigger,
			"source_bucket": j.SourceBucketCfg.Name,
		}
	}

	reverted, err := t.RevertBucketConfigToSourceIfLogsExist(ctx, j.SourceBucketCfg)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, logFields("failed to check source bucket after permanent log move failure, reverting bucket config anyway")))
		if forceErr := t.RevertBucketConfigToSource(ctx, j.SourceBucketCfg); forceErr != nil {
			grip.Error(ctx, message.WrapError(forceErr, logFields("failed to revert bucket config to source bucket")))
		}
		return
	}

	if reverted {
		grip.Info(ctx, logFields("reverted bucket config to source bucket after permanent log move failure: logs are accessible in source bucket"))
		return
	}

	fields := logFields("permanently failed to move logs and no logs found in source bucket: task logs may be inaccessible")
	fields["destination_bucket"] = failedBucketName
	grip.Error(ctx, fields)
}
