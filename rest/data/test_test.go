package data

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestFindTestsByTaskId(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindTestsByTaskId")
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	assert.NoError(t, db.Clear(task.Collection))

	serviceContext := &DBConnector{}
	numTests := 10
	numTasks := 2
	testFileNames := make([]string, numTests)
	for ix := range testFileNames {
		testFileNames[ix] = fmt.Sprintf("file_%d", ix)
	}
	sort.StringSlice(testFileNames).Sort()

	Convey("When there are task and test documents in the database", t, func() {
		testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
			" '%v' collection", task.Collection)
		testutil.HandleTestingErr(db.Clear(testresult.Collection), t, "Error clearing"+
			" '%v' collection", testresult.Collection)
		for i := 0; i < numTasks; i++ {
			id := fmt.Sprintf("task_%d", i)
			testTask := &task.Task{
				Id: id,
			}
			tests := make([]testresult.TestResult, numTests)
			for j := 0; j < numTests; j++ {
				status := "pass"
				if j%2 == 0 {
					status = "fail"
				}
				tests[j] = testresult.TestResult{
					TaskID:    id,
					Execution: 0,
					Status:    status,
					TestFile:  testFileNames[j],
				}
			}
			So(testTask.Insert(), ShouldBeNil)
			for _, test := range tests {
				So(test.Insert(), ShouldBeNil)
			}
		}

		Convey("then properly finding each set of tests should succeed", func() {
			for i := 0; i < numTasks; i++ {
				foundTests, err := serviceContext.FindTestsByTaskId(fmt.Sprintf("task_%d", i), "", "", 0, 1, 0)
				So(err, ShouldBeNil)
				So(len(foundTests), ShouldEqual, numTests)
			}
		})
		Convey("then properly finding only tasks with status should return correct set", func() {
			for _, status := range []string{"pass", "fail"} {
				for i := 0; i < numTasks; i++ {
					foundTests, err := serviceContext.FindTestsByTaskId(fmt.Sprintf("task_%d", i), "", status, 0, 1, 0)
					So(err, ShouldBeNil)
					So(len(foundTests), ShouldEqual, numTests/2)
					for _, t := range foundTests {
						So(t.Status, ShouldEqual, status)
					}
				}
			}
		})
		Convey("then properly finding only tasks from test file should return correct set", func() {
			taskId := "task_1"
			for _, sort := range []int{1, -1} {
				for i := 0; i < numTests; i++ {
					foundTests, err := serviceContext.FindTestsByTaskId(taskId, testFileNames[i], "", 0, sort, 0)
					So(err, ShouldBeNil)

					startAt := 0
					if sort < 0 {
						startAt = len(testFileNames) - 1
					}

					So(len(foundTests), ShouldEqual, (numTests-startAt)-i*sort)
					for ix, t := range foundTests {
						index := ix
						if sort > 0 {
							index += i
						}
						So(t.TestFile, ShouldEqual, testFileNames[index])
					}
				}
			}
		})
		Convey("then adding a limit should return correct number and set of results"+
			" fail with an APIError", func() {
			taskname := "task_0"
			limit := 2
			for i := 0; i < numTests/limit; i++ {
				index := i * limit
				testName := testFileNames[index]
				foundTests, err := serviceContext.FindTestsByTaskId(taskname, testName, "", limit, 1, 0)
				So(err, ShouldBeNil)
				So(len(foundTests), ShouldEqual, limit)
				for ix, t := range foundTests {
					So(t.TestFile, ShouldEqual, testFileNames[ix+index])
				}
			}

		})
		Convey("then searching for task that doesn't exist should"+
			" fail with an APIError", func() {
			foundTests, err := serviceContext.FindTestsByTaskId("fake_task", "", "", 0, 1, 0)
			So(err, ShouldNotBeNil)
			So(len(foundTests), ShouldEqual, 0)

			So(err, ShouldHaveSameTypeAs, &rest.APIError{})
			apiErr, ok := err.(*rest.APIError)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusNotFound)
		})
		Convey("then searching for a task with no test_file should return first result",
			func() {
				taskname := "task_0"
				foundTests, err := serviceContext.FindTestsByTaskId(taskname, "", "", 1, 1, 0)
				So(err, ShouldBeNil)
				So(len(foundTests), ShouldEqual, 1)
				test1 := foundTests[0]
				So(test1.TestFile, ShouldEqual, testFileNames[0])
			})
	})
}
