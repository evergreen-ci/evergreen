package task

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
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
		So(taskDoc.Insert(), ShouldBeNil)

		Convey("sanity check the local version of the function in the nil case", func() {
			taskDoc.DependsOn = []Dependency{}
			met, err := taskDoc.AllDependenciesSatisfied(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
		})

		Convey("if the task has no dependencies its dependencies should"+
			" be met by default", func() {
			taskDoc.DependsOn = []Dependency{}
			met, err := taskDoc.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
		})

		Convey("task with overridden dependencies should be met", func() {
			taskDoc.DependsOn = depTaskIds
			taskDoc.OverrideDependencies = true
			met, err := taskDoc.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
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
				taskDoc.DependenciesMetTime = utility.ZeroTime
			})

		Convey("if all of the tasks dependencies are finished properly, it"+
			" should correctly believe its dependencies are met", func() {
			taskDoc.DependsOn = depTaskIds
			updateTestDepTasks(t)
			met, err := taskDoc.DependenciesMet(map[string]Task{})
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
		})

		Convey("tasks not in the dependency cache should be pulled into the"+
			" cache during dependency checking", func() {
			dependencyCache := make(map[string]Task)
			taskDoc.DependsOn = depTaskIds
			updateTestDepTasks(t)
			met, err := taskDoc.DependenciesMet(dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeTrue)
			taskDoc.DependenciesMetTime = utility.ZeroTime
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
			taskDoc.DependenciesMetTime = utility.ZeroTime

			// alter the dependency cache so that it should seem as if the
			// dependencies are not met
			cachedTask := dependencyCache[depTaskIds[0].TaskId]
			So(cachedTask.Status, ShouldEqual, evergreen.TaskSucceeded)
			cachedTask.Status = evergreen.TaskFailed
			dependencyCache[depTaskIds[0].TaskId] = cachedTask
			met, err = taskDoc.DependenciesMet(dependencyCache)
			So(err, ShouldBeNil)
			So(met, ShouldBeFalse)
			taskDoc.DependenciesMetTime = utility.ZeroTime

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
				taskDoc.DependenciesMetTime = utility.ZeroTime

				met, err = taskDoc.AllDependenciesSatisfied(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeFalse)

				// remove the failed task from the dependencies (but not from
				// the cache).  it should be ignored in the next pass
				taskDoc.DependsOn = []Dependency{depTaskIds[0], depTaskIds[1]}
				met, err = taskDoc.DependenciesMet(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeTrue)
				taskDoc.DependenciesMetTime = utility.ZeroTime

				met, err = taskDoc.AllDependenciesSatisfied(dependencyCache)
				So(err, ShouldBeNil)
				So(met, ShouldBeTrue)
			})
	})
}

func TestRefreshBlockedDependencies(t *testing.T) {
	taskId := "t1"
	taskDoc := &Task{
		Id: taskId,
	}
	depTasks := []*Task{
		{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched},
		{Id: depTaskIds[1].TaskId, Status: evergreen.TaskUndispatched},
		{Id: depTaskIds[2].TaskId, Status: evergreen.TaskFailed},
		{Id: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
		{Id: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
		{Id: "td6", Status: evergreen.TaskDispatched, DependsOn: []Dependency{{TaskId: "DNE", Unattainable: true}}},
	}
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	for name, test := range map[string]func(*testing.T){
		"NoDeps": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{}
			require.NoError(t, taskDoc.Insert())

			tasks, err := taskDoc.RefreshBlockedDependencies(map[string]Task{})
			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"Satisfied": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert())

			tasks, err := taskDoc.RefreshBlockedDependencies(map[string]Task{})
			assert.NoError(t, err)
			assert.Empty(t, tasks)
		},
		"UnsatisfiedAndFinished": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[2].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert())

			tasks, err := taskDoc.RefreshBlockedDependencies(map[string]Task{})
			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		},
		"BlockedEarly": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded, Unattainable: true},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert())

			tasks, err := taskDoc.RefreshBlockedDependencies(map[string]Task{})
			assert.NoError(t, err)
			// already marked blocked
			assert.Len(t, tasks, 0)
		},
		"BlockedLater": func(t *testing.T) {
			taskDoc.DependsOn = []Dependency{
				{TaskId: depTaskIds[3].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: depTaskIds[4].TaskId, Status: evergreen.TaskSucceeded},
				{TaskId: "td6", Status: evergreen.TaskSucceeded},
			}
			require.NoError(t, taskDoc.Insert())

			tasks, err := taskDoc.RefreshBlockedDependencies(map[string]Task{})
			assert.NoError(t, err)
			assert.Len(t, tasks, 1)
		}} {
		require.NoError(t, db.Clear(Collection))
		for _, depTask := range depTasks {
			require.NoError(t, depTask.Insert())
		}
		t.Run(name, test)
	}
}

func TestBlockedOnDeactivatedDependency(t *testing.T) {
	taskId := "t1"
	taskDoc := &Task{
		Id: taskId,
	}
	depTasks := []*Task{
		{Id: depTaskIds[0].TaskId, Status: evergreen.TaskUndispatched, Activated: true},
		{Id: depTaskIds[1].TaskId, Status: evergreen.TaskSucceeded, Activated: false},
		{Id: depTaskIds[2].TaskId, Status: evergreen.TaskUndispatched, Activated: false},
	}
	require.NoError(t, db.Clear(Collection))
	for _, depTask := range depTasks {
		require.NoError(t, depTask.Insert())
	}
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	t.Run("NotBlocked", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[0].TaskId},
		}
		blockingTasks, err := taskDoc.BlockedOnDeactivatedDependency(map[string]Task{})
		require.NoError(t, err)
		assert.Empty(t, blockingTasks)
	})
	t.Run("NoBlockedFinished", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[1].TaskId},
		}
		blockingTasks, err := taskDoc.BlockedOnDeactivatedDependency(map[string]Task{})
		require.NoError(t, err)
		assert.Empty(t, blockingTasks)
	})
	t.Run("Blocked", func(t *testing.T) {
		taskDoc.DependsOn = []Dependency{
			{TaskId: depTaskIds[2].TaskId},
		}
		blockingTasks, err := taskDoc.BlockedOnDeactivatedDependency(map[string]Task{})
		require.NoError(t, err)
		assert.Len(t, blockingTasks, 1)
	})
}

