package units

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
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
	"gopkg.in/mgo.v2/bson"
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
	githubStatus  *notification.Notification
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
			Payload: "o hai",
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
				Type:   event.JIRAIssueSubscriberType,
				Target: "SERVER",
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
	}
	s.webhook = &s.notifications[0]
	s.email = &s.notifications[1]
	s.slack = &s.notifications[2]
	s.jiraComment = &s.notifications[3]
	s.jiraIssue = &s.notifications[4]
	s.githubStatus = &s.notifications[5]

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
		s.True(match, "error doesn't match regex")
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
		job := newEventNotificationJob(s.notifications[i].ID).(*eventNotificationJob)
		job.env = s.env

		job.Run(s.ctx)
		s.EqualError(job.Error(), "sender is disabled, not sending notification")

		s.NotZero(s.notificationHasError(s.webhook.ID, "sender is disabled, not sending notification"))
	}
}

func (s *eventNotificationSuite) TestEvergreenWebhook() {
	job := newEventNotificationJob(s.webhook.ID).(*eventNotificationJob)
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

func (s *eventNotificationSuite) TestEvergreenWebhookWithDeadServer() {
	job := newEventNotificationJob(s.webhook.ID).(*eventNotificationJob)
	job.Run(s.ctx)
	s.NoError(job.Error())

	pattern := "evergreen-webhook failed to send webhook data: Post http://127.0.0.1:12345: dial tcp 127.0.0.1:12345: [a-zA-Z]+: connection refused"
	s.NotZero(s.notificationHasError(s.webhook.ID, pattern))
}

func (s *eventNotificationSuite) TestEvergreenWebhookWithBadSecret() {
	job := newEventNotificationJob(s.webhook.ID).(*eventNotificationJob)
	//job.env = s.env

	handler := &mockWebhookHandler{
		secret: []byte("somethingelse"),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()
	s.NoError(db.UpdateId(notification.Collection, s.webhook.ID, bson.M{
		"$set": bson.M{
			"subscriber.target.url": "http://" + ln.Addr().String(),
		},
	}))

	s.NotPanics(func() {
		go httpServer(ln, handler)
	})

	job.Run(s.ctx)
	s.NoError(job.Error())

	n, err := notification.Find(s.webhook.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal("evergreen-webhook response status was 400 Bad Request", n.Error)
	s.NotZero(s.notificationHasError(s.webhook.ID, "evergreen-webhook response status was 400"))
}

func (s *eventNotificationSuite) TestEmail() {
	job := newEventNotificationJob(s.email.ID).(*eventNotificationJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.email.ID, ""))
	s.Nil(job.Error())

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		email := msg.Message.Raw().(*message.Email)

		s.Equal("i'm a notification", email.Body)
		s.Equal("o hai", email.Subject)
		s.Equal([]string{"o@hai.hai"}, email.Recipients)
		s.Len(email.Headers, 1)
		s.Equal([]string{"much"}, email.Headers["such"])
	})
}

func (s *eventNotificationSuite) TestEmailWithUnreachableSMTP() {
	job := newEventNotificationJob(s.email.ID).(*eventNotificationJob)
	notify := evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			From:     "evergreen@example.com",
			Server:   "127.0.0.1",
			Port:     12345,
			Username: "much",
			Password: "security",
		},
	}
	s.Require().NoError(notify.Set())

	job.Run(s.ctx)
	s.Require().Error(job.Error())

	errMsg := job.Error().Error()
	s.Require().NotEmpty(errMsg)
	pattern := "error building sender for notification: dial tcp 127.0.0.1:12345: [a-zA-Z-_]+: connection refused"
	match, err := regexp.MatchString(pattern, errMsg)
	s.NoError(err)
	s.True(match)
	s.NotZero(s.notificationHasError(s.email.ID, pattern))
}

func (s *eventNotificationSuite) TestSlack() {
	job := newEventNotificationJob(s.slack.ID).(*eventNotificationJob)
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
	job := newEventNotificationJob(s.jiraComment.ID).(*eventNotificationJob)
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
	job := newEventNotificationJob(s.jiraIssue.ID).(*eventNotificationJob)
	job.env = s.env
	job.Run(s.ctx)

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.jiraIssue.ID, ""))

	msg, recv := s.env.InternalSender.GetMessageSafe()
	s.True(recv)
	s.NotPanics(func() {
		jira := msg.Message.Raw().(message.JiraIssue)
		s.Equal("SERVER", jira.Project)
		s.Equal("Tell the evergreen team that they're awesome", jira.Summary)
		s.Equal("The evergreen team is awesome. Inform them of it", jira.Description)
		s.Equal("eliot.horowitz", jira.Reporter)
	})
}

func (s *eventNotificationSuite) TestSendFailureResultsInNoMessages() {
	s.Require().NoError(db.ClearCollections(notification.Collection))
	for i := range s.notifications {
		// make the payload malformed
		s.notifications[i].Payload = nil
		s.NoError(notification.InsertMany(s.notifications[i]))

		job := newEventNotificationJob(s.notifications[i].ID).(*eventNotificationJob)
		job.env = s.env
		job.Run(s.ctx)
		s.Error(job.Error())

		_, recv := s.env.InternalSender.GetMessageSafe()
		s.False(recv)
		s.NotZero(s.notificationHasError(s.webhook.ID, "^composer is not loggable$"))
	}
}
