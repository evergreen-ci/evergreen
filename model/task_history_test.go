package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskHistory(t *testing.T) {

	Convey("With a task history iterator", t, func() {

		buildVariants := []string{"bv_0", "bv_1", "bv_2"}
		projectName := "project"
		taskHistoryIterator := NewTaskHistoryIterator("compile",
			buildVariants, projectName)

		Convey("when finding task history items", func() {
			require.NoError(t, db.ClearCollections(VersionCollection, task.Collection))

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
					Requester:           evergreen.RepotrackerVersionRequester,
					Identifier:          projectToUse,
				}

				require.NoError(t, ver.Insert(),
					"Error inserting version")
				for j := 0; j < 3; j++ {
					newTask := &task.Task{
						Id:                  fmt.Sprintf("t%v_%v", i, j),
						BuildVariant:        fmt.Sprintf("bv_%v", j),
						DisplayName:         "compile",
						RevisionOrderNumber: i,
						Revision:            vid,
						Requester:           evergreen.RepotrackerVersionRequester,
						Project:             projectToUse,
					}
					require.NoError(t, newTask.Insert(),
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

				vBefore, err := VersionFindOne(VersionById("v15"))
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

func TestTaskHistoryPickaxe(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, RepositoriesCollection))
	assert := assert.New(t)
	proj := Project{
		Identifier: "proj",
	}
	t1 := task.Task{
		Id:                  "t1",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 1,
	}
	t2 := task.Task{
		Id:                  "t2",
		Project:             proj.Identifier,
		DisplayName:         "notMatchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 2,
	}
	t3 := task.Task{
		Id:                  "t3",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 3,
	}
	t4 := task.Task{
		Id:                  "t4",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 4,
	}
	assert.NoError(t1.Insert())
	assert.NoError(t2.Insert())
	assert.NoError(t3.Insert())
	assert.NoError(t4.Insert())
	for i := 0; i < 5; i++ {
		_, err := GetNewRevisionOrderNumber(proj.Identifier)
		assert.NoError(err)
	}

	// Test that a basic case returns the correct results.
	params := PickaxeParams{
		Project:       &proj,
		TaskName:      "matchingName",
		NewestOrder:   4,
		OldestOrder:   1,
		BuildVariants: []string{"bv"},
	}
	results, err := TaskHistoryPickaxe(params)
	require.NoError(t, err)
	require.Len(t, results, 3)
}
