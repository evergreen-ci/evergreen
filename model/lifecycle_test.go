package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func taskIdInSlice(tasks []task.Task, id string) bool {
	for _, task := range tasks {
		if task.Id == id {
			return true
		}
	}
	return false
}

func TestTaskSetPriority(t *testing.T) {

	Convey("With a task", t, func() {

		require.NoError(t, db.ClearCollections(task.Collection, build.Collection))

		tasks := []task.Task{
			{
				Id:        "one",
				DependsOn: []task.Dependency{{TaskId: "two", Status: ""}, {TaskId: "three", Status: ""}, {TaskId: "four", Status: ""}},
				Activated: true,
				BuildId:   "b0",
			},
			{
				Id:        "two",
				Priority:  5,
				Activated: true,
			},
			{
				Id:        "three",
				DependsOn: []task.Dependency{{TaskId: "five", Status: ""}},
				Activated: true,
			},
			{
				Id:        "four",
				DependsOn: []task.Dependency{{TaskId: "five", Status: ""}},
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

		b0 := build.Build{Id: "b0"}
		require.NoError(t, b0.Insert())

		Convey("setting its priority should update it and all dependencies in the database", func() {

			So(SetTaskPriority(tasks[0], 1, "user"), ShouldBeNil)

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
			So(SetTaskPriority(tasks[0], 1, "user"), ShouldBeNil)
			So(tasks[0].Activated, ShouldEqual, true)
			So(SetTaskPriority(tasks[0], -1, "user"), ShouldBeNil)

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

func TestBuildSetPriority(t *testing.T) {

	Convey("With a build", t, func() {

		require.NoError(t, db.ClearCollections(build.Collection, task.Collection),
			"Error clearing test collection")

		b := &build.Build{
			Id: "build",
		}
		So(b.Insert(), ShouldBeNil)

		taskOne := &task.Task{Id: "taskOne", BuildId: b.Id}
		So(taskOne.Insert(), ShouldBeNil)

		taskTwo := &task.Task{Id: "taskTwo", BuildId: b.Id}
		So(taskTwo.Insert(), ShouldBeNil)

		taskThree := &task.Task{Id: "taskThree", BuildId: b.Id}
		So(taskThree.Insert(), ShouldBeNil)

		Convey("setting its priority should update the priority"+
			" of all its tasks in the database", func() {

			So(SetBuildPriority(b.Id, 42, ""), ShouldBeNil)

			tasks, err := task.Find(task.ByBuildId(b.Id))
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 3)
			So(tasks[0].Priority, ShouldEqual, 42)
			So(tasks[1].Priority, ShouldEqual, 42)
			So(tasks[2].Priority, ShouldEqual, 42)
		})

	})

}

func TestBuildRestart(t *testing.T) {
	Convey("Restarting a build", t, func() {

		Convey("with task abort should update the status of"+
			" non in-progress tasks and abort in-progress ones", func() {

			require.NoError(t, db.ClearCollections(build.Collection, task.Collection, task.OldCollection),
				"Error clearing test collection")
			b := &build.Build{Id: "build"}
			So(b.Insert(), ShouldBeNil)

			taskOne := &task.Task{
				Id:          "task1",
				DisplayName: "task1",
				BuildId:     b.Id,
				Status:      evergreen.TaskSucceeded,
				Activated:   true,
			}
			So(taskOne.Insert(), ShouldBeNil)

			taskTwo := &task.Task{
				Id:          "task2",
				DisplayName: "task2",
				BuildId:     b.Id,
				Status:      evergreen.TaskDispatched,
				Activated:   true,
			}
			So(taskTwo.Insert(), ShouldBeNil)

			So(RestartBuild(b.Id, []string{"task1", "task2"}, true, evergreen.DefaultTaskActivator), ShouldBeNil)
			var err error
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			So(b.Activated, ShouldEqual, true)
			taskOne, err = task.FindOne(task.ById("task1"))
			So(err, ShouldBeNil)
			So(taskOne.Status, ShouldEqual, evergreen.TaskUndispatched)
			taskTwo, err = task.FindOne(task.ById("task2"))
			So(err, ShouldBeNil)
			So(taskTwo.Aborted, ShouldEqual, true)
		})

		Convey("without task abort should update the status"+
			" of only those build tasks not in-progress", func() {

			require.NoError(t, db.ClearCollections(build.Collection),
				"Error clearing test collection")
			b := &build.Build{Id: "build"}
			So(b.Insert(), ShouldBeNil)

			taskThree := &task.Task{
				Id:          "task3",
				DisplayName: "task3",
				BuildId:     b.Id,
				Status:      evergreen.TaskSucceeded,
				Activated:   true,
			}
			So(taskThree.Insert(), ShouldBeNil)

			taskFour := &task.Task{
				Id:          "task4",
				DisplayName: "task4",
				BuildId:     b.Id,
				Status:      evergreen.TaskDispatched,
				Activated:   true,
			}
			So(taskFour.Insert(), ShouldBeNil)

			So(RestartBuild(b.Id, []string{"task3", "task4"}, false, evergreen.DefaultTaskActivator), ShouldBeNil)
			var err error
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			taskThree, err = task.FindOne(task.ById("task3"))
			So(err, ShouldBeNil)
			So(taskThree.Status, ShouldEqual, evergreen.TaskUndispatched)
			taskFour, err = task.FindOne(task.ById("task4"))
			So(err, ShouldBeNil)
			So(taskFour.Aborted, ShouldEqual, false)
			So(taskFour.Status, ShouldEqual, evergreen.TaskDispatched)
		})

	})
}

func TestBuildMarkAborted(t *testing.T) {
	Convey("With a build", t, func() {

		require.NoError(t, db.ClearCollections(build.Collection, task.Collection, VersionCollection),
			"Error clearing test collection")

		v := &Version{
			Id: "v",
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant:     "bv",
					ActivationStatus: ActivationStatus{Activated: true},
				},
			},
		}

		So(v.Insert(), ShouldBeNil)

		b := &build.Build{
			Id:           "build",
			Activated:    true,
			BuildVariant: "bv",
			Version:      "v",
		}
		So(b.Insert(), ShouldBeNil)

		Convey("when marking it as aborted", func() {

			Convey("it should be deactivated", func() {
				var err error
				So(AbortBuild(b.Id, evergreen.DefaultTaskActivator), ShouldBeNil)
				b, err = build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeFalse)
			})

			Convey("all abortable tasks for it should be aborted", func() {

				// insert two abortable tasks and one non-abortable task
				// for the correct build, and one abortable task for
				// a different build

				abortableOne := &task.Task{
					Id:      "abortableOne",
					BuildId: b.Id,
					Status:  evergreen.TaskStarted,
				}
				So(abortableOne.Insert(), ShouldBeNil)

				abortableTwo := &task.Task{
					Id:      "abortableTwo",
					BuildId: b.Id,
					Status:  evergreen.TaskDispatched,
				}
				So(abortableTwo.Insert(), ShouldBeNil)

				notAbortable := &task.Task{
					Id:      "notAbortable",
					BuildId: b.Id,
					Status:  evergreen.TaskSucceeded,
				}
				So(notAbortable.Insert(), ShouldBeNil)

				wrongBuildId := &task.Task{
					Id:      "wrongBuildId",
					BuildId: "blech",
					Status:  evergreen.TaskStarted,
				}
				So(wrongBuildId.Insert(), ShouldBeNil)

				// aborting the build should mark only the two abortable tasks
				// with the correct build id as aborted

				So(AbortBuild(b.Id, evergreen.DefaultTaskActivator), ShouldBeNil)

				abortedTasks, err := task.Find(task.ByAborted(true))
				So(err, ShouldBeNil)
				So(len(abortedTasks), ShouldEqual, 2)
				So(taskIdInSlice(abortedTasks, abortableOne.Id), ShouldBeTrue)
				So(taskIdInSlice(abortedTasks, abortableTwo.Id), ShouldBeTrue)
			})
		})
	})
}

func TestSetVersionActivation(t *testing.T) {
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection))

	vID := "abcdef"
	builds := []build.Build{
		{Id: "b0", Version: vID, Activated: true},
		{Id: "b1", Version: vID, Activated: true},
	}
	for _, build := range builds {
		require.NoError(t, build.Insert())
	}

	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Activated: true, Status: evergreen.TaskUndispatched},
		{Id: "t1", BuildId: "b1", Activated: true, Status: evergreen.TaskSucceeded},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	assert.NoError(t, SetVersionActivation(vID, false, "user"))
	builds, err := build.FindBuildsByVersions([]string{vID})
	require.NoError(t, err)
	require.Len(t, builds, 2)
	for _, b := range builds {
		assert.False(t, b.Activated)
	}

	t0, err := task.FindOneId(tasks[0].Id)
	require.NoError(t, err)
	assert.False(t, t0.Activated)

	t1, err := task.FindOneId(tasks[1].Id)
	require.NoError(t, err)
	assert.True(t, t1.Activated)
}

