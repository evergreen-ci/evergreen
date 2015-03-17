package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model/version"
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

			versionOne := &version.Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			pushLog, err := FindPushLogAfter(fileLoc, versionOne.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for an older commit,"+
			" nothing should be returned", func() {

			versionOne := &version.Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			versionTwo := &version.Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 600,
			}

			pushLog, err := FindPushLogAfter(fileLoc, versionTwo.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for the same commit,"+
			" it should be returned", func() {

			versionOne := &version.Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			pushLog, err := FindPushLogAfter(fileLoc, versionOne.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

		Convey("if there is a push at the same location for a newer commit,"+
			" it should be returned", func() {

			versionOne := &version.Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			versionTwo := &version.Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 400,
			}

			pushLog, err := FindPushLogAfter(fileLoc, versionTwo.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

	})

}
