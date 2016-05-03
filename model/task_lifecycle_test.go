package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	oneMs = time.Millisecond
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
}

func TestSetActiveState(t *testing.T) {
	Convey("With one task with no dependencies", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		b := &build.Build{
			Id: "buildtest",
		}
		testTask := task.Task{
			Id:            "testone",
			DisplayName:   displayName,
			ScheduledTime: testTime,
			Activated:     false,
			BuildId:       b.Id,
		}
		b.Tasks = []build.TaskCache{
			build.TaskCache{
				Id: testTask.Id,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)

		Convey("activating the task should set the task state to active", func() {
			So(SetActiveState(testTask.Id, "randomUser", true), ShouldBeNil)
			testTask, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldBeTrue)
			So(testTask.ScheduledTime, ShouldHappenWithin, oneMs, testTime)

			Convey("deactivating an active task as a normal user should deactivate the task", func() {
				So(SetActiveState(testTask.Id, userName, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(testTask.Activated, ShouldBeFalse)
			})
		})
		Convey("when deactivating an active task as evergreen", func() {
			Convey("if the task is activated by evergreen, the task should deactivate", func() {
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, true), ShouldBeNil)
				testTask, err := task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, evergreen.DefaultTaskActivator)
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, false)
			})
			Convey("if the task is not activated by evergreen, the task should not deactivate", func() {
				So(SetActiveState(testTask.Id, userName, true), ShouldBeNil)
				testTask, err := task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.ActivatedBy, ShouldEqual, userName)
				So(SetActiveState(testTask.Id, evergreen.DefaultTaskActivator, false), ShouldBeNil)
				testTask, err = task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Activated, ShouldEqual, true)
			})

		})
	})
	Convey("With one task has tasks it depends on", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		userName := "testUser"
		testTime := time.Now()
		taskId := "t1"
		buildId := "b1"

		dep1 := &task.Task{
			Id:            "t2",
			ScheduledTime: testTime,
			BuildId:       buildId,
		}
		dep2 := &task.Task{
			Id:            "t3",
			ScheduledTime: testTime,
			BuildId:       buildId,
		}
		So(dep1.Insert(), ShouldBeNil)
		So(dep2.Insert(), ShouldBeNil)

		testTask := task.Task{
			Id:          taskId,
			DisplayName: displayName,
			Activated:   false,
			BuildId:     buildId,
			DependsOn: []task.Dependency{
				{"t2", evergreen.TaskSucceeded},
				{"t3", evergreen.TaskSucceeded},
			},
		}

		b := &build.Build{
			Id:    buildId,
			Tasks: []build.TaskCache{{Id: taskId}, {Id: "t2"}, {Id: "t3"}},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)

		Convey("activating the task should activate the tasks it depends on", func() {
			So(SetActiveState(testTask.Id, userName, true), ShouldBeNil)
			depTask, err := task.FindOne(task.ById(dep1.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			depTask, err = task.FindOne(task.ById(dep2.Id))
			So(err, ShouldBeNil)
			So(depTask.Activated, ShouldBeTrue)

			Convey("deactivating the task should not deactive the tasks it depends on", func() {
				So(SetActiveState(testTask.Id, userName, false), ShouldBeNil)
				depTask, err := task.FindOne(task.ById(depTask.Id))
				So(err, ShouldBeNil)
				So(depTask.Activated, ShouldBeTrue)
			})
		})
	})
}

func TestActivatePreviousTask(t *testing.T) {
	Convey("With two tasks and a build", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
			"Error clearing task and build collections")
		// create two tasks
		displayName := "testTask"
		b := &build.Build{
			Id: "testBuild",
		}
		previousTask := &task.Task{
			Id:                  "one",
			DisplayName:         displayName,
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           false,
			BuildId:             b.Id,
		}
		currentTask := &task.Task{
			Id:                  "two",
			DisplayName:         displayName,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
			Priority:            1,
			Activated:           true,
			BuildId:             b.Id,
		}
		tc := []build.TaskCache{
			build.TaskCache{
				DisplayName: displayName,
				Id:          previousTask.Id,
			},
			build.TaskCache{
				DisplayName: displayName,
				Id:          currentTask.Id,
			},
		}
		b.Tasks = tc
		So(b.Insert(), ShouldBeNil)
		So(previousTask.Insert(), ShouldBeNil)
		So(currentTask.Insert(), ShouldBeNil)
		Convey("activating a previous task should set the previous task's active field to true", func() {
			So(ActivatePreviousTask(currentTask.Id, ""), ShouldBeNil)
			t, err := task.FindOne(task.ById(previousTask.Id))
			So(err, ShouldBeNil)
			So(t.Activated, ShouldBeTrue)
		})
	})
}

