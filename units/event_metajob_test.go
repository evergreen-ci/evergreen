package units

import (
	"bufio"
	"context"
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventMetaJobSuite struct {
	suite.Suite
	cancel func()
}

func TestEventMetaJob(t *testing.T) {
	suite.Run(t, &eventMetaJobSuite{})
}

func (s *eventMetaJobSuite) SetupSuite() {
	evergreen.ResetEnvironment()

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))

	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *eventMetaJobSuite) TearDownSuite() {
	s.cancel()
}

func (s *eventMetaJobSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, event.TaskLogCollection, evergreen.ConfigCollection))

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

// TODO: No events implemented, can't test this yet
//func (s *eventMetaJobSuite) TestSenderDegradedModePreventsJobInsertion() {
//	job := NewEventMetaJob(event.AllLogCollection)
//	job.Run()
//	s.EqualError(job.Error())
//}

func (s *eventMetaJobSuite) TestNotificationIsEnabled() {
	flags := evergreen.ServiceFlags{}
	n := []notification.Notification{
		{
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
			},
		},
		{
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
			},
		},
		{
			Subscriber: event.Subscriber{
				Type: event.JIRACommentSubscriberType,
			},
		},
		{
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
		},
		{
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
		},
		{
			Subscriber: event.Subscriber{
				Type: event.SlackSubscriberType,
			},
		},
	}
	for i := range n {
		s.True(notificationIsEnabled(&flags, &n[i]))
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

	for i := range n {
		s.False(notificationIsEnabled(&flags, &n[i]))
	}
}

type eventNotificationSuite struct {
	suite.Suite

	webhook notification.Notification
	email   notification.Notification
}

func TestEventNotificationJob(t *testing.T) {
	suite.Run(t, &eventNotificationSuite{})
}

func (s *eventNotificationSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *eventNotificationSuite) SetupTest() {
	s.NoError(db.ClearCollections(notification.NotificationsCollection, evergreen.ConfigCollection))
	s.webhook = notification.Notification{
		ID: bson.NewObjectId(),
		Subscriber: event.Subscriber{
			Type: event.EvergreenWebhookSubscriberType,
			Target: event.WebhookSubscriber{
				URL:    "http://127.0.0.1:12345",
				Secret: []byte("memes"),
			},
		},
		Payload: "o hai",
	}
	s.email = notification.Notification{
		ID: bson.NewObjectId(),
		Subscriber: event.Subscriber{
			Type:   event.EmailSubscriberType,
			Target: "o@hai.hai",
		},
		Payload: notification.EmailPayload{
			Subject: "o hai",
			Body:    "i'm a notification",
		},
	}

	s.NoError(notification.InsertMany(s.webhook, s.email))
}

type mockWebhookHandler struct {
	secret []byte
	m      sync.Mutex

	body []byte
	err  error
}

func (m *mockWebhookHandler) error(outErr error, w http.ResponseWriter) {
	if outErr == nil {
		return
	}
	w.WriteHeader(http.StatusBadRequest)

	_, err := w.Write([]byte(outErr.Error()))
	grip.Error(err)
	m.err = outErr
}

func (m *mockWebhookHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if req.Method != http.MethodPost {
		m.error(errors.Errorf("expected method POST, got %s", req.Method), w)
		return
	}

	mid := req.Header.Get("X-Evergreen-Notification-ID")
	if len(mid) == 0 {
		m.error(errors.New("no message id"), w)
		return
	}
	sig := []byte(req.Header.Get("X-Evergreen-Signature"))
	if len(sig) == 0 {
		m.error(errors.New("no signature"), w)
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		m.error(err, w)
		return
	}
	hash, err := calculateHMACHash(m.secret, body)
	if err != nil {
		m.error(err, w)
		return
	}

	if !hmac.Equal([]byte(hash), sig) {
		m.error(errors.Errorf("expected signature: %s, got %s", sig, hash), w)
		return
	}
	m.body = body

	w.WriteHeader(http.StatusNoContent)
	grip.Info(message.Fields{
		"message":   fmt.Sprintf("received %s", mid),
		"signature": string(sig),
		"body":      string(body),
	})
	return
}

func (s *eventNotificationSuite) notificationHasError(id bson.ObjectId, pattern string) time.Time {
	n, err := notification.Find(id)
	s.Require().NoError(err)
	s.Require().NotNil(n)

	if len(pattern) == 0 {
		s.Empty(n.Error)

	} else {
		match, err := regexp.MatchString(pattern, n.Error)
		s.NoError(err)
		s.True(match)
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

	job := newEventNotificationJob(s.webhook.ID)
	job.Run()
	s.EqualError(job.Error(), "sender is disabled, not sending notification")

	s.NotZero(s.notificationHasError(s.webhook.ID, "sender is disabled, not sending notification"))

	job = newEventNotificationJob(s.email.ID)
	job.Run()
	s.EqualError(job.Error(), "sender is disabled, not sending notification")

	s.NotZero(s.notificationHasError(s.webhook.ID, "sender is disabled, not sending notification"))
}

func (s *eventNotificationSuite) TestEvergreenWebhookWithDeadServer() {
	job := newEventNotificationJob(s.webhook.ID)
	job.Run()
	s.Require().NotNil(job.Error())
	errMsg := job.Error().Error()

	pattern := "evergreen-webhook failed to send webhook data: Post http://127.0.0.1:12345: dial tcp 127.0.0.1:12345: [a-zA-Z]+: connection refused"
	s.True(regexp.MatchString(pattern, errMsg))
	s.NotZero(s.notificationHasError(s.webhook.ID, pattern))
}

func (s *eventNotificationSuite) TestEvergreenWebhook() {
	job := newEventNotificationJob(s.webhook.ID)

	handler := &mockWebhookHandler{
		secret: []byte("memes"),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.NoError(err)
	s.NoError(db.UpdateId(notification.NotificationsCollection, s.webhook.ID, bson.M{
		"$set": bson.M{
			"subscriber.target.url": "http://" + ln.Addr().String(),
		},
	}))

	go func() {
		err := http.Serve(ln, handler)
		grip.Info(err)
	}()

	job.Run()
	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.webhook.ID, ""))
	s.Nil(job.Error())

	s.NoError(ln.Close())
}

