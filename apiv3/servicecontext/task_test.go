package servicecontext

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	testConfig = testutil.TestConfig()
)

func TestFindTaskById(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindTaskById")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	serviceContext := &DBServiceContext{}
	numTasks := 10

	Convey("When there are task documents in the database", t, func() {
		testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
			" '%v' collection", task.Collection)
		for i := 0; i < numTasks; i++ {
			testTask := &task.Task{
				Id:      fmt.Sprintf("task_%d", i),
				BuildId: fmt.Sprintf("build_%d", i),
			}
			So(testTask.Insert(), ShouldBeNil)
		}

		Convey("then properly finding each task should succeed", func() {
			for i := 0; i < numTasks; i++ {
				found, err := serviceContext.FindTaskById(fmt.Sprintf("task_%d", i))
				So(err, ShouldBeNil)
				So(found.BuildId, ShouldEqual, fmt.Sprintf("build_%d", i))
			}
		})
		Convey("then searching for task that doesn't exist should"+
			" fail with an APIError", func() {
			found, err := serviceContext.FindTaskById("fake_task")
			So(err, ShouldNotBeNil)
			So(found, ShouldBeNil)

			So(err, ShouldHaveSameTypeAs, &apiv3.APIError{})
			apiErr, ok := err.(*apiv3.APIError)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusNotFound)

		})
	})
}

func TestFindTasksByBuildId(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindTaskById")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	serviceContext := &DBServiceContext{}
	numTasks := 10

	Convey("When there are task documents in the database", t, func() {
		testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
			" '%v' collection", task.Collection)

		for i := 0; i < numTasks; i++ {
			testTask := &task.Task{
				Id:      fmt.Sprintf("task_%d", i),
				BuildId: fmt.Sprintf("build_a"),
			}
			So(testTask.Insert(), ShouldBeNil)
		}
		for i := 0; i < numTasks; i++ {
			testTask := &task.Task{
				Id:      fmt.Sprintf("task_%d", i+numTasks),
				BuildId: fmt.Sprintf("build_b"),
			}
			So(testTask.Insert(), ShouldBeNil)
		}

		Convey("then simply finding tasks by buildId should succeed", func() {
			i := 0
			for _, char := range []string{"a", "b"} {
				found, err := serviceContext.FindTasksByBuildId(fmt.Sprintf("build_%s", char), "", 0)
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, numTasks)
				for _, foundTask := range found {
					So(foundTask.BuildId, ShouldEqual, fmt.Sprintf("build_%s", char))
					So(foundTask.Id, ShouldEqual, fmt.Sprintf("task_%d", i))
					i++
				}
			}
		})
		Convey("then querying for a buildId that doesn't exist should fail"+
			" with APIError", func() {
			found, err := serviceContext.FindTasksByBuildId("fake_build", "", 10)
			So(err, ShouldNotBeNil)
			So(len(found), ShouldEqual, 0)

			So(err, ShouldHaveSameTypeAs, &apiv3.APIError{})
			apiErr, ok := err.(*apiv3.APIError)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusNotFound)
		})
		Convey("and a valid startTaskId is specified", func() {
			taskStartAt := 5
			found, err := serviceContext.FindTasksByBuildId("build_a",
				fmt.Sprintf("task_%d", taskStartAt), 0)
			Convey("then results should start at that task", func() {
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, numTasks-taskStartAt)
				for i, foundTask := range found {
					So(foundTask.Id, ShouldEqual, fmt.Sprintf("task_%d", i+taskStartAt))
				}
			})
		})
	})
}