func TestBuildSetActivated(t *testing.T) {
	Convey("With a build", t, func() {

		require.NoError(t, db.ClearCollections(build.Collection, task.Collection),
			"Error clearing test collection")

		Convey("when changing the activated status of the build to true", func() {
			Convey("the activated status of the build and all undispatched"+
				" tasks that are part of it should be set", func() {

				user := "differentUser"

				b := &build.Build{
					Id:           "build",
					Activated:    true,
					BuildVariant: "bv",
				}
				So(b.Insert(), ShouldBeNil)

				// insert three tasks, with only one of them undispatched and
				// belonging to the correct build

				wrongBuildId := &task.Task{
					Id:        "wrongBuildId",
					BuildId:   "blech",
					Status:    evergreen.TaskUndispatched,
					Activated: true,
				}
				So(wrongBuildId.Insert(), ShouldBeNil)

				wrongStatus := &task.Task{
					Id:        "wrongStatus",
					BuildId:   b.Id,
					Status:    evergreen.TaskDispatched,
					Activated: true,
				}
				So(wrongStatus.Insert(), ShouldBeNil)

				matching := &task.Task{
					Id:        "matching",
					BuildId:   b.Id,
					Status:    evergreen.TaskUndispatched,
					Activated: true,
					DependsOn: []task.Dependency{
						{
							TaskId: "dependency",
							Status: evergreen.TaskSucceeded,
						},
					},
				}

				So(matching.Insert(), ShouldBeNil)

				differentUser := &task.Task{
					Id:          "differentUser",
					BuildId:     b.Id,
					Status:      evergreen.TaskUndispatched,
					Activated:   true,
					ActivatedBy: user,
				}
				So(differentUser.Insert(), ShouldBeNil)

				dependency := &task.Task{
					Id:           "dependency",
					BuildId:      "dependent_build",
					Status:       evergreen.TaskUndispatched,
					Activated:    false,
					DispatchTime: utility.ZeroTime,
				}
				So(dependency.Insert(), ShouldBeNil)

				canary := &task.Task{
					Id:           "canary",
					BuildId:      "dependent_build",
					Status:       evergreen.TaskUndispatched,
					Activated:    false,
					DispatchTime: utility.ZeroTime,
				}
				So(canary.Insert(), ShouldBeNil)

				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator), ShouldBeNil)
				// the build should have been updated in the db
				b, err := build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeFalse)
				So(b.ActivatedBy, ShouldEqual, evergreen.DefaultTaskActivator)

				// only the matching task should have been updated that has not been set by a user
				deactivatedTasks, err := task.Find(task.ByActivation(false))
				So(err, ShouldBeNil)
				So(len(deactivatedTasks), ShouldEqual, 3)
				So(deactivatedTasks[0].Id, ShouldEqual, matching.Id)

				// task with the different user activating should be activated with that user
				differentUserTask, err := task.FindOne(task.ById(differentUser.Id))
				So(err, ShouldBeNil)
				So(differentUserTask.Activated, ShouldBeTrue)
				So(differentUserTask.ActivatedBy, ShouldEqual, user)

				So(SetBuildActivation(b.Id, true, evergreen.DefaultTaskActivator), ShouldBeNil)
				activatedTasks, err := task.Find(task.ByActivation(true))
				So(err, ShouldBeNil)
				So(len(activatedTasks), ShouldEqual, 5)
			})

			Convey("if a build is activated by a user it should not be able to be deactivated by evergreen", func() {
				user := "differentUser"

				b := &build.Build{
					Id:           "anotherBuild",
					Activated:    true,
					BuildVariant: "bv",
				}

				So(b.Insert(), ShouldBeNil)

				matching := &task.Task{
					Id:        "matching",
					BuildId:   b.Id,
					Status:    evergreen.TaskUndispatched,
					Activated: false,
				}
				So(matching.Insert(), ShouldBeNil)

				matching2 := &task.Task{
					Id:        "matching2",
					BuildId:   b.Id,
					Status:    evergreen.TaskUndispatched,
					Activated: false,
				}
				So(matching2.Insert(), ShouldBeNil)

				// have a user set the build activation to true
				So(SetBuildActivation(b.Id, true, user), ShouldBeNil)

				// task with the different user activating should be activated with that user
				task1, err := task.FindOne(task.ById(matching.Id))
				So(err, ShouldBeNil)
				So(task1.Activated, ShouldBeTrue)
				So(task1.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task2, err := task.FindOne(task.ById(matching2.Id))
				So(err, ShouldBeNil)
				So(task2.Activated, ShouldBeTrue)
				So(task2.ActivatedBy, ShouldEqual, user)

				// refresh from the database and check again
				b, err = build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeTrue)
				So(b.ActivatedBy, ShouldEqual, user)

				// deactivate the task from evergreen and nothing should be deactivated.
				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator), ShouldBeNil)

				// refresh from the database and check again
				b, err = build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeTrue)
				So(b.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task1, err = task.FindOne(task.ById(matching.Id))
				So(err, ShouldBeNil)
				So(task1.Activated, ShouldBeTrue)
				So(task1.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task2, err = task.FindOne(task.ById(matching2.Id))
				So(err, ShouldBeNil)
				So(task2.Activated, ShouldBeTrue)
				So(task2.ActivatedBy, ShouldEqual, user)

			})
		})

	})
}

func TestBuildMarkStarted(t *testing.T) {
	Convey("With a build", t, func() {

		require.NoError(t, db.Clear(build.Collection), "Error clearing"+
			" '%v' collection", build.Collection)

		b := &build.Build{
			Id:     "build",
			Status: evergreen.BuildCreated,
		}
		So(b.Insert(), ShouldBeNil)

		Convey("marking it as started should update the status and"+
			" start time, both in memory and in the database", func() {

			startTime := time.Now()
			So(build.TryMarkStarted(b.Id, startTime), ShouldBeNil)

			// refresh from db and check again
			var err error
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			So(b.StartTime.Round(time.Second).Equal(
				startTime.Round(time.Second)), ShouldBeTrue)
		})
	})
}

func TestBuildMarkFinished(t *testing.T) {

	Convey("With a build", t, func() {

		require.NoError(t, db.Clear(build.Collection), "Error clearing"+
			" '%v' collection", build.Collection)

		startTime := time.Now()
		b := &build.Build{
			Id:        "build",
			StartTime: startTime,
		}
		So(b.Insert(), ShouldBeNil)

		Convey("marking it as finished should update the status,"+
			" finish time, and duration, both in memory and in the"+
			" database", func() {

			finishTime := time.Now()
			So(b.MarkFinished(evergreen.BuildSucceeded, finishTime), ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildSucceeded)
			So(b.FinishTime.Equal(finishTime), ShouldBeTrue)
			So(b.TimeTaken, ShouldEqual, finishTime.Sub(startTime))

			var err error
			// refresh from db and check again
			b, err = build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildSucceeded)
			So(b.FinishTime.Round(time.Second).Equal(
				finishTime.Round(time.Second)), ShouldBeTrue)
			So(b.TimeTaken, ShouldEqual, finishTime.Sub(startTime))
		})
	})
}

