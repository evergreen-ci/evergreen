package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	eventSendJobName = "event-send"
)

func init() {
	registry.AddJobType(eventSendJobName, func() amboy.Job { return makeEventSendJob() })
}

type eventSendJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	flags    *evergreen.ServiceFlags

	NotificationID string `bson:"notification_id" json:"notification_id" yaml:"notification_id"`
}

func makeEventSendJob() *eventSendJob {
	j := &eventSendJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    eventSendJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewEventSendJob(id, ts string) amboy.Job {
	j := makeEventSendJob()
	j.NotificationID = id

	j.SetID(fmt.Sprintf("%s:%s:%s", eventSendJobName, id, ts))
	return j
}

func (j *eventSendJob) setup() error {
	if len(j.NotificationID) == 0 {
		return errors.New("notification ID is not valid")
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.flags == nil {
		j.flags = &evergreen.ServiceFlags{}
		if err := j.flags.Get(j.env); err != nil {
			return errors.Wrap(err, "error retrieving service flags")

		} else if j.flags == nil {
			return errors.Wrap(err, "fetched no service flags configuration")
		}
	}

	return nil
}

func (j *eventSendJob) Run(_ context.Context) {
	defer j.MarkComplete()

	if err := j.setup(); err != nil {
		j.AddError(err)
		return
	}

	n, err := notification.Find(j.NotificationID)
	j.AddError(err)
	if err == nil && n == nil {
		j.AddError(errors.Errorf("can't find notification with ID: '%s", j.NotificationID))
	}
	if j.HasErrors() {
		return
	}

	if err = j.checkDegradedMode(n); err != nil {
		j.AddError(n.MarkError(err))
		return
	}

	if !utility.IsZeroTime(n.SentAt) {
		j.AddError(errors.Errorf("notification '%s' has already been processed", n.ID))
		return
	}

	err = j.send(n)
	grip.Error(message.WrapError(err, message.Fields{
		"job_id":            j.ID(),
		"notification_id":   n.ID,
		"notification_type": n.Subscriber.Type,
		"message":           "send failed",
	}))
	j.AddError(err)
	j.AddError(n.MarkSent())
	j.AddError(n.MarkError(err))
}

func (j *eventSendJob) send(n *notification.Notification) error {
	c, err := n.Composer(j.env)
	if err != nil {
		return err
	}
	if err = c.SetPriority(level.Notice); err != nil {
		return errors.Wrap(err, "can't set priority")
	}
	if !c.Loggable() {
		return errors.New("composer is not loggable")
	}

	key, err := n.SenderKey()
	if err != nil {
		return errors.Wrap(err, "can't build sender for notification")
	}

	sender, err := j.env.GetSender(key)
	if err != nil {
		return errors.Wrap(err, "error building sender for notification")
	}

	sender.Send(c)
	return nil
}

func (j *eventSendJob) checkDegradedMode(n *notification.Notification) error {
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType, event.GithubCheckSubscriberType:
		return checkFlag(j.flags.GithubStatusAPIDisabled)

	case event.SlackSubscriberType:
		return checkFlag(j.flags.SlackNotificationsDisabled)

	case event.JIRAIssueSubscriberType:
		return checkFlag(j.flags.JIRANotificationsDisabled)

	case event.JIRACommentSubscriberType:
		return checkFlag(j.flags.JIRANotificationsDisabled)

	case event.EvergreenWebhookSubscriberType:
		return checkFlag(j.flags.WebhookNotificationsDisabled)

	case event.EmailSubscriberType:
		return checkFlag(j.flags.EmailNotificationsDisabled)

	case event.EnqueuePatchSubscriberType:
		return checkFlag(j.flags.CommitQueueDisabled)

	default:
		return errors.Errorf("unknown subscriber type: %s", n.Subscriber.Type)
	}
}

func checkFlag(flag bool) error {
	if flag {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     eventSendJobName,
			"message": "sender is disabled, not sending notification",
		})
		return errors.New("sender is disabled, not sending notification")
	}

	return nil
}
