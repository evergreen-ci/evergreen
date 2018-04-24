package trigger

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestPatchTriggers(t *testing.T) {
	suite.Run(t, &patchSuite{})
}

type patchSuite struct {
	event event.EventLogEntry
	patch patch.Patch
	subs  []event.Subscription

	suite.Suite
}

func (s *patchSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *patchSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, patch.Collection, event.SubscriptionsCollection))

	s.patch = patch.Patch{
		Id:      bson.NewObjectId(),
		Project: "test",
		Author:  "someone",
	}
	s.NoError(s.patch.Insert())

	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		ResourceId:   s.patch.Id.Hex(),
	}

	s.subs = []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypePatch,
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
			Type:    event.ResourceTypePatch,
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
			Type:    event.ResourceTypePatch,
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
}

func (s *patchSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)

	s.patch.Status = evergreen.PatchSucceeded
	s.NoError(db.Update(patch.Collection, bson.M{"_id": s.patch.Id}, &s.patch))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.patch.Status = evergreen.PatchFailed
	s.NoError(db.Update(patch.Collection, bson.M{"_id": s.patch.Id}, &s.patch))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)
}

func (s *patchSuite) TestPatchSuccess() {
	gen, err := patchSuccess(&s.event, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.patch.Status = evergreen.PatchFailed
	gen, err = patchSuccess(&s.event, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.patch.Status = evergreen.PatchSucceeded
	gen, err = patchSuccess(&s.event, &s.patch)
	s.NoError(err)
	s.NotNil(gen)
	s.False(gen.isEmpty())
}

func (s *patchSuite) TestPatchFailure() {
	s.patch.Status = evergreen.PatchSucceeded
	gen, err := patchFailure(&s.event, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.patch.Status = evergreen.PatchFailed
	gen, err = patchFailure(&s.event, &s.patch)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
}

func (s *patchSuite) TestPatchOutcome() {
	s.patch.Status = evergreen.PatchCreated
	gen, err := patchOutcome(&s.event, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.patch.Status = evergreen.PatchSucceeded
	gen, err = patchOutcome(&s.event, &s.patch)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())

	s.patch.Status = evergreen.PatchFailed
	gen, err = patchOutcome(&s.event, &s.patch)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
}
