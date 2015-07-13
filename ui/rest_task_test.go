package ui

import (
	"encoding/json"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/render"
	. "github.com/smartystreets/goconvey/convey"

	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

var (
	taskTestConfig = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskTestConfig))
}

func TestGetTaskInfo(t *testing.T) {

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
		Funcs:        nil,
	})

	router, err := uis.NewRouter()
	testutil.HandleTestingErr(err, t, "Failure in uis.NewRouter()")

	Convey("When finding info on a particular task", t, func() {
		testutil.HandleTestingErr(db.Clear(model.TasksCollection), t,
			"Error clearing '%v' collection", model.TasksCollection)

		taskId := "my-task"
		versionId := "my-version"
		projectName := "project_test"

		testResult := model.TestResult{
			Status:    "success",
			TestFile:  "some-test",
			URL:       "some-url",
			StartTime: float64(time.Now().Add(-9 * time.Minute).Unix()),
			EndTime:   float64(time.Now().Add(-1 * time.Minute).Unix()),
		}
		task := &model.Task{
			Id:                  taskId,
			CreateTime:          time.Now().Add(-20 * time.Minute),
			ScheduledTime:       time.Now().Add(-15 * time.Minute),
			DispatchTime:        time.Now().Add(-14 * time.Minute),
			StartTime:           time.Now().Add(-10 * time.Minute),
			FinishTime:          time.Now().Add(-5 * time.Second),
			PushTime:            time.Now().Add(-1 * time.Millisecond),
			Version:             versionId,
			Project:             projectName,
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Priority:            10,
			LastHeartbeat:       time.Now(),
			Activated:           false,
			BuildId:             "some-build-id",
			DistroId:            "some-distro-id",
			BuildVariant:        "some-build-variant",
			DependsOn:           []model.Dependency{{"some-other-task", ""}},
			DisplayName:         "My task",
			HostId:              "some-host-id",
			Restarts:            0,
			Execution:           0,
			Archived:            false,
			RevisionOrderNumber: 42,
			Requester:           evergreen.RepotrackerVersionRequester,
			Status:              "success",
			Details: apimodels.TaskEndDetail{
				TimedOut:    false,
				Description: "some-stage",
			},
			Aborted:          false,
			TimeTaken:        time.Duration(100 * time.Millisecond),
			ExpectedDuration: time.Duration(99 * time.Millisecond),
			TestResults:      []model.TestResult{testResult},
			MinQueuePos:      0,
		}
		So(task.Insert(), ShouldBeNil)

		file := artifact.File{
			Name: "Some Artifact",
			Link: "some-url",
		}
		artifact := artifact.Entry{
			TaskId: taskId,
			Files:  []artifact.File{file},
		}
		So(artifact.Upsert(), ShouldBeNil)

		url, err := router.Get("task_info").URL("task_id", taskId)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		Convey("response should match contents of database", func() {
			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)
			Println(string(response.Body.Bytes()))

			var rawJsonBody map[string]*json.RawMessage
			err = json.Unmarshal(response.Body.Bytes(), &rawJsonBody)
			So(err, ShouldBeNil)

			So(jsonBody["id"], ShouldEqual, task.Id)

			var createTime time.Time
			err = json.Unmarshal(*rawJsonBody["create_time"], &createTime)
			So(err, ShouldBeNil)
			So(createTime, ShouldHappenWithin, rest.TimePrecision, task.CreateTime)

			var scheduledTime time.Time
			err = json.Unmarshal(*rawJsonBody["scheduled_time"], &scheduledTime)
			So(err, ShouldBeNil)
			So(scheduledTime, ShouldHappenWithin, rest.TimePrecision, task.ScheduledTime)

			var dispatchTime time.Time
			err = json.Unmarshal(*rawJsonBody["dispatch_time"], &dispatchTime)
			So(err, ShouldBeNil)
			So(dispatchTime, ShouldHappenWithin, rest.TimePrecision, task.DispatchTime)

			var startTime time.Time
			err = json.Unmarshal(*rawJsonBody["start_time"], &startTime)
			So(err, ShouldBeNil)
			So(startTime, ShouldHappenWithin, rest.TimePrecision, task.StartTime)

			var finishTime time.Time
			err = json.Unmarshal(*rawJsonBody["finish_time"], &finishTime)
			So(err, ShouldBeNil)
			So(finishTime, ShouldHappenWithin, rest.TimePrecision, task.FinishTime)

			var pushTime time.Time
			err = json.Unmarshal(*rawJsonBody["push_time"], &pushTime)
			So(err, ShouldBeNil)
			So(pushTime, ShouldHappenWithin, rest.TimePrecision, task.PushTime)

			So(jsonBody["version"], ShouldEqual, task.Version)
			So(jsonBody["project"], ShouldEqual, task.Project)
			So(jsonBody["revision"], ShouldEqual, task.Revision)
			So(jsonBody["priority"], ShouldEqual, task.Priority)

			var lastHeartbeat time.Time
			err = json.Unmarshal(*rawJsonBody["last_heartbeat"], &lastHeartbeat)
			So(err, ShouldBeNil)
			So(lastHeartbeat, ShouldHappenWithin, rest.TimePrecision, task.LastHeartbeat)

			So(jsonBody["activated"], ShouldEqual, task.Activated)
			So(jsonBody["build_id"], ShouldEqual, task.BuildId)
			So(jsonBody["distro"], ShouldEqual, task.DistroId)
			So(jsonBody["build_variant"], ShouldEqual, task.BuildVariant)

			var dependsOn []model.Dependency
			So(rawJsonBody["depends_on"], ShouldNotBeNil)
			err = json.Unmarshal(*rawJsonBody["depends_on"], &dependsOn)
			So(err, ShouldBeNil)
			So(dependsOn, ShouldResemble, task.DependsOn)

			So(jsonBody["display_name"], ShouldEqual, task.DisplayName)
			So(jsonBody["host_id"], ShouldEqual, task.HostId)
			So(jsonBody["restarts"], ShouldEqual, task.Restarts)
			So(jsonBody["execution"], ShouldEqual, task.Execution)
			So(jsonBody["archived"], ShouldEqual, task.Archived)
			So(jsonBody["order"], ShouldEqual, task.RevisionOrderNumber)
			So(jsonBody["requester"], ShouldEqual, task.Requester)
			So(jsonBody["status"], ShouldEqual, task.Status)

			_jsonStatusDetails, ok := jsonBody["status_details"]
			So(ok, ShouldBeTrue)
			jsonStatusDetails, ok := _jsonStatusDetails.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonStatusDetails["timed_out"], ShouldEqual, task.Details.TimedOut)
			So(jsonStatusDetails["timeout_stage"], ShouldEqual, task.Details.Description)

			So(jsonBody["aborted"], ShouldEqual, task.Aborted)
			So(jsonBody["time_taken"], ShouldEqual, task.TimeTaken)
			So(jsonBody["expected_duration"], ShouldEqual, task.ExpectedDuration)

			_jsonTestResults, ok := jsonBody["test_results"]
			So(ok, ShouldBeTrue)
			jsonTestResults, ok := _jsonTestResults.(map[string]interface{})
			So(ok, ShouldBeTrue)
			So(len(jsonTestResults), ShouldEqual, 1)

			_jsonTestResult, ok := jsonTestResults[testResult.TestFile]
			So(ok, ShouldBeTrue)
			jsonTestResult, ok := _jsonTestResult.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonTestResult["status"], ShouldEqual, testResult.Status)
			So(jsonTestResult["time_taken"], ShouldNotBeNil) // value correctness is unchecked

			_jsonTestResultLogs, ok := jsonTestResult["logs"]
			So(ok, ShouldBeTrue)
			jsonTestResultLogs, ok := _jsonTestResultLogs.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonTestResultLogs["url"], ShouldEqual, testResult.URL)

			So(jsonBody["min_queue_pos"], ShouldEqual, task.MinQueuePos)

			var jsonFiles []map[string]interface{}
			err = json.Unmarshal(*rawJsonBody["files"], &jsonFiles)
			So(err, ShouldBeNil)
			So(len(jsonFiles), ShouldEqual, 1)

			jsonFile := jsonFiles[0]
			So(jsonFile["name"], ShouldEqual, file.Name)
			So(jsonFile["url"], ShouldEqual, file.Link)
		})
	})

	Convey("When finding info on a nonexistent task", t, func() {
		taskId := "not-present"

		url, err := router.Get("task_info").URL("task_id", taskId)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusNotFound)

		Convey("response should contain a sensible error message", func() {
			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)

			So(jsonBody["message"], ShouldEqual,
				fmt.Sprintf("Error finding task '%v'", taskId))
		})
	})
}

