package trigger

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestTaskTriggers(t *testing.T) {
	suite.Run(t, &taskSuite{})
}

type taskSuite struct {
	event event.EventLogEntry
	data  *event.TaskEventData
	task  task.Task
	subs  []event.Subscription

	suite.Suite
}

func (s *taskSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *taskSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, task.Collection, event.SubscriptionsCollection, alertrecord.Collection))
	startTime := time.Now().Truncate(time.Millisecond)

	s.task = task.Task{
		Id:           "test",
		Version:      "test",
		BuildId:      "test",
		BuildVariant: "test",
		DistroId:     "test",
		Project:      "test",
		DisplayName:  "Test",
		StartTime:    startTime,
		FinishTime:   startTime.Add(10 * time.Minute),
	}
	s.NoError(s.task.Insert())

	s.data = &event.TaskEventData{}
	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypeTask,
		ResourceId:   "test",
		Data:         s.data,
	}

	s.subs = []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeTask,
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
			Type:    event.ResourceTypeTask,
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
			Type:    event.ResourceTypeTask,
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

func (s *taskSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)

	s.task.Status = evergreen.TaskSucceeded
	s.data.Status = evergreen.TaskSucceeded
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.task.Status = evergreen.TaskFailed
	s.data.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)
}

func (s *taskSuite) TestSuccess() {
	gen, err := taskSuccess(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.TaskFailed
	gen, err = taskSuccess(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.TaskSucceeded
	gen, err = taskSuccess(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("success", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "success",
	})
}

func (s *taskSuite) TestFailure() {
	s.data.Status = evergreen.TaskSucceeded
	gen, err := taskFailure(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.TaskFailed
	gen, err = taskFailure(s.data, &s.task)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("failure", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "failure",
	})
}

func (s *taskSuite) TestOutcome() {
	s.data.Status = evergreen.TaskStarted
	gen, err := taskOutcome(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	s.data.Status = evergreen.TaskSucceeded
	gen, err = taskOutcome(s.data, &s.task)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())

	s.data.Status = evergreen.TaskFailed
	gen, err = taskOutcome(s.data, &s.task)
	s.NoError(err)
	s.Require().NotNil(gen)
	s.False(gen.isEmpty())
	s.Equal("outcome", gen.triggerName)
	s.Contains(gen.selectors, event.Selector{
		Type: "trigger",
		Data: "outcome",
	})
}

func (s *taskSuite) TestFirstFailureInVersion() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	gen, err := taskFirstFailureInVersion(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)

	// rerun that fails should not do anything
	gen, err = taskFirstFailureInVersion(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	gen, err = taskFirstFailureInVersion(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs with other tasks in other builds should not do anything
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	gen, err = taskFirstFailureInVersion(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs with other tasks in other versions should still generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	gen, err = taskFirstFailureInVersion(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)
}

func (s *taskSuite) TestFirstFailureInBuild() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	gen, err := taskFirstFailureInBuild(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)

	// rerun that fails should not do anything
	gen, err = taskFirstFailureInBuild(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	gen, err = taskFirstFailureInBuild(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs with other tasks in other builds should generate
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	gen, err = taskFirstFailureInBuild(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)

	// subsequent runs with other tasks in other versions should generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	gen, err = taskFirstFailureInBuild(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)
}

func (s *taskSuite) TestFirstFailureInVersionWithName() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	gen, err := taskFirstFailureInVersionWithName(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)

	// rerun that fails should not do anything
	gen, err = taskFirstFailureInVersionWithName(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	gen, err = taskFirstFailureInVersionWithName(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs with other tasks in other builds should not generate
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	gen, err = taskFirstFailureInVersionWithName(s.data, &s.task)
	s.NoError(err)
	s.Nil(gen)

	// subsequent runs in other versions should generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	gen, err = taskFirstFailureInVersionWithName(s.data, &s.task)
	s.NoError(err)
	s.NotNil(gen)
}
