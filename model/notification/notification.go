package notification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// makeNotificationID creates a string representing the notification generated
// from the given event, with the given trigger, for the given subscriber.
// This function will produce an ID that will collide to prevent duplicate
// notifications from being inserted
func makeNotificationID(eventID, trigger string, subscriber *event.Subscriber) string {
	return fmt.Sprintf("%s-%s-%s", eventID, trigger, subscriber.String())
}

// New returns a new Notification, with a correctly initialised ID
func New(eventID, trigger string, subscriber *event.Subscriber, payload any) (*Notification, error) {
	if len(trigger) == 0 {
		return nil, errors.New("cannot create notification from nil trigger")
	}
	if subscriber == nil {
		return nil, errors.New("cannot create notification from nil subscriber")
	}
	if payload == nil {
		return nil, errors.New("cannot create notification with nil payload")
	}

	return &Notification{
		ID:         makeNotificationID(eventID, trigger, subscriber),
		Subscriber: *subscriber,
		Payload:    payload,
	}, nil
}

type Notification struct {
	ID         string           `bson:"_id"`
	Subscriber event.Subscriber `bson:"subscriber"`
	Payload    any              `bson:"payload"`

	SentAt   time.Time            `bson:"sent_at,omitempty"`
	Error    string               `bson:"error,omitempty"`
	Metadata NotificationMetadata `bson:"metadata,omitempty"`
}

type NotificationMetadata struct {
	TaskID        string `bson:"task_id,omitempty"`
	TaskExecution int    `bson:"task_execution,omitempty"`
}

// SenderKey returns an evergreen.SenderKey to get a grip sender for this
// notification from the evergreen environment
func (n *Notification) SenderKey() (evergreen.SenderKey, error) {
	switch n.Subscriber.Type {
	case event.EvergreenWebhookSubscriberType:
		return evergreen.SenderEvergreenWebhook, nil

	case event.EmailSubscriberType:
		return evergreen.SenderEmail, nil

	case event.JIRAIssueSubscriberType:
		return evergreen.SenderJIRAIssue, nil

	case event.JIRACommentSubscriberType:
		return evergreen.SenderJIRAComment, nil

	case event.SlackSubscriberType:
		return evergreen.SenderSlack, nil

	case event.GithubPullRequestSubscriberType, event.GithubCheckSubscriberType, event.GithubMergeSubscriberType:
		return evergreen.SenderGithubStatus, nil

	default:
		return evergreen.SenderEmail, errors.Errorf("unknown type '%s'", n.Subscriber.Type)
	}
}

