package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model/version"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"labix.org/v2/mgo/bson"
	"testing"
	"time"
)

var (
	buildTestConfig = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(buildTestConfig))
}

func buildIdInSlice(builds []Build, id string) bool {
	for _, build := range builds {
		if build.Id == id {
			return true
		}
	}
	return false
}

func taskIdInSlice(tasks []Task, id string) bool {
	for _, task := range tasks {
		if task.Id == id {
			return true
		}
	}
	return false
}

func TestGenericBuildFinding(t *testing.T) {

	Convey("When finding builds", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		Convey("when finding one build", func() {

			Convey("the matching build should be returned", func() {

				buildOne := &Build{
					Id: "buildOne",
				}
				So(buildOne.Insert(), ShouldBeNil)

				buildTwo := &Build{
					Id: "buildTwo",
				}
				So(buildTwo.Insert(), ShouldBeNil)

				found, err := FindOneBuild(
					bson.M{
						BuildIdKey: buildOne.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, buildOne.Id)

				found, err = FindOneBuild(
					bson.M{
						BuildIdKey: buildTwo.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, buildTwo.Id)

			})

		})

		Convey("when finding multiple builds", func() {

			Convey("a slice of all of the matching builds should be"+
				" returned", func() {

				buildOne := &Build{
					Id:      "buildOne",
					Project: "b1",
				}
				So(buildOne.Insert(), ShouldBeNil)

				buildTwo := &Build{
					Id:      "buildTwo",
					Project: "b1",
				}
				So(buildTwo.Insert(), ShouldBeNil)

				buildThree := &Build{
					Id:      "buildThree",
					Project: "b2",
				}
				So(buildThree.Insert(), ShouldBeNil)

				found, err := FindAllBuilds(
					bson.M{
						BuildProjectKey: "b1",
					},
					db.NoProjection,
					db.NoSort,
					db.NoSkip,
					db.NoLimit,
				)
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, 2)
				So(buildIdInSlice(found, buildOne.Id), ShouldBeTrue)
				So(buildIdInSlice(found, buildTwo.Id), ShouldBeTrue)

			})

		})

	})

}

