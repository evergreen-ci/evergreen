package units

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
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

func init() {
	registry.AddJobType(eventSendJobName, func() amboy.Job { return makeEventSendJob() })
}

type eventSendJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	flags    *evergreen.ServiceFlags

	NotificationID string `bson:"notification_id" json:"notification_id" yaml:"notification_id"`
	// IsRetry allows re-sending a notification that was already processed.
	IsRetry bool `bson:"is_retry,omitempty" json:"is_retry,omitempty" yaml:"is_retry,omitempty"`
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
	return newEventSendJob(id, ts, false)
}

// NewRetryEventSendJob re-sends an already-processed notification to retry failed deliveries.
func NewRetryEventSendJob(id, ts string) amboy.Job {
	return newEventSendJob(id, ts, true)
}

func newEventSendJob(id, ts string, retry bool) amboy.Job {
	j := makeEventSendJob()
	j.NotificationID = id
	j.IsRetry = retry

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
		if j.IsRetry {
			// The notification is gone, so there is nothing left to retry.
			j.AddError(notification.RemoveFailedWebhook(ctx, j.NotificationID))
		}
		j.AddError(errors.Errorf("notification '%s' not found", j.NotificationID))
		return
	}

	if err = j.checkDegradedMode(ctx, n); err != nil {
		j.AddError(markGitHubStatusResync(ctx, n, err))
		j.AddError(errors.Wrapf(n.MarkError(ctx, errors.Wrap(err, "checking degraded mode")), "setting error for notification '%s'", n.ID))
		return
	}

	if !j.IsRetry && !utility.IsZeroTime(n.SentAt) {
		j.AddError(errors.Errorf("notification '%s' has already been processed", n.ID))
		return
	}

	if j.IsRetry {
		// Rebuild the delivery target from the current subscription so the
		// retry uses the current URL, secret, and headers. Drop the record when
		// the target cannot be refreshed: the notification predates recording
		// its subscription, or the subscription was deleted or no longer sends
		// webhooks.
		var sub *event.Subscription
		if n.SubscriptionID != "" {
			sub, err = event.FindSubscriptionByID(ctx, n.SubscriptionID)
			if err != nil {
				j.AddError(errors.Wrapf(err, "finding subscription '%s' for notification '%s'", n.SubscriptionID, n.ID))
				return
			}
		}
		if sub == nil || sub.Subscriber.Type != n.Subscriber.Type {
			j.AddError(notification.RemoveFailedWebhook(ctx, n.ID))
			return
		}
		// The original send already exhausted its retry budget, so a retry makes one attempt.
		if webhookSub, ok := sub.Subscriber.Target.(*event.WebhookSubscriber); ok {
			webhookSub.Retries = 0
		}
		n.Subscriber = sub.Subscriber

		// Claim the record before sending so concurrent retries cannot send it
		// twice. MarkError re-arms it on failure, and the claim lease re-arms it
		// if the job dies mid-send.
		claimed, err := notification.MarkFailedWebhookRetried(ctx, n.ID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "marking failed webhook for notification '%s' as retried", n.ID))
			return
		}
		if !claimed {
			return
		}
	}

	err = j.send(ctx, n)
	if err == nil {
		j.AddError(errors.Wrapf(n.MarkSent(ctx), "marking notification '%s' as sent", n.ID))
		if j.IsRetry {
			j.AddError(notification.RemoveFailedWebhook(ctx, n.ID))
		}
		return
	}

	logFields := message.Fields{
		"job_id":            j.ID(),
		"notification_id":   n.ID,
		"notification_type": n.Subscriber.Type,
		"message":           "send failed",
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
	}

	grip.Error(ctx, message.WrapError(err, logFields))
	j.AddError(err)
	j.AddError(markGitHubStatusResync(ctx, n, err))
	j.AddError(errors.Wrapf(n.MarkError(ctx, err), "setting error for notification '%s'", n.ID))
}

// markGitHubStatusResync marks the patch that a GitHub status notification
// reports on so its statuses can be sent again. Statuses that GitHub rejected
// as invalid are not marked because sending them again won't help.
func markGitHubStatusResync(ctx context.Context, n *notification.Notification, sendErr error) error {
	if n.Metadata.PatchID == "" || isRejectedGitHubSend(sendErr) {
		return nil
	}
	return patch.MarkGitHubStatusResync(ctx, n.Metadata.PatchID, n.Subscriber.Type == event.GithubMergeSubscriberType)
}

// isGitHubRateLimit returns whether GitHub refused the request with a status
// that it uses for rate limits.
func isGitHubRateLimit(err error) bool {
	var githubErr *send.GitHubSendError
	if !errors.As(err, &githubErr) {
		return false
	}
	return githubErr.StatusCode == http.StatusForbidden || githubErr.StatusCode == http.StatusTooManyRequests
}

// isRejectedGitHubSend returns whether GitHub rejected the request with a
// client error other than a rate limit.
func isRejectedGitHubSend(err error) bool {
	var githubErr *send.GitHubSendError
	if !errors.As(err, &githubErr) || isGitHubRateLimit(err) {
		return false
	}
	return githubErr.StatusCode >= http.StatusBadRequest && githubErr.StatusCode < http.StatusInternalServerError
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