// Composer builds a grip/message.Composer for the notification. Composer is
// guaranteed to be non-nil if error is nil, but the composer may not be
// loggable
func (n *Notification) Composer(ctx context.Context) (message.Composer, error) {
	switch n.Subscriber.Type {
	case event.EvergreenWebhookSubscriberType:
		sub, ok := n.Subscriber.Target.(*event.WebhookSubscriber)
		if !ok {
			return nil, errors.New("evergreen-webhook subscriber is invalid")
		}

		payload, ok := n.Payload.(*util.EvergreenWebhook)
		if !ok || payload == nil {
			return nil, errors.New("evergreen-webhook payload is invalid")
		}

		payload.Secret = sub.Secret
		payload.URL = sub.URL
		payload.NotificationID = n.ID
		payload.Retries = sub.Retries
		payload.MinDelayMS = sub.MinDelayMS
		payload.TimeoutMS = sub.TimeoutMS
		for _, header := range sub.Headers {
			payload.Headers.Add(header.Key, header.Value)
		}

		return util.NewWebhookMessage(*payload), nil

	case event.EmailSubscriberType:
		sub, ok := n.Subscriber.Target.(*string)
		if !ok {
			return nil, errors.New("email subscriber is invalid")
		}

		payload, ok := n.Payload.(*message.Email)
		if !ok || payload == nil {
			return nil, errors.New("email payload is invalid")
		}

		payload.Recipients = []string{*sub}
		return message.NewEmailMessage(level.Notice, *payload), nil

	case event.JIRAIssueSubscriberType:
		jiraIssue, ok := n.Subscriber.Target.(*event.JIRAIssueSubscriber)
		if !ok {
			return nil, errors.New("jira-issue subscriber is invalid")
		}
		payload, ok := n.Payload.(*message.JiraIssue)
		if !ok || payload == nil {
			return nil, errors.New("jira-issue payload is invalid")
		}

		payload.Project = jiraIssue.Project
		payload.Type = jiraIssue.IssueType
		payload.Callback = func(issueKey string) {
			event.LogJiraIssueCreated(ctx, n.Metadata.TaskID, n.Metadata.TaskExecution, issueKey)
		}

		return message.MakeJiraMessage(payload), nil

	case event.JIRACommentSubscriberType:
		sub, ok := n.Subscriber.Target.(*string)
		if !ok {
			return nil, errors.New("jira-comment subscriber is invalid")
		}

		payload, ok := n.Payload.(*string)
		if !ok || payload == nil {
			return nil, errors.New("jira-comment payload is invalid")
		}

		return message.NewJIRACommentMessage(level.Notice, *sub, *payload), nil

	case event.SlackSubscriberType:
		sub, ok := n.Subscriber.Target.(*string)
		if !ok {
			return nil, errors.New("slack subscriber is invalid")
		}

		formattedTarget, err := FormatSlackTarget(ctx, *sub)
		if err != nil {
			return nil, errors.Wrap(err, "formatting slack target")
		}

		payload, ok := n.Payload.(*SlackPayload)
		if !ok || payload == nil {
			return nil, errors.New("slack payload is invalid")
		}

		return message.NewSlackMessage(level.Notice, formattedTarget, payload.Body, payload.Attachments), nil

	case event.GithubPullRequestSubscriberType:
		sub := n.Subscriber.Target.(*event.GithubPullRequestSubscriber)
		payload, ok := n.Payload.(*message.GithubStatus)
		if !ok || payload == nil {
			return nil, errors.New("github-pull-request payload is invalid")
		}
		payload.Owner = sub.Owner
		payload.Repo = sub.Repo
		payload.Ref = sub.Ref
		return message.NewGithubStatusMessageWithRepo(level.Notice, *payload), nil

	case event.GithubCheckSubscriberType:
		sub := n.Subscriber.Target.(*event.GithubCheckSubscriber)
		payload, ok := n.Payload.(*message.GithubStatus)
		if !ok || payload == nil {
			return nil, errors.New("github-check payload is invalid")
		}
		payload.Owner = sub.Owner
		payload.Repo = sub.Repo
		payload.Ref = sub.Ref
		return message.NewGithubStatusMessageWithRepo(level.Notice, *payload), nil

	case event.GithubMergeSubscriberType:
		sub := n.Subscriber.Target.(*event.GithubMergeSubscriber)
		payload, ok := n.Payload.(*message.GithubStatus)
		if !ok || payload == nil {
			return nil, errors.New("github-merge payload is invalid")
		}
		payload.Owner = sub.Owner
		payload.Repo = sub.Repo
		payload.Ref = sub.Ref
		return message.NewGithubStatusMessageWithRepo(level.Notice, *payload), nil

	default:
		return nil, errors.Errorf("unknown type '%s'", n.Subscriber.Type)
	}
}

func (n *Notification) MarkSent(ctx context.Context) error {
	if len(n.ID) == 0 {
		return errors.New("notification has no ID")
	}

	n.SentAt = time.Now().Truncate(time.Millisecond)

	update := bson.M{
		"$set": bson.M{
			sentAtKey: n.SentAt,
		},
	}

	if err := db.UpdateIdContext(ctx, Collection, n.ID, update); err != nil {
		return errors.Wrap(err, "marking notification as sent")
	}

	return nil
}

