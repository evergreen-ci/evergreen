package trigger

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

func TestPatchTriggers(t *testing.T) {
	suite.Run(t, &patchSuite{})
}

type patchSuite struct {
	event  event.EventLogEntry
	data   *event.PatchEventData
	patch  patch.Patch
	subs   []event.Subscription
	ctx    context.Context
	cancel context.CancelFunc

	t *patchTriggers

	suite.Suite
}

func (s *patchSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &patchTriggers{})
}

func (s *patchSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.NoError(db.ClearCollections(event.EventCollection, patch.Collection, event.SubscriptionsCollection, model.ProjectRefCollection, model.VersionCollection))
	startTime := time.Now().Truncate(time.Millisecond)

	patchID := mgobson.ObjectIdHex("5aeb4514f27e4f9984646d97")

	childPatchId := "5aab4514f27e4f9984646d97"

	pRef := model.ProjectRef{
		Id:         "test",
		Identifier: "testing",
	}
	s.NoError(pRef.Insert(s.ctx))
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
	s.NoError(s.patch.Insert(s.ctx))

	childPatch := patch.Patch{
		Id:         mgobson.ObjectIdHex(childPatchId),
		Project:    "test",
		Author:     "someone",
		Status:     evergreen.VersionCreated,
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
	s.NoError(childPatch.Insert(s.ctx))

	version := model.Version{
		Id:      s.patch.Id.Hex(),
		Aborted: false,
	}
	s.NoError(version.Insert(s.ctx))
	childVersion := model.Version{
		Id:      childPatchId,
		Aborted: false,
	}
	s.NoError(childVersion.Insert(s.ctx))

	s.data = &event.PatchEventData{
		Status: evergreen.VersionCreated,
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
		s.NoError(s.subs[i].Upsert(s.ctx))
	}

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set(s.ctx))

	s.t = makePatchTriggers().(*patchTriggers)
	s.t.event = &s.event
	s.t.data = s.data
	s.t.patch = &s.patch
	s.t.uiConfig = *ui
}

func (s *patchSuite) TearDownTest() {
	s.cancel()
}

func (s *patchSuite) TestFetch() {
	t, ok := makePatchTriggers().(*patchTriggers)
	s.Require().True(ok)
	s.NoError(t.Fetch(s.ctx, &s.event))
	s.NotNil(t.event)
	s.Equal(t.event, &s.event)
	s.NotNil(t.data)
	s.NotNil(t.patch)
	s.NotZero(t.uiConfig)
	s.NotEmpty(t.triggers)
}

