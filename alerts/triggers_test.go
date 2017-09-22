package alerts

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
)

var (
	schedulerTestConf = testutil.TestConfig()
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
	Revision:            "aaa",
	RevisionOrderNumber: 3,
}

var testProject = model.ProjectRef{
	Identifier: "testProject",
	BatchTime:  5,
}

var testVersion = version.Version{
	Identifier: "testProject",
	Requester:  evergreen.RepotrackerVersionRequester,
	Revision:   "aaa",
	Config: `
buildvariants:
- name: bv1
  batchtime: 60
- name: bv2
  batchtime: 1
- name: bv3
`}

func hasTrigger(triggers []Trigger, t Trigger) bool {
	for _, trig := range triggers {
		if trig.Id() == t.Id() {
			return true
		}
	}
	return false
}

func TestEmptyTaskTriggers(t *testing.T) {
	testutil.HandleTestingErr(db.Clear(task.Collection), t, "problem clearing collection")
	testutil.HandleTestingErr(db.Clear(alertrecord.Collection), t, "problem clearing collection")

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
		testutil.HandleTestingErr(db.Clear(task.Collection), t, "problem clearing collection")
		testutil.HandleTestingErr(db.Clear(alertrecord.Collection), t, "problem clearing collection")
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
		testutil.HandleTestingErr(db.Clear(task.Collection), t, "problem clearing collection")
		testutil.HandleTestingErr(db.Clear(model.ProjectRefCollection), t, "problem clearing collection")
		testutil.HandleTestingErr(db.Clear(version.Collection), t, "problem clearing collection")
		So(testProject.Insert(), ShouldBeNil)
		So(testVersion.Insert(), ShouldBeNil)
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
			So(len(triggers), ShouldEqual, 2)
			So(triggers[0].Id(), ShouldEqual, TaskFailed{}.Id())
		})
	})
}

func TestTransitionResend(t *testing.T) {
	Convey("With a set of failures that previously transitioned", t, func() {
		So(db.Clear(task.Collection), ShouldBeNil)
		So(db.Clear(alertrecord.Collection), ShouldBeNil)
		So(db.Clear(model.ProjectRefCollection), ShouldBeNil)
		So(db.Clear(version.Collection), ShouldBeNil)
		// insert 15 failed task documents
		ts := []task.Task{
			*testTask,
			*testTask,
			*testTask,
		}
		for i := range ts {
			ts[i].Id = fmt.Sprintf("t%v", i)
			ts[i].BuildVariant = fmt.Sprintf("bv%v", i+1)
			if i == 1 {
				ts[i].FinishTime = time.Now().Add(-80 * time.Hour)
			} else {
				ts[i].FinishTime = time.Now().Add(-10 * time.Hour)
			}
			So(ts[i].Insert(), ShouldBeNil)
		}

		pastAlert := alertrecord.AlertRecord{
			Id:        bson.NewObjectId(),
			Type:      alertrecord.TaskFailTransitionId,
			TaskName:  ts[0].DisplayName,
			ProjectId: ts[0].Project,
		}
		So(testProject.Insert(), ShouldBeNil)
		So(testVersion.Insert(), ShouldBeNil)
		Convey("a failure 10 minutes ago with a batch time of 60 should not retrigger", func() {
			pastAlert.TaskId = ts[0].Id
			pastAlert.Variant = ts[0].BuildVariant
			So(pastAlert.Insert(), ShouldBeNil)
			ctx, err := getTaskTriggerContext(&ts[0])
			So(err, ShouldBeNil)
			ctx.previousCompleted = &task.Task{Status: evergreen.TaskFailed}
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeFalse)
		})
		Convey("a failure 2 days ago should retrigger", func() {
			pastAlert.TaskId = ts[1].Id
			pastAlert.Variant = ts[1].BuildVariant
			So(pastAlert.Insert(), ShouldBeNil)
			ctx, err := getTaskTriggerContext(&ts[1])
			So(err, ShouldBeNil)
			ctx.previousCompleted = &task.Task{Status: evergreen.TaskFailed}
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeTrue)
		})
		Convey("a failure 10 minutes ago with a batch time of 5 should not retrigger", func() {
			pastAlert.TaskId = ts[2].Id
			pastAlert.Variant = ts[2].BuildVariant
			So(pastAlert.Insert(), ShouldBeNil)
			ctx, err := getTaskTriggerContext(&ts[2])
			So(err, ShouldBeNil)
			ctx.previousCompleted = &task.Task{Status: evergreen.TaskFailed}
			triggers, err := getActiveTaskFailureTriggers(*ctx)
			So(err, ShouldBeNil)
			So(hasTrigger(triggers, TaskFailTransition{}), ShouldBeFalse)
		})
	})
}

func TestSpawnExpireWarningTrigger(t *testing.T) {
	Convey("With a spawnhost due to expire in two hours", t, func() {
		So(db.Clear(host.Collection), ShouldBeNil)
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

func TestReachedFailureLimit(t *testing.T) {
	Convey("With 3 failed task and relevant project variants", t, func() {
		testutil.HandleTestingErr(db.Clear(task.Collection), t, "problem clearing collection")
		testutil.HandleTestingErr(db.Clear(model.ProjectRefCollection), t, "problem clearing collection")
		testutil.HandleTestingErr(db.Clear(version.Collection), t, "problem clearing collection")
		t := task.Task{
			Id:           "t1",
			Revision:     "aaa",
			Project:      "testProject",
			BuildVariant: "bv1",
			FinishTime:   time.Now().Add(-time.Hour),
		}

		So(t.Insert(), ShouldBeNil)
		t.Id = "t2"
		t.BuildVariant = "bv2"
		So(t.Insert(), ShouldBeNil)
		t.Id = "t3"
		t.BuildVariant = "bv3"
		So(t.Insert(), ShouldBeNil)
		t.Id = "t4"
		t.FinishTime = time.Now().Add(-72 * time.Hour)
		So(t.Insert(), ShouldBeNil)

		So(testProject.Insert(), ShouldBeNil)
		So(testVersion.Insert(), ShouldBeNil)

		Convey("failures that have repeated recently, shouldn't resend", func() {
			out, err := taskFinishedTwoOrMoreDaysAgo("t1")
			So(err, ShouldBeNil)
			So(out, ShouldBeFalse)
			out, err = taskFinishedTwoOrMoreDaysAgo("t2")
			So(err, ShouldBeNil)
			So(out, ShouldBeFalse)
			out, err = taskFinishedTwoOrMoreDaysAgo("t3")
			So(err, ShouldBeNil)
			So(out, ShouldBeFalse)
		})

		Convey("tasks that finished more than 2 days ago should resend", func() {
			out, err := taskFinishedTwoOrMoreDaysAgo("t4")
			So(err, ShouldBeNil)
			So(out, ShouldBeTrue)
		})
	})
}
