package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestVersionTriggers(t *testing.T) {
	suite.Run(t, &VersionSuite{})
}

type VersionSuite struct {
	event   event.EventLogEntry
	data    *event.VersionEventData
	version version.Version
	subs    []event.Subscription

	suite.Suite
}

func (s *VersionSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *VersionSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, version.Collection, event.SubscriptionsCollection))
	startTime := time.Now().Truncate(time.Millisecond)

	versionID := "5aeb4514f27e4f9984646d97"

	s.version = version.Version{
		Id:         versionID,
		Identifier: "test",
		StartTime:  startTime,
		FinishTime: startTime.Add(10 * time.Minute),
	}
	s.NoError(s.version.Insert())

	s.data = &event.VersionEventData{}
	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypeVersion,
		ResourceId:   versionID,
		Data:         s.data,
	}

	s.subs = []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeVersion,
			Trigger: "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
			Owner: "someone",
		},
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeVersion,
			Trigger: "success",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
			Owner: "someone",
		},
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeVersion,
			Trigger: "failure",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
			Owner: "someone",
		},
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set())
}

func (s *VersionSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)

	s.version.Status = evergreen.VersionSucceeded
	s.data.Status = evergreen.VersionSucceeded
	s.NoError(db.Update(version.Collection, bson.M{"_id": s.version.Id}, &s.version))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.version.Status = evergreen.VersionFailed
	s.data.Status = evergreen.VersionFailed
	s.NoError(db.Update(version.Collection, bson.M{"_id": s.version.Id}, &s.version))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.version.Status = evergreen.VersionFailed
	s.data.Status = evergreen.VersionCreated
	s.NoError(db.Update(version.Collection, bson.M{"_id": s.version.Id}, &s.version))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)
}

func (s *VersionSuite) TestVersionSuccess() {
	gen, err := versionSuccess(s.data, &s.version)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.VersionFailed
	gen, err = versionSuccess(s.data, &s.version)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.VersionSucceeded
	gen, err = versionSuccess(s.data, &s.version)
	s.NoError(err)
	s.NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("success", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "success",
	})
}

func (s *VersionSuite) TestVersionFailure() {
	s.data.Status = evergreen.VersionSucceeded
	gen, err := versionFailure(s.data, &s.version)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.VersionFailed
	gen, err = versionFailure(s.data, &s.version)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("failure", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "failure",
	})
}

func (s *VersionSuite) TestVersionOutcome() {
	s.data.Status = evergreen.VersionCreated
	gen, err := versionOutcome(s.data, &s.version)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.VersionSucceeded
	gen, err = versionOutcome(s.data, &s.version)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())

	s.data.Status = evergreen.VersionFailed
	gen, err = versionOutcome(s.data, &s.version)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("outcome", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "outcome",
	})
}
