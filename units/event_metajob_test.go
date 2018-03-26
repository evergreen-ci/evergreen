package units

import (
	"context"
	"crypto/hmac"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
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
	s.EqualError(job.Error(), "events processing is disabled, all events will be marked processed")

	out := []event.EventLogEntry{}
	s.NoError(db.FindAllQ(event.AllLogCollection, db.Query(event.UnprocessedEvents()), &out))
	s.Empty(out)

	s.NoError(db.FindAllQ(event.TaskLogCollection, db.Query(event.UnprocessedEvents()), &out))
	s.Len(out, 1)
}

type eventNotificationSuite struct {
	suite.Suite

	webhook notification.Notification
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

	s.NoError(notification.InsertMany(s.webhook))
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

func (s *eventNotificationSuite) notificationHasError(id bson.ObjectId, e string) time.Time {
	n, err := notification.Find(id)
	s.Require().NoError(err)
	s.Require().NotNil(n)

	s.Equal(e, n.Error)
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
}

func (s *eventNotificationSuite) TestEvergreenWebhookWithDeadServer() {
	job := newEventNotificationJob(s.webhook.ID)
	job.Run()
	s.EqualError(job.Error(), "evergreen-webhook failed to send webhook data: Post http://127.0.0.1:12345: dial tcp 127.0.0.1:12345: connect: connection refused")
	s.NotZero(s.notificationHasError(s.webhook.ID, "evergreen-webhook failed to send webhook data: Post http://127.0.0.1:12345: dial tcp 127.0.0.1:12345: connect: connection refused"))
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
	s.NoError(err)
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

	s.NoError(ln.Close())
}
