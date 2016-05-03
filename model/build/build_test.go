package build

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
)

var (
	buildTestConfig = evergreen.TestConfig()
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

func TestGenericBuildFinding(t *testing.T) {

	Convey("When finding builds", t, func() {
		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

		Convey("when finding one build", func() {
			Convey("the matching build should be returned", func() {
				buildOne := &Build{Id: "buildOne"}
				So(buildOne.Insert(), ShouldBeNil)

				buildTwo := &Build{Id: "buildTwo"}
				So(buildTwo.Insert(), ShouldBeNil)

				found, err := FindOne(ById(buildOne.Id))
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, buildOne.Id)

				found, err = FindOne(ById(buildTwo.Id))
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

func TestFindIntermediateBuilds(t *testing.T) {

	Convey("When finding intermediate builds", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

		// the two builds to use as endpoints

		currBuild := &Build{
			Id:                  "curr",
			RevisionOrderNumber: 1000,
			BuildVariant:        "bv1",
			Requester:           "r1",
			Project:             "project1",
		}
		So(currBuild.Insert(), ShouldBeNil)

		prevBuild := &Build{Id: "prev", RevisionOrderNumber: 10}
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

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

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

			found, err := currBuild.PreviousActivated("project1", "r1")
			So(err, ShouldBeNil)
			So(found.Id, ShouldEqual, matchingHigher.Id)
		})
	})
}

func TestRecentlyFinishedBuilds(t *testing.T) {

	Convey("When finding all recently finished builds", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
			" '%v' collection", Collection)

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
			testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing '%v' collection", Collection)
		})

		Convey("updating a single build should update the specified build"+
			" in the database", func() {

			buildOne := &Build{Id: "buildOne"}
			So(buildOne.Insert(), ShouldBeNil)

			err := UpdateOne(
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
			testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing '%v' collection", Collection)
		})

		build := &Build{Id: "build"}
		So(build.Insert(), ShouldBeNil)

		Convey("setting its status should update it both in-memory and"+
			" in the database", func() {
			So(build.UpdateStatus(evergreen.BuildSucceeded), ShouldBeNil)
			So(build.Status, ShouldEqual, evergreen.BuildSucceeded)
			build, err := FindOne(ById(build.Id))
			So(err, ShouldBeNil)
			So(build.Status, ShouldEqual, evergreen.BuildSucceeded)
		})
	})
}
