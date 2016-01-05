package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"testing"
	"time"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
}

func taskIdInSlice(tasks []Task, id string) bool {
	for _, task := range tasks {
		if task.Id == id {
			return true
		}
	}
	return false
}

func TestBuildSetPriority(t *testing.T) {

	Convey("With a build", t, func() {

		testutil.HandleTestingErr(db.ClearCollections(build.Collection, TasksCollection), t,
			"Error clearing test collection")

		b := &build.Build{
			Id: "build",
		}
		So(b.Insert(), ShouldBeNil)

		taskOne := &Task{Id: "taskOne", BuildId: b.Id}
		So(taskOne.Insert(), ShouldBeNil)

		taskTwo := &Task{Id: "taskTwo", BuildId: b.Id}
		So(taskTwo.Insert(), ShouldBeNil)

		taskThree := &Task{Id: "taskThree", BuildId: b.Id}
		So(taskThree.Insert(), ShouldBeNil)

		Convey("setting its priority should update the priority"+
			" of all its tasks in the database", func() {

			So(SetBuildPriority(b.Id, 42), ShouldBeNil)

			tasks, err := FindAllTasks(
				bson.M{
					TaskBuildIdKey: b.Id,
				},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
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

		testutil.HandleTestingErr(db.ClearCollections(build.Collection, TasksCollection), t,
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

		Convey("with task abort should update the status of"+
			" non in-progress tasks and abort in-progress ones", func() {

			taskOne := &Task{
				Id:          "task1",
				DisplayName: "task1",
				BuildId:     b.Id,
				Status:      evergreen.TaskSucceeded,
			}
			So(taskOne.Insert(), ShouldBeNil)

			taskTwo := &Task{
				Id:          "task2",
				DisplayName: "task2",
				BuildId:     b.Id,
				Status:      evergreen.TaskDispatched,
			}
			So(taskTwo.Insert(), ShouldBeNil)

			So(RestartBuild(b.Id, []string{"task1", "task2"}, true, evergreen.DefaultTaskActivator), ShouldBeNil)
			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildCreated)
			So(b.Activated, ShouldEqual, true)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[1].Status, ShouldEqual, evergreen.TaskDispatched)
			So(b.Tasks[0].Activated, ShouldEqual, true)
			So(b.Tasks[1].Activated, ShouldEqual, true)
			taskOne, err = FindTask("task1")
			So(err, ShouldBeNil)
			So(taskOne.Status, ShouldEqual, evergreen.TaskUndispatched)
			taskTwo, err = FindTask("task2")
			So(err, ShouldBeNil)
			So(taskTwo.Aborted, ShouldEqual, true)
		})

		Convey("without task abort should update the status"+
			" of only those build tasks not in-progress", func() {

			taskThree := &Task{
				Id:          "task3",
				DisplayName: "task3",
				BuildId:     b.Id,
				Status:      evergreen.TaskSucceeded,
			}
			So(taskThree.Insert(), ShouldBeNil)

			taskFour := &Task{
				Id:          "task4",
				DisplayName: "task4",
				BuildId:     b.Id,
				Status:      evergreen.TaskDispatched,
			}
			So(taskFour.Insert(), ShouldBeNil)

			So(RestartBuild(b.Id, []string{"task3", "task4"}, false, evergreen.DefaultTaskActivator), ShouldBeNil)
			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildCreated)
			So(b.Activated, ShouldEqual, true)
			So(b.Tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(b.Tasks[3].Status, ShouldEqual, evergreen.TaskDispatched)
			So(b.Tasks[2].Activated, ShouldEqual, true)
			So(b.Tasks[3].Activated, ShouldEqual, true)
			taskThree, err = FindTask("task3")
			So(err, ShouldBeNil)
			So(taskThree.Status, ShouldEqual, evergreen.TaskUndispatched)
			taskFour, err = FindTask("task4")
			So(err, ShouldBeNil)
			So(taskFour.Aborted, ShouldEqual, false)
			So(taskFour.Status, ShouldEqual, evergreen.TaskDispatched)
		})

	})
}

func TestBuildMarkAborted(t *testing.T) {
	Convey("With a build", t, func() {

		testutil.HandleTestingErr(db.ClearCollections(build.Collection, TasksCollection, version.Collection), t,
			"Error clearing test collection")

		v := &version.Version{
			Id: "v",
			BuildVariants: []version.BuildStatus{
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
				So(AbortBuild(b.Id, evergreen.DefaultTaskActivator), ShouldBeNil)
				b, err := build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeFalse)
			})

			Convey("all abortable tasks for it should be aborted", func() {

				// insert two abortable tasks and one non-abortable task
				// for the correct build, and one abortable task for
				// a different build

				abortableOne := &Task{
					Id:      "abortableOne",
					BuildId: b.Id,
					Status:  evergreen.TaskStarted,
				}
				So(abortableOne.Insert(), ShouldBeNil)

				abortableTwo := &Task{
					Id:      "abortableTwo",
					BuildId: b.Id,
					Status:  evergreen.TaskDispatched,
				}
				So(abortableTwo.Insert(), ShouldBeNil)

				notAbortable := &Task{
					Id:      "notAbortable",
					BuildId: b.Id,
					Status:  evergreen.TaskSucceeded,
				}
				So(notAbortable.Insert(), ShouldBeNil)

				wrongBuildId := &Task{
					Id:      "wrongBuildId",
					BuildId: "blech",
					Status:  evergreen.TaskStarted,
				}
				So(wrongBuildId.Insert(), ShouldBeNil)

				// aborting the build should mark only the two abortable tasks
				// with the correct build id as aborted

				So(AbortBuild(b.Id, evergreen.DefaultTaskActivator), ShouldBeNil)

				abortedTasks, err := FindAllTasks(
					bson.M{
						TaskAbortedKey: true,
					},
					db.NoProjection,
					db.NoSort,
					db.NoSkip,
					db.NoLimit,
				)

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

		testutil.HandleTestingErr(db.ClearCollections(build.Collection, TasksCollection), t,
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

				wrongBuildId := &Task{
					Id:        "wrongBuildId",
					BuildId:   "blech",
					Status:    evergreen.TaskUndispatched,
					Activated: true,
				}
				So(wrongBuildId.Insert(), ShouldBeNil)

				wrongStatus := &Task{
					Id:        "wrongStatus",
					BuildId:   b.Id,
					Status:    evergreen.TaskDispatched,
					Activated: true,
				}
				So(wrongStatus.Insert(), ShouldBeNil)

				matching := &Task{
					Id:        "matching",
					BuildId:   b.Id,
					Status:    evergreen.TaskUndispatched,
					Activated: true,
				}

				So(matching.Insert(), ShouldBeNil)

				differentUser := &Task{
					Id:          "differentUser",
					BuildId:     b.Id,
					Status:      evergreen.TaskUndispatched,
					Activated:   true,
					ActivatedBy: user,
				}

				So(differentUser.Insert(), ShouldBeNil)

				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator), ShouldBeNil)
				// the build should have been updated in the db
				b, err := build.FindOne(build.ById(b.Id))
				So(err, ShouldBeNil)
				So(b.Activated, ShouldBeFalse)
				So(b.ActivatedBy, ShouldEqual, evergreen.DefaultTaskActivator)

				// only the matching task should have been updated that has not been set by a user
				deactivatedTasks, err := FindAllTasks(
					bson.M{TaskActivatedKey: false},
					db.NoProjection,
					db.NoSort,
					db.NoSkip,
					db.NoLimit,
				)
				So(err, ShouldBeNil)
				So(len(deactivatedTasks), ShouldEqual, 1)
				So(deactivatedTasks[0].Id, ShouldEqual, matching.Id)

				// task with the different user activating should be activated with that user
				differentUserTask, err := FindTask(differentUser.Id)
				So(err, ShouldBeNil)
				So(differentUserTask.Activated, ShouldBeTrue)
				So(differentUserTask.ActivatedBy, ShouldEqual, user)

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

				t1 := &Task{Id: "tc1", DisplayName: "tc1", BuildId: b.Id, Status: evergreen.TaskUndispatched, Activated: true}
				t2 := &Task{Id: "tc2", DisplayName: "tc2", BuildId: b.Id, Status: evergreen.TaskDispatched, Activated: true}
				t3 := &Task{Id: "tc3", DisplayName: "tc3", BuildId: b.Id, Status: evergreen.TaskUndispatched, Activated: true}
				t4 := &Task{Id: "tc4", DisplayName: "tc4", BuildId: b.Id, Status: evergreen.TaskUndispatched, Activated: true, ActivatedBy: "anotherUser"}
				So(t1.Insert(), ShouldBeNil)
				So(t2.Insert(), ShouldBeNil)
				So(t3.Insert(), ShouldBeNil)
				So(t4.Insert(), ShouldBeNil)

				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator), ShouldBeNil)
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

				matching := &Task{
					Id:        "matching",
					BuildId:   b.Id,
					Status:    evergreen.TaskUndispatched,
					Activated: false,
				}
				So(matching.Insert(), ShouldBeNil)

				matching2 := &Task{
					Id:        "matching2",
					BuildId:   b.Id,
					Status:    evergreen.TaskUndispatched,
					Activated: false,
				}
				So(matching2.Insert(), ShouldBeNil)

				// have a user set the build activation to true
				So(SetBuildActivation(b.Id, true, user), ShouldBeNil)

				// task with the different user activating should be activated with that user
				task1, err := FindTask(matching.Id)
				So(err, ShouldBeNil)
				So(task1.Activated, ShouldBeTrue)
				So(task1.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task2, err := FindTask(matching2.Id)
				So(err, ShouldBeNil)
				So(task2.Activated, ShouldBeTrue)
				So(task2.ActivatedBy, ShouldEqual, user)

				// refresh from the database and check again
				b, err = build.FindOne(build.ById(b.Id))
				So(b.Activated, ShouldBeTrue)
				So(b.ActivatedBy, ShouldEqual, user)

				// deactivate the task from evergreen and nothing should be deactivated.
				So(SetBuildActivation(b.Id, false, evergreen.DefaultTaskActivator), ShouldBeNil)

				// refresh from the database and check again
				b, err = build.FindOne(build.ById(b.Id))
				So(b.Activated, ShouldBeTrue)
				So(b.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task1, err = FindTask(matching.Id)
				So(err, ShouldBeNil)
				So(task1.Activated, ShouldBeTrue)
				So(task1.ActivatedBy, ShouldEqual, user)

				// task with the different user activating should be activated with that user
				task2, err = FindTask(matching2.Id)
				So(err, ShouldBeNil)
				So(task2.Activated, ShouldBeTrue)
				So(task2.ActivatedBy, ShouldEqual, user)

			})
		})

	})
}