func TestDeactivatePreviousTask(t *testing.T) {
	Convey("With two tasks and a build", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection), t,
			"Error clearing task and build collections")
		// create two tasks
		displayName := "testTask"
		userName := "user"
		b := &build.Build{
			Id: "testBuild",
		}
		previousTask := &task.Task{
			Id:                  "one",
			DisplayName:         displayName,
			RevisionOrderNumber: 1,
			Priority:            1,
			Activated:           true,
			ActivatedBy:         "user",
			BuildId:             b.Id,
			Status:              evergreen.TaskUndispatched,
			Project:             "sample",
		}
		currentTask := &task.Task{
			Id:                  "two",
			DisplayName:         displayName,
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
			Priority:            1,
			Activated:           true,
			BuildId:             b.Id,
			Project:             "sample",
		}
		tc := []build.TaskCache{
			build.TaskCache{
				DisplayName: displayName,
				Id:          previousTask.Id,
			},
			build.TaskCache{
				DisplayName: displayName,
				Id:          currentTask.Id,
			},
		}
		b.Tasks = tc
		So(b.Insert(), ShouldBeNil)
		So(previousTask.Insert(), ShouldBeNil)
		So(currentTask.Insert(), ShouldBeNil)
		Convey("activating a previous task should set the previous task's active field to true", func() {
			So(DeactivatePreviousTasks(currentTask.Id, userName), ShouldBeNil)
			previousTask, err := task.FindOne(task.ById(previousTask.Id))
			So(err, ShouldBeNil)
			So(previousTask.Activated, ShouldBeFalse)
		})
	})
}

func TestUpdateBuildStatusForTask(t *testing.T) {
	Convey("With two tasks and a build", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildStarted,
			Version: "abc",
		}
		v := &version.Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		testTask := task.Task{
			Id:          "testone",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskFailed,
		}
		anotherTask := task.Task{
			Id:          "two",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskFailed,
		}

		b.Tasks = []build.TaskCache{
			build.TaskCache{
				Id:     testTask.Id,
				Status: evergreen.TaskSucceeded,
			},
			build.TaskCache{
				Id:     anotherTask.Id,
				Status: evergreen.TaskFailed,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(anotherTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		Convey("updating the build for a task should update the build's status and the version's status", func() {
			So(UpdateBuildAndVersionStatusForTask(testTask.Id), ShouldBeNil)
			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildFailed)
			v, err := version.FindOne(version.ById(v.Id))
			So(v.Status, ShouldEqual, evergreen.VersionFailed)

		})

	})
}

func TestMarkEnd(t *testing.T) {
	Convey("With a task and a build", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		userName := "testUser"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildStarted,
			Version: "abc",
		}
		p := &Project{
			Identifier: "sample",
		}
		v := &version.Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		testTask := task.Task{
			Id:          "testone",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskStarted,
		}

		b.Tasks = []build.TaskCache{
			build.TaskCache{
				Id:     testTask.Id,
				Status: evergreen.TaskStarted,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		Convey("task, build and version status will be updated properly", func() {
			details := apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			So(MarkEnd(testTask.Id, userName, time.Now(), &details, p, false), ShouldBeNil)

		})
	})
}

