package units

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen"
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

// dispatchUnprocessedNotifications gets unprocessed notifications
// leftover by previous runs and dispatches them
func (j *eventMetaJob) dispatchUnprocessedNotifications(ctx context.Context) error {
	unprocessedNotifications, err := notification.FindUnprocessed()
	if err != nil {
		return errors.Wrap(err, "can't find unprocessed notifications")
	}

	return dispatchNotifications(ctx, unprocessedNotifications, j.q, j.flags)
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
	j.events, err = event.FindUnprocessedEvents(-1)
	if err != nil {
		j.AddError(err)
		return
	}

	j.AddError(errors.Wrap(j.logEventCount(), "can't log unprocessed event count"))

	if len(j.events) == 0 {
		return
	}
	eventIDs := []string{}
	for i, evt := range j.events {
		eventIDs = append(eventIDs, evt.ID)
		if (i+1)%j.limit == 0 || i+1 == len(j.events) {
			err = j.q.Put(ctx, NewEventNotifierJob(j.env, j.q, sha256sum(eventIDs), eventIDs))
			grip.Error(message.WrapError(err, message.Fields{
				"message": "unable to queue event notifier job",
				"job_id":  j.ID(),
			}))
			eventIDs = []string{}
		}
	}
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

func sha256sum(ids []string) string {
	sum := sha256.Sum256([]byte(strings.Join(ids, "")))
	return hex.EncodeToString(sum[:])
}
