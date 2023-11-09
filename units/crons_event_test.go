package units

import (
	"context"
	"crypto/hmac"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type cronsEventSuite struct {
	suite.Suite
	cancel   func()
	n        []notification.Notification
	suiteCtx context.Context
	ctx      context.Context
}

func TestEventCrons(t *testing.T) {
	s := &cronsEventSuite{}
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)

	suite.Run(t, s)
}

func (s *cronsEventSuite) TearDownSuite() {
	s.cancel()
}

func (s *cronsEventSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())
	s.Require().NoError(db.ClearCollections(event.EventCollection, evergreen.ConfigCollection, notification.Collection,
		event.SubscriptionsCollection, patch.Collection, model.ProjectRefCollection))

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

	for i := range events {
		s.NoError(events[i].Log())
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

func (s *cronsEventSuite) TestDegradedMode() {
	// Reset to original flags after the test finishes.
	originalFlags, err := evergreen.GetServiceFlags(s.ctx)
	s.Require().NoError(err)
	defer func() {
		s.NoError(originalFlags.Set(s.ctx))
	}()

	flags := evergreen.ServiceFlags{
		EventProcessingDisabled: true,
	}
	s.NoError(flags.Set(s.ctx))

	e := event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		EventType:    event.PatchStateChange,
		ResourceId:   "12345",
		Data: &event.PatchEventData{
			Status: evergreen.VersionFailed,
		},
	}

	// degraded mode shouldn't process events
	s.NoError(e.Log())
	jobs, err := eventNotifierJobs(s.ctx, time.Time{})
	s.NoError(err)
	s.Len(jobs, 0)
}

func (s *cronsEventSuite) TestSenderDegradedModeDoesntDispatchJobs() {
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

	jobs, err := notificationJobs(ctx, s.n, &flags, time.Time{})
	s.NoError(err)
	s.Len(jobs, 0)

	out := []notification.Notification{}
	s.NoError(db.FindAllQ(notification.Collection, db.Q{}, &out))
	s.Len(out, 6)
	for i := range out {
		s.Equal("notification is disabled", out[i].Error)
	}
}

func (s *cronsEventSuite) TestNotificationIsEnabled() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flags := evergreen.ServiceFlags{}
	for i := range s.n {
		s.True(notificationIsEnabled(&flags, &s.n[i]))
	}

	// Reset to original flags after the test finishes.
	originalFlags, err := evergreen.GetServiceFlags(s.ctx)
	s.Require().NoError(err)
	defer func() {
		s.NoError(originalFlags.Set(ctx))
	}()

	flags = evergreen.ServiceFlags{
		JIRANotificationsDisabled:    true,
		SlackNotificationsDisabled:   true,
		EmailNotificationsDisabled:   true,
		WebhookNotificationsDisabled: true,
		GithubStatusAPIDisabled:      true,
		BackgroundStatsDisabled:      true,
	}
	s.Require().NoError(flags.Set(ctx))

	for i := range s.n {
		s.False(notificationIsEnabled(&flags, &s.n[i]))
	}
}

func (s *cronsEventSuite) TestEndToEnd() {
	defer evergreen.SetEnvironment(evergreen.GetEnvironment())

	env := &mock.Environment{}
	s.Require().NoError(env.Configure(s.ctx))
	evergreen.SetEnvironment(env)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	defer ln.Close()

	p := &patch.Patch{
		Id:      mgobson.NewObjectId(),
		Project: "test",
		Status:  evergreen.VersionFailed,
		Author:  "somebody",
	}
	s.NoError(p.Insert())

	pRef := model.ProjectRef{
		Id:         "test",
		Identifier: "testing",
	}
	s.NoError(pRef.Insert())
	e := event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		EventType:    event.PatchStateChange,
		ResourceId:   p.Id.Hex(),
		Data: &event.PatchEventData{
			Status: evergreen.VersionFailed,
		},
	}

	s.NoError(e.Log())

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

	q := evergreen.GetEnvironment().RemoteQueue()
	jobs, err := eventNotifierJobs(s.ctx, time.Time{})
	s.NoError(err)
	s.NoError(q.PutMany(s.ctx, jobs))

	// Wait for event notifier to finish.
	amboy.WaitInterval(s.ctx, q, 10*time.Millisecond)
	// Wait for event send to finish.
	amboy.WaitInterval(s.ctx, q, 10*time.Millisecond)

	out := []notification.Notification{}
	s.NoError(db.FindAllQ(notification.Collection, db.Q{}, &out))
	s.Require().Len(out, 1)

	s.NotZero(out[0].SentAt, "%+v", out[0])
	s.Empty(out[0].Error)
}

func (s *cronsEventSuite) TestSendNotificationJobs() {
	s.NoError(notification.InsertMany(s.n...))

	jobs, err := sendNotificationJobs(s.ctx, time.Time{})
	s.NoError(err)
	s.Len(jobs, 6)
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

	body, err := io.ReadAll(req.Body)
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
