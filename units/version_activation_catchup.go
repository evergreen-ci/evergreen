package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const versionActivationCatchupJobName = "version-activation-catchup"

func init() {
	registry.AddJobType(versionActivationCatchupJobName, func() amboy.Job {
		return makeVersionActivationCatchupJob()
	})

}

type versionActivationCatchup struct {
	job.Base  `bson:"metadata" json:"metadata" yaml:"metadata"`
	TimeStamp string `bson:"timestamp" json:"timestamp" yaml:"timestamp"`
}

func makeVersionActivationCatchupJob() *versionActivationCatchup {
	j := &versionActivationCatchup{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    versionActivationCatchupJobName,
				Version: 1,
			},
		},
	}
	return j

}

func NewVersionActivationJob(ts string) amboy.Job {
	j := makeVersionActivationCatchupJob()
	j.TimeStamp = ts
	j.SetID(fmt.Sprintf("%s.%s", versionActivationCatchupJobName, ts))
	return j
}

func (j *versionActivationCatchup) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.SchedulerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "scheduler is disabled",
			"impact":  "skipping batch time activation",
			"mode":    "degraded",
		})
		return
	}

	projects, err := model.FindAllMergedTrackedProjectRefs(ctx)
	if err != nil {
		j.AddError(err)
		return
	}

	ts, err := time.Parse(TSFormat, j.TimeStamp)
	if err != nil {
		j.AddError(err)
		return
	}

	count := 0
	projectsActivated := []string{}
	for _, ref := range projects {
		if !ref.Enabled {
			continue
		}

		// Do not use a version that is in the active cron range because those will be ignored when the one before
		// it has an activate_at that is within the cron range.
		ok, err := repotracker.ActivateBuildsForProject(ctx, ref, ts.Add(-model.CronActiveRange))
		j.AddError(errors.Wrapf(err, "activating builds for project '%s'", ref.Id))
		if ok {
			projectsActivated = append(projectsActivated, ref.Identifier)
		}
		count++
	}

	grip.Info(message.Fields{
		"message":            "version activation catch up report",
		"projects":           len(projects),
		"projects_activated": projectsActivated,
		"active":             count,
		"errors":             j.HasErrors(),
		"timestamp_used":     ts,
	})
}