func TestCreateBuildFromVersion(t *testing.T) {

	Convey("When creating a build from a version", t, func() {

		require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection, build.Collection, task.Collection, ProjectAliasCollection),
			"Error clearing test collection")

		// the mock build variant we'll be using. runs all three tasks
		buildVar1 := parserBV{
			Name:        "buildVar",
			DisplayName: "Build Variant",
			RunOn:       []string{"arch"},
			Tasks: parserBVTaskUnits{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskD"},
			},
			DisplayTasks: []displayTask{
				displayTask{
					Name: "bv1DisplayTask1",
					ExecutionTasks: []string{
						"taskA",
						"taskB",
					},
				},
				displayTask{
					Name: "bv1DisplayTask2",
					ExecutionTasks: []string{
						"taskC",
						"taskD",
					},
				},
			},
		}
		buildVar2 := parserBV{
			Name:        "buildVar2",
			DisplayName: "Build Variant 2",
			Tasks: parserBVTaskUnits{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskE"},
			},
		}
		buildVar3 := parserBV{
			Name:        "buildVar3",
			DisplayName: "Build Variant 3",
			Tasks: parserBVTaskUnits{
				{
					// wait for the first BV's taskA to complete
					Name: "taskA",
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "taskA",
							Variant: &variantSelector{StringSelector: "buildVar"}},
						},
					},
				},
			},
		}

		pref := &ProjectRef{
			Id:         "projectId",
			Identifier: "projectName",
		}
		So(pref.Insert(), ShouldBeNil)

		alias := ProjectAlias{ProjectID: pref.Id, TaskTags: []string{"pull-requests"}, Alias: evergreen.GithubPRAlias,
			Variant: ".*"}
		So(alias.Upsert(), ShouldBeNil)
		mustHaveResults := true
		parserProject := &ParserProject{
			Identifier: "projectId",
			Tasks: []parserTask{
				{
					Name:      "taskA",
					Priority:  5,
					Tags:      []string{"tag1", "tag2"},
					DependsOn: nil,
				},
				{
					Name: "taskB",
					Tags: []string{"tag1", "tag2"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "taskA",
							Variant: &variantSelector{StringSelector: "buildVar"}},
						},
					},
				},
				{
					Name: "taskC",
					Tags: []string{"tag1", "tag2", "pull-requests"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "taskA"}},
						{TaskSelector: taskSelector{Name: "taskB"}},
					},
				},
				{
					Name: "taskD",
					Tags: []string{"tag1", "tag2"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: AllDependencies}},
					},
					MustHaveResults: &mustHaveResults,
				},
				{
					Name: "taskE",
					Tags: []string{"tag1", "tag2"},
					DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: AllDependencies,
							Variant: &variantSelector{StringSelector: AllVariants}},
						},
					},
				},
			},
			BuildVariants: []parserBV{buildVar1, buildVar2, buildVar3},
		}

		// the mock version we'll be using
		v := &Version{
			Id:                  "versionId",
			CreateTime:          time.Now(),
			Revision:            "foobar",
			RevisionOrderNumber: 500,
			Requester:           evergreen.RepotrackerVersionRequester,
			BuildVariants: []VersionBuildStatus{
				{
					BuildVariant:     buildVar1.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
				{
					BuildVariant:     buildVar2.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
				{
					BuildVariant:     buildVar3.Name,
					ActivationStatus: ActivationStatus{Activated: true},
				},
			},
		}
		So(v.Insert(), ShouldBeNil)

		project, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(project, ShouldNotBeNil)
		table := NewTaskIdTable(project, v, "", "")
		tt := table.ExecutionTasks
		dt := table.DisplayTasks

		Convey("the task id table should be well-formed", func() {
			So(tt.GetId("buildVar", "taskA"), ShouldNotEqual, "")
			So(tt.GetId("buildVar", "taskB"), ShouldNotEqual, "")
			So(tt.GetId("buildVar", "taskC"), ShouldNotEqual, "")
			So(tt.GetId("buildVar", "taskD"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskA"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskB"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskC"), ShouldNotEqual, "")
			So(tt.GetId("buildVar2", "taskE"), ShouldNotEqual, "")
			So(tt.GetId("buildVar3", "taskA"), ShouldNotEqual, "")
			So(dt.GetId("buildVar", "bv1DisplayTask1"), ShouldNotEqual, "")
			So(dt.GetId("buildVar", "bv1DisplayTask2"), ShouldNotEqual, "")

			Convey(`and incorrect GetId() calls should return ""`, func() {
				So(tt.GetId("buildVar", "taskF"), ShouldEqual, "")
				So(tt.GetId("buildVar2", "taskD"), ShouldEqual, "")
				So(tt.GetId("buildVar7", "taskA"), ShouldEqual, "")
				So(dt.GetId("buildVar7", "displayTask"), ShouldEqual, "")
			})
		})

		Convey("if a non-existent build variant is passed in, an error should be returned", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     "blecch",
				ActivateBuild: false,
				TaskNames:     []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldNotBeNil)
			So(build, ShouldBeNil)
			So(tasks, ShouldBeNil)
		})

		Convey("if no task names are passed in to be used, all of the default"+
			" tasks for the build variant should be created", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: false,
				TaskNames:     []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build, ShouldNotBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 6)

			args.BuildName = buildVar2.Name
			build, tasks, err = CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build, ShouldNotBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 4)
			So(len(tasks[0].Tags), ShouldEqual, 2)
		})

		Convey("if a non-empty list of task names is passed in, only the"+
			" specified tasks should be created", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: true,
				TaskNames:     []string{"taskA", "taskB"},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 2)
			for _, t := range tasks {
				So(t.Activated, ShouldBeTrue)
			}
		})

		Convey("if a non-empty list of TasksWithBatchTime is passed in, only the specified tasks should be activated", func() {
			batchTimeTasks := []string{"taskA", "taskB"}
			args := BuildCreateArgs{
				Project:            *project,
				Version:            *v,
				TaskIDs:            table,
				BuildName:          buildVar1.Name,
				ActivateBuild:      true,
				TaskNames:          []string{"taskA", "taskB", "taskC", "taskD"}, // excluding display tasks
				TasksWithBatchTime: batchTimeTasks,
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 4)
			for _, t := range tasks {
				if utility.StringSliceContains(batchTimeTasks, t.DisplayName) {
					So(t.Activated, ShouldBeFalse)
				} else {
					So(t.Activated, ShouldBeTrue)
				}
			}
		})

		Convey("if an alias is passed in, dependencies are also created", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar2.Name,
				ActivateBuild: false,
				Aliases:       []ProjectAlias{alias},
				TaskNames:     []string{"taskA", "taskB", "taskC"},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 3)
		})

		Convey("ensure distro is populated to tasks", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: false,
				TaskNames:     []string{"taskA", "taskB"},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			for _, t := range tasks {
				So(t.DistroId, ShouldEqual, "arch")
			}

		})

		Convey("the build should contain task caches that correspond exactly"+
			" to the tasks created", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar2.Name,
				ActivateBuild: false,
				TaskNames:     []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(tasks), ShouldEqual, 4)
			So(len(build.Tasks), ShouldEqual, 4)

			// make sure the task caches are correct.  they should also appear
			// in the same order that they appear in the project file
			So(build.Tasks[0].Id, ShouldContainSubstring, "taskA")
			So(build.Tasks[1].Id, ShouldContainSubstring, "taskB")
			So(build.Tasks[2].Id, ShouldContainSubstring, "taskC")
			So(build.Tasks[3].Id, ShouldContainSubstring, "taskE")
		})

		Convey("a task cache should not contain execution tasks that are part of a display task", func() {
			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: false,
				TaskNames:     []string{},
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			So(len(build.Tasks), ShouldEqual, 2)

			// make sure the task caches are correct
			So(build.Tasks[0].Id, ShouldContainSubstring, buildVar1.DisplayTasks[0].Name)
			So(build.Tasks[1].Id, ShouldContainSubstring, buildVar1.DisplayTasks[1].Name)

			// check the display tasks too
			So(len(tasks), ShouldEqual, 6)
			So(tasks[0].DisplayName, ShouldEqual, buildVar1.DisplayTasks[0].Name)
			So(tasks[0].DisplayOnly, ShouldBeTrue)
			So(len(tasks[0].ExecutionTasks), ShouldEqual, 2)
			So(tasks[1].DisplayName, ShouldEqual, buildVar1.DisplayTasks[1].Name)
			So(tasks[1].DisplayOnly, ShouldBeTrue)
		})
		Convey("all of the tasks created should have the dependencies"+
			"and priorities specified in the project", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: false,
				TaskNames:     []string{},
			}
			build, tasks1, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			args.BuildName = buildVar2.Name
			build, tasks2, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")
			args.BuildName = buildVar3.Name
			build, tasks3, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			So(tasks1.InsertUnordered(context.Background()), ShouldBeNil)
			So(tasks2.InsertUnordered(context.Background()), ShouldBeNil)
			So(tasks3.InsertUnordered(context.Background()), ShouldBeNil)
			dbTasks, err := task.Find(task.All.Sort([]string{task.DisplayNameKey, task.BuildVariantKey}))
			So(err, ShouldBeNil)
			So(len(dbTasks), ShouldEqual, 9)

			// taskA
			So(len(dbTasks[0].DependsOn), ShouldEqual, 0)
			So(len(dbTasks[1].DependsOn), ShouldEqual, 0)
			So(len(dbTasks[2].DependsOn), ShouldEqual, 1)
			So(dbTasks[0].Priority, ShouldEqual, 5)
			So(dbTasks[1].Priority, ShouldEqual, 5)
			So(dbTasks[2].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded}})

			// taskB
			So(dbTasks[3].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded}})
			So(dbTasks[4].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded}}) //cross-variant
			So(dbTasks[3].Priority, ShouldEqual, 0)
			So(dbTasks[4].Priority, ShouldEqual, 0) //default priority

			// taskC
			So(dbTasks[5].DependsOn, ShouldHaveLength, 2)
			So(dbTasks[5].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[5].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[3].Id, Status: evergreen.TaskSucceeded})

			So(dbTasks[6].DependsOn, ShouldHaveLength, 2)
			So(dbTasks[6].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[1].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[6].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[4].Id, Status: evergreen.TaskSucceeded})

			So(dbTasks[7].DependsOn, ShouldHaveLength, 3)
			So(dbTasks[7].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[0].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[7].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[3].Id, Status: evergreen.TaskSucceeded})
			So(dbTasks[7].DependsOn, ShouldContain, task.Dependency{TaskId: dbTasks[5].Id, Status: evergreen.TaskSucceeded})

			So(dbTasks[8].DisplayName, ShouldEqual, "taskE")
			So(len(dbTasks[8].DependsOn), ShouldEqual, 8)
		})

		Convey("all of the build's essential fields should be set correctly", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: false,
				TaskNames:     []string{},
			}
			build, _, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			// verify all the fields are set appropriately
			So(len(build.Tasks), ShouldEqual, 2)
			So(build.CreateTime.Truncate(time.Second), ShouldResemble,
				v.CreateTime.Truncate(time.Second))
			So(build.Activated, ShouldBeFalse)
			So(build.ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(build.Project, ShouldEqual, project.Identifier)
			So(build.Revision, ShouldEqual, v.Revision)
			So(build.Status, ShouldEqual, evergreen.BuildCreated)
			So(build.BuildVariant, ShouldEqual, buildVar1.Name)
			So(build.Version, ShouldEqual, v.Id)
			So(build.DisplayName, ShouldEqual, buildVar1.DisplayName)
			So(build.RevisionOrderNumber, ShouldEqual, v.RevisionOrderNumber)
			So(build.Requester, ShouldEqual, v.Requester)
		})

		Convey("all of the tasks' essential fields should be set correctly", func() {

			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: false,
				TaskNames:     []string{},
				SyncAtEndOpts: patch.SyncAtEndOptions{
					BuildVariants: []string{buildVar1.Name},
					Tasks:         []string{"taskA", "taskB"},
					VariantsTasks: []patch.VariantTasks{
						{
							Variant: buildVar1.Name,
							Tasks:   []string{"taskA", "taskB"},
						},
					},
				},
				TaskCreateTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
			}
			build, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			So(build.Id, ShouldNotEqual, "")

			So(len(tasks), ShouldEqual, 6)
			So(tasks[2].Id, ShouldNotEqual, "")
			So(tasks[2].Secret, ShouldNotEqual, "")
			So(tasks[2].DisplayName, ShouldEqual, "taskA")
			So(tasks[2].BuildId, ShouldEqual, build.Id)
			So(tasks[2].DistroId, ShouldEqual, "arch")
			So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[2].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
			So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[2].Activated, ShouldBeFalse)
			So(tasks[2].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[2].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[2].Requester, ShouldEqual, build.Requester)
			So(tasks[2].Version, ShouldEqual, v.Id)
			So(tasks[2].Revision, ShouldEqual, v.Revision)
			So(tasks[2].Project, ShouldEqual, project.Identifier)
			So(tasks[2].CanSync, ShouldBeTrue)

			So(tasks[3].Id, ShouldNotEqual, "")
			So(tasks[3].Secret, ShouldNotEqual, "")
			So(tasks[3].DisplayName, ShouldEqual, "taskB")
			So(tasks[3].BuildId, ShouldEqual, build.Id)
			So(tasks[3].DistroId, ShouldEqual, "arch")
			So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[3].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
			So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[3].Activated, ShouldBeFalse)
			So(tasks[3].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[3].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[3].Requester, ShouldEqual, build.Requester)
			So(tasks[3].Version, ShouldEqual, v.Id)
			So(tasks[3].Revision, ShouldEqual, v.Revision)
			So(tasks[3].Project, ShouldEqual, project.Identifier)
			So(tasks[3].CanSync, ShouldBeTrue)

			So(tasks[4].Id, ShouldNotEqual, "")
			So(tasks[4].Secret, ShouldNotEqual, "")
			So(tasks[4].DisplayName, ShouldEqual, "taskC")
			So(tasks[4].BuildId, ShouldEqual, build.Id)
			So(tasks[4].DistroId, ShouldEqual, "arch")
			So(tasks[4].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[4].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
			So(tasks[4].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[4].Activated, ShouldBeFalse)
			So(tasks[4].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[4].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[4].Requester, ShouldEqual, build.Requester)
			So(tasks[4].Version, ShouldEqual, v.Id)
			So(tasks[4].Revision, ShouldEqual, v.Revision)
			So(tasks[4].Project, ShouldEqual, project.Identifier)
			So(tasks[4].CanSync, ShouldBeFalse)

			So(tasks[5].Id, ShouldNotEqual, "")
			So(tasks[5].Secret, ShouldNotEqual, "")
			So(tasks[5].DisplayName, ShouldEqual, "taskD")
			So(tasks[5].BuildId, ShouldEqual, build.Id)
			So(tasks[5].DistroId, ShouldEqual, "arch")
			So(tasks[5].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[5].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
			So(tasks[5].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[5].Activated, ShouldBeFalse)
			So(tasks[5].ActivatedTime.Equal(utility.ZeroTime), ShouldBeTrue)
			So(tasks[5].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[5].Requester, ShouldEqual, build.Requester)
			So(tasks[5].Version, ShouldEqual, v.Id)
			So(tasks[5].Revision, ShouldEqual, v.Revision)
			So(tasks[5].Project, ShouldEqual, project.Identifier)
			So(tasks[5].CanSync, ShouldBeFalse)
		})

		Convey("if the activated flag is set, the build and all its tasks should be activated",
			func() {

				args := BuildCreateArgs{
					Project:        *project,
					Version:        *v,
					TaskIDs:        table,
					BuildName:      buildVar1.Name,
					ActivateBuild:  true,
					TaskNames:      []string{},
					TaskCreateTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
				}
				build, tasks, err := CreateBuildFromVersionNoInsert(args)
				So(err, ShouldBeNil)
				So(build.Id, ShouldNotEqual, "")
				So(build.Activated, ShouldBeTrue)
				So(build.ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)

				So(len(tasks), ShouldEqual, 6)
				So(tasks[2].Id, ShouldNotEqual, "")
				So(tasks[2].Secret, ShouldNotEqual, "")
				So(tasks[2].DisplayName, ShouldEqual, "taskA")
				So(tasks[2].BuildId, ShouldEqual, build.Id)
				So(tasks[2].DistroId, ShouldEqual, "arch")
				So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[2].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
				So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[2].Activated, ShouldBeTrue)
				So(tasks[2].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[2].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[2].Requester, ShouldEqual, build.Requester)
				So(tasks[2].Version, ShouldEqual, v.Id)
				So(tasks[2].Revision, ShouldEqual, v.Revision)
				So(tasks[2].Project, ShouldEqual, project.Identifier)

				So(tasks[3].Id, ShouldNotEqual, "")
				So(tasks[3].Secret, ShouldNotEqual, "")
				So(tasks[3].DisplayName, ShouldEqual, "taskB")
				So(tasks[3].BuildId, ShouldEqual, build.Id)
				So(tasks[3].DistroId, ShouldEqual, "arch")
				So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[3].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
				So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[3].Activated, ShouldBeTrue)
				So(tasks[3].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[3].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[3].Requester, ShouldEqual, build.Requester)
				So(tasks[3].Version, ShouldEqual, v.Id)
				So(tasks[3].Revision, ShouldEqual, v.Revision)
				So(tasks[3].Project, ShouldEqual, project.Identifier)

				So(tasks[4].Id, ShouldNotEqual, "")
				So(tasks[4].Secret, ShouldNotEqual, "")
				So(tasks[4].DisplayName, ShouldEqual, "taskC")
				So(tasks[4].BuildId, ShouldEqual, build.Id)
				So(tasks[4].DistroId, ShouldEqual, "arch")
				So(tasks[4].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[4].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
				So(tasks[4].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[4].Activated, ShouldBeTrue)
				So(tasks[4].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[4].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[4].Requester, ShouldEqual, build.Requester)
				So(tasks[4].Version, ShouldEqual, v.Id)
				So(tasks[4].Revision, ShouldEqual, v.Revision)
				So(tasks[4].Project, ShouldEqual, project.Identifier)

				So(tasks[5].Id, ShouldNotEqual, "")
				So(tasks[5].Secret, ShouldNotEqual, "")
				So(tasks[5].DisplayName, ShouldEqual, "taskD")
				So(tasks[5].BuildId, ShouldEqual, build.Id)
				So(tasks[5].DistroId, ShouldEqual, "arch")
				So(tasks[5].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[5].CreateTime.Equal(args.TaskCreateTime), ShouldBeTrue)
				So(tasks[5].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[5].Activated, ShouldBeTrue)
				So(tasks[5].ActivatedTime.Equal(utility.ZeroTime), ShouldBeFalse)
				So(tasks[5].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[5].Requester, ShouldEqual, build.Requester)
				So(tasks[5].Version, ShouldEqual, v.Id)
				So(tasks[5].Revision, ShouldEqual, v.Revision)
				So(tasks[5].Project, ShouldEqual, project.Identifier)
			})

		Convey("the 'must have test results' flag should be set", func() {
			args := BuildCreateArgs{
				Project:       *project,
				Version:       *v,
				TaskIDs:       table,
				BuildName:     buildVar1.Name,
				ActivateBuild: true,
				TaskNames:     []string{},
			}
			_, tasks, err := CreateBuildFromVersionNoInsert(args)
			So(err, ShouldBeNil)
			for _, t := range tasks {
				if t.DisplayName == "taskD" {
					So(t.MustHaveResults, ShouldBeTrue)
				}
			}
		})

	})
}

