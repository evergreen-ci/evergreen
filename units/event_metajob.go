package units

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	eventMetaJobName        = "event-metajob"
	EventProcessingInterval = 5 * time.Minute
)

func init() {
	registry.AddJobType(eventMetaJobName, func() amboy.Job { return makeEventMetaJob() })
}

func notificationIsEnabled(flags *evergreen.ServiceFlags, n *notification.Notification) bool {
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType:
		return !flags.GithubStatusAPIDisabled

	case event.JIRAIssueSubscriberType, event.JIRACommentSubscriberType:
		return !flags.JIRANotificationsDisabled

	case event.EvergreenWebhookSubscriberType:
		return !flags.WebhookNotificationsDisabled

	case event.EmailSubscriberType:
		return !flags.EmailNotificationsDisabled

	case event.SlackSubscriberType:
		return !flags.SlackNotificationsDisabled

	default:
		grip.Alert(message.Fields{
			"message": "notificationIsEnabled saw unknown subscriber type",
			"cause":   "programmer error",
		})
	}

	return false
}

type eventMetaJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	q        amboy.Queue
	events   []event.EventLogEntry
	flags    *evergreen.ServiceFlags
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
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(50)

	return j
}

func NewEventMetaJob(q amboy.Queue, ts string) amboy.Job {
	j := makeEventMetaJob()
	j.q = q

	j.SetID(fmt.Sprintf("%s:%s", eventMetaJobName, ts))

	return j
}

func (j *eventMetaJob) dispatchLoop() error {
	// TODO: if this is a perf problem, it could be multithreaded. For now,
	// we just log time
	startTime := time.Now()
	logger := event.NewDBEventLogger(event.AllLogCollection)
	catcher := grip.NewSimpleCatcher()
	for i := range j.events {
		var notifications []notification.Notification
		notifications, err := notification.NotificationsFromEvent(&j.events[i])
		catcher.Add(err)

		grip.Error(message.WrapError(err, message.Fields{
			"job_id":     j.ID(),
			"job":        eventMetaJobName,
			"source":     "events-processing",
			"message":    "errors processing triggers for event",
			"event_id":   j.events[i].ID.Hex(),
			"event_type": j.events[i].Type(),
		}))

		// TODO: buffered writes after EVG-3062
		if err = notification.InsertMany(notifications...); err != nil {
			catcher.Add(err)
			continue
		}

		catcher.Add(j.dispatch(notifications))
		catcher.Add(logger.MarkProcessed(&j.events[i]))
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
		"n":          len(j.events),
	})

	return catcher.Resolve()
}

func (j *eventMetaJob) dispatch(notifications []notification.Notification) error {
	catcher := grip.NewSimpleCatcher()
	for i := range notifications {
		if notificationIsEnabled(j.flags, &notifications[i]) {
			catcher.Add(j.q.Put(newEventNotificationJob(notifications[i].ID)))

		} else {
			catcher.Add(notifications[i].MarkError(errors.New("sender disabled")))
		}
	}

	return catcher.Resolve()
}

func (j *eventMetaJob) Run() {
	defer j.MarkComplete()

	if j.q == nil {
		env := evergreen.GetEnvironment()
		j.q = env.RemoteQueue()
	}
	if j.q == nil || !j.q.Started() {
		j.AddError(errors.New("evergreen environment not setup correctly"))
		return
	}

	var err error
	j.flags, err = evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if j.flags.EventProcessingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     eventMetaJobName,
			"message": "events processing is disabled, all events will be marked processed",
		})

		j.AddError(event.MarkAllEventsProcessed(event.AllLogCollection))
		return
	}

	j.events, err = event.Find(event.AllLogCollection, db.Query(event.UnprocessedEvents()))
	if err != nil {
		j.AddError(err)
		return
	}

	if len(j.events) == 0 {
		grip.Info(message.Fields{
			"job_id":  j.ID(),
			"job":     eventMetaJobName,
			"time":    time.Now().String(),
			"message": "no events need to be processed",
			"source":  "events-processing",
		})
		return
	}

	j.AddError(j.dispatchLoop())
}
