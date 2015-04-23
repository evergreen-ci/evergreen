package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	pushTestConfig = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(pushTestConfig))
}

func TestFindPushLogAfter(t *testing.T) {

	Convey("When checking for duplicate pushes", t, func() {

		util.HandleTestingErr(db.Clear(PushlogCollection), t, "Error clearing"+
			" '%v' collection", PushlogCollection)

		fileLoc := "s3://test/location"

		Convey("if there is no conflicting push, then nothing should be"+
			" returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			pushLog, err := versionOne.FindPushLogAfter(fileLoc)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for an older commit,"+
			" nothing should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			versionTwo := &Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 600,
			}

			pushLog, err := versionTwo.FindPushLogAfter(fileLoc)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for the same commit,"+
			" it should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			pushLog, err := versionOne.FindPushLogAfter(fileLoc)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

		Convey("if there is a push at the same location for a newer commit,"+
			" it should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			versionTwo := &Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 400,
			}

			pushLog, err := versionTwo.FindPushLogAfter(fileLoc)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

	})

}