func (s *patchSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(s.ctx, &s.event)
	s.NoError(err)
	s.Empty(n)

	s.patch.Status = evergreen.VersionSucceeded
	s.data.Status = evergreen.VersionSucceeded
	_, err = db.Replace(s.ctx, patch.Collection, bson.M{"_id": s.patch.Id}, &s.patch)
	s.NoError(err)

	n, err = NotificationsFromEvent(s.ctx, &s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.patch.Status = evergreen.VersionFailed
	s.data.Status = evergreen.VersionFailed
	_, err = db.Replace(s.ctx, patch.Collection, bson.M{"_id": s.patch.Id}, &s.patch)
	s.NoError(err)

	n, err = NotificationsFromEvent(s.ctx, &s.event)
	s.NoError(err)
	s.Len(n, 2)
}

func (s *patchSuite) TestPatchSuccess() {
	n, err := s.t.patchSuccess(s.ctx, &s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.patchSuccess(s.ctx, &s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.patchSuccess(s.ctx, &s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}

func (s *patchSuite) TestPatchFailure() {
	s.data.Status = evergreen.VersionCreated
	n, err := s.t.patchFailure(s.ctx, &s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.patchFailure(s.ctx, &s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.patchFailure(s.ctx, &s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *patchSuite) TestPatchOutcome() {
	s.data.Status = evergreen.VersionCreated
	n, err := s.t.patchOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.patchOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.patchOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *patchSuite) TestRunChildrenOnPatchOutcome() {
	childPatchId := "5aab4514f27e4f9984646d97"
	childPatchSubSuccess := event.Subscriber{
		Type: event.RunChildPatchSubscriberType,
		Target: &event.ChildPatchSubscriber{
			ParentStatus: evergreen.VersionSucceeded,
			ChildPatchId: childPatchId,
			Requester:    evergreen.TriggerRequester,
		},
	}
	childPatchSubFailure := event.Subscriber{
		Type: event.RunChildPatchSubscriberType,
		Target: &event.ChildPatchSubscriber{
			ParentStatus: evergreen.VersionFailed,
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
		s.NoError(s.subs[i].Upsert(s.ctx))
	}
	s.data.Status = evergreen.VersionSucceeded
	n, err := s.t.patchOutcome(s.ctx, &s.subs[0])
	// there is no token set up in settings, but hitting this error
	// means it's trying to finalize the patch
	s.Require().Error(err)
	s.Contains(err.Error(), "finalizing child patch")
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.patchOutcome(s.ctx, &s.subs[1])
	s.Require().Error(err)
	s.Contains(err.Error(), "finalizing child patch")
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.patchOutcome(s.ctx, &s.subs[2])
	s.Require().Error(err)
	s.Contains(err.Error(), "finalizing child patch")
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.patchOutcome(s.ctx, &s.subs[2])
	s.Require().Error(err)
	s.Contains(err.Error(), "finalizing child patch")
	s.Nil(n)

}

func (s *patchSuite) TestPatchFamilyOutcomeWithAbortedPatch() {
	patchID := mgobson.NewObjectId()
	eventData := event.PatchEventData{
		Status: evergreen.VersionFailed,
		Author: "myself",
	}
	e := event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		EventType:    event.PatchChildrenCompletion,
		ResourceId:   patchID.Hex(),
		Data:         &eventData,
	}

	p := patch.Patch{
		Id:     patchID,
		Status: evergreen.VersionFailed,
	}
	s.Require().NoError(p.Insert(s.ctx))

	v := model.Version{
		Id:        patchID.Hex(),
		Status:    evergreen.VersionFailed,
		Requester: evergreen.PatchVersionRequester,
		Aborted:   true,
	}
	s.Require().NoError(v.Insert(s.ctx))

	subscriber := event.Subscriber{
		Type: event.EvergreenWebhookSubscriberType,
		Target: &event.WebhookSubscriber{
			URL:    "http://example.com/2",
			Secret: []byte("secret"),
		},
	}
	subscription := event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerFamilyOutcome, e.ResourceId, subscriber)
	s.Require().NoError(subscription.Upsert(s.ctx))

	t := makePatchTriggers().(*patchTriggers)
	t.event = &e
	t.data = &eventData
	t.patch = &p

	suppressable, err := t.isNotificationSuppressableForAbort(s.ctx, &subscription)
	s.Require().NoError(err)
	s.True(suppressable)

	n, err := t.patchFamilyOutcome(s.ctx, &subscription)
	s.NoError(err)
	s.Zero(n, "should not create notification for aborted user patch")
}

func (s *patchSuite) TestPatchFamilyOutcomeWithAbortedGitHubMergePatch() {
	patchID := mgobson.NewObjectId()
	eventData := event.PatchEventData{
		Status: evergreen.VersionFailed,
		Author: evergreen.GithubMergeUser,
	}
	e := event.EventLogEntry{
		ResourceType: event.ResourceTypePatch,
		EventType:    event.PatchChildrenCompletion,
		ResourceId:   patchID.Hex(),
		Data:         &eventData,
	}

	p := patch.Patch{
		Id:     patchID,
		Status: evergreen.VersionFailed,
	}
	s.Require().NoError(p.Insert(s.ctx))

	v := model.Version{
		Id:        patchID.Hex(),
		Status:    evergreen.VersionFailed,
		Requester: evergreen.GithubMergeRequester,
		Aborted:   true,
	}
	s.Require().NoError(v.Insert(s.ctx))

	subscriber := event.Subscriber{
		Type: event.GithubMergeSubscriberType,
		Target: &event.GithubMergeSubscriber{
			Owner: "owner",
			Repo:  "repo",
			Ref:   "abc123",
		},
	}
	subscription := event.NewSubscriptionByID(event.ResourceTypePatch, event.TriggerFamilyOutcome, e.ResourceId, subscriber)
	s.Require().NoError(subscription.Upsert(s.ctx))

	t := makePatchTriggers().(*patchTriggers)
	t.event = &e
	t.data = &eventData
	t.patch = &p

	suppressable, err := t.isNotificationSuppressableForAbort(s.ctx, &subscription)
	s.Require().NoError(err)
	s.False(suppressable)

	n, err := t.patchFamilyOutcome(s.ctx, &subscription)
	s.NoError(err)
	s.NotNil(n)
	s.NotNil(n, "should create notification for aborted GitHub merge queue patch because the subscriber is the merge queue status check")
}

func (s *patchSuite) TestPatchStarted() {
	n, err := s.t.patchStarted(s.ctx, &s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionStarted
	n, err = s.t.patchStarted(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}
