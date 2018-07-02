package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/monitor"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const legacyMonitorRunnerJobName = "legacy-monitor-runner"

func init() {
	registry.AddJobType(legacyMonitorRunnerJobName,
		func() amboy.Job { return makeLegacyMonitorRunnerJob() })
}

type legacyMonitorJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeLegacyMonitorRunnerJob() *legacyMonitorJob {
	j := &legacyMonitorJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    legacyMonitorRunnerJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewLegacyMonitorRunnerJob(env evergreen.Environment, id string) amboy.Job {
	j := makeLegacyMonitorRunnerJob()
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s", legacyMonitorRunnerJobName, id))
	return j
}

func (j *legacyMonitorJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	config := j.env.Settings()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  "monitor",
			"id":      j.ID(),
			"message": "monitor is disabled, exiting",
		})
		return
	}

	notifier := &monitor.Notifier{
		NotificationBuilders: []monitor.NotificationBuilder{
			monitor.SlowProvisioningWarnings,
		},
	}

	// send notifications
	grip.Error(message.WrapError(notifier.Notify(config), message.Fields{
		"runner":  "monitor",
		"id":      j.ID(),
		"message": "Error sending notifications",
	}))

}
