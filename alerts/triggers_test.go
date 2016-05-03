package alerts

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	schedulerTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(schedulerTestConf))
}

// Used as a template for task objects inserted/queried on within this file
var testTask = &task.Task{
	Id:                  "testTask",
	Status:              evergreen.TaskFailed,
	DisplayName:         "compile",
	Project:             "testProject",
	BuildVariant:        "testVariant",
	Version:             "testVersion",
	RevisionOrderNumber: 3,
}

func hasTrigger(triggers []Trigger, t Trigger) bool {
	for _, trig := range triggers {
		if trig.Id() == t.Id() {
			return true
		}
	}
	return false
}

func TestEmptyTaskTriggers(t *testing.T) {
	db.Clear(task.Collection)
	db.Clear(alertrecord.Collection)
	Convey("With no existing tasks in the database", t, func() {
		Convey("a newly failed task should return all triggers", func() {
			// pre-bookkeeping
			ctx, err := getTaskTriggerContext(testTask)
			So(err, ShouldBeNil)
			So(ctx.previousCompleted, ShouldBeNil)
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(len(triggers), ShouldEqual, 5)
			So(hasTrigger(triggers, TaskFailed{}), ShouldBeTrue)
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeTrue)
			So(hasTrigger(triggers, FirstFailureInVersion{}), ShouldBeTrue)
			So(hasTrigger(triggers, FirstFailureInVariant{}), ShouldBeTrue)
			So(hasTrigger(triggers, FirstFailureInTaskType{}), ShouldBeTrue)

			// post-bookkeeping
			err = storeTriggerBookkeeping(*ctx, triggers)
			So(err, ShouldBeNil)
			triggers, err = getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)

			So(len(triggers), ShouldEqual, 2)
			So(hasTrigger(triggers, TaskFailed{}), ShouldBeTrue)

			// The previous task doesn't exist, so this will still be true
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeTrue)
		})
		Convey("a successful task should trigger nothing", func() {
			testTask.Status = evergreen.TaskSucceeded
			ctx, err := getTaskTriggerContext(testTask)
			So(err, ShouldBeNil)
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(len(triggers), ShouldEqual, 0)
		})
	})
}

func TestExistingPassedTaskTriggers(t *testing.T) {
	Convey("With a previously passing instance of task in the database", t, func() {
		db.Clear(task.Collection)
		db.Clear(alertrecord.Collection)
		testTask.Status = evergreen.TaskSucceeded
		err := testTask.Insert()
		So(err, ShouldBeNil)
		t2 := &task.Task{
			Id:                  "testTask2",
			Status:              evergreen.TaskFailed,
			DisplayName:         testTask.DisplayName,
			Project:             testTask.Project,
			BuildVariant:        testTask.BuildVariant,
			Version:             "testVersion2",
			RevisionOrderNumber: testTask.RevisionOrderNumber + 2,
		}
		Convey("a newly failed task should trigger TaskFailed and TaskFailTransition", func() {
			ctx, err := getTaskTriggerContext(t2)
			So(err, ShouldBeNil)
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(hasTrigger(triggers, TaskFailed{}), ShouldBeTrue)
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeTrue)
		})
		Convey("a newly failed task should not trigger TaskFailTransition after bookkeeping is already done", func() {
			// Pre-bookkeeping

			ctx, err := getTaskTriggerContext(t2)
			So(err, ShouldBeNil)
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(hasTrigger(triggers, TaskFailed{}), ShouldBeTrue)
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeTrue)

			// Post-bookkeeping
			err = storeTriggerBookkeeping(*ctx, triggers)
			So(err, ShouldBeNil)
			triggers, err = getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)

			So(hasTrigger(triggers, TaskFailed{}), ShouldBeTrue)
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeFalse)
		})
	})
}

func TestExistingFailedTaskTriggers(t *testing.T) {
	Convey("With a previously failed instance of task in the database", t, func() {
		db.Clear(task.Collection)
		testTask.Status = evergreen.TaskFailed
		err := testTask.Insert()
		So(err, ShouldBeNil)
		Convey("a newly failed task should trigger TaskFailed but *not* TaskFailTransition", func() {
			t2 := &task.Task{
				Id:                  "testTask2",
				Status:              evergreen.TaskFailed,
				DisplayName:         testTask.DisplayName,
				Project:             testTask.Project,
				BuildVariant:        testTask.BuildVariant,
				Version:             "testVersion2",
				RevisionOrderNumber: testTask.RevisionOrderNumber + 2,
			}
			ctx, err := getTaskTriggerContext(t2)
			So(err, ShouldBeNil)
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(len(triggers), ShouldEqual, 1)
			So(triggers[0].Id(), ShouldEqual, TaskFailed{}.Id())
		})
	})
}

func TestSpawnExpireWarningTrigger(t *testing.T) {
	Convey("With a spawnhost due to expire in two hours", t, func() {
		db.Clear(host.Collection)
		testHost := host.Host{
			Id:             "testhost",
			StartedBy:      "test_user",
			Status:         "running",
			ExpirationTime: time.Now().Add(1 * time.Hour),
		}

		trigger := SpawnTwoHourWarning{}
		ctx := triggerContext{host: &testHost}
		shouldExec, err := trigger.ShouldExecute(ctx)
		So(err, ShouldBeNil)
		So(shouldExec, ShouldBeTrue)

		// run bookkeeping
		err = storeTriggerBookkeeping(ctx, []Trigger{trigger})
		So(err, ShouldBeNil)

		// should exec should now return false
		shouldExec, err = trigger.ShouldExecute(ctx)
		So(err, ShouldBeNil)
		So(shouldExec, ShouldBeFalse)
	})
}
