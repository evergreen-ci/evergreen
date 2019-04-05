package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	runtimeId     = "runtimeId"
	allRuntimeIds = []string{runtimeId + "1", runtimeId + "2", runtimeId + "3"}
	testConfig    = testutil.TestConfig()
)

func resetCollection() {
	grip.Error(db.Clear(RuntimesCollection))
}

// Test that Upserts happen properly
func TestRuntimeUpsert(t *testing.T) {
	Convey("When updating runtimes in an empty collection", t, func() {
		Convey("An update should insert a document if that id is not"+
			" present", func() {
			found, err := FindEveryProcessRuntime()
			So(len(found), ShouldEqual, 0)
			So(err, ShouldBeNil)

			So(SetProcessRuntimeCompleted(allRuntimeIds[0], time.Duration(0)), ShouldBeNil)
			found, err = FindEveryProcessRuntime()
			So(len(found), ShouldEqual, 1)
			So(err, ShouldBeNil)

			So(SetProcessRuntimeCompleted(allRuntimeIds[1], time.Duration(0)), ShouldBeNil)
			found, err = FindEveryProcessRuntime()
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
		})

		Convey("An update should not create new docs if the Id already"+
			" exists", func() {
			So(SetProcessRuntimeCompleted(allRuntimeIds[0], time.Duration(0)), ShouldBeNil)
			So(SetProcessRuntimeCompleted(allRuntimeIds[1], time.Duration(0)), ShouldBeNil)
			found, err := FindEveryProcessRuntime()
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
		})

		Reset(func() {
			resetCollection()
		})
	})
}

// Tests that time is properly updated when Report is called
func TestRuntimeTimeUpdate(t *testing.T) {
	Convey("When a new time is reported", t, func() {
		Convey("The time document should save the time at which it was"+
			" updated", func() {
			So(SetProcessRuntimeCompleted(allRuntimeIds[0], time.Duration(0)), ShouldBeNil)
			time.Sleep(time.Millisecond)
			testCutoff := time.Now()
			time.Sleep(time.Millisecond)
			runtime, err := FindOneProcessRuntime(bson.M{}, bson.M{})
			So(err, ShouldBeNil)
			So(runtime, ShouldNotBeNil)
			So(runtime.FinishedAt, ShouldHappenBefore, testCutoff)

			So(SetProcessRuntimeCompleted(allRuntimeIds[0], time.Duration(0)), ShouldBeNil)
			runtime, err = FindOneProcessRuntime(bson.M{}, bson.M{})
			So(err, ShouldBeNil)
			So(runtime, ShouldNotBeNil)
			So(runtime.FinishedAt, ShouldHappenAfter, testCutoff)
		})

		Reset(func() {
			resetCollection()
		})
	})
}

// Tests that only runtimes before the given cutoff are returned
func TestFindLateRuntimes(t *testing.T) {
	Convey("When finding late runtime reports", t, func() {
		So(SetProcessRuntimeCompleted(allRuntimeIds[0], time.Duration(0)), ShouldBeNil)
		So(SetProcessRuntimeCompleted(allRuntimeIds[1], time.Duration(0)), ShouldBeNil)
		time.Sleep(time.Millisecond)
		testCutoff := time.Now()
		time.Sleep(time.Millisecond)
		So(SetProcessRuntimeCompleted(allRuntimeIds[2], time.Duration(0)), ShouldBeNil)

		Convey("Only times committed before the cutoff are returned", func() {
			all, err := FindEveryProcessRuntime()
			So(len(all), ShouldEqual, 3)
			So(err, ShouldBeNil)

			lateRuntimes, err := FindAllLateProcessRuntimes(testCutoff)
			So(err, ShouldBeNil)
			So(len(lateRuntimes), ShouldEqual, 2)
		})

		Convey("And when all times are more recent than the cutoff, none are returned", func() {
			So(SetProcessRuntimeCompleted(allRuntimeIds[0], time.Duration(0)), ShouldBeNil)
			So(SetProcessRuntimeCompleted(allRuntimeIds[1], time.Duration(0)), ShouldBeNil)
			lateRuntimes, err := FindAllLateProcessRuntimes(testCutoff)
			So(err, ShouldBeNil)
			So(len(lateRuntimes), ShouldEqual, 0)
		})

		Reset(func() {
			resetCollection()
		})
	})
}

func TestRuntimeDuration(t *testing.T) {
	Convey("When updating a runtime duration", t, func() {
		Convey("The proper value and type is returned", func() {
			err := SetProcessRuntimeCompleted(allRuntimeIds[0], time.Minute)
			So(err, ShouldBeNil)

			runtime, err := FindProcessRuntime(allRuntimeIds[0])
			So(runtime, ShouldNotBeNil)
			So(runtime.Runtime, ShouldEqual, time.Minute)
			So(err, ShouldBeNil)
		})

		Reset(func() {
			resetCollection()
		})
	})

}
