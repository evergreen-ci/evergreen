package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	eventSendJobName = "event-send"
)

// The delays are cumulative so that a terminal status is retried for about
// 30 minutes without keeping a worker occupied.
var githubNotificationRetryDelays = []time.Duration{
	time.Minute,
	2 * time.Minute,
	4 * time.Minute,
	8 * time.Minute,
	15 * time.Minute,
}

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
	return j
}

func NewEventSendJob(id, ts string) amboy.Job {
	j := makeEventSendJob()
	j.NotificationID = id

	j.SetID(fmt.Sprintf("%s:%s:%s", eventSendJobName, id, ts))
	return j
}

func (j *eventSendJob) setup(ctx context.Context) error {
	if len(j.NotificationID) == 0 {
		return errors.New("notification ID is not valid")
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.flags == nil {
		j.flags = &evergreen.ServiceFlags{}
		if err := j.flags.Get(ctx); err != nil {
			return errors.Wrap(err, "getting service flags")
		}
	}

	return nil
}

func (j *eventSendJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.setup(ctx); err != nil {
		j.AddError(err)
		return
	}

	n, err := notification.Find(ctx, j.NotificationID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding notification '%s'", j.NotificationID))
		return
	}
	if n == nil {
		j.AddError(errors.Errorf("notification '%s' not found", j.NotificationID))
		return
	}

	if err = j.checkDegradedMode(ctx, n); err != nil {
		j.AddError(errors.Wrapf(n.MarkError(ctx, errors.Wrap(err, "checking degraded mode")), "setting error for notification '%s'", n.ID))
		return
	}

	if !utility.IsZeroTime(n.SentAt) {
		j.AddError(errors.Errorf("notification '%s' has already been processed", n.ID))
		return
	}

	err = j.send(ctx, n)
	if err == nil {
		j.AddError(errors.Wrapf(n.MarkSent(ctx), "marking notification '%s' as sent", n.ID))
		return
	}

	logFields := message.Fields{
		"job_id":               j.ID(),
		"notification_id":      n.ID,
		"notification_type":    n.Subscriber.Type,
		"message":              "send failed",
		"notification_attempt": n.SendAttempts + 1,
	}
	var githubErr *send.GitHubSendError
	if errors.As(err, &githubErr) {
		logFields["status_code"] = githubErr.StatusCode
		logFields["retry_count"] = githubErr.Attempts - 1
		status, _ := n.Payload.(*message.GithubStatus)
		if status != nil {
			logFields["context"] = status.Context
			logFields["sha"] = status.Ref
		}
		// Retrying an old pending status after a terminal status is sent can
		// make GitHub consider the check pending again.
		canRetry := githubErr.Retryable &&
			status != nil &&
			status.State != message.GithubStatePending &&
			n.SendAttempts < len(githubNotificationRetryDelays)
		if canRetry {
			delay := githubNotificationRetryDelays[n.SendAttempts]
			logFields["retry_delay_secs"] = delay.Seconds()
			grip.Error(ctx, message.WrapError(err, logFields))
			j.AddError(err)
			j.AddError(errors.Wrapf(n.MarkRetry(ctx, err, delay), "preserving notification '%s' for retry", n.ID))
			return
		}
	}

	grip.Error(ctx, message.WrapError(err, logFields))
	j.AddError(err)
	j.AddError(errors.Wrapf(n.MarkError(ctx, err), "setting error for notification '%s'", n.ID))
}

func (j *eventSendJob) send(ctx context.Context, n *notification.Notification) error {
	c, err := n.Composer(ctx)
	if err != nil {
		return err
	}
	if err = c.SetPriority(level.Notice); err != nil {
		return errors.Wrap(err, "setting priority")
	}
	if !c.Loggable() {
		return errors.New("composer is not loggable")
	}

	key, err := n.SenderKey()
	if err != nil {
		return errors.Wrap(err, "getting sender key for notification")
	}

	var sender send.Sender
	if key == evergreen.SenderGithubStatus {
		payload, ok := n.Payload.(*message.GithubStatus)
		if !ok || payload == nil {
			return errors.New("github status payload is invalid")
		}
		sender, err = j.env.GetGitHubSender(payload.Owner, payload.Repo, githubapp.CreateGitHubAppAuth(j.env.Settings()).CreateGitHubSenderInstallationToken)
		if err != nil {
			return errors.Wrap(err, "getting github status sender")
		}
	} else {
		sender, err = j.env.GetSender(key)
		if err != nil {
			return errors.Wrap(err, "getting global notification sender")
		}
	}
	return send.SendWithError(ctx, sender, c)
}

func (j *eventSendJob) checkDegradedMode(ctx context.Context, n *notification.Notification) error {
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType, event.GithubCheckSubscriberType, event.GithubMergeSubscriberType:
		return checkFlag(ctx, j.flags.GithubStatusAPIDisabled)

	case event.SlackSubscriberType:
		return checkFlag(ctx, j.flags.SlackNotificationsDisabled)

	case event.JIRAIssueSubscriberType:
		return checkFlag(ctx, j.flags.JIRANotificationsDisabled)

	case event.JIRACommentSubscriberType:
		return checkFlag(ctx, j.flags.JIRANotificationsDisabled)

	case event.EvergreenWebhookSubscriberType:
		return checkFlag(ctx, j.flags.WebhookNotificationsDisabled)

	case event.EmailSubscriberType:
		return checkFlag(ctx, j.flags.EmailNotificationsDisabled)

	default:
		return errors.Errorf("unknown subscriber type '%s'", n.Subscriber.Type)
	}
}

func checkFlag(ctx context.Context, flag bool) error {
	if flag {
		grip.InfoWhen(ctx, sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     eventSendJobName,
			"message": "sender is disabled, not sending notification",
		})
		return errors.New("sender is disabled, not sending notification")
	}

	return nil
}
