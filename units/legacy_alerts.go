package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/model/alert"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const legacyAlertRunnerJobName = "legacy-alert-runner"

func init() {
	registry.AddJobType(legacyAlertRunnerJobName,
		func() amboy.Job { return makeLegacyAlertsJob() })

}

type legacyAlertsJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeLegacyAlertsJob() *legacyAlertsJob {
	j := &legacyAlertsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    legacyAlertRunnerJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
}

func NewLegacyAlertsRunnerJob(env evergreen.Environment, id string) amboy.Job {
	j := makeLegacyMonitorRunnerJob()
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s", legacyAlertRunnerJobName, id))
	return j
}

func (j *legacyAlertsJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	config := j.env.Settings()
	qp := alerts.NewQueueProcessor(config, evergreen.FindEvergreenHome())

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.AlertsDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent),
			message.Fields{
				"runner":  "alerts",
				"message": "alerts are disabled, exiting",
				"id":      j.ID(),
			})
		return
	}

	if len(config.SuperUsers) == 0 {
		grip.Critical(message.Fields{
			"message": "no superusers configured, some alerts may have no recipient",
			"runner":  "alerts",
			"id":      j.ID(),
		})
	}
	superUsers, err := user.Find(user.ByIds(config.SuperUsers...))
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"runner":    "alerts",
			"status":    "failed",
			"id":        j.ID(),
			"operation": "finding super users",
		}))

		j.AddError(errors.Wrap(err, "problem getting superuser list"))
		return
	}
	qp.AddSuperUsers(superUsers)

	for {
		nextAlert, err := alert.DequeueAlertRequest()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner": "alerts",
				"status": "failed to dequeue request",
				"id":     j.ID(),
			}))

			j.AddError(errors.Wrap(err, "Failed to dequeue alert request"))
			return
		}
		if nextAlert == nil {
			grip.Debug(message.Fields{
				"message": "Reached end of queue items - stopping",
				"runner":  "alerts",
				"id":      j.ID(),
			})
			break
		}

		grip.Debug(message.Fields{
			"operation": "processing queue item",
			"id":        j.ID(),
			"runner":    "alerts",
			"alert":     nextAlert.Id.Hex(),
		})

		alertContext, err := qp.LoadAlertContext(nextAlert)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner":    "alerts",
				"operation": "loading context",
				"status":    "failed",
				"id":        j.ID(),
			}))

			j.AddError(errors.Wrap(err, "Failed to load alert context"))
			return
		}

		grip.Debug(message.Fields{
			"message": "Delivering queue item",
			"item":    nextAlert.Id.Hex(),
			"runner":  "alerts",
			"id":      j.ID(),
		})

		err = qp.Deliver(nextAlert, alertContext)
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "Got error delivering message",
			"id":      j.ID(),
			"runner":  "alerts",
		}))
		j.AddError(err)
	}
}
