package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/amboy"
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
	registry.AddJobType(notificationsStatsCollectorJobName, func() amboy.Job {
		return makeNotificationsStatsCollector()
	})
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

	return j
}

func NewNotificationStatsCollector(id string) amboy.Job {
	j := makeNotificationsStatsCollector()
	j.SetID(fmt.Sprintf("%s-%s", notificationsStatsCollectorJobName, id))
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: time.Minute,
	})
	return j
}

func (j *notificationsStatsCollector) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.logger == nil {
		j.logger = logging.MakeGrip(grip.GetSender())
	}

	msg := message.Fields{
		"start_time": j.TimeInfo().Start,
		"stats":      "notifications",
	}

	e, err := event.FindLastProcessedEvent()
	j.AddError(errors.Wrap(err, "fetching most recently processed event"))
	if j.HasErrors() {
		return
	}
	if e != nil {
		msg["last_processed_at"] = e.ProcessedAt
	}

	nUnprocessed, err := event.CountUnprocessedEvents()
	j.AddError(errors.Wrap(err, "counting unprocessed events"))
	if j.HasErrors() {
		return
	}
	msg["unprocessed_events"] = nUnprocessed

	stats, err := notification.CollectUnsentNotificationStats()
	j.AddError(errors.Wrap(err, "collecting unsent notification stats"))
	if j.HasErrors() {
		return
	}

	msg["pending_notifications_by_type"] = stats

	if ctx.Err() == nil {
		j.logger.Info(msg)
	}
}
