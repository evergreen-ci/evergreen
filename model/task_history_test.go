package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	taskHistoryTestConfig = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(taskHistoryTestConfig))
}

func TestTaskHistory(t *testing.T) {

	Convey("With a task history iterator", t, func() {

		buildVariants := []string{"bv_0", "bv_1", "bv_2"}
		projectName := "project"
		taskHistoryIterator := NewTaskHistoryIterator(mci.CompileStage,
			buildVariants, projectName)

		Convey("when finding task history items", func() {

			util.HandleTestingErr(db.ClearCollections(VersionsCollection, TasksCollection),
				t, "Error clearing test collections")

			for i := 10; i < 20; i++ {
				projectToUse := projectName
				if i == 14 {
					projectToUse = "otherBranch"
				}

				vid := fmt.Sprintf("v%v", i)
				ver := &Version{
					Id:                  vid,
					RevisionOrderNumber: i,
					Revision:            vid,
					Requester:           mci.RepotrackerVersionRequester,
					Project:             projectToUse,
				}

				util.HandleTestingErr(ver.Insert(), t,
					"Error inserting version")
				for j := 0; j < 3; j++ {
					task := &Task{
						Id:                  fmt.Sprintf("t%v_%v", i, j),
						BuildVariant:        fmt.Sprintf("bv_%v", j),
						DisplayName:         mci.CompileStage,
						RevisionOrderNumber: i,
						Revision:            vid,
						Requester:           mci.RepotrackerVersionRequester,
						Project:             projectToUse,
					}
					util.HandleTestingErr(task.Insert(), t,
						"Error inserting task")
				}

			}

			Convey("the specified number of task history items should be"+
				" fetched, starting at the specified version", func() {

				taskHistoryChunk, err := taskHistoryIterator.GetChunk(nil, 5, 0, false)
				versions := taskHistoryChunk.Versions
				tasks := taskHistoryChunk.Tasks
				So(err, ShouldBeNil)
				So(taskHistoryChunk.Exhausted.Before, ShouldBeFalse)
				So(taskHistoryChunk.Exhausted.After, ShouldBeTrue)
				So(len(versions), ShouldEqual, 5)
				So(len(tasks), ShouldEqual, len(versions))
				So(versions[0].Id, ShouldEqual, tasks[0]["_id"])
				So(versions[len(versions)-1].Id, ShouldEqual, "v15")
				So(tasks[len(tasks)-1]["_id"], ShouldEqual,
					versions[len(versions)-1].Id)

			})

			Convey("tasks from a different project should be filtered"+
				" out", func() {

				vBefore, err := FindVersion("v15")
				So(err, ShouldBeNil)

				taskHistoryChunk, err := taskHistoryIterator.GetChunk(vBefore, 5, 0, false)
				versions := taskHistoryChunk.Versions
				tasks := taskHistoryChunk.Tasks
				So(err, ShouldBeNil)
				So(taskHistoryChunk.Exhausted.Before, ShouldBeTrue)
				So(taskHistoryChunk.Exhausted.After, ShouldBeFalse)
				// Should skip 14 because its in another project
				So(versions[0].Id, ShouldEqual, "v13")
				So(versions[0].Id, ShouldEqual, tasks[0]["_id"])
				So(len(tasks), ShouldEqual, 4)
				So(len(tasks), ShouldEqual, len(versions))
				So(tasks[len(tasks)-1]["_id"], ShouldEqual,
					versions[len(versions)-1].Id)

			})

		})

	})

}