func TestCreateTaskGroup(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection), "Error clearing collection")
	projYml := `
  tasks:
  - name: example_task_1
  - name: example_task_2
    depends_on:
      - name: "example_task_1"
  - name: example_task_3
    depends_on:
      - name: "example_task_2"
  task_groups:
  - name: example_task_group
    max_hosts: 2
    priority: 50
    setup_group:
    - command: shell.exec
      params:
        script: "echo setup_group"
    teardown_group:
    - command: shell.exec
      params:
        script: "echo teardown_group"
    setup_task:
    - command: shell.exec
      params:
        script: "echo setup_group"
    teardown_task:
    - command: shell.exec
      params:
        script: "echo setup_group"
    tasks:
    - example_task_1
    - example_task_2
  buildvariants:
  - name: "bv"
    tasks:
    - name: example_task_group
    - name: example_task_3
  `
	proj := &Project{}
	_, err := LoadProjectInto([]byte(projYml), "test", proj)
	assert.NotNil(proj)
	assert.NoError(err)
	v := &Version{
		Id:                  "versionId",
		CreateTime:          time.Now(),
		Revision:            "foobar",
		RevisionOrderNumber: 500,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant:     "bv",
				ActivationStatus: ActivationStatus{Activated: false},
			},
		},
		Config: projYml,
	}
	table := NewTaskIdTable(proj, v, "", "")

	args := BuildCreateArgs{
		Project:       *proj,
		Version:       *v,
		TaskIDs:       table,
		BuildName:     "bv",
		ActivateBuild: true,
	}
	build, tasks, err := CreateBuildFromVersionNoInsert(args)
	assert.NoError(err)
	assert.Len(build.Tasks, 3)
	assert.Len(tasks, 3)
	assert.Equal("example_task_1", tasks[0].DisplayName)
	assert.Equal("example_task_group", tasks[0].TaskGroup)

	assert.Equal("example_task_2", tasks[1].DisplayName)
	assert.Contains(tasks[1].DependsOn[0].TaskId, "example_task_1")
	assert.Equal("example_task_group", tasks[1].TaskGroup)

	assert.Equal("example_task_3", tasks[2].DisplayName)
	assert.Empty(tasks[2].TaskGroup)
	assert.NotContains(tasks[2].TaskGroup, "example_task_group")
	assert.Contains(tasks[2].DependsOn[0].TaskId, "example_task_2")
}

