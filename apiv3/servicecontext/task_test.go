package servicecontext

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	testConfig = evergreen.TestConfig()
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
			testTask.Insert()
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
