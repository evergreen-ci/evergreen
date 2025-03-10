package build

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func init() {
	testutil.Setup()
}

func buildIdInSlice(builds []Build, id string) bool {
	for _, build := range builds {
		if build.Id == id {
			return true
		}
	}
	return false
}

func TestGenericBuildFinding(t *testing.T) {

	Convey("When finding builds", t, func() {
		require.NoError(t, db.Clear(Collection))

		Convey("when finding one build", func() {
			Convey("the matching build should be returned", func() {
				buildOne := &Build{Id: "buildOne"}
				So(buildOne.Insert(), ShouldBeNil)

				buildTwo := &Build{Id: "buildTwo"}
				So(buildTwo.Insert(), ShouldBeNil)

				found, err := FindOne(ById(buildOne.Id))
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, buildOne.Id)
			})
		})

		Convey("when finding multiple builds", func() {
			Convey("a slice of all of the matching builds should be returned", func() {

				buildOne := &Build{Id: "buildOne", Project: "b1"}
				So(buildOne.Insert(), ShouldBeNil)

				buildTwo := &Build{Id: "buildTwo", Project: "b1"}
				So(buildTwo.Insert(), ShouldBeNil)

				buildThree := &Build{Id: "buildThree", Project: "b2"}
				So(buildThree.Insert(), ShouldBeNil)

				found, err := Find(ByProject("b1"))
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, 2)
				So(buildIdInSlice(found, buildOne.Id), ShouldBeTrue)
				So(buildIdInSlice(found, buildTwo.Id), ShouldBeTrue)
			})
		})
	})
}

func TestRecentlyFinishedBuilds(t *testing.T) {

	Convey("When finding all recently finished builds", t, func() {

		require.NoError(t, db.Clear(Collection))

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

			found, err := Find(ByFinishedAfter(finishTime, "project1", "r1"))
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

			found, err := Find(ByFinishedAfter(finishTime, "project1", "r1"))
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

			found, err := Find(ByFinishedAfter(finishTime, "project1", "r1"))
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 1)
			So(found[0].Id, ShouldEqual, matching.Id)
		})

	})

}

func TestGenericBuildUpdating(t *testing.T) {
	Convey("When updating builds", t, func() {

		Reset(func() {
			require.NoError(t, db.Clear(Collection))
		})

		Convey("updating a single build should update the specified build"+
			" in the database", func() {

			buildOne := &Build{Id: "buildOne"}
			So(buildOne.Insert(), ShouldBeNil)

			err := UpdateOne(
				t.Context(),
				bson.M{IdKey: buildOne.Id},
				bson.M{"$set": bson.M{ProjectKey: "blah"}},
			)
			So(err, ShouldBeNil)

			buildOne, err = FindOne(ById(buildOne.Id))
			So(err, ShouldBeNil)
			So(buildOne.Project, ShouldEqual, "blah")
		})
	})
}

func TestBuildUpdateStatus(t *testing.T) {
	Convey("With a build", t, func() {

		Reset(func() {
			require.NoError(t, db.Clear(Collection))
		})

		var err error
		build := &Build{Id: "build"}
		So(build.Insert(), ShouldBeNil)

		Convey("setting its status should update it both in-memory and"+
			" in the database", func() {
			So(build.UpdateStatus(t.Context(), evergreen.BuildSucceeded), ShouldBeNil)
			So(build.Status, ShouldEqual, evergreen.BuildSucceeded)
			build, err = FindOne(ById(build.Id))
			So(err, ShouldBeNil)
			So(build.Status, ShouldEqual, evergreen.BuildSucceeded)
		})
	})
}

func TestBuildSetHasUnfinishedEssentialTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, b Build){
		"FailsWithNonexistentBuild": func(t *testing.T, b Build) {
			assert.Error(t, b.SetHasUnfinishedEssentialTask(t.Context(), true))
			assert.False(t, b.HasUnfinishedEssentialTask)
		},
		"NoopsWithSameValue": func(t *testing.T, b Build) {
			require.NoError(t, b.Insert())
			require.NoError(t, b.SetHasUnfinishedEssentialTask(t.Context(), false))
			assert.False(t, b.HasUnfinishedEssentialTask)

			dbBuild, err := FindOneId(b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.False(t, dbBuild.HasUnfinishedEssentialTask)
		},
		"SetsFlag": func(t *testing.T, b Build) {
			require.NoError(t, b.Insert())
			require.NoError(t, b.SetHasUnfinishedEssentialTask(t.Context(), true))
			assert.True(t, b.HasUnfinishedEssentialTask)

			dbBuild, err := FindOneId(b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.True(t, dbBuild.HasUnfinishedEssentialTask)
		},
		"ClearsFlag": func(t *testing.T, b Build) {
			b.HasUnfinishedEssentialTask = true
			require.NoError(t, b.Insert())
			require.NoError(t, b.SetHasUnfinishedEssentialTask(t.Context(), false))
			assert.False(t, b.HasUnfinishedEssentialTask)

			dbBuild, err := FindOneId(b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.False(t, dbBuild.HasUnfinishedEssentialTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			tCase(t, Build{
				Id:                         "build_id",
				HasUnfinishedEssentialTask: false,
			})
		})
	}
}

func TestBulkInsert(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	builds := Builds{
		&Build{Id: "b1"},
		&Build{Id: "b1"},
		&Build{Id: "b2"},
		&Build{Id: "b3"},
	}

	assert.Error(t, builds.InsertMany(context.Background(), true))
	dbBuilds, err := Find(db.Q{})
	assert.NoError(t, err)
	assert.Len(t, dbBuilds, 1)

	assert.NoError(t, db.ClearCollections(Collection))
	assert.Error(t, builds.InsertMany(context.Background(), false))
	dbBuilds, err = Find(db.Q{})
	assert.NoError(t, err)
	assert.Len(t, dbBuilds, 3)
}

func TestGetPRNotificationDescription(t *testing.T) {
	b := &Build{
		Id:           mgobson.NewObjectId().Hex(),
		BuildVariant: "testvariant",
		Version:      "testversion",
		Status:       evergreen.BuildFailed,
		StartTime:    time.Time{},
		FinishTime:   time.Time{}.Add(10 * time.Second),
	}

	t.Run("NoTasksInBuildReturnsNoTasks", func(t *testing.T) {
		assert.Equal(t, "no tasks were run", b.GetPRNotificationDescription(nil))
	})
	t.Run("UnscheduledTasksInBuildReturnsNoTasks", func(t *testing.T) {
		tasks := []task.Task{{Status: evergreen.TaskUndispatched, Activated: false}}
		assert.Equal(t, "no tasks were run", b.GetPRNotificationDescription(tasks))
	})
	t.Run("OneSuccessfulTaskReturnsOneSuccessAndNoFailures", func(t *testing.T) {
		tasks := []task.Task{
			{
				Status: evergreen.TaskSucceeded,
			},
		}
		assert.Equal(t, "1 succeeded, none failed in 10s", b.GetPRNotificationDescription(tasks))
	})
	t.Run("OneFailedTasksReturnsNoSuccessAndOneFailure", func(t *testing.T) {
		tasks := []task.Task{
			{
				Status: evergreen.TaskFailed,
			},
		}
		assert.Equal(t, "none succeeded, 1 failed in 10s", b.GetPRNotificationDescription(tasks))
	})
	t.Run("UnscheduledEssentialTasksThatWillNotRunReturnsIncompleteBuild", func(t *testing.T) {
		tasks := []task.Task{
			{Status: evergreen.TaskSucceeded},
			{Status: evergreen.TaskFailed},
			{Status: evergreen.TaskUndispatched, IsEssentialToSucceed: true, Activated: false},
		}
		assert.Equal(t, "1 succeeded, 1 failed, 1 essential task(s) not scheduled in 10s", b.GetPRNotificationDescription(tasks))
	})
	t.Run("MixOfUnscheduledEssentialTasksAndRunningTasksReturnsRunningBuild", func(t *testing.T) {
		tasks := []task.Task{
			{Status: evergreen.TaskStarted},
			{Status: evergreen.TaskFailed},
			{Status: evergreen.TaskUndispatched, IsEssentialToSucceed: true, Activated: false},
		}
		assert.Equal(t, "tasks are running", b.GetPRNotificationDescription(tasks))
	})
	t.Run("RunningEssentialTasksThatWillRunReturnsTasksRunning", func(t *testing.T) {
		tasks := []task.Task{
			{Status: evergreen.TaskSucceeded},
			{Status: evergreen.TaskFailed},
			{Status: evergreen.TaskUndispatched, IsEssentialToSucceed: true, Activated: true},
		}
		assert.Equal(t, "tasks are running", b.GetPRNotificationDescription(tasks))
	})
	t.Run("MixOfSuccessfulAndFailedAndUnscheduledEssentialTasksReturnsFailedBuild", func(t *testing.T) {
		tasks := []task.Task{
			{Status: evergreen.TaskSucceeded},
			{Status: evergreen.TaskFailed},
			{Status: evergreen.TaskUndispatched, IsEssentialToSucceed: true, Activated: false},
		}
		assert.Equal(t, "1 succeeded, 1 failed, 1 essential task(s) not scheduled in 10s", b.GetPRNotificationDescription(tasks))
	})
	t.Run("ScheduledTasksThatWillRunReturnsTasksRunning", func(t *testing.T) {
		tasks := []task.Task{
			{Status: evergreen.TaskUndispatched, Activated: true},
		}
		assert.Equal(t, "tasks are running", b.GetPRNotificationDescription(tasks))
	})
}