func TestFindIntermediateBuilds(t *testing.T) {

	Convey("When finding intermediate builds", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		// the two builds to use as endpoints

		currBuild := &Build{
			Id:                  "curr",
			RevisionOrderNumber: 1000,
			BuildVariant:        "bv1",
			Requester:           "r1",
			Project:             "project1",
		}
		So(currBuild.Insert(), ShouldBeNil)

		prevBuild := &Build{
			Id:                  "prev",
			RevisionOrderNumber: 10,
		}
		So(prevBuild.Insert(), ShouldBeNil)

		Convey("all builds returned should be in commits between the current"+
			" build and the specified previous one", func() {

			// insert two builds with commit order numbers in the correct
			// range

			matchingBuildOne := &Build{
				Id:                  "mb1",
				RevisionOrderNumber: 50,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(matchingBuildOne.Insert(), ShouldBeNil)

			matchingBuildTwo := &Build{
				Id:                  "mb2",
				RevisionOrderNumber: 51,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(matchingBuildTwo.Insert(), ShouldBeNil)

			// insert two builds with commit order numbers out of range (one too
			// high and one too low)

			numberTooLow := &Build{
				Id:                  "tooLow",
				RevisionOrderNumber: 5,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(numberTooLow.Insert(), ShouldBeNil)

			numberTooHigh := &Build{
				Id:                  "tooHigh",
				RevisionOrderNumber: 5000,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(numberTooHigh.Insert(), ShouldBeNil)

			// finding intermediate builds should return only the two in range

			found, err := currBuild.FindIntermediateBuilds(prevBuild)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(buildIdInSlice(found, matchingBuildOne.Id), ShouldBeTrue)
			So(buildIdInSlice(found, matchingBuildTwo.Id), ShouldBeTrue)

		})

		Convey("all builds returned should have the same build variant,"+
			" requester, and project as the current one", func() {

			// insert four builds - one with the wrong build variant, one
			// with the wrong requester, one with the wrong project, and one
			// with all the correct values

			wrongBV := &Build{
				Id:                  "wrongBV",
				RevisionOrderNumber: 50,
				BuildVariant:        "bv2",
				Requester:           "r1",
				Project:             "project1",
			}
			So(wrongBV.Insert(), ShouldBeNil)

			wrongReq := &Build{
				Id:                  "wrongReq",
				RevisionOrderNumber: 51,
				BuildVariant:        "bv1",
				Requester:           "r2",
				Project:             "project1",
			}
			So(wrongReq.Insert(), ShouldBeNil)

			wrongProject := &Build{
				Id:                  "wrongProject",
				RevisionOrderNumber: 52,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project2",
			}
			So(wrongProject.Insert(), ShouldBeNil)

			allCorrect := &Build{
				Id:                  "allCorrect",
				RevisionOrderNumber: 53,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(allCorrect.Insert(), ShouldBeNil)

			// finding intermediate builds should return only the one with
			// all the correctly matching values

			found, err := currBuild.FindIntermediateBuilds(prevBuild)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 1)
			So(found[0].Id, ShouldEqual, allCorrect.Id)

		})

		Convey("the builds returned should be sorted in ascending order"+
			" by commit order number", func() {

			// insert two builds with commit order numbers in the correct
			// range

			matchingBuildOne := &Build{
				Id:                  "mb1",
				RevisionOrderNumber: 52,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(matchingBuildOne.Insert(), ShouldBeNil)

			matchingBuildTwo := &Build{
				Id:                  "mb2",
				RevisionOrderNumber: 50,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(matchingBuildTwo.Insert(), ShouldBeNil)

			matchingBuildThree := &Build{
				Id:                  "mb3",
				RevisionOrderNumber: 51,
				BuildVariant:        "bv1",
				Requester:           "r1",
				Project:             "project1",
			}
			So(matchingBuildThree.Insert(), ShouldBeNil)

			// the builds should come out sorted by commit order number

			found, err := currBuild.FindIntermediateBuilds(prevBuild)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 3)
			So(found[0].Id, ShouldEqual, matchingBuildTwo.Id)
			So(found[1].Id, ShouldEqual, matchingBuildThree.Id)
			So(found[2].Id, ShouldEqual, matchingBuildOne.Id)
		})

	})

}

func TestFindPreviousActivatedBuild(t *testing.T) {

	Convey("When finding the previous activated build", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		currBuild := &Build{
			Id:                  "curr",
			RevisionOrderNumber: 1000,
			BuildVariant:        "bv1",
			Project:             "project1",
			Requester:           "r1",
		}

		Convey("the last activated build before the specified one with the"+
			" same build variant and specified requester + project should be"+
			" fetched", func() {

			// insert 7 builds:
			//  one with too high a commit number
			//  one with the wrong build variant
			//  one with the wrong project
			//  one with the wrong requester
			//  one inactive
			//  two matching ones

			tooHigh := &Build{
				Id:                  "tooHigh",
				RevisionOrderNumber: 5000,
				BuildVariant:        "bv1",
				Project:             "project1",
				Requester:           "r1",
				Activated:           true,
			}
			So(tooHigh.Insert(), ShouldBeNil)

			wrongBV := &Build{
				Id:                  "wrongBV",
				RevisionOrderNumber: 500,
				BuildVariant:        "bv2",
				Project:             "project1",
				Requester:           "r1",
				Activated:           true,
			}
			So(wrongBV.Insert(), ShouldBeNil)

			wrongProject := &Build{
				Id:                  "wrongProject",
				RevisionOrderNumber: 500,
				BuildVariant:        "bv1",
				Project:             "project2",
				Requester:           "r1",
				Activated:           true,
			}
			So(wrongProject.Insert(), ShouldBeNil)

			wrongReq := &Build{
				Id:                  "wrongReq",
				RevisionOrderNumber: 500,
				BuildVariant:        "bv1",
				Project:             "project1",
				Requester:           "r2",
				Activated:           true,
			}
			So(wrongReq.Insert(), ShouldBeNil)

			notActive := &Build{
				Id:                  "notActive",
				RevisionOrderNumber: 500,
				BuildVariant:        "bv1",
				Project:             "project1",
				Requester:           "r1",
			}
			So(notActive.Insert(), ShouldBeNil)

			matchingHigher := &Build{
				Id:                  "matchingHigher",
				RevisionOrderNumber: 900,
				BuildVariant:        "bv1",
				Project:             "project1",
				Requester:           "r1",
				Activated:           true,
			}
			So(matchingHigher.Insert(), ShouldBeNil)

			matchingLower := &Build{
				Id:                  "matchingLower",
				RevisionOrderNumber: 800,
				BuildVariant:        "bv1",
				Project:             "project1",
				Requester:           "r1",
				Activated:           true,
			}
			So(matchingLower.Insert(), ShouldBeNil)

			// the matching build with the higher commit order number should
			// be returned

			found, err := currBuild.PreviousActivatedBuild("project1", "r1")
			So(err, ShouldBeNil)
			So(found.Id, ShouldEqual, matchingHigher.Id)

		})

	})
}

func TestRecentlyFinishedBuilds(t *testing.T) {

	Convey("When finding all recently finished builds", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		Convey("all builds returned should be finished", func() {

			finishTime := time.Now().Add(-10)

			// insert two finished builds and one unfinished build

			finishedOne := &Build{
				Id:         "fin1",
				Project:    "project1",
				Requester:  "r1",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime.Add(1 * time.Second),
			}
			So(finishedOne.Insert(), ShouldBeNil)

			finishedTwo := &Build{
				Id:         "fin2",
				Project:    "project1",
				Requester:  "r1",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime.Add(2 * time.Second),
			}
			So(finishedTwo.Insert(), ShouldBeNil)

			unfinished := &Build{
				Id:        "unfin",
				Project:   "project1",
				Requester: "r1",
			}
			So(unfinished.Insert(), ShouldBeNil)

			// only the finished ones should be returned

			found, err := RecentlyFinishedBuilds(finishTime, "project1", "r1")
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(buildIdInSlice(found, finishedOne.Id), ShouldBeTrue)
			So(buildIdInSlice(found, finishedTwo.Id), ShouldBeTrue)

		})

		Convey("all builds returned should have finished after the specified"+
			" time", func() {

			finishTime := time.Now().Add(-10)

			// insert three finished builds

			finishedOne := &Build{
				Id:         "fin1",
				Project:    "project1",
				Requester:  "r1",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime.Add(1 * time.Second),
			}
			So(finishedOne.Insert(), ShouldBeNil)

			finishedTwo := &Build{
				Id:         "fin2",
				Project:    "project1",
				Requester:  "r1",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime,
			}
			So(finishedTwo.Insert(), ShouldBeNil)

			finishedThree := &Build{
				Id:         "fin3",
				Project:    "project1",
				Requester:  "r1",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime.Add(-1 * time.Second),
			}
			So(finishedThree.Insert(), ShouldBeNil)

			// only the one that finished after the specified time should
			// be returned

			found, err := RecentlyFinishedBuilds(finishTime, "project1", "r1")
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 1)
			So(found[0].Id, ShouldEqual, finishedOne.Id)

		})

		Convey("all builds should have the specified requester and"+
			" project", func() {

			finishTime := time.Now().Add(-10)

			// insert three finished builds; one with the wrong requester,
			// one with the wrong project, and one with both correct

			wrongReq := &Build{
				Id:         "wrongReq",
				Project:    "project1",
				Requester:  "r2",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime.Add(1 * time.Second),
			}
			So(wrongReq.Insert(), ShouldBeNil)

			wrongProject := &Build{
				Id:         "wrongProject",
				Project:    "project2",
				Requester:  "r1",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime.Add(1 * time.Second),
			}
			So(wrongProject.Insert(), ShouldBeNil)

			matching := &Build{
				Id:         "matching",
				Project:    "project1",
				Requester:  "r1",
				TimeTaken:  time.Duration(1),
				FinishTime: finishTime.Add(1 * time.Second),
			}
			So(matching.Insert(), ShouldBeNil)

			// only the one with the correct project and requester should be
			// returned

			found, err := RecentlyFinishedBuilds(finishTime, "project1", "r1")
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 1)
			So(found[0].Id, ShouldEqual, matching.Id)
		})

	})

}

