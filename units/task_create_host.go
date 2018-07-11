package units

import (
	"context"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const taskHostJobName = "task-create-host"

func init() {
	registry.AddJobType(taskHostJobName, func() amboy.Job {
		return makeTaskHostJob()
	})
}

type taskHostJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeTaskHostJob() *taskHostJob {
	j := &taskHostJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskHostJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewTaskHostCreateJob(taskID string, createHost apimodels.CreateHost) amboy.Job {
	j := makeTaskHostJob()
	return j
}

func (j *taskHostJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	j.AddError(errors.New("job is not yet implemented (EVG-3230)"))
}