func TestSetTasksScheduledTime(t *testing.T) {
	Convey("With some tasks", t, func() {

		So(db.Clear(Collection), ShouldBeNil)

		tasks := []Task{
			{Id: "t0", ScheduledTime: utility.ZeroTime, ExecutionTasks: []string{"t1", "t2"}},
			{Id: "t1", ScheduledTime: utility.ZeroTime},
			{Id: "t2", ScheduledTime: utility.ZeroTime},
			{Id: "t3", ScheduledTime: utility.ZeroTime},
		}
		for _, task := range tasks {
			So(task.Insert(), ShouldBeNil)
		}
		Convey("when updating ScheduledTime for some of the tasks", func() {
			testTime := time.Unix(31337, 0)
			So(SetTasksScheduledTime(tasks[2:], testTime), ShouldBeNil)

			Convey("the tasks should be updated in memory", func() {
				So(tasks[1].ScheduledTime, ShouldResemble, utility.ZeroTime)
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
					So(t1.ScheduledTime.Round(oneMs), ShouldResemble, utility.ZeroTime)
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

func TestFailedTasksByVersion(t *testing.T) {
	Convey("When calling FailedTasksByVersion...", t, func() {
		So(db.Clear(Collection), ShouldBeNil)
		Convey("only tasks with the failed statuses should be returned", func() {

			tasks := []Task{
				{
					Id:      "one",
					Version: "v1",
					Status:  evergreen.TaskFailed,
				},
				{
					Id:      "two",
					Version: "v1",
					Status:  evergreen.TaskSetupFailed,
				},
				{
					Id:      "three",
					Version: "v1",
					Status:  evergreen.TaskSucceeded,
				},
			}

			for _, task := range tasks {
				So(task.Insert(), ShouldBeNil)
			}

			dbTasks, err := Find(FailedTasksByVersion("v1"))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 2)
			So(dbTasks[0].Id, ShouldNotEqual, "three")
			So(dbTasks[1].Id, ShouldNotEqual, "three")
		})
	})
}

func TestFindTasksByBuildIdAndGithubChecks(t *testing.T) {
	tasks := []Task{
		{
			Id:            "t1",
			BuildId:       "b1",
			IsGithubCheck: true,
		},
		{
			Id:      "t2",
			BuildId: "b1",
		},
		{
			Id:            "t3",
			BuildId:       "b2",
			IsGithubCheck: true,
		},
		{
			Id:            "t4",
			BuildId:       "b2",
			IsGithubCheck: true,
		},
	}

	for _, task := range tasks {
		assert.NoError(t, task.Insert())
	}
	dbTasks, err := FindAll(ByBuildIdAndGithubChecks("b1"))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 1)
	dbTasks, err = FindAll(ByBuildIdAndGithubChecks("b2"))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 2)
	dbTasks, err = FindAll(ByBuildIdAndGithubChecks("b3"))
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 0)
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

func TestIsSystemUnresponsive(t *testing.T) {
	var task Task

	task = Task{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{Type: evergreen.CommandTypeSystem, TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat}}
	assert.True(t, task.IsSystemUnresponsive(), "current definition")

	task = Task{Status: evergreen.TaskSystemUnresponse}
	assert.True(t, task.IsSystemUnresponsive(), "legacy definition")

	task = Task{Status: evergreen.TaskFailed, Details: apimodels.TaskEndDetail{TimedOut: true, Description: evergreen.TaskDescriptionHeartbeat}}
	assert.False(t, task.IsSystemUnresponsive(), "normal timeout")

	task = Task{Status: evergreen.TaskSucceeded}
	assert.False(t, task.IsSystemUnresponsive(), "success")

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

	TestStartTime := utility.FromPythonTime(StartTime).In(time.UTC)
	TestEndTime := utility.FromPythonTime(EndTime).In(time.UTC)

	testresults := []TestResult{
		{
			Status:          "pass",
			TestFile:        "job0_fixture_setup",
			DisplayTestName: "display",
			GroupID:         "group",
			URL:             "https://logkeeper.mongodb.org/build/dd239a5697eedef049a753c6a40a3e7e/test/5d8ba136c2ab68304e1d741c",
			URLRaw:          "https://logkeeper.mongodb.org/build/dd239a5697eedef049a753c6a40a3e7e/test/5d8ba136c2ab68304e1d741c?raw=1",
			ExitCode:        0,
			StartTime:       StartTime,
			EndTime:         EndTime,
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

func TestFindOneIdAndExecutionWithDisplayStatus(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))
	taskDoc := Task{
		Id:        "task",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	}
	assert.NoError(taskDoc.Insert())
	task, err := FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.DisplayStatus, evergreen.TaskWillRun)

	// Should fetch tasks from the old collection
	assert.NoError(taskDoc.Archive())
	task, err = FindOneOldByIdAndExecution(taskDoc.Id, 0)
	assert.NoError(err)
	assert.NotNil(task)
	task, err = FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.OldTaskId, taskDoc.Id)

	// Should fetch recent executions by default
	task, err = FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, nil)
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.Execution, 1)
	assert.Equal(task.DisplayStatus, evergreen.TaskWillRun)

	taskDoc = Task{
		Id:        "task2",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	assert.NoError(taskDoc.Insert())
	task, err = FindOneIdAndExecutionWithDisplayStatus(taskDoc.Id, utility.ToIntPtr(0))
	assert.NoError(err)
	assert.NotNil(task)
	assert.Equal(task.DisplayStatus, evergreen.TaskUnscheduled)
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

func TestFindAllFirstExecution(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, OldCollection))
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", Execution: 1},
		{Id: "t2", DisplayOnly: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}
	oldTask := Task{Id: MakeOldID("t1", 0)}
	require.NoError(t, db.Insert(OldCollection, &oldTask))

	foundTasks, err := FindAllFirstExecution(db.Query(nil))
	assert.NoError(t, err)
	assert.Len(t, foundTasks, 3)
	expectedIDs := []string{"t0", MakeOldID("t1", 0), "t2"}
	for _, task := range foundTasks {
		assert.Contains(t, expectedIDs, task.Id)
		assert.Equal(t, 0, task.Execution)
	}
}

func TestWithinTimePeriodProjectFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, OldCollection))

	taskDocs := []Task{
		{
			Id:      "task1",
			Project: "proj",
		},
		{
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
		{
			Id:         "task1",
			FinishTime: time.Now().AddDate(0, 0, -2), // Should match
			StartTime:  time.Now().AddDate(0, 0, -5), // Shouldn't match
		},
		{
			Id:         "task2",
			FinishTime: time.Now().AddDate(0, 0, -2), // Should match
			StartTime:  time.Now().AddDate(0, 0, -3), // Should match
		},
		{
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
		{
			Id:     "task1",
			Status: "A",
		},
		{
			Id:     "task2",
			Status: "B",
		},
		{
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
		Id:     "task",
		Status: evergreen.TaskSucceeded,
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

	task01, err := FindOneIdOldOrNew("task", 1)
	assert.NoError(err)
	require.NotNil(task01)
	assert.Equal("task", task01.Id)
	assert.Equal(1, task01.Execution)
}

func TestPopulateTestResultsForDisplayTask(t *testing.T) {
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
	require.NoError(t, dt.populateTestResultsForDisplayTask())
	require.Len(t, dt.LocalTestResults, 1)
	assert.Equal("myTest", dt.LocalTestResults[0].TestFile)
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
			state, err := t1.BlockedState(map[string]*Task{})
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
			dependencies := map[string]*Task{t2.Id: &t2}
			state, err := t1.BlockedState(dependencies)
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
			dependencies := map[string]*Task{t2.Id: &t2}
			state, err := t1.BlockedState(dependencies)
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
	dependencies := map[string]*Task{
		"t2": &t2,
		"t3": &t3,
		"t4": &t4,
	}
	state, err := t1.BlockedState(dependencies)
	assert.NoError(err)
	assert.Equal(evergreen.TaskStatusPending, state)
}

func TestBulkInsert(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	t1_a := Task{
		Id:      "t1",
		Version: "version",
	}
	t1_b := Task{
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
	tasks := Tasks{&t1_a, &t1_b, &t2, &t3}
	assert.Error(tasks.InsertUnordered(context.Background()))
	dbTasks, err := Find(ByVersion("version"))
	assert.NoError(err)
	assert.Len(dbTasks, 3)
	for _, dbTask := range dbTasks {
		assert.Equal("version", dbTask.Version)
	}
}

func TestUnscheduleStaleUnderwaterTasksNoDistro(t *testing.T) {
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

func TestUnscheduleStaleUnderwaterTasksWithDistro(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, distro.Collection))
	t1 := Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
		DistroId:      "d0",
	}
	require.NoError(t, t1.Insert())

	d := distro.Distro{
		Id: "d0",
	}
	require.NoError(t, d.Insert())

	_, err := UnscheduleStaleUnderwaterTasks("d0")
	assert.NoError(t, err)
	dbTask, err := FindOneId("t1")
	assert.NoError(t, err)
	assert.False(t, dbTask.Activated)
	assert.EqualValues(t, -1, dbTask.Priority)
}

func TestUnscheduleStaleUnderwaterTasksWithDistroAlias(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, distro.Collection))
	t1 := Task{
		Id:            "t1",
		Status:        evergreen.TaskUndispatched,
		Activated:     true,
		Priority:      0,
		ActivatedTime: time.Time{},
		DistroId:      "d0.0",
	}
	require.NoError(t, t1.Insert())

	d := distro.Distro{
		Id:      "d0",
		Aliases: []string{"d0.0", "d0.1"},
	}
	require.NoError(t, d.Insert())

	_, err := UnscheduleStaleUnderwaterTasks("d0")
	assert.NoError(t, err)
	dbTask, err := FindOneId("t1")
	assert.NoError(t, err)
	assert.False(t, dbTask.Activated)
	assert.EqualValues(t, -1, dbTask.Priority)
}

