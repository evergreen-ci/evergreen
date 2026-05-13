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
