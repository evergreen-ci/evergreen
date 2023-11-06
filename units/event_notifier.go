package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

func init() {
	registry.AddJobType(eventNotifierName, func() amboy.Job { return makeEventNotifierJob() })
}

const (
	eventNotifierName = "event-notifier"
)

type eventNotifierJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	q        amboy.Queue
	flags    *evergreen.ServiceFlags

	EventID string `bson:"event_id"`
}

func makeEventNotifierJob() *eventNotifierJob {
	j := &eventNotifierJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    eventNotifierName,
				Version: 0,
			},
		},
	}
	return j
}

func NewEventNotifierJob(env evergreen.Environment, eventID, ts string) amboy.Job {
	j := makeEventNotifierJob()
	j.env = env

	j.SetID(fmt.Sprintf("%s.%s.%s", eventNotifierName, eventID, ts))
	j.EventID = eventID
	j.SetScopes([]string{fmt.Sprintf("%s.%s", eventNotifierName, eventID)})
	j.SetEnqueueAllScopes(true)

	return j
}

func (j *eventNotifierJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.q == nil {
		j.q = j.env.RemoteQueue()
	}
	var err error
	j.flags, err = evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting admin settings"))
		return
	}
	if j.flags.EventProcessingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job_type": j.Type().Name,
			"message":  "events processing is disabled",
		})
		return
	}

	e, err := event.FindByID(j.EventID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding event '%s'", j.EventID))
		return
	}
	if e == nil {
		j.AddError(errors.Errorf("event '%s' not found", j.EventID))
		return
	}
	if !e.ProcessedAt.IsZero() {
		return
	}

	j.AddError(j.processEvent(ctx, e))
}

func (j *eventNotifierJob) processEvent(ctx context.Context, e *event.EventLogEntry) error {
	startTime := time.Now()
	catcher := grip.NewSimpleCatcher()

	n, err := j.processEventTriggers(ctx, e)
	catcher.Add(err)
	catcher.Add(e.MarkProcessed())

	if err = notification.InsertMany(n...); err != nil {
		// Consider that duplicate key errors are expected.
		shouldLogError := !db.IsDuplicateKey(err)
		grip.ErrorWhen(shouldLogError, message.WrapError(err, message.Fields{
			"job_id":        j.ID(),
			"job_type":      j.Type().Name,
			"source":        "events-processing",
			"notifications": n,
			"message":       "can't insert notifications",
		}))
		catcher.AddWhen(shouldLogError, errors.Wrap(err, "bulk inserting notifications"))
	}

	jobs, err := notificationJobs(ctx, n, j.flags, utility.RoundPartOfMinute(0))
	// Continue on error even if some jobs couldn't be marked disabled.
	catcher.Add(errors.Wrap(err, "getting notification jobs"))
	catcher.Add(errors.Wrap(j.q.PutMany(ctx, jobs), "enqueueing notification jobs"))

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	grip.Info(message.Fields{
		"job_id":        j.ID(),
		"job_type":      j.Type().Name,
		"operation":     "events-processing",
		"message":       "event-stats",
		"event_id":      j.EventID,
		"start_time":    startTime.String(),
		"end_time":      endTime.String(),
		"duration_secs": totalDuration.Seconds(),
		"has_errors":    catcher.HasErrors(),
		"num_errors":    catcher.Len(),
	})

	return catcher.Resolve()
}

func (j *eventNotifierJob) processEventTriggers(ctx context.Context, e *event.EventLogEntry) (n []notification.Notification, err error) {
	if e == nil {
		return nil, errors.New("cannot process event triggers for nil event")
	}

	defer func() {
		if r := recover(); r != nil {
			n = nil
			err = errors.Errorf("panicked while processing event '%s'", e.ID)
			grip.Alert(message.WrapError(err, message.Fields{
				"job_id":      j.ID(),
				"job_type":    j.Type().Name,
				"source":      "events-processing",
				"event_id":    e.ID,
				"event_type":  e.ResourceType,
				"panic_value": r,
			}))
		}
	}()

	startDebug := time.Now()
	n, err = trigger.NotificationsFromEvent(ctx, e)
	grip.Info(message.Fields{
		"job_id":        j.ID(),
		"job_type":      j.Type().Name,
		"source":        "events-processing",
		"message":       "event processed",
		"event_id":      e.ID,
		"event_type":    e.ResourceType,
		"notifications": len(n),
		"duration_secs": time.Since(startDebug).Seconds(),
		"stat":          "notifications-from-event",
	})

	grip.Error(message.WrapError(err, message.Fields{
		"job_id":     j.ID(),
		"job_type":   j.Type().Name,
		"source":     "events-processing",
		"message":    "errors processing triggers for event",
		"event_id":   e.ID,
		"event_type": e.ResourceType,
	}))

	v, err := trigger.EvalProjectTriggers(ctx, e, trigger.TriggerDownstreamVersion)
	grip.Info(message.Fields{
		"job_id":        j.ID(),
		"job_type":      j.Type().Name,
		"source":        "events-processing",
		"message":       "project triggers evaluated",
		"event_id":      e.ID,
		"event_type":    e.ResourceType,
		"duration_secs": time.Since(startDebug).Seconds(),
		"stat":          "eval-project-triggers",
	})
	versions := []string{}
	for _, version := range v {
		versions = append(versions, version.Id)
	}
	grip.InfoWhen(len(versions) > 0, message.Fields{
		"job_id":   j.ID(),
		"job_type": j.Type().Name,
		"source":   "events-processing",
		"message":  "triggering downstream builds",
		"event_id": e.ID,
		"versions": versions,
	})

	return n, err
}

func notificationJobs(ctx context.Context, notifications []notification.Notification, flags *evergreen.ServiceFlags, ts time.Time) ([]amboy.Job, error) {
	catcher := grip.NewBasicCatcher()
	var jobs []amboy.Job
	for i := range notifications {
		if notificationIsEnabled(flags, &notifications[i]) {
			jobs = append(jobs, NewEventSendJob(notifications[i].ID, ts.Format(TSFormat)))
		} else {
			catcher.Wrapf(notifications[i].MarkError(errors.New("notification is disabled")), "setting error for notification '%s'", notifications[i].ID)
		}
	}
	return jobs, catcher.Resolve()
}

func notificationIsEnabled(flags *evergreen.ServiceFlags, n *notification.Notification) bool {
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType, event.GithubCheckSubscriberType, event.GithubMergeSubscriberType:
		return !flags.GithubStatusAPIDisabled

	case event.JIRAIssueSubscriberType, event.JIRACommentSubscriberType:
		return !flags.JIRANotificationsDisabled

	case event.EvergreenWebhookSubscriberType:
		return !flags.WebhookNotificationsDisabled

	case event.EmailSubscriberType:
		return !flags.EmailNotificationsDisabled

	case event.SlackSubscriberType:
		return !flags.SlackNotificationsDisabled

	case event.EnqueuePatchSubscriberType:
		return !flags.CommitQueueDisabled

	default:
		grip.Alert(message.Fields{
			"message": "notificationIsEnabled saw unknown subscriber type",
			"cause":   "programmer error",
		})
	}

	return false
}