func TestGetTaskStatus(t *testing.T) {

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
		Funcs:        nil,
	})

	router, err := uis.NewRouter()
	testutil.HandleTestingErr(err, t, "Failure in uis.NewRouter()")

	Convey("When finding the status of a particular task", t, func() {
		testutil.HandleTestingErr(db.Clear(model.TasksCollection), t,
			"Error clearing '%v' collection", model.TasksCollection)

		taskId := "my-task"

		testResult := model.TestResult{
			Status:    "success",
			TestFile:  "some-test",
			URL:       "some-url",
			StartTime: float64(time.Now().Add(-9 * time.Minute).Unix()),
			EndTime:   float64(time.Now().Add(-1 * time.Minute).Unix()),
		}
		task := &model.Task{
			Id:          taskId,
			DisplayName: "My task",
			Status:      "success",
			Details: apimodels.TaskEndDetail{
				TimedOut:    false,
				Description: "some-stage",
			},
			TestResults: []model.TestResult{testResult},
		}
		So(task.Insert(), ShouldBeNil)

		url, err := router.Get("task_status").URL("task_id", taskId)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		Convey("response should match contents of database", func() {
			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)

			var rawJsonBody map[string]*json.RawMessage
			err = json.Unmarshal(response.Body.Bytes(), &rawJsonBody)
			So(err, ShouldBeNil)

			So(jsonBody["task_id"], ShouldEqual, task.Id)
			So(jsonBody["task_name"], ShouldEqual, task.DisplayName)
			So(jsonBody["status"], ShouldEqual, task.Status)

			_jsonStatusDetails, ok := jsonBody["status_details"]
			So(ok, ShouldBeTrue)
			jsonStatusDetails, ok := _jsonStatusDetails.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonStatusDetails["timed_out"], ShouldEqual, task.Details.TimedOut)
			So(jsonStatusDetails["timeout_stage"], ShouldEqual, task.Details.Description)

			_jsonTestResults, ok := jsonBody["tests"]
			So(ok, ShouldBeTrue)
			jsonTestResults, ok := _jsonTestResults.(map[string]interface{})
			So(ok, ShouldBeTrue)
			So(len(jsonTestResults), ShouldEqual, 1)

			_jsonTestResult, ok := jsonTestResults[testResult.TestFile]
			So(ok, ShouldBeTrue)
			jsonTestResult, ok := _jsonTestResult.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonTestResult["status"], ShouldEqual, testResult.Status)
			So(jsonTestResult["time_taken"], ShouldNotBeNil) // value correctness is unchecked

			_jsonTestResultLogs, ok := jsonTestResult["logs"]
			So(ok, ShouldBeTrue)
			jsonTestResultLogs, ok := _jsonTestResultLogs.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonTestResultLogs["url"], ShouldEqual, testResult.URL)
		})
	})

	Convey("When finding the status of a nonexistent task", t, func() {
		taskId := "not-present"

		url, err := router.Get("task_status").URL("task_id", taskId)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusNotFound)

		Convey("response should contain a sensible error message", func() {
			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)

			So(jsonBody["message"], ShouldEqual,
				fmt.Sprintf("Error finding task '%v'", taskId))
		})
	})
}