func TestFindTasksByProjectAndCommit(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindTasksByProjectAndCommit")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	serviceContext := &DBServiceContext{}
	numCommits := 2
	numProjects := 2
	numTasks := 16
	taskIds := make([][][]string, numProjects)

	for pix := 0; pix < numProjects; pix++ {
		taskIds[pix] = make([][]string, numCommits)
		for cix := 0; cix < numCommits; cix++ {
			tids := make([]string, numTasks)
			for tix := range tids {
				tids[tix] = fmt.Sprintf("task_%d_project%d_commit%d", tix, pix, cix)
			}
			sort.StringSlice(tids).Sort()
			taskIds[pix][cix] = tids
		}
	}

	Convey("When there are task documents in the database", t, func() {
		testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
			" '%v' collection", task.Collection)
		for cix := 0; cix < numCommits; cix++ {
			for pix := 0; pix < numProjects; pix++ {
				for tix, tid := range taskIds[pix][cix] {
					status := "pass"
					if (tix % 2) == 0 {
						status = "fail"
					}
					testTask := &task.Task{
						Id:       tid,
						Revision: fmt.Sprintf("commit_%d", cix),
						Project:  fmt.Sprintf("project_%d", pix),
						Status:   status,
					}
					So(testTask.Insert(), ShouldBeNil)
				}
			}
		}

		Convey("then properly finding each set of tasks should succeed", func() {
			for pix := 0; pix < numProjects; pix++ {
				for cix := 0; cix < numCommits; cix++ {
					foundTasks, err := serviceContext.FindTasksByProjectAndCommit(fmt.Sprintf("project_%d", pix),
						fmt.Sprintf("commit_%d", cix), "", "", 0, 1)
					So(err, ShouldBeNil)
					So(len(foundTasks), ShouldEqual, numTasks)
					for tix, t := range foundTasks {
						So(t.Id, ShouldEqual, taskIds[pix][cix][tix])
					}
				}
			}
		})
		Convey("then properly finding only tasks with status should return correct set", func() {
			for _, status := range []string{"pass", "fail"} {
				for pix := 0; pix < numProjects; pix++ {
					for cix := 0; cix < numCommits; cix++ {
						foundTasks, err := serviceContext.FindTasksByProjectAndCommit(fmt.Sprintf("project_%d", pix),
							fmt.Sprintf("commit_%d", cix), "", status, 0, 1)
						So(err, ShouldBeNil)
						So(len(foundTasks), ShouldEqual, numTasks/2)
						for _, t := range foundTasks {
							So(t.Status, ShouldEqual, status)
						}
					}
				}
			}
		})
		Convey("then properly finding only tasks from taskid should return correct set", func() {
			commitId := "commit_1"
			projectId := "project_1"
			tids := taskIds[1][1]
			for _, sort := range []int{1, -1} {
				for i := 0; i < numTasks; i++ {
					foundTasks, err := serviceContext.FindTasksByProjectAndCommit(projectId, commitId,
						tids[i], "", 0, sort)
					So(err, ShouldBeNil)

					startAt := 0
					if sort < 0 {
						startAt = len(tids) - 1
					}

					So(len(foundTasks), ShouldEqual, (numTasks-startAt)-i*sort)
					for ix, t := range foundTasks {
						index := ix
						if sort > 0 {
							index += i
						}
						So(t.Id, ShouldEqual, tids[index])
					}
				}
			}
		})
		Convey("then adding a limit should return correct number and set of results",
			func() {
				commitId := "commit_0"
				projectId := "project_0"
				limit := 2
				tids := taskIds[0][0]
				for i := 0; i < numTasks/limit; i++ {
					index := i * limit
					taskName := tids[index]
					foundTasks, err := serviceContext.FindTasksByProjectAndCommit(projectId, commitId,
						taskName, "", limit, 1)
					So(err, ShouldBeNil)
					So(len(foundTasks), ShouldEqual, limit)
					for ix, t := range foundTasks {
						So(t.Id, ShouldEqual, tids[ix+index])
					}
				}

			})
		Convey("then searching for project that doesn't exist should"+
			" fail with an APIError", func() {
			foundTests, err := serviceContext.FindTasksByProjectAndCommit("fake_project", "commit_0", "", "", 0, 1)
			So(err, ShouldNotBeNil)
			So(len(foundTests), ShouldEqual, 0)

			So(err, ShouldHaveSameTypeAs, &apiv3.APIError{})
			apiErr, ok := err.(*apiv3.APIError)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusNotFound)
		})
		Convey("then searching for a project and commit with no task name should return first result",
			func() {
				projectId := "project_0"
				commitId := "commit_0"
				foundTasks, err := serviceContext.FindTasksByProjectAndCommit(projectId, commitId, "", "", 1, 1)
				So(err, ShouldBeNil)
				So(len(foundTasks), ShouldEqual, 1)
				task1 := foundTasks[0]
				So(task1.Id, ShouldEqual, taskIds[0][0][0])
			})
		Convey("then starting at a task that doesn't exist"+
			" fail with an APIError", func() {
			foundTests, err := serviceContext.FindTasksByProjectAndCommit("project_0", "commit_0", "fake_task", "", 0, 1)
			So(err, ShouldNotBeNil)
			So(len(foundTests), ShouldEqual, 0)

			So(err, ShouldHaveSameTypeAs, &apiv3.APIError{})
			apiErr, ok := err.(*apiv3.APIError)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusNotFound)
		})
		Convey("then searching for a commit that doesn't exist"+
			" fail with an APIError", func() {
			foundTests, err := serviceContext.FindTasksByProjectAndCommit("project_0", "fake_commit", "", "", 0, 1)
			So(err, ShouldNotBeNil)
			So(len(foundTests), ShouldEqual, 0)

			So(err, ShouldHaveSameTypeAs, &apiv3.APIError{})
			apiErr, ok := err.(*apiv3.APIError)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusNotFound)
		})
	})
}
