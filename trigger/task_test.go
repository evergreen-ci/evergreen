package trigger

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestBuildBreakNotificationsFromRepotracker(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, task.Collection, user.Collection, event.SubscriptionsCollection, build.Collection))
	proj := model.ProjectRef{
		Identifier:           "proj",
		NotifyOnBuildFailure: true,
		Admins:               []string{"admin"},
	}
	assert.NoError(proj.Insert())
	v1 := model.Version{
		Id:         "v1",
		Identifier: proj.Identifier,
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(v1.Insert())
	b1 := build.Build{
		Id:      "b1",
		Version: v1.Id,
	}
	assert.NoError(b1.Insert())
	t1 := task.Task{
		Id:          "t1",
		Version:     v1.Id,
		BuildId:     b1.Id,
		Status:      evergreen.TaskFailed,
		Project:     proj.Identifier,
		Requester:   evergreen.RepotrackerVersionRequester,
		DisplayName: "t1",
	}
	assert.NoError(t1.Insert())
	u := user.DBUser{
		Id:           "admin",
		EmailAddress: "a@b.com",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{
				BuildBreak: user.PreferenceSlack,
			},
		},
	}
	assert.NoError(u.Insert())

	// a build break that no one is subscribed to should go to admins
	assert.NoError(repotracker.AddBuildBreakSubscriptions(&v1, &proj))
	e := event.EventLogEntry{
		ResourceType: event.ResourceTypeTask,
		ResourceId:   t1.Id,
		EventType:    event.TaskFinished,
		Data: &event.TaskEventData{
			Status: evergreen.TaskFailed,
		},
	}
	n, err := NotificationsFromEvent(&e)
	assert.NoError(err)
	assert.Len(n, 1)

	// a build triggered build break that the committer is subscribed to
	// should only go to admins
	v2 := model.Version{
		Id:         "v2",
		Identifier: proj.Identifier,
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(v2.Insert())
	b2 := build.Build{
		Id:      "b2",
		Version: v2.Id,
	}
	assert.NoError(b2.Insert())
	t2 := task.Task{
		Id:          "t2",
		Version:     v2.Id,
		BuildId:     b2.Id,
		Status:      evergreen.TaskFailed,
		Project:     proj.Identifier,
		Requester:   evergreen.RepotrackerVersionRequester,
		TriggerID:   "abc",
		DisplayName: "t2",
	}
	assert.NoError(t2.Insert())
	sub := event.NewBuildBreakSubscriptionByOwner("me", event.Subscriber{
		Type:   event.EmailSubscriberType,
		Target: "committer@example.com",
	})
	assert.NoError(sub.Upsert())
	assert.NoError(repotracker.AddBuildBreakSubscriptions(&v2, &proj))
	e = event.EventLogEntry{
		ResourceType: event.ResourceTypeTask,
		ResourceId:   t2.Id,
		EventType:    event.TaskFinished,
		Data: &event.TaskEventData{
			Status: evergreen.TaskFailed,
		},
	}
	n, err = NotificationsFromEvent(&e)
	assert.NoError(err)
	grip.Error(err)
	assert.Len(n, 1)
	assert.EqualValues(user.PreferenceSlack, n[0].Subscriber.Type)
}

func TestTaskTriggers(t *testing.T) {
	suite.Run(t, &taskSuite{})
}

type taskSuite struct {
	event      event.EventLogEntry
	data       *event.TaskEventData
	task       task.Task
	build      build.Build
	projectRef model.ProjectRef
	subs       []event.Subscription

	t *taskTriggers

	suite.Suite
}