func (s *eventNotificationSuite) TestEvergreenWebhookWithBadSecret() {
	job := newEventNotificationJob(s.webhook.ID)

	handler := &mockWebhookHandler{
		secret: []byte("somethingelse"),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()
	s.NoError(db.UpdateId(notification.NotificationsCollection, s.webhook.ID, bson.M{
		"$set": bson.M{
			"subscriber.target.url": "http://" + ln.Addr().String(),
		},
	}))

	go func() {
		err := http.Serve(ln, handler)
		if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
			s.T().Errorf("unexpected error: %+v", err)
		}
	}()

	job.Run()
	s.EqualError(job.Error(), "evergreen-webhook response status was 400")
	s.NotZero(s.notificationHasError(s.webhook.ID, "evergreen-webhook response status was 400"))
	s.Error(job.Error())
}

func (s *eventNotificationSuite) TestEmail() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()

	addr := strings.Split(ln.Addr().String(), ":")
	s.Require().Len(addr, 2)

	port, err := strconv.Atoi(addr[1])
	s.Require().NoError(err)

	job := newEventNotificationJob(s.email.ID).(*eventNotificationJob)
	job.settings, err = evergreen.GetConfig()
	s.NoError(err)
	job.settings.Notify = evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			From:     "evergreen@example.com",
			Server:   "127.0.0.1",
			Port:     port,
			Username: "much",
			Password: "security",
		},
	}
	body := make(chan string, 1)

	go func() {
		s.EqualError(smtpServer(ln, body), "EOF")
	}()

	job.Run()

	bodyText := <-body

	s.NotEmpty(bodyText)

	split := strings.Split(bodyText, "\r\n")
	for i := range split {
		if strings.HasPrefix(split[i], "To: ") {
			s.Equal("To: <o@hai.hai>", split[i])

		} else if strings.HasPrefix(split[i], "Subject: ") {
			s.Equal("Subject: o hai", split[i])

		} else if match, _ := regexp.MatchString(".*:", split[i]); !match {
			base64Body := ""
			for ; split[i] != "." || i == len(split); i++ {
				base64Body += split[i]
			}
			data, err := base64.StdEncoding.DecodeString(base64Body)
			s.NoError(err)
			s.Equal("i'm a notification", string(data))
			break
		}
	}

	s.NoError(job.Error())
	s.NotZero(s.notificationHasError(s.email.ID, ""))
	s.Nil(job.Error())
}

func (s *eventNotificationSuite) TestEmailWithUnreachableSMTP() {
	var err error
	job := newEventNotificationJob(s.email.ID).(*eventNotificationJob)
	job.settings, err = evergreen.GetConfig()
	s.NoError(err)
	job.settings.Notify = evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			From:     "evergreen@example.com",
			Server:   "127.0.0.1",
			Port:     12345,
			Username: "much",
			Password: "security",
		},
	}

	job.Run()
	s.Require().Error(job.Error())

	errMsg := job.Error().Error()
	pattern := "email settings are invalid: dial tcp 127.0.0.1:12345: [a-zA-Z]+: connection refused"
	s.True(regexp.MatchString(pattern, errMsg))
	s.NotZero(s.notificationHasError(s.email.ID, pattern))
}

func smtpServer(ln net.Listener, bodyOut chan string) error {
	defer ln.Close()

	conn, err := ln.Accept()
	if err != nil {
		grip.Error(err)
		return err
	}
	defer conn.Close()

	grip.Info("Accepted connection")

	_, err = conn.Write([]byte("220 127.0.0.1 ESMTP Postfix\r\n"))
	if err != nil {
		grip.Error(err)
		return err
	}

	for {
		message, err := bufio.NewReader(conn).ReadString('\n')
		grip.Error(err)
		if err != nil {
			return err
		}
		grip.Infof("C: %s", message)

		msg := ""
		if strings.HasPrefix(message, "EHLO ") {
			msg = "250-localhost Hello localhost\r\n250-SIZE 5000\r\n250 AUTH LOGIN PLAIN"

		} else if strings.HasPrefix(message, "AUTH ") {
			msg = "235 2.7.0 Authentication successful"

		} else if strings.HasPrefix(message, "DATA") {
			msg = "354 End data with <CR><LF>.<CR><LF>"
			_, err = conn.Write([]byte(msg + "\r\n"))
			grip.Error(err)
			if err != nil {
				return err
			}

			message := ""
			bytes := make([]byte, 1)
			for !strings.HasSuffix(message, "\r\n.\r\n") {
				_, err = conn.Read(bytes)
				grip.Error(err)
				if err != nil {
					return err
				}
				message += string(bytes)
			}
			grip.Infof("C: %s", message)
			bodyOut <- message

			msg = "250 Ok: queued as 9001"

		} else if strings.HasPrefix(message, "QUIT") {
			msg = "221 Bye"

		} else {
			msg = "250 Ok"
		}

		if msg != "" {
			grip.Infof("S: %s\n", msg)
			_, err = conn.Write([]byte(msg + "\r\n"))
			grip.Error(err)
			if msg == "221 Bye" || err != nil {
				return err
			}
		}
	}
}
