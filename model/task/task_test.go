package task_test

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

var (
	conf  = testutil.TestConfig()
	oneMs = time.Millisecond
)

func init() {
	db.SetGlobalSessionProvider(conf.SessionFactory())
}

var depTaskIds = []task.Dependency{
	{"td1", evergreen.TaskSucceeded},
	{"td2", evergreen.TaskSucceeded},
	{"td3", ""}, // Default == "success"
	{"td4", evergreen.TaskFailed},
	{"td5", task.AllStatuses},
}

// update statuses of test tasks in the db
func updateTestDepTasks(t *testing.T) {
	// cases for success/default
	for _, depTaskId := range depTaskIds[:3] {
		testutil.HandleTestingErr(task.UpdateOne(
			bson.M{"_id": depTaskId.TaskId},
			bson.M{"$set": bson.M{"status": evergreen.TaskSucceeded}},
		), t, "Error setting task status")
	}
	// cases for * and failure
	for _, depTaskId := range depTaskIds[3:] {
		testutil.HandleTestingErr(task.UpdateOne(
			bson.M{"_id": depTaskId.TaskId},
			bson.M{"$set": bson.M{"status": evergreen.TaskFailed}},
		), t, "Error setting task status")
	}
}

func TestDependenciesMet(t *testing.T) {

	var taskId string
	var taskDoc *task.Task
	var depTasks []*task.Task

	Convey("With a task", t, func() {

		taskId = "t1"

		taskDoc = &task.Task{
			Id: taskId,
		}

		depTasks = []*task.Task{
			{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[1].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[2].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[3].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[4].TaskId, Status: evergreen.TaskUndispatched},
		}

		So(db.Clear(task.Collection), ShouldBeNil)
		for _, depTask := range depTasks {
			So(depTask.Insert(), ShouldBeNil)
		}

		Convey("sanity check the local version of the function in the nil case", func() {
			taskDoc.DependsOn = []task.Dependency{}
			met, err := taskDoc.AllDependenciesSatisfied(map[string]task.Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("if the task has no dependencies its dependencies should"+
			" be met by default", func() {
			taskDoc.DependsOn = []task.Dependency{}
			met, err := taskDoc.DependenciesMet(map[string]task.Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("task with overridden dependencies should be met", func() {
			taskDoc.DependsOn = depTaskIds
			taskDoc.OverrideDependencies = true
			met, err := taskDoc.DependenciesMet(map[string]task.Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("if only some of the tasks dependencies are finished"+
			" successfully, then it should not think its dependencies are met",
			func() {
				taskDoc.DependsOn = depTaskIds
				So(task.UpdateOne(
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				met, err := taskDoc.DependenciesMet(map[string]task.Task{})
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)
			})

		Convey("if all of the tasks dependencies are finished properly, it"+
			" should correctly believe its dependencies are met", func() {
			taskDoc.DependsOn = depTaskIds
			updateTestDepTasks(t)
			met, err := taskDoc.DependenciesMet(map[string]task.Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("tasks not in the dependency cache should be pulled into the"+
			" cache during dependency checking", func() {
			dependencyCache := make(map[string]task.Task)
			taskDoc.DependsOn = depTaskIds
			updateTestDepTasks(t)
			met, err := taskDoc.DependenciesMet(dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			for _, depTaskId := range depTaskIds[:4] {
				So(dependencyCache[depTaskId.TaskId].Id, ShouldEqual, depTaskId.TaskId)
			}
			So(dependencyCache["td5"].Id, ShouldEqual, "td5")
		})

		Convey("cached dependencies should be used rather than fetching them"+
			" from the database", func() {
			updateTestDepTasks(t)
			dependencyCache := make(map[string]task.Task)
			taskDoc.DependsOn = depTaskIds
			met, err := taskDoc.DependenciesMet(dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)

			// alter the dependency cache so that it should seem as if the
			// dependencies are not met
			cachedTask := dependencyCache[depTaskIds[0].TaskId]
			So(cachedTask.Status, ShouldEqual, evergreen.TaskSucceeded)
			cachedTask.Status = evergreen.TaskFailed
			dependencyCache[depTaskIds[0].TaskId] = cachedTask
			met, err = taskDoc.DependenciesMet(dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeFalse)

		})

		Convey("new task resolver should error if cache is empty, but there are deps", func() {
			updateTestDepTasks(t)
			dependencyCache := make(map[string]task.Task)
			taskDoc.DependsOn = depTaskIds
			met, err := taskDoc.AllDependenciesSatisfied(dependencyCache)
			So(err, ShouldNotBeNil)
			So(met, ShouldBeFalse)
		})

		Convey("extraneous tasks in the dependency cache should be ignored",
			func() {
				So(task.UpdateOne(
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(task.UpdateOne(
					bson.M{"_id": depTaskIds[1].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(task.UpdateOne(
					bson.M{"_id": depTaskIds[2].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskFailed,
						},
					},
				), ShouldBeNil)

				dependencyCache := make(map[string]task.Task)
				taskDoc.DependsOn = []task.Dependency{depTaskIds[0], depTaskIds[1],
					depTaskIds[2]}
				met, err := taskDoc.DependenciesMet(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)

				met, err = taskDoc.AllDependenciesSatisfied(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)

				// remove the failed task from the dependencies (but not from
				// the cache).  it should be ignored in the next pass
				taskDoc.DependsOn = []task.Dependency{depTaskIds[0], depTaskIds[1]}
				met, err = taskDoc.DependenciesMet(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeTrue)

				met, err = taskDoc.AllDependenciesSatisfied(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeTrue)
			})
	})
}

func TestSetTasksScheduledTime(t *testing.T) {
	Convey("With some tasks", t, func() {

		So(db.Clear(task.Collection), ShouldBeNil)

		tasks := []task.Task{
			{Id: "t1", ScheduledTime: util.ZeroTime},
			{Id: "t2", ScheduledTime: util.ZeroTime},
			{Id: "t3", ScheduledTime: util.ZeroTime},
		}
		for _, task := range tasks {
			So(task.Insert(), ShouldBeNil)
		}
		Convey("when updating ScheduledTime for some of the tasks", func() {
			testTime := time.Unix(31337, 0)
			So(task.SetTasksScheduledTime(tasks[1:], testTime), ShouldBeNil)

			Convey("the tasks should be updated in memory", func() {
				So(tasks[0].ScheduledTime, ShouldResemble, util.ZeroTime)
				So(tasks[1].ScheduledTime, ShouldResemble, testTime)
				So(tasks[2].ScheduledTime, ShouldResemble, testTime)

				Convey("and in the db", func() {
					// Need to use a margin of error on time tests
					// because of minor differences between how mongo
					// and golang store dates. The date from the db
					// can be interpreted as being a few nanoseconds off
					t1, err := task.FindOne(task.ById("t1"))
					So(err, ShouldBeNil)
					So(t1.ScheduledTime.Round(oneMs), ShouldResemble, util.ZeroTime)
					t2, err := task.FindOne(task.ById("t2"))
					So(err, ShouldBeNil)
					So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
					t3, err := task.FindOne(task.ById("t3"))
					So(err, ShouldBeNil)
					So(t3.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
				})

				Convey("if we update a second time", func() {
					newTime := time.Unix(99999999, 0)
					So(newTime, ShouldHappenAfter, testTime)
					So(task.SetTasksScheduledTime(tasks, newTime), ShouldBeNil)

					Convey("only unset scheduled times should be updated", func() {
						t1, err := task.FindOne(task.ById("t1"))
						So(err, ShouldBeNil)
						So(t1.ScheduledTime.Round(oneMs), ShouldResemble, newTime)
						t2, err := task.FindOne(task.ById("t2"))
						So(err, ShouldBeNil)
						So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						t3, err := task.FindOne(task.ById("t3"))
						So(err, ShouldBeNil)
						So(t3.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
					})
				})

			})

		})
	})
}

func TestTaskSetPriority(t *testing.T) {

	Convey("With a task", t, func() {

		testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
			" '%v' collection", task.Collection)

		tasks := []task.Task{
			{
				Id:        "one",
				DependsOn: []task.Dependency{{"two", ""}, {"three", ""}, {"four", ""}},
				Activated: true,
			},
			{
				Id:        "two",
				Priority:  5,
				Activated: true,
			},
			{
				Id:        "three",
				DependsOn: []task.Dependency{{"five", ""}},
				Activated: true,
			},
			{
				Id:        "four",
				DependsOn: []task.Dependency{{"five", ""}},
				Activated: true,
			},
			{
				Id:        "five",
				Activated: true,
			},
			{
				Id:        "six",
				Activated: true,
			},
		}

		for _, task := range tasks {
			So(task.Insert(), ShouldBeNil)
		}

		Convey("setting its priority should update it in-memory"+
			" and update it and all dependencies in the database", func() {

			So(tasks[0].SetPriority(1, "user"), ShouldBeNil)
			So(tasks[0].Priority, ShouldEqual, 1)

			t, err := task.FindOne(task.ById("one"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(task.ById("two"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 5)

			t, err = task.FindOne(task.ById("three"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(task.ById("four"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "four")
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(task.ById("five"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "five")
			So(t.Priority, ShouldEqual, 1)

			t, err = task.FindOne(task.ById("six"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "six")
			So(t.Priority, ShouldEqual, 0)

		})

		Convey("decreasing priority should update the task but not its dependencies", func() {

			So(tasks[0].SetPriority(1, "user"), ShouldBeNil)
			So(tasks[0].Activated, ShouldEqual, true)
			So(tasks[0].SetPriority(-1, "user"), ShouldBeNil)
			So(tasks[0].Priority, ShouldEqual, -1)

			t, err := task.FindOne(task.ById("one"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, -1)
			So(t.Activated, ShouldEqual, false)

			t, err = task.FindOne(task.ById("two"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 5)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(task.ById("three"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(task.ById("four"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "four")
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(task.ById("five"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "five")
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = task.FindOne(task.ById("six"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "six")
			So(t.Priority, ShouldEqual, 0)
			So(t.Activated, ShouldEqual, true)
		})
	})

}

func TestFindTasksByIds(t *testing.T) {
	Convey("When calling FindTasksByIds...", t, func() {
		So(db.Clear(task.Collection), ShouldBeNil)
		Convey("only tasks with the specified ids should be returned", func() {

			tasks := []task.Task{
				{
					Id: "one",
				},
				{
					Id: "two",
				},
				{
					Id: "three",
				},
			}

			for _, task := range tasks {
				So(task.Insert(), ShouldBeNil)
			}

			dbTasks, err := task.Find(task.ByIds([]string{"one", "two"}))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}

func TestCountSimilarFailingTasks(t *testing.T) {
	Convey("When calling CountSimilarFailingTasks...", t, func() {
		So(db.Clear(task.Collection), ShouldBeNil)
		Convey("only failed tasks with the same project, requester, display "+
			"name and revision but different buildvariants should be returned",
			func() {
				project := "project"
				requester := "testing"
				displayName := "compile"
				buildVariant := "testVariant"
				revision := "asdf ;lkj asdf ;lkj "

				tasks := []task.Task{
					{
						Id:           "one",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "1",
						Revision:     revision,
						Requester:    requester,
					},
					{
						Id:           "two",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// task succeeded so should not be returned
					{
						Id:           "three",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskSucceeded,
					},
					// same buildvariant so should not be returned
					{
						Id:           "four",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "1",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// different project so should not be returned
					{
						Id:           "five",
						Project:      project + "1",
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// different requester so should not be returned
					{
						Id:           "six",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester + "1",
						Status:       evergreen.TaskFailed,
					},
					// different revision so should not be returned
					{
						Id:           "seven",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "1",
						Revision:     revision + "1",
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// different display name so should not be returned
					{
						Id:           "eight",
						Project:      project,
						DisplayName:  displayName + "1",
						BuildVariant: buildVariant,
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
				}

				for _, task := range tasks {
					So(task.Insert(), ShouldBeNil)
				}

				dbTasks, err := tasks[0].CountSimilarFailingTasks()
				So(err, ShouldBeNil)
				So(dbTasks, ShouldEqual, 1)
			})
	})
}

func TestMarkAsDispatched(t *testing.T) {

	var (
		taskId   string
		hostId   string
		buildId  string
		distroId string
		taskDoc  *task.Task
		b        *build.Build
	)

	Convey("With a task", t, func() {

		taskId = "t1"
		hostId = "h1"
		buildId = "b1"
		distroId = "d1"

		taskDoc = &task.Task{
			Id:      taskId,
			BuildId: buildId,
		}

		b = &build.Build{
			Id: buildId,
			Tasks: []build.TaskCache{
				{Id: taskId},
			},
		}

		testutil.HandleTestingErr(
			db.ClearCollections(task.Collection, build.Collection),
			t, "Error clearing test collections")

		So(taskDoc.Insert(), ShouldBeNil)
		So(b.Insert(), ShouldBeNil)

		Convey("when marking the task as dispatched, the fields for"+
			" the task, the host it is on, and the build it is a part of"+
			" should be set to reflect this", func() {

			// mark the task as dispatched
			So(taskDoc.MarkAsDispatched(hostId, distroId, time.Now()), ShouldBeNil)

			// make sure the task's fields were updated, both in ©memory and
			// in the db
			So(taskDoc.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(taskDoc.Status, ShouldEqual, evergreen.TaskDispatched)
			So(taskDoc.HostId, ShouldEqual, hostId)
			So(taskDoc.LastHeartbeat, ShouldResemble, taskDoc.DispatchTime)
			taskDoc, err := task.FindOne(task.ById(taskId))
			So(err, ShouldBeNil)
			So(taskDoc.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(taskDoc.Status, ShouldEqual, evergreen.TaskDispatched)
			So(taskDoc.HostId, ShouldEqual, hostId)
			So(taskDoc.LastHeartbeat, ShouldResemble, taskDoc.DispatchTime)

		})

	})

}

func TestTimeAggregations(t *testing.T) {
	Convey("With multiple tasks with different times", t, func() {
		So(db.Clear(task.Collection), ShouldBeNil)
		task1 := task.Task{Id: "bogus",
			ScheduledTime: time.Unix(1000, 0),
			StartTime:     time.Unix(1010, 0),
			FinishTime:    time.Unix(1030, 0),
			DistroId:      "osx"}
		task2 := task.Task{Id: "fake",
			ScheduledTime: time.Unix(1000, 0),
			StartTime:     time.Unix(1020, 0),
			FinishTime:    time.Unix(1050, 0),
			DistroId:      "osx"}
		task3 := task.Task{Id: "placeholder",
			ScheduledTime: time.Unix(1000, 0),
			StartTime:     time.Unix(1060, 0),
			FinishTime:    time.Unix(1180, 0),
			DistroId:      "templOS"}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(task3.Insert(), ShouldBeNil)

		Convey("on an aggregation on FinishTime - StartTime", func() {
			timeMap, err := task.AverageTaskTimeDifference(
				task.StartTimeKey,
				task.FinishTimeKey,
				task.DistroIdKey,
				util.ZeroTime)
			So(err, ShouldBeNil)

			Convey("the proper averages should be computed", func() {
				// osx = ([1030-1010] + [1050-1020])/2 = (20+30)/2 = 25
				So(timeMap["osx"].Seconds(), ShouldEqual, 25)
				// templOS = (1180 - 1060)/1 = 120/1 = 120
				So(timeMap["templOS"].Seconds(), ShouldEqual, 120)
			})
		})

		Convey("on an aggregation on StartTime - ScheduledTime", func() {
			timeMap, err := task.AverageTaskTimeDifference(
				task.ScheduledTimeKey,
				task.StartTimeKey,
				task.DistroIdKey,
				util.ZeroTime)
			So(err, ShouldBeNil)

			Convey("the proper averages should be computed", func() {
				// osx = ([1010-1000] + [1020-1000])/2 = (10+20)/2 = 15
				So(timeMap["osx"].Seconds(), ShouldEqual, 15)
				// templOS = (1060-1000)/1 = 60/1 = 60
				So(timeMap["templOS"].Seconds(), ShouldEqual, 60)
			})
		})

		Convey("but when given non-time fields", func() {

			Convey("most cases should return an empty map", func() {
				timeMap, err := task.AverageTaskTimeDifference(
					task.IdKey,
					task.DistroIdKey,
					task.DistroIdKey,
					util.ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldBeNil)
				timeMap, err = task.AverageTaskTimeDifference(
					task.DistroIdKey,
					task.SecretKey,
					task.DistroIdKey,
					util.ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldBeNil)
			})

			Convey("special key cases should cause real agg errors", func() {
				timeMap, err := task.AverageTaskTimeDifference(
					task.StartTimeKey,
					"$$$$$$",
					task.DistroIdKey,
					util.ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestEndingTask(t *testing.T) {
	Convey("With tasks that are attempting to be marked as finished", t, func() {
		So(db.Clear(task.Collection), ShouldBeNil)
		Convey("a task that has a start time set", func() {
			now := time.Now()
			t := &task.Task{
				Id:        "taskId",
				Status:    evergreen.TaskStarted,
				StartTime: now.Add(-5 * time.Minute),
			}
			So(t.Insert(), ShouldBeNil)
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			So(t.MarkEnd(now, details), ShouldBeNil)
			t, err := task.FindOne(task.ById(t.Id))
			So(err, ShouldBeNil)
			So(t.Status, ShouldEqual, evergreen.TaskFailed)
			So(t.FinishTime.Unix(), ShouldEqual, now.Unix())
			So(t.StartTime.Unix(), ShouldEqual, now.Add(-5*time.Minute).Unix())
		})
		Convey("a task with no start time set should have one added", func() {
			now := time.Now()
			Convey("a task with a create time < 2 hours should have the start time set to the create time", func() {
				t := &task.Task{
					Id:         "tid",
					Status:     evergreen.TaskDispatched,
					CreateTime: now.Add(-30 * time.Minute),
				}
				So(t.Insert(), ShouldBeNil)
				details := &apimodels.TaskEndDetail{
					Status: evergreen.TaskFailed,
				}
				So(t.MarkEnd(now, details), ShouldBeNil)
				t, err := task.FindOne(task.ById(t.Id))
				So(err, ShouldBeNil)
				So(t.StartTime.Unix(), ShouldEqual, t.CreateTime.Unix())
				So(t.FinishTime.Unix(), ShouldEqual, now.Unix())
			})
			Convey("a task with a create time > 2 hours should have the start time set to two hours"+
				"before the finish time", func() {
				t := &task.Task{
					Id:         "tid",
					Status:     evergreen.TaskDispatched,
					CreateTime: now.Add(-3 * time.Hour),
				}
				So(t.Insert(), ShouldBeNil)
				details := &apimodels.TaskEndDetail{
					Status: evergreen.TaskFailed,
				}
				So(t.MarkEnd(now, details), ShouldBeNil)
				t, err := task.FindOne(task.ById(t.Id))
				So(err, ShouldBeNil)
				startTime := now.Add(-2 * time.Hour)
				So(t.StartTime.Unix(), ShouldEqual, startTime.Unix())
				So(t.FinishTime.Unix(), ShouldEqual, now.Unix())
			})

		})

	})
}

func TestTaskResultOutcome(t *testing.T) {
	assert := assert.New(t)

	tasks := []task.Task{
		{Status: evergreen.TaskUndispatched, Activated: false},                                                                     // 0
		{Status: evergreen.TaskUndispatched, Activated: true},                                                                      // 1
		{Status: evergreen.TaskStarted},                                                                                            // 2
		{Status: evergreen.TaskSucceeded},                                                                                          // 3
		{Status: evergreen.TaskFailed},                                                                                             // 4
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: "system"}},                                           // 5
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: "system", TimedOut: true}},                           // 6
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: "system", TimedOut: true, Description: "heartbeat"}}, // 7
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{TimedOut: true, Description: "heartbeat"}},                 // 8
		{Status: evergreen.TaskSetupFailed, Details: apimodels.TaskEndDetail{Type: "setup"}},                                       // 5
	}

	out := task.GetResultCounts(tasks)
	assert.Equal(len(tasks), out.Total)
	assert.Equal(1, out.Inactive)
	assert.Equal(1, out.Unstarted)
	assert.Equal(1, out.Started)
	assert.Equal(1, out.Succeeded)
	assert.Equal(1, out.Failed)
	assert.Equal(1, out.SystemFailed)
	assert.Equal(1, out.SystemUnresponsive)
	assert.Equal(1, out.SystemTimedOut)
	assert.Equal(1, out.TestTimedOut)
	assert.Equal(1, out.SetupFailed)

	//

	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[0]}).Inactive)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[1]}).Unstarted)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[2]}).Started)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[3]}).Succeeded)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[4]}).Failed)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[5]}).SystemFailed)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[6]}).SystemTimedOut)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[7]}).SystemUnresponsive)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[8]}).TestTimedOut)
	assert.Equal(1, task.GetResultCounts([]task.Task{tasks[9]}).SetupFailed)
}

func TestMergeTestResultsBulk(t *testing.T) {
	testutil.HandleTestingErr(db.Clear(testresult.Collection), t, "error clearing collections")
	assert := assert.New(t)

	tasks := []task.Task{
		{
			Id:        "task1",
			Execution: 0,
		},
		{
			Id:        "task2",
			Execution: 0,
		},
		{
			Id:        "task3",
			Execution: 0,
		},
	}

	assert.NoError((&testresult.TestResult{
		TaskID:    "task1",
		Status:    evergreen.TestFailedStatus,
		Execution: 0,
	}).Insert())
	assert.NoError((&testresult.TestResult{
		TaskID:    "task2",
		Status:    evergreen.TestFailedStatus,
		Execution: 0,
	}).Insert())
	assert.NoError((&testresult.TestResult{
		TaskID:    "task3",
		Status:    evergreen.TestFailedStatus,
		Execution: 0,
	}).Insert())
	assert.NoError((&testresult.TestResult{
		TaskID:    "task1",
		Status:    evergreen.TestFailedStatus,
		Execution: 1,
	}).Insert())
	assert.NoError((&testresult.TestResult{
		TaskID:    "task4",
		Status:    evergreen.TestFailedStatus,
		Execution: 0,
	}).Insert())
	assert.NoError((&testresult.TestResult{
		TaskID:    "task1",
		Status:    evergreen.TestSucceededStatus,
		Execution: 0,
	}).Insert())

	out, err := task.MergeTestResultsBulk(tasks, nil)
	assert.NoError(err)
	count := 0
	for _, t := range out {
		count += len(t.LocalTestResults)
	}
	assert.Equal(4, count)

	query := db.Query(bson.M{
		testresult.StatusKey: evergreen.TestFailedStatus,
	})
	out, err = task.MergeTestResultsBulk(tasks, &query)
	assert.NoError(err)
	count = 0
	for _, t := range out {
		count += len(t.LocalTestResults)
		for _, result := range t.LocalTestResults {
			assert.Equal(evergreen.TestFailedStatus, result.Status)
		}
	}
	assert.Equal(3, count)
}

func TestFindOldTasksByID(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, task.OldCollection))

	taskDoc := task.Task{
		Id: "task",
	}
	assert.NoError(taskDoc.Insert())
	assert.NoError(taskDoc.Archive())
	taskDoc.Execution += 1
	assert.NoError(taskDoc.Archive())
	taskDoc.Execution += 1

	tasks, err := task.FindOld(task.ByOldTaskID("task"))
	assert.NoError(err)
	assert.Len(tasks, 2)
	assert.Equal(0, tasks[0].Execution)
	assert.Equal("task_0", tasks[0].Id)
	assert.Equal("task", tasks[0].OldTaskId)
	assert.Equal(1, tasks[1].Execution)
	assert.Equal("task_1", tasks[1].Id)
	assert.Equal("task", tasks[1].OldTaskId)
}

func TestTaskStatusCount(t *testing.T) {
	assert := assert.New(t)
	counts := task.TaskStatusCount{}
	details := apimodels.TaskEndDetail{
		TimedOut:    true,
		Description: "heartbeat",
	}
	counts.IncrementStatus(evergreen.TaskSetupFailed, details)
	counts.IncrementStatus(evergreen.TaskFailed, apimodels.TaskEndDetail{})
	counts.IncrementStatus(evergreen.TaskDispatched, details)
	counts.IncrementStatus(evergreen.TaskInactive, details)
	assert.Equal(1, counts.TimedOut)
	assert.Equal(1, counts.Failed)
	assert.Equal(1, counts.Started)
	assert.Equal(1, counts.Inactive)
}

func TestFindOneIdOldOrNew(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(task.Collection, task.OldCollection))

	taskDoc := task.Task{
		Id: "task",
	}
	require.NoError(taskDoc.Insert())
	require.NoError(taskDoc.Archive())
	result0 := testresult.TestResult{
		ID:        bson.NewObjectId(),
		TaskID:    "task",
		Execution: 0,
	}
	result1 := testresult.TestResult{
		ID:        bson.NewObjectId(),
		TaskID:    "task",
		Execution: 1,
	}
	require.NoError(result0.Insert())
	require.NoError(result1.Insert())

	task00, err := task.FindOneIdOldOrNew("task", 0)
	assert.NoError(err)
	require.NotNil(task00)
	assert.Equal("task_0", task00.Id)
	assert.Equal(0, task00.Execution)
	assert.Len(task00.LocalTestResults, 1)

	task01, err := task.FindOneIdOldOrNew("task", 1)
	assert.NoError(err)
	require.NotNil(task01)
	assert.Equal("task", task01.Id)
	assert.Equal(1, task01.Execution)
	assert.Len(task01.LocalTestResults, 1)
}

func TestGetTestResultsForDisplayTask(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, testresult.Collection))
	dt := task.Task{
		Id:             "dt",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et"},
	}
	assert.NoError(dt.Insert())
	test := testresult.TestResult{
		TaskID:   "et",
		TestFile: "myTest",
	}
	assert.NoError(test.Insert())
	results, err := dt.GetTestResultsForDisplayTask()
	assert.NoError(err)
	assert.Len(results, 1)
	assert.Equal("myTest", results[0].TestFile)
}

func TestBlockedState(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	t1 := task.Task{
		Id: "t1",
		DependsOn: []task.Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t1.Insert())
	t2 := task.Task{
		Id:     "t2",
		Status: evergreen.TaskFailed,
		DependsOn: []task.Dependency{
			{TaskId: "t3", Status: evergreen.TaskFailed},
		},
	}
	assert.NoError(t2.Insert())
	t3 := task.Task{
		Id:     "t3",
		Status: evergreen.TaskUnstarted,
		DependsOn: []task.Dependency{
			{TaskId: "t4", Status: task.AllStatuses},
		},
	}
	assert.NoError(t3.Insert())
	t4 := task.Task{
		Id:     "t4",
		Status: evergreen.TaskSucceeded,
	}
	assert.NoError(t4.Insert())

	state, err := t4.BlockedState()
	assert.NoError(err)
	assert.Equal("", state)
	state, err = t3.BlockedState()
	assert.NoError(err)
	assert.Equal("", state)
	state, err = t2.BlockedState()
	assert.NoError(err)
	assert.Equal("pending", state)
	state, err = t1.BlockedState()
	assert.NoError(err)
	assert.Equal("blocked", state)
}

func TestBulkInsert(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	t1 := task.Task{
		Id:      "t1",
		Version: "version",
	}
	t2 := task.Task{
		Id:      "t2",
		Version: "version",
	}
	t3 := task.Task{
		Id:      "t3",
		Version: "version",
	}
	tasks := task.Tasks{t1, t2, t3}
	assert.NoError(tasks.InsertUnordered())
	dbTasks, err := task.Find(task.ByVersion("version"))
	assert.NoError(err)
	assert.Len(dbTasks, 3)
	for _, dbTask := range dbTasks {
		assert.Equal("version", dbTask.Version)
	}
}
