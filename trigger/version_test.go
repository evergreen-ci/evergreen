package trigger

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

func TestVersionTriggers(t *testing.T) {
	suite.Run(t, &VersionSuite{})
}

type VersionSuite struct {
	event   event.EventLogEntry
	data    *event.VersionEventData
	version model.Version
	subs    []event.Subscription
	ctx     context.Context
	cancel  context.CancelFunc

	t *versionTriggers

	suite.Suite
}

func (s *VersionSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.NoError(db.ClearCollections(event.EventCollection, model.VersionCollection, event.SubscriptionsCollection, task.Collection, alertrecord.Collection, patch.Collection))
	startTime := time.Now().Truncate(time.Millisecond)

	versionID := "5aeb4514f27e4f9984646d97"
	const versionStatus = evergreen.VersionStarted

	s.version = model.Version{
		Id:                  versionID,
		Identifier:          "test",
		Status:              versionStatus,
		StartTime:           startTime,
		FinishTime:          startTime.Add(10 * time.Minute),
		Branch:              "mci",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 2,
	}
	s.NoError(s.version.Insert())

	s.data = &event.VersionEventData{
		Status: versionStatus,
	}
	s.event = event.EventLogEntry{
		ID:           utility.RandomString(),
		ResourceType: event.ResourceTypeVersion,
		EventType:    event.VersionStateChange,
		ResourceId:   versionID,
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
		event.NewSubscriptionByID(event.ResourceTypeVersion, event.TriggerOutcome, s.event.ResourceId, apiSub),
		event.NewSubscriptionByID(event.ResourceTypeVersion, event.TriggerSuccess, s.event.ResourceId, apiSub),
		event.NewSubscriptionByID(event.ResourceTypeVersion, event.TriggerFailure, s.event.ResourceId, apiSub),
		event.NewSubscriptionByID(event.ResourceTypeVersion, event.TriggerRegression, s.event.ResourceId, apiSub),
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeVersion,
			Trigger:      event.TriggerExceedsDuration,
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.JIRACommentSubscriberType,
				Target: "A-1",
			},
			Owner: "someone",
			TriggerData: map[string]string{
				event.VersionDurationKey: "300",
			},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeVersion,
			Trigger:      event.TriggerRuntimeChangeByPercent,
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.JIRACommentSubscriberType,
				Target: "A-1",
			},
			Owner: "someone",
			TriggerData: map[string]string{
				event.VersionPercentChangeKey: "50",
			},
		},
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set(s.ctx))

	s.t = makeVersionTriggers().(*versionTriggers)
	s.t.event = &s.event
	s.t.data = s.data
	s.t.version = &s.version
	s.t.uiConfig = *ui
}

func (s *VersionSuite) TearDownTest() {
	s.cancel()
}

func (s *VersionSuite) TearDownSuite() {
	s.NoError(db.ClearCollections(event.EventCollection, model.VersionCollection, event.SubscriptionsCollection, task.Collection, alertrecord.Collection, patch.Collection))
}

func (s *VersionSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(s.ctx, &s.event)
	s.NoError(err)
	s.Len(n, 0)

	s.version.Status = evergreen.VersionSucceeded
	s.data.Status = evergreen.VersionSucceeded
	s.NoError(db.Update(model.VersionCollection, bson.M{"_id": s.version.Id}, &s.version))

	n, err = NotificationsFromEvent(s.ctx, &s.event)
	s.NoError(err)
	s.Len(n, 3)

	s.version.Status = evergreen.VersionFailed
	s.data.Status = evergreen.VersionFailed
	s.NoError(db.Update(model.VersionCollection, bson.M{"_id": s.version.Id}, &s.version))

	n, err = NotificationsFromEvent(s.ctx, &s.event)
	s.NoError(err)
	s.Len(n, 3)

	s.version.Status = evergreen.VersionFailed
	s.data.Status = evergreen.VersionCreated
	s.NoError(db.Update(model.VersionCollection, bson.M{"_id": s.version.Id}, &s.version))

	n, err = NotificationsFromEvent(s.ctx, &s.event)
	s.NoError(err)
	s.Len(n, 0)
}

