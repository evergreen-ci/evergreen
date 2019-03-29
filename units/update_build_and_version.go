package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const updateBuildAndVersionJobName = "update-build-and-version"

func init() {
	registry.AddJobType(updateBuildAndVersionJobName, func() amboy.Job { return makeUpdateBuildAndVersionJob() })
}

type updateBuildAndVersionJob struct {
	TaskID   string `bson:"task_id" json:"task_id"`
	job.Base `bson:"job_base" json:"job_base"`
}

func makeUpdateBuildAndVersionJob() *updateBuildAndVersionJob {
	j := &updateBuildAndVersionJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    updateBuildAndVersionJobName,
				Version: 1,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewUpdateBuildAndVersionJob creates a job to update builds and version for a finished
func NewUpdateBuildAndVersionJob(taskID string) amboy.Job {
	job := makeUpdateBuildAndVersionJob()
	job.SetID(fmt.Sprintf("update-build-and-version-%s", taskID))
	job.TaskID = taskID
	job.SetPriority(1)
	return job
}

func (j *updateBuildAndVersionJob) Run(_ context.Context) {
	defer j.MarkComplete()
	updates := &model.StatusChanges{}
	if err := model.UpdateBuildAndVersionStatusForTask(j.TaskID, updates); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "Error updating build status",
			"task":    j.TaskID,
			"job":     j.ID(),
		}))
		j.AddError(errors.Wrap(err, "Error updating build status"))
	}
	isBuildCompleteStatus := updates.BuildNewStatus == evergreen.BuildFailed || updates.BuildNewStatus == evergreen.BuildSucceeded
	if len(updates.BuildNewStatus) != 0 {
		if updates.BuildComplete || !isBuildCompleteStatus {
			t, err := task.FindOneId(j.TaskID)
			grip.Error(message.WrapError(err, message.Fields{
				"message": "Error finding task",
				"task":    j.TaskID,
				"job":     j.ID(),
			}))
			if err != nil {
				j.AddError(errors.Wrap(err, "Error finding task"))
			}
			event.LogBuildStateChangeEvent(t.BuildId, updates.BuildNewStatus)
		}
	}
}
