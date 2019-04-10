package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

var pushTestConfig = testutil.TestConfig()

func TestFindPushLogAfter(t *testing.T) {

	Convey("When checking for duplicate pushes", t, func() {

		require.NoError(t, db.Clear(PushlogCollection), "Error clearing '%v' collection", PushlogCollection)

		fileLoc := "s3://test/location"

		Convey("if there is no conflicting push, then nothing should be"+
			" returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			pushLog, err := FindPushLogAfter(fileLoc, versionOne.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for an older commit,"+
			" nothing should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			t := &task.Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, t, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			versionTwo := &Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 600,
			}

			pushLog, err := FindPushLogAfter(fileLoc, versionTwo.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for the same commit,"+
			" it should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			t := &task.Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, t, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			pushLog, err := FindPushLogAfter(fileLoc, versionOne.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

		Convey("if there is a push at the same location for a newer commit,"+
			" it should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			t := &task.Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, t, fileLoc)
			So(pushLog.Insert(), ShouldBeNil)

			versionTwo := &Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 400,
			}

			pushLog, err := FindPushLogAfter(fileLoc, versionTwo.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

	})

}
