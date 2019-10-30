package task

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mgobson "gopkg.in/mgo.v2/bson"
)

var (
	conf  = testutil.TestConfig()
	oneMs = time.Millisecond
)

var depTaskIds = []Dependency{
	{TaskId: "td1", Status: evergreen.TaskSucceeded},
	{TaskId: "td2", Status: evergreen.TaskSucceeded},
	{TaskId: "td3", Status: ""}, // Default == "success"
	{TaskId: "td4", Status: evergreen.TaskFailed},
	{TaskId: "td5", Status: AllStatuses},
}

// update statuses of test tasks in the db
func updateTestDepTasks(t *testing.T) {
	// cases for success/default
	for _, depTaskId := range depTaskIds[:3] {
		require.NoError(t, UpdateOne(bson.M{"_id": depTaskId.TaskId}, bson.M{"$set": bson.M{"status": evergreen.TaskSucceeded}}), "Error setting task status")
	}
	// cases for * and failure
	for _, depTaskId := range depTaskIds[3:] {
		require.NoError(t, UpdateOne(bson.M{"_id": depTaskId.TaskId}, bson.M{"$set": bson.M{"status": evergreen.TaskFailed}}), "Error setting task status")
	}
}

func TestDependenciesMet(t *testing.T) {

	var taskId string
	var taskDoc *Task
	var depTasks []*Task

	Convey("With a task", t, func() {

		taskId = "t1"

		taskDoc = &Task{
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

		Convey("sanity check the local version of the function in the nil case", func() {
			taskDoc.DependsOn = []Dependency{}
			met, err := taskDoc.AllDependenciesSatisfied(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("if the task has no dependencies its dependencies should"+
			" be met by default", func() {
			taskDoc.DependsOn = []Dependency{}
			met, err := taskDoc.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("task with overridden dependencies should be met", func() {
			taskDoc.DependsOn = depTaskIds
			taskDoc.OverrideDependencies = true
			met, err := taskDoc.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("if only some of the tasks dependencies are finished"+
			" successfully, then it should not think its dependencies are met",
			func() {
				taskDoc.DependsOn = depTaskIds
				So(UpdateOne(
					bson.M{"_id": depTaskIds[0].TaskId},
					bson.M{
						"$set": bson.M{
							"status": evergreen.TaskSucceeded,
						},
					},
				), ShouldBeNil)
				met, err := taskDoc.DependenciesMet(map[string]Task{})
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)
			})

		Convey("if all of the tasks dependencies are finished properly, it"+
			" should correctly believe its dependencies are met", func() {
			taskDoc.DependsOn = depTaskIds
			updateTestDepTasks(t)
			met, err := taskDoc.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
		})

		Convey("tasks not in the dependency cache should be pulled into the"+
			" cache during dependency checking", func() {
			dependencyCache := make(map[string]Task)
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
			dependencyCache := make(map[string]Task)
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
			dependencyCache := make(map[string]Task)
			taskDoc.DependsOn = depTaskIds
			met, err := taskDoc.AllDependenciesSatisfied(dependencyCache)
			So(err, ShouldNotBeNil)
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
				taskDoc.DependsOn = []Dependency{depTaskIds[0], depTaskIds[1],
					depTaskIds[2]}
				met, err := taskDoc.DependenciesMet(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)

				met, err = taskDoc.AllDependenciesSatisfied(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)

				// remove the failed task from the dependencies (but not from
				// the cache).  it should be ignored in the next pass
				taskDoc.DependsOn = []Dependency{depTaskIds[0], depTaskIds[1]}
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

		So(db.Clear(Collection), ShouldBeNil)

		tasks := []Task{
			{Id: "t0", ScheduledTime: util.ZeroTime, ExecutionTasks: []string{"t1", "t2"}},
			{Id: "t1", ScheduledTime: util.ZeroTime},
			{Id: "t2", ScheduledTime: util.ZeroTime},
			{Id: "t3", ScheduledTime: util.ZeroTime},
		}
		for _, task := range tasks {
			So(task.Insert(), ShouldBeNil)
		}
		Convey("when updating ScheduledTime for some of the tasks", func() {
			testTime := time.Unix(31337, 0)
			So(SetTasksScheduledTime(tasks[2:], testTime), ShouldBeNil)

			Convey("the tasks should be updated in memory", func() {
				So(tasks[1].ScheduledTime, ShouldResemble, util.ZeroTime)
				So(tasks[2].ScheduledTime, ShouldResemble, testTime)
				So(tasks[3].ScheduledTime, ShouldResemble, testTime)

				Convey("and in the db", func() {
					// Need to use a margin of error on time tests
					// because of minor differences between how mongo
					// and golang store dates. The date from the db
					// can be interpreted as being a few nanoseconds off
					t0, err := FindOne(ById("t0"))
					So(err, ShouldBeNil)
					So(t0.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
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
						t0, err := FindOne(ById("t0"))
						So(err, ShouldBeNil)
						So(t0.ScheduledTime.Round(oneMs), ShouldResemble, testTime)
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

		require.NoError(t, db.Clear(Collection), "Error clearing"+
			" '%v' collection", Collection)

		tasks := []Task{
			{
				Id:        "one",
				DependsOn: []Dependency{{TaskId: "two", Status: ""}, {TaskId: "three", Status: ""}, {TaskId: "four", Status: ""}},
				Activated: true,
			},
			{
				Id:        "two",
				Priority:  5,
				Activated: true,
			},
			{
				Id:        "three",
				DependsOn: []Dependency{{TaskId: "five", Status: ""}},
				Activated: true,
			},
			{
				Id:        "four",
				DependsOn: []Dependency{{TaskId: "five", Status: ""}},
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

			t, err := FindOne(ById("one"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)

			t, err = FindOne(ById("two"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 5)

			t, err = FindOne(ById("three"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)

			t, err = FindOne(ById("four"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "four")
			So(t.Priority, ShouldEqual, 1)

			t, err = FindOne(ById("five"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "five")
			So(t.Priority, ShouldEqual, 1)

			t, err = FindOne(ById("six"))
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

			t, err := FindOne(ById("one"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, -1)
			So(t.Activated, ShouldEqual, false)

			t, err = FindOne(ById("two"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 5)
			So(t.Activated, ShouldEqual, true)

			t, err = FindOne(ById("three"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = FindOne(ById("four"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "four")
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = FindOne(ById("five"))
			So(err, ShouldBeNil)
			So(t, ShouldNotBeNil)
			So(t.Id, ShouldEqual, "five")
			So(t.Priority, ShouldEqual, 1)
			So(t.Activated, ShouldEqual, true)

			t, err = FindOne(ById("six"))
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
			Convey("if no logs are present, it should not be nil", func() {
				So(t.Logs, ShouldBeNil)
			})
		})
		Convey("a task with no start time set should have one added", func() {
			now := time.Now()
			Convey("a task with a create time < 2 hours should have the start time set to the ingest time", func() {
				t := &Task{
					Id:         "tid",
					Status:     evergreen.TaskDispatched,
					IngestTime: now.Add(-30 * time.Minute),
				}
				So(t.Insert(), ShouldBeNil)
				details := &apimodels.TaskEndDetail{
					Status: evergreen.TaskFailed,
				}
				So(t.MarkEnd(now, details), ShouldBeNil)
				t, err := FindOne(ById(t.Id))
				So(err, ShouldBeNil)
				So(t.StartTime.Unix(), ShouldEqual, t.IngestTime.Unix())
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

func TestTaskResultOutcome(t *testing.T) {
	assert := assert.New(t)

	tasks := []Task{
		{Status: evergreen.TaskUndispatched, Activated: false}, // 0
		{Status: evergreen.TaskUndispatched, Activated: true},  // 1
		{Status: evergreen.TaskStarted},                        // 2
		{Status: evergreen.TaskSucceeded},                      // 3
		{Status: evergreen.TaskFailed},                         // 4
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem}},                                                                  // 5
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem, TimedOut: true}},                                                  // 6
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem, TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat}}, // 7
		{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat}},                                    // 8
		{Status: evergreen.TaskSetupFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSetup}},                                                              // 5
	}

	out := GetResultCounts(tasks)
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

	assert.Equal(1, GetResultCounts([]Task{tasks[0]}).Inactive)
	assert.Equal(1, GetResultCounts([]Task{tasks[1]}).Unstarted)
	assert.Equal(1, GetResultCounts([]Task{tasks[2]}).Started)
	assert.Equal(1, GetResultCounts([]Task{tasks[3]}).Succeeded)
	assert.Equal(1, GetResultCounts([]Task{tasks[4]}).Failed)
	assert.Equal(1, GetResultCounts([]Task{tasks[5]}).SystemFailed)
	assert.Equal(1, GetResultCounts([]Task{tasks[6]}).SystemTimedOut)
	assert.Equal(1, GetResultCounts([]Task{tasks[7]}).SystemUnresponsive)
	assert.Equal(1, GetResultCounts([]Task{tasks[8]}).TestTimedOut)
	assert.Equal(1, GetResultCounts([]Task{tasks[9]}).SetupFailed)
}

func TestMergeTestResultsBulk(t *testing.T) {
	require.NoError(t, db.Clear(testresult.Collection), "error clearing collections")
	assert := assert.New(t)

	tasks := []Task{
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

	out, err := MergeTestResultsBulk(tasks, nil)
	assert.NoError(err)
	count := 0
	for _, t := range out {
		count += len(t.LocalTestResults)
	}
	assert.Equal(4, count)

	query := db.Query(bson.M{
		testresult.StatusKey: evergreen.TestFailedStatus,
	})
	out, err = MergeTestResultsBulk(tasks, &query)
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

func TestTaskSetResultsFields(t *testing.T) {
        taskID := "jstestfuzz_self_tests_replication_fuzzers_master_initial_sync_fuzzer_69e2630b3272211f46bf85dd2577cd9a34c7c2cc_19_09_25_17_40_35"
        project := "jstestfuzz-self-tests"
        distroID := "amazon2-test"
        buildVariant := "replication_fuzzers"
        displayName := "fuzzer"
        requester := "gitter_request"

        displayTaskID := "jstestfuzz_self_tests_replication_fuzzers_display_master_69e2630b3272211f46bf85dd2577cd9a34c7c2cc_19_09_25_17_40_35"
        executionDisplayName := "master"
        StartTime := 1569431862.508
        EndTime := 1569431887.2

        TestStartTime := util.FromPythonTime(StartTime).In(time.UTC)
        TestEndTime := util.FromPythonTime(EndTime).In(time.UTC)

        testresults := []TestResult{
                {
                        Status:    "pass",
                        TestFile:  "job0_fixture_setup",
                        URL:       "https://logkeeper.mongodb.org/build/dd239a5697eedef049a753c6a40a3e7e/test/5d8ba136c2ab68304e1d741c",
                        URLRaw:    "https://logkeeper.mongodb.org/build/dd239a5697eedef049a753c6a40a3e7e/test/5d8ba136c2ab68304e1d741c?raw=1",
                        ExitCode:  0,
                        StartTime: StartTime,
                        EndTime:   EndTime,
                },
        }

        Convey("SetResults", t, func() {
                So(db.Clear(Collection), ShouldBeNil)
                So(db.Clear(testresult.Collection), ShouldBeNil)

                taskCreateTime, err := time.Parse(time.RFC3339, "2019-09-25T17:40:35Z")
                So(err, ShouldBeNil)

                task := Task{
                        Id:           taskID,
                        CreateTime:   taskCreateTime,
                        Project:      project,
                        DistroId:     distroID,
                        BuildVariant: buildVariant,
                        DisplayName:  displayName,
                        Execution:    0,
                        Requester:    requester,
                }

                executionDisplayTask := Task{
                        Id:             displayTaskID,
                        CreateTime:     taskCreateTime,
                        Project:        project,
                        DistroId:       distroID,
                        BuildVariant:   buildVariant,
                        DisplayName:    executionDisplayName,
                        Execution:      0,
                        Requester:      requester,
                        ExecutionTasks: []string{taskID},
                }

                So(task.Insert(), ShouldBeNil)
                Convey("Without a display task", func() {

                        So(task.SetResults(testresults), ShouldBeNil)

                        written, err := testresult.Find(testresult.ByTaskIDs([]string{taskID}))
                        So(err, ShouldBeNil)
                        So(1, ShouldEqual, len(written))
                        So(written[0].Project, ShouldEqual, project)
                        So(written[0].BuildVariant, ShouldEqual, buildVariant)
                        So(written[0].DistroId, ShouldEqual, distroID)
                        So(written[0].Requester, ShouldEqual, requester)
                        So(written[0].DisplayName, ShouldEqual, displayName)
                        So(written[0].ExecutionDisplayName, ShouldBeBlank)
                        So(written[0].TaskCreateTime.UTC(), ShouldResemble, taskCreateTime.UTC())
                        So(written[0].TestStartTime.UTC(), ShouldResemble, TestStartTime.UTC())
                        So(written[0].TestEndTime.UTC(), ShouldResemble, TestEndTime.UTC())
                })

                Convey("With a display task", func() {
                        So(executionDisplayTask.Insert(), ShouldBeNil)

                        So(task.SetResults(testresults), ShouldBeNil)

                        written, err := testresult.Find(testresult.ByTaskIDs([]string{taskID}))
                        So(err, ShouldBeNil)
                        So(1, ShouldEqual, len(written))
                        So(written[0].Project, ShouldEqual, project)
                        So(written[0].BuildVariant, ShouldEqual, buildVariant)
                        So(written[0].DistroId, ShouldEqual, distroID)
                        So(written[0].Requester, ShouldEqual, requester)
                        So(written[0].DisplayName, ShouldEqual, displayName)
                        So(written[0].ExecutionDisplayName, ShouldEqual, executionDisplayName)
                        So(written[0].TaskCreateTime.UTC(), ShouldResemble, taskCreateTime.UTC())
                        So(written[0].TestStartTime.UTC(), ShouldResemble, TestStartTime.UTC())
                        So(written[0].TestEndTime.UTC(), ShouldResemble, TestEndTime.UTC())
                })
        })
}

func TestFindOldTasksByID(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDoc := Task{
		Id: "task",
	}
	assert.NoError(taskDoc.Insert())
	assert.NoError(taskDoc.Archive())
	taskDoc.Execution += 1
	assert.NoError(taskDoc.Archive())
	taskDoc.Execution += 1

	tasks, err := FindOld(ByOldTaskID("task"))
	assert.NoError(err)
	assert.Len(tasks, 2)
	assert.Equal(0, tasks[0].Execution)
	assert.Equal("task_0", tasks[0].Id)
	assert.Equal("task", tasks[0].OldTaskId)
	assert.Equal(1, tasks[1].Execution)
	assert.Equal("task_1", tasks[1].Id)
	assert.Equal("task", tasks[1].OldTaskId)
}

func TestWithinTimePeriodProjectFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDocs := []Task{
		Task{
			Id:      "task1",
			Project: "proj",
		},
		Task{
			Id:      "task2",
			Project: "other",
		},
	}

	for _, taskDoc := range taskDocs {
		assert.NoError(taskDoc.Insert())
	}

	tasks, err := Find(WithinTimePeriod(time.Time{}, time.Time{}, "proj", []string{}))
	assert.NoError(err)
	assert.Len(tasks, 1)
	assert.Equal(tasks[0].Id, "task1")
}

func TestWithinTimePeriodDatesFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDocs := []Task{
		Task{
			Id:         "task1",
			FinishTime: time.Now().AddDate(0, 0, -2), // Should match
			StartTime:  time.Now().AddDate(0, 0, -5), // Shouldn't match
		},
		Task{
			Id:         "task2",
			FinishTime: time.Now().AddDate(0, 0, -2), // Should match
			StartTime:  time.Now().AddDate(0, 0, -3), // Should match
		},
		Task{
			Id:         "task3",
			FinishTime: time.Now(),                   // Shouldn't match
			StartTime:  time.Now().AddDate(0, 0, -3), // Should match
		},
	}

	for _, taskDoc := range taskDocs {
		assert.NoError(taskDoc.Insert())
	}

	tasks, err := Find(WithinTimePeriod(
		time.Now().AddDate(0, 0, -4), time.Now().AddDate(0, 0, -1), "", []string{}))
	assert.NoError(err)
	assert.Len(tasks, 1)
	assert.Equal(tasks[0].Id, "task2")
}

func TestWithinTimePeriodStatusesFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDocs := []Task{
		Task{
			Id:     "task1",
			Status: "A",
		},
		Task{
			Id:     "task2",
			Status: "B",
		},
		Task{
			Id:     "task3",
			Status: "C",
		},
	}

	for _, taskDoc := range taskDocs {
		assert.NoError(taskDoc.Insert())
	}

	statuses := []string{"A", "B"}

	tasks, err := Find(WithinTimePeriod(time.Time{}, time.Time{}, "", statuses))
	assert.NoError(err)
	assert.Len(tasks, 2)
	assert.Subset([]string{tasks[0].Status, tasks[1].Status}, statuses)
}

func TestTaskStatusCount(t *testing.T) {
	assert := assert.New(t)
	counts := TaskStatusCount{}
	details := apimodels.TaskEndDetail{
		TimedOut:    true,
		Description: evergreen.TaskDescriptionHeartbeat,
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

	require.NoError(db.ClearCollections(Collection, OldCollection))

	taskDoc := Task{
		Id: "task",
	}
	require.NoError(taskDoc.Insert())
	require.NoError(taskDoc.Archive())
	result0 := testresult.TestResult{
		ID:        mgobson.NewObjectId(),
		TaskID:    "task",
		Execution: 0,
	}
	result1 := testresult.TestResult{
		ID:        mgobson.NewObjectId(),
		TaskID:    "task",
		Execution: 1,
	}
	require.NoError(result0.Insert())
	require.NoError(result1.Insert())

	task00, err := FindOneIdOldOrNew("task", 0)
	assert.NoError(err)
	require.NotNil(task00)
	assert.Equal("task_0", task00.Id)
	assert.Equal(0, task00.Execution)
	assert.Len(task00.LocalTestResults, 1)

	task01, err := FindOneIdOldOrNew("task", 1)
	assert.NoError(err)
	require.NotNil(task01)
	assert.Equal("task", task01.Id)
	assert.Equal(1, task01.Execution)
	assert.Len(task01.LocalTestResults, 1)
}

func TestGetTestResultsForDisplayTask(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, testresult.Collection))
	dt := Task{
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

func TestBlocked(t *testing.T) {
	for name, test := range map[string]func(*testing.T){
		"blocked": func(*testing.T) {
			t1 := Task{
				Id: "t1",
				DependsOn: []Dependency{
					{Unattainable: false},
					{Unattainable: false},
					{Unattainable: true},
				},
			}
			assert.True(t, t1.Blocked())
		},
		"not blocked": func(*testing.T) {
			t1 := Task{
				Id: "t1",
				DependsOn: []Dependency{
					{Unattainable: false},
					{Unattainable: false},
					{Unattainable: false},
				},
			}
			assert.False(t, t1.Blocked())
		},
		"blocked state cached": func(*testing.T) {
			t1 := Task{
				Id: "t1",
				DependsOn: []Dependency{
					{TaskId: "t2", Status: evergreen.TaskSucceeded, Unattainable: true},
				},
			}
			state, err := t1.BlockedState()
			assert.NoError(t, err)
			assert.Equal(t, evergreen.TaskStatusBlocked, state)
		},
		"blocked state pending": func(*testing.T) {
			t1 := Task{
				Id: "t1",
				DependsOn: []Dependency{
					{TaskId: "t2", Status: evergreen.TaskSucceeded},
				},
			}
			t2 := Task{
				Id:     "t2",
				Status: evergreen.TaskDispatched,
			}
			require.NoError(t, t2.Insert())

			state, err := t1.BlockedState()
			assert.NoError(t, err)
			assert.Equal(t, evergreen.TaskStatusPending, state)
		},
		"blocked state all statuses": func(*testing.T) {
			t1 := Task{
				Id: "t1",
				DependsOn: []Dependency{
					{TaskId: "t2", Status: AllStatuses},
				},
			}
			t2 := Task{
				Id:     "t2",
				Status: evergreen.TaskUndispatched,
				DependsOn: []Dependency{
					{TaskId: "t3", Unattainable: true},
				},
			}
			require.NoError(t, t2.Insert())

			state, err := t1.BlockedState()
			assert.NoError(t, err)
			assert.Equal(t, "", state)
		},
	} {
		assert.NoError(t, db.ClearCollections(Collection))
		t.Run(name, test)
	}
}

func TestCircularDependency(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	t1 := Task{
		Id:          "t1",
		DisplayName: "t1",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t1.Insert())
	t2 := Task{
		Id:          "t2",
		DisplayName: "t2",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []Dependency{
			{TaskId: "t1", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t2.Insert())
	assert.NotPanics(func() {
		err := t1.CircularDependencies()
		assert.Contains(err.Error(), "Dependency cycle detected")
	})
}

func TestSiblingDependency(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	t1 := Task{
		Id:          "t1",
		DisplayName: "t1",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
			{TaskId: "t3", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t1.Insert())
	t2 := Task{
		Id:          "t2",
		DisplayName: "t2",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []Dependency{
			{TaskId: "t4", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t2.Insert())
	t3 := Task{
		Id:          "t3",
		DisplayName: "t3",
		Activated:   true,
		Status:      evergreen.TaskStarted,
		DependsOn: []Dependency{
			{TaskId: "t4", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t3.Insert())
	t4 := Task{
		Id:          "t4",
		DisplayName: "t4",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
	}
	assert.NoError(t4.Insert())
	state, err := t1.BlockedState()
	assert.NoError(err)
	assert.Equal(evergreen.TaskStatusPending, state)
}

func TestBulkInsert(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	t1 := Task{
		Id:      "t1",
		Version: "version",
	}
	t2 := Task{
		Id:      "t2",
		Version: "version",
	}
	t3 := Task{
		Id:      "t3",
		Version: "version",
	}
	tasks := Tasks{&t1, &t2, &t3}
	assert.NoError(tasks.InsertUnordered(context.Background()))
	dbTasks, err := Find(ByVersion("version"))
	assert.NoError(err)
	assert.Len(dbTasks, 3)
	for _, dbTask := range dbTasks {
		assert.Equal("version", dbTask.Version)
	}
}

func TestUnscheduleStaleUnderwaterTasks(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	t1 := Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
	}
	assert.NoError(t1.Insert())

	_, err := UnscheduleStaleUnderwaterTasks("")
	assert.NoError(err)
	dbTask, err := FindOneId("t1")
	assert.NoError(err)
	assert.False(dbTask.Activated)
	assert.EqualValues(-1, dbTask.Priority)
}

func TestGetRecentTaskStats(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	tasks := []Task{
		Task{Id: "t1", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		Task{Id: "t2", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		Task{Id: "t3", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		Task{Id: "t4", Status: evergreen.TaskSucceeded, DistroId: "d2", FinishTime: time.Now()},
		Task{Id: "t5", Status: evergreen.TaskFailed, DistroId: "d1", FinishTime: time.Now()},
		Task{Id: "t6", Status: evergreen.TaskFailed, DistroId: "d1", FinishTime: time.Now()},
		Task{Id: "t7", Status: evergreen.TaskFailed, DistroId: "d2", FinishTime: time.Now()},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert())
	}

	list, err := GetRecentTaskStats(time.Minute, DistroIdKey)
	assert.NoError(err)

	// Two statuses
	assert.Len(list, 2)
	// Two distros to report status for
	assert.Len(list[0].Stats, 2)

	for _, status := range list {
		if status.Status == evergreen.TaskSucceeded {
			// Sorted order
			assert.Equal("d1", status.Stats[0].Name)
			assert.Equal(3, status.Stats[0].Count)
			assert.Equal("d2", status.Stats[1].Name)
			assert.Equal(1, status.Stats[1].Count)
		}
		if status.Status == evergreen.TaskFailed {
			// Sorted order
			assert.Equal("d1", status.Stats[0].Name)
			assert.Equal(2, status.Stats[0].Count)
			assert.Equal("d2", status.Stats[1].Name)
			assert.Equal(1, status.Stats[1].Count)
		}
	}
}

func TestGetResultCountList(t *testing.T) {
	assert := assert.New(t)
	statsList := []StatusItem{
		{Status: evergreen.TaskSucceeded, Stats: []Stat{{Name: "d1", Count: 2}, {Name: "d2", Count: 1}}},
		{Status: evergreen.TaskFailed, Stats: []Stat{{Name: "d1", Count: 3}, {Name: "d2", Count: 2}}},
	}

	list := GetResultCountList(statsList)
	_, ok := list[evergreen.TaskSucceeded]
	assert.True(ok)
	_, ok = list[evergreen.TaskFailed]
	assert.True(ok)

	assert.Len(list["totals"], 2)
	assert.Equal("d1", list["totals"][0].Name)
	assert.Equal(5, list["totals"][0].Count)
	assert.Equal("d2", list["totals"][1].Name)
	assert.Equal(3, list["totals"][1].Count)
}

func TestFindVariantsWithTask(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(Collection))
	tasks := Tasks{
		&Task{Id: "1", DisplayName: "match", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 15, BuildVariant: "bv1"},
		&Task{Id: "2", DisplayName: "match", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 12, BuildVariant: "bv2"},
		&Task{Id: "3", DisplayName: "nomatch", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 14, BuildVariant: "bv1"},
		&Task{Id: "4", DisplayName: "match", Project: "p", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 50, BuildVariant: "bv1"},
	}
	assert.NoError(tasks.Insert())

	bvs, err := FindVariantsWithTask("match", "p", 10, 20)
	assert.NoError(err)
	require.Len(t, bvs, 2)
	assert.Equal(bvs[0], "bv2")
	assert.Equal(bvs[1], "bv1")
}

func TestUpdateBlockedDependencies(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	tasks := []Task{
		{
			Id:     "t0",
			Status: evergreen.TaskFailed,
		},
		{
			Id: "t1",
			DependsOn: []Dependency{
				{
					TaskId: "t0",
					Status: evergreen.TaskSucceeded,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id: "t2",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskSucceeded,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id: "t3",
			DependsOn: []Dependency{
				{
					TaskId:       "t2",
					Status:       evergreen.TaskSucceeded,
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id: "t4",
			DependsOn: []Dependency{
				{
					TaskId: "t3",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
		{
			Id: "t5",
			DependsOn: []Dependency{
				{
					TaskId: "t0",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert())
	}

	assert.NoError(tasks[0].UpdateBlockedDependencies())
	dbTask1, err := FindOneId(tasks[1].Id)
	assert.NoError(err)
	assert.True(dbTask1.DependsOn[0].Unattainable)
	dbTask2, err := FindOneId(tasks[2].Id)
	assert.NoError(err)
	assert.True(dbTask2.DependsOn[0].Unattainable)
	dbTask3, err := FindOneId(tasks[3].Id)
	assert.NoError(err)
	assert.True(dbTask3.DependsOn[0].Unattainable)
	// We don't traverse past t3 which was already unattainable == true
	dbTask4, err := FindOneId(tasks[4].Id)
	assert.NoError(err)
	assert.False(dbTask4.DependsOn[0].Unattainable)

	// update more than one dependency (t1 and t5)
	dbTask5, err := FindOneId(tasks[5].Id)
	assert.NoError(err)
	assert.True(dbTask5.DependsOn[0].Unattainable)
}

func TestUpdateUnblockedDependencies(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	tasks := []Task{
		{Id: "t0"},
		{
			Id: "t1",
			DependsOn: []Dependency{
				{
					TaskId:       "t0",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id: "t2",
			DependsOn: []Dependency{
				{
					TaskId:       "t0",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id: "t3",
			DependsOn: []Dependency{
				{
					TaskId:       "t2",
					Unattainable: false,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
		{
			Id: "t4",
			DependsOn: []Dependency{
				{
					TaskId:       "t3",
					Unattainable: true,
				},
			},
			Status: evergreen.TaskUndispatched,
		},
	}

	for _, task := range tasks {
		assert.NoError(task.Insert())
	}

	assert.NoError(tasks[0].UpdateUnblockedDependencies())
	dbTask1, err := FindOneId(tasks[1].Id)
	assert.NoError(err)
	assert.False(dbTask1.DependsOn[0].Unattainable)
	dbTask2, err := FindOneId(tasks[2].Id)
	assert.NoError(err)
	assert.False(dbTask2.DependsOn[0].Unattainable)
	dbTask3, err := FindOneId(tasks[3].Id)
	assert.NoError(err)
	assert.False(dbTask3.DependsOn[0].Unattainable)
	// We don't traverse past the t3 which was already unattainable == false
	dbTask4, err := FindOneId(tasks[4].Id)
	assert.NoError(err)
	assert.True(dbTask4.DependsOn[0].Unattainable)
}

func TestFindAllUnmarkedBlockedDependencies(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	t1 := &Task{
		Id:     "t1",
		Status: evergreen.TaskFailed,
	}

	tasks := []Task{
		{
			Id: "t2",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
		{
			Id: "t3",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskFailed,
				},
			},
		},
		{
			Id: "t4",
			DependsOn: []Dependency{
				{
					TaskId:       "t1",
					Status:       evergreen.TaskSucceeded,
					Unattainable: true,
				},
			},
		},
		{
			Id: "t5",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
					Status: evergreen.TaskFailed,
				},
				{
					TaskId: "t2",
					Status: evergreen.TaskSucceeded,
				},
			},
		},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert())
	}

	deps, err := t1.findAllUnmarkedBlockedDependencies()
	assert.NoError(err)
	assert.Len(deps, 1)
}

func TestFindAllMarkedUnattainableDependencies(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	t1 := &Task{Id: "t1"}
	tasks := []Task{
		{
			Id: "t2",
			DependsOn: []Dependency{
				{
					TaskId:       "t1",
					Unattainable: true,
				},
			},
		},
		{
			Id: "t3",
			DependsOn: []Dependency{
				{
					TaskId: "t1",
				},
				{
					TaskId:       "t2",
					Unattainable: true,
				},
			},
		},
	}

	for _, task := range tasks {
		assert.NoError(task.Insert())
	}

	unattainableTasks, err := t1.findAllMarkedUnattainableDependencies()
	assert.NoError(err)
	assert.Len(unattainableTasks, 1)
}

func TestUnattainableScheduleableTasksQuery(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	tasks := []Task{
		{
			Id:        "t0",
			Activated: true,
			Status:    evergreen.TaskUndispatched,
			DependsOn: []Dependency{
				{
					TaskId:       "t10",
					Unattainable: false,
				},
				{
					TaskId:       "t11",
					Unattainable: true,
				},
			},
			Priority: 0,
		},
		{
			Id:        "t1",
			Activated: true,
			Status:    evergreen.TaskUndispatched,
			DependsOn: []Dependency{
				{
					TaskId:       "t10",
					Unattainable: false,
				},
				{
					TaskId:       "t11",
					Unattainable: false,
				},
			},
			Priority: 0,
		},
		{
			Id:        "t2",
			Activated: true,
			Status:    evergreen.TaskUndispatched,
			Priority:  0,
			DependsOn: []Dependency{
				{
					TaskId:       "t10",
					Unattainable: true,
				},
			},
			OverrideDependencies: true,
		},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert())
	}

	q := scheduleableTasksQuery()
	schedulableTasks, err := FindAll(db.Query(q))
	assert.NoError(err)
	assert.Len(schedulableTasks, 2)
}

func TestGetTimeSpent(t *testing.T) {
	assert := assert.New(t)
	referenceTime := time.Unix(1136239445, 0)
	tasks := []Task{
		{
			StartTime:  referenceTime,
			FinishTime: referenceTime.Add(time.Hour),
			TimeTaken:  time.Hour,
		},
		{
			StartTime:  referenceTime,
			FinishTime: referenceTime.Add(2 * time.Hour),
			TimeTaken:  2 * time.Hour,
		},
		{
			StartTime:  referenceTime,
			FinishTime: util.ZeroTime,
			TimeTaken:  0,
		},
		{
			StartTime:  util.ZeroTime,
			FinishTime: util.ZeroTime,
			TimeTaken:  0,
		},
	}

	timeTaken, makespan := GetTimeSpent(tasks)
	assert.Equal(3*time.Hour, timeTaken)
	assert.Equal(2*time.Hour, makespan)

	timeTaken, makespan = GetTimeSpent(tasks[2:])
	assert.EqualValues(0, timeTaken)
	assert.EqualValues(0, makespan)
}

func TestUpdateDependencies(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := &Task{
		Id:        "t1",
		DependsOn: []Dependency{{TaskId: "t2"}},
	}
	assert.NoError(t, t1.Insert())

	dependsOn := []Dependency{{TaskId: "t3"}}
	assert.NoError(t, t1.UpdateDependencies(dependsOn))
	assert.Len(t, t1.DependsOn, 2)
	dbT1, err := FindOneId("t1")
	assert.NoError(t, err)
	assert.Len(t, dbT1.DependsOn, 2)

	// If the task is out of sync with the db the update fails
	t1.DependsOn = []Dependency{{TaskId: "t4"}}
	assert.Error(t, t1.UpdateDependencies(dependsOn))
}

func TestDisplayTaskCache(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(Collection))
	const displayTaskCount = 50
	const execPerDisplay = 10

	for i := 0; i < displayTaskCount; i++ {
		dt := Task{
			Id:          fmt.Sprintf("d%d", i),
			DisplayOnly: true,
		}
		for j := 0; j < execPerDisplay; j++ {
			et := Task{
				Id: fmt.Sprintf("%d-%d", i, j),
			}
			assert.NoError(et.Insert())
			dt.ExecutionTasks = append(dt.ExecutionTasks, et.Id)
		}
		assert.NoError(dt.Insert())
	}

	cache := NewDisplayTaskCache()
	dt, err := cache.Get(&Task{Id: "1-1"})
	assert.NoError(err)
	assert.Equal("d1", dt.Id)
	dt, err = cache.Get(&Task{Id: "1-5"})
	assert.NoError(err)
	assert.Equal("d1", dt.Id)
	assert.Len(cache.displayTasks, 1)
	assert.Len(cache.execToDisplay, execPerDisplay)

	for i := 0; i < displayTaskCount; i++ {
		_, err = cache.Get(&Task{Id: fmt.Sprintf("%d-1", i)})
		assert.NoError(err)
	}
	assert.Len(cache.execToDisplay, displayTaskCount*execPerDisplay)
	assert.Len(cache.List(), displayTaskCount)
}

func TestFindMergeTaskForVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := &Task{
		Id:               "t1",
		Version:          "abcdef123456",
		CommitQueueMerge: false,
	}
	assert.NoError(t, t1.Insert())

	_, err := FindMergeTaskForVersion("abcdef123456")
	assert.Error(t, err)
	assert.True(t, adb.ResultsNotFound(err))

	t2 := &Task{
		Id:               "t2",
		Version:          "abcdef123456",
		CommitQueueMerge: true,
	}
	assert.NoError(t, t2.Insert())
	t2Db, err := FindMergeTaskForVersion("abcdef123456")
	assert.NoError(t, err)
	assert.Equal(t, "t2", t2Db.Id)
}

func TestMarkGeneratedTasks(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	t1 := &Task{
		Id: "t1",
	}
	require.NoError(t, t1.Insert())

	mockError := errors.New("mock error")

	require.NoError(t, MarkGeneratedTasks(t1.Id, nil))
	found, err := FindOneId(t1.Id)
	require.NoError(t, err)
	require.Equal(t, true, found.GeneratedTasks)
	require.Equal(t, "", found.GenerateTasksError)

	require.NoError(t, MarkGeneratedTasks(t1.Id, mockError))
	found, err = FindOneId(t1.Id)
	require.NoError(t, err)
	require.Equal(t, true, found.GeneratedTasks)
	require.Equal(t, "", found.GenerateTasksError, "calling after GeneratedTasks is set should not set an error")

	t2 := &Task{
		Id: "t2",
	}
	require.NoError(t, t2.Insert())
	require.NoError(t, MarkGeneratedTasks(t2.Id, mockError))
	found, err = FindOneId(t2.Id)
	require.NoError(t, err)
	require.Equal(t, true, found.GeneratedTasks)
	require.Equal(t, mockError.Error(), found.GenerateTasksError)

	t3 := &Task{
		Id: "t3",
	}
	require.NoError(t, t3.Insert())
	require.NoError(t, MarkGeneratedTasks(t3.Id, mongo.ErrNoDocuments))
	found, err = FindOneId(t3.Id)
	require.NoError(t, err)
	require.Equal(t, false, found.GeneratedTasks, "document not found should not set generated tasks, since this was a race and did not generate.tasks")
	require.Equal(t, "", found.GenerateTasksError)
}