func TestBuildMarkStarted(t *testing.T) {

	Convey("With a build", t, func() {

		testutil.HandleTestingErr(db.Clear(build.Collection), t, "Error clearing"+
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
			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			So(b.StartTime.Round(time.Second).Equal(
				startTime.Round(time.Second)), ShouldBeTrue)
		})
	})
}

func TestBuildMarkFinished(t *testing.T) {

	Convey("With a build", t, func() {

		testutil.HandleTestingErr(db.Clear(build.Collection), t, "Error clearing"+
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

			// refresh from db and check again

			b, err := build.FindOne(build.ById(b.Id))
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

		testutil.HandleTestingErr(db.ClearCollections(build.Collection, TasksCollection), t,
			"Error clearing test collection")

		// the mock build variant we'll be using. runs all three tasks
		buildVar1 := BuildVariant{
			Name:        "buildVar",
			DisplayName: "Build Variant",
			Tasks: []BuildVariantTask{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskD"},
			},
		}
		buildVar2 := BuildVariant{
			Name:        "buildVar2",
			DisplayName: "Build Variant 2",
			Tasks: []BuildVariantTask{
				{Name: "taskA"}, {Name: "taskB"}, {Name: "taskC"}, {Name: "taskE"},
			},
		}
		buildVar3 := BuildVariant{
			Name:        "buildVar3",
			DisplayName: "Build Variant 3",
			Tasks: []BuildVariantTask{
				{
					// wait for the first BV's taskA to complete
					Name:      "taskA",
					DependsOn: []TaskDependency{{Name: "taskA", Variant: "buildVar"}},
				},
			},
		}

		project := &Project{
			Tasks: []ProjectTask{
				{
					Name:      "taskA",
					Priority:  5,
					Tags:      []string{"tag1", "tag2"},
					DependsOn: []TaskDependency{},
				},
				{
					Name:      "taskB",
					Tags:      []string{"tag1", "tag2"},
					DependsOn: []TaskDependency{{Name: "taskA", Variant: "buildVar"}},
				},
				{
					Name: "taskC",
					Tags: []string{"tag1", "tag2"},
					DependsOn: []TaskDependency{
						{Name: "taskA"},
						{Name: "taskB"},
					},
				},
				{
					Name:      "taskD",
					Tags:      []string{"tag1", "tag2"},
					DependsOn: []TaskDependency{{Name: AllDependencies}},
				},
				{
					Name: "taskE",
					Tags: []string{"tag1", "tag2"},
					DependsOn: []TaskDependency{
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
		v := &version.Version{
			Id:                  "versionId",
			CreateTime:          time.Now(),
			Revision:            "foobar",
			RevisionOrderNumber: 500,
			Requester:           evergreen.RepotrackerVersionRequester,
			BuildVariants: []version.BuildStatus{
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
		}

		tt := BuildTaskIdTable(project, v)

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

			Convey(`and incorrect GetId() calls should return ""`, func() {
				So(tt.GetId("buildVar", "taskF"), ShouldEqual, "")
				So(tt.GetId("buildVar2", "taskD"), ShouldEqual, "")
				So(tt.GetId("buildVar7", "taskA"), ShouldEqual, "")
			})
		})

		Convey("if a non-existent build variant is passed in, an error should be returned", func() {

			buildId, err := CreateBuildFromVersion(project, v, tt, "blecch", false, []string{})
			So(err, ShouldNotBeNil)
			So(buildId, ShouldEqual, "")

		})

		Convey("if no task names are passed in to be used, all of the default"+
			" tasks for the build variant should be created", func() {

			buildId, err := CreateBuildFromVersion(project, v, tt, buildVar1.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")
			buildId2, err := CreateBuildFromVersion(project, v, tt, buildVar2.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId2, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := FindAllTasks(
				bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 8)
			So(len(tasks[0].Tags), ShouldEqual, 2)

		})

		Convey("if a non-empty list of task names is passed in, only the"+
			" specified tasks should be created", func() {

			buildId, err := CreateBuildFromVersion(project, v, tt, buildVar1.Name, false,
				[]string{"taskA", "taskB"})
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := FindAllTasks(
				bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 2)

		})

		Convey("the build should contain task caches that correspond exactly"+
			" to the tasks created", func() {

			buildId, err := CreateBuildFromVersion(project, v, tt, buildVar1.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := FindAllTasks(
				bson.M{},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
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
			So(b.Tasks[3].DisplayName, ShouldEqual, "taskD")
			So(b.Tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)

		})

		Convey("all of the tasks created should have the dependencies"+
			"and priorities specified in the project", func() {

			buildId, err := CreateBuildFromVersion(project, v, tt, buildVar1.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")
			buildId2, err := CreateBuildFromVersion(project, v, tt, buildVar2.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId2, ShouldNotEqual, "")
			buildId3, err := CreateBuildFromVersion(project, v, tt, buildVar3.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId3, ShouldNotEqual, "")

			// find the tasks, make sure they were all created
			tasks, err := FindAllTasks(
				bson.M{},
				db.NoProjection,
				[]string{"display_name", "build_variant"},
				db.NoSkip,
				db.NoLimit,
			)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 9)

			// taskA
			So(len(tasks[0].DependsOn), ShouldEqual, 0)
			So(len(tasks[1].DependsOn), ShouldEqual, 0)
			So(len(tasks[2].DependsOn), ShouldEqual, 1)
			So(tasks[0].Priority, ShouldEqual, 5)
			So(tasks[1].Priority, ShouldEqual, 5)
			So(tasks[2].DependsOn, ShouldResemble,
				[]Dependency{{tasks[0].Id, evergreen.TaskSucceeded}})

			// taskB
			So(tasks[3].DependsOn, ShouldResemble,
				[]Dependency{{tasks[0].Id, evergreen.TaskSucceeded}})
			So(tasks[4].DependsOn, ShouldResemble,
				[]Dependency{{tasks[0].Id, evergreen.TaskSucceeded}}) //cross-variant
			So(tasks[3].Priority, ShouldEqual, 0)
			So(tasks[4].Priority, ShouldEqual, 0) //default priority

			// taskC
			So(tasks[5].DependsOn, ShouldResemble,
				[]Dependency{
					{tasks[0].Id, evergreen.TaskSucceeded},
					{tasks[3].Id, evergreen.TaskSucceeded}})
			So(tasks[6].DependsOn, ShouldResemble,
				[]Dependency{
					{tasks[1].Id, evergreen.TaskSucceeded},
					{tasks[4].Id, evergreen.TaskSucceeded}})
			So(tasks[7].DependsOn, ShouldResemble,
				[]Dependency{
					{tasks[0].Id, evergreen.TaskSucceeded},
					{tasks[3].Id, evergreen.TaskSucceeded},
					{tasks[5].Id, evergreen.TaskSucceeded}})
			So(tasks[8].DisplayName, ShouldEqual, "taskE")
			So(len(tasks[8].DependsOn), ShouldEqual, 8)
		})

		Convey("all of the build's essential fields should be set"+
			" correctly", func() {

			buildId, err := CreateBuildFromVersion(project, v, tt, buildVar1.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the build from the db
			b, err := build.FindOne(build.ById(buildId))
			So(err, ShouldBeNil)

			// verify all the fields are set appropriately
			So(len(b.Tasks), ShouldEqual, 4)
			So(b.CreateTime.Truncate(time.Second), ShouldResemble,
				v.CreateTime.Truncate(time.Second))
			So(b.PushTime.Truncate(time.Second), ShouldResemble,
				v.CreateTime.Truncate(time.Second))
			So(b.Activated, ShouldEqual, v.BuildVariants[0].Activated)
			So(b.Project, ShouldEqual, project.Identifier)
			So(b.Revision, ShouldEqual, v.Revision)
			So(b.Status, ShouldEqual, evergreen.BuildCreated)
			So(b.BuildVariant, ShouldEqual, buildVar1.Name)
			So(b.Version, ShouldEqual, v.Id)
			So(b.DisplayName, ShouldEqual, buildVar1.DisplayName)
			So(b.RevisionOrderNumber, ShouldEqual, v.RevisionOrderNumber)
			So(b.Requester, ShouldEqual, v.Requester)

		})

		Convey("all of the tasks' essential fields should be set"+
			" correctly", func() {

			buildId, err := CreateBuildFromVersion(project, v, tt, buildVar1.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the build from the db
			b, err := build.FindOne(build.ById(buildId))
			So(err, ShouldBeNil)

			// find the tasks, make sure they were all created
			tasks, err := FindAllTasks(
				bson.M{},
				db.NoProjection,
				[]string{"display_name"},
				db.NoSkip,
				db.NoLimit,
			)
			So(err, ShouldBeNil)
			So(len(tasks), ShouldEqual, 4)

			So(tasks[0].Id, ShouldNotEqual, "")
			So(tasks[0].Secret, ShouldNotEqual, "")
			So(tasks[0].DisplayName, ShouldEqual, "taskA")
			So(tasks[0].BuildId, ShouldEqual, buildId)
			So(tasks[0].DistroId, ShouldEqual, "")
			So(tasks[0].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[0].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[0].PushTime.Truncate(time.Second), ShouldResemble,
				b.PushTime.Truncate(time.Second))
			So(tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[0].Activated, ShouldEqual, b.Activated)
			So(tasks[0].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[0].Requester, ShouldEqual, b.Requester)
			So(tasks[0].Version, ShouldEqual, v.Id)
			So(tasks[0].Revision, ShouldEqual, v.Revision)
			So(tasks[0].Project, ShouldEqual, project.Identifier)

			So(tasks[1].Id, ShouldNotEqual, "")
			So(tasks[1].Secret, ShouldNotEqual, "")
			So(tasks[1].DisplayName, ShouldEqual, "taskB")
			So(tasks[1].BuildId, ShouldEqual, buildId)
			So(tasks[1].DistroId, ShouldEqual, "")
			So(tasks[1].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[1].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[1].PushTime.Truncate(time.Second), ShouldResemble,
				b.PushTime.Truncate(time.Second))
			So(tasks[1].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[1].Activated, ShouldEqual, b.Activated)
			So(tasks[1].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[1].Requester, ShouldEqual, b.Requester)
			So(tasks[1].Version, ShouldEqual, v.Id)
			So(tasks[1].Revision, ShouldEqual, v.Revision)
			So(tasks[1].Project, ShouldEqual, project.Identifier)

			So(tasks[2].Id, ShouldNotEqual, "")
			So(tasks[2].Secret, ShouldNotEqual, "")
			So(tasks[2].DisplayName, ShouldEqual, "taskC")
			So(tasks[2].BuildId, ShouldEqual, buildId)
			So(tasks[2].DistroId, ShouldEqual, "")
			So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[2].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[2].PushTime.Truncate(time.Second), ShouldResemble,
				b.PushTime.Truncate(time.Second))
			So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[2].Activated, ShouldEqual, b.Activated)
			So(tasks[2].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[2].Requester, ShouldEqual, b.Requester)
			So(tasks[2].Version, ShouldEqual, v.Id)
			So(tasks[2].Revision, ShouldEqual, v.Revision)
			So(tasks[2].Project, ShouldEqual, project.Identifier)

			So(tasks[3].Id, ShouldNotEqual, "")
			So(tasks[3].Secret, ShouldNotEqual, "")
			So(tasks[3].DisplayName, ShouldEqual, "taskD")
			So(tasks[3].BuildId, ShouldEqual, buildId)
			So(tasks[3].DistroId, ShouldEqual, "")
			So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
			So(tasks[3].CreateTime.Truncate(time.Second), ShouldResemble,
				b.CreateTime.Truncate(time.Second))
			So(tasks[3].PushTime.Truncate(time.Second), ShouldResemble,
				b.PushTime.Truncate(time.Second))
			So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
			So(tasks[3].Activated, ShouldEqual, b.Activated)
			So(tasks[3].RevisionOrderNumber, ShouldEqual, b.RevisionOrderNumber)
			So(tasks[3].Requester, ShouldEqual, b.Requester)
			So(tasks[3].Version, ShouldEqual, v.Id)
			So(tasks[3].Revision, ShouldEqual, v.Revision)
			So(tasks[3].Project, ShouldEqual, project.Identifier)
		})

		Convey("if the activated flag is set, the build and all its tasks should be activated",
			func() {

				buildId, err := CreateBuildFromVersion(project, v, tt, buildVar1.Name, true, nil)
				So(err, ShouldBeNil)
				So(buildId, ShouldNotEqual, "")

				// find the build from the db
				build, err := build.FindOne(build.ById(buildId))
				So(err, ShouldBeNil)
				So(build.Activated, ShouldBeTrue)

				// find the tasks, make sure they were all created
				tasks, err := FindAllTasks(
					bson.M{},
					db.NoProjection,
					[]string{"display_name"},
					db.NoSkip,
					db.NoLimit,
				)
				So(err, ShouldBeNil)
				So(len(tasks), ShouldEqual, 4)

				So(tasks[0].Id, ShouldNotEqual, "")
				So(tasks[0].Secret, ShouldNotEqual, "")
				So(tasks[0].DisplayName, ShouldEqual, "taskA")
				So(tasks[0].BuildId, ShouldEqual, buildId)
				So(tasks[0].DistroId, ShouldEqual, "")
				So(tasks[0].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[0].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[0].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[0].Activated, ShouldEqual, build.Activated)
				So(tasks[0].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[0].Requester, ShouldEqual, build.Requester)
				So(tasks[0].Version, ShouldEqual, v.Id)
				So(tasks[0].Revision, ShouldEqual, v.Revision)
				So(tasks[0].Project, ShouldEqual, project.Identifier)

				So(tasks[1].Id, ShouldNotEqual, "")
				So(tasks[1].Secret, ShouldNotEqual, "")
				So(tasks[1].DisplayName, ShouldEqual, "taskB")
				So(tasks[1].BuildId, ShouldEqual, buildId)
				So(tasks[1].DistroId, ShouldEqual, "")
				So(tasks[1].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[1].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[1].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[1].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[1].Activated, ShouldEqual, build.Activated)
				So(tasks[1].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[1].Requester, ShouldEqual, build.Requester)
				So(tasks[1].Version, ShouldEqual, v.Id)
				So(tasks[1].Revision, ShouldEqual, v.Revision)
				So(tasks[1].Project, ShouldEqual, project.Identifier)

				So(tasks[2].Id, ShouldNotEqual, "")
				So(tasks[2].Secret, ShouldNotEqual, "")
				So(tasks[2].DisplayName, ShouldEqual, "taskC")
				So(tasks[2].BuildId, ShouldEqual, buildId)
				So(tasks[2].DistroId, ShouldEqual, "")
				So(tasks[2].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[2].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[2].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[2].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[2].Activated, ShouldEqual, build.Activated)
				So(tasks[2].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[2].Requester, ShouldEqual, build.Requester)
				So(tasks[2].Version, ShouldEqual, v.Id)
				So(tasks[2].Revision, ShouldEqual, v.Revision)
				So(tasks[2].Project, ShouldEqual, project.Identifier)

				So(tasks[3].Id, ShouldNotEqual, "")
				So(tasks[3].Secret, ShouldNotEqual, "")
				So(tasks[3].DisplayName, ShouldEqual, "taskD")
				So(tasks[3].BuildId, ShouldEqual, buildId)
				So(tasks[3].DistroId, ShouldEqual, "")
				So(tasks[3].BuildVariant, ShouldEqual, buildVar1.Name)
				So(tasks[3].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[3].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[3].Status, ShouldEqual, evergreen.TaskUndispatched)
				So(tasks[3].Activated, ShouldEqual, build.Activated)
				So(tasks[3].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
				So(tasks[3].Requester, ShouldEqual, build.Requester)
				So(tasks[3].Version, ShouldEqual, v.Id)
				So(tasks[3].Revision, ShouldEqual, v.Revision)
				So(tasks[3].Project, ShouldEqual, project.Identifier)
			})

	})
}

func TestDeletingBuild(t *testing.T) {

	Convey("With a build", t, func() {

		testutil.HandleTestingErr(db.Clear(build.Collection), t, "Error clearing"+
			" '%v' collection", build.Collection)

		b := &build.Build{
			Id: "build",
		}
		So(b.Insert(), ShouldBeNil)

		Convey("deleting it should remove it and all its associated"+
			" tasks from the database", func() {

			testutil.HandleTestingErr(db.ClearCollections(TasksCollection), t, "Error"+
				" clearing '%v' collection", TasksCollection)

			// insert two tasks that are part of the build, and one that isn't
			matchingTaskOne := &Task{
				Id:      "matchingOne",
				BuildId: b.Id,
			}
			So(matchingTaskOne.Insert(), ShouldBeNil)

			matchingTaskTwo := &Task{
				Id:      "matchingTwo",
				BuildId: b.Id,
			}
			So(matchingTaskTwo.Insert(), ShouldBeNil)

			nonMatchingTask := &Task{
				Id:      "nonMatching",
				BuildId: "blech",
			}
			So(nonMatchingTask.Insert(), ShouldBeNil)

			// delete the build, make sure only it and its tasks are deleted

			So(DeleteBuild(b.Id), ShouldBeNil)

			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b, ShouldBeNil)

			matchingTasks, err := FindAllTasks(
				bson.M{TaskBuildIdKey: "build"},
				db.NoProjection,
				db.NoSort,
				db.NoSkip,
				db.NoLimit,
			)
			So(err, ShouldBeNil)
			So(len(matchingTasks), ShouldEqual, 0)

			nonMatchingTask, err = FindOneTask(
				bson.M{
					TaskIdKey: nonMatchingTask.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(nonMatchingTask, ShouldNotBeNil)
		})
	})
}

func TestSetNumDeps(t *testing.T) {
	Convey("setNumDeps correctly sets NumDependents for each task", t, func() {
		tasks := []*Task{
			&Task{
				Id: "task1",
			},
			&Task{
				Id:        "task2",
				DependsOn: []Dependency{{TaskId: "task1"}},
			},
			&Task{
				Id:        "task3",
				DependsOn: []Dependency{{TaskId: "task1"}},
			},
			&Task{
				Id:        "task4",
				DependsOn: []Dependency{{TaskId: "task2"}, {TaskId: "task3"}, {TaskId: "not_here"}},
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
			tasks := []Task{
				{
					Id:          "idA",
					DisplayName: "A",
					DependsOn: []Dependency{
						{TaskId: "idB"},
					},
				},
				{
					Id:          "idB",
					DisplayName: "B",
					DependsOn: []Dependency{
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
			tasks := []Task{
				{
					Id:          "idA",
					DisplayName: "A",
					DependsOn: []Dependency{
						{TaskId: "idB"},
						{TaskId: "idC"},
					},
				},
				{
					Id:          "idB",
					DisplayName: "B",
					DependsOn: []Dependency{
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
		tasks := []Task{
			{
				Id:          "idA",
				DisplayName: "A",
				DependsOn: []Dependency{
					{TaskId: "idE"},
				},
			},
			{
				Id:          "idB",
				DisplayName: "B",
				DependsOn: []Dependency{
					{TaskId: "idD"},
				},
			},
			{
				Id:          "idC",
				DisplayName: "C",
				DependsOn: []Dependency{
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
		tasks := []Task{
			{
				Id:          "idA",
				DisplayName: "A",
				DependsOn: []Dependency{
					{TaskId: "idB"},
					{TaskId: "idC"},
				},
			},
			{
				Id:          "idB",
				DisplayName: "B",
				DependsOn: []Dependency{
					{TaskId: "idC"},
				},
			},
			{
				Id:          "idC",
				DisplayName: "C",
				DependsOn: []Dependency{
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
				Task{
					Id:          "idE",
					DisplayName: "E",
					DependsOn: []Dependency{
						{TaskId: "cross-variant2"},
					}},
				Task{
					Id:          "idF",
					DisplayName: "F",
					DependsOn: []Dependency{
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
