package units

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type eventNotificationSuite struct {
	suite.Suite

	suiteCtx context.Context
	ctx      context.Context
	cancel   context.CancelFunc

	env *mock.Environment

	notifications []notification.Notification
	webhook       *notification.Notification
	email         *notification.Notification
	slack         *notification.Notification
	jiraComment   *notification.Notification
	jiraIssue     *notification.Notification
}

type eventSendEnvironment struct {
	*mock.Environment
	githubSender send.Sender
}

func (e *eventSendEnvironment) GetGitHubSender(string, string, evergreen.CreateInstallationTokenFunc) (send.Sender, error) {
	return e.githubSender, nil
}

type eventSendErrorSender struct {
	send.Sender
	err error
}

func (s *eventSendErrorSender) SendWithError(context.Context, message.Composer) error {
	return s.err
}

func TestEventNotificationJob(t *testing.T) {
	suite.Run(t, &eventNotificationSuite{})
}

func (s *eventNotificationSuite) SetupSuite() {
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, s.T())
}

func (s *eventNotificationSuite) TearDownSuite() {
	s.cancel()
}

func (s *eventNotificationSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())
	s.env = &mock.Environment{}
	s.NoError(s.env.Configure(s.ctx))

	s.NoError(db.ClearCollections(notification.Collection, evergreen.ConfigCollection))

	s.notifications = []notification.Notification{
		{
			ID: "webhook",
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: event.WebhookSubscriber{
					URL:    "https://example.com",
					Secret: []byte("memes"),
				},
			},
			Payload: &util.EvergreenWebhook{
				Body: []byte("o hai"),
			},
		},
		{
			ID: "email",
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "o@hai.hai",
			},
			Payload: message.Email{
				Subject: "o hai",
				Body:    "i'm a notification",
				Headers: map[string][]string{
					"such": {"much"},
				},
			},
		},
		{
			ID: "slack",
			Subscriber: event.Subscriber{
				Type:   event.SlackSubscriberType,
				Target: "#evg-test-channel",
			},
			Payload: notification.SlackPayload{
				Body: "Hi",
			},
		},

		{
			ID: "jira-comment",
			Subscriber: event.Subscriber{
				Type:   event.JIRACommentSubscriberType,
				Target: "EVG-2863",
			},
			Payload: "eventNotificationSuite jira comment message",
		},

		{
			ID: "jira-issue",
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
				Target: event.JIRAIssueSubscriber{
					Project:   "SERVER",
					IssueType: "Build Failure",
				},
			},
			Payload: message.JiraIssue{
				Summary:     "Tell the evergreen team that they're awesome",
				Description: "The evergreen team is awesome. Inform them of it",
				Reporter:    "eliot.horowitz",
			},
		},
		{
			ID: "github-status",
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
				Target: event.GithubPullRequestSubscriber{
					Owner:    "evergreen-ci",
					Repo:     "evergreen",
					PRNumber: 1234,
					Ref:      "main",
				},
			},
			Payload: message.GithubStatus{
				Context: "evergreen",
				URL:     "https://example.com",
				State:   message.GithubStateFailure,
			},
		},
	}
	s.webhook = &s.notifications[0]
	s.email = &s.notifications[1]
	s.slack = &s.notifications[2]
	s.jiraComment = &s.notifications[3]
	s.jiraIssue = &s.notifications[4]

	s.NoError(notification.InsertMany(s.ctx, s.notifications...))
}

func (s *eventNotificationSuite) notificationHasError(ctx context.Context, id string, pattern string) time.Time {
	n, err := notification.Find(ctx, id)
	s.Require().NoError(err)
	s.Require().NotNil(n)

	if len(pattern) == 0 {
		s.Empty(n.Error)

	} else {
		match, err := regexp.MatchString(pattern, n.Error)
		s.NoError(err)
		s.True(match, n.Error)
	}

	return n.SentAt
}

