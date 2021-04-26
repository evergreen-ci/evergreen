package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const (
	duplicateTaskCheckJobName = "duplicate-task-check"
)

func init() {
	registry.AddJobType(duplicateTaskCheckJobName, func() amboy.Job { return makeDuplicateTaskCheckJob() })
}

type duplicateTaskCheckJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeDuplicateTaskCheckJob() *duplicateTaskCheckJob {
	j := &duplicateTaskCheckJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    duplicateTaskCheckJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewDuplicateTaskCheckJob checks for any tasks that appear in multiple primary
// task queues.
func NewDuplicateTaskCheckJob(id string) amboy.Job {
	j := makeDuplicateTaskCheckJob()
	j.SetID(fmt.Sprintf("%s.%s", duplicateTaskCheckJobName, id))
	return j
}

func (j *duplicateTaskCheckJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	dups, err := model.FindDuplicateEnqueuedTasks(model.TaskQueuesCollection)
	if err != nil {
		j.AddError(err)
		return
	}
	dupTaskToDistros := map[string][]string{}
	for _, dup := range dups {
		dupTaskToDistros[dup.TaskID] = dup.DistroIDs
	}
	if len(dupTaskToDistros) != 0 {
		grip.Error(message.Fields{
			"message":         "tasks are enqueued multiple times in primary task queues",
			"task_to_distros": dupTaskToDistros,
			"job":             j.ID(),
			"job_type":        j.Type().Name,
		})
	}
}