func TestGetRecentTaskStats(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	tasks := []Task{
		{Id: "t1", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t2", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t3", Status: evergreen.TaskSucceeded, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t4", Status: evergreen.TaskSucceeded, DistroId: "d2", FinishTime: time.Now()},
		{Id: "t5", Status: evergreen.TaskFailed, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t6", Status: evergreen.TaskFailed, DistroId: "d1", FinishTime: time.Now()},
		{Id: "t7", Status: evergreen.TaskFailed, DistroId: "d2", FinishTime: time.Now()},
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
	assert.Contains(bvs, "bv1")
	assert.Contains(bvs, "bv2")
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

	deps, err := t1.FindAllUnmarkedBlockedDependencies()
	assert.NoError(err)
	assert.Len(deps, 1)
}

func TestAddDependency(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := &Task{Id: "t1", DependsOn: depTaskIds}
	assert.NoError(t, t1.Insert())

	assert.NoError(t, t1.AddDependency(depTaskIds[0]))

	updated, err := FindOneId(t1.Id)
	assert.NoError(t, err)
	assert.Equal(t, t1.DependsOn, updated.DependsOn)

	assert.NoError(t, t1.AddDependency(Dependency{TaskId: "td1", Status: evergreen.TaskSucceeded, Unattainable: true}))

	updated, err = FindOneId(t1.Id)
	assert.NoError(t, err)
	assert.Equal(t, len(depTaskIds), len(updated.DependsOn))
	assert.True(t, updated.DependsOn[0].Unattainable)

	assert.NoError(t, t1.AddDependency(Dependency{TaskId: "td1", Status: evergreen.TaskFailed}))

	updated, err = FindOneId(t1.Id)
	assert.NoError(t, err)
	assert.Equal(t, len(depTaskIds)+1, len(updated.DependsOn))

	assert.NoError(t, t1.RemoveDependency("td3"))
	for _, d := range t1.DependsOn {
		if d.TaskId == "td3" {
			assert.Fail(t, "did not remove dependency from in-memory task")
		}
	}
	updated, err = FindOneId(t1.Id)
	assert.NoError(t, err)
	for _, d := range updated.DependsOn {
		if d.TaskId == "td3" {
			assert.Fail(t, "did not remove dependency from db task")
		}
	}
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

	unattainableTasks, err := t1.FindAllMarkedUnattainableDependencies()
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
			DisplayOnly: true,
			FinishTime:  referenceTime.Add(3 * time.Hour),
			TimeTaken:   2 * time.Hour,
		},
		{
			StartTime:  referenceTime,
			FinishTime: utility.ZeroTime,
			TimeTaken:  0,
		},
		{
			StartTime:  utility.ZeroTime,
			FinishTime: utility.ZeroTime,
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

func TestAddHostCreateDetails(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	task := Task{Id: "t1", Execution: 0}
	assert.NoError(t, task.Insert())
	errToSave := errors.Wrapf(errors.New("InsufficientCapacityError"), "error trying to start host")
	assert.NoError(t, AddHostCreateDetails(task.Id, "h1", 0, errToSave))
	dbTask, err := FindOneId(task.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dbTask)
	require.Len(t, dbTask.HostCreateDetails, 1)
	assert.Equal(t, dbTask.HostCreateDetails[0].HostId, "h1")
	assert.Contains(t, dbTask.HostCreateDetails[0].Error, "InsufficientCapacityError")

	assert.NoError(t, AddHostCreateDetails(task.Id, "h2", 0, errToSave))
	dbTask, err = FindOneId(task.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dbTask)
	assert.Len(t, dbTask.HostCreateDetails, 2)
}

func TestUpdateDependsOn(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := &Task{Id: "t1"}
	assert.NoError(t, t1.Insert())
	t2 := &Task{
		Id: "t2",
		DependsOn: []Dependency{
			{TaskId: "t1", Status: evergreen.TaskSucceeded},
			{TaskId: "t5", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t, t2.Insert())

	var err error
	assert.NoError(t, t1.UpdateDependsOn(evergreen.TaskFailed, []string{"t3", "t4"}))
	t2, err = FindOneId("t2")
	assert.NoError(t, err)
	assert.Len(t, t2.DependsOn, 2)

	assert.NoError(t, t1.UpdateDependsOn(evergreen.TaskSucceeded, []string{"t3", "t4"}))
	t2, err = FindOneId("t2")
	assert.NoError(t, err)
	assert.Len(t, t2.DependsOn, 4)
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

	require.NoError(t, MarkGeneratedTasks(t1.Id))
	found, err := FindOneId(t1.Id)
	require.NoError(t, err)
	require.Equal(t, true, found.GeneratedTasks)
	require.Equal(t, "", found.GenerateTasksError)

	require.NoError(t, MarkGeneratedTasks(t1.Id))
	require.NoError(t, MarkGeneratedTasksErr(t1.Id, mockError))
	found, err = FindOneId(t1.Id)
	require.NoError(t, err)
	require.Equal(t, true, found.GeneratedTasks)
	require.Equal(t, "", found.GenerateTasksError, "calling after GeneratedTasks is set should not set an error")

	t3 := &Task{
		Id: "t3",
	}
	require.NoError(t, t3.Insert())
	require.NoError(t, MarkGeneratedTasksErr(t3.Id, mongo.ErrNoDocuments))
	found, err = FindOneId(t3.Id)
	require.NoError(t, err)
	require.Equal(t, false, found.GeneratedTasks, "document not found should not set generated tasks, since this was a race and did not generate.tasks")
	require.Equal(t, "", found.GenerateTasksError)

	t4 := &Task{
		Id: "t4",
	}
	dupError := errors.New("duplicate key error")
	require.NoError(t, t4.Insert())
	require.NoError(t, MarkGeneratedTasksErr(t4.Id, dupError))
	found, err = FindOneId(t4.Id)
	require.NoError(t, err)
	require.Equal(t, false, found.GeneratedTasks, "duplicate key error should not set generated tasks, since this was a race and did not generate.tasks")
	require.Equal(t, "", found.GenerateTasksError)
}

func TestGetAllDependencies(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{
			Id:        "t0",
			DependsOn: []Dependency{{TaskId: "dependedOn0"}},
		},
		{
			Id:        "t1",
			DependsOn: []Dependency{{TaskId: "dependedOn1"}},
		},
	}
	// not in the map and not in the db
	dependencies, err := GetAllDependencies([]string{tasks[0].Id}, map[string]*Task{})
	assert.Error(t, err)
	assert.Nil(t, dependencies)

	// in the map
	dependencies, err = GetAllDependencies([]string{tasks[0].Id}, map[string]*Task{tasks[0].Id: &tasks[0]})
	assert.NoError(t, err)
	assert.Len(t, dependencies, 1)
	assert.Equal(t, "dependedOn0", dependencies[0].TaskId)

	// mix of map and db
	require.NoError(t, tasks[1].Insert())
	taskMap := map[string]*Task{tasks[0].Id: &tasks[0]}
	dependencies, err = GetAllDependencies([]string{tasks[0].Id, tasks[1].Id}, taskMap)
	assert.NoError(t, err)
	assert.Len(t, dependencies, 2)
	assert.Len(t, taskMap, 1)
}

func TestGetRecursiveDependenciesUp(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1"},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	taskDependsOn, err := GetRecursiveDependenciesUp([]Task{tasks[3], tasks[4]}, nil)
	assert.NoError(t, err)
	assert.Len(t, taskDependsOn, 3)
	expectedIDs := []string{"t2", "t1", "t0"}
	for _, task := range taskDependsOn {
		assert.Contains(t, expectedIDs, task.Id)
	}
}

func TestGetRecursiveDependenciesUpWithTaskGroup(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{Id: "t0", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 0},
		{Id: "t1", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 1},
		{Id: "t2", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 2},
		{Id: "t3", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 3},
		{Id: "t4", BuildId: "b1", TaskGroup: "tg", TaskGroupMaxHosts: 1, TaskGroupOrder: 4},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}
	taskDependsOn, err := GetRecursiveDependenciesUp([]Task{tasks[2], tasks[3]}, nil)
	assert.NoError(t, err)
	assert.Len(t, taskDependsOn, 2)
	expectedIDs := []string{"t0", "t1"}
	for _, task := range taskDependsOn {
		assert.Contains(t, expectedIDs, task.Id)
	}
}

func TestGetRecursiveDependenciesDown(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1"},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	dependingOnMe, err := getRecursiveDependenciesDown([]string{"t0"}, nil)
	assert.NoError(t, err)
	assert.Len(t, dependingOnMe, 3)
	expectedIDs := []string{"t2", "t4", "t5"}
	for _, task := range dependingOnMe {
		assert.Contains(t, expectedIDs, task.Id)
	}
}

func TestDeactivateDependencies(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))

	tasks := []Task{
		{Id: "t0"},
		{Id: "t1"},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}, Activated: false},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}, Activated: true},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}, Activated: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	updatedIDs := []string{"t4", "t5"}
	err := DeactivateDependencies([]string{"t0"}, "")
	assert.NoError(t, err)

	dbTasks, err := FindAll(db.Q{})
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 6)

	for _, task := range dbTasks {
		if utility.StringSliceContains(updatedIDs, task.Id) {
			assert.False(t, task.Activated)
			assert.True(t, task.DeactivatedForDependency)
		} else {
			for _, origTask := range tasks {
				if origTask.Id == task.Id {
					assert.Equal(t, origTask.Activated, task.Activated)
				}
			}
		}
	}
}