func (s *eventNotificationSuite) TestDegradedMode() {
	flags := evergreen.ServiceFlags{
		JIRANotificationsDisabled:    true,
		SlackNotificationsDisabled:   true,
		EmailNotificationsDisabled:   true,
		WebhookNotificationsDisabled: true,
		GithubStatusAPIDisabled:      true,
		BackgroundStatsDisabled:      true,
	}
	s.NoError(flags.Set(s.ctx))

	for i := range s.notifications {
		job := NewEventSendJob(s.notifications[i].ID, "").(*eventSendJob)
		job.env = evergreen.GetEnvironment()

		job.Run(s.ctx)
		s.NoError(job.Error())
	}

	s.NotZero(s.notificationHasError(s.ctx, s.webhook.ID, "sender is disabled, not sending notification"))
}

func (s *eventNotificationSuite) TestEvergreenWebhook() {
	job := NewEventSendJob(s.webhook.ID, "").(*eventSendJob)
	job.env = s.env

	job.Run(s.ctx)
	s.NoError(job.Error())

	s.NotZero(s.notificationHasError(s.ctx, s.webhook.ID, ""))
	s.NoError(job.Error())

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		_ = msg.Message.Raw().(*util.EvergreenWebhook)
	})
}

func (s *eventNotificationSuite) TestSlack() {
	job := NewEventSendJob(s.slack.ID, "").(*eventSendJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.ctx, s.slack.ID, ""))

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		slack := msg.Message.Raw().(*message.Slack)
		s.Equal("Hi", slack.Msg)
		s.Equal("#evg-test-channel", slack.Target)
		s.Empty(slack.Attachments)
	})
}

func (s *eventNotificationSuite) TestJIRAComment() {
	job := NewEventSendJob(s.jiraComment.ID, "").(*eventSendJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.ctx, s.jiraComment.ID, ""))

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		jira := msg.Message.Raw().(*message.JIRAComment)
		s.Equal("eventNotificationSuite jira comment message", jira.Body)
		s.Equal("EVG-2863", jira.IssueID)
	})
}

func (s *eventNotificationSuite) TestJIRAIssue() {
	job := NewEventSendJob(s.jiraIssue.ID, "").(*eventSendJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.ctx, s.jiraIssue.ID, ""))

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		jira := msg.Message.Raw().(*message.JiraIssue)
		s.Equal("SERVER", jira.Project)
		s.Equal("Tell the evergreen team that they're awesome", jira.Summary)
		s.Equal("The evergreen team is awesome. Inform them of it", jira.Description)
		s.Equal("eliot.horowitz", jira.Reporter)
	})
}

func (s *eventNotificationSuite) TestSendFailureResultsInNoMessages() {
	s.Require().NoError(db.ClearCollections(notification.Collection))
	n := s.notifications[:len(s.notifications)-1]
	for i := range n {
		// make the payload malformed
		n[i].Payload = nil
		s.NoError(notification.InsertMany(s.ctx, n[i]))

		job := NewEventSendJob(n[i].ID, "").(*eventSendJob)
		job.env = s.env
		job.Run(s.ctx)
		s.Error(job.Error())

		_, recv := s.env.InternalSender.GetMessageSafe()
		s.False(recv)
	}

	s.NotZero(s.notificationHasError(s.ctx, s.webhook.ID, "^composer is not loggable$"))
}

func (s *eventNotificationSuite) TestRetryableGitHubFailurePreservesNotification() {
	sender := &eventSendErrorSender{
		Sender: s.env.InternalSender,
		err: &send.GitHubSendError{
			StatusCode: http.StatusInternalServerError,
			Attempts:   evergreen.GitHubRetryAttempts,
			Retryable:  true,
			Err:        errors.New("GitHub unavailable"),
		},
	}
	job := NewEventSendJob("github-status", "").(*eventSendJob)
	job.env = &eventSendEnvironment{Environment: s.env, githubSender: sender}
	job.Run(s.ctx)

	s.Error(job.Error())
	n, err := notification.Find(s.ctx, "github-status")
	s.NoError(err)
	s.Require().NotNil(n)
	s.Zero(n.SentAt)
	s.Equal(1, n.SendAttempts)
	s.WithinDuration(time.Now().Add(time.Minute), n.NextAttemptAt, time.Second)
}

