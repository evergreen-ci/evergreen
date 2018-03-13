package units

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/triggers"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	eventMetaJobName         = "event-metajob"
	eventNotificationJobName = "event-send"
	evergreenWebhookTimeout  = 5 * time.Second

	EventMetaJobPeriod = 5 * time.Minute
)

func init() {
	registry.AddJobType(eventMetaJobName, func() amboy.Job { return makeEventMetaJob() })
	//registry.AddJobType(eventNotificationJobName, func() amboy.Job { return makeEventNotificationJob() })
}

type eventMetaJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeEventMetaJob() *eventMetaJob {
	j := &eventMetaJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    eventMetaJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways()) //wat?

	return j
}

func NewEventMetaJob() amboy.Job {
	j := makeEventMetaJob()

	// TODO: safe?
	j.SetID(fmt.Sprintf("%s:%d", eventMetaJobName, job.GetNumber()))

	return j
}

func (j *eventMetaJob) Run() {
	defer j.MarkComplete()

	if j.env == nil || j.env.RemoteQueue() == nil || !j.env.RemoteQueue().Started() {
		j.AddError(errors.New("evergreen environment not setup correctly"))
		return
	}
	// TODO degraded mode

	events, err := event.Find(event.AllLogCollection, event.UnprocessedEvents())
	logger := event.NewDBEventLogger(event.AllLogCollection)

	if err != nil {
		j.AddError(err)
		return
	}

	if len(events) == 0 {
		grip.Info(message.Fields{
			"job_id":  j.ID(),
			"job":     eventMetaJobName,
			"message": "no events need to be processed",
			"source":  "events-processing",
		})
		return
	}

	// TODO: if this is a perf problem, it could be multithreaded. For now,
	// we just log time
	startTime := time.Now()
	for i := range events {
		triggerStartTime := time.Now()

		var notifications []notification.Notification
		notifications, err = triggers.ProcessTriggers(&events[i])

		triggerEndTime := time.Now()
		triggerDuration := triggerEndTime.Sub(triggerStartTime)
		j.AddError(err)
		grip.Info(message.Fields{
			"job_id":            j.ID(),
			"job":               eventMetaJobName,
			"source":            "events-processing",
			"message":           "event-stats",
			"event_id":          events[i].ID.Hex(),
			"event_type":        events[i].Type(),
			"start_time":        triggerStartTime.String(),
			"end_time":          triggerEndTime.String(),
			"duration":          triggerDuration.String(),
			"num_notifications": len(notifications),
		})
		grip.Error(message.WrapError(err, message.Fields{
			"job_id":     j.ID(),
			"job":        eventMetaJobName,
			"source":     "events-processing",
			"message":    "errors processing triggers for event",
			"event_id":   events[i].ID.Hex(),
			"event_type": events[i].Type(),
		}))

		if err = notification.InsertMany(notifications...); err != nil {
			j.AddError(err)
			continue
		}

		j.AddError(logger.MarkProcessed(&events[i]))
	}
	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	grip.Info(message.Fields{
		"job_id":     j.ID(),
		"job":        eventMetaJobName,
		"source":     "events-processing",
		"message":    "stats",
		"start_time": startTime.String(),
		"end_time":   endTime.String(),
		"duration":   totalDuration.String(),
		"n":          len(events),
	})
}

type eventNotificationJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	NotificationID bson.ObjectId `bson:"notification_id" json:"notification_id" yaml:"notification_id"`
}

func makeEventNotificationJob() *eventNotificationJob {
	j := &eventNotificationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    eventNotificationJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewEventNotificationJob(id bson.ObjectId) amboy.Job {
	j := makeEventNotificationJob()

	j.SetID(fmt.Sprintf("%s:%s", eventNotificationJobName, id.Hex()))
	return j
}

func (j *eventNotificationJob) Run() {
	defer j.MarkComplete()

	if !j.NotificationID.Valid() {
		j.AddError(errors.New("notification ID is not valid"))
		return
	}

	n, err := notification.Find(j.NotificationID)
	j.AddError(err)
	if n == nil {
		j.AddError(errors.Errorf("can't find notification with ID: '%s", j.NotificationID.Hex()))
	}
	if j.HasErrors() {
		return
	}

	switch n.Type {
	default:
		j.AddError(errors.Errorf("unknown notification type: %s", n.Type))
	}
}