func TestActivateDeactivatedDependencies(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))

	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}}, Activated: false},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t1"}}, Activated: false, DeactivatedForDependency: true},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t0"}}, Activated: false, DeactivatedForDependency: true},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t3"}}, Activated: false, DeactivatedForDependency: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	updatedIDs := []string{"t3", "t4"}
	err := ActivateDeactivatedDependencies([]string{"t0"}, "")
	assert.NoError(t, err)

	dbTasks, err := FindAll(db.Q{})
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 5)

	for _, task := range dbTasks {
		if utility.StringSliceContains(updatedIDs, task.Id) {
			assert.True(t, task.Activated)
			assert.False(t, task.DeactivatedForDependency)
		} else {
			for _, origTask := range tasks {
				if origTask.Id == task.Id {
					assert.Equal(t, origTask.Activated, task.Activated)
				}
			}
		}
	}
}

func TestTopologicalSort(t *testing.T) {
	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t2"}}},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t0"}}},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t1"}}},
	}

	sortedTasks, err := topologicalSort(tasks)
	assert.NoError(t, err)
	assert.Len(t, sortedTasks, 4)

	indexMap := make(map[string]int)
	for i, task := range sortedTasks {
		indexMap[task.Id] = i
	}

	assert.True(t, indexMap["t0"] < indexMap["t1"])
	assert.True(t, indexMap["t0"] < indexMap["t2"])
	assert.True(t, indexMap["t0"] < indexMap["t3"])
	assert.True(t, indexMap["t2"] < indexMap["t1"])
	assert.True(t, indexMap["t1"] < indexMap["t3"])
}

func TestActivateTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))

	tasks := []Task{
		{Id: "t0"},
		{Id: "t1", DependsOn: []Dependency{{TaskId: "t0"}}, Activated: false},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t1"}}, Activated: false, DeactivatedForDependency: true},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t0"}}, Activated: false, DeactivatedForDependency: true},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t0"}, {TaskId: "t3"}}, Activated: false, DeactivatedForDependency: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	updatedIDs := []string{"t0", "t3", "t4"}
	err := ActivateTasks([]Task{tasks[0]}, time.Time{}, "")
	assert.NoError(t, err)

	dbTasks, err := FindAll(db.Q{})
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 5)

	for _, task := range dbTasks {
		if utility.StringSliceContains(updatedIDs, task.Id) {
			assert.True(t, task.Activated)
		} else {
			for _, origTask := range tasks {
				if origTask.Id == task.Id {
					assert.Equal(t, origTask.Activated, task.Activated, fmt.Sprintf("task '%s' mismatch", task.Id))
				}
			}
		}
	}
}

func TestDeactivateTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))

	tasks := []Task{
		{Id: "t0"},
		{Id: "t1"},
		{Id: "t2", DependsOn: []Dependency{{TaskId: "t1"}, {TaskId: "t0"}}, Activated: false},
		{Id: "t3", DependsOn: []Dependency{{TaskId: "t1"}}},
		{Id: "t4", DependsOn: []Dependency{{TaskId: "t2"}}, Activated: true},
		{Id: "t5", DependsOn: []Dependency{{TaskId: "t4"}}, Activated: true},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	updatedIDs := []string{"t0", "t4", "t5"}
	err := DeactivateTasks([]Task{tasks[0]}, "")
	assert.NoError(t, err)

	dbTasks, err := FindAll(db.Q{})
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 6)

	for _, task := range dbTasks {
		if utility.StringSliceContains(updatedIDs, task.Id) {
			assert.False(t, task.Activated)
		} else {
			for _, origTask := range tasks {
				if origTask.Id == task.Id {
					assert.Equal(t, origTask.Activated, task.Activated)
				}
			}
		}
	}
}

func TestSetDisabledPriority(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))

	tasks := []Task{
		{Id: "t0", ExecutionTasks: []string{"t1", "t2"}},
		{Id: "t1"},
		{Id: "t2"},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	assert.NoError(t, tasks[0].SetDisabledPriority(""))

	dbTasks, err := FindAll(db.Q{})
	assert.NoError(t, err)
	assert.Len(t, dbTasks, 3)

	for _, task := range dbTasks {
		assert.Equal(t, evergreen.DisabledTaskPriority, task.Priority)
	}
}

func TestSetHasLegacyResults(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert())
	assert.NoError(t, task.SetHasLegacyResults(true))

	assert.True(t, utility.FromBoolPtr(task.HasLegacyResults))

	taskFromDb, err := FindOneId("t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.NotNil(t, taskFromDb.HasLegacyResults)
	assert.True(t, utility.FromBoolPtr(taskFromDb.HasLegacyResults))
}

func TestSetGeneratedTasksToActivate(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert())

	// add stepback task to variant
	assert.NoError(t, task.SetGeneratedTasksToActivate("bv2", "t2"))
	taskFromDb, err := FindOneId("t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, taskFromDb.GeneratedTasksToActivate["bv2"], []string{"t2"})

	// add different stepback task to variant
	assert.NoError(t, task.SetGeneratedTasksToActivate("bv2", "t2.0"))
	taskFromDb, err = FindOneId("t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, taskFromDb.GeneratedTasksToActivate["bv2"], []string{"t2", "t2.0"})

	// verify duplicate doesn't overwrite
	assert.NoError(t, task.SetGeneratedTasksToActivate("bv2", "t2.0"))
	taskFromDb, err = FindOneId("t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, taskFromDb.GeneratedTasksToActivate["bv2"], []string{"t2", "t2.0"})

	// adding second variant doesn't affect previous
	assert.NoError(t, task.SetGeneratedTasksToActivate("bv3", "t3"))
	taskFromDb, err = FindOneId("t1")
	assert.NoError(t, err)
	assert.NotNil(t, taskFromDb)
	assert.Equal(t, taskFromDb.GeneratedTasksToActivate["bv2"], []string{"t2", "t2.0"})
	assert.Equal(t, taskFromDb.GeneratedTasksToActivate["bv3"], []string{"t3"})
}

func TestDisplayStatus(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:     "t1",
		Status: evergreen.TaskSucceeded,
	}
	assert.NoError(t, t1.Insert())
	checkStatuses(t, evergreen.TaskSucceeded, t1)
	t2 := Task{
		Id:        "t2",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	}
	assert.NoError(t, t2.Insert())
	checkStatuses(t, evergreen.TaskWillRun, t2)
	t3 := Task{
		Id:        "t3",
		Status:    evergreen.TaskFailed,
		Activated: true,
	}
	assert.NoError(t, t3.Insert())
	checkStatuses(t, evergreen.TaskFailed, t3)
	t4 := Task{
		Id:     "t4",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSetup,
		},
	}
	assert.NoError(t, t4.Insert())
	checkStatuses(t, evergreen.TaskSetupFailed, t4)
	t5 := Task{
		Id:     "t5",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeSystem,
		},
	}
	assert.NoError(t, t5.Insert())
	checkStatuses(t, evergreen.TaskSystemFailed, t5)
	t6 := Task{
		Id:     "t6",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type:     evergreen.CommandTypeSystem,
			TimedOut: true,
		},
	}
	assert.NoError(t, t6.Insert())
	checkStatuses(t, evergreen.TaskSystemTimedOut, t6)
	t7 := Task{
		Id:     "t7",
		Status: evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type:        evergreen.CommandTypeSystem,
			TimedOut:    true,
			Description: evergreen.TaskDescriptionHeartbeat,
		},
	}
	assert.NoError(t, t7.Insert())
	checkStatuses(t, evergreen.TaskSystemUnresponse, t7)
	t8 := Task{
		Id:        "t8",
		Status:    evergreen.TaskStarted,
		Activated: true,
	}
	assert.NoError(t, t8.Insert())
	checkStatuses(t, evergreen.TaskStarted, t8)
	t9 := Task{
		Id:        "t9",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	assert.NoError(t, t9.Insert())
	checkStatuses(t, evergreen.TaskUnscheduled, t9)
	t10 := Task{
		Id:        "t10",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []Dependency{
			{
				TaskId:       "t9",
				Unattainable: true,
				Status:       "success",
			},
			{
				TaskId:       "t8",
				Unattainable: false,
				Status:       "success",
			},
		},
	}
	assert.NoError(t, t10.Insert())
	checkStatuses(t, evergreen.TaskStatusBlocked, t10)
	t11 := Task{
		Id:        "t11",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []Dependency{
			{
				TaskId:       "t8",
				Unattainable: false,
				Status:       "success",
			},
		},
	}
	assert.NoError(t, t11.Insert())
	checkStatuses(t, evergreen.TaskWillRun, t11)
	t12 := Task{
		Id:                   "t12",
		Status:               evergreen.TaskUndispatched,
		Activated:            true,
		OverrideDependencies: true,
		DependsOn: []Dependency{
			{
				TaskId:       "t9",
				Unattainable: true,
				Status:       "success",
			},
		},
	}
	assert.NoError(t, t12.Insert())
	checkStatuses(t, evergreen.TaskWillRun, t11)
}

