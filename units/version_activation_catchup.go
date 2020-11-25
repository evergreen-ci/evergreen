package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
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
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
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
	j.SetDependency(dependency.NewAlways())
	return j

}

func NewVersionActivationJob(id string) amboy.Job {
	j := makeVersionActivationCatchupJob()

	j.SetID(fmt.Sprintf("%s.%s", versionActivationCatchupJobName, id))
	j.SetPriority(-1)
	return j
}

func (j *versionActivationCatchup) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
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

	projects, err := model.FindAllMergedTrackedProjectRefs()
	if err != nil {
		j.AddError(err)
		return
	}

	count := 0
	for _, ref := range projects {
		if !ref.Enabled {
			continue
		}
		j.AddError(errors.Wrapf(repotracker.ActivateBuildsForProject(ref),
			"problem activating builds for project %s", ref.Id))
		count++
	}

	grip.Info(message.Fields{
		"message":  "version activation catch up report",
		"projects": len(projects),
		"active":   count,
		"errors":   j.HasErrors(),
	})
}
