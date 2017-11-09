package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
)

var testConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func TestGetTestHistory(t *testing.T) {
	userManager, err := auth.LoadUserManager(taskTestConfig.AuthConfig)
	testutil.HandleTestingErr(err, t, "Failure in loading UserManager from config")

	uis := UIServer{
		RootURL:     taskTestConfig.Ui.Url,
		Settings:    *taskTestConfig,
		UserManager: userManager,
	}

	home := evergreen.FindEvergreenHome()

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: true,
	})

	testutil.HandleTestingErr(uis.InitPlugins(), t, "problem installing plugins")
	router := mux.NewRouter()
	err = uis.AttachRoutes(router)
	testutil.HandleTestingErr(err, t, "Failed to create ui server router")

	Convey("When retrieving the test history", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(task.Collection, version.Collection, testresult.Collection), t,
			"Error clearing test collections")
		project := "project-test"
		err = modelutil.CreateTestLocalConfig(buildTestConfig, "project_test", "")
		So(err, ShouldBeNil)

		now := time.Now()

		testVersion := version.Version{
			Id:                  "testVersion",
			Revision:            "fgh",
			RevisionOrderNumber: 1,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(testVersion.Insert(), ShouldBeNil)
		testVersion2 := version.Version{
			Id:                  "anotherVersion",
			Revision:            "def",
			RevisionOrderNumber: 2,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(testVersion2.Insert(), ShouldBeNil)
		testVersion3 := version.Version{
			Id:                  "testV",
			Revision:            "abcd",
			RevisionOrderNumber: 4,
			Identifier:          project,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(testVersion3.Insert(), ShouldBeNil)

		task1 := task.Task{
			Id:                  "task1",
			DisplayName:         "test",
			BuildVariant:        "osx",
			Revision:            "fgh",
			Project:             project,
			StartTime:           now,
			RevisionOrderNumber: 1,
			Status:              evergreen.TaskFailed,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(task1.Insert(), ShouldBeNil)
		task1results := []testresult.TestResult{
			testresult.TestResult{
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
				URL:       "url",
				StartTime: float64(now.Unix()),
				EndTime:   float64(now.Add(time.Duration(10 * time.Second)).Unix()),
			},
			testresult.TestResult{
				Status:    evergreen.TestSucceededStatus,
				TestFile:  "test2",
				URL:       "anotherurl",
				StartTime: float64(now.Unix()),
				EndTime:   float64(now.Add(time.Duration(60 * time.Second)).Unix()),
			},
		}
		So(testresult.InsertManyByTaskIDAndExecution(task1results, task1.Id, task1.Execution), ShouldBeNil)
		task2 := task.Task{
			Id:                  "task2",
			DisplayName:         "test",
			BuildVariant:        "osx",
			Revision:            "fgh",
			Project:             project,
			StartTime:           now.Add(time.Duration(30 * time.Minute)),
			RevisionOrderNumber: 2,
			Status:              evergreen.TaskFailed,
			Requester:           evergreen.PatchVersionRequester,
		}
		So(task2.Insert(), ShouldBeNil)
		task2results := []testresult.TestResult{
			testresult.TestResult{
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test1",
				URL:       "url",
				StartTime: float64(now.Unix()),
				EndTime:   float64(now.Add(time.Duration(45 * time.Second)).Unix()),
			},
			testresult.TestResult{
				Status:    evergreen.TestFailedStatus,
				TestFile:  "test2",
				URL:       "anotherurl",
				StartTime: float64(now.Unix()),
				EndTime:   float64(now.Add(time.Duration(30 * time.Second)).Unix()),
			},
		}
		So(testresult.InsertManyByTaskIDAndExecution(task2results, task2.Id, task2.Execution), ShouldBeNil)

		task3 := task.Task{
			Id:                  "task3",
			DisplayName:         "test2",
			BuildVariant:        "osx",
			Project:             project,
			Revision:            "fgh",
			StartTime:           now,
			RevisionOrderNumber: 1,
			Status:              evergreen.TaskFailed,
		}
		So(task3.Insert(), ShouldBeNil)
		task3results := []testresult.TestResult{
			testresult.TestResult{
				Status:   evergreen.TestFailedStatus,
				TestFile: "test1",
				LogID:    "2",
			},
			testresult.TestResult{
				Status:   evergreen.TestSucceededStatus,
				TestFile: "test3",
				LogID:    "4",
			},
		}
		So(testresult.InsertManyByTaskIDAndExecution(task3results, task3.Id, task3.Execution), ShouldBeNil)

		Convey("response should be a list of test results", func() {

			url, err := router.Get("test_history").URL("project_id", project)
			So(err, ShouldBeNil)

			request, err := http.NewRequest("GET", url.String()+"?tasks=test,test2&limit=20&requestSource=any", nil)
			So(err, ShouldBeNil)

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)
			So(response.Code, ShouldEqual, http.StatusOK)

			var results []RestTestHistoryResult
			err = json.Unmarshal(response.Body.Bytes(), &results)
			So(err, ShouldBeNil)
			So(len(results), ShouldEqual, 4)
			So(results[0].TestFile, ShouldEqual, "test2")
			So(results[0].TaskName, ShouldEqual, "test")
			So(results[0].TestStatus, ShouldEqual, evergreen.TestFailedStatus)
			So(results[0].TaskStatus, ShouldEqual, evergreen.TaskFailed)
			So(results[0].Revision, ShouldEqual, "fgh")
			So(results[0].Project, ShouldEqual, project)
			So(results[0].TaskId, ShouldEqual, "task2")
			So(results[0].BuildVariant, ShouldEqual, "osx")
			So(results[0].StartTime.Unix(), ShouldResemble, int64(task2results[1].StartTime))
			So(results[0].DurationMS, ShouldEqual, time.Duration(30*time.Second))
			So(results[0].Url, ShouldEqual, "anotherurl")

			So(results[1].Url, ShouldEqual, "url")
			grip.Info(results)
			So(results[2].Url, ShouldEqual, fmt.Sprintf("%v/test_log/2", taskTestConfig.Ui.Url))
		})

		Convey("response when only requesting patches should have one result ", func() {
			url, err := router.Get("test_history").URL("project_id", project)
			So(err, ShouldBeNil)

			request, err := http.NewRequest("GET", url.String()+"?tasks=test,test2&limit=20&requestSource=patch", nil)
			So(err, ShouldBeNil)

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)
			So(response.Code, ShouldEqual, http.StatusOK)

			var results []RestTestHistoryResult
			err = json.Unmarshal(response.Body.Bytes(), &results)
			So(err, ShouldBeNil)
			So(len(results), ShouldEqual, 2)
			So(results[0].Url, ShouldEqual, "anotherurl")
		})

		Convey("response with invalid build request source should be an error", func() {
			url, err := router.Get("test_history").URL("project_id", project)
			So(err, ShouldBeNil)

			request, err := http.NewRequest("GET", url.String()+"?tasks=test,test2&limit=20&requestSource=INVALID", nil)
			So(err, ShouldBeNil)

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)
			So(response.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("response with commit requests should have the expected results", func() {
			url, err := router.Get("test_history").URL("project_id", project)
			So(err, ShouldBeNil)

			request, err := http.NewRequest("GET", url.String()+"?tasks=test,test2&limit=20&requestSource=commit", nil)
			So(err, ShouldBeNil)

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)
			So(response.Code, ShouldEqual, http.StatusOK)

			var results []RestTestHistoryResult
			err = json.Unmarshal(response.Body.Bytes(), &results)
			So(err, ShouldBeNil)
			So(len(results), ShouldEqual, 1)
		})

		Convey("response with no request source argument should have the same results as commit", func() {
			url, err := router.Get("test_history").URL("project_id", project)
			So(err, ShouldBeNil)

			request, err := http.NewRequest("GET", url.String()+"?tasks=test,test2&limit=20", nil)
			So(err, ShouldBeNil)

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)
			So(response.Code, ShouldEqual, http.StatusOK)

			var results []RestTestHistoryResult
			err = json.Unmarshal(response.Body.Bytes(), &results)
			So(err, ShouldBeNil)
			So(len(results), ShouldEqual, 1)
		})
	})
}