func TestFindTaskNamesByBuildVariant(t *testing.T) {
	Convey("Should return unique task names for a given build variant", t, func() {
		assert.NoError(t, db.ClearCollections(Collection))
		t1 := Task{
			Id:           "t1",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "dist",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, t1.Insert())
		t2 := Task{
			Id:           "t2",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-agent",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, t2.Insert())
		t3 := Task{
			Id:           "t3",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-graphql",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, t3.Insert())
		t4 := Task{
			Id:           "t4",
			Status:       evergreen.TaskFailed,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-graphql",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, t4.Insert())
		buildVariantTask, err := FindTaskNamesByBuildVariant("evergreen", "ubuntu1604")
		assert.NoError(t, err)
		assert.Equal(t, []string{"dist", "test-agent", "test-graphql"}, buildVariantTask)

	})
	Convey("Should only include tasks that appear on mainline commits", t, func() {
		assert.NoError(t, db.ClearCollections(Collection))
		t1 := Task{
			Id:           "t1",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-patch-only",
			Project:      "evergreen",
			Requester:    evergreen.PatchVersionRequester,
		}
		assert.NoError(t, t1.Insert())
		t2 := Task{
			Id:           "t2",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-graphql",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, t2.Insert())
		t3 := Task{
			Id:           "t3",
			Status:       evergreen.TaskSucceeded,
			BuildVariant: "ubuntu1604",
			DisplayName:  "dist",
			Project:      "evergreen",
			Requester:    evergreen.PatchVersionRequester,
		}
		assert.NoError(t, t3.Insert())
		t4 := Task{
			Id:           "t4",
			Status:       evergreen.TaskFailed,
			BuildVariant: "ubuntu1604",
			DisplayName:  "test-something",
			Project:      "evergreen",
			Requester:    evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, t4.Insert())
		buildVariantTasks, err := FindTaskNamesByBuildVariant("evergreen", "ubuntu1604")
		assert.NoError(t, err)
		assert.Equal(t, []string{"test-graphql", "test-something"}, buildVariantTasks)
	})

}

func checkStatuses(t *testing.T, expected string, toCheck Task) {
	var dbTasks []Task
	aggregation := []bson.M{
		{"$match": bson.M{
			IdKey: toCheck.Id,
		}},
		addDisplayStatus,
	}
	err := db.Aggregate(Collection, aggregation, &dbTasks)
	assert.NoError(t, err)
	assert.Equal(t, expected, dbTasks[0].DisplayStatus)
	assert.Equal(t, expected, toCheck.GetDisplayStatus())
}

func TestGetLatestExecution(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	sample := Task{
		Id:        "task_id_some_other_stuff",
		Execution: 55,
	}
	assert.NoError(t, sample.Insert())
	execution, err := GetLatestExecution(sample.Id)
	assert.NoError(t, err)
	assert.Equal(t, sample.Execution, execution)
	execution, err = GetLatestExecution(fmt.Sprintf("%s_3", sample.Id))
	assert.NoError(t, err)
	assert.Equal(t, sample.Execution, execution)
}

func TestArchiveMany(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, OldCollection))
	t1 := Task{
		Id:      "t1",
		Status:  evergreen.TaskFailed,
		Aborted: true,
		Version: "v",
	}
	assert.NoError(t, t1.Insert())
	t2 := Task{
		Id:       "t2",
		Status:   evergreen.TaskFailed,
		Aborted:  true,
		Restarts: 2,
		Version:  "v",
	}
	assert.NoError(t, t2.Insert())
	dt := Task{
		Id:             "dt",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et"},
		Version:        "v",
	}
	assert.NoError(t, dt.Insert())
	et := Task{
		Id:      "et",
		Version: "v",
	}
	assert.NoError(t, et.Insert())

	tasks := []Task{t1, t2, dt}
	err := ArchiveMany(tasks)
	assert.NoError(t, err)
	currentTasks, err := FindAll(ByVersion("v"))
	assert.NoError(t, err)
	assert.Len(t, currentTasks, 4)
	for _, task := range currentTasks {
		assert.False(t, task.Aborted)
		assert.Equal(t, 1, task.Execution)
		if task.Id == t2.Id {
			assert.Equal(t, 3, task.Restarts)
		}
	}
	oldTasks, err := FindAllOld(ByVersion("v"))
	assert.NoError(t, err)
	assert.Len(t, oldTasks, 4)
	for _, task := range oldTasks {
		assert.True(t, task.Archived)
		assert.Equal(t, 0, task.Execution)
	}
}

func TestAddParentDisplayTasks(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	dt1 := Task{
		Id:             "dt1",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et1", "et2"},
	}
	assert.NoError(t, dt1.Insert())
	dt2 := Task{
		Id:             "dt2",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et3", "et4"},
	}
	assert.NoError(t, dt2.Insert())
	execTasks := []Task{
		{Id: "et1"},
		{Id: "et2"},
		{Id: "et3"},
		{Id: "et4"},
	}
	for _, et := range execTasks {
		assert.NoError(t, et.Insert())
	}
	tasks, err := AddParentDisplayTasks(execTasks)
	assert.NoError(t, err)
	assert.Equal(t, "et1", tasks[0].Id)
	assert.Equal(t, dt1.Id, tasks[0].DisplayTask.Id)
	assert.Equal(t, "et2", tasks[1].Id)
	assert.Equal(t, dt1.Id, tasks[1].DisplayTask.Id)
	assert.Equal(t, "et3", tasks[2].Id)
	assert.Equal(t, dt2.Id, tasks[2].DisplayTask.Id)
	assert.Equal(t, "et4", tasks[3].Id)
	assert.Equal(t, dt2.Id, tasks[3].DisplayTask.Id)
}

func TestAddDisplayTaskIdToExecTasks(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	t1 := &Task{
		Id:            "t1",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	t2 := &Task{
		Id:            "t2",
		DisplayTaskId: nil,
	}
	t3 := &Task{
		Id:            "t3",
		DisplayTaskId: utility.ToStringPtr(""),
	}
	assert.NoError(t, t1.Insert())
	assert.NoError(t, t2.Insert())
	assert.NoError(t, t3.Insert())

	assert.NoError(t, AddDisplayTaskIdToExecTasks("dt", []string{t1.Id, t2.Id}))

	var err error
	t1, err = FindOneId(t1.Id)
	assert.NoError(t, err)
	assert.Equal(t, utility.FromStringPtr(t1.DisplayTaskId), "dt")

	t2, err = FindOneId(t2.Id)
	assert.NoError(t, err)
	assert.Equal(t, utility.FromStringPtr(t2.DisplayTaskId), "dt")

	t3, err = FindOneId(t3.Id)
	assert.NoError(t, err)
	assert.NotEqual(t, utility.FromStringPtr(t3.DisplayTaskId), "dt")
}

func TestAddExecTasksToDisplayTask(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	dt1 := Task{
		Id:             "dt1",
		DisplayOnly:    true,
		Activated:      false,
		ExecutionTasks: []string{"et1", "et2"},
	}
	assert.NoError(t, dt1.Insert())

	// no tasks to add
	assert.NoError(t, AddExecTasksToDisplayTask(dt1.Id, []string{}, false))
	dtFromDB, err := FindOneId(dt1.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dtFromDB)
	assert.Len(t, dtFromDB.ExecutionTasks, 2)
	assert.Contains(t, dtFromDB.ExecutionTasks, "et1")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et2")

	// new and existing tasks to add (existing tasks not duplicated)
	assert.NoError(t, AddExecTasksToDisplayTask(dt1.Id, []string{"et2", "et3", "et4"}, true))
	dtFromDB, err = FindOneId(dt1.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dtFromDB)
	assert.Len(t, dtFromDB.ExecutionTasks, 4)
	assert.Contains(t, dtFromDB.ExecutionTasks, "et1")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et2")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et3")
	assert.Contains(t, dtFromDB.ExecutionTasks, "et4")
	assert.True(t, dtFromDB.Activated)
	assert.False(t, utility.IsZeroTime(dtFromDB.ActivatedTime))
}

func TestGetTasksByVersionExecTasks(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	// test that we can handle the different kinds of tasks
	t1 := Task{
		Id:            "execWithDisplayId",
		Version:       "v1",
		DisplayTaskId: utility.ToStringPtr("displayTask"),
	}
	t2 := Task{
		Id:            "notAnExec",
		Version:       "v1",
		DisplayTaskId: utility.ToStringPtr(""),
	}

	t3 := Task{
		Id:      "execWithNoId",
		Version: "v1",
	}
	t4 := Task{
		Id:      "notAnExecWithNoId",
		Version: "v1",
	}
	dt := Task{
		Id:             "displayTask",
		Version:        "v1",
		DisplayOnly:    true,
		ExecutionTasks: []string{"execWithDisplayId", "execWithNoId"},
	}
	assert.NoError(t, db.InsertMany(Collection, t1, t2, t3, t4, dt))

	// execution tasks have been filtered outs
	opts := GetTasksByVersionOptions{}
	tasks, count, err := GetTasksByVersion("v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, count, 3)
	// alphabetical order
	assert.Equal(t, dt.Id, tasks[0].Id)
	assert.Equal(t, t2.Id, tasks[1].Id)
	assert.Equal(t, t4.Id, tasks[2].Id)
}

func TestGetTasksByVersionAnnotations(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, annotations.Collection))
	t1 := Task{
		Id:        "t1",
		Version:   "v1",
		Execution: 2,
		Status:    evergreen.TaskSucceeded,
	}
	t2 := Task{
		Id:        "t2",
		Version:   "v1",
		Execution: 3,
		Status:    evergreen.TaskFailed,
	}
	t3 := Task{
		Id:        "t3",
		Version:   "v1",
		Execution: 1,
		Status:    evergreen.TaskFailed,
	}
	assert.NoError(t, db.InsertMany(Collection, t1, t2, t3))

	a := annotations.TaskAnnotation{
		Id:            "myAnnotation",
		TaskId:        t2.Id,
		TaskExecution: t2.Execution,
		Issues: []annotations.IssueLink{
			{IssueKey: "EVG-1212"},
		},
	}
	assert.NoError(t, a.Upsert())

	opts := GetTasksByVersionOptions{}
	tasks, count, err := GetTasksByVersion("v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, count, 3)
	assert.Equal(t, tasks[0].Id, "t1")
	assert.Equal(t, evergreen.TaskSucceeded, tasks[0].DisplayStatus)
	assert.Equal(t, tasks[1].Id, "t2")
	assert.Equal(t, evergreen.TaskKnownIssue, tasks[1].DisplayStatus)
	assert.Equal(t, tasks[2].Id, "t3")
	assert.Equal(t, evergreen.TaskFailed, tasks[2].DisplayStatus)
}

func TestAbortVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	finishedExecTask := &Task{
		Id:      "et1",
		Version: "v1",
		Status:  evergreen.TaskSucceeded,
	}
	failingExecTask := &Task{
		Id:      "et2",
		Version: "v1",
		Status:  evergreen.TaskFailed,
	}
	otherExecTask := &Task{
		Id:      "et3",
		Version: "v1",
		Status:  evergreen.TaskStarted,
	}
	dt := &Task{
		Id:             "dt",
		Version:        "v1",
		Status:         evergreen.TaskStarted,
		ExecutionTasks: []string{"et1", "et2", "et3"},
	}
	assert.NoError(t, db.InsertMany(Collection, finishedExecTask, failingExecTask, otherExecTask, dt))

	assert.NoError(t, AbortVersion("v1", AbortInfo{TaskID: "et2"}))

	var err error
	dt, err = FindOneId("dt")
	assert.NoError(t, err)
	require.NotNil(t, dt)
	assert.False(t, dt.Aborted)
	assert.Empty(t, dt.AbortInfo.TaskID)

	otherExecTask, err = FindOneId("et3")
	assert.NoError(t, err)
	require.NotNil(t, otherExecTask)
	assert.True(t, otherExecTask.Aborted)
	assert.NotEmpty(t, otherExecTask.AbortInfo.TaskID)
}

func TestGetTaskStatsByVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:        "t1",
		Version:   "v1",
		Execution: 0,
		Status:    evergreen.TaskSucceeded,
	}
	t2 := Task{
		Id:        "t2",
		Version:   "v1",
		Execution: 0,
		Status:    evergreen.TaskFailed,
	}
	t3 := Task{
		Id:        "t3",
		Version:   "v1",
		Execution: 1,
		Status:    evergreen.TaskSucceeded,
	}
	t4 := Task{
		Id:        "t4",
		Version:   "v1",
		Execution: 1,
		Status:    evergreen.TaskFailed,
	}
	t5 := Task{
		Id:        "t5",
		Version:   "v1",
		Execution: 2,
		Status:    evergreen.TaskStatusPending,
	}
	t6 := Task{
		Id:        "t6",
		Version:   "v1",
		Execution: 2,
		Status:    evergreen.TaskFailed,
	}
	assert.NoError(t, db.InsertMany(Collection, t1, t2, t3, t4, t5, t6))
	opts := GetTasksByVersionOptions{}
	stats, err := GetTaskStatsByVersion("v1", opts)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(stats))

}