func TestGetTaskIdTable(t *testing.T) {
	require.NoError(t, db.Clear(task.Collection))

	v := &Version{
		Id:         "v0",
		Revision:   "abcde",
		CreateTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
	}

	p := &Project{
		Identifier: "p0_id",
		BuildVariants: []BuildVariant{
			{
				Name: "bv0",
				Tasks: []BuildVariantTaskUnit{
					{
						Name: "t0",
					},
					{
						Name: "t1",
					},
				},
			},
		},
	}

	newPairs := TaskVariantPairs{
		ExecTasks: TVPairSet{
			// imagine t1 is a patch_optional task not included in newPairs
			{Variant: "bv0", TaskName: "t0"},
		},
	}
	existingTask := task.Task{Id: "t2", DisplayName: "existing_task", BuildVariant: "bv0", Version: v.Id}
	require.NoError(t, existingTask.Insert())

	tables, err := getTaskIdTables(v, p, newPairs, "p0")
	assert.NoError(t, err)
	assert.Len(t, tables.ExecutionTasks, 2)
	assert.Equal(t, "p0_bv0_t0_abcde_09_11_10_23_00_00", tables.ExecutionTasks.GetId("bv0", "t0"))
	assert.Equal(t, "t2", tables.ExecutionTasks.GetId("bv0", "existing_task"))
}

func TestMakeDeps(t *testing.T) {
	table := TaskIdTable{
		TVPair{TaskName: "t0", Variant: "bv0"}: "bv0_t0",
		TVPair{TaskName: "t1", Variant: "bv0"}: "bv0_t1",
		TVPair{TaskName: "t0", Variant: "bv1"}: "bv1_t0",
		TVPair{TaskName: "t1", Variant: "bv1"}: "bv1_t1",
	}
	thisTask := &task.Task{
		Id:           "bv1_t1",
		BuildVariant: "bv1",
		DisplayName:  "t1",
	}
	tSpec := BuildVariantTaskUnit{}

	t.Run("All tasks in all variants", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies, Variant: AllVariants},
		}

		deps := makeDeps(tSpec, thisTask, table)
		assert.Len(t, deps, 3)
		expectedIDs := []string{"bv0_t0", "bv0_t1", "bv1_t0"}
		for _, dep := range deps {
			assert.Contains(t, expectedIDs, dep.TaskId)
			assert.Equal(t, evergreen.TaskSucceeded, dep.Status)
		}
	})

	t.Run("All tasks in bv0", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies, Variant: "bv0"},
		}

		deps := makeDeps(tSpec, thisTask, table)
		assert.Len(t, deps, 2)
		expectedIDs := []string{"bv0_t0", "bv0_t1"}
		for _, dep := range deps {
			assert.Contains(t, expectedIDs, dep.TaskId)
			assert.Equal(t, evergreen.TaskSucceeded, dep.Status)
		}
	})

	t.Run("specific task", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: "t0", Variant: "bv0"},
		}

		deps := makeDeps(tSpec, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv0_t0", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskSucceeded, deps[0].Status)
	})

	t.Run("no duplicates", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies, Variant: AllVariants},
			{Name: "t0", Variant: "bv0"},
		}

		deps := makeDeps(tSpec, thisTask, table)
		assert.Len(t, deps, 3)
	})

	t.Run("non-default status", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: "t0", Variant: "bv0", Status: evergreen.TaskFailed},
		}

		deps := makeDeps(tSpec, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv0_t0", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskFailed, deps[0].Status)
	})

	t.Run("unspecified variant", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Name: AllDependencies},
		}

		deps := makeDeps(tSpec, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv1_t0", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskSucceeded, deps[0].Status)
	})

	t.Run("unspecified name", func(t *testing.T) {
		tSpec.DependsOn = []TaskUnitDependency{
			{Variant: AllVariants},
		}

		deps := makeDeps(tSpec, thisTask, table)
		assert.Len(t, deps, 1)
		assert.Equal(t, "bv0_t1", deps[0].TaskId)
		assert.Equal(t, evergreen.TaskSucceeded, deps[0].Status)
	})
}

