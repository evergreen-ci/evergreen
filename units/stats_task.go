package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
)

const (
	taskStatsCollectorJobName  = "task-stats-collector"
	taskStatsCollectorInterval = time.Minute
)

func init() {
	registry.AddJobType(taskStatsCollectorJobName,
		func() amboy.Job { return makeTaskStatsCollector() })
}

type taskStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

// NewTaskStatsCollector captures a single report of the status of
// tasks that have completed in the last minute.
func NewTaskStatsCollector(id string) amboy.Job {
	t := makeTaskStatsCollector()
	t.SetID(fmt.Sprintf("%s-%s", taskStatsCollectorJobName, id))
	return t
}

func makeTaskStatsCollector() *taskStatsCollector {
	j := &taskStatsCollector{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskStatsCollectorJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *taskStatsCollector) Run(_ context.Context) {
	defer j.MarkComplete()

	tasks, err := task.GetRecentTasks(taskStatsCollectorInterval)
	if err != nil {
		j.AddError(err)
		return
	}

	grip.Info(task.GetResultCounts(tasks))
}