func TestHasMatchingTasks(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	t1 := Task{
		Id:        "t1",
		Version:   "v1",
		Execution: 0,
		Status:    evergreen.TaskSucceeded,
	}
	t2 := Task{
		Id:        "t2",
		Version:   "v1",
		Execution: 0,
		Status:    evergreen.TaskFailed,
	}
	t3 := Task{
		Id:        "t3",
		Version:   "v1",
		Execution: 1,
		Status:    evergreen.TaskSucceeded,
	}
	t4 := Task{
		Id:        "t4",
		Version:   "v1",
		Execution: 1,
		Status:    evergreen.TaskFailed,
	}
	t5 := Task{
		Id:        "t5",
		Version:   "v1",
		Execution: 2,
		Status:    evergreen.TaskStatusPending,
	}
	t6 := Task{
		Id:        "t6",
		Version:   "v1",
		Execution: 2,
		Status:    evergreen.TaskFailed,
	}
	assert.NoError(t, db.InsertMany(Collection, t1, t2, t3, t4, t5, t6))
	opts := HasMatchingTasksOptions{
		Statuses: []string{evergreen.TaskFailed},
	}
	hasMatchingTasks, err := HasMatchingTasks("v1", opts)
	assert.NoError(t, err)
	assert.True(t, hasMatchingTasks)

	opts.Statuses = []string{evergreen.TaskWillRun}

	hasMatchingTasks, err = HasMatchingTasks("v1", opts)
	assert.NoError(t, err)
	assert.False(t, hasMatchingTasks)
}

