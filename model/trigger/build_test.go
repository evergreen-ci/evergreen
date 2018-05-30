package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestBuildTriggers(t *testing.T) {
	suite.Run(t, &buildSuite{})

}

type buildSuite struct {
	event event.EventLogEntry
	data  *event.BuildEventData
	build build.Build
	subs  []event.Subscription

	suite.Suite
}

func (s *buildSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *buildSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, build.Collection, event.SubscriptionsCollection))

	s.build = build.Build{
		Id:           "test",
		BuildVariant: "testvariant",
		Status:       evergreen.BuildCreated,
	}
	s.NoError(s.build.Insert())

	s.data = &event.BuildEventData{}
	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypeBuild,
		ResourceId:   "test",
		Data:         s.data,
	}

	s.subs = []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeBuild,
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
			Type:    event.ResourceTypeBuild,
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
			Type:    event.ResourceTypeBuild,
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

func (s *buildSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)

	s.build.Status = evergreen.BuildSucceeded
	s.data.Status = evergreen.BuildSucceeded
	s.NoError(db.Update(build.Collection, bson.M{"_id": s.build.Id}, &s.build))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.build.Status = evergreen.BuildFailed
	s.data.Status = evergreen.BuildFailed
	s.NoError(db.Update(build.Collection, bson.M{"_id": s.build.Id}, &s.build))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.build.Status = evergreen.BuildFailed
	s.data.Status = evergreen.BuildCreated
	s.NoError(db.Update(build.Collection, bson.M{"_id": s.build.Id}, &s.build))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)
}

func (s *buildSuite) TestSuccess() {
	gen, err := buildSuccess(s.data, &s.build)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.BuildSucceeded
	gen, err = buildSuccess(s.data, &s.build)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.Equal("success", gen.triggerName)
	s.False(gen.isEmpty())
}

func (s *buildSuite) TestFailure() {
	s.data.Status = evergreen.BuildSucceeded
	gen, err := buildFailure(s.data, &s.build)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.BuildFailed
	gen, err = buildFailure(s.data, &s.build)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.Equal("failure", gen.triggerName)
	s.False(gen.isEmpty())
}

func (s *buildSuite) TestOutcome() {
	s.data.Status = evergreen.BuildCreated
	gen, err := buildOutcome(s.data, &s.build)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.BuildFailed
	gen, err = buildOutcome(s.data, &s.build)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.Equal("outcome", gen.triggerName)
	s.False(gen.isEmpty())
}

func (s *buildSuite) TestTaskStatusToDesc() {
	b := &build.Build{
		Id:           bson.NewObjectId().Hex(),
		BuildVariant: "testvariant",
		Version:      "testversion",
		Status:       evergreen.BuildFailed,
		StartTime:    time.Time{},
		FinishTime:   time.Time{}.Add(10 * time.Second),
	}

	s.Equal("no tasks were run", taskStatusToDesc(b))

	b.Tasks = []build.TaskCache{
		{
			Status: evergreen.TaskSucceeded,
		},
	}
	s.Equal("1 succeeded, none failed in 10s", taskStatusToDesc(b))

	b.Tasks = []build.TaskCache{
		{
			Status: evergreen.TaskSystemFailed,
		},
	}
	s.Equal("none succeeded, none failed, 1 internal errors in 10s", taskStatusToDesc(b))

	b.Tasks = []build.TaskCache{
		{
			Status: evergreen.TaskFailed,
		},
	}
	s.Equal("none succeeded, 1 failed in 10s", taskStatusToDesc(b))
}