func (s *VersionSuite) TestVersionSuccess() {
	n, err := s.t.versionSuccess(s.ctx, &s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.versionSuccess(s.ctx, &s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.versionSuccess(s.ctx, &s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}

func (s *VersionSuite) TestVersionFailure() {
	s.data.Status = evergreen.VersionCreated
	n, err := s.t.versionOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.versionFailure(s.ctx, &s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.versionFailure(s.ctx, &s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *VersionSuite) TestVersionOutcome() {
	s.data.Status = evergreen.VersionCreated
	n, err := s.t.versionOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.versionOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.versionOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *VersionSuite) TestVersionRegression() {
	t1 := task.Task{
		Id:                  "t1",
		Version:             s.version.Id,
		BuildVariant:        "test",
		Project:             s.version.Branch,
		DisplayName:         "Test",
		Status:              evergreen.TaskSucceeded,
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 2,
	}
	s.NoError(t1.Insert())

	// a successful version should not generate
	s.data.Status = evergreen.VersionSucceeded
	n, err := s.t.versionRegression(s.ctx, &s.subs[3])
	s.NoError(err)
	s.Nil(n)

	// a failing version for the first time should generate
	t2 := task.Task{
		Id:                  "t2",
		Version:             s.version.Id,
		BuildVariant:        "test",
		Project:             s.version.Branch,
		DisplayName:         "Test",
		Status:              evergreen.TaskFailed,
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 2,
	}
	s.NoError(t2.Insert())
	s.data.Status = evergreen.VersionFailed
	n, err = s.t.versionRegression(s.ctx, &s.subs[3])
	s.NoError(err)
	s.NotNil(n)

	// a version with a task that has failed before should not generate
	t3 := task.Task{
		Id:                  "t3",
		Version:             "foo",
		BuildVariant:        "test",
		Project:             s.version.Branch,
		DisplayName:         "Test",
		Status:              evergreen.TaskFailed,
		FinishTime:          time.Now().Add(-1 * time.Hour),
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 1,
	}
	s.NoError(t3.Insert())
	s.NoError(newAlertRecord(s.subs[3].ID, &t3, alertrecord.TaskFailTransitionId).Insert())
	n, err = s.t.versionRegression(s.ctx, &s.subs[3])
	s.NoError(err)
	s.Nil(n)
}

func (s *VersionSuite) TestVersionExceedsTime() {
	// version that exceeds time should generate
	s.t.event = &event.EventLogEntry{
		EventType: event.VersionStateChange,
	}
	s.t.data.Status = evergreen.VersionSucceeded
	s.t.version.FinishTime = time.Now()
	s.t.version.StartTime = s.t.version.FinishTime.Add(-20 * time.Minute)
	n, err := s.t.versionExceedsDuration(s.ctx, &s.subs[4])
	s.NoError(err)
	s.NotNil(n)

	// build that does not exceed should not generate
	s.t.version.StartTime = s.t.version.FinishTime.Add(-4 * time.Minute)
	n, err = s.t.versionExceedsDuration(s.ctx, &s.subs[4])
	s.NoError(err)
	s.Nil(n)

	// unfinished build should not generate
	s.t.data.Status = evergreen.VersionStarted
	s.t.version.StartTime = s.t.version.FinishTime.Add(-20 * time.Minute)
	n, err = s.t.versionExceedsDuration(s.ctx, &s.subs[4])
	s.NoError(err)
	s.Nil(n)
}

func (s *VersionSuite) TestVersionRuntimeChange() {
	// no previous version should not generate
	s.t.version.FinishTime = time.Now()
	s.t.version.StartTime = s.t.version.FinishTime.Add(-20 * time.Minute)
	s.t.event = &event.EventLogEntry{
		EventType: event.VersionStateChange,
	}
	s.t.data.Status = evergreen.VersionSucceeded
	n, err := s.t.versionRuntimeChange(s.ctx, &s.subs[5])
	s.NoError(err)
	s.Nil(n)

	// version that exceeds threshold should generate
	lastGreen := model.Version{
		RevisionOrderNumber: 1,
		Identifier:          s.version.Identifier,
		StartTime:           time.Now().Add(-10 * time.Minute),
		FinishTime:          time.Now(),
		Status:              evergreen.VersionSucceeded,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	s.NoError(lastGreen.Insert())
	n, err = s.t.versionRuntimeChange(s.ctx, &s.subs[5])
	s.NoError(err)
	s.NotNil(n)

	// version that does not exceed threshold should not generate
	s.t.version.StartTime = s.t.version.FinishTime.Add(-11 * time.Minute)
	n, err = s.t.versionRuntimeChange(s.ctx, &s.subs[5])
	s.NoError(err)
	s.Nil(n)

	// build that finished too quickly should generate
	s.t.version.StartTime = s.t.version.FinishTime.Add(-4 * time.Minute)
	n, err = s.t.versionRuntimeChange(s.ctx, &s.subs[5])
	s.NoError(err)
	s.NotNil(n)
}

func (s *VersionSuite) TestMakeDataForRepotrackerVersion() {
	sub := s.subs[0]
	data, err := s.t.makeData(&sub, "")

	s.Require().NoError(err)
	s.Equal(s.version.Id, data.ID)
	s.Equal(s.event.ID, data.EventID)
	s.Equal(sub.ID, data.SubscriptionID)
	s.Equal(s.version.Id, data.DisplayName)
	s.Equal(event.ObjectVersion, data.Object)
	s.Equal(data.PastTenseStatus, s.version.Status)
}

func (s *VersionSuite) TestMakeDataForPatchVersion() {
	p := patch.Patch{
		Id:     mgobson.ObjectIdHex(s.version.Id),
		Status: evergreen.VersionSucceeded,
	}
	s.Require().NoError(p.Insert())

	s.version.Requester = evergreen.PatchVersionRequester
	sub := s.subs[0]
	data, err := s.t.makeData(&sub, "")

	s.Require().NoError(err)
	s.Equal(s.version.Id, data.ID)
	s.Equal(s.event.ID, data.EventID)
	s.Equal(sub.ID, data.SubscriptionID)
	s.Equal(s.version.Id, data.DisplayName)
	s.Equal(event.ObjectVersion, data.Object)
	s.Equal(data.PastTenseStatus, "succeeded")
}
