package units

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	eventMetaJobName = "event-metajob"
)

func init() {
	registry.AddJobType(eventMetaJobName, func() amboy.Job { return makeEventMetaJob() })
}

func notificationIsEnabled(flags *evergreen.ServiceFlags, n *notification.Notification) bool {
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType:
		return !flags.GithubStatusAPIDisabled

	case event.GithubMergeSubscriberType:
		return !flags.CommitQueueDisabled

	case event.JIRAIssueSubscriberType, event.JIRACommentSubscriberType:
		return !flags.JIRANotificationsDisabled

	case event.EvergreenWebhookSubscriberType:
		return !flags.WebhookNotificationsDisabled

	case event.EmailSubscriberType:
		return !flags.EmailNotificationsDisabled

	case event.SlackSubscriberType:
		return !flags.SlackNotificationsDisabled

	case event.CommitQueueDequeueSubscriberType:
		return !flags.CommitQueueDisabled

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
	limit    int
	events   []event.EventLogEntry
	flags    *evergreen.ServiceFlags
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
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(1)

	return j
}

func NewEventMetaJob(env evergreen.Environment, q amboy.Queue, ts string) amboy.Job {
	j := makeEventMetaJob()
	j.q = q
	j.env = env

	j.SetID(fmt.Sprintf("%s:%s", eventMetaJobName, ts))

	return j
}

func tryProcessOneEvent(e *event.EventLogEntry) (n []notification.Notification, err error) {
	if e == nil {
		return nil, errors.New("nil event")
	}

	defer func() {
		if r := recover(); r != nil {
			n = nil
			err = errors.Errorf("panicked while processing event %s", e.ID)
			grip.Alert(message.WrapError(err, message.Fields{
				"job_id":      j.ID(),
				"job":         eventMetaJobName,
				"source":      "events-processing",
				"event_id":    e.ID,
				"event_type":  e.ResourceType,
				"panic_value": r,
			}))
		}
	}()

	startDebug := time.Now()
	n, err = trigger.NotificationsFromEvent(e)
	grip.Info(message.Fields{
		"job_id":        j.ID(),
		"job":           eventMetaJobName,
		"source":        "events-processing",
		"message":       "event processed",
		"event_id":      e.ID,
		"event_type":    e.ResourceType,
		"notifications": len(n),
		"duration":      time.Now().Sub(startDebug),
		"stat":          "notifications-from-event",
	})

	grip.Error(message.WrapError(err, message.Fields{
		"job_id":     j.ID(),
		"job":        eventMetaJobName,
		"source":     "events-processing",
		"message":    "errors processing triggers for event",
		"event_id":   e.ID,
		"event_type": e.ResourceType,
	}))

	v, err := trigger.EvalProjectTriggers(e, trigger.TriggerDownstreamVersion)
	grip.Info(message.Fields{
		"job_id":     j.ID(),
		"job":        eventMetaJobName,
		"source":     "events-processing",
		"message":    "project triggers evaluated",
		"event_id":   e.ID,
		"event_type": e.ResourceType,
		"duration":   time.Now().Sub(startDebug),
		"stat":       "eval-project-triggers",
	})
	versions := []string{}
	for _, version := range v {
		versions = append(versions, version.Id)
	}
	grip.InfoWhen(len(versions) > 0, message.Fields{
		"job_id":   j.ID(),
		"job":      eventMetaJobName,
		"source":   "events-processing",
		"message":  "triggering downstream builds",
		"event_id": e.ID,
		"versions": versions,
	})

	return n, err
}

func (j *eventMetaJob) dispatchLoop(ctx context.Context) error {
	// TODO: in the future we may want to split up this job so
	// that the parallelism is pushed up to amboy rather than down
	// into this job.
	startTime := time.Now()
	logger := event.NewDBEventLogger(event.AllLogCollection)
	catcher := grip.NewSimpleCatcher()

	input := make(chan event.EventLogEntry, len(j.events))
	for _, e := range j.events {
		select {
		case input <- e:
			continue
		case <-ctx.Done():
			return errors.New("operation aborted")
		}
	}
	close(input)

	wg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer recovery.LogStackTraceAndContinue("problem during notification dispatch")

			for e := range input {
				n, err := tryProcessOneEvent(&e)
				catcher.Add(err)
				catcher.Add(logger.MarkProcessed(&e))

				if err = notification.InsertMany(n...); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"job_id":        j.ID(),
						"job":           eventMetaJobName,
						"source":        "events-processing",
						"notifications": n,
						"message":       "can't insert notifications",
					}))
					catcher.Add(err)
				}

				catcher.Add(j.dispatch(ctx, n))
			}
		}()
	}
	wg.Wait()

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	grip.Info(message.Fields{
		"job_id":     j.ID(),
		"job":        eventMetaJobName,
		"operation":  "events-processing",
		"message":    "event-stats",
		"start_time": startTime.String(),
		"end_time":   endTime.String(),
		"duration":   totalDuration.Seconds(),
		"n":          len(j.events),
		"has_errors": catcher.HasErrors(),
		"num_errors": catcher.Len(),
	})

	return catcher.Resolve()
}

func (j *eventMetaJob) dispatch(ctx context.Context, notifications []notification.Notification) error {
	catcher := grip.NewBasicCatcher()
	for i := range notifications {
		if notificationIsEnabled(j.flags, &notifications[i]) {
			if err := j.q.Put(ctx, NewEventNotificationJob(notifications[i].ID)); !amboy.IsDuplicateJobError(err) {
				catcher.Add(err)
			}
		} else {
			catcher.Add(notifications[i].MarkError(errors.New("sender disabled")))
		}
	}

	return catcher.Resolve()
}

// dispatchUnprocessedNotifications gets unprocessed notifications
// leftover by previous runs and dispatches them
func (j *eventMetaJob) dispatchUnprocessedNotifications(ctx context.Context) error {
	unprocessedNotifications, err := notification.FindUnprocessed()
	if err != nil {
		return errors.Wrap(err, "can't find unprocessed notifications")
	}

	return j.dispatch(ctx, unprocessedNotifications)
}

func (j *eventMetaJob) Run(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.q == nil {
		j.q = j.env.RemoteQueue()
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
			"message": "events processing is disabled",
		})
		return
	}

	j.AddError(j.dispatchUnprocessedNotifications(ctx))

	notifySettings := j.env.Settings().Notify
	j.AddError(notifySettings.Get(j.env))
	j.limit = notifySettings.EventProcessingLimit
	if j.limit <= 0 {
		j.limit = evergreen.DefaultEventProcessingLimit
	}
	j.events, err = event.FindUnprocessedEvents(j.limit)
	if err != nil {
		j.AddError(err)
		return
	}

	j.AddError(errors.Wrap(j.logEventCount(), "can't log unprocessed event count"))

	if len(j.events) == 0 {
		return
	}

	j.AddError(j.dispatchLoop(ctx))
}

func (j *eventMetaJob) logEventCount() error {
	eventCount := len(j.events)
	if eventCount == j.limit {
		var err error
		eventCount, err = event.CountUnprocessedEvents()
		if err != nil {
			return errors.Wrap(err, "error getting unprocessed event count")
		}
	}

	grip.Info(message.Fields{
		"job_id":  j.ID(),
		"job":     eventMetaJobName,
		"message": "unprocessed event count",
		"pending": eventCount,
		"limit":   j.limit,
		"current": len(j.events),
		"source":  "events-processing",
	})

	return nil
}