func TestTryResetTask(t *testing.T) {
	Convey("With a task, a build, version and a project", t, func() {
		Convey("resetting a task without a max number of executions", func() {
			testutil.HandleTestingErr(db.ClearCollections(task.Collection, task.OldCollection, build.Collection, version.Collection), t,
				"Error clearing task and build collections")
			displayName := "testName"
			userName := "testUser"
			b := &build.Build{
				Id:      "buildtest",
				Status:  evergreen.BuildStarted,
				Version: "abc",
			}
			v := &version.Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
			}
			testTask := task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   1,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
			}
			p := &Project{
				Identifier: "sample",
			}
			detail := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}

			b.Tasks = []build.TaskCache{
				build.TaskCache{
					Id: testTask.Id,
				},
			}
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			Convey("should reset and add a task to the old tasks collection", func() {
				So(TryResetTask(testTask.Id, userName, "", p, detail), ShouldBeNil)
				testTask, err := task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(testTask.FinishTime, ShouldResemble, util.ZeroTime)
				oldTaskId := fmt.Sprintf("%v_%v", testTask.Id, 1)
				fmt.Println(oldTaskId)
				oldTask, err := task.FindOneOld(task.ById(oldTaskId))
				So(err, ShouldBeNil)
				So(oldTask, ShouldNotBeNil)
				So(oldTask.Execution, ShouldEqual, 1)
				So(oldTask.Details, ShouldResemble, *detail)
				So(oldTask.FinishTime, ShouldNotResemble, util.ZeroTime)
			})

		})
		Convey("resetting a task with a max number of excutions", func() {
			testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
				"Error clearing task and build collections")
			displayName := "testName"
			userName := "testUser"
			b := &build.Build{
				Id:      "buildtest",
				Status:  evergreen.BuildStarted,
				Version: "abc",
			}
			v := &version.Version{
				Id:     b.Version,
				Status: evergreen.VersionStarted,
			}
			testTask := task.Task{
				Id:          "testone",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   evergreen.MaxTaskExecution,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
			}
			p := &Project{
				Identifier: "sample",
			}
			detail := &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
			}
			anotherTask := task.Task{
				Id:          "two",
				DisplayName: displayName,
				Activated:   false,
				BuildId:     b.Id,
				Execution:   evergreen.MaxTaskExecution,
				Project:     "sample",
				Status:      evergreen.TaskSucceeded,
			}
			b.Tasks = []build.TaskCache{
				build.TaskCache{
					Id: testTask.Id,
				},
				build.TaskCache{
					Id: anotherTask.Id,
				},
			}
			So(b.Insert(), ShouldBeNil)
			So(testTask.Insert(), ShouldBeNil)
			So(v.Insert(), ShouldBeNil)
			So(anotherTask.Insert(), ShouldBeNil)

			Convey("should not reset if an origin other than the ui package tries to reset", func() {
				So(TryResetTask(testTask.Id, userName, "", p, detail), ShouldBeNil)
				testTask, err := task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(testTask.Details, ShouldResemble, *detail)
				So(testTask.Status, ShouldEqual, detail.Status)
				So(testTask.FinishTime, ShouldNotResemble, util.ZeroTime)
			})
			Convey("should reset and use detail information if the UI package passes in a detail ", func() {
				So(TryResetTask(anotherTask.Id, userName, evergreen.UIPackage, p, detail), ShouldBeNil)
				a, err := task.FindOne(task.ById(anotherTask.Id))
				So(err, ShouldBeNil)
				So(a.Details, ShouldResemble, apimodels.TaskEndDetail{})
				So(a.Status, ShouldEqual, evergreen.TaskUndispatched)
				So(a.FinishTime, ShouldResemble, util.ZeroTime)
			})
		})
	})
}

func TestAbortTask(t *testing.T) {
	Convey("With a task and a build", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task, build, and version collections")
		displayName := "testName"
		userName := "testUser"
		b := &build.Build{
			Id: "buildtest",
		}
		testTask := task.Task{
			Id:          "testone",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Status:      evergreen.TaskStarted,
		}
		finishedTask := task.Task{
			Id:          "another",
			DisplayName: displayName,
			Activated:   false,
			BuildId:     b.Id,
			Status:      evergreen.TaskFailed,
		}
		b.Tasks = []build.TaskCache{
			build.TaskCache{
				Id: testTask.Id,
			},
			build.TaskCache{
				Id: finishedTask.Id,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(finishedTask.Insert(), ShouldBeNil)
		Convey("with a task that has started, aborting a task should work", func() {
			So(AbortTask(testTask.Id, userName), ShouldBeNil)
			testTask, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Activated, ShouldEqual, false)
			So(testTask.Aborted, ShouldEqual, true)
		})
		Convey("a task that is finished should error when aborting", func() {
			So(AbortTask(finishedTask.Id, userName), ShouldNotBeNil)
		})
	})

}
func TestMarkStart(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildCreated,
			Version: "abc",
		}
		v := &version.Version{
			Id:     b.Version,
			Status: evergreen.VersionCreated,
		}
		testTask := task.Task{
			Id:          "testTask",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskUndispatched,
			Version:     b.Version,
		}

		b.Tasks = []build.TaskCache{
			build.TaskCache{
				Id:     testTask.Id,
				Status: evergreen.TaskUndispatched,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			So(MarkStart(testTask.Id), ShouldBeNil)
			testTask, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Status, ShouldEqual, evergreen.BuildStarted)
			So(b.Tasks, ShouldNotBeNil)
			So(len(b.Tasks), ShouldEqual, 1)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskStarted)
			v, err := version.FindOne(version.ById(v.Id))
			So(err, ShouldBeNil)
			So(v.Status, ShouldEqual, evergreen.VersionStarted)
		})
	})
}

