package units

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventMetaJobSuite struct {
	suite.Suite
	cancel func()
	n      []notification.Notification
}

func TestEventMetaJob(t *testing.T) {
	suite.Run(t, &eventMetaJobSuite{})
}

func (s *eventMetaJobSuite) TearDownTest() {
	s.cancel()
}

func (s *eventMetaJobSuite) SetupTest() {
	evergreen.ResetEnvironment()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))

	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())

	s.NoError(db.ClearCollections(event.AllLogCollection, event.TaskLogCollection, evergreen.ConfigCollection, notification.NotificationsCollection, event.SubscriptionsCollection))

	events := []event.EventLogEntry{
		{
			ResourceType: event.ResourceTypeHost,
			Data: &event.HostEventData{
				ResourceType: event.ResourceTypeHost,
			},
		},
		{
			ProcessedAt:  time.Now(),
			ResourceType: event.ResourceTypeHost,
			Data: &event.HostEventData{
				ResourceType: event.ResourceTypeHost,
			},
		},
	}

	logger := event.NewDBEventLogger(event.AllLogCollection)
	logger2 := event.NewDBEventLogger(event.TaskLogCollection)
	for i := range events {
		s.NoError(logger.LogEvent(&events[i]))
		s.NoError(logger2.LogEvent(&events[i]))
	}

	s.n = []notification.Notification{
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.JIRACommentSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.SlackSubscriberType,
			},
		},
	}
}

func (s *eventMetaJobSuite) TestDegradedMode() {
	flags := evergreen.ServiceFlags{
		EventProcessingDisabled: true,
	}
	s.NoError(flags.Set())

	job := NewEventMetaJob(event.AllLogCollection)
	job.Run()
	s.NoError(job.Error())

	out := []event.EventLogEntry{}
	s.NoError(db.FindAllQ(event.AllLogCollection, db.Query(event.UnprocessedEvents()), &out))
	s.Empty(out)

	s.NoError(db.FindAllQ(event.TaskLogCollection, db.Query(event.UnprocessedEvents()), &out))
	s.Len(out, 1)
}

func (s *eventMetaJobSuite) TestSenderDegradedModeDoesntDispatchJobs() {
	flags := evergreen.ServiceFlags{
		JIRANotificationsDisabled:    true,
		SlackNotificationsDisabled:   true,
		EmailNotificationsDisabled:   true,
		WebhookNotificationsDisabled: true,
		GithubStatusAPIDisabled:      true,
		BackgroundStatsDisabled:      true,
	}

	s.NoError(notification.InsertMany(s.n...))

	startingStats := evergreen.GetEnvironment().RemoteQueue().Stats()

	job := NewEventMetaJob(event.AllLogCollection).(*eventMetaJob)
	job.flags = &flags
	s.NoError(job.dispatch(s.n))
	s.NoError(job.Error())

	out := []notification.Notification{}
	s.NoError(db.FindAllQ(notification.NotificationsCollection, db.Q{}, &out))
	s.Len(out, 6)
	for i := range out {
		s.Equal("sender disabled", out[i].Error)
	}

	stats := evergreen.GetEnvironment().RemoteQueue().Stats()
	s.Equal(startingStats.Running, stats.Running)
	s.Equal(startingStats.Blocked, stats.Blocked)
	s.Equal(startingStats.Completed, stats.Completed)
	s.Equal(startingStats.Pending, stats.Pending)
	s.Equal(startingStats.Total, stats.Total)
}

func (s *eventMetaJobSuite) TestNotificationIsEnabled() {
	flags := evergreen.ServiceFlags{}
	for i := range s.n {
		s.True(notificationIsEnabled(&flags, &s.n[i]))
	}

	flags = evergreen.ServiceFlags{
		JIRANotificationsDisabled:    true,
		SlackNotificationsDisabled:   true,
		EmailNotificationsDisabled:   true,
		WebhookNotificationsDisabled: true,
		GithubStatusAPIDisabled:      true,
		BackgroundStatsDisabled:      true,
	}
	s.Require().NoError(flags.Set())

	for i := range s.n {
		s.False(notificationIsEnabled(&flags, &s.n[i]))
	}
}

