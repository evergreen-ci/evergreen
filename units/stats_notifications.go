package units

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	notificationsStatsCollectorJobName = "notifications-stats-collector"
)

func init() {
	registry.AddJobType(notificationsStatsCollectorJobName,
		func() amboy.Job { return makeNotificationsStatsCollector() })
}

type notificationsStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

func makeNotificationsStatsCollector() *notificationsStatsCollector {
	j := &notificationsStatsCollector{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    notificationsStatsCollectorJobName,
				Version: 0,
			},
		},
		logger: logging.MakeGrip(grip.GetSender()),
	}
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(-1)
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: time.Minute,
	})

	return j
}

func NewNotificationStatsCollector(id string) amboy.Job {
	job := makeNotificationsStatsCollector()
	job.SetID(id)

	return job
}

func (j *notificationsStatsCollector) Run(ctx context.Context) {
	defer j.MarkComplete()

	msg := message.Fields{
		"start_time": j.TimeInfo().Start.String(),
	}

	e, err := event.FindLastProcessedEvent()
	j.AddError(errors.Wrap(err, "failed to fetch most recently processed event"))
	if j.HasErrors() {
		return
	}
	if e != nil {
		msg["last_processed_at"] = e.ProcessedAt
	}

	nUnprocessed, err := event.CountUnprocessedEvents()
	j.AddError(errors.Wrap(err, "failed to count unprocessed events"))
	if j.HasErrors() {
		return
	}
	msg["unprocessed_events"] = nUnprocessed

	stats, err := notification.CollectUnsentNotificationStats()
	j.AddError(errors.Wrap(err, "failed to collect notification stats"))
	if j.HasErrors() {
		return
	}

	msg["pending_notifications_by_type"] = stats

	j.AddError(ctx.Err())
	if ctx.Err() == nil {
		j.logger.Info(msg)

	} else {
		j.logger.Warning(message.WrapError(ctx.Err(), message.Fields{
			"start_time": j.TimeInfo().Start.String(),
			"message":    "job context cancelled",
			"job":        notificationsStatsCollectorJobName,
			"job_id":     j.ID(),
		}))
	}
}