func TestByExecutionTasksAndMaxExecution(t *testing.T) {
	tasksToFetch := []*string{utility.ToStringPtr("t1"), utility.ToStringPtr("t2")}
	t.Run("Fetching latest execution with same executions", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(Collection, OldCollection))
		t1 := Task{
			Id:        "t1",
			Version:   "v1",
			Execution: 1,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(Collection, t1))

		ot1 := t1
		ot1.Execution = 0
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot1))

		t2 := Task{
			Id:        "t2",
			Version:   "v1",
			Execution: 1,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(Collection, t2))
		ot2 := t2
		ot2.Execution = 0
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot2))

		tasks, err := FindByExecutionTasksAndMaxExecution(tasksToFetch, 1)
		tasks = convertOldTasksIntoTasks(tasks)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(tasks))
		assertTasksAreEqual(t, t1, tasks[0], 1)
		assertTasksAreEqual(t, t2, tasks[1], 1)
	})
	t.Run("Fetching latest execution with mismatching executions", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(Collection, OldCollection))
		t1 := Task{
			Id:        "t1",
			Version:   "v1",
			Execution: 2,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(Collection, t1))

		ot1 := t1
		ot1.Execution = 1
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot1))

		ot1.Execution = 1
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot1))

		t2 := Task{
			Id:        "t2",
			Version:   "v1",
			Execution: 1,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(Collection, t2))
		ot2 := t2
		ot2.Execution = 0
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot2))

		tasks, err := FindByExecutionTasksAndMaxExecution(tasksToFetch, 2)
		tasks = convertOldTasksIntoTasks(tasks)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(tasks))
		assertTasksAreEqual(t, t1, tasks[0], 2)
		assertTasksAreEqual(t, t2, tasks[1], 1)
	})
	t.Run("Fetching older executions with same execution", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(Collection, OldCollection))
		t1 := Task{
			Id:        "t1",
			Version:   "v1",
			Execution: 2,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(Collection, t1))

		ot1 := t1
		ot1.Execution = 1
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot1))

		ot1.Execution = 0
		ot1 = *ot1.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot1))

		t2 := Task{
			Id:        "t2",
			Version:   "v1",
			Execution: 2,
			Status:    evergreen.TaskSucceeded,
		}
		assert.NoError(t, db.Insert(Collection, t2))

		ot2 := t2
		ot2.Execution = 1
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot2))

		ot2.Execution = 0
		ot2 = *ot2.makeArchivedTask()
		assert.NoError(t, db.Insert(OldCollection, ot2))

		tasks, err := FindByExecutionTasksAndMaxExecution(tasksToFetch, 1)
		tasks = convertOldTasksIntoTasks(tasks)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(tasks))
		assertTasksAreEqual(t, t1, tasks[0], 1)
		assertTasksAreEqual(t, t2, tasks[1], 1)
	})

}

func convertOldTasksIntoTasks(tasks []Task) []Task {
	updatedTasks := []Task{}
	for _, t := range tasks {
		if t.OldTaskId != "" {
			t.Id = t.OldTaskId
		}
		updatedTasks = append(updatedTasks, t)
	}
	return updatedTasks
}

func assertTasksAreEqual(t *testing.T, expected, actual Task, exectedExecution int) {
	assert.Equal(t, expected.Id, actual.Id)
	assert.Equal(t, expected.Version, actual.Version)
	assert.Equal(t, expected.Status, actual.Status)
	assert.Equal(t, exectedExecution, actual.Execution)
}
