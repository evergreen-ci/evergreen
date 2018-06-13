package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
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

	t *versionTriggers

	suite.Suite
}

func (s *VersionSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *VersionSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, version.Collection, event.SubscriptionsCollection, task.Collection, alertrecord.Collection))
	startTime := time.Now().Truncate(time.Millisecond)

	versionID := "5aeb4514f27e4f9984646d97"

	s.version = version.Version{
		Id:         versionID,
		Identifier: "test",
		StartTime:  startTime,
		FinishTime: startTime.Add(10 * time.Minute),
		Branch:     "mci",
	}
	s.NoError(s.version.Insert())

	s.data = &event.VersionEventData{
		Status: evergreen.VersionStarted,
	}
	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypeVersion,
		ResourceId:   versionID,
		Data:         s.data,
	}

	s.subs = []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeVersion,
			Trigger: triggerOutcome,
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
			Trigger: triggerSuccess,
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
			Trigger: triggerFailure,
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
			Trigger: triggerRegression,
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

	s.t = makeVersionTriggers().(*versionTriggers)
	s.t.event = &s.event
	s.t.data = s.data
	s.t.version = &s.version
	s.t.uiConfig = *ui
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
	n, err := s.t.versionSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.versionSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.versionSuccess(&s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}

func (s *VersionSuite) TestVersionFailure() {
	s.data.Status = evergreen.VersionCreated
	n, err := s.t.versionOutcome(&s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.versionFailure(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.versionFailure(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *VersionSuite) TestVersionOutcome() {
	s.data.Status = evergreen.VersionCreated
	n, err := s.t.versionOutcome(&s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.VersionSucceeded
	n, err = s.t.versionOutcome(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)

	s.data.Status = evergreen.VersionFailed
	n, err = s.t.versionOutcome(&s.subs[0])
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
	n, err := s.t.versionRegression(&s.subs[3])
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
	n, err = s.t.versionRegression(&s.subs[3])
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
	s.NoError(newAlertRecord(&t3, alertrecord.TaskFailTransitionId).Insert())
	n, err = s.t.versionRegression(&s.subs[3])
	s.NoError(err)
	s.Nil(n)
}