func TestGenericBuildUpdating(t *testing.T) {

	Convey("When updating builds", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		Convey("updating a single build should update the specified build"+
			" in the database", func() {

			buildOne := &Build{
				Id: "buildOne",
			}
			So(buildOne.Insert(), ShouldBeNil)

			So(
				UpdateOneBuild(
					bson.M{
						BuildIdKey: buildOne.Id,
					},
					bson.M{
						"$set": bson.M{
							BuildProjectKey: "blah",
						},
					},
				),
				ShouldBeNil,
			)

			buildOne, err := FindOneBuild(
				bson.M{
					BuildIdKey: buildOne.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(buildOne.Project, ShouldEqual, "blah")

		})

	})

}

func TestBuildUpdateStatus(t *testing.T) {

	Convey("With a build", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		build := &Build{
			Id: "build",
		}
		So(build.Insert(), ShouldBeNil)

		Convey("setting its status should update it both in-memory and"+
			" in the database", func() {

			So(build.UpdateStatus(mci.BuildSucceeded), ShouldBeNil)
			So(build.Status, ShouldEqual, mci.BuildSucceeded)

			build, err := FindOneBuild(
				bson.M{
					BuildIdKey: build.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(build.Status, ShouldEqual, mci.BuildSucceeded)

		})

	})
}

func TestBuildSetPriority(t *testing.T) {

	Convey("With a build", t, func() {

		util.HandleTestingErr(db.ClearCollections(BuildsCollection, TasksCollection), t,
			"Error clearing test collection")

		build := &Build{
			Id: "build",
		}
		So(build.Insert(), ShouldBeNil)

		taskOne := &Task{Id: "taskOne", BuildId: build.Id}
		So(taskOne.Insert(), ShouldBeNil)

		taskTwo := &Task{Id: "taskTwo", BuildId: build.Id}
		So(taskTwo.Insert(), ShouldBeNil)

		taskThree := &Task{Id: "taskThree", BuildId: build.Id}
		So(taskThree.Insert(), ShouldBeNil)

		Convey("setting its priority should update the priority"+
			" of all its tasks in the database", func() {

			So(build.SetPriority(42), ShouldBeNil)

			tasks, err := FindAllTasks(
				bson.M{
					TaskBuildIdKey: build.Id,
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

		util.HandleTestingErr(db.ClearCollections(BuildsCollection, TasksCollection), t,
			"Error clearing test collection")
		build := &Build{
			Id: "build",
			Tasks: []TaskCache{
				TaskCache{
					Id:        "taskOne",
					Status:    mci.TaskSucceeded,
					Activated: true,
				},
				TaskCache{
					Id:        "taskTwo",
					Status:    mci.TaskDispatched,
					Activated: true,
				},
				TaskCache{
					Id:        "taskThree",
					Status:    mci.TaskDispatched,
					Activated: true,
				},
				TaskCache{
					Id:        "taskFour",
					Status:    mci.TaskDispatched,
					Activated: true,
				},
			},
		}
		So(build.Insert(), ShouldBeNil)

		Convey("with task abort should update the status of"+
			" non in-progress tasks and abort in-progress ones", func() {

			taskOne := &Task{
				Id:      "taskOne",
				BuildId: build.Id,
				Status:  mci.TaskSucceeded,
			}
			So(taskOne.Insert(), ShouldBeNil)

			taskTwo := &Task{
				Id:      "taskTwo",
				BuildId: build.Id,
				Status:  mci.TaskDispatched,
			}
			So(taskTwo.Insert(), ShouldBeNil)

			So(RestartBuild(build.Id, true), ShouldBeNil)
			build, err := FindBuild(build.Id)
			So(err, ShouldBeNil)
			So(build.Status, ShouldEqual, mci.BuildCreated)
			So(build.Activated, ShouldEqual, true)
			So(build.Tasks[0].Status, ShouldEqual, mci.TaskUndispatched)
			So(build.Tasks[1].Status, ShouldEqual, mci.TaskDispatched)
			So(build.Tasks[0].Activated, ShouldEqual, true)
			So(build.Tasks[1].Activated, ShouldEqual, true)
			taskOne, err = FindTask("taskOne")
			So(err, ShouldBeNil)
			So(taskOne.Status, ShouldEqual, mci.TaskUndispatched)
			taskTwo, err = FindTask("taskTwo")
			So(err, ShouldBeNil)
			So(taskTwo.Aborted, ShouldEqual, true)
		})

		Convey("without task abort should update the status"+
			" of only those build tasks not in-progress", func() {

			taskThree := &Task{
				Id:      "taskThree",
				BuildId: build.Id,
				Status:  mci.TaskSucceeded,
			}
			So(taskThree.Insert(), ShouldBeNil)

			taskFour := &Task{
				Id:      "taskFour",
				BuildId: build.Id,
				Status:  mci.TaskDispatched,
			}
			So(taskFour.Insert(), ShouldBeNil)

			So(RestartBuild(build.Id, false), ShouldBeNil)
			build, err := FindBuild(build.Id)
			So(err, ShouldBeNil)
			So(err, ShouldBeNil)
			So(build.Status, ShouldEqual, mci.BuildCreated)
			So(build.Activated, ShouldEqual, true)
			So(build.Tasks[2].Status, ShouldEqual, mci.TaskUndispatched)
			So(build.Tasks[3].Status, ShouldEqual, mci.TaskDispatched)
			So(build.Tasks[2].Activated, ShouldEqual, true)
			So(build.Tasks[3].Activated, ShouldEqual, true)
			taskThree, err = FindTask("taskThree")
			So(err, ShouldBeNil)
			So(taskThree.Status, ShouldEqual, mci.TaskUndispatched)
			taskFour, err = FindTask("taskFour")
			So(err, ShouldBeNil)
			So(taskFour.Aborted, ShouldEqual, false)
			So(taskFour.Status, ShouldEqual, mci.TaskDispatched)
		})

	})
}

func TestBuildMarkAborted(t *testing.T) {
	Convey("With a build", t, func() {

		util.HandleTestingErr(db.ClearCollections(BuildsCollection, TasksCollection, version.Collection), t,
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

		build := &Build{
			Id:           "build",
			Activated:    true,
			BuildVariant: "bv",
			Version:      "v",
		}
		So(build.Insert(), ShouldBeNil)

		Convey("when marking it as aborted", func() {

			Convey("it should be deactivated", func() {

				So(AbortBuild(build.Id), ShouldBeNil)
				build, err := FindOneBuild(
					bson.M{
						BuildIdKey: build.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(build.Activated, ShouldBeFalse)
			})

			Convey("all abortable tasks for it should be aborted", func() {

				// insert two abortable tasks and one non-abortable task
				// for the correct build, and one abortable task for
				// a different build

				abortableOne := &Task{
					Id:      "abortableOne",
					BuildId: build.Id,
					Status:  mci.TaskStarted,
				}
				So(abortableOne.Insert(), ShouldBeNil)

				abortableTwo := &Task{
					Id:      "abortableTwo",
					BuildId: build.Id,
					Status:  mci.TaskDispatched,
				}
				So(abortableTwo.Insert(), ShouldBeNil)

				notAbortable := &Task{
					Id:      "notAbortable",
					BuildId: build.Id,
					Status:  mci.TaskSucceeded,
				}
				So(notAbortable.Insert(), ShouldBeNil)

				wrongBuildId := &Task{
					Id:      "wrongBuildId",
					BuildId: "blech",
					Status:  mci.TaskStarted,
				}
				So(wrongBuildId.Insert(), ShouldBeNil)

				// aborting the build should mark only the two abortable tasks
				// with the correct build id as aborted

				So(AbortBuild(build.Id), ShouldBeNil)

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

		util.HandleTestingErr(db.ClearCollections(BuildsCollection, TasksCollection), t,
			"Error clearing test collection")

		Convey("when changing the activated status of the build", func() {

			Convey("the activated status of the build and all undispatched"+
				" tasks that are part of it should be set", func() {

				build := &Build{
					Id:           "build",
					Activated:    true,
					BuildVariant: "bv",
				}
				So(build.Insert(), ShouldBeNil)

				// insert three tasks, with only one of them undispatched and
				// belonging to the correct build

				wrongBuildId := &Task{
					Id:        "wrongBuildId",
					BuildId:   "blech",
					Status:    mci.TaskUndispatched,
					Activated: true,
				}
				So(wrongBuildId.Insert(), ShouldBeNil)

				wrongStatus := &Task{
					Id:        "wrongStatus",
					BuildId:   build.Id,
					Status:    mci.TaskDispatched,
					Activated: true,
				}
				So(wrongStatus.Insert(), ShouldBeNil)

				matching := &Task{
					Id:        "matching",
					BuildId:   build.Id,
					Status:    mci.TaskUndispatched,
					Activated: true,
				}
				So(matching.Insert(), ShouldBeNil)

				So(SetBuildActivation(build.Id, false), ShouldBeNil)
				// the build should have been updated in the db
				build, err := FindOneBuild(
					bson.M{
						BuildIdKey: build.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(build.Activated, ShouldBeFalse)

				// only the matching task should have been updated

				deactivatedTasks, err := FindAllTasks(
					bson.M{
						TaskActivatedKey: false,
					},
					db.NoProjection,
					db.NoSort,
					db.NoSkip,
					db.NoLimit,
				)
				So(err, ShouldBeNil)
				So(len(deactivatedTasks), ShouldEqual, 1)
				So(deactivatedTasks[0].Id, ShouldEqual, matching.Id)

			})

			Convey("all of the undispatched task caches within the build"+
				" should be updated, both in memory and in the"+
				" database", func() {

				build := &Build{
					Id:           "build",
					Activated:    true,
					BuildVariant: "foo",
					Tasks: []TaskCache{
						TaskCache{
							Id:        "tc1",
							Status:    mci.TaskUndispatched,
							Activated: true,
						},
						TaskCache{
							Id:        "tc2",
							Status:    mci.TaskDispatched,
							Activated: true,
						},
						TaskCache{
							Id:        "tc3",
							Status:    mci.TaskUndispatched,
							Activated: true,
						},
					},
				}
				So(build.Insert(), ShouldBeNil)

				t1 := &Task{Id: "tc1", BuildId: build.Id, Status: mci.TaskUndispatched, Activated: true}
				t2 := &Task{Id: "tc2", BuildId: build.Id, Status: mci.TaskDispatched, Activated: true}
				t3 := &Task{Id: "tc3", BuildId: build.Id, Status: mci.TaskUndispatched, Activated: true}
				So(t1.Insert(), ShouldBeNil)
				So(t2.Insert(), ShouldBeNil)
				So(t3.Insert(), ShouldBeNil)

				So(SetBuildActivation(build.Id, false), ShouldBeNil)
				// refresh from the database and check again
				build, err := FindOneBuild(
					bson.M{
						BuildIdKey: build.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(build.Activated, ShouldBeFalse)
				So(build.Tasks[0].Activated, ShouldBeFalse)
				So(build.Tasks[1].Activated, ShouldBeTrue)
				So(build.Tasks[2].Activated, ShouldBeFalse)

			})

		})

	})
}

func TestBuildMarkStarted(t *testing.T) {

	Convey("With a build", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		build := &Build{
			Id:     "build",
			Status: mci.BuildCreated,
		}
		So(build.Insert(), ShouldBeNil)

		Convey("marking it as started should update the status and"+
			" start time, both in memory and in the database", func() {

			startTime := time.Now()
			So(TryMarkBuildStarted(build.Id, startTime), ShouldBeNil)

			// refresh from db and check again
			build, err := FindOneBuild(
				bson.M{
					BuildIdKey: build.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(build.Status, ShouldEqual, mci.BuildStarted)
			So(build.StartTime.Round(time.Second).Equal(
				startTime.Round(time.Second)), ShouldBeTrue)
		})
	})
}

func TestBuildMarkFinished(t *testing.T) {

	Convey("With a build", t, func() {

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		startTime := time.Now()
		build := &Build{
			Id:        "build",
			StartTime: startTime,
		}
		So(build.Insert(), ShouldBeNil)

		Convey("marking it as finished should update the status,"+
			" finish time, and duration, both in memory and in the"+
			" database", func() {

			finishTime := time.Now()
			So(build.MarkFinished(mci.BuildSucceeded, finishTime), ShouldBeNil)
			So(build.Status, ShouldEqual, mci.BuildSucceeded)
			So(build.FinishTime.Equal(finishTime), ShouldBeTrue)
			So(build.TimeTaken, ShouldEqual, finishTime.Sub(startTime))

			// refresh from db and check again

			build, err := FindOneBuild(
				bson.M{
					BuildIdKey: build.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(build.Status, ShouldEqual, mci.BuildSucceeded)
			So(build.FinishTime.Round(time.Second).Equal(
				finishTime.Round(time.Second)), ShouldBeTrue)
			So(build.TimeTaken, ShouldEqual, finishTime.Sub(startTime))

		})

	})

}

func TestCreateBuildFromVersion(t *testing.T) {

	Convey("When creating a build from a version", t, func() {

		util.HandleTestingErr(db.ClearCollections(BuildsCollection, TasksCollection), t,
			"Error clearing test collection")

		// the mock build variant we'll be using. runs all three tasks
		buildVar := BuildVariant{
			Name:        "buildVar",
			DisplayName: "Build Variant",
			Tasks: []BuildVariantTask{
				BuildVariantTask{
					Name: "taskA",
				},
				BuildVariantTask{
					Name: "taskB",
				},
				BuildVariantTask{
					Name: "taskC",
				},
				BuildVariantTask{
					Name: "taskD",
				},
			},
		}

		// the mock project we'll be using.  the mock tasks are:
		// taskOne - no dependencies
		// taskTwo - depends on taskOne
		// taskThree - depends on taskOne and taskTwo
		project := &Project{
			Owner:    "branchOwner",
			Repo:     "repo",
			RepoKind: GithubRepoType,
			Branch:   "project",
			Tasks: []ProjectTask{
				ProjectTask{
					Name:      "taskA",
					DependsOn: []TaskDependency{},
				},
				ProjectTask{
					Name: "taskB",
					DependsOn: []TaskDependency{
						TaskDependency{
							Name: "taskA",
						},
					},
				},
				ProjectTask{
					Name: "taskC",
					DependsOn: []TaskDependency{
						TaskDependency{
							Name: "taskA",
						},
						TaskDependency{
							Name: "taskB",
						},
					},
				},
				ProjectTask{
					Name: "taskD",
					DependsOn: []TaskDependency{
						TaskDependency{
							Name: AllDependencies,
						},
					},
				},
			},
			BuildVariants: []BuildVariant{
				buildVar,
			},
		}

		// the mock version we'll be using
		v := &version.Version{
			Id:                  "versionId",
			CreateTime:          time.Now(),
			Revision:            "foobar",
			RevisionOrderNumber: 500,
			Requester:           mci.RepotrackerVersionRequester,
			BuildVariants: []version.BuildStatus{
				{
					BuildVariant: buildVar.Name,
					Activated:    false,
				},
			},
		}

		Convey("if a non-existent build variant is passed in, an error should"+
			" be returned", func() {

			buildId, err := CreateBuildFromVersion(project, v, "blecch", false, []string{})
			So(err, ShouldNotBeNil)
			So(buildId, ShouldEqual, "")

		})

		Convey("if no task names are passed in to be used, all of the default"+
			" tasks for the build variant should be created", func() {

			buildId, err := CreateBuildFromVersion(project, v, buildVar.Name, false, nil)
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

		})

		Convey("if a non-empty list of task names is passed in, only the"+
			" specified tasks should be created", func() {

			buildId, err := CreateBuildFromVersion(project, v, buildVar.Name, false,
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

			buildId, err := CreateBuildFromVersion(project, v, buildVar.Name, false, nil)
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
			build, err := FindBuild(buildId)
			So(err, ShouldBeNil)
			So(len(build.Tasks), ShouldEqual, 4)

			// make sure the task caches are correct.  they should also appear
			// in the same order that they appear in the project file
			So(build.Tasks[0].Id, ShouldNotEqual, "")
			So(build.Tasks[0].DisplayName, ShouldEqual, "taskA")
			So(build.Tasks[0].Status, ShouldEqual, mci.TaskUndispatched)
			So(build.Tasks[1].Id, ShouldNotEqual, "")
			So(build.Tasks[1].DisplayName, ShouldEqual, "taskB")
			So(build.Tasks[1].Status, ShouldEqual, mci.TaskUndispatched)
			So(build.Tasks[2].Id, ShouldNotEqual, "")
			So(build.Tasks[2].DisplayName, ShouldEqual, "taskC")
			So(build.Tasks[2].Status, ShouldEqual, mci.TaskUndispatched)
			So(build.Tasks[3].Id, ShouldNotEqual, "")
			So(build.Tasks[3].DisplayName, ShouldEqual, "taskD")
			So(build.Tasks[3].Status, ShouldEqual, mci.TaskUndispatched)

		})

		Convey("all of the tasks created should have the dependencies"+
			" specified in the project", func() {

			buildId, err := CreateBuildFromVersion(project, v, buildVar.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

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

			So(len(tasks[0].DependsOn), ShouldEqual, 0)
			So(tasks[1].DependsOn, ShouldResemble,
				[]string{tasks[0].Id})
			So(tasks[2].DependsOn, ShouldResemble,
				[]string{tasks[0].Id, tasks[1].Id})
			So(tasks[3].DependsOn, ShouldResemble,
				[]string{tasks[0].Id, tasks[1].Id, tasks[2].Id})

		})

		Convey("all of the build's essential fields should be set"+
			" correctly", func() {

			buildId, err := CreateBuildFromVersion(project, v, buildVar.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the build from the db
			build, err := FindBuild(buildId)
			So(err, ShouldBeNil)

			// verify all the fields are set appropriately
			So(len(build.Tasks), ShouldEqual, 4)
			So(build.CreateTime.Truncate(time.Second), ShouldResemble,
				v.CreateTime.Truncate(time.Second))
			So(build.PushTime.Truncate(time.Second), ShouldResemble,
				v.CreateTime.Truncate(time.Second))
			So(build.Activated, ShouldEqual, v.BuildVariants[0].Activated)
			So(build.Project, ShouldEqual, project.Identifier)
			So(build.Revision, ShouldEqual, v.Revision)
			So(build.Status, ShouldEqual, mci.BuildCreated)
			So(build.BuildVariant, ShouldEqual, buildVar.Name)
			So(build.Version, ShouldEqual, v.Id)
			So(build.DisplayName, ShouldEqual, buildVar.DisplayName)
			So(build.RevisionOrderNumber, ShouldEqual, v.RevisionOrderNumber)
			So(build.Requester, ShouldEqual, v.Requester)

		})

		Convey("all of the tasks' essential fields should be set"+
			" correctly", func() {

			buildId, err := CreateBuildFromVersion(project, v, buildVar.Name, false, nil)
			So(err, ShouldBeNil)
			So(buildId, ShouldNotEqual, "")

			// find the build from the db
			build, err := FindBuild(buildId)
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
			So(tasks[0].BuildVariant, ShouldEqual, buildVar.Name)
			So(tasks[0].CreateTime.Truncate(time.Second), ShouldResemble,
				build.CreateTime.Truncate(time.Second))
			So(tasks[0].PushTime.Truncate(time.Second), ShouldResemble,
				build.PushTime.Truncate(time.Second))
			So(tasks[0].Status, ShouldEqual, mci.TaskUndispatched)
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
			So(tasks[1].BuildVariant, ShouldEqual, buildVar.Name)
			So(tasks[1].CreateTime.Truncate(time.Second), ShouldResemble,
				build.CreateTime.Truncate(time.Second))
			So(tasks[1].PushTime.Truncate(time.Second), ShouldResemble,
				build.PushTime.Truncate(time.Second))
			So(tasks[1].Status, ShouldEqual, mci.TaskUndispatched)
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
			So(tasks[2].BuildVariant, ShouldEqual, buildVar.Name)
			So(tasks[2].CreateTime.Truncate(time.Second), ShouldResemble,
				build.CreateTime.Truncate(time.Second))
			So(tasks[2].PushTime.Truncate(time.Second), ShouldResemble,
				build.PushTime.Truncate(time.Second))
			So(tasks[2].Status, ShouldEqual, mci.TaskUndispatched)
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
			So(tasks[3].BuildVariant, ShouldEqual, buildVar.Name)
			So(tasks[3].CreateTime.Truncate(time.Second), ShouldResemble,
				build.CreateTime.Truncate(time.Second))
			So(tasks[3].PushTime.Truncate(time.Second), ShouldResemble,
				build.PushTime.Truncate(time.Second))
			So(tasks[3].Status, ShouldEqual, mci.TaskUndispatched)
			So(tasks[3].Activated, ShouldEqual, build.Activated)
			So(tasks[3].RevisionOrderNumber, ShouldEqual, build.RevisionOrderNumber)
			So(tasks[3].Requester, ShouldEqual, build.Requester)
			So(tasks[3].Version, ShouldEqual, v.Id)
			So(tasks[3].Revision, ShouldEqual, v.Revision)
			So(tasks[3].Project, ShouldEqual, project.Identifier)
		})

		Convey("if the activated flag is set, the build and all its tasks should be activated",
			func() {

				buildId, err := CreateBuildFromVersion(project, v, buildVar.Name, true, nil)
				So(err, ShouldBeNil)
				So(buildId, ShouldNotEqual, "")

				// find the build from the db
				build, err := FindBuild(buildId)
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
				So(tasks[0].BuildVariant, ShouldEqual, buildVar.Name)
				So(tasks[0].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[0].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[0].Status, ShouldEqual, mci.TaskUndispatched)
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
				So(tasks[1].BuildVariant, ShouldEqual, buildVar.Name)
				So(tasks[1].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[1].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[1].Status, ShouldEqual, mci.TaskUndispatched)
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
				So(tasks[2].BuildVariant, ShouldEqual, buildVar.Name)
				So(tasks[2].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[2].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[2].Status, ShouldEqual, mci.TaskUndispatched)
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
				So(tasks[3].BuildVariant, ShouldEqual, buildVar.Name)
				So(tasks[3].CreateTime.Truncate(time.Second), ShouldResemble,
					build.CreateTime.Truncate(time.Second))
				So(tasks[3].PushTime.Truncate(time.Second), ShouldResemble,
					build.PushTime.Truncate(time.Second))
				So(tasks[3].Status, ShouldEqual, mci.TaskUndispatched)
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

		util.HandleTestingErr(db.Clear(BuildsCollection), t, "Error clearing"+
			" '%v' collection", BuildsCollection)

		build := &Build{
			Id: "build",
		}
		So(build.Insert(), ShouldBeNil)

		Convey("deleting it should remove it and all its associated"+
			" tasks from the database", func() {

			util.HandleTestingErr(db.Clear(TasksCollection), t, "Error"+
				" clearing '%v' collection", TasksCollection)

			// insert two tasks that are part of the build, and one that isn't

			matchingTaskOne := &Task{
				Id:      "matchingOne",
				BuildId: build.Id,
			}
			So(matchingTaskOne.Insert(), ShouldBeNil)

			matchingTaskTwo := &Task{
				Id:      "matchingTwo",
				BuildId: build.Id,
			}
			So(matchingTaskTwo.Insert(), ShouldBeNil)

			nonMatchingTask := &Task{
				Id:      "nonMatching",
				BuildId: "blech",
			}
			So(nonMatchingTask.Insert(), ShouldBeNil)

			// delete the build, make sure only it and its tasks are deleted

			So(DeleteBuild(build.Id), ShouldBeNil)

			build, err := FindOneBuild(
				bson.M{
					BuildIdKey: build.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(build, ShouldBeNil)

			matchingTasks, err := FindAllTasks(
				bson.M{
					TaskBuildIdKey: "build",
				},
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
