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
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
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
		Status:         evergreen.HostRunning,
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
		event.NewSubscriptionByID(event.ResourceTypeHost, event.TriggerSpawnHostIdle, s.t.host.Id, event.Subscriber{
			Type:   event.EmailSubscriberType,
			Target: "foo@bar.com",
		}),
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	s.uiConfig = &evergreen.UIConfig{
		Url:     "https://evergreen.mongodb.com",
		UIv2Url: "https://spruce.mongodb.com",
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
	email, err := s.testData.hostEmailPayload(expiringHostEmailSubject, expiringHostEmailBody, s.t.Attributes())
	s.NoError(err)
	s.Equal("myDistro host termination reminder", email.Subject)
	s.Contains(email.Body, "Your myDistro host 'hostName' will be terminated at")

	email, err = s.testData.hostEmailPayload(idleHostEmailSubject, idleStoppedHostEmailBody, s.t.Attributes())
	s.NoError(err)
	s.Equal("myDistro idle stopped host notice", email.Subject)
	s.Contains(email.Body, "Your stopped myDistro host 'hostName' has been idle for at least three months")
}

func (s *hostSuite) TestSlackMessage() {
	msg, err := s.testData.hostSlackPayload(expiringHostSlackBody, "linkTitle")
	s.NoError(err)
	s.Contains(msg.Body, "Your myDistro host 'hostName' will be terminated at")
}

func (s *hostSuite) TestFetch() {
	triggers := hostTriggers{}
	s.NoError(triggers.Fetch(s.ctx, s.t.event))
	s.Equal(s.t.host.Id, triggers.templateData.ID)
	s.Equal(fmt.Sprintf("%s/spawn/host", s.uiConfig.UIv2Url), triggers.templateData.URL)

	s.t.event = &event.EventLogEntry{
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventSpawnHostIdleNotification,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
	}
	s.NoError(triggers.Fetch(s.ctx, s.t.event))
	s.Equal(s.t.host.Id, triggers.templateData.ID)
	s.Equal(fmt.Sprintf("%s/spawn/host", s.uiConfig.UIv2Url), triggers.templateData.URL)
}

func (s *hostSuite) TestAllTriggers() {
	// Valid event should trigger a notification
	n, err := NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 1)
	email, ok := n[0].Payload.(*message.Email)
	s.True(ok)
	s.Contains(email.Body, "will be terminated")

	s.t.event = &event.EventLogEntry{
		ID:           "idleEvent",
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventSpawnHostIdleNotification,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
		ProcessedAt:  time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
		Timestamp:    time.Date(2024, time.April, 18, 17, 1, 13, 384000000, time.Local),
		Expirable:    true,
	}

	// A running host should not trigger a notification.
	s.Require().NoError(host.UpdateOne(s.ctx, host.ById(s.t.host.Id), bson.M{
		"$set": bson.M{
			host.NoExpirationKey: true,
			host.StatusKey:       evergreen.HostRunning,
		},
	}))
	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 0)

	// A stopped host with no stopped event should trigger a notification.
	s.Require().NoError(host.UpdateOne(s.ctx, host.ById(s.t.host.Id), bson.M{
		"$set": bson.M{
			host.StatusKey: evergreen.HostStopped,
		},
	}))

	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 1)
	email, ok = n[0].Payload.(*message.Email)
	s.True(ok)
	s.Contains(email.Body, "has been idle")

	// A four-month stopped host should trigger a notification.
	stoppedEvent := &event.EventLogEntry{
		ID:           "stoppedEvent",
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventHostStopped,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
		Timestamp:    time.Now().Add(-time.Hour * 24 * 31 * 4),
		Expirable:    true,
	}
	s.NoError(db.Insert(event.EventCollection, stoppedEvent))
	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 1)
	email, ok = n[0].Payload.(*message.Email)
	s.True(ok)
	s.Contains(email.Body, "has been idle")

	// A one-month idle stopped host should not trigger a notification.
	stoppedEvent = &event.EventLogEntry{
		ID:           "recentStoppedEvent",
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventHostStopped,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
		Timestamp:    time.Now().Add(-time.Hour * 24 * 31),
		Expirable:    true,
	}
	s.NoError(db.Insert(event.EventCollection, stoppedEvent))

	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 0)
}

func (s *hostSuite) TestHostExpiration() {
	n, err := s.t.hostExpiration(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *hostSuite) TestHostTemporaryExemptionExpiration() {
	s.t.event = &event.EventLogEntry{
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventHostTemporaryExemptionExpirationWarningSent,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
	}
	s.t.host.NoExpiration = true
	s.t.host.ExpirationTime = time.Now().Add(evergreen.SpawnHostExpireDays * evergreen.SpawnHostNoExpirationDuration)
	s.t.host.SleepSchedule.TemporarilyExemptUntil = time.Now().Add(time.Hour)

	n, err := s.t.hostExpiration(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *hostSuite) TestSpawnHostIdle() {
	s.t.host.NoExpiration = true
	s.t.host.Status = evergreen.HostStopped
	n, err := s.t.spawnHostIdle(s.ctx, &s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}