func (s *taskSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &taskTriggers{})
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *taskSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, task.Collection, task.OldCollection, model.VersionCollection, event.SubscriptionsCollection, alertrecord.Collection, testresult.Collection, event.SubscriptionsCollection, build.Collection, model.ProjectRefCollection))
	startTime := time.Now().Truncate(time.Millisecond).Add(-time.Hour)

	s.task = task.Task{
		Id:                  "test",
		Version:             "test_version_id",
		BuildId:             "test_build_id",
		BuildVariant:        "test_build_variant",
		DistroId:            "test_distro_id",
		Project:             "test_project",
		DisplayName:         "test-display-name",
		StartTime:           startTime,
		FinishTime:          startTime.Add(20 * time.Minute),
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	s.NoError(s.task.Insert())

	s.projectRef = model.ProjectRef{
		Identifier: "test_project",
	}
	s.NoError(s.projectRef.Insert())

	s.build = build.Build{
		Id: "test_build_id",
	}
	s.NoError(s.build.Insert())

	s.data = &event.TaskEventData{
		Status: evergreen.TaskStarted,
	}
	s.event = event.EventLogEntry{
		ID:           "event1234",
		ResourceType: event.ResourceTypeTask,
		EventType:    event.TaskFinished,
		ResourceId:   "test",
		Data:         s.data,
	}
	v := model.Version{
		Id:       "test_version_id",
		AuthorID: "me",
	}
	s.NoError(v.Insert())

	apiSub := event.Subscriber{
		Type: event.EvergreenWebhookSubscriberType,
		Target: &event.WebhookSubscriber{
			URL:    "http://example.com/2",
			Secret: []byte("secret"),
		},
	}

	s.subs = []event.Subscription{
		event.NewSubscriptionByID(event.ResourceTypeTask, triggerOutcome, s.event.ResourceId, apiSub),
		event.NewSubscriptionByID(event.ResourceTypeTask, triggerSuccess, s.event.ResourceId, apiSub),
		event.NewSubscriptionByID(event.ResourceTypeTask, triggerFailure, s.event.ResourceId, apiSub),
		{
			ID:           bson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeTask,
			Trigger:      triggerExceedsDuration,
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
				event.TaskDurationKey: "300",
			},
		},
		{
			ID:           bson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeTask,
			Trigger:      triggerRuntimeChangeByPercent,
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.JIRACommentSubscriberType,
				Target: "A-2",
			},
			Owner: "someone",
			TriggerData: map[string]string{
				event.TaskPercentChangeKey: "50",
			},
		},
		{
			ID:           bson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeTask,
			Trigger:      triggerRuntimeChangeByPercent,
			Selectors: []event.Selector{
				{
					Type: "project",
					Data: "test_project",
				},
				{
					Type: "requester",
					Data: evergreen.RepotrackerVersionRequester,
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "email",
			},
			RegexSelectors: []event.Selector{
				{
					Type: selectorDisplayName,
					Data: "test-display-name",
				},
			},
			Owner:     "test_project",
			OwnerType: event.OwnerTypeProject,
			TriggerData: map[string]string{
				event.TaskPercentChangeKey: "10",
			},
		},
		event.NewBuildBreakSubscriptionByOwner("me", event.Subscriber{
			Type:   event.JIRACommentSubscriberType,
			Target: "A-3",
		}),
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

func (s *taskSuite) TestTriggerEvent() {
	s.NoError(db.ClearCollections(task.Collection, event.SubscriptionsCollection))
	sub := &event.Subscription{
		ID:           bson.NewObjectId().Hex(),
		ResourceType: event.ResourceTypeTask,
		Trigger:      triggerOutcome,
		Selectors: []event.Selector{
			{
				Type: "id",
				Data: s.event.ResourceId,
			},
			{
				Type: "requester",
				Data: evergreen.RepotrackerVersionRequester,
			},
		},
		Subscriber: event.Subscriber{
			Type:   event.JIRACommentSubscriberType,
			Target: "A-1",
		},
		Owner: "someone",
	}
	s.NoError(sub.Upsert())
	t := task.Task{
		Id:                  "test",
		Version:             "test_version_id",
		BuildId:             "test_build_id",
		BuildVariant:        "test_build_variant",
		DistroId:            "test_distro_id",
		Project:             "test_project",
		DisplayName:         "test-display-name",
		RevisionOrderNumber: 1,
		Requester:           evergreen.TriggerRequester,
		Status:              evergreen.TaskFailed,
	}
	s.NoError(t.Insert())

	s.data.Status = evergreen.TaskFailed
	s.event.Data = s.data
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 1)
}

