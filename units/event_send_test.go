package units

import (
	"context"
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
	"github.com/stretchr/testify/suite"
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
					URL:    "http://127.0.0.1:12345",
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
