package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"testing"
	"time"
)

var (
	conf  = evergreen.TestConfig()
	oneMs = time.Millisecond
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(conf))
	evergreen.SetLogger("/tmp/task_test.log")
}

var depTaskIds = []Dependency{
	{"td1", evergreen.TaskSucceeded},
	{"td2", evergreen.TaskSucceeded},
	{"td3", ""}, // Default == "success"
	{"td4", evergreen.TaskFailed},
	{"td5", AllStatuses},
}

// update statuses of test tasks in the db
func updateTestDepTasks(t *testing.T) {
	// cases for success/default
	for _, depTaskId := range depTaskIds[:3] {
		testutil.HandleTestingErr(UpdateOneTask(
			bson.M{"_id": depTaskId.TaskId},
			bson.M{"$set": bson.M{"status": evergreen.TaskSucceeded}},
		), t, "Error setting task status")
	}
	// cases for * and failure
	for _, depTaskId := range depTaskIds[3:] {
		testutil.HandleTestingErr(UpdateOneTask(
			bson.M{"_id": depTaskId.TaskId},
			bson.M{"$set": bson.M{"status": evergreen.TaskFailed}},
		), t, "Error setting task status")
	}
}

func TestDependenciesMet(t *testing.T) {

	var taskId string
	var task *Task
	var depTasks []*Task

	Convey("With a task", t, func() {

		taskId = "t1"

		task = &Task{
			Id: taskId,
		}

		depTasks = []*Task{
			&Task{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched},
			&Task{Id: depTaskIds[1].TaskId, Status: evergreen.TaskUndispatched},
			&Task{Id: depTaskIds[2].TaskId, Status: evergreen.TaskUndispatched},
			&Task{Id: depTaskIds[3].TaskId, Status: evergreen.TaskUndispatched},
			&Task{Id: depTaskIds[4].TaskId, Status: evergreen.TaskUndispatched},
		}

		So(db.Clear(TasksCollection), ShouldBeNil)
		for _, depTask := range depTasks {
			So(depTask.Insert(), ShouldBeNil)
		}

		Convey("if the task has no dependencies its dependencies should"+
			" be met by default", func() {
			task.DependsOn = []Dependency{}
			met, err := task.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("if only some of the tasks dependencies are finished"+
			" successfully, then it should not think its dependencies are met",
			func() {
				task.DependsOn = depTaskIds
				So(UpdateOneTask(
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				met, err := task.DependenciesMet(map[string]Task{})
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)
			})

		Convey("if all of the tasks dependencies are finished properly, it"+
			" should correctly believe its dependencies are met", func() {
			task.DependsOn = depTaskIds
			updateTestDepTasks(t)
			met, err := task.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("tasks not in the dependency cache should be pulled into the"+
			" cache during dependency checking", func() {
			dependencyCache := make(map[string]Task)
			task.DependsOn = depTaskIds
			updateTestDepTasks(t)
			met, err := task.DependenciesMet(dependencyCache)
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
			dependencyCache := make(map[string]Task)
			task.DependsOn = depTaskIds
			met, err := task.DependenciesMet(dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)

			// alter the dependency cache so that it should seem as if the
			// dependencies are not met
			cachedTask := dependencyCache[depTaskIds[0].TaskId]
			So(cachedTask.Status, ShouldEqual, evergreen.TaskSucceeded)
			cachedTask.Status = evergreen.TaskFailed
			dependencyCache[depTaskIds[0].TaskId] = cachedTask
			met, err = task.DependenciesMet(dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeFalse)

		})

		Convey("extraneous tasks in the dependency cache should be ignored",
			func() {
				So(UpdateOneTask(
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(UpdateOneTask(
					bson.M{"_id": depTaskIds[1].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(UpdateOneTask(
					bson.M{"_id": depTaskIds[2].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskFailed,
						},
					},
				), ShouldBeNil)

				dependencyCache := make(map[string]Task)
				task.DependsOn = []Dependency{depTaskIds[0], depTaskIds[1],
					depTaskIds[2]}
				met, err := task.DependenciesMet(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)

				// remove the failed task from the dependencies (but not from
				// the cache).  it should be ignored in the next pass
				task.DependsOn = []Dependency{depTaskIds[0], depTaskIds[1]}
				met, err = task.DependenciesMet(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeTrue)
			})
	})
}

func TestSetTasksScheduledTime(t *testing.T) {
	Convey("With some tasks", t, func() {

		So(db.Clear(TasksCollection), ShouldBeNil)

		tasks := []Task{
			{Id: "t1", ScheduledTime: ZeroTime},
			{Id: "t2", ScheduledTime: ZeroTime},
			{Id: "t3", ScheduledTime: ZeroTime},
		}
		for _, task := range tasks {
			So(task.Insert(), ShouldBeNil)
		}
		Convey("when updating ScheduledTime for some of the tasks", func() {
			testTime := time.Unix(31337, 0)
			So(SetTasksScheduledTime(tasks[1:], testTime), ShouldBeNil)

			Convey("the tasks should be updated in memory", func() {
				So(tasks[0].ScheduledTime, ShouldResemble, ZeroTime)
				So(tasks[1].ScheduledTime, ShouldResemble, testTime)
				So(tasks[2].ScheduledTime, ShouldResemble, testTime)

				Convey("and in the db", func() {
					// Need to use a margin of error on time tests
					// because of minor differences between how mongo
					// and golang store dates. The date from the db
					// can be interpreted as being a few nanoseconds off
					t1, err := FindTask("t1")
					So(err, ShouldBeNil)
					So(t1.ScheduledTime.Round(oneMs), ShouldResemble, ZeroTime)
					t2, err := FindTask("t2")
					So(err, ShouldBeNil)
					So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
					t3, err := FindTask("t3")
					So(err, ShouldBeNil)
					So(t3.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
				})

				Convey("if we update a second time", func() {
					newTime := time.Unix(99999999, 0)
					So(newTime, ShouldHappenAfter, testTime)
					So(SetTasksScheduledTime(tasks, newTime), ShouldBeNil)

					Convey("only unset scheduled times should be updated", func() {
						t1, err := FindTask("t1")
						So(err, ShouldBeNil)
						So(t1.ScheduledTime.Round(oneMs), ShouldResemble, newTime)
						t2, err := FindTask("t2")
						So(err, ShouldBeNil)
						So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						t3, err := FindTask("t3")
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

		testutil.HandleTestingErr(db.Clear(TasksCollection), t, "Error clearing"+
			" '%v' collection", TasksCollection)

		task := &Task{
			Id: "task",
		}
		So(task.Insert(), ShouldBeNil)

		Convey("setting its priority should update it both in-memory"+
			" and in the database", func() {

			So(task.SetPriority(1), ShouldBeNil)
			So(task.Priority, ShouldEqual, 1)

			task, err := FindTask(task.Id)
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Priority, ShouldEqual, 1)
		})

	})

}

func TestFindTasksByIds(t *testing.T) {
	Convey("When calling FindTasksByIds...", t, func() {
		So(db.Clear(TasksCollection), ShouldBeNil)
		Convey("only tasks with the specified ids should be returned", func() {

			tasks := []Task{
				Task{
					Id: "one",
				},
				Task{
					Id: "two",
				},
				Task{
					Id: "three",
				},
			}

			for _, task := range tasks {
				So(task.Insert(), ShouldBeNil)
			}

			dbTasks, err := FindTasksByIds([]string{"one", "two"})
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}

func TestCountSimilarFailingTasks(t *testing.T) {
	Convey("When calling CountSimilarFailingTasks...", t, func() {
		So(db.Clear(TasksCollection), ShouldBeNil)
		Convey("only failed tasks with the same project, requester, display "+
			"name and revision but different buildvariants should be returned",
			func() {
				project := "project"
				requester := "testing"
				displayName := "compile"
				buildVariant := "testVariant"
				revision := "asdf ;lkj asdf ;lkj "

				tasks := []Task{
					Task{
						Id:           "one",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "1",
						Revision:     revision,
						Requester:    requester,
					},
					Task{
						Id:           "two",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// task succeeded so should not be returned
					Task{
						Id:           "three",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskSucceeded,
					},
					// same buildvariant so should not be returned
					Task{
						Id:           "four",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "1",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// different project so should not be returned
					Task{
						Id:           "five",
						Project:      project + "1",
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// different requester so should not be returned
					Task{
						Id:           "six",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "2",
						Revision:     revision,
						Requester:    requester + "1",
						Status:       evergreen.TaskFailed,
					},
					// different revision so should not be returned
					Task{
						Id:           "seven",
						Project:      project,
						DisplayName:  displayName,
						BuildVariant: buildVariant + "1",
						Revision:     revision + "1",
						Requester:    requester,
						Status:       evergreen.TaskFailed,
					},
					// different display name so should not be returned
					Task{
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

func TestSetTaskActivated(t *testing.T) {

	Convey("With a task and build", t, func() {

		testutil.HandleTestingErr(
			db.ClearCollections(TasksCollection, build.Collection, host.Collection),
			t, "Error clearing test collections")

		taskId := "t1"
		buildId := "b1"
		testTime := time.Now()

		task := &Task{
			Id:            taskId,
			ScheduledTime: testTime,
			BuildId:       buildId,
			DependsOn: []Dependency{
				{"t2", evergreen.TaskSucceeded},
				{"t3", evergreen.TaskSucceeded},
			},
		}

		b := &build.Build{
			Id: buildId,
			Tasks: []build.TaskCache{
				{Id: taskId}, {Id: "t2"}, {Id: "t3"},
			},
		}

		dep1 := &Task{
			Id:            "t2",
			ScheduledTime: testTime,
			BuildId:       buildId,
		}
		dep2 := &Task{
			Id:            "t3",
			ScheduledTime: testTime,
			BuildId:       buildId,
		}
		So(dep1.Insert(), ShouldBeNil)
		So(dep2.Insert(), ShouldBeNil)

		So(task.Insert(), ShouldBeNil)
		So(b.Insert(), ShouldBeNil)

		Convey("setting the test to active will update relevant db fields", func() {
			So(SetTaskActivated(taskId, "", true), ShouldBeNil)
			dbTask, err := FindTask(taskId)
			So(err, ShouldBeNil)
			So(dbTask.Activated, ShouldBeTrue)
			So(dbTask.ScheduledTime, ShouldHappenWithin, oneMs, testTime)

			// make sure the dependencies were activated
			dbDepOne, err := FindTask(dep1.Id)
			So(err, ShouldBeNil)
			So(dbDepOne.Activated, ShouldBeTrue)
			dbDepTwo, err := FindTask(dep2.Id)
			So(err, ShouldBeNil)
			So(dbDepTwo.Activated, ShouldBeTrue)

			Convey("and setting active to false will reset the relevant fields", func() {
				So(SetTaskActivated(taskId, "", false), ShouldBeNil)
				dbTask, err := FindTask(taskId)
				So(err, ShouldBeNil)
				So(dbTask.Activated, ShouldBeFalse)
				So(dbTask.ScheduledTime, ShouldHappenWithin, oneMs, ZeroTime)

			})
		})
	})
}

func TestMarkAsDispatched(t *testing.T) {

	var (
		taskId  string
		hostId  string
		buildId string
		task    *Task
		myHost  *host.Host
		b       *build.Build
	)

	Convey("With a task", t, func() {

		taskId = "t1"
		hostId = "h1"
		buildId = "b1"

		task = &Task{
			Id:      taskId,
			BuildId: buildId,
		}

		myHost = &host.Host{
			Id: hostId,
		}

		b = &build.Build{
			Id: buildId,
			Tasks: []build.TaskCache{
				{Id: taskId},
			},
		}

		testutil.HandleTestingErr(
			db.ClearCollections(TasksCollection, build.Collection, host.Collection),
			t, "Error clearing test collections")

		So(task.Insert(), ShouldBeNil)
		So(myHost.Insert(), ShouldBeNil)
		So(b.Insert(), ShouldBeNil)

		Convey("when marking the task as dispatched, the fields for"+
			" the task, the host it is on, and the build it is a part of"+
			" should be set to reflect this", func() {

			// mark the task as dispatched
			So(task.MarkAsDispatched(myHost, time.Now()), ShouldBeNil)

			// make sure the task's fields were updated, both in memory and
			// in the db
			So(task.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(task.Status, ShouldEqual, evergreen.TaskDispatched)
			So(task.HostId, ShouldEqual, myHost.Id)
			So(task.LastHeartbeat, ShouldResemble, task.DispatchTime)
			task, err := FindTask(taskId)
			So(err, ShouldBeNil)
			So(task.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(task.Status, ShouldEqual, evergreen.TaskDispatched)
			So(task.HostId, ShouldEqual, myHost.Id)
			So(task.LastHeartbeat, ShouldResemble, task.DispatchTime)

			// make sure the build's fields were updated in the db
			b, err = build.FindOne(build.ById(buildId))
			So(err, ShouldBeNil)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskDispatched)

		})

	})

}

func TestTimeAggregations(t *testing.T) {
	Convey("With multiple tasks with different times", t, func() {
		So(db.Clear(TasksCollection), ShouldBeNil)
		task1 := Task{Id: "bogus",
			ScheduledTime: time.Unix(1000, 0),
			StartTime:     time.Unix(1010, 0),
			FinishTime:    time.Unix(1030, 0),
			DistroId:      "osx"}
		task2 := Task{Id: "fake",
			ScheduledTime: time.Unix(1000, 0),
			StartTime:     time.Unix(1020, 0),
			FinishTime:    time.Unix(1050, 0),
			DistroId:      "osx"}
		task3 := Task{Id: "placeholder",
			ScheduledTime: time.Unix(1000, 0),
			StartTime:     time.Unix(1060, 0),
			FinishTime:    time.Unix(1180, 0),
			DistroId:      "templeos"}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(task3.Insert(), ShouldBeNil)

		Convey("on an aggregation on FinishTime - StartTime", func() {
			timeMap, err := AverageTaskTimeDifference(
				TaskStartTimeKey,
				TaskFinishTimeKey,
				TaskDistroIdKey,
				ZeroTime)
			So(err, ShouldBeNil)

			Convey("the proper averages should be computed", func() {
				// osx = ([1030-1010] + [1050-1020])/2 = (20+30)/2 = 25
				So(timeMap["osx"].Seconds(), ShouldEqual, 25)
				// templeos = (1180 - 1060)/1 = 120/1 = 120
				So(timeMap["templeos"].Seconds(), ShouldEqual, 120)
			})
		})

		Convey("on an aggregation on StartTime - ScheduledTime", func() {
			timeMap, err := AverageTaskTimeDifference(
				TaskScheduledTimeKey,
				TaskStartTimeKey,
				TaskDistroIdKey,
				ZeroTime)
			So(err, ShouldBeNil)

			Convey("the proper averages should be computed", func() {
				// osx = ([1010-1000] + [1020-1000])/2 = (10+20)/2 = 15
				So(timeMap["osx"].Seconds(), ShouldEqual, 15)
				// templeos = (1060-1000)/1 = 60/1 = 60
				So(timeMap["templeos"].Seconds(), ShouldEqual, 60)
			})
		})

		Convey("but when given non-time fields", func() {

			Convey("most cases should return an empty map", func() {
				timeMap, err := AverageTaskTimeDifference(
					TaskIdKey,
					TaskDistroIdKey,
					TaskDistroIdKey,
					ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldBeNil)
				timeMap, err = AverageTaskTimeDifference(
					TaskDistroIdKey,
					TaskSecretKey,
					TaskDistroIdKey,
					ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldBeNil)
			})

			Convey("special key cases should cause real agg errors", func() {
				timeMap, err := AverageTaskTimeDifference(
					TaskStartTimeKey,
					"$$$$$$",
					TaskDistroIdKey,
					ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestFindTasksForHostIds(t *testing.T) {
	Convey("Should return all tasks for hosts", t, func() {
		So(db.Clear(TasksCollection), ShouldBeNil)
		task1 := Task{
			Id:        "t1",
			HostId:    "h1",
			Status:    evergreen.TaskSucceeded,
			Requester: "r1",
		}
		task2 := Task{
			Id:        "t2",
			HostId:    "h2",
			Status:    evergreen.TaskFailed,
			Requester: "r1",
		}
		task3 := Task{
			Id:        "t3",
			HostId:    "h3",
			Status:    evergreen.TaskSucceeded,
			Requester: "r2",
		}
		task4 := Task{
			Id:        "t4",
			HostId:    "h1",
			Status:    evergreen.TaskDispatched,
			Requester: "r2",
		}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(task3.Insert(), ShouldBeNil)
		So(task4.Insert(), ShouldBeNil)

		result, err := FindTasksForHostIds([]string{"h1", "h2"})
		So(err, ShouldBeNil)
		So(len(result), ShouldEqual, 2)
		So(result[0].Id == "t1" || result[1].Id == "t1", ShouldBeTrue)
		So(result[0].Id == "t2" || result[1].Id == "t2", ShouldBeTrue)
		So(result[0].Requester, ShouldEqual, "r1")
		So(result[1].Requester, ShouldEqual, "r1")
	})
}

func TestGetStepback(t *testing.T) {
	Convey("When the project has a stepback policy set to true", t, func() {
		_true, _false := true, false
		project := &Project{
			Stepback: true,
			BuildVariants: []BuildVariant{
				BuildVariant{
					Name: "sbnil",
				},
				BuildVariant{
					Name:     "sbtrue",
					Stepback: &_true,
				},
				BuildVariant{
					Name:     "sbfalse",
					Stepback: &_false,
				},
			},
			Tasks: []ProjectTask{
				ProjectTask{Name: "nil"},
				ProjectTask{Name: "true", Stepback: &_true},
				ProjectTask{Name: "false", Stepback: &_false},
				ProjectTask{Name: "bvnil"},
				ProjectTask{Name: "bvtrue"},
				ProjectTask{Name: "bvfalse"},
			},
		}

		Convey("if the task does not override the setting", func() {
			task := &Task{DisplayName: "nil"}
			Convey("then the value should be true", func() {
				So(task.getStepback(project), ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with true", func() {
			task := &Task{DisplayName: "true"}
			Convey("then the value should be true", func() {
				So(task.getStepback(project), ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with false", func() {
			task := &Task{DisplayName: "false"}
			Convey("then the value should be false", func() {
				So(task.getStepback(project), ShouldBeFalse)
			})
		})

		Convey("if the buildvariant does not override the setting", func() {
			task := &Task{DisplayName: "bvnil", BuildVariant: "sbnil"}
			Convey("then the value should be true", func() {
				So(task.getStepback(project), ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with true", func() {
			task := &Task{DisplayName: "bvtrue", BuildVariant: "sbtrue"}
			Convey("then the value should be true", func() {
				So(task.getStepback(project), ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with false", func() {
			task := &Task{DisplayName: "bvfalse", BuildVariant: "sbfalse"}
			Convey("then the value should be false", func() {
				So(task.getStepback(project), ShouldBeFalse)
			})
		})

	})
}