func (s *taskSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 1)

	s.task.Status = evergreen.TaskSucceeded
	s.data.Status = evergreen.TaskSucceeded
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 3)

	s.task.Status = evergreen.TaskFailed
	s.data.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 4)

	s.task.DisplayOnly = true
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Empty(n)
}

func (s *taskSuite) TestExecutionTask() {
	t := task.Task{
		DisplayName:    "displaytask",
		ExecutionTasks: []string{s.task.Id},
	}
	s.NoError(t.Insert())
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Require().Len(n, 1)
	s.Contains(*n[0].Payload.(*string), "displaytask")
	s.Contains(*n[0].Payload.(*string), "https://evergreen.mongodb.com/task/test/0")
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
	s.build.Id = "test2"
	s.NoError(s.build.Insert())
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
	s.build.Id = "test2"
	s.NoError(s.build.Insert())
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
	s.build.Id = "test2"
	s.NoError(s.build.Insert())
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
	s.task.RevisionOrderNumber = 0
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	// brand new task fails should generate
	s.task.RevisionOrderNumber = 1
	s.task.Id = "task1"
	s.NoError(s.task.Insert())
	n, err := s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// next fail shouldn't generate
	s.task.RevisionOrderNumber = 2
	s.task.Id = "test2"
	s.NoError(s.task.Insert())

	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// successful task shouldn't generate
	s.build.Id = "test3"
	s.NoError(s.build.Insert())
	s.task.Id = "test3"
	s.task.Version = "test3"
	s.task.BuildId = "test3"
	s.task.RevisionOrderNumber = 3
	s.task.Status = evergreen.TaskSucceeded
	s.data.Status = evergreen.TaskSucceeded
	s.NoError(s.task.Insert())

	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// formerly succeeding task should generate
	s.build.Id = "test4"
	s.NoError(s.build.Insert())
	s.task.Id = "test4"
	s.task.Version = "test4"
	s.task.BuildId = "test4"
	s.task.RevisionOrderNumber = 4
	s.task.Status = evergreen.TaskFailed
	s.data.Status = evergreen.TaskFailed
	s.NoError(s.task.Insert())

	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// Don't renotify if it's recent
	s.build.Id = "test5"
	s.NoError(s.build.Insert())
	s.task.Id = "test5"
	s.task.Version = "test5"
	s.task.BuildId = "test5"
	s.task.RevisionOrderNumber = 5
	s.NoError(s.task.Insert())
	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// already failing task should not renotify if before the renotification interval
	s.subs[2].TriggerData = map[string]string{event.RenotifyIntervalKey: "96"}
	oldFinishTime := time.Now().Add(-3 * 24 * time.Hour)
	s.NoError(db.Update(task.Collection, bson.M{"_id": "test4"}, bson.M{
		"$set": bson.M{
			"finish_time": oldFinishTime,
		},
	}))
	s.build.Id = "test6"
	s.NoError(s.build.Insert())
	s.task.Id = "test6"
	s.task.Version = "test6"
	s.task.BuildId = "test6"
	s.task.RevisionOrderNumber = 6
	s.NoError(s.task.Insert())
	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// but should renotify if after the interval
	s.subs[2].TriggerData = nil // use the default value of 48 hours
	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	// if regression was trigged after an older success, we should generate
	s.build.Id = "test7"
	s.NoError(s.build.Insert())
	s.task.Id = "test7"
	s.task.Version = "test7"
	s.task.BuildId = "test7"
	s.task.RevisionOrderNumber = 7
	s.task.Status = evergreen.TaskSucceeded
	s.NoError(s.task.Insert())

	s.build.Id = "test8"
	s.NoError(s.build.Insert())
	s.task.Id = "test8"
	s.task.Version = "test8"
	s.task.BuildId = "test8"
	s.task.RevisionOrderNumber = 8
	s.task.Status = evergreen.TaskFailed
	s.NoError(s.task.Insert())
	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// suppose we reran task test4, it shouldn't generate because we already
	// alerted on it
	task4 := &task.Task{}
	s.NoError(db.FindOneQ(task.Collection, db.Query(bson.M{"_id": "test4"}), task4))
	s.NotZero(*task4)
	task4.Execution = 1
	s.task = *task4
	n, err = s.t.taskRegression(&s.subs[2])
	s.NoError(err)
	s.Nil(n)
}

