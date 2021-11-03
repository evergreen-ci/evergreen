package units

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/utility"
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

func init() {
	registry.AddJobType(eventNotifierName, func() amboy.Job { return makeEventNotifierJob() })
}

const (
	eventNotifierName = "event-notifier"
)

type eventNotifierJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	q        amboy.Queue
	env      evergreen.Environment
	flags    *evergreen.ServiceFlags

	Events []string `bson:"events"`
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
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewEventNotifierJob(env evergreen.Environment, q amboy.Queue, id string, events []string) amboy.Job {
	j := makeEventNotifierJob()
	j.q = q
	j.env = env

	j.SetID(fmt.Sprintf("%s.%s", eventNotifierName, id))
	j.Events = events

	return j
}

func (j *eventNotifierJob) Run(ctx context.Context) {
	if len(j.Events) == 0 {
		return
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.q == nil {
		j.q = j.env.RemoteQueue()
	}
	if j.q == nil || !j.q.Info().Started {
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
			"job_type": j.Type().Name,
			"message":  "events processing is disabled",
		})
		return
	}

	events, err := event.FindEventsByIDs(j.Events)
	if err != nil {
		j.AddError(err)
		return
	}

	j.AddError(j.dispatchLoop(ctx, events))
}

func (j *eventNotifierJob) dispatchLoop(ctx context.Context, events []event.EventLogEntry) error {
	// TODO: in the future we may want to split up this job so
	// that the parallelism is pushed up to amboy rather than down
	// into this job.
	startTime := time.Now()
	logger := event.NewDBEventLogger(event.AllLogCollection)
	catcher := grip.NewSimpleCatcher()

	input := make(chan event.EventLogEntry, len(events))
	for _, e := range events {
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
				n, err := j.tryProcessOneEvent(&e)
				catcher.Add(err)
				catcher.Add(logger.MarkProcessed(&e))

				if err = notification.InsertMany(n...); err != nil {
					// consider that duplicate key errors are expected
					shouldLogError := !db.IsDuplicateKey(err)
					grip.ErrorWhen(shouldLogError, message.WrapError(err, message.Fields{
						"job_id":        j.ID(),
						"job_type":      j.Type().Name,
						"source":        "events-processing",
						"notifications": n,
						"message":       "can't insert notifications",
					}))
					catcher.AddWhen(shouldLogError, err)
				}

				catcher.Add(dispatchNotifications(ctx, n, j.q, j.flags))
			}
		}()
	}
	wg.Wait()

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	grip.Info(message.Fields{
		"job_id":     j.ID(),
		"job_type":   j.Type().Name,
		"operation":  "events-processing",
		"message":    "event-stats",
		"start_time": startTime.String(),
		"end_time":   endTime.String(),
		"duration":   totalDuration.Seconds(),
		"n":          len(events),
		"has_errors": catcher.HasErrors(),
		"num_errors": catcher.Len(),
	})

	return catcher.Resolve()
}

func (j *eventNotifierJob) tryProcessOneEvent(e *event.EventLogEntry) (n []notification.Notification, err error) {
	if e == nil {
		return nil, errors.New("nil event")
	}

	defer func() {
		if r := recover(); r != nil {
			n = nil
			err = errors.Errorf("panicked while processing event %s", e.ID)
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
	n, err = trigger.NotificationsFromEvent(e)
	grip.Info(message.Fields{
		"job_id":        j.ID(),
		"job_type":      j.Type().Name,
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
		"job_type":   j.Type().Name,
		"source":     "events-processing",
		"message":    "errors processing triggers for event",
		"event_id":   e.ID,
		"event_type": e.ResourceType,
	}))

	v, err := trigger.EvalProjectTriggers(e, trigger.TriggerDownstreamVersion)
	grip.Info(message.Fields{
		"job_id":     j.ID(),
		"job_type":   j.Type().Name,
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
		"job_type": j.Type().Name,
		"source":   "events-processing",
		"message":  "triggering downstream builds",
		"event_id": e.ID,
		"versions": versions,
	})

	return n, err
}

func dispatchNotifications(ctx context.Context, notifications []notification.Notification, q amboy.Queue, flags *evergreen.ServiceFlags) error {
	catcher := grip.NewBasicCatcher()
	for i := range notifications {
		if notificationIsEnabled(flags, &notifications[i]) {
			if err := q.Put(ctx, NewEventSendJob(notifications[i].ID, utility.RoundPartOfMinute(1).Format(TSFormat))); !amboy.IsDuplicateJobError(err) {
				catcher.Add(err)
			}
		} else {
			catcher.Add(notifications[i].MarkError(errors.New("sender disabled")))
		}
	}

	return catcher.Resolve()
}

func notificationIsEnabled(flags *evergreen.ServiceFlags, n *notification.Notification) bool {
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType, event.GithubCheckSubscriberType:
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

func sha256sum(ids []string) string {
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "")))
	return hex.EncodeToString(sum[:])
}
