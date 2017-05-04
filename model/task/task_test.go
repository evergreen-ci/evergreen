package task

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
)

var (
	conf  = testutil.TestConfig()
	oneMs = time.Millisecond
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(conf))
	grip.CatchError(grip.SetSender(testutil.SetupTestSender("")))
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
		testutil.HandleTestingErr(UpdateOne(
			bson.M{"_id": depTaskId.TaskId},
			bson.M{"$set": bson.M{"status": evergreen.TaskSucceeded}},
		), t, "Error setting task status")
	}
	// cases for * and failure
	for _, depTaskId := range depTaskIds[3:] {
		testutil.HandleTestingErr(UpdateOne(
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
			{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[1].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[2].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[3].TaskId, Status: evergreen.TaskUndispatched},
			{Id: depTaskIds[4].TaskId, Status: evergreen.TaskUndispatched},
		}

		So(db.Clear(Collection), ShouldBeNil)
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
				So(UpdateOne(
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
				So(UpdateOne(
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(UpdateOne(
					bson.M{"_id": depTaskIds[1].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				So(UpdateOne(
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

		So(db.Clear(Collection), ShouldBeNil)

		tasks := []Task{
			{Id: "t1", ScheduledTime: util.ZeroTime},
			{Id: "t2", ScheduledTime: util.ZeroTime},
			{Id: "t3", ScheduledTime: util.ZeroTime},
		}
		for _, task := range tasks {
			So(task.Insert(), ShouldBeNil)
		}
		Convey("when updating ScheduledTime for some of the tasks", func() {
			testTime := time.Unix(31337, 0)
			So(SetTasksScheduledTime(tasks[1:], testTime), ShouldBeNil)

			Convey("the tasks should be updated in memory", func() {
				So(tasks[0].ScheduledTime, ShouldResemble, util.ZeroTime)
				So(tasks[1].ScheduledTime, ShouldResemble, testTime)
				So(tasks[2].ScheduledTime, ShouldResemble, testTime)

				Convey("and in the db", func() {
					// Need to use a margin of error on time tests
					// because of minor differences between how mongo
					// and golang store dates. The date from the db
					// can be interpreted as being a few nanoseconds off
					t1, err := FindOne(ById("t1"))
					So(err, ShouldBeNil)
					So(t1.ScheduledTime.Round(oneMs), ShouldResemble, util.ZeroTime)
					t2, err := FindOne(ById("t2"))
					So(err, ShouldBeNil)
					So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
					t3, err := FindOne(ById("t3"))
					So(err, ShouldBeNil)
					So(t3.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
				})

				Convey("if we update a second time", func() {
					newTime := time.Unix(99999999, 0)
					So(newTime, ShouldHappenAfter, testTime)
					So(SetTasksScheduledTime(tasks, newTime), ShouldBeNil)

					Convey("only unset scheduled times should be updated", func() {
						t1, err := FindOne(ById("t1"))
						So(err, ShouldBeNil)
						So(t1.ScheduledTime.Round(oneMs), ShouldResemble, newTime)
						t2, err := FindOne(ById("t2"))
						So(err, ShouldBeNil)
						So(t2.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
						t3, err := FindOne(ById("t3"))
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

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

		tasks := []Task{
			{
				Id:        "one",
				DependsOn: []Dependency{{"two", ""}, {"three", ""}, {"four", ""}},
				Activated: true,
			},
			{
				Id:        "two",
				Priority:  5,
				Activated: true,
			},
			{
				Id:        "three",
				DependsOn: []Dependency{{"five", ""}},
				Activated: true,
			},
			{
				Id:        "four",
				DependsOn: []Dependency{{"five", ""}},
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

			So(tasks[0].SetPriority(1), ShouldBeNil)
			So(tasks[0].Priority, ShouldEqual, 1)

			task, err := FindOne(ById("one"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Priority, ShouldEqual, 1)

			task, err = FindOne(ById("two"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Priority, ShouldEqual, 5)

			task, err = FindOne(ById("three"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Priority, ShouldEqual, 1)

			task, err = FindOne(ById("four"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Id, ShouldEqual, "four")
			So(task.Priority, ShouldEqual, 1)

			task, err = FindOne(ById("five"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Id, ShouldEqual, "five")
			So(task.Priority, ShouldEqual, 1)

			task, err = FindOne(ById("six"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Id, ShouldEqual, "six")
			So(task.Priority, ShouldEqual, 0)

		})

		Convey("decreasing priority should update the task but not its dependencies", func() {

			So(tasks[0].SetPriority(1), ShouldBeNil)
			So(tasks[0].Activated, ShouldEqual, true)
			So(tasks[0].SetPriority(-1), ShouldBeNil)
			So(tasks[0].Priority, ShouldEqual, -1)

			task, err := FindOne(ById("one"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Priority, ShouldEqual, -1)
			So(task.Activated, ShouldEqual, false)

			task, err = FindOne(ById("two"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Priority, ShouldEqual, 5)
			So(task.Activated, ShouldEqual, true)

			task, err = FindOne(ById("three"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Priority, ShouldEqual, 1)
			So(task.Activated, ShouldEqual, true)

			task, err = FindOne(ById("four"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Id, ShouldEqual, "four")
			So(task.Priority, ShouldEqual, 1)
			So(task.Activated, ShouldEqual, true)

			task, err = FindOne(ById("five"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Id, ShouldEqual, "five")
			So(task.Priority, ShouldEqual, 1)
			So(task.Activated, ShouldEqual, true)

			task, err = FindOne(ById("six"))
			So(err, ShouldBeNil)
			So(task, ShouldNotBeNil)
			So(task.Id, ShouldEqual, "six")
			So(task.Priority, ShouldEqual, 0)
			So(task.Activated, ShouldEqual, true)
		})
	})

}

func TestFindTasksByIds(t *testing.T) {
	Convey("When calling FindTasksByIds...", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("only tasks with the specified ids should be returned", func() {

			tasks := []Task{
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

			dbTasks, err := Find(ByIds([]string{"one", "two"}))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}

func TestCountSimilarFailingTasks(t *testing.T) {
	Convey("When calling CountSimilarFailingTasks...", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("only failed tasks with the same project, requester, display "+
			"name and revision but different buildvariants should be returned",
			func() {
				project := "project"
				requester := "testing"
				displayName := "compile"
				buildVariant := "testVariant"
				revision := "asdf ;lkj asdf ;lkj "

				tasks := []Task{
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
		task     *Task
		myHost   *host.Host
		b        *build.Build
	)

	Convey("With a task", t, func() {

		taskId = "t1"
		hostId = "h1"
		buildId = "b1"
		distroId = "d1"

		task = &Task{
			Id:      taskId,
			BuildId: buildId,
		}

		myHost = &host.Host{
			Id:     hostId,
			Distro: distro.Distro{Id: distroId},
		}

		b = &build.Build{
			Id: buildId,
			Tasks: []build.TaskCache{
				{Id: taskId},
			},
		}

		testutil.HandleTestingErr(
			db.ClearCollections(Collection, build.Collection, host.Collection),
			t, "Error clearing test collections")

		So(task.Insert(), ShouldBeNil)
		So(myHost.Insert(), ShouldBeNil)
		So(b.Insert(), ShouldBeNil)

		Convey("when marking the task as dispatched, the fields for"+
			" the task, the host it is on, and the build it is a part of"+
			" should be set to reflect this", func() {

			// mark the task as dispatched
			So(task.MarkAsDispatched(myHost.Id, myHost.Distro.Id, time.Now()), ShouldBeNil)

			// make sure the task's fields were updated, both in ©memory and
			// in the db
			So(task.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(task.Status, ShouldEqual, evergreen.TaskDispatched)
			So(task.HostId, ShouldEqual, myHost.Id)
			So(task.LastHeartbeat, ShouldResemble, task.DispatchTime)
			task, err := FindOne(ById(taskId))
			So(err, ShouldBeNil)
			So(task.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(task.Status, ShouldEqual, evergreen.TaskDispatched)
			So(task.HostId, ShouldEqual, myHost.Id)
			So(task.LastHeartbeat, ShouldResemble, task.DispatchTime)

		})

	})

}

func TestTimeAggregations(t *testing.T) {
	Convey("With multiple tasks with different times", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
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
			DistroId:      "templOS"}
		So(task1.Insert(), ShouldBeNil)
		So(task2.Insert(), ShouldBeNil)
		So(task3.Insert(), ShouldBeNil)

		Convey("on an aggregation on FinishTime - StartTime", func() {
			timeMap, err := AverageTaskTimeDifference(
				StartTimeKey,
				FinishTimeKey,
				DistroIdKey,
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
			timeMap, err := AverageTaskTimeDifference(
				ScheduledTimeKey,
				StartTimeKey,
				DistroIdKey,
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
				timeMap, err := AverageTaskTimeDifference(
					IdKey,
					DistroIdKey,
					DistroIdKey,
					util.ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldBeNil)
				timeMap, err = AverageTaskTimeDifference(
					DistroIdKey,
					SecretKey,
					DistroIdKey,
					util.ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldBeNil)
			})

			Convey("special key cases should cause real agg errors", func() {
				timeMap, err := AverageTaskTimeDifference(
					StartTimeKey,
					"$$$$$$",
					DistroIdKey,
					util.ZeroTime)
				So(len(timeMap), ShouldEqual, 0)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestGetTestUrl(t *testing.T) {
	Convey("With a test result struct", t, func() {
		tr := &TestResult{
			URL: "testurl",
		}
		url := GetTestUrl(tr, "root")
		So(url, ShouldEqual, "testurl")
		anotherTr := &TestResult{
			LogId: "5",
		}
		url = GetTestUrl(anotherTr, "root")
		So(url, ShouldEqual, "root/test_log/5")
		emptyTr := &TestResult{}
		url = GetTestUrl(emptyTr, "root")
		So(url, ShouldBeEmpty)
	})
}

func TestEndingTask(t *testing.T) {
	Convey("With tasks that are attempting to be marked as finished", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("a task that has a start time set", func() {
			now := time.Now()
			t := &Task{
				Id:        "taskId",
				Status:    evergreen.TaskStarted,
				StartTime: now.Add(-5 * time.Minute),
			}
			So(t.Insert(), ShouldBeNil)
			details := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			So(t.MarkEnd(now, details), ShouldBeNil)
			t, err := FindOne(ById(t.Id))
			So(err, ShouldBeNil)
			So(t.Status, ShouldEqual, evergreen.TaskFailed)
			So(t.FinishTime.Unix(), ShouldEqual, now.Unix())
			So(t.StartTime.Unix(), ShouldEqual, now.Add(-5*time.Minute).Unix())
		})
		Convey("a task with no start time set should have one added", func() {
			now := time.Now()
			Convey("a task with a create time < 2 hours should have the start time set to the create time", func() {
				t := &Task{
					Id:         "tid",
					Status:     evergreen.TaskDispatched,
					CreateTime: now.Add(-30 * time.Minute),
				}
				So(t.Insert(), ShouldBeNil)
				details := &apimodels.TaskEndDetail{
					Status: evergreen.TaskFailed,
				}
				So(t.MarkEnd(now, details), ShouldBeNil)
				t, err := FindOne(ById(t.Id))
				So(err, ShouldBeNil)
				So(t.StartTime.Unix(), ShouldEqual, t.CreateTime.Unix())
				So(t.FinishTime.Unix(), ShouldEqual, now.Unix())
			})
			Convey("a task with a create time > 2 hours should have the start time set to two hours"+
				"before the finish time", func() {
				t := &Task{
					Id:         "tid",
					Status:     evergreen.TaskDispatched,
					CreateTime: now.Add(-3 * time.Hour),
				}
				So(t.Insert(), ShouldBeNil)
				details := &apimodels.TaskEndDetail{
					Status: evergreen.TaskFailed,
				}
				So(t.MarkEnd(now, details), ShouldBeNil)
				t, err := FindOne(ById(t.Id))
				So(err, ShouldBeNil)
				startTime := now.Add(-2 * time.Hour)
				So(t.StartTime.Unix(), ShouldEqual, startTime.Unix())
				So(t.FinishTime.Unix(), ShouldEqual, now.Unix())
			})

		})

	})
}