func (s *taskSuite) makeTask(n int, taskStatus string) {
	s.task.Id = fmt.Sprintf("task_%d", n)
	s.task.Version = fmt.Sprintf("version_%d", n)
	s.task.BuildId = fmt.Sprintf("build_id_%d", n)
	s.task.RevisionOrderNumber = n
	s.task.Status = taskStatus
	s.data.Status = taskStatus
	s.event.ResourceId = s.task.Id
	s.NoError(s.task.Insert())
	v := model.Version{
		Id: s.task.Version,
	}
	s.NoError(v.Insert())

	s.build.Id = s.task.BuildId
	s.NoError(s.build.Insert())
}

func (s *taskSuite) makeTest(n, execution int, testName, testStatus string) {
	if len(testName) == 0 {
		testName = "test_0"
	}
	results := testresult.TestResult{
		ID:        bson.NewObjectId(),
		TestFile:  testName,
		TaskID:    s.task.Id,
		Execution: execution,
		Status:    testStatus,
	}
	s.NoError(results.Insert())
}

func (s *taskSuite) tryDoubleTrigger(shouldGenerate bool) {
	s.t = s.makeTaskTriggers(s.task.Id, s.task.Execution)
	n, err := s.t.taskRegressionByTest(&s.subs[2])
	s.NoError(err)
	msg := fmt.Sprintf("expected nil notification; got '%s'", s.task.Id)
	if shouldGenerate {
		msg = "expected non nil notification"
	}
	s.Equal(shouldGenerate, n != nil, msg)

	// triggering the notification again should not generate anything
	n, err = s.t.taskRegressionByTest(&s.subs[2])
	s.NoError(err)
	s.Nil(n)
}

