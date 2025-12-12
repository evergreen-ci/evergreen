package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	moveLogsToFailedBucketJobName = "move-logs-to-failed-bucket"
	fetchTimeout                  = 3 * time.Minute
)

func init() {
	registry.AddJobType(moveLogsToFailedBucketJobName, func() amboy.Job {
		return makeMoveLogsToFailedBucketJob()
	})
}

type moveLogsToFailedBucketJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	TaskID   string `bson:"task_id"`

	task *task.Task
	env  evergreen.Environment
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
func NewMoveLogsToFailedBucketJob(env evergreen.Environment, taskID, ts string) amboy.Job {
	j := makeMoveLogsToFailedBucketJob()
	jobID := fmt.Sprintf("%s.%s.%s", moveLogsToFailedBucketJobName, taskID, ts)
	j.SetID(jobID)
	j.SetScopes([]string{jobID})
	j.SetEnqueueAllScopes(true)
	j.env = env
	j.TaskID = taskID
	return j
}

func (j *moveLogsToFailedBucketJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	t, err := task.FindOneId(ctx, j.TaskID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding task '%s'", j.TaskID))
		return
	}
	if t == nil {
		j.AddError(errors.Errorf("task '%s' not found", j.TaskID))
		return
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	fetchContext, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	if err := t.MoveTestAndTaskLogsToFailedBucket(fetchContext, j.env.Settings()); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "moving logs to failed bucket",
			"task_id":   t.Id,
			"execution": t.Execution,
		}))
	}

	grip.Info(message.Fields{
		"message":   "moved logs to failed bucket",
		"task_id":   t.Id,
		"execution": t.Execution,
		"job":       j.ID(),
	})
}