func TestDeletingBuild(t *testing.T) {

	Convey("With a build", t, func() {

		require.NoError(t, db.Clear(build.Collection), "Error clearing"+
			" '%v' collection", build.Collection)

		b := &build.Build{
			Id: "build",
		}
		So(b.Insert(), ShouldBeNil)

		Convey("deleting it should remove it and all its associated"+
			" tasks from the database", func() {

			require.NoError(t, db.ClearCollections(task.Collection), "Error"+
				" clearing '%v' collection", task.Collection)

			// insert two tasks that are part of the build, and one that isn't
			matchingTaskOne := &task.Task{
				Id:      "matchingOne",
				BuildId: b.Id,
			}
			So(matchingTaskOne.Insert(), ShouldBeNil)

			matchingTaskTwo := &task.Task{
				Id:      "matchingTwo",
				BuildId: b.Id,
			}
			So(matchingTaskTwo.Insert(), ShouldBeNil)

			nonMatchingTask := &task.Task{
				Id:      "nonMatching",
				BuildId: "blech",
			}
			So(nonMatchingTask.Insert(), ShouldBeNil)
		})
	})
}

func TestSetNumDeps(t *testing.T) {
	Convey("setNumDeps correctly sets NumDependents for each task", t, func() {
		tasks := []*task.Task{
			{Id: "task1"},
			{
				Id:        "task2",
				DependsOn: []task.Dependency{{TaskId: "task1"}},
			},
			{
				Id:        "task3",
				DependsOn: []task.Dependency{{TaskId: "task1"}},
			},
			{
				Id:        "task4",
				DependsOn: []task.Dependency{{TaskId: "task2"}, {TaskId: "task3"}, {TaskId: "not_here"}},
			},
		}
		setNumDeps(tasks)
		So(len(tasks), ShouldEqual, 4)
		So(tasks[0].NumDependents, ShouldEqual, 3)
		So(tasks[1].NumDependents, ShouldEqual, 1)
		So(tasks[2].NumDependents, ShouldEqual, 1)
		So(tasks[3].NumDependents, ShouldEqual, 0)
	})
}

func TestSortTasks(t *testing.T) {
	Convey("sortTasks topologically sorts tasks by dependency", t, func() {
		Convey("for tasks with single dependencies", func() {
			tasks := []task.Task{
				{
					Id:          "idA",
					DisplayName: "A",
					DependsOn: []task.Dependency{
						{TaskId: "idB"},
					},
				},
				{
					Id:          "idB",
					DisplayName: "B",
					DependsOn: []task.Dependency{
						{TaskId: "idC"},
					},
				},
				{
					Id:          "idC",
					DisplayName: "C",
				},
			}

			sortedTasks := sortTasks(tasks)
			So(len(sortedTasks), ShouldEqual, 3)
			So(sortedTasks[0].DisplayName, ShouldEqual, "C")
			So(sortedTasks[1].DisplayName, ShouldEqual, "B")
			So(sortedTasks[2].DisplayName, ShouldEqual, "A")
		})
		Convey("for tasks with multiplie dependencies", func() {
			tasks := []task.Task{
				{
					Id:          "idA",
					DisplayName: "A",
					DependsOn: []task.Dependency{
						{TaskId: "idB"},
						{TaskId: "idC"},
					},
				},
				{
					Id:          "idB",
					DisplayName: "B",
					DependsOn: []task.Dependency{
						{TaskId: "idC"},
					},
				},
				{
					Id:          "idC",
					DisplayName: "C",
				},
			}

			sortedTasks := sortTasks(tasks)
			So(len(sortedTasks), ShouldEqual, 3)
			So(sortedTasks[0].DisplayName, ShouldEqual, "C")
			So(sortedTasks[1].DisplayName, ShouldEqual, "B")
			So(sortedTasks[2].DisplayName, ShouldEqual, "A")
		})
	})

	Convey("grouping tasks by common dependencies and sorting alphabetically within groups", t, func() {
		tasks := []task.Task{
			{
				Id:          "idA",
				DisplayName: "A",
				DependsOn: []task.Dependency{
					{TaskId: "idE"},
				},
			},
			{
				Id:          "idB",
				DisplayName: "B",
				DependsOn: []task.Dependency{
					{TaskId: "idD"},
				},
			},
			{
				Id:          "idC",
				DisplayName: "C",
				DependsOn: []task.Dependency{
					{TaskId: "idD"},
				},
			},
			{
				Id:          "idD",
				DisplayName: "D",
			},
			{
				Id:          "idE",
				DisplayName: "E",
			},
		}

		sortedTasks := sortTasks(tasks)
		So(len(sortedTasks), ShouldEqual, 5)
		So(sortedTasks[0].DisplayName, ShouldEqual, "D")
		So(sortedTasks[1].DisplayName, ShouldEqual, "E")
		So(sortedTasks[2].DisplayName, ShouldEqual, "B")
		So(sortedTasks[3].DisplayName, ShouldEqual, "C")
		So(sortedTasks[4].DisplayName, ShouldEqual, "A")
	})

	Convey("special-casing tasks with cross-variant dependencies to the far right", t, func() {
		tasks := []task.Task{
			{
				Id:          "idA",
				DisplayName: "A",
				DependsOn: []task.Dependency{
					{TaskId: "idB"},
					{TaskId: "idC"},
				},
			},
			{
				Id:          "idB",
				DisplayName: "B",
				DependsOn: []task.Dependency{
					{TaskId: "idC"},
				},
			},
			{
				Id:          "idC",
				DisplayName: "C",
				DependsOn: []task.Dependency{
					{TaskId: "cross-variant"},
				},
			},
			{
				Id:          "idD",
				DisplayName: "D",
			},
		}

		sortedTasks := sortTasks(tasks)
		So(len(sortedTasks), ShouldEqual, 4)
		So(sortedTasks[0].DisplayName, ShouldEqual, "D")
		So(sortedTasks[1].DisplayName, ShouldEqual, "C")
		So(sortedTasks[2].DisplayName, ShouldEqual, "B")
		So(sortedTasks[3].DisplayName, ShouldEqual, "A")

		Convey("when there are cross-variant dependencies on different tasks", func() {

			tasks = append(tasks,
				task.Task{
					Id:          "idE",
					DisplayName: "E",
					DependsOn: []task.Dependency{
						{TaskId: "cross-variant2"},
					}},
				task.Task{
					Id:          "idF",
					DisplayName: "F",
					DependsOn: []task.Dependency{
						{TaskId: "idE"},
					}})
			sortedTasks = sortTasks(tasks)
			So(len(sortedTasks), ShouldEqual, 6)
			So(sortedTasks[0].DisplayName, ShouldEqual, "D")
			So(sortedTasks[1].DisplayName, ShouldEqual, "C")
			So(sortedTasks[2].DisplayName, ShouldEqual, "E")
			So(sortedTasks[3].DisplayName, ShouldEqual, "B")
			So(sortedTasks[4].DisplayName, ShouldEqual, "F")
			So(sortedTasks[5].DisplayName, ShouldEqual, "A")
		})
	})
}