func (s *taskSuite) TestRegressionByTestSimpleRegression() {
	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	// brand new test fails should generate
	s.makeTask(1, evergreen.TaskFailed)
	s.makeTest(1, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// next fail with same test shouldn't generate
	s.makeTask(2, evergreen.TaskFailed)
	s.makeTest(2, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(false)

	// but if we add a new failed test, it should notify
	s.makeTask(3, evergreen.TaskFailed)
	s.makeTest(3, 0, "test_1", evergreen.TestFailedStatus)
	s.makeTest(3, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// transition to failure
	s.makeTask(4, evergreen.TaskSucceeded)
	s.makeTest(4, 0, "", evergreen.TestSucceededStatus)
	s.tryDoubleTrigger(false)

	s.makeTask(5, evergreen.TaskFailed)
	s.makeTest(5, 0, "test_1", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)
}

func (s *taskSuite) TestRegressionByTestWithNonAlertingStatuses() {
	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	// brand new task that succeeds should not generate
	s.makeTask(10, evergreen.TaskSucceeded)
	s.makeTest(11, 0, "", evergreen.TestSucceededStatus)
	s.tryDoubleTrigger(false)

	// even after a failed task
	s.makeTask(12, evergreen.TaskFailed)
	s.makeTest(12, 0, "", evergreen.TestFailedStatus)

	s.makeTask(13, evergreen.TaskSucceeded)
	s.makeTest(13, 0, "", evergreen.TestSucceededStatus)
	s.tryDoubleTrigger(false)
}

func (s *taskSuite) TestRegressionByTestWithTestChanges() {
	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	// given a task with a failing test, and a succeeding one...
	s.makeTask(14, evergreen.TaskFailed)
	s.makeTest(14, 0, "", evergreen.TestFailedStatus)
	s.makeTest(14, 0, "test_1", evergreen.TestSucceededStatus)
	s.tryDoubleTrigger(true)

	// Remove the successful test, but leave the failing one. Since we
	// already notified, this should not generate
	// failed test
	s.makeTask(15, evergreen.TaskFailed)
	s.makeTest(15, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(false)

	// add some successful tests, this should not notify
	s.makeTask(16, evergreen.TaskFailed)
	s.makeTest(16, 0, "", evergreen.TestFailedStatus)
	s.makeTest(16, 0, "test_1", evergreen.TestSucceededStatus)
	s.makeTest(16, 0, "test_2", evergreen.TestSucceededStatus)
	s.tryDoubleTrigger(false)
}

func (s *taskSuite) TestRegressionByTestWithReruns() {
	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	// insert a couple of successful tasks
	s.makeTask(17, evergreen.TaskSucceeded)
	s.makeTest(17, 0, "", evergreen.TestSucceededStatus)

	s.makeTask(18, evergreen.TaskSucceeded)
	s.makeTest(18, 0, "", evergreen.TestSucceededStatus)

	task18 := s.task

	s.makeTask(19, evergreen.TaskSucceeded)
	s.makeTest(19, 0, "", evergreen.TestSucceededStatus)

	// now simulate a rerun of task18 failing
	s.task = task18
	s.NoError(s.task.Archive())
	s.task.Status = evergreen.TaskFailed
	s.task.Execution = 1
	s.event.ResourceId = s.task.Id
	s.data.Status = s.task.Status
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	s.makeTest(18, 1, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// make it fail again; it shouldn't generate
	s.NoError(s.task.Archive())
	s.task.Status = evergreen.TaskFailed
	s.task.Execution = 2
	s.event.ResourceId = s.task.Id
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	s.makeTest(18, 2, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(false)
}

func (s *taskSuite) TestRegressionByTestWithTestsWithoutTasks() {
	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	// TaskFailed with no tests should generate
	s.makeTask(22, evergreen.TaskSucceeded)
	s.makeTask(23, evergreen.TaskFailed)
	s.tryDoubleTrigger(true)

	// but not in a subsequent task
	s.makeTask(24, evergreen.TaskFailed)
	s.tryDoubleTrigger(false)

	// try same error status, but now with tests
	s.makeTask(25, evergreen.TaskFailed)
	s.makeTest(25, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// force fully move the time of task 25 back 48 hours
	s.task.FinishTime = time.Now().Add(-48 * time.Hour)
	s.NoError(db.Update(task.Collection, bson.M{task.IdKey: s.task.Id}, &s.task))

	s.makeTask(26, evergreen.TaskFailed)
	s.makeTest(26, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)
}

func (s *taskSuite) TestRegressionByTestWithDuplicateTestNames() {
	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	s.makeTask(26, evergreen.TaskFailed)
	s.makeTest(26, 0, "", evergreen.TestFailedStatus)
	s.makeTest(26, 0, "", evergreen.TestSucceededStatus)
	s.tryDoubleTrigger(true)
}

func (s *taskSuite) TestRegressionByTestWithRegex() {
	sub := event.Subscription{
		ID:           bson.NewObjectId().Hex(),
		ResourceType: event.ResourceTypeTask,
		Trigger:      triggerTaskRegressionByTest,
		Selectors: []event.Selector{
			{
				Type: selectorProject,
				Data: "myproj",
			},
		},
		Subscriber: event.Subscriber{
			Type:   event.EmailSubscriberType,
			Target: "a@b.com",
		},
		TriggerData: map[string]string{
			event.TestRegexKey: "test*",
		},
		Owner: "someone",
	}
	s.NoError(sub.Upsert())

	v1 := model.Version{
		Id:        "v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	s.NoError(v1.Insert())

	t1 := task.Task{
		Id:          "t1",
		Requester:   evergreen.RepotrackerVersionRequester,
		Status:      evergreen.TaskFailed,
		DisplayName: "task1",
		Version:     "v1",
		BuildId:     "test_build_id",
		Project:     "myproj",
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:          "t2",
		Requester:   evergreen.RepotrackerVersionRequester,
		Status:      evergreen.TaskFailed,
		DisplayName: "task2",
		Version:     "v1",
		BuildId:     "test_build_id",
		Project:     "myproj",
	}
	s.NoError(t2.Insert())

	results := []testresult.TestResult{
		{ID: bson.NewObjectId(), TaskID: "t1", TestFile: "test1", Status: evergreen.TestFailedStatus},
		{ID: bson.NewObjectId(), TaskID: "t1", TestFile: "something", Status: evergreen.TestSucceededStatus},
	}
	s.NoError(testresult.InsertMany(results))
	results = []testresult.TestResult{
		{ID: bson.NewObjectId(), TaskID: "t2", TestFile: "test1", Status: evergreen.TestSucceededStatus},
		{ID: bson.NewObjectId(), TaskID: "t2", TestFile: "something", Status: evergreen.TestFailedStatus},
	}
	s.NoError(testresult.InsertMany(results))

	ref := model.ProjectRef{
		Identifier: "myproj",
	}
	s.NoError(ref.Insert())

	willNotify := event.EventLogEntry{
		ResourceType: event.ResourceTypeTask,
		ResourceId:   "t1",
		EventType:    event.TaskFinished,
		Data:         &event.TaskEventData{},
	}
	n, err := NotificationsFromEvent(&willNotify)
	s.NoError(err)
	s.Len(n, 1)
	payload := n[0].Payload.(*message.Email)
	s.Contains(payload.Subject, "task1 (test1)")
	wontNotify := event.EventLogEntry{
		ResourceType: event.ResourceTypeTask,
		ResourceId:   "t2",
		EventType:    event.TaskFinished,
		Data:         &event.TaskEventData{},
	}
	n, err = NotificationsFromEvent(&wontNotify)
	s.NoError(err)
	s.Len(n, 0)
}

func (s *taskSuite) makeTaskTriggers(id string, execution int) *taskTriggers {
	t := makeTaskTriggers()
	e := event.EventLogEntry{
		ResourceId: id,
		Data: &event.TaskEventData{
			Execution: execution,
		},
	}
	s.Require().NoError(t.Fetch(&e))
	return t.(*taskTriggers)
}

func TestIsTestRegression(t *testing.T) {
	assert := assert.New(t)

	assert.True(isTestStatusRegression(evergreen.TestSkippedStatus, evergreen.TestFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSkippedStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSkippedStatus, evergreen.TestSkippedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSkippedStatus, evergreen.TestSucceededStatus))

	assert.False(isTestStatusRegression(evergreen.TestFailedStatus, evergreen.TestFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestFailedStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestFailedStatus, evergreen.TestSkippedStatus))
	assert.False(isTestStatusRegression(evergreen.TestFailedStatus, evergreen.TestSucceededStatus))

	assert.True(isTestStatusRegression(evergreen.TestSucceededStatus, evergreen.TestFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSucceededStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSucceededStatus, evergreen.TestSkippedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSucceededStatus, evergreen.TestSucceededStatus))

	assert.True(isTestStatusRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestSkippedStatus))
	assert.False(isTestStatusRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestSucceededStatus))
}

func TestMapTestResultsByTestFile(t *testing.T) {
	assert := assert.New(t)

	taskDoc := task.Task{}

	statuses := []string{evergreen.TestSucceededStatus, evergreen.TestFailedStatus,
		evergreen.TestSilentlyFailedStatus, evergreen.TestSkippedStatus}

	for i := range statuses {
		first := evergreen.TestFailedStatus
		second := statuses[i]
		if rand.Intn(2) == 0 {
			first = statuses[i]
			second = evergreen.TestFailedStatus
		}
		taskDoc.LocalTestResults = append(taskDoc.LocalTestResults,
			task.TestResult{
				TestFile: fmt.Sprintf("file%d", i),
				Status:   first,
			},
			task.TestResult{
				TestFile: fmt.Sprintf("file%d", i),
				Status:   second,
			},
		)
	}

	m := mapTestResultsByTestFile(&taskDoc)
	assert.Len(m, 4)

	for _, v := range m {
		assert.Equal(evergreen.TestFailedStatus, v.Status)
	}
}

func (s *taskSuite) TestTaskExceedsTime() {
	now := time.Now()
	// task that exceeds time should generate
	s.t.event = &event.EventLogEntry{
		EventType: event.TaskFinished,
	}
	s.t.data.Status = evergreen.TaskSucceeded
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err := s.t.taskExceedsDuration(&s.subs[3])
	s.NoError(err)
	s.NotNil(n)

	// task that does not exceed should not generate
	s.task = task.Task{
		Id:         "test",
		StartTime:  now,
		FinishTime: now.Add(1 * time.Minute),
	}
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskExceedsDuration(&s.subs[3])
	s.NoError(err)
	s.Nil(n)

	// unfinished task should not generate
	s.event.EventType = event.TaskStarted
	n, err = s.t.taskExceedsDuration(&s.subs[3])
	s.NoError(err)
	s.Nil(n)
}

func (s *taskSuite) TestTaskRuntimeChange() {
	// no previous task should not generate
	s.t.event = &event.EventLogEntry{
		EventType: event.TaskFinished,
	}
	n, err := s.t.taskRuntimeChange(&s.subs[4])
	s.NoError(err)
	s.Nil(n)

	// task that exceeds threshold should generate
	lastGreen := task.Task{
		Id:                  "test1",
		BuildVariant:        "test_build_variant",
		DistroId:            "test_distro_id",
		Project:             "test_project",
		DisplayName:         "test-display-name",
		StartTime:           s.task.StartTime.Add(-time.Hour),
		RevisionOrderNumber: -1,
		Status:              evergreen.TaskSucceeded,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	lastGreen.FinishTime = lastGreen.StartTime.Add(10 * time.Minute)
	s.NoError(lastGreen.Insert())
	s.t.task.Status = evergreen.TaskSucceeded
	n, err = s.t.taskRuntimeChange(&s.subs[4])
	s.NoError(err)
	s.NotNil(n)

	// task that does not exceed threshold should not generate
	s.task.FinishTime = s.task.StartTime.Add(13 * time.Minute)
	n, err = s.t.taskRuntimeChange(&s.subs[4])
	s.NoError(err)
	s.Nil(n)

	// task that finished too quickly should generate
	s.task.FinishTime = s.task.StartTime.Add(2 * time.Minute)
	n, err = s.t.taskRuntimeChange(&s.subs[4])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestProjectTrigger() {
	lastGreen := task.Task{
		Id:                  "test1",
		BuildVariant:        "test_build_variant",
		DistroId:            "test_distro_id",
		Project:             "test_project",
		DisplayName:         "test-display-name",
		StartTime:           s.task.StartTime.Add(-time.Hour),
		RevisionOrderNumber: -1,
		Status:              evergreen.TaskSucceeded,
	}
	lastGreen.FinishTime = lastGreen.StartTime.Add(10 * time.Minute)
	s.NoError(lastGreen.Insert())

	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 1)
}

func (s *taskSuite) TestBuildBreak() {
	lastGreen := task.Task{
		Id:           "test1",
		BuildVariant: "test_build_variant",
		DistroId:     "test_distro_id",
		Project:      "test_project",
		DisplayName:  "test-display-name",

		RevisionOrderNumber: -1,
		Status:              evergreen.TaskSucceeded,
	}
	s.NoError(lastGreen.Insert())

	// successful task should not trigger
	s.task.Status = evergreen.TaskSucceeded
	n, err := s.t.buildBreak(&s.subs[5])
	s.NoError(err)
	s.Nil(n)

	// task regression should trigger
	s.task.Status = evergreen.TaskFailed
	n, err = s.t.buildBreak(&s.subs[5])
	s.NoError(err)
	s.NotNil(n)

	// another regression in the same version should not trigger
	n, err = s.t.buildBreak(&s.subs[5])
	s.NoError(err)
	s.Nil(n)
}