func (s *eventMetaJobSuite) TestEndToEnd() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()

	addr := strings.Split(ln.Addr().String(), ":")
	s.Require().Len(addr, 2)

	port, err := strconv.Atoi(addr[1])
	s.Require().NoError(err)

	notifyConfig := evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			From:     "evergreen@example.com",
			Server:   "127.0.0.1",
			Port:     port,
			Username: "much",
			Password: "security",
		},
	}
	s.Require().NoError(notifyConfig.Set())

	s.Require().True(evergreen.GetEnvironment().RemoteQueue().Started())
	s.NoError(db.ClearCollections(event.AllLogCollection, event.TaskLogCollection, notification.NotificationsCollection, event.SubscriptionsCollection))

	e := event.EventLogEntry{
		ResourceType: event.ResourceTypeTest,
		Data: &event.TestEvent{
			ResourceType: event.ResourceTypeTest,
			Message:      "i'm an event driven notification",
		},
	}

	logger := event.NewDBEventLogger(event.AllLogCollection)
	s.NoError(logger.LogEvent(&e))

	subs := []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    "test",
			Trigger: "test",
			Selectors: []event.Selector{
				{
					Type: "test",
					Data: "awesomeness",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "test1@no.op",
			},
		},
		{
			ID:      bson.NewObjectId(),
			Type:    "test",
			Trigger: "test",
			Selectors: []event.Selector{
				{
					Type: "test",
					Data: "awesomeness",
				},
				{
					Type: "test2",
					Data: "dontpickme",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "test3@no.op",
			},
		},
	}

	for i := range subs {
		s.NoError(subs[i].Upsert())
	}

	bodyC := make(chan string, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go smtpServer(ctx, ln, bodyC)

	job := NewEventMetaJob(event.AllLogCollection)
	job.Run()
	s.NoError(job.Error())

	bodyText := <-bodyC

	body := parseEmailBody(bodyText)
	s.Equal(`"evergreen" <evergreen@example.com>`, body["From"])
	s.Equal(`<test1@no.op>`, body["To"])
	s.Equal(`Hi`, body["Subject"])
	s.Equal(`event says 'i'm an event driven notification'`, body["body"])

	time.Sleep(5 * time.Second)
	out := []notification.Notification{}
	s.NoError(db.FindAllQ(notification.NotificationsCollection, db.Q{}, &out))
	s.Require().Len(out, 1)

	s.NotZero(out[0].SentAt)
	s.Empty(out[0].Error)
}

func smtpServer(ctx context.Context, ln net.Listener, bodyOut chan string) {
	d, _ := ctx.Deadline()
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()
	defer ln.Close()

	conn, err := ln.Accept()
	if err != nil {
		grip.Error(err)
		bodyOut <- "Err: " + err.Error()
		return
	}
	defer conn.Close()

	grip.Info("Accepted connection")

	_, err = conn.Write([]byte("220 127.0.0.1 ESMTP imsorrysmtp\r\n"))
	if err != nil {
		grip.Error(err)
		bodyOut <- "Err: " + err.Error()
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}
		message, err := bufio.NewReader(conn).ReadString('\n')
		grip.Error(err)
		if err != nil {
			bodyOut <- "Err: " + err.Error()
			return
		}
		grip.Infof("C: %s", message)

		var out string
		if strings.HasPrefix(message, "EHLO ") {
			out = "250-localhost Hello localhost\r\n250-SIZE 5000\r\n250 AUTH LOGIN PLAIN"

		} else if strings.HasPrefix(message, "AUTH ") {
			out = "235 2.7.0 Authentication successful"

		} else if strings.HasPrefix(message, "DATA") {
			out = "354 End data with <CR><LF>.<CR><LF>"
			_, err = conn.Write([]byte(out + "\r\n"))
			grip.Error(err)
			if err != nil {
				bodyOut <- "Err: " + err.Error()
				return
			}

			input := ""
			bytes := make([]byte, 1)
			for !strings.HasSuffix(input, "\r\n.\r\n") {
				_, err = conn.Read(bytes)
				grip.Error(err)
				if err != nil {
					bodyOut <- "Err: " + err.Error()
					return
				}
				input += string(bytes)
			}
			grip.Infof("C: %s", input)
			bodyOut <- input

			out = "250 Ok: queued as 9001"

		} else if strings.HasPrefix(message, "QUIT") {
			out = "221 Bye"

		} else {
			out = "250 Ok"
		}

		if out != "" {
			grip.Infof("S: %s\n", out)
			_, err = conn.Write([]byte(out + "\r\n"))
			grip.Error(err)
			if out == "221 Bye" || err != nil {
				bodyOut <- "Err: " + err.Error()
				return
			}
		}
	}
}

func parseEmailBody(body string) map[string]string {
	m := map[string]string{}
	c := regexp.MustCompile(".+:.+")
	s := strings.Split(body, "\r\n")
	for i := range s {
		if c.MatchString(s[i]) {
			header := strings.SplitN(s[i], ":", 2)
			m[header[0]] = strings.Trim(header[1], " ")

		} else {
			body := ""
			for ; s[i] != "." || i == len(s); i++ {
				body += s[i]
			}
			if enc, ok := m["Content-Transfer-Encoding"]; ok && enc == "base64" {
				data, err := base64.StdEncoding.DecodeString(body)
				grip.Error(err)
				if err == nil {
					m["body"] = string(data)
				}
			} else {
				m["body"] = body
			}
			break
		}
	}
	return m
}