func (s *eventNotificationSuite) TestPermanentGitHubFailureCompletesNotification() {
	sender := &eventSendErrorSender{
		Sender: s.env.InternalSender,
		err: &send.GitHubSendError{
			StatusCode: http.StatusUnprocessableEntity,
			Attempts:   1,
			Retryable:  false,
			Err:        errors.New("invalid status"),
		},
	}
	job := NewEventSendJob("github-status", "").(*eventSendJob)
	job.env = &eventSendEnvironment{Environment: s.env, githubSender: sender}
	job.Run(s.ctx)

	s.Error(job.Error())
	n, err := notification.Find(s.ctx, "github-status")
	s.NoError(err)
	s.Require().NotNil(n)
	s.NotZero(n.SentAt)
	s.Equal("invalid status after 1 attempt(s)", n.Error)
}

func (s *eventNotificationSuite) TestPendingGitHubFailureCompletesNotification() {
	n, err := notification.Find(s.ctx, "github-status")
	s.NoError(err)
	s.Require().NotNil(n)
	status, ok := n.Payload.(*message.GithubStatus)
	s.Require().True(ok)
	status.State = message.GithubStatePending
	_, err = db.Replace(s.ctx, notification.Collection, bson.M{"_id": n.ID}, n)
	s.NoError(err)

	sender := &eventSendErrorSender{
		Sender: s.env.InternalSender,
		err: &send.GitHubSendError{
			StatusCode: http.StatusInternalServerError,
			Attempts:   evergreen.GitHubRetryAttempts,
			Retryable:  true,
			Err:        errors.New("GitHub unavailable"),
		},
	}
	job := NewEventSendJob(n.ID, "").(*eventSendJob)
	job.env = &eventSendEnvironment{Environment: s.env, githubSender: sender}
	job.Run(s.ctx)

	s.Error(job.Error())
	n, err = notification.Find(s.ctx, n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.NotZero(n.SentAt)
	s.Zero(n.SendAttempts)
	s.Zero(n.NextAttemptAt)
}

func (s *eventNotificationSuite) TestExhaustedGitHubRetriesCompleteNotification() {
	n, err := notification.Find(s.ctx, "github-status")
	s.NoError(err)
	s.Require().NotNil(n)
	for range githubNotificationRetryDelays {
		s.NoError(n.MarkRetry(s.ctx, errors.New("GitHub unavailable"), 0))
	}

	sender := &eventSendErrorSender{
		Sender: s.env.InternalSender,
		err: &send.GitHubSendError{
			StatusCode: http.StatusServiceUnavailable,
			Attempts:   evergreen.GitHubRetryAttempts,
			Retryable:  true,
			Err:        errors.New("GitHub unavailable"),
		},
	}
	job := NewEventSendJob(n.ID, "").(*eventSendJob)
	job.env = &eventSendEnvironment{Environment: s.env, githubSender: sender}
	job.Run(s.ctx)

	s.Error(job.Error())
	n, err = notification.Find(s.ctx, n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.NotZero(n.SentAt)
	s.Equal(len(githubNotificationRetryDelays), n.SendAttempts)
	s.Equal("GitHub unavailable after 3 attempt(s)", n.Error)
}

func (s *eventNotificationSuite) TestSuccessfulGitHubRetryMarksNotificationSent() {
	n, err := notification.Find(s.ctx, "github-status")
	s.NoError(err)
	s.Require().NotNil(n)
	s.NoError(n.MarkRetry(s.ctx, errors.New("GitHub unavailable"), 0))

	sender := &eventSendErrorSender{Sender: s.env.InternalSender}
	job := NewEventSendJob(n.ID, "").(*eventSendJob)
	job.env = &eventSendEnvironment{Environment: s.env, githubSender: sender}
	job.Run(s.ctx)

	s.NoError(job.Error())
	n, err = notification.Find(s.ctx, n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.NotZero(n.SentAt)
	s.Equal(1, n.SendAttempts)
	s.Empty(n.Error)
	s.Zero(n.NextAttemptAt)
}
