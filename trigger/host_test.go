package trigger

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/suite"
)

func TestHostTriggers(t *testing.T) {
	suite.Run(t, &hostSuite{})
}

type hostSuite struct {
	subs     []event.Subscription
	t        *hostTriggers
	testData hostTemplateData
	uiConfig *evergreen.UIConfig
	ctx      context.Context
	cancel   context.CancelFunc

	suite.Suite
}

func (s *hostSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &hostTriggers{})
}

func (s *hostSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.NoError(db.ClearCollections(event.EventCollection, host.Collection, event.SubscriptionsCollection, alertrecord.Collection))

	s.t = makeHostTriggers().(*hostTriggers)
	s.t.host = &host.Host{
		Id:             "host",
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}
	s.NoError(s.t.host.Insert(s.ctx))

	s.t.event = &event.EventLogEntry{
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventHostExpirationWarningSent,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
	}

	s.subs = []event.Subscription{
		event.NewSubscriptionByID(event.ResourceTypeHost, event.TriggerExpiration, s.t.host.Id, event.Subscriber{
			Type:   event.EmailSubscriberType,
			Target: "foo@bar.com",
		}),
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	s.uiConfig = &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(s.uiConfig.Set(s.ctx))

	s.testData = hostTemplateData{
		ID:     "myHost",
		Name:   "hostName",
		Distro: "myDistro",
		URL:    "idk",
	}
}

func (s *hostSuite) TearDownTest() {
	s.cancel()
}

func (s *hostSuite) TestEmailMessage() {
	email, err := s.testData.hostExpirationEmailPayload(expiringHostEmailSubject, expiringHostEmailBody, s.t.Attributes())
	s.NoError(err)
	s.Equal("myDistro host termination reminder", email.Subject)
	s.Contains(email.Body, "Your myDistro host 'hostName' will be terminated at")
}

func (s *hostSuite) TestSlackMessage() {
	msg, err := s.testData.hostExpirationSlackPayload(expiringHostSlackBody, "linkTitle")
	s.NoError(err)
	s.Contains(msg.Body, "Your myDistro host 'hostName' will be terminated at")
}

func (s *hostSuite) TestFetch() {
	triggers := hostTriggers{}
	s.NoError(triggers.Fetch(s.ctx, s.t.event))
	s.Equal(s.t.host.Id, triggers.templateData.ID)
	s.Equal(fmt.Sprintf("%s/spawn#?resourcetype=hosts&id=%s", s.uiConfig.Url, s.t.host.Id), triggers.templateData.URL)
}

func (s *hostSuite) TestAllTriggers() {
	// valid event should trigger a notification
	n, err := NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Len(n, 1)
}

func (s *hostSuite) TestHostExpiration() {
	n, err := s.t.hostExpiration(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}
