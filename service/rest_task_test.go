package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func insertTaskForTesting(taskId, versionId, projectName string) (*task.Task, error) {
	task := &task.Task{
		Id:                  taskId,
		CreateTime:          time.Now().Add(-20 * time.Minute),
		ScheduledTime:       time.Now().Add(-15 * time.Minute),
		DispatchTime:        time.Now().Add(-14 * time.Minute),
		StartTime:           time.Now().Add(-10 * time.Minute),
		FinishTime:          time.Now().Add(-5 * time.Second),
		Version:             versionId,
		Project:             projectName,
		Revision:            fmt.Sprintf("%x", rand.Int()),
		Priority:            10,
		LastHeartbeat:       time.Now(),
		Activated:           false,
		BuildId:             "some-build-id",
		DistroId:            "some-distro-id",
		BuildVariant:        "some-build-variant",
		DependsOn:           []task.Dependency{{TaskId: "some-other-task", Status: ""}},
		DisplayName:         "My task",
		HostId:              "some-host-id",
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
		TimeTaken:        100 * time.Millisecond,
		ExpectedDuration: 99 * time.Millisecond,
		HasCedarResults:  true,
	}

	return task, task.Insert()
}

func TestGetTaskInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	router, err := newTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	cedarSrv, cedarHandler := mock.NewCedarServer(nil)
	defer cedarSrv.Close()

	Convey("When finding info on a particular task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection),
			"Error clearing '%v' collection", task.Collection)

		taskId := "my-task"
		versionId := "my-version"
		projectName := "project_test"

		testResult := task.TestResult{
			Status:    "success",
			TaskID:    taskId,
			Execution: 0,
			TestFile:  "some-test",
			URL:       "some-url",
			StartTime: float64(time.Now().Add(-9 * time.Minute).Unix()),
			EndTime:   float64(time.Now().Add(-1 * time.Minute).Unix()),
		}
		testTask, err := insertTaskForTesting(taskId, versionId, projectName)
		So(err, ShouldBeNil)
		So(cedarHandler.SetTestResults([]task.TestResult{testResult}, nil), ShouldBeNil)

		file := artifact.File{
			Name: "Some Artifact",
			Link: "some-url",
		}
		a := artifact.Entry{
			TaskId: taskId,
			Files:  []artifact.File{file},
		}
		So(a.Upsert(), ShouldBeNil)

		url := "/rest/v1/tasks/" + taskId

		request, err := http.NewRequest("GET", url, nil)
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

			rawJSONBody := map[string]*json.RawMessage{}
			err = json.Unmarshal(response.Body.Bytes(), &rawJSONBody)
			So(err, ShouldBeNil)

			So(jsonBody["id"], ShouldEqual, testTask.Id)

			var createTime time.Time
			err = json.Unmarshal(*rawJSONBody["create_time"], &createTime)
			So(err, ShouldBeNil)
			So(createTime, ShouldHappenWithin, TimePrecision, testTask.CreateTime)

			var scheduledTime time.Time
			err = json.Unmarshal(*rawJSONBody["scheduled_time"], &scheduledTime)
			So(err, ShouldBeNil)
			So(scheduledTime, ShouldHappenWithin, TimePrecision, testTask.ScheduledTime)

			var dispatchTime time.Time
			err = json.Unmarshal(*rawJSONBody["dispatch_time"], &dispatchTime)
			So(err, ShouldBeNil)
			So(dispatchTime, ShouldHappenWithin, TimePrecision, testTask.DispatchTime)

			var startTime time.Time
			err = json.Unmarshal(*rawJSONBody["start_time"], &startTime)
			So(err, ShouldBeNil)
			So(startTime, ShouldHappenWithin, TimePrecision, testTask.StartTime)

			var finishTime time.Time
			err = json.Unmarshal(*rawJSONBody["finish_time"], &finishTime)
			So(err, ShouldBeNil)
			So(finishTime, ShouldHappenWithin, TimePrecision, testTask.FinishTime)

			So(jsonBody["version"], ShouldEqual, testTask.Version)
			So(jsonBody["project"], ShouldEqual, testTask.Project)
			So(jsonBody["revision"], ShouldEqual, testTask.Revision)
			So(jsonBody["priority"], ShouldEqual, testTask.Priority)

			var lastHeartbeat time.Time
			err = json.Unmarshal(*rawJSONBody["last_heartbeat"], &lastHeartbeat)
			So(err, ShouldBeNil)
			So(lastHeartbeat, ShouldHappenWithin, TimePrecision, testTask.LastHeartbeat)

			So(jsonBody["activated"], ShouldEqual, testTask.Activated)
			So(jsonBody["build_id"], ShouldEqual, testTask.BuildId)
			So(jsonBody["distro"], ShouldEqual, testTask.DistroId)
			So(jsonBody["build_variant"], ShouldEqual, testTask.BuildVariant)

			var dependsOn []task.Dependency
			So(rawJSONBody["depends_on"], ShouldNotBeNil)
			err = json.Unmarshal(*rawJSONBody["depends_on"], &dependsOn)
			So(err, ShouldBeNil)
			So(dependsOn, ShouldResemble, testTask.DependsOn)

			So(jsonBody["display_name"], ShouldEqual, testTask.DisplayName)
			So(jsonBody["host_id"], ShouldEqual, testTask.HostId)
			So(jsonBody["execution"], ShouldEqual, testTask.Execution)
			So(jsonBody["archived"], ShouldEqual, testTask.Archived)
			So(jsonBody["order"], ShouldEqual, testTask.RevisionOrderNumber)
			So(jsonBody["requester"], ShouldEqual, testTask.Requester)
			So(jsonBody["status"], ShouldEqual, testTask.Status)

			_jsonStatusDetails, ok := jsonBody["status_details"]
			So(ok, ShouldBeTrue)
			jsonStatusDetails, ok := _jsonStatusDetails.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonStatusDetails["timed_out"], ShouldEqual, testTask.Details.TimedOut)
			So(jsonStatusDetails["timeout_stage"], ShouldEqual, testTask.Details.Description)

			So(jsonBody["aborted"], ShouldEqual, testTask.Aborted)
			So(jsonBody["time_taken"], ShouldEqual, testTask.TimeTaken)
			So(jsonBody["expected_duration"], ShouldEqual, testTask.ExpectedDuration)

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

			var jsonFiles []map[string]interface{}
			err = json.Unmarshal(*rawJSONBody["files"], &jsonFiles)
			So(err, ShouldBeNil)
			So(len(jsonFiles), ShouldEqual, 1)

			jsonFile := jsonFiles[0]
			So(jsonFile["name"], ShouldEqual, file.Name)
			So(jsonFile["url"], ShouldEqual, file.Link)
		})
	})

	Convey("When finding info on a nonexistent task", t, func() {
		taskId := "not-present"

		url := "/rest/v1/tasks/" + taskId

		request, err := http.NewRequest("GET", url, nil)
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
			So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
		})
	})
}

func TestGetTaskStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	router, err := newTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	cedarSrv, cedarHandler := mock.NewCedarServer(nil)
	defer cedarSrv.Close()

	Convey("When finding the status of a particular task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection),
			"Error clearing '%v' collection", task.Collection)

		taskId := "my-task"

		testTask := &task.Task{
			Id:          taskId,
			DisplayName: "My task",
			Status:      "success",
			Details: apimodels.TaskEndDetail{
				TimedOut:    false,
				Description: "some-stage",
			},
			HasCedarResults: true,
		}
		testResult := task.TestResult{
			Status:    "success",
			TaskID:    testTask.Id,
			Execution: testTask.Execution,
			TestFile:  "some-test",
			URL:       "some-url",
			StartTime: float64(time.Now().Add(-9 * time.Minute).Unix()),
			EndTime:   float64(time.Now().Add(-1 * time.Minute).Unix()),
		}
		So(testTask.Insert(), ShouldBeNil)
		So(cedarHandler.SetTestResults([]task.TestResult{testResult}, nil), ShouldBeNil)

		url := "/rest/v1/tasks/" + taskId + "/status"

		request, err := http.NewRequest("GET", url, nil)
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

			rawJSONBody := map[string]*json.RawMessage{}
			err = json.Unmarshal(response.Body.Bytes(), &rawJSONBody)
			So(err, ShouldBeNil)

			So(jsonBody["task_id"], ShouldEqual, testTask.Id)
			So(jsonBody["task_name"], ShouldEqual, testTask.DisplayName)
			So(jsonBody["status"], ShouldEqual, testTask.Status)

			_jsonStatusDetails, ok := jsonBody["status_details"]
			So(ok, ShouldBeTrue)
			jsonStatusDetails, ok := _jsonStatusDetails.(map[string]interface{})
			So(ok, ShouldBeTrue)

			So(jsonStatusDetails["timed_out"], ShouldEqual, testTask.Details.TimedOut)
			So(jsonStatusDetails["timeout_stage"], ShouldEqual, testTask.Details.Description)

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

		url := "/rest/v1/tasks/" + taskId + "/status"

		request, err := http.NewRequest("GET", url, nil)
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
			So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
		})
	})
}

func TestGetDisplayTaskInfo(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(env.Configure(ctx))
	router, err := newTestUIRouter(ctx, env)
	require.NoError(err, "error setting up router")

	cedarSrv, cedarHandler := mock.NewCedarServer(nil)
	defer cedarSrv.Close()

	require.NoError(db.ClearCollections(task.Collection))

	executionTaskId := "execution-task"
	displayTaskId := "display-task"
	versionId := "my-version"
	projectName := "project_test"
	_, err = insertTaskForTesting(executionTaskId, versionId, projectName)
	require.NoError(err)
	displayTask, err := insertTaskForTesting(displayTaskId, versionId, projectName)
	require.NoError(err)

	testResult := task.TestResult{
		Status:    "success",
		TaskID:    executionTaskId,
		Execution: 0,
		TestFile:  "some-test",
		URL:       "some-url",
		StartTime: float64(time.Now().Add(-9 * time.Minute).Unix()),
		EndTime:   float64(time.Now().Add(-1 * time.Minute).Unix()),
	}
	require.NoError(cedarHandler.SetTestResults([]task.TestResult{testResult}, nil))

	displayTask.ExecutionTasks = []string{executionTaskId}
	err = db.Update(task.Collection,
		bson.M{task.IdKey: displayTaskId},
		bson.M{"$set": bson.M{
			task.ExecutionTasksKey: []string{executionTaskId},
			task.DisplayOnlyKey:    true,
		}})
	require.NoError(err)

	url := "/rest/v1/tasks/" + displayTaskId

	request, err := http.NewRequest("GET", url, nil)
	require.NoError(err)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(http.StatusOK, response.Code)

	var jsonBody map[string]interface{}
	err = json.Unmarshal(response.Body.Bytes(), &jsonBody)

	assert.NoError(err)
	assert.Equal(displayTask.Id, jsonBody["id"])
	found, ok := jsonBody["test_results"].(map[string]interface{})
	assert.True(ok)
	assert.Contains(found, "some-test")
}
