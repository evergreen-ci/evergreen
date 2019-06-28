package trigger

import (
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

	suite.Suite
}

func (s *hostSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &hostTriggers{})
}

func (s *hostSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, host.Collection, event.SubscriptionsCollection, alertrecord.Collection))

	s.t = makeHostTriggers().(*hostTriggers)
	s.t.host = &host.Host{
		Id:             "host",
		ExpirationTime: time.Now().Add(12 * time.Hour),
	}
	s.NoError(s.t.host.Insert())

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

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set())

	s.testData = hostTemplateData{
		ID:             "myHost",
		Distro:         "myDistro",
		ExpirationTime: time.Now().Add(2 * time.Hour),
		URL:            "idk",
	}
}

func (s *hostSuite) TestEmailMessage() {
	email, err := hostExpirationEmailPayload(s.testData, expiringHostTitle, expiringHostBody, s.t.Selectors())
	s.NoError(err)
	s.Equal("myDistro host termination reminder", email.Subject)
	s.Contains(email.Body, "Your myDistro host with id myHost will be terminated at")
}

func (s *hostSuite) TestSlackMessage() {
	msg, err := hostExpirationSlackPayload(s.testData, expiringHostBody, s.t.Selectors())
	s.NoError(err)
	s.Contains(msg.Body, "Your myDistro host with id myHost will be terminated at")
}

func (s *hostSuite) TestAllTriggers() {
	// valid event should trigger a notification
	n, err := NotificationsFromEvent(s.t.event)
	s.NoError(err)
	s.Len(n, 1)
}

func (s *hostSuite) TestHostExpiration() {
	n, err := s.t.hostExpiration(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}
