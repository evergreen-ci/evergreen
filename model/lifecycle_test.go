package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
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

			So(SetBuildPriority(b.Id, 42), ShouldBeNil)

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
			b := &build.Build{
				Id: "build",
				Tasks: []build.TaskCache{
					{
						Id:        "task1",
						Status:    evergreen.TaskSucceeded,
						Activated: true,
					},
					{
						Id:        "task2",
						Status:    evergreen.TaskDispatched,
						Activated: true,
					},
				},
			}
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
			So(b.Status, ShouldEqual, evergreen.BuildCreated)
			So(b.Activated, ShouldEqual, true)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[1].Status, ShouldEqual, evergreen.TaskDispatched)
			So(b.Tasks[0].Activated, ShouldEqual, true)
			So(b.Tasks[1].Activated, ShouldEqual, true)
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
			b := &build.Build{
				Id: "build",
				Tasks: []build.TaskCache{
					{
						Id:        "task1",
						Status:    evergreen.TaskSucceeded,
						Activated: true,
					},
					{
						Id:        "task2",
						Status:    evergreen.TaskDispatched,
						Activated: true,
					},
					{
						Id:        "task3",
						Status:    evergreen.TaskDispatched,
						Activated: true,
					},
					{
						Id:        "task4",
						Status:    evergreen.TaskDispatched,
						Activated: true,
					},
				},
			}
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
			So(b.Status, ShouldEqual, evergreen.BuildCreated)
			So(b.Activated, ShouldEqual, true)
			So(b.Tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[3].Status, ShouldEqual, evergreen.TaskDispatched)
			So(b.Tasks[2].Activated, ShouldEqual, true)
			So(b.Tasks[3].Activated, ShouldEqual, true)
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
					BuildVariant: "bv",
					Activated:    true,
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
					DispatchTime: util.ZeroTime,
				}
				So(dependency.Insert(), ShouldBeNil)

				canary := &task.Task{
					Id:           "canary",
					BuildId:      "dependent_build",
					Status:       evergreen.TaskUndispatched,
					Activated:    false,
					DispatchTime: util.ZeroTime,
				}
				So(canary.Insert(), ShouldBeNil)

				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator, false), ShouldBeNil)
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

				So(SetBuildActivation(b.Id, true, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				activatedTasks, err := task.Find(task.ByActivation(true))
				So(err, ShouldBeNil)
				So(len(activatedTasks), ShouldEqual, 5)
			})

			Convey("all of the undispatched task caches within the build"+
				" should be updated, both in memory and in the"+
				" database", func() {

				b := &build.Build{
					Id:           "build",
					Activated:    true,
					BuildVariant: "foo",
					Tasks: []build.TaskCache{
						{
							Id:        "tc1",
							Status:    evergreen.TaskUndispatched,
							Activated: true,
						},
						{
							Id:        "tc2",
							Status:    evergreen.TaskDispatched,
							Activated: true,
						},
						{
							Id:        "tc3",
							Status:    evergreen.TaskUndispatched,
							Activated: true,
						},
						{
							Id:        "tc4",
							Status:    evergreen.TaskUndispatched,
							Activated: true,
						},
					},
				}
				So(b.Insert(), ShouldBeNil)

				t1 := &task.Task{Id: "tc1", DisplayName: "tc1", BuildId: b.Id, Status: evergreen.TaskUndispatched, Activated: true}
				t2 := &task.Task{Id: "tc2", DisplayName: "tc2", BuildId: b.Id, Status: evergreen.TaskDispatched, Activated: true}
				t3 := &task.Task{Id: "tc3", DisplayName: "tc3", BuildId: b.Id, Status: evergreen.TaskUndispatched, Activated: true}
				t4 := &task.Task{Id: "tc4", DisplayName: "tc4", BuildId: b.Id, Status: evergreen.TaskUndispatched, Activated: true, ActivatedBy: "anotherUser"}
				So(t1.Insert(), ShouldBeNil)
				So(t2.Insert(), ShouldBeNil)
				So(t3.Insert(), ShouldBeNil)
				So(t4.Insert(), ShouldBeNil)

				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				// refresh from the database and check again
				b, err := build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeFalse)
				So(b.Tasks[0].Activated, ShouldBeFalse)
				So(b.Tasks[1].Activated, ShouldBeTrue)
				So(b.Tasks[2].Activated, ShouldBeFalse)
				So(b.Tasks[3].Activated, ShouldBeTrue)
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
				So(SetBuildActivation(b.Id, true, user, false), ShouldBeNil)

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
				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator, false), ShouldBeNil)

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

		require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection, build.Collection, task.Collection),
			"Error clearing test collection")

		// the mock build variant we'll be using. runs all three tasks
		buildVar1 := BuildVariant{
			Name:        "buildVar",
			DisplayName: "Build Variant",
			RunOn:       []string{"arch"},
			Tasks: []BuildVariantTaskUnit{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskD"},
			},
			DisplayTasks: []DisplayTask{
				DisplayTask{
					Name: "bv1DisplayTask1",
					ExecutionTasks: []string{
						"taskA",
						"taskB",
					},
				},
				DisplayTask{
					Name: "bv1DisplayTask2",
					ExecutionTasks: []string{
						"taskC",
						"taskD",
					},
				},
			},
		}
		buildVar2 := BuildVariant{
			Name:        "buildVar2",
			DisplayName: "Build Variant 2",
			Tasks: []BuildVariantTaskUnit{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskE"},
			},
		}
		buildVar3 := BuildVariant{
			Name:        "buildVar3",
			DisplayName: "Build Variant 3",
			Tasks: []BuildVariantTaskUnit{
				{
					// wait for the first BV's taskA to complete
					Name:      "taskA",
					DependsOn: []TaskUnitDependency{{Name: "taskA", Variant: "buildVar"}},
				},
			},
		}

		pref := &ProjectRef{
			Identifier: "projectName",
		}
		So(pref.Insert(), ShouldBeNil)

		project := &Project{
			Identifier: "projectName",
			Tasks: []ProjectTask{
				{
					Name:      "taskA",
					Priority:  5,
					Tags:      []string{"tag1", "tag2"},
					DependsOn: []TaskUnitDependency{},
				},
				{
					Name:      "taskB",
					Tags:      []string{"tag1", "tag2"},
					DependsOn: []TaskUnitDependency{{Name: "taskA", Variant: "buildVar"}},
				},
				{
					Name: "taskC",
					Tags: []string{"tag1", "tag2"},
					DependsOn: []TaskUnitDependency{
						{Name: "taskA"},
						{Name: "taskB"},
					},
				},
				{
					Name:      "taskD",
					Tags:      []string{"tag1", "tag2"},
					DependsOn: []TaskUnitDependency{{Name: AllDependencies}},
				},
				{
					Name: "taskE",
					Tags: []string{"tag1", "tag2"},
					DependsOn: []TaskUnitDependency{
						{
							Name:    AllDependencies,
							Variant: AllVariants,
						},
					},
				},
			},
			BuildVariants: []BuildVariant{buildVar1, buildVar2, buildVar3},
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
					BuildVariant: buildVar1.Name,
					Activated:    false,
				},
				{
					BuildVariant: buildVar2.Name,
					Activated:    false,
				},
				{
					BuildVariant: buildVar3.Name,
					Activated:    false,
				},
			},
			Config: `
buildvariants:
- name: "buildVar"
  run_on:
   - "arch"
  tasks:
   - name: "taskA"
   - name: "taskB"
   - name: "taskC"
   - name: "taskD"
`,
		}
		So(v.Insert(), ShouldBeNil)

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
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: "blecch",
				Activated: false,
				TaskNames: []string{},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldNotBeNil)
			So(buildId, ShouldEqual, "")

		})

		Convey("if no task names are passed in to be used, all of the default"+
			" tasks for the build variant should be created", func() {

			args := BuildCreateArgs{
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar1.Name,
				Activated: false,
				TaskNames: []string{},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")
			args.BuildName = buildVar2.Name
			buildId2, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId2, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := task.Find(task.All)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 8)
			So(len(tasks[0].Tags), ShouldEqual, 2)

		})

		Convey("if a non-empty list of task names is passed in, only the"+
			" specified tasks should be created", func() {

			args := BuildCreateArgs{
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar1.Name,
				Activated: false,
				TaskNames: []string{"taskA", "taskB"},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := task.Find(task.All)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 2)

		})

		Convey("ensure distro is populated to tasks", func() {

			args := BuildCreateArgs{
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar1.Name,
				Activated: false,
				TaskNames: []string{"taskA", "taskB"},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := task.Find(task.All)
			So(err, ShouldBeNil)

			for _, t := range tasks {
				So(t.DistroId, ShouldEqual, "arch")
			}

		})

		Convey("the build should contain task caches that correspond exactly"+
			" to the tasks created", func() {

			args := BuildCreateArgs{
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar2.Name,
				Activated: false,
				TaskNames: []string{},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := task.Find(task.All)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 4)

			// find the build from the db
			b, err := build.FindOne(build.ById(buildId))
			So(err, ShouldBeNil)
			So(len(b.Tasks), ShouldEqual, 4)

			// make sure the task caches are correct.  they should also appear
			// in the same order that they appear in the project file
			So(b.Tasks[0].Id, ShouldNotEqual, "")
			So(b.Tasks[0].DisplayName, ShouldEqual, "taskA")
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[1].Id, ShouldNotEqual, "")
			So(b.Tasks[1].DisplayName, ShouldEqual, "taskB")
			So(b.Tasks[1].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[2].Id, ShouldNotEqual, "")
			So(b.Tasks[2].DisplayName, ShouldEqual, "taskC")
			So(b.Tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[3].Id, ShouldNotEqual, "")
			So(b.Tasks[3].DisplayName, ShouldEqual, "taskE")
			So(b.Tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
		})

		Convey("a task cache should not contain execution tasks that are part of a display task", func() {

			args := BuildCreateArgs{
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar1.Name,
				Activated: false,
				TaskNames: []string{},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the execution tasks, make sure they were all created
			tasks, err := task.Find(task.All)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 4)

			// find the build from the db
			b, err := build.FindOne(build.ById(buildId))
			So(err, ShouldBeNil)
			So(len(b.Tasks), ShouldEqual, 2)

			// make sure the task caches are correct
			So(b.Tasks[0].Id, ShouldNotEqual, "")
			So(b.Tasks[0].DisplayName, ShouldEqual, buildVar1.DisplayTasks[0].Name)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[1].Id, ShouldNotEqual, "")
			So(b.Tasks[1].DisplayName, ShouldEqual, buildVar1.DisplayTasks[1].Name)
			So(b.Tasks[1].Status, ShouldEqual, evergreen.TaskUndispatched)

			// check the display tasks too
			tasks, err = task.FindWithDisplayTasks(task.ByBuildId(buildId))
			So(err, ShouldBeNil)
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
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar1.Name,
				Activated: false,
				TaskNames: []string{},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")
			args.BuildName = buildVar2.Name
			buildId2, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId2, ShouldNotEqual, "")
			args.BuildName = buildVar3.Name
			buildId3, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId3, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := task.Find(task.All.Sort([]string{task.DisplayNameKey, task.BuildVariantKey}))
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 9)

			// taskA
			So(len(tasks[0].DependsOn), ShouldEqual, 0)
			So(len(tasks[1].DependsOn), ShouldEqual, 0)
			So(len(tasks[2].DependsOn), ShouldEqual, 1)
			So(tasks[0].Priority, ShouldEqual, 5)
			So(tasks[1].Priority, ShouldEqual, 5)
			So(tasks[2].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: tasks[0].Id, Status: evergreen.TaskSucceeded}})

			// taskB
			So(tasks[3].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: tasks[0].Id, Status: evergreen.TaskSucceeded}})
			So(tasks[4].DependsOn, ShouldResemble,
				[]task.Dependency{{TaskId: tasks[0].Id, Status: evergreen.TaskSucceeded}}) //cross-variant
			So(tasks[3].Priority, ShouldEqual, 0)
			So(tasks[4].Priority, ShouldEqual, 0) //default priority

			// taskC
			So(tasks[5].DependsOn, ShouldResemble,
				[]task.Dependency{
					{TaskId: tasks[0].Id, Status: evergreen.TaskSucceeded},
					{TaskId: tasks[3].Id, Status: evergreen.TaskSucceeded}})
			So(tasks[6].DependsOn, ShouldResemble,
				[]task.Dependency{
					{TaskId: tasks[1].Id, Status: evergreen.TaskSucceeded},
					{TaskId: tasks[4].Id, Status: evergreen.TaskSucceeded}})
			So(tasks[7].DependsOn, ShouldResemble,
				[]task.Dependency{
					{TaskId: tasks[0].Id, Status: evergreen.TaskSucceeded},
					{TaskId: tasks[3].Id, Status: evergreen.TaskSucceeded},
					{TaskId: tasks[5].Id, Status: evergreen.TaskSucceeded}})
			So(tasks[8].DisplayName, ShouldEqual, "taskE")
			So(len(tasks[8].DependsOn), ShouldEqual, 8)
		})

		Convey("all of the build's essential fields should be set correctly", func() {

			args := BuildCreateArgs{
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar1.Name,
				Activated: false,
				TaskNames: []string{},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the build from the db
			b, err := build.FindOne(build.ById(buildId))
			So(err, ShouldBeNil)

			// verify all the fields are set appropriately
			So(len(b.Tasks), ShouldEqual, 2)
			So(b.CreateTime.Truncate(time.Second), ShouldResemble,
				v.CreateTime.Truncate(time.Second))
			So(b.Activated, ShouldBeFalse)
			So(b.ActivatedTime.Equal(util.ZeroTime), ShouldBeTrue)
			So(b.Project, ShouldEqual, project.Identifier)
			So(b.Revision, ShouldEqual, v.Revision)
			So(b.Status, ShouldEqual, evergreen.BuildCreated)
			So(b.BuildVariant, ShouldEqual, buildVar1.Name)
			So(b.Version, ShouldEqual, v.Id)
			So(b.DisplayName, ShouldEqual, buildVar1.DisplayName)
			So(b.RevisionOrderNumber, ShouldEqual, v.RevisionOrderNumber)
			So(b.Requester, ShouldEqual, v.Requester)
		})

		Convey("all of the tasks' essential fields should be set correctly", func() {

			args := BuildCreateArgs{
				Project:   *project,
				Version:   *v,
				TaskIDs:   table,
				BuildName: buildVar1.Name,
				Activated: false,
				TaskNames: []string{},
			}
			buildId, err := CreateBuildFromVersion(args)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the build from the db
			b, err := build.FindOne(build.ById(buildId))
			So(err, ShouldBeNil)

			// find the tasks, make sure they were all created
			tasks, err := task.Find(task.All.Sort([]string{task.DisplayNameKey}))
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 4)

			So(tasks[0].Id, ShouldNotEqual, "")
			So(tasks[0].Secret, ShouldNotEqual, "")
			So(tasks[0].DisplayName, ShouldEqual, "taskA")
			So(tasks[0].BuildId, ShouldEqual, buildId)
			So(tasks[0].DistroId, ShouldEqual, "arch")
			So(tasks[0].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[0].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[0].Activated, ShouldBeFalse)
			So(tasks[0].ActivatedTime.Equal(util.ZeroTime), ShouldBeTrue)
			So(tasks[0].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[0].Requester, ShouldEqual, b.Requester)
			So(tasks[0].Version, ShouldEqual, v.Id)
			So(tasks[0].Revision, ShouldEqual, v.Revision)
			So(tasks[0].Project, ShouldEqual, project.Identifier)

			So(tasks[1].Id, ShouldNotEqual, "")
			So(tasks[1].Secret, ShouldNotEqual, "")
			So(tasks[1].DisplayName, ShouldEqual, "taskB")
			So(tasks[1].BuildId, ShouldEqual, buildId)
			So(tasks[1].DistroId, ShouldEqual, "arch")
			So(tasks[1].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[1].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[1].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[1].Activated, ShouldBeFalse)
			So(tasks[1].ActivatedTime.Equal(util.ZeroTime), ShouldBeTrue)
			So(tasks[1].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[1].Requester, ShouldEqual, b.Requester)
			So(tasks[1].Version, ShouldEqual, v.Id)
			So(tasks[1].Revision, ShouldEqual, v.Revision)
			So(tasks[1].Project, ShouldEqual, project.Identifier)

			So(tasks[2].Id, ShouldNotEqual, "")
			So(tasks[2].Secret, ShouldNotEqual, "")
			So(tasks[2].DisplayName, ShouldEqual, "taskC")
			So(tasks[2].BuildId, ShouldEqual, buildId)
			So(tasks[2].DistroId, ShouldEqual, "arch")
			So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[2].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[2].Activated, ShouldBeFalse)
			So(tasks[2].ActivatedTime.Equal(util.ZeroTime), ShouldBeTrue)
			So(tasks[2].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[2].Requester, ShouldEqual, b.Requester)
			So(tasks[2].Version, ShouldEqual, v.Id)
			So(tasks[2].Revision, ShouldEqual, v.Revision)
			So(tasks[2].Project, ShouldEqual, project.Identifier)

			So(tasks[3].Id, ShouldNotEqual, "")
			So(tasks[3].Secret, ShouldNotEqual, "")
			So(tasks[3].DisplayName, ShouldEqual, "taskD")
			So(tasks[3].BuildId, ShouldEqual, buildId)
			So(tasks[3].DistroId, ShouldEqual, "arch")
			So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[3].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[3].Activated, ShouldBeFalse)
			So(tasks[3].ActivatedTime.Equal(util.ZeroTime), ShouldBeTrue)
			So(tasks[3].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[3].Requester, ShouldEqual, b.Requester)
			So(tasks[3].Version, ShouldEqual, v.Id)
			So(tasks[3].Revision, ShouldEqual, v.Revision)
			So(tasks[3].Project, ShouldEqual, project.Identifier)
		})

		Convey("if the activated flag is set, the build and all its tasks should be activated",
			func() {

				args := BuildCreateArgs{
					Project:   *project,
					Version:   *v,
					TaskIDs:   table,
					BuildName: buildVar1.Name,
					Activated: true,
					TaskNames: []string{},
				}
				buildId, err := CreateBuildFromVersion(args)
				So(err, ShouldBeNil)
				So(buildId, ShouldNotEqual, "")

				// find the build from the db
				b, err := build.FindOne(build.ById(buildId))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeTrue)
				So(b.ActivatedTime.Equal(util.ZeroTime), ShouldBeFalse)

				// find the tasks, make sure they were all created
				tasks, err := task.Find(task.All.Sort([]string{task.DisplayNameKey}))
				So(err, ShouldBeNil)
				So(len(tasks), ShouldEqual, 4)

				So(tasks[0].Id, ShouldNotEqual, "")
				So(tasks[0].Secret, ShouldNotEqual, "")
				So(tasks[0].DisplayName, ShouldEqual, "taskA")
				So(tasks[0].BuildId, ShouldEqual, buildId)
				So(tasks[0].DistroId, ShouldEqual, "arch")
				So(tasks[0].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[0].CreateTime.Truncate(time.Second), ShouldResemble,
					b.CreateTime.Truncate(time.Second))
				So(tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[0].Activated, ShouldBeTrue)
				So(tasks[0].ActivatedTime.Equal(util.ZeroTime), ShouldBeFalse)
				So(tasks[0].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
				So(tasks[0].Requester, ShouldEqual, b.Requester)
				So(tasks[0].Version, ShouldEqual, v.Id)
				So(tasks[0].Revision, ShouldEqual, v.Revision)
				So(tasks[0].Project, ShouldEqual, project.Identifier)

				So(tasks[1].Id, ShouldNotEqual, "")
				So(tasks[1].Secret, ShouldNotEqual, "")
				So(tasks[1].DisplayName, ShouldEqual, "taskB")
				So(tasks[1].BuildId, ShouldEqual, buildId)
				So(tasks[1].DistroId, ShouldEqual, "arch")
				So(tasks[1].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[1].CreateTime.Truncate(time.Second), ShouldResemble,
					b.CreateTime.Truncate(time.Second))
				So(tasks[1].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[1].Activated, ShouldBeTrue)
				So(tasks[1].ActivatedTime.Equal(util.ZeroTime), ShouldBeFalse)
				So(tasks[1].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
				So(tasks[1].Requester, ShouldEqual, b.Requester)
				So(tasks[1].Version, ShouldEqual, v.Id)
				So(tasks[1].Revision, ShouldEqual, v.Revision)
				So(tasks[1].Project, ShouldEqual, project.Identifier)

				So(tasks[2].Id, ShouldNotEqual, "")
				So(tasks[2].Secret, ShouldNotEqual, "")
				So(tasks[2].DisplayName, ShouldEqual, "taskC")
				So(tasks[2].BuildId, ShouldEqual, buildId)
				So(tasks[2].DistroId, ShouldEqual, "arch")
				So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[2].CreateTime.Truncate(time.Second), ShouldResemble,
					b.CreateTime.Truncate(time.Second))
				So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[2].Activated, ShouldBeTrue)
				So(tasks[2].ActivatedTime.Equal(util.ZeroTime), ShouldBeFalse)
				So(tasks[2].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
				So(tasks[2].Requester, ShouldEqual, b.Requester)
				So(tasks[2].Version, ShouldEqual, v.Id)
				So(tasks[2].Revision, ShouldEqual, v.Revision)
				So(tasks[2].Project, ShouldEqual, project.Identifier)

				So(tasks[3].Id, ShouldNotEqual, "")
				So(tasks[3].Secret, ShouldNotEqual, "")
				So(tasks[3].DisplayName, ShouldEqual, "taskD")
				So(tasks[3].BuildId, ShouldEqual, buildId)
				So(tasks[3].DistroId, ShouldEqual, "arch")
				So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[3].CreateTime.Truncate(time.Second), ShouldResemble,
					b.CreateTime.Truncate(time.Second))
				So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[3].Activated, ShouldBeTrue)
				So(tasks[3].ActivatedTime.Equal(util.ZeroTime), ShouldBeFalse)
				So(tasks[3].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
				So(tasks[3].Requester, ShouldEqual, b.Requester)
				So(tasks[3].Version, ShouldEqual, v.Id)
				So(tasks[3].Revision, ShouldEqual, v.Revision)
				So(tasks[3].Project, ShouldEqual, project.Identifier)
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
	proj, errs := projectFromYAML([]byte(projYml))
	proj.Identifier = "test"
	assert.NotNil(proj)
	assert.Empty(errs)
	v := &Version{
		Id:                  "versionId",
		CreateTime:          time.Now(),
		Revision:            "foobar",
		RevisionOrderNumber: 500,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "bv",
				Activated:    false,
			},
		},
		Config: projYml,
	}
	table := NewTaskIdTable(proj, v, "", "")

	args := BuildCreateArgs{
		Project:   *proj,
		Version:   *v,
		TaskIDs:   table,
		BuildName: "bv",
		Activated: true,
	}
	buildId, err := CreateBuildFromVersion(args)
	assert.NoError(err)
	dbBuild, err := build.FindOne(build.ById(buildId))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbBuild.Tasks, 3)
	dbTasks, err := task.Find(task.ByBuildId(buildId))
	assert.NoError(err)
	assert.Len(dbTasks, 3)
	for _, t := range dbTasks {
		if t.DisplayName == "example_task_1" {
			assert.Equal("example_task_group", t.TaskGroup)
		}
		if t.DisplayName == "example_task_2" {
			assert.Contains(t.DependsOn[0].TaskId, "example_task_1")
			assert.Equal("example_task_group", t.TaskGroup)
		}
		if t.DisplayName == "example_task_3" {
			assert.Empty(t.TaskGroup)
			assert.NotContains(t.TaskGroup, "example_task_group")
			assert.Contains(t.DependsOn[0].TaskId, "example_task_2")
		}
	}
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

	// test that aborting in-progress tasks works correctly
	assert.NoError(resetTaskData())
	taskIds = []string{"task2"}
	assert.NoError(RestartVersion("version", taskIds, true, "test"))
	dbTask, err := task.FindOne(task.ById("task2"))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.True(dbTask.Aborted)
	assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	assert.True(dbTask.Activated)

	// test that not aborting in-progress tasks does not reset them
	assert.NoError(resetTaskData())
	taskIds = []string{"task2"}
	assert.NoError(RestartVersion("version", taskIds, false, "test"))
	dbTask, err = task.FindOne(task.ById("task2"))
	assert.NoError(err)
	assert.NotNil(dbTask)
	assert.False(dbTask.Aborted)
	assert.Equal(evergreen.TaskDispatched, dbTask.Status)
}