func TestVersionRestart(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(resetTaskData())

	// test that restarting a version restarts its tasks
	taskIds := []string{"task1", "task3", "task4"}
	assert.NoError(RestartVersion("version", taskIds, false, "test"))
	tasks, err := task.Find(task.ByIds(taskIds))
	assert.NoError(err)
	assert.NotEmpty(tasks)
	for _, t := range tasks {
		assert.Equal(evergreen.TaskUndispatched, t.Status)
		assert.True(t.Activated)
	}
	dbVersion, err := VersionFindOneId("version")
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, dbVersion.Status)

	// test that aborting in-progress tasks works correctly
	assert.NoError(resetTaskData())
	taskIds = []string{"task2"}
	assert.NoError(RestartVersion("version", taskIds, true, "test"))
	dbTask, err := task.FindOne(task.ById("task2"))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.True(dbTask.Aborted)
	assert.Equal("test", dbTask.AbortInfo.User)
	assert.Equal(evergreen.TaskDispatched, dbTask.Status)
	assert.True(dbTask.ResetWhenFinished)
	dbVersion, err = VersionFindOneId("version")
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, dbVersion.Status)

	// test that not aborting in-progress tasks does not reset them
	assert.NoError(resetTaskData())
	taskIds = []string{"task2"}
	assert.NoError(RestartVersion("version", taskIds, false, "test"))
	dbTask, err = task.FindOne(task.ById("task2"))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.False(dbTask.Aborted)
	assert.Equal(evergreen.TaskDispatched, dbTask.Status)
	dbVersion, err = VersionFindOneId("version")
	assert.NoError(err)
	assert.Equal(evergreen.VersionStarted, dbVersion.Status)
}

func TestDisplayTaskRestart(t *testing.T) {
	assert := assert.New(t)
	displayTasks := []string{"displayTask"}
	allTasks := []string{"displayTask", "task5", "task6"}

	// test restarting a version
	assert.NoError(resetTaskData())
	assert.NoError(RestartVersion("version", displayTasks, false, "test"))
	tasks, err := task.FindAll(task.ByIds(allTasks))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}

	// test restarting a build
	assert.NoError(resetTaskData())
	assert.NoError(RestartBuild("build3", displayTasks, false, "test"))
	tasks, err = task.FindAll(task.ByIds(allTasks))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}

	// test that restarting a task correctly resets the task and archives it
	assert.NoError(resetTaskData())
	assert.NoError(resetTask("displayTask", "caller", false))
	archivedTasks, err := task.FindOldWithDisplayTasks(task.All)
	assert.NoError(err)
	assert.Len(archivedTasks, 3)
	foundDisplayTask := false
	for _, ot := range archivedTasks {
		if ot.OldTaskId == "displayTask" {
			foundDisplayTask = true
		}
	}
	assert.True(foundDisplayTask)
	tasks, err = task.FindAll(task.ByIds(allTasks))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}

	// test that execution tasks cannot be restarted
	assert.NoError(resetTaskData())
	assert.Error(TryResetTask("task5", "", "", nil))

	// trying to restart execution tasks should restart the entire display task, if it's done
	assert.NoError(resetTaskData())
	assert.NoError(RestartVersion("version", allTasks, false, "test"))
	tasks, err = task.FindAll(task.ByIds(allTasks))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}
}

func resetTaskData() error {
	if err := db.ClearCollections(build.Collection, task.Collection, VersionCollection, task.OldCollection); err != nil {
		return err
	}
	v := &Version{
		Id: "version",
	}
	if err := v.Insert(); err != nil {
		return err
	}
	build1 := &build.Build{
		Id:      "build1",
		Version: v.Id,
	}
	build2 := &build.Build{
		Id:      "build2",
		Version: v.Id,
	}
	build3 := &build.Build{
		Id:      "build3",
		Version: v.Id,
	}
	if err := build1.Insert(); err != nil {
		return err
	}
	if err := build2.Insert(); err != nil {
		return err
	}
	if err := build3.Insert(); err != nil {
		return err
	}
	task1 := &task.Task{
		Id:          "task1",
		DisplayName: "task1",
		BuildId:     build1.Id,
		Version:     v.Id,
		Status:      evergreen.TaskSucceeded,
	}
	if err := task1.Insert(); err != nil {
		return err
	}
	task2 := &task.Task{
		Id:          "task2",
		DisplayName: "task2",
		BuildId:     build1.Id,
		Version:     v.Id,
		Status:      evergreen.TaskDispatched,
	}
	if err := task2.Insert(); err != nil {
		return err
	}
	task3 := &task.Task{
		Id:          "task3",
		DisplayName: "task3",
		BuildId:     build2.Id,
		Version:     v.Id,
		Status:      evergreen.TaskSucceeded,
	}
	if err := task3.Insert(); err != nil {
		return err
	}
	task4 := &task.Task{
		Id:          "task4",
		DisplayName: "task4",
		BuildId:     build2.Id,
		Version:     v.Id,
		Status:      evergreen.TaskFailed,
	}
	if err := task4.Insert(); err != nil {
		return err
	}
	task5 := &task.Task{
		Id:           "task5",
		DisplayName:  "task5",
		BuildId:      build3.Id,
		Version:      v.Id,
		Status:       evergreen.TaskSucceeded,
		DispatchTime: time.Now(),
	}
	if err := task5.Insert(); err != nil {
		return err
	}
	task6 := &task.Task{
		Id:           "task6",
		DisplayName:  "task6",
		BuildId:      build3.Id,
		Version:      v.Id,
		Status:       evergreen.TaskFailed,
		DispatchTime: time.Now(),
	}
	if err := task6.Insert(); err != nil {
		return err
	}
	displayTask := &task.Task{
		Id:             "displayTask",
		DisplayName:    "displayTask",
		BuildId:        build3.Id,
		Version:        v.Id,
		DisplayOnly:    true,
		ExecutionTasks: []string{task5.Id, task6.Id},
		Status:         evergreen.TaskFailed,
		DispatchTime:   time.Now(),
	}
	if err := displayTask.Insert(); err != nil {
		return err
	}
	if err := UpdateDisplayTask(displayTask); err != nil {
		return err
	}
	return nil
}

func TestCreateTasksFromGroup(t *testing.T) {
	assert := assert.New(t)
	in := BuildVariantTaskUnit{
		Name:            "name",
		IsGroup:         true,
		GroupName:       "task_group",
		Priority:        0,
		DependsOn:       []TaskUnitDependency{{Name: "new_dependency"}},
		RunOn:           []string{},
		ExecTimeoutSecs: 0,
	}
	p := &Project{
		Tasks: []ProjectTask{
			{
				Name:      "first_task",
				DependsOn: []TaskUnitDependency{{Name: "dependency"}},
			},
			{
				Name: "second_task",
			},
		},
		TaskGroups: []TaskGroup{
			{
				Name:  "name",
				Tasks: []string{"first_task", "second_task"},
			},
		},
	}
	bvts := CreateTasksFromGroup(in, p)
	assert.Equal("new_dependency", bvts[0].DependsOn[0].Name)
	assert.Equal("new_dependency", bvts[1].DependsOn[0].Name)
}

