package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
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

func TestSetDefaultsAndValidate(t *testing.T) {
	Convey("With various test parameters", t, func() {
		Convey("an empty test history parameters struct should not be valid", func() {
			params := TestHistoryParameters{}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct without a project should not be valid", func() {
			params := TestHistoryParameters{
				TestNames: []string{"blah"},
				TaskNames: []string{"blah"},
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct without a set of test or task names should not be valid", func() {
			params := TestHistoryParameters{
				Project: "p",
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct without a task status should have a default set", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TestStatuses: []string{evergreen.TestFailedStatus, evergreen.TestSucceededStatus},
				Limit:        10,
			}
			So(len(params.TaskStatuses), ShouldEqual, 0)
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			So(len(params.TaskStatuses), ShouldEqual, 1)
			So(params.TaskStatuses[0], ShouldEqual, evergreen.TaskFailed)

		})
		Convey("a test history parameters struct without a test status should have a default set", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TaskStatuses: []string{evergreen.TaskFailed},
				Limit:        10,
			}
			So(len(params.TestStatuses), ShouldEqual, 0)
			So(params.SetDefaultsAndValidate(), ShouldBeNil)
			So(len(params.TestStatuses), ShouldEqual, 1)
			So(params.TestStatuses[0], ShouldEqual, evergreen.TestFailedStatus)
		})
		Convey("a test history parameters struct with an invalid test status should not be valid", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TestStatuses: []string{"blah"},
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct with an invalid task status should not be valid", func() {
			params := TestHistoryParameters{
				Project:      "project",
				TestNames:    []string{"test"},
				TaskStatuses: []string{"blah"},
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
		})
		Convey("a test history parameters struct with both a date and revision should not be valid", func() {
			params := TestHistoryParameters{
				Project:       "project",
				TestNames:     []string{"test"},
				AfterRevision: "abc",
				BeforeDate:    time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:        "project",
				TestNames:      []string{"test"},
				BeforeRevision: "abc",
				BeforeDate:     time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:       "project",
				TestNames:     []string{"test"},
				AfterRevision: "abc",
				AfterDate:     time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:        "project",
				TestNames:      []string{"test"},
				BeforeRevision: "abc",
				AfterDate:      time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
			params = TestHistoryParameters{
				Project:        "project",
				TestNames:      []string{"test"},
				AfterRevision:  "abc",
				BeforeDate:     time.Now(),
				BeforeRevision: "abc",
				AfterDate:      time.Now(),
			}
			So(params.SetDefaultsAndValidate(), ShouldNotBeNil)
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

	// test that a basic case returns the correct results
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
	for _, r := range results {
		assert.Equal("test", r.LocalTestResults[0].TestFile)
		assert.Equal(evergreen.TestFailedStatus, r.LocalTestResults[0].Status)
	}

	// test that a suite-style test result is found
	r5 := testresult.TestResult{
		TaskID:   t4.Id,
		TestFile: "foo/bar/test",
		Status:   evergreen.TestFailedStatus,
	}
	assert.NoError(r5.Insert())
	results, err = TaskHistoryPickaxe(params)
	assert.NoError(err)
	assert.Len(results, 3)
	for _, r := range results {
		if r.Id == t4.Id {
			assert.Len(r.LocalTestResults, 2)
		}
	}

	// test that the only matching tasks param works
	t5 := task.Task{
		Id:                  "t5",
		Project:             proj.Identifier,
		DisplayName:         "matchingName",
		BuildVariant:        "bv",
		RevisionOrderNumber: 5,
	}
	assert.NoError(t5.Insert())
	params.NewestOrder = 5
	params.OnlyMatchingTasks = true
	results, err = TaskHistoryPickaxe(params)
	assert.NoError(err)
	assert.Len(results, 3)
}
