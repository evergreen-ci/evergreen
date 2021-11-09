package trigger

import (
	"testing"
	"time"

	dbModel "github.com/evergreen-ci/evergreen/model"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

func TestPatchTriggers(t *testing.T) {
	suite.Run(t, &patchSuite{})
}

type patchSuite struct {
	event event.EventLogEntry
	data  *event.PatchEventData
	patch patch.Patch
	subs  []event.Subscription

	t *patchTriggers

	suite.Suite
}

func (s *patchSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &patchTriggers{})
}

func (s *patchSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, patch.Collection, event.SubscriptionsCollection, dbModel.ProjectRefCollection))
	startTime := time.Now().Truncate(time.Millisecond)

	patchID := mgobson.ObjectIdHex("5aeb4514f27e4f9984646d97")

	childPatchId := "5aab4514f27e4f9984646d97"

	pRef := dbModel.ProjectRef{
		Id:         "test",
		Identifier: "testing",
	}
	s.NoError(pRef.Insert())
	s.patch = patch.Patch{
		Id:         patchID,
		Project:    "test",
		Author:     "someone",
		StartTime:  startTime,
		FinishTime: startTime.Add(10 * time.Minute),
		GithubPatchData: thirdparty.GithubPatch{
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

	childPatch := patch.Patch{
		Id:         mgobson.ObjectIdHex(childPatchId),
		Project:    "test",
		Author:     "someone",
		Status:     evergreen.PatchCreated,
		StartTime:  startTime,
		FinishTime: startTime.Add(10 * time.Minute),
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "tychoish",
			HeadRepo:  "evergreen",
			PRNumber:  448,
			HeadHash:  "776f608b5b12cd27b8d931c8ee4ca0c13f857299",
		},
	}
	s.NoError(childPatch.Insert())

	s.data = &event.PatchEventData{
		Status: evergreen.PatchCreated,
	}
	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		EventType:    event.PatchStateChange,
		ResourceId:   patchID.Hex(),
		Data:         s.data,
	}

	apiSub := event.Subscriber{
		Type: event.EvergreenWebhookSubscriberType,
		Target: &event.WebhookSubscriber{
			URL:    "http://example.com/2",
			Secret: []byte("secret"),
		},
	}

	s.subs = []event.Subscription{
		event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerOutcome, s.event.ResourceId, apiSub),
		event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerSuccess, s.event.ResourceId, apiSub),
		event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerFailure, s.event.ResourceId, apiSub),
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set())

	s.t = makePatchTriggers().(*patchTriggers)
	s.t.event = &s.event
	s.t.data = s.data
	s.t.patch = &s.patch
	s.t.uiConfig = *ui
}

func (s *patchSuite) TestFetch() {
	t, ok := makePatchTriggers().(*patchTriggers)
	s.Require().True(ok)
	s.NoError(t.Fetch(&s.event))
	s.NotNil(t.event)
	s.Equal(t.event, &s.event)
	s.NotNil(t.data)
	s.NotNil(t.patch)
	s.NotZero(t.uiConfig)
	s.NotEmpty(t.triggers)
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
	n, err := s.t.patchSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.PatchFailed
	n, err = s.t.patchSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.PatchSucceeded
	n, err = s.t.patchSuccess(&s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}

func (s *patchSuite) TestPatchFailure() {
	s.data.Status = evergreen.PatchCreated
	n, err := s.t.patchFailure(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.PatchSucceeded
	n, err = s.t.patchFailure(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.PatchFailed
	n, err = s.t.patchFailure(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *patchSuite) TestPatchOutcome() {
	s.data.Status = evergreen.PatchCreated
	n, err := s.t.patchOutcome(&s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.PatchSucceeded
	n, err = s.t.patchOutcome(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)

	s.data.Status = evergreen.PatchFailed
	n, err = s.t.patchOutcome(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *patchSuite) TestRunChildrenOnPatchOutcome() {
	childPatchId := "5aab4514f27e4f9984646d97"
	childPatchSubSuccess := event.Subscriber{
		Type: event.RunChildPatchSubscriberType,
		Target: &event.ChildPatchSubscriber{
			ParentStatus: "succeeded",
			ChildPatchId: childPatchId,
			Requester:    evergreen.TriggerRequester,
		},
	}
	childPatchSubFailure := event.Subscriber{
		Type: event.RunChildPatchSubscriberType,
		Target: &event.ChildPatchSubscriber{
			ParentStatus: "failed",
			ChildPatchId: childPatchId,
			Requester:    evergreen.TriggerRequester,
		},
	}

	childPatchSubAny := event.Subscriber{
		Type: event.RunChildPatchSubscriberType,
		Target: &event.ChildPatchSubscriber{
			ParentStatus: "*",
			ChildPatchId: childPatchId,
			Requester:    evergreen.TriggerRequester,
		},
	}
	s.subs = []event.Subscription{
		event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerOutcome, s.event.ResourceId, childPatchSubSuccess),
		event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerOutcome, s.event.ResourceId, childPatchSubFailure),
		event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerOutcome, s.event.ResourceId, childPatchSubAny),
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	s.data.Status = evergreen.PatchSucceeded
	n, err := s.t.patchOutcome(&s.subs[0])
	// there is no token set up in settings, but hitting this error
	// means it's trying to finalize the patch
	s.Equal("Failed to finalize child patch: can't get Github OAuth token from configuration: no 'github' token in settings", err.Error())
	s.Nil(n)

	s.data.Status = evergreen.PatchFailed
	n, err = s.t.patchOutcome(&s.subs[1])
	s.Equal("Failed to finalize child patch: can't get Github OAuth token from configuration: no 'github' token in settings", err.Error())
	s.Nil(n)

	s.data.Status = evergreen.PatchSucceeded
	n, err = s.t.patchOutcome(&s.subs[2])
	s.Equal("Failed to finalize child patch: can't get Github OAuth token from configuration: no 'github' token in settings", err.Error())
	s.Nil(n)

	s.data.Status = evergreen.PatchFailed
	n, err = s.t.patchOutcome(&s.subs[2])
	s.Equal("Failed to finalize child patch: can't get Github OAuth token from configuration: no 'github' token in settings", err.Error())
	s.Nil(n)

}

func (s *patchSuite) TestPatchStarted() {
	n, err := s.t.patchStarted(&s.subs[0])
	s.Nil(err)
	s.Nil(n)

	s.data.Status = evergreen.PatchStarted
	n, err = s.t.patchStarted(&s.subs[0])
	s.Nil(err)
	s.NotNil(n)
}
