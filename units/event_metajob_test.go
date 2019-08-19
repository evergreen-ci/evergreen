package units

import (
	"context"
	"crypto/hmac"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type eventMetaJobSuite struct {
	suite.Suite
	cancel func()
	n      []notification.Notification
	ctx    context.Context
	env    evergreen.Environment
}

func TestEventMetaJob(t *testing.T) {
	s := &eventMetaJobSuite{}
	s.ctx, s.cancel = context.WithCancel(context.Background())

	env := evergreen.GetEnvironment()
	require.NoError(t, env.RemoteQueue().Start(s.ctx))
	s.env = env
	suite.Run(t, s)
}

func (s *eventMetaJobSuite) TearDownSuite() {
	s.cancel()
}

func (s *eventMetaJobSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, event.TaskLogCollection, evergreen.ConfigCollection, notification.Collection, event.SubscriptionsCollection, patch.Collection))

	events := []event.EventLogEntry{
		{
			ResourceType: event.ResourceTypeHost,
			Data:         &event.HostEventData{},
		},
		{
			ProcessedAt:  time.Now(),
			ResourceType: event.ResourceTypeHost,
			Data:         &event.HostEventData{},
		},
	}

	logger := event.NewDBEventLogger(event.AllLogCollection)
	for i := range events {
		s.NoError(logger.LogEvent(&events[i]))
	}

	s.n = []notification.Notification{
		{
			ID: "1",
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
			},
		},
		{
			ID: "2",
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
			},
		},
		{
			ID: "3",
			Subscriber: event.Subscriber{
				Type: event.JIRACommentSubscriberType,
			},
		},
		{
			ID: "4",
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
		},
		{
			ID: "5",
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
		},
		{
			ID: "6",
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

	e := event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		EventType:    event.PatchStateChange,
		ResourceId:   "12345",
		Data: &event.PatchEventData{
			Status: evergreen.PatchFailed,
		},
	}

	// degraded mode shouldn't process events
	logger := event.NewDBEventLogger(event.AllLogCollection)
	s.NoError(logger.LogEvent(&e))

	job := NewEventMetaJob(evergreen.GetEnvironment().RemoteQueue(), "1")
	job.Run(s.ctx)
	s.NoError(job.Error())

	out, err := event.FindUnprocessedEvents()
	s.NoError(err)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(notification.InsertMany(s.n...))

	startingStats := evergreen.GetEnvironment().RemoteQueue().Stats(ctx)

	job := NewEventMetaJob(evergreen.GetEnvironment().RemoteQueue(), "1").(*eventMetaJob)
	job.flags = &flags
	s.NoError(job.dispatch(ctx, s.n))
	s.NoError(job.Error())

	out := []notification.Notification{}
	s.NoError(db.FindAllQ(notification.Collection, db.Q{}, &out))
	s.Len(out, 6)
	for i := range out {
		s.Equal("sender disabled", out[i].Error)
	}

	stats := evergreen.GetEnvironment().RemoteQueue().Stats(ctx)
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

	p := &patch.Patch{
		Id:      mgobson.NewObjectId(),
		Project: "test",
		Status:  evergreen.PatchFailed,
		Author:  "somebody",
	}
	s.NoError(p.Insert())

	e := event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		EventType:    event.PatchStateChange,
		ResourceId:   p.Id.Hex(),
		Data: &event.PatchEventData{
			Status: evergreen.PatchFailed,
		},
	}

	logger := event.NewDBEventLogger(event.AllLogCollection)
	s.NoError(logger.LogEvent(&e))

	subs := []event.Subscription{
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: e.ResourceType,
			Trigger:      "outcome",
			Selectors: []event.Selector{
				{
					Type: "owner",
					Data: "somebody",
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    fmt.Sprintf("http://%s", ln.Addr()),
					Secret: []byte("darkmagic"),
				},
			},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: e.ResourceType,
			Trigger:      "outcome",
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
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    fmt.Sprintf("http://%s", ln.Addr()),
					Secret: []byte("darkermagic"),
				},
			},
		},
	}

	for i := range subs {
		s.NoError(subs[i].Upsert())
	}

	handler := &mockWebhookHandler{
		secret: []byte("darkmagic"),
	}

	go httpServer(ln, handler)

	job := NewEventMetaJob(evergreen.GetEnvironment().LocalQueue(), "1").(*eventMetaJob)
	job.q = evergreen.GetEnvironment().LocalQueue()
	job.Run(s.ctx)
	s.NoError(job.Error())

	grip.Info("waiting for dispatches")
	amboy.WaitInterval(s.ctx, job.q, 10*time.Millisecond)
	grip.Info("waiting for senders")
	amboy.Wait(s.ctx, job.q)
	grip.Info("senders are done")

	out := []notification.Notification{}
	s.NoError(db.FindAllQ(notification.Collection, db.Q{}, &out))
	s.Require().Len(out, 1)

	s.NotZero(out[0].SentAt, "%+v", out[0])
	s.Empty(out[0].Error)
}

func httpServer(ln net.Listener, handler *mockWebhookHandler) {
	err := http.Serve(ln, handler)
	grip.Error(err)
	if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
		panic(err)
	}
}

type mockWebhookHandler struct {
	secret []byte

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
	const (
		evergreenNotificationIDHeader = "X-Evergreen-Notification-ID"
		evergreenHMACHeader           = "X-Evergreen-Signature"
	)
	defer req.Body.Close()

	if req.Method != http.MethodPost {
		m.error(errors.Errorf("expected method POST, got %s", req.Method), w)
		return
	}

	mid := req.Header.Get(evergreenNotificationIDHeader)
	if len(mid) == 0 {
		m.error(errors.New("no message id"), w)
		return
	}
	sig := []byte(req.Header.Get(evergreenHMACHeader))
	if len(sig) == 0 {
		m.error(errors.New("no signature"), w)
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		m.error(err, w)
		return
	}
	hash, err := util.CalculateHMACHash(m.secret, body)
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
}
