package units

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type eventNotificationSuite struct {
	suite.Suite

	ctx    context.Context
	cancel func()

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
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *eventNotificationSuite) SetupTest() {
	s.env = &mock.Environment{}
	s.NoError(s.env.Configure(s.ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))

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
					"such": []string{"much"},
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
			Payload: fmt.Sprintf("eventNotificationSuite jira comment message"),
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
					Ref:      "master",
				},
			},
			Payload: message.GithubStatus{
				Context: "evergreen",
				URL:     "https://example.com",
				State:   message.GithubStateFailure,
			},
		},
		{
			ID: "github-pr-merge",
			Subscriber: event.Subscriber{
				Type: event.GithubMergeSubscriberType,
				Target: event.GithubMergeSubscriber{
					ProjectID:     "mci",
					Owner:         "evergreen-ci",
					Repo:          "evergreen",
					PRNumber:      1234,
					CommitMessage: "merged your PR",
				},
			},
			Payload: commitqueue.GithubMergePR{},
		},
	}
	s.webhook = &s.notifications[0]
	s.email = &s.notifications[1]
	s.slack = &s.notifications[2]
	s.jiraComment = &s.notifications[3]
	s.jiraIssue = &s.notifications[4]

	s.NoError(notification.InsertMany(s.notifications...))
}

func (s *eventNotificationSuite) notificationHasError(id string, pattern string) time.Time {
	n, err := notification.Find(id)
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
	s.NoError(flags.Set())

	for i := range s.notifications {
		job := NewEventNotificationJob(s.notifications[i].ID).(*eventNotificationJob)
		job.env = s.env

		job.Run(s.ctx)
		s.NoError(job.Error())
	}

	s.NotZero(s.notificationHasError(s.webhook.ID, "sender is disabled, not sending notification"))
}

func (s *eventNotificationSuite) TestEvergreenWebhook() {
	job := NewEventNotificationJob(s.webhook.ID).(*eventNotificationJob)
	job.env = s.env

	job.Run(s.ctx)
	s.NoError(job.Error())

	s.NotZero(s.notificationHasError(s.webhook.ID, ""))
	s.Nil(job.Error())

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		_ = msg.Message.Raw().(*util.EvergreenWebhook)
	})
}

func (s *eventNotificationSuite) TestSlack() {
	job := NewEventNotificationJob(s.slack.ID).(*eventNotificationJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.slack.ID, ""))

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
	job := NewEventNotificationJob(s.jiraComment.ID).(*eventNotificationJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.jiraComment.ID, ""))

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		jira := msg.Message.Raw().(*message.JIRAComment)
		s.Equal("eventNotificationSuite jira comment message", jira.Body)
		s.Equal("EVG-2863", jira.IssueID)
	})
}

func (s *eventNotificationSuite) TestJIRAIssue() {
	job := NewEventNotificationJob(s.jiraIssue.ID).(*eventNotificationJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.jiraIssue.ID, ""))

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
		s.NoError(notification.InsertMany(n[i]))

		job := NewEventNotificationJob(n[i].ID).(*eventNotificationJob)
		job.env = s.env
		job.Run(s.ctx)
		s.Error(job.Error())

		_, recv := s.env.InternalSender.GetMessageSafe()
		s.False(recv)
	}

	s.NotZero(s.notificationHasError(s.webhook.ID, "^composer is not loggable$"))
}