func TestDisplayTaskRestart(t *testing.T) {
	assert := assert.New(t)
	displayTasks := []string{"displayTask"}
	allTasks := []string{"displayTask", "task5", "task6"}

	// test restarting a version
	assert.NoError(resetTaskData())
	assert.NoError(RestartVersion("version", displayTasks, false, "test"))
	tasks, err := task.FindWithDisplayTasks(task.ByIds(allTasks))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}

	// test restarting a build
	assert.NoError(resetTaskData())
	assert.NoError(RestartBuild("build3", displayTasks, false, "test"))
	tasks, err = task.FindWithDisplayTasks(task.ByIds(allTasks))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}

	// test that restarting a task correctly resets the task and archives it
	assert.NoError(resetTaskData())
	assert.NoError(resetTask("displayTask", "caller"))
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
	tasks, err = task.FindWithDisplayTasks(task.ByIds(allTasks))
	assert.NoError(err)
	assert.Len(tasks, 3)
	for _, dbTask := range tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status, dbTask.Id)
		assert.True(dbTask.Activated, dbTask.Id)
	}
	b, err := build.FindOne(build.ById("build3"))
	assert.NoError(err)
	assert.NotNil(b)
	for _, dbTask := range b.Tasks {
		assert.Equal(evergreen.TaskUndispatched, dbTask.Status)
	}

	// test that execution tasks cannot be restarted
	assert.NoError(resetTaskData())
	assert.Error(TryResetTask("task5", "", "", nil))
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
		Tasks: []build.TaskCache{
			{
				Id:        "task1",
				Status:    evergreen.TaskSucceeded,
				Activated: true,
			},
			{
				Id:        "task2",
				Status:    evergreen.TaskDispatched,
				Activated: true,
			},
		},
	}
	build2 := &build.Build{
		Id:      "build2",
		Version: v.Id,
		Tasks: []build.TaskCache{
			{
				Id:        "task3",
				Status:    evergreen.TaskSucceeded,
				Activated: true,
			},
			{
				Id:        "task4",
				Status:    evergreen.TaskFailed,
				Activated: true,
			},
		},
	}
	build3 := &build.Build{
		Id:      "build3",
		Version: v.Id,
		Tasks: []build.TaskCache{
			{
				Id:        "displayTask",
				Status:    evergreen.TaskFailed,
				Activated: true,
			},
		},
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

func TestSkipOnPatch(t *testing.T) {
	assert := assert.New(t)
	falseTmp := false
	bvt := BuildVariantTaskUnit{Patchable: &falseTmp}

	b := &build.Build{Requester: evergreen.RepotrackerVersionRequester}
	assert.False(b.IsPatchBuild() && bvt.SkipOnPatchBuild())

	b.Requester = evergreen.PatchVersionRequester
	assert.True(b.IsPatchBuild() && bvt.SkipOnPatchBuild())

	b.Requester = evergreen.GithubPRRequester
	assert.True(b.IsPatchBuild() && bvt.SkipOnPatchBuild())

	b.Requester = evergreen.MergeTestRequester
	assert.True(b.IsPatchBuild() && bvt.SkipOnPatchBuild())

	bvt.Patchable = nil
	assert.False(b.IsPatchBuild() && bvt.SkipOnPatchBuild())
}

func TestSkipOnNonPatch(t *testing.T) {
	assert := assert.New(t)
	boolTmp := true
	bvt := BuildVariantTaskUnit{PatchOnly: &boolTmp}
	b := &build.Build{Requester: evergreen.RepotrackerVersionRequester}
	assert.True(!b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())

	b.Requester = evergreen.PatchVersionRequester
	assert.False(!b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())

	b.Requester = evergreen.GithubPRRequester
	assert.False(!b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())
	bvt.PatchOnly = nil
	assert.False(!b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())

	b.Requester = evergreen.GithubPRRequester
	assert.False(b.IsPatchBuild() && bvt.SkipOnPatchBuild())
}

func TestCreateTasksFromGroup(t *testing.T) {
	assert := assert.New(t)
	in := BuildVariantTaskUnit{
		Name:            "name",
		IsGroup:         true,
		GroupName:       "task_group",
		Priority:        0,
		DependsOn:       []TaskUnitDependency{{Name: "new_dependency"}},
		Requires:        nil,
		Distros:         []string{},
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

		require.NoError(t, db.ClearCollections(task.Collection, build.Collection), "Error clearing test collections")

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
