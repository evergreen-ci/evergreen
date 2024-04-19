package trigger

import (
	"context"
	"fmt"
	"github.com/mongodb/anser/bsonutil"
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
	email, err := s.testData.hostEmailPayload(expiringHostEmailSubject, expiringHostEmailBody, s.t.Attributes())
	s.NoError(err)
	s.Equal("myDistro host termination reminder", email.Subject)
	s.Contains(email.Body, "Your myDistro host 'hostName' will be terminated at")

	email, err = s.testData.hostEmailPayload(idleHostEmailSubject, idleStoppedHostEmailBody, s.t.Attributes())
	s.NoError(err)
	s.Equal("myDistro idle host notice", email.Subject)
	s.Contains(email.Body, "Your stopped myDistro host 'hostName' has been idle since")
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
	s.Equal(fmt.Sprintf("%s/spawn#?resourcetype=hosts&id=%s", s.uiConfig.Url, s.t.host.Id), triggers.templateData.URL)

	s.t.event = &event.EventLogEntry{
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventSpawnHostIdleNotificationSent,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
	}
	s.NoError(triggers.Fetch(s.ctx, s.t.event))
	s.Equal(s.t.host.Id, triggers.templateData.ID)
	s.Equal(fmt.Sprintf("%s/spawn#?resourcetype=hosts&id=%s", s.uiConfig.Url, s.t.host.Id), triggers.templateData.URL)
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
		ID:           "66218a19751026f955192ba5",
		ResourceType: event.ResourceTypeHost,
		EventType:    event.EventSpawnHostIdleNotificationSent,
		ResourceId:   s.t.host.Id,
		Data:         &event.HostEventData{},
		ProcessedAt:  time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
		Timestamp:    time.Date(2024, time.April, 18, 17, 1, 13, 384000000, time.Local),
		Expirable:    true,
	}

	s.Require().NoError(host.UpdateOne(context.Background(), host.ById(s.t.host.Id), bson.M{
		"$set": bson.M{
			host.NoExpirationKey: true,
			host.StatusKey:       evergreen.HostRunning,
			bsonutil.GetDottedKeyName(host.SleepScheduleKey, host.SleepScheduleShouldKeepOffKey): true,
			host.LastCommunicationTimeKey: time.Now().Add(-1 * time.Hour * 24 * 31), // 1 months ago
		},
	}))
	// A one-month idle running host should not trigger a notification.
	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 0)

	s.Require().NoError(host.UpdateOne(context.Background(), host.ById(s.t.host.Id), bson.M{
		"$set": bson.M{
			host.StatusKey:                evergreen.HostStopped,
			host.LastCommunicationTimeKey: time.Now().Add(-1 * time.Hour * 24 * 31), // 1 months ago
		},
	}))
	// A one-month idle stopped host should not trigger a notification.
	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 0)

	// A three-month idle stopped host should trigger a notification.
	s.Require().NoError(host.UpdateOne(context.Background(), host.ById(s.t.host.Id), bson.M{
		"$set": bson.M{
			host.LastCommunicationTimeKey: time.Now().Add(-6 * time.Hour * 24 * 31), // 3 months ago
		},
	}))
	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 1)
	email, ok = n[0].Payload.(*message.Email)
	s.True(ok)
	s.Contains(email.Body, "has been idle")

	// A temporarily stopped three-month idle host should not trigger a notification.
	s.Require().NoError(host.UpdateOne(context.Background(), host.ById(s.t.host.Id), bson.M{
		"$unset": bson.M{
			bsonutil.GetDottedKeyName(host.SleepScheduleKey, host.SleepScheduleShouldKeepOffKey): 1,
		},
	}))
	n, err = NotificationsFromEvent(s.ctx, s.t.event)
	s.NoError(err)
	s.Require().Len(n, 0)
}

func (s *hostSuite) TestHostExpiration() {
	s.t.host.NoExpiration = false
	n, err := s.t.hostExpiration(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *hostSuite) TestSpawnHostIdle() {
	s.t.host.NoExpiration = true
	s.t.host.Status = evergreen.HostStopped
	s.t.host.SleepSchedule.ShouldKeepOff = true
	n, err := s.t.spawnHostIdle(&s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}
