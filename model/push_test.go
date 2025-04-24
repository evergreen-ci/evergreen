package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestFindPushLogAfter(t *testing.T) {

	Convey("When checking for duplicate pushes", t, func() {

		require.NoError(t, db.Clear(PushlogCollection))

		fileLoc := "s3://test/location"

		Convey("if there is no conflicting push, then nothing should be"+
			" returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			pushLog, err := FindPushLogAfter(t.Context(), fileLoc, versionOne.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for an older commit,"+
			" nothing should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &task.Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(t.Context()), ShouldBeNil)

			versionTwo := &Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 600,
			}

			pushLog, err := FindPushLogAfter(t.Context(), fileLoc, versionTwo.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldBeNil)

		})

		Convey("if there is a push at the same location for the same commit,"+
			" it should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &task.Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(t.Context()), ShouldBeNil)

			pushLog, err := FindPushLogAfter(t.Context(), fileLoc, versionOne.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

		Convey("if there is a push at the same location for a newer commit,"+
			" it should be returned", func() {

			versionOne := &Version{
				Id:                  "versionIdOne",
				RevisionOrderNumber: 500,
			}

			task := &task.Task{
				Id: "taskId",
			}

			pushLog := NewPushLog(versionOne, task, fileLoc)
			So(pushLog.Insert(t.Context()), ShouldBeNil)

			versionTwo := &Version{
				Id:                  "versionIdTwo",
				RevisionOrderNumber: 400,
			}

			pushLog, err := FindPushLogAfter(t.Context(), fileLoc, versionTwo.RevisionOrderNumber)
			So(err, ShouldBeNil)
			So(pushLog, ShouldNotBeNil)

		})

	})

}
