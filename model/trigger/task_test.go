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

	t *taskTriggers

	suite.Suite
}

func (s *taskSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &taskTriggers{})
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *taskSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, task.Collection, event.SubscriptionsCollection, alertrecord.Collection))
	startTime := time.Now().Truncate(time.Millisecond)

	s.task = task.Task{
		Id:                  "test",
		Version:             "test",
		BuildId:             "test",
		BuildVariant:        "test",
		DistroId:            "test",
		Project:             "test",
		DisplayName:         "Test",
		StartTime:           startTime,
		FinishTime:          startTime.Add(10 * time.Minute),
		RevisionOrderNumber: 1,
	}
	s.NoError(s.task.Insert())

	s.data = &event.TaskEventData{
		Status: evergreen.TaskStarted,
	}
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
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeTask,
			Trigger: "exceeds_time",
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
				taskDurationKey: "300",
			},
		},
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeTask,
			Trigger: "exceeds_percentage",
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
				percentIncreaseKey: "50",
			},
		},
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set())

	s.t = makeTaskTriggers().(*taskTriggers)
	s.t.event = &s.event
	s.t.data = s.data
	s.t.task = &s.task
	s.t.uiConfig = *ui
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
	n, err := s.t.taskSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskFailed
	n, err = s.t.taskSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskSucceeded
	n, err = s.t.taskSuccess(&s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFailure() {
	n, err := s.t.taskFailure(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskSucceeded
	n, err = s.t.taskFailure(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskFailed
	n, err = s.t.taskFailure(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestOutcome() {
	s.data.Status = evergreen.TaskStarted
	n, err := s.t.taskOutcome(&s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskSucceeded
	n, err = s.t.taskOutcome(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)

	s.data.Status = evergreen.TaskFailed
	n, err = s.t.taskOutcome(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFirstFailureInVersion() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err := s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// rerun that fails should not do anything
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other builds should not do anything
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other versions should still generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFirstFailureInBuild() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err := s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// rerun that fails should not do anything
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other builds should generate
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// subsequent runs with other tasks in other versions should generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFirstFailureInVersionWithName() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err := s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// rerun that fails should not do anything
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other builds should not generate
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs in other versions should generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestRegression() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	// brand new task fails should generate
	n, err := s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// next fail shouldn't generate
	s.task.RevisionOrderNumber = 2
	s.task.Id = "test2"
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// successful task shouldn't generate
	s.task.Id = "test3"
	s.task.Version = "test3"
	s.task.BuildId = "test3"
	s.task.RevisionOrderNumber = 3
	s.task.Status = evergreen.TaskSucceeded
	s.data.Status = evergreen.TaskSucceeded
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// formerly succeeding task should generate
	s.task.Id = "test4"
	s.task.Version = "test4"
	s.task.BuildId = "test4"
	s.task.RevisionOrderNumber = 4
	s.task.Status = evergreen.TaskFailed
	s.data.Status = evergreen.TaskFailed
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// Don't renotify if it's recent
	s.task.Id = "test5"
	s.task.Version = "test5"
	s.task.BuildId = "test5"
	s.task.RevisionOrderNumber = 5
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// already failing task should renotify if the task failed more than
	// 2 days ago
	oldTime := s.task.FinishTime
	s.task.FinishTime = oldTime.Add(-3 * 24 * time.Hour)
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	s.task.Id = "test6"
	s.task.Version = "test6"
	s.task.BuildId = "test6"
	s.task.RevisionOrderNumber = 6
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
	s.task.FinishTime = oldTime
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	// if regression was trigged after an older success, we should generate
	s.task.Id = "test7"
	s.task.Version = "test7"
	s.task.BuildId = "test7"
	s.task.RevisionOrderNumber = 7
	s.task.Status = evergreen.TaskSucceeded
	s.NoError(s.task.Insert())
	s.task.Id = "test8"
	s.task.Version = "test8"
	s.task.BuildId = "test8"
	s.task.RevisionOrderNumber = 8
	s.task.Status = evergreen.TaskFailed
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// suppose we reran task test4, it shouldn't generate because we already
	// alerted on it
	task4 := &task.Task{}
	s.NoError(db.FindOneQ(task.Collection, db.Query(bson.M{"_id": "test4"}), task4))
	s.NotZero(*task4)
	task4.Execution = 1
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)
}

func (s *taskSuite) TestTaskExceedsTime() {
	now := time.Now()
	// task that exceeds time should generate
	s.t.event = &event.EventLogEntry{
		EventType: event.TaskFinished,
	}
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err := s.t.taskExceedsTime(&s.subs[3])
	s.NoError(err)
	s.NotNil(n)

	// task that does not exceed should not generate
	s.task = task.Task{
		Id:         "test",
		StartTime:  now,
		FinishTime: now.Add(1 * time.Minute),
	}
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskExceedsTime(&s.subs[3])
	s.NoError(err)
	s.Nil(n)

	// unfinished task should not generate
	s.event.EventType = event.TaskStarted
	n, err = s.t.taskExceedsTime(&s.subs[3])
	s.NoError(err)
	s.Nil(n)
}

func (s *taskSuite) TestTaskRuntimeIncrease() {
	now := time.Now()
	lastGreen := task.Task{
		Id:                  "test1",
		BuildVariant:        "test",
		Project:             "test",
		DisplayName:         "Test",
		StartTime:           now,
		FinishTime:          now.Add(10 * time.Minute),
		RevisionOrderNumber: -1,
		Status:              evergreen.TaskSucceeded,
	}
	s.NoError(lastGreen.Insert())

	// task that exceeds threshold should generate
	s.task.FinishTime = s.task.StartTime.Add(20 * time.Minute)
	s.t.event = &event.EventLogEntry{
		EventType: event.TaskFinished,
	}
	n, err := s.t.taskRuntimeIncrease(&s.subs[4])
	s.NoError(err)
	s.NotNil(n)

	// task that does not exceed threshold should not generate
	s.task.FinishTime = s.task.StartTime.Add(13 * time.Minute)
	n, err = s.t.taskRuntimeIncrease(&s.subs[4])
	s.NoError(err)
	s.Nil(n)

	// unfinished task should not generate
	s.event.EventType = event.TaskStarted
	n, err = s.t.taskRuntimeIncrease(&s.subs[4])
	s.NoError(err)
	s.Nil(n)
}