func (n *Notification) MarkError(ctx context.Context, sendErr error) error {
	if sendErr == nil {
		return nil
	}
	if len(n.ID) == 0 {
		return errors.New("notification has no ID")
	}
	if n.SentAt.IsZero() {
		if err := n.MarkSent(ctx); err != nil {
			return err
		}
	}

	errMsg := sendErr.Error()
	update := bson.M{
		"$set": bson.M{
			errorKey: errMsg,
		},
	}
	n.Error = errMsg

	if err := db.UpdateIdContext(ctx, Collection, n.ID, update); err != nil {
		n.Error = ""
		return errors.Wrap(err, "setting error for notification")
	}

	return nil
}

func (n *Notification) SetTaskMetadata(ID string, execution int) {
	n.Metadata.TaskID = ID
	n.Metadata.TaskExecution = execution
}

// FormatSlackTarget uses the slackMemberId instead of the userName when possible.
func FormatSlackTarget(ctx context.Context, target string) (string, error) {
	if strings.HasPrefix(target, "@") {
		trimmedTarget := strings.TrimPrefix(target, "@")
		user, err := user.FindBySlackUsername(ctx, trimmedTarget)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not find user by Slack username, falling back to default target instead of using the member ID",
				"target":  target,
			}))
			return target, nil
		}
		if user != nil && user.Settings.SlackMemberId != "" {
			return user.Settings.SlackMemberId, nil
		}
	}
	return target, nil
}

type NotificationStats struct {
	GithubPullRequest int `json:"github_pull_request" bson:"github_pull_request" yaml:"github_pull_request"`
	JIRAIssue         int `json:"jira_issue" bson:"jira_issue" yaml:"jira_issue"`
	JIRAComment       int `json:"jira_comment" bson:"jira_comment" yaml:"jira_comment"`
	EvergreenWebhook  int `json:"evergreen_webhook" bson:"evergreen_webhook" yaml:"evergreen_webhook"`
	Email             int `json:"email" bson:"email" yaml:"email"`
	Slack             int `json:"slack" bson:"slack" yaml:"slack"`
	GithubCheck       int `json:"github_check" bson:"github_check" yaml:"github_check"`
	GithubMerge       int `json:"github_merge" bson:"github_merge" yaml:"github_merge"`
}

func CollectUnsentNotificationStats(ctx context.Context) (*NotificationStats, error) {
	const subscriberTypeKey = "type"
	pipeline := []bson.M{
		{
			"$match": bson.M{
				sentAtKey: bson.M{
					"$exists": false,
				},
			},
		},
		{
			"$group": bson.M{
				"_id": "$" + bsonutil.GetDottedKeyName(subscriberKey, subscriberTypeKey),
				"n": bson.M{
					"$sum": 1,
				},
			},
		},
	}

	stats := []struct {
		Key   string `bson:"_id"`
		Count int    `bson:"n"`
	}{}

	if err := db.Aggregate(ctx, Collection, pipeline, &stats); err != nil {
		return nil, errors.Wrap(err, "counting unsent notifications")
	}

	nStats := NotificationStats{}

	for _, data := range stats {
		switch data.Key {
		case event.GithubPullRequestSubscriberType:
			nStats.GithubPullRequest = data.Count

		case event.GithubCheckSubscriberType:
			nStats.GithubCheck = data.Count

		case event.GithubMergeSubscriberType:
			nStats.GithubMerge = data.Count

		case event.JIRAIssueSubscriberType:
			nStats.JIRAIssue = data.Count

		case event.JIRACommentSubscriberType:
			nStats.JIRAComment = data.Count

		case event.EvergreenWebhookSubscriberType:
			nStats.EvergreenWebhook = data.Count

		case event.EmailSubscriberType:
			nStats.Email = data.Count

		case event.SlackSubscriberType:
			nStats.Slack = data.Count

		default:
			grip.Error(message.Fields{
				"message": fmt.Sprintf("unknown subscriber '%s'", data.Key),
			})
		}
	}

	return &nStats, nil
}
