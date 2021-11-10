package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const schedulerAliasJobName = "distro-alias-scheduler"

func init() {
	registry.AddJobType(schedulerAliasJobName, func() amboy.Job {
		return makeDistroAliasSchedulerJob()
	})
}

type distroAliasSchedulerJob struct {
	DistroID string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeDistroAliasSchedulerJob() *distroAliasSchedulerJob {
	j := &distroAliasSchedulerJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    schedulerAliasJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

func NewDistroAliasSchedulerJob(distroID string, ts time.Time) amboy.Job {
	j := makeDistroAliasSchedulerJob()
	j.DistroID = distroID
	j.SetID(fmt.Sprintf("%s.%s.%s", schedulerAliasJobName, distroID, ts.Format(TSFormat)))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", schedulerAliasJobName, distroID)})
	j.SetEnqueueAllScopes(true)

	return j
}

func (j *distroAliasSchedulerJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.SchedulerDisabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"distro":   j.DistroID,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	startAt := time.Now()
	tasks, err := task.FindSchedulableForAlias(j.DistroID)
	j.AddError(errors.Wrap(err, "problem finding tasks"))
	if tasks == nil {
		return
	}

	d, err := distro.FindByID(j.DistroID)
	j.AddError(errors.Wrapf(err, "problem finding distro '%s'", j.DistroID))
	if d == nil {
		return
	}
	plan, err := scheduler.PrioritizeTasks(d, tasks, scheduler.TaskPlannerOptions{
		StartedAt:        startAt,
		ID:               j.ID(),
		IsSecondaryQueue: true,
	})
	if err != nil {
		j.AddError(err)
		return
	}

	grip.Info(message.Fields{
		"runner":        scheduler.RunnerName,
		"distro":        j.DistroID,
		"alias":         true,
		"job":           j.ID(),
		"size":          len(plan),
		"input_size":    len(tasks),
		"duration_secs": time.Since(startAt).Seconds(),
	})
}