func TestMarkUndispatched(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildStarted,
			Version: "abc",
		}
		v := &version.Version{
			Id:     b.Version,
			Status: evergreen.VersionStarted,
		}
		testTask := &task.Task{
			Id:          "testTask",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskStarted,
			Version:     b.Version,
		}

		b.Tasks = []build.TaskCache{
			build.TaskCache{
				Id:     testTask.Id,
				Status: evergreen.TaskStarted,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		So(v.Insert(), ShouldBeNil)
		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			So(MarkTaskUndispatched(testTask), ShouldBeNil)
			testTask, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Tasks, ShouldNotBeNil)
			So(len(b.Tasks), ShouldEqual, 1)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskUndispatched)
		})
	})
}

func TestMarkDispatched(t *testing.T) {
	Convey("With a task, build and version", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, build.Collection, version.Collection), t,
			"Error clearing task and build collections")
		displayName := "testName"
		b := &build.Build{
			Id:      "buildtest",
			Status:  evergreen.BuildCreated,
			Version: "abc",
		}
		testTask := &task.Task{
			Id:          "testTask",
			DisplayName: displayName,
			Activated:   true,
			BuildId:     b.Id,
			Project:     "sample",
			Status:      evergreen.TaskUndispatched,
			Version:     b.Version,
		}

		b.Tasks = []build.TaskCache{
			build.TaskCache{
				Id:     testTask.Id,
				Status: evergreen.TaskUndispatched,
			},
		}
		So(b.Insert(), ShouldBeNil)
		So(testTask.Insert(), ShouldBeNil)
		Convey("when calling MarkStart, the task, version and build should be updated", func() {
			So(MarkTaskDispatched(testTask, "testHost", "distroId"), ShouldBeNil)
			testTask, err := task.FindOne(task.ById(testTask.Id))
			So(err, ShouldBeNil)
			So(testTask.Status, ShouldEqual, evergreen.TaskDispatched)
			So(testTask.HostId, ShouldEqual, "testHost")
			So(testTask.DistroId, ShouldEqual, "distroId")
			b, err := build.FindOne(build.ById(b.Id))
			So(err, ShouldBeNil)
			So(b.Tasks, ShouldNotBeNil)
			So(len(b.Tasks), ShouldEqual, 1)
			So(b.Tasks[0].Status, ShouldEqual, evergreen.TaskDispatched)
		})
	})
}

func TestGetstepback(t *testing.T) {
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
			testTask := &task.Task{Id: "t1", DisplayName: "nil"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with true", func() {
			testTask := &task.Task{Id: "t2", DisplayName: "true"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the task overrides the setting with false", func() {
			testTask := &task.Task{Id: "t3", DisplayName: "false"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})

		Convey("if the buildvariant does not override the setting", func() {
			testTask := &task.Task{Id: "t4", DisplayName: "bvnil", BuildVariant: "sbnil"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with true", func() {
			testTask := &task.Task{Id: "t5", DisplayName: "bvtrue", BuildVariant: "sbtrue"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be true", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeTrue)
			})
		})

		Convey("if the buildvariant overrides the setting with false", func() {
			testTask := &task.Task{Id: "t6", DisplayName: "bvfalse", BuildVariant: "sbfalse"}
			So(testTask.Insert(), ShouldBeNil)
			Convey("then the value should be false", func() {
				val, err := getStepback(testTask.Id, project)
				So(err, ShouldBeNil)
				So(val, ShouldBeFalse)
			})
		})

	})
}
