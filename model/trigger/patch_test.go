package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestPatchTriggers(t *testing.T) {
	suite.Run(t, &patchSuite{})
}

type patchSuite struct {
	event event.EventLogEntry
	data  *event.PatchEventData
	patch patch.Patch
	subs  []event.Subscription

	suite.Suite
}

func (s *patchSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *patchSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, patch.Collection, event.SubscriptionsCollection))
	startTime := time.Now().Truncate(time.Millisecond)

	patchID := bson.ObjectIdHex("5aeb4514f27e4f9984646d97")

	s.patch = patch.Patch{
		Id:         patchID,
		Project:    "test",
		Author:     "someone",
		StartTime:  startTime,
		FinishTime: startTime.Add(10 * time.Minute),
		GithubPatchData: patch.GithubPatch{
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "tychoish",
			HeadRepo:  "evergreen",
			PRNumber:  448,
			HeadHash:  "776f608b5b12cd27b8d931c8ee4ca0c13f857299",
		},
	}
	s.patch.Version = s.patch.Id.Hex()
	s.NoError(s.patch.Insert())

	s.data = &event.PatchEventData{}
	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		ResourceId:   patchID.Hex(),
		Data:         s.data,
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

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set())
}

func (s *patchSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)

	s.patch.Status = evergreen.PatchSucceeded
	s.data.Status = evergreen.PatchSucceeded
	s.NoError(db.Update(patch.Collection, bson.M{"_id": s.patch.Id}, &s.patch))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.patch.Status = evergreen.PatchFailed
	s.data.Status = evergreen.PatchFailed
	s.NoError(db.Update(patch.Collection, bson.M{"_id": s.patch.Id}, &s.patch))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)
}

func (s *patchSuite) TestPatchSuccess() {
	gen, err := patchSuccess(s.data, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.PatchFailed
	gen, err = patchSuccess(s.data, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.PatchSucceeded
	gen, err = patchSuccess(s.data, &s.patch)
	s.NoError(err)
	s.NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("success", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "success",
	})
}

func (s *patchSuite) TestPatchFailure() {
	s.data.Status = evergreen.PatchSucceeded
	gen, err := patchFailure(s.data, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.PatchFailed
	gen, err = patchFailure(s.data, &s.patch)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("failure", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "failure",
	})
}

func (s *patchSuite) TestPatchOutcome() {
	s.data.Status = evergreen.PatchCreated
	gen, err := patchOutcome(s.data, &s.patch)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.PatchSucceeded
	gen, err = patchOutcome(s.data, &s.patch)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())

	s.data.Status = evergreen.PatchFailed
	gen, err = patchOutcome(s.data, &s.patch)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("outcome", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "outcome",
	})
}

func (s *patchSuite) TestPatchCreated() {
	gen, err := patchCreated(s.data, &s.patch)
	s.Nil(err)
	s.Nil(gen)

	s.data.Status = evergreen.PatchCreated
	gen, err = patchCreated(s.data, &s.patch)
	s.Nil(err)
	s.Require().NotNil(gen)
	s.Require().NotNil(gen.githubStatusAPI)

	s.Equal("created", gen.triggerName)
	s.Equal("evergreen", gen.githubStatusAPI.Context)
	s.Equal(message.GithubStatePending, gen.githubStatusAPI.State)
	s.Equal("preparing to run tasks", gen.githubStatusAPI.Description)
	s.Equal("https://evergreen.mongodb.com/version/5aeb4514f27e4f9984646d97", gen.githubStatusAPI.URL)
}

func (s *patchSuite) TestPatchStarted() {
	gen, err := patchStarted(s.data, &s.patch)
	s.Nil(err)
	s.Nil(gen)

	s.data.Status = evergreen.PatchStarted
	gen, err = patchStarted(s.data, &s.patch)
	s.Nil(err)
	s.Require().NotNil(gen)
	s.Require().NotNil(gen.githubStatusAPI)

	s.Equal("started", gen.triggerName)
	s.Equal("evergreen", gen.githubStatusAPI.Context)
	s.Equal(message.GithubStatePending, gen.githubStatusAPI.State)
	s.Equal("tasks are running", gen.githubStatusAPI.Description)
	s.Equal("https://evergreen.mongodb.com/version/5aeb4514f27e4f9984646d97", gen.githubStatusAPI.URL)
}