func TestMarkAsDispatched(t *testing.T) {

	var (
		taskId       string
		hostId       string
		agentVersion string
		buildId      string
		distroId     string
		taskDoc      *task.Task
		b            *build.Build
	)

	Convey("With a task", t, func() {

		taskId = "t1"
		hostId = "h1"
		agentVersion = "a1"
		buildId = "b1"
		distroId = "d1"

		taskDoc = &task.Task{
			Id:      taskId,
			BuildId: buildId,
		}

		b = &build.Build{Id: buildId}

		require.NoError(t, db.ClearCollections(task.Collection, build.Collection), "Error clearing test collections")

		So(taskDoc.Insert(), ShouldBeNil)
		So(b.Insert(), ShouldBeNil)

		Convey("when marking the task as dispatched, the fields for"+
			" the task, the host it is on, and the build it is a part of"+
			" should be set to reflect this", func() {

			// mark the task as dispatched
			So(taskDoc.MarkAsDispatched(hostId, distroId, agentVersion, time.Now()), ShouldBeNil)

			// make sure the task's fields were updated, both in ©memory and
			// in the db
			So(taskDoc.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(taskDoc.Status, ShouldEqual, evergreen.TaskDispatched)
			So(taskDoc.HostId, ShouldEqual, hostId)
			So(taskDoc.AgentVersion, ShouldEqual, agentVersion)
			So(taskDoc.LastHeartbeat, ShouldResemble, taskDoc.DispatchTime)
			taskDoc, err := task.FindOne(task.ById(taskId))
			So(err, ShouldBeNil)
			So(taskDoc.DispatchTime, ShouldNotResemble, time.Unix(0, 0))
			So(taskDoc.Status, ShouldEqual, evergreen.TaskDispatched)
			So(taskDoc.HostId, ShouldEqual, hostId)
			So(taskDoc.AgentVersion, ShouldEqual, agentVersion)
			So(taskDoc.LastHeartbeat, ShouldResemble, taskDoc.DispatchTime)

		})

	})

}

func TestShouldSyncTask(t *testing.T) {
	for testName, testCase := range map[string]struct {
		syncVTs    []patch.VariantTasks
		bv         string
		task       string
		shouldSync bool
	}{
		"MatchesTaskInBV": {
			syncVTs: []patch.VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1"},
				},
			},
			bv:         "bv1",
			task:       "t1",
			shouldSync: true,
		},
		"DoesNotMatchDisplayTaskName": {
			syncVTs: []patch.VariantTasks{
				{
					Variant: "bv1",
					DisplayTasks: []patch.DisplayTask{
						{
							Name: "dt1",
						},
					},
				},
			},
			bv:         "bv1",
			task:       "dt1",
			shouldSync: false,
		},
		"MatchesExecutionTaskWithinDisplayTask": {
			syncVTs: []patch.VariantTasks{
				{
					Variant: "bv1",
					DisplayTasks: []patch.DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"et1"},
						},
					},
				},
			},
			bv:         "bv1",
			task:       "et1",
			shouldSync: true,
		},
		"NoMatchForTask": {
			syncVTs: []patch.VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1 ", "et1"},
					DisplayTasks: []patch.DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"et1"},
						},
					},
				},
			},
			bv:         "bv1",
			task:       "t2",
			shouldSync: false,
		},
		"NoMatchForBuildVariant": {
			syncVTs: []patch.VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1 ", "et1"},
					DisplayTasks: []patch.DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"et1"},
						},
					},
				},
			},
			bv:         "bv1",
			task:       "t2",
			shouldSync: false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			shouldSync := shouldSyncTask(testCase.syncVTs, testCase.bv, testCase.task)
			assert.Equal(t, testCase.shouldSync, shouldSync)
		})
	}
}

func TestSetTaskActivationForBuildsActivated(t *testing.T) {
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection))
	build := build.Build{Id: "b0"}
	require.NoError(t, build.Insert())

	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Status: evergreen.TaskUndispatched},
		{Id: "t1", BuildId: "b1", Status: evergreen.TaskUndispatched},
		{Id: "t2", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t1"}}, Status: evergreen.TaskUndispatched},
		{Id: "t3", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t0"}}, Status: evergreen.TaskUndispatched},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	// t0 should still be activated because it's a dependency of a task that is being activated
	assert.NoError(t, setTaskActivationForBuilds([]string{"b0"}, true, []string{"t0"}, ""))

	dbTasks, err := task.FindAll(db.Q{})
	require.NoError(t, err)
	require.Len(t, dbTasks, 4)
	for _, task := range dbTasks {
		assert.True(t, task.Activated)
	}
}

func TestSetTaskActivationForBuildsWithIgnoreTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection))

	build := build.Build{Id: "b0"}
	require.NoError(t, build.Insert())

	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Status: evergreen.TaskUndispatched},
		{Id: "t1", BuildId: "b1", Status: evergreen.TaskUndispatched},
		{Id: "t2", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t1"}}, Status: evergreen.TaskUndispatched},
		{Id: "t3", BuildId: "b0", DependsOn: []task.Dependency{{TaskId: "t0"}}, Status: evergreen.TaskUndispatched},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	assert.NoError(t, setTaskActivationForBuilds([]string{"b0"}, true, []string{"t3"}, ""))

	dbTasks, err := task.FindAll(db.Q{})
	require.NoError(t, err)
	require.Len(t, dbTasks, 4)
	for _, dbTask := range dbTasks {
		if dbTask.Id == "t3" {
			assert.False(t, dbTask.Activated)
			continue
		}
		assert.True(t, dbTask.Activated)
	}
}

func TestSetTaskActivationForBuildsDeactivated(t *testing.T) {
	require.NoError(t, db.ClearCollections(build.Collection, task.Collection))
	build := build.Build{Id: "b0"}
	require.NoError(t, build.Insert())

	tasks := []task.Task{
		{Id: "t0", Activated: true, BuildId: "b0", Status: evergreen.TaskUndispatched},
		{Id: "t1", Activated: true, BuildId: "b1", DependsOn: []task.Dependency{{TaskId: "t2"}}, Status: evergreen.TaskUndispatched},
		{Id: "t2", Activated: true, BuildId: "b0", Status: evergreen.TaskUndispatched},
	}

	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	// ignore tasks is ignored for deactivating
	assert.NoError(t, setTaskActivationForBuilds([]string{"b0"}, false, []string{"t0", "t1", "t2"}, ""))

	dbTasks, err := task.FindAll(db.Q{})
	require.NoError(t, err)
	require.Len(t, dbTasks, 3)
	for _, task := range dbTasks {
		assert.False(t, task.Activated)
	}
}

func TestRecomputeNumDependents(t *testing.T) {
	assert.NoError(t, db.Clear(task.Collection))
	t1 := task.Task{
		Id: "1",
		DependsOn: []task.Dependency{
			{TaskId: "2"},
		},
		Version: "v1",
	}
	assert.NoError(t, t1.Insert())
	t2 := task.Task{
		Id: "2",
		DependsOn: []task.Dependency{
			{TaskId: "3"},
		},
		Version: "v1",
	}
	assert.NoError(t, t2.Insert())
	t3 := task.Task{
		Id: "3",
		DependsOn: []task.Dependency{
			{TaskId: "4"},
		},
		Version: "v1",
	}
	assert.NoError(t, t3.Insert())
	t4 := task.Task{
		Id: "4",
		DependsOn: []task.Dependency{
			{TaskId: "5"},
		},
		Version: "v1",
	}
	assert.NoError(t, t4.Insert())
	t5 := task.Task{
		Id:      "5",
		Version: "v1",
	}
	assert.NoError(t, t5.Insert())

	assert.NoError(t, RecomputeNumDependents(t3))
	tasks, err := task.Find(task.ByVersion(t1.Version))
	assert.NoError(t, err)
	for i, dbTask := range tasks {
		assert.Equal(t, i, dbTask.NumDependents)
	}

	assert.NoError(t, RecomputeNumDependents(t5))
	tasks, err = task.Find(task.ByVersion(t1.Version))
	assert.NoError(t, err)
	for i, dbTask := range tasks {
		assert.Equal(t, i, dbTask.NumDependents)
	}

	t6 := task.Task{
		Id: "6",
		DependsOn: []task.Dependency{
			{TaskId: "8"},
		},
		Version: "v2",
	}
	assert.NoError(t, t6.Insert())
	t7 := task.Task{
		Id: "7",
		DependsOn: []task.Dependency{
			{TaskId: "8"},
		},
		Version: "v2",
	}
	assert.NoError(t, t7.Insert())
	t8 := task.Task{
		Id: "8",
		DependsOn: []task.Dependency{
			{TaskId: "9"},
		},
		Version: "v2",
	}
	assert.NoError(t, t8.Insert())
	t9 := task.Task{
		Id:      "9",
		Version: "v2",
	}
	assert.NoError(t, t9.Insert())

	assert.NoError(t, RecomputeNumDependents(t8))
	tasks, err = task.Find(task.ByVersion(t6.Version))
	assert.NoError(t, err)
	expected := map[string]int{
		"6": 0,
		"7": 0,
		"8": 2,
		"9": 3,
	}
	for _, dbTask := range tasks {
		assert.Equal(t, expected[dbTask.Id], dbTask.NumDependents)
	}
}
