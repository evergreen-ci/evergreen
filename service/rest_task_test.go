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
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/testresult/testutil"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	goparquet "github.com/fraugster/parquet-go"
	"github.com/fraugster/parquet-go/floor"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func insertTaskForTesting(ctx context.Context, env evergreen.Environment, taskId, versionId, projectName string, testResults []testresult.TestResult, path string) (*task.Task, error) {
	svc := task.NewTestResultService(env)
	tsk := &task.Task{
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
		TaskOutputInfo: &task.TaskOutput{
			TestResults: task.TestResultOutput{
				Version: task.TestResultServiceEvergreen,
				BucketConfig: evergreen.BucketConfig{
					Type:              evergreen.BucketTypeLocal,
					TestResultsPrefix: "test-results",
					Name:              path,
				},
			},
		},
	}

	if len(testResults) > 0 {
		tsk.HasTestResults = true
		info := testresult.TestResultsInfo{TaskID: taskId, Execution: 0}
		tr := &testresult.DbTaskTestResults{
			ID:          info.ID(),
			Info:        info,
			CompletedAt: time.Now().UTC().Round(time.Millisecond),
		}

		testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: tsk.TaskOutputInfo.TestResults.BucketConfig.Name})
		if err != nil {
			return nil, err
		}
		w, err := testBucket.Writer(ctx, fmt.Sprintf("%s/%s", tsk.TaskOutputInfo.TestResults.BucketConfig.TestResultsPrefix, testresult.PartitionKey(tr.CreatedAt, tr.Info.Project, tr.ID)))
		if err != nil {
			return nil, err
		}
		defer func() { w.Close() }()

		pw := floor.NewWriter(goparquet.NewFileWriter(w, goparquet.WithSchemaDefinition(task.ParquetTestResultsSchemaDef)))
		savedParquet := testresult.ParquetTestResults{
			Version:   tr.Info.Version,
			Variant:   tr.Info.Variant,
			TaskName:  tr.Info.TaskName,
			TaskID:    tr.Info.TaskID,
			Execution: int32(tr.Info.Execution),
			Requester: tr.Info.Requester,
			CreatedAt: tr.CreatedAt.UTC(),
			Results:   make([]testresult.ParquetTestResult, len(testResults)),
		}
		for i := 0; i < len(testResults); i++ {
			savedParquet.Results[i] = testresult.ParquetTestResult{
				TestName:       testResults[i].TestName,
				GroupID:        utility.ToStringPtr(testResults[i].GroupID),
				Status:         testResults[i].Status,
				LogInfo:        testResults[i].LogInfo,
				LogURL:         utility.ToStringPtr(testResults[i].LogURL),
				TaskCreateTime: testResults[i].TaskCreateTime.UTC(),
				TestStartTime:  testResults[i].TestStartTime.UTC(),
				TestEndTime:    testResults[i].TestEndTime.UTC(),
			}
		}
		err = pw.Write(savedParquet)
		if err != nil {
			return nil, err
		}
		err = pw.Close()
		if err != nil {
			return nil, err
		}

		if err = db.Insert(ctx, testresult.Collection, testresult.DbTaskTestResults{
			ID:   info.ID(),
			Info: info,
		}); err != nil {
			return nil, err
		}
		if err = svc.AppendTestResultMetadata(testutil.MakeAppendTestResultMetadataReq(ctx, testResults, tr.ID)); err != nil {
			return nil, err
		}
	}
	if err := tsk.Insert(ctx); err != nil {
		return nil, err
	}

	return tsk, nil
}

func TestGetTaskInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	router, err := newTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
		assert.NoError(t, task.ClearTestResults(ctx, env))
	}()

	Convey("When finding info on a particular task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection, artifact.Collection))
		require.NoError(t, task.ClearTestResults(ctx, env))

		taskId := "my-task"
		versionId := "my-version"
		projectName := "project_test"

		testResult := testresult.TestResult{
			Status:        "success",
			TaskID:        taskId,
			Execution:     0,
			TestName:      "some-test",
			LogURL:        "some-url",
			TestStartTime: time.Now().Add(-9 * time.Minute),
			TestEndTime:   time.Now().Add(-1 * time.Minute),
		}
		testTask, err := insertTaskForTesting(ctx, env, taskId, versionId, projectName, []testresult.TestResult{testResult}, t.TempDir())
		So(err, ShouldBeNil)

		publicFile := artifact.File{
			Name: "Some Artifact",
			Link: "some-url",
		}
		noVisibilityFile := artifact.File{
			Name:       "Secret Artifact",
			Link:       "some-secret-url",
			Visibility: artifact.None,
		}
		taskArtifacts := artifact.Entry{
			TaskId: taskId,
			Files:  []artifact.File{publicFile, noVisibilityFile},
		}
		So(taskArtifacts.Upsert(t.Context()), ShouldBeNil)

		url := "/rest/v1/tasks/" + taskId

		request, err := http.NewRequest("GET", url, nil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		Convey("response should match contents of database and should omit hidden artifacts", func() {
			var jsonBody map[string]any
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

			jsonStatusDetails, ok := jsonBody["status_details"]
			So(ok, ShouldBeTrue)
			statusDetails, ok := jsonStatusDetails.(map[string]any)
			So(ok, ShouldBeTrue)

			So(statusDetails["timed_out"], ShouldEqual, testTask.Details.TimedOut)
			So(statusDetails["timeout_stage"], ShouldEqual, testTask.Details.Description)

			So(jsonBody["aborted"], ShouldEqual, testTask.Aborted)
			So(jsonBody["time_taken"], ShouldEqual, testTask.TimeTaken)
			So(jsonBody["expected_duration"], ShouldEqual, testTask.ExpectedDuration)

			jsonTestResults, ok := jsonBody["test_results"]
			So(ok, ShouldBeTrue)
			testResults, ok := jsonTestResults.(map[string]any)
			So(ok, ShouldBeTrue)
			So(len(testResults), ShouldEqual, 1)

			jsonTestResult, ok := testResults[testResult.TestName]
			So(ok, ShouldBeTrue)
			testResultForTestName, ok := jsonTestResult.(map[string]any)
			So(ok, ShouldBeTrue)

			So(testResultForTestName["status"], ShouldEqual, testResult.Status)
			So(testResultForTestName["time_taken"], ShouldNotBeNil) // value correctness is unchecked

			jsonTestResultLogs, ok := testResultForTestName["logs"]
			So(ok, ShouldBeTrue)
			testResultLogs, ok := jsonTestResultLogs.(map[string]any)
			So(ok, ShouldBeTrue)

			So(testResultLogs["url"], ShouldEqual, testResult.LogURL)

			var jsonFiles []map[string]any
			err = json.Unmarshal(*rawJSONBody["files"], &jsonFiles)
			So(err, ShouldBeNil)
			So(len(jsonFiles), ShouldEqual, 1)

			jsonFile := jsonFiles[0]
			So(jsonFile["name"], ShouldEqual, publicFile.Name)
			So(jsonFile["url"], ShouldEqual, publicFile.Link)
		})
	})

	Convey("When finding info on a nonexistent task", t, func() {
		taskId := "not-present"

		url := "/rest/v1/tasks/" + taskId

		request, err := http.NewRequest("GET", url, nil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusNotFound)

		Convey("response should contain a sensible error message", func() {
			var jsonBody map[string]any
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
	Convey("When finding the status of a particular task", t, func() {
		require.NoError(t, db.ClearCollections(task.Collection),
			"Error clearing '%v' collection", task.Collection)
		require.NoError(t, task.ClearTestResults(ctx, env))

		taskId := "my-task"

		testTask := &task.Task{
			Id:          taskId,
			Version:     "my-version",
			Project:     "my-project",
			DisplayName: "My task",
			Status:      "success",
			Details: apimodels.TaskEndDetail{
				TimedOut:    false,
				Description: "some-stage",
			},
			HasTestResults: true,
			TaskOutputInfo: &task.TaskOutput{
				TestResults: task.TestResultOutput{
					Version: task.TestResultServiceEvergreen,
					BucketConfig: evergreen.BucketConfig{
						Type:              evergreen.BucketTypeLocal,
						TestResultsPrefix: "test-results",
						Name:              t.TempDir(),
					},
				},
			},
		}
		testResult := testresult.TestResult{
			Status:        "success",
			TaskID:        testTask.Id,
			Execution:     testTask.Execution,
			TestName:      "some-test",
			LogURL:        "some-url",
			TestStartTime: time.Now().Add(-9 * time.Minute),
			TestEndTime:   time.Now().Add(-1 * time.Minute),
		}
		_, err = insertTaskForTesting(ctx, env, taskId, testTask.Version, testTask.Project, []testresult.TestResult{testResult}, t.TempDir())
		require.NoError(t, err)

		url := "/rest/v1/tasks/" + taskId + "/status"

		request, err := http.NewRequest("GET", url, nil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		Convey("response should match contents of database", func() {
			var jsonBody map[string]any
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)

			rawJSONBody := map[string]*json.RawMessage{}
			err = json.Unmarshal(response.Body.Bytes(), &rawJSONBody)
			So(err, ShouldBeNil)

			So(jsonBody["task_id"], ShouldEqual, testTask.Id)
			So(jsonBody["task_name"], ShouldEqual, testTask.DisplayName)
			So(jsonBody["status"], ShouldEqual, testTask.Status)

			jsonStatusDetails, ok := jsonBody["status_details"]
			So(ok, ShouldBeTrue)
			statusDetails, ok := jsonStatusDetails.(map[string]any)
			So(ok, ShouldBeTrue)

			So(statusDetails["timed_out"], ShouldEqual, testTask.Details.TimedOut)
			So(statusDetails["timeout_stage"], ShouldEqual, testTask.Details.Description)

			jsonTestResults, ok := jsonBody["tests"]
			So(ok, ShouldBeTrue)
			testResults, ok := jsonTestResults.(map[string]any)
			So(ok, ShouldBeTrue)
			So(len(testResults), ShouldEqual, 1)

			jsonTestResult, ok := testResults[testResult.TestName]
			So(ok, ShouldBeTrue)
			testResultForTestName, ok := jsonTestResult.(map[string]any)
			So(ok, ShouldBeTrue)

			So(testResultForTestName["status"], ShouldEqual, testResult.Status)
			So(testResultForTestName["time_taken"], ShouldNotBeNil) // value correctness is unchecked

			jsonTestResultLogs, ok := testResultForTestName["logs"]
			So(ok, ShouldBeTrue)
			testResultLogs, ok := jsonTestResultLogs.(map[string]any)
			So(ok, ShouldBeTrue)

			So(testResultLogs["url"], ShouldEqual, testResult.LogURL)
		})
	})

	Convey("When finding the status of a nonexistent task", t, func() {
		taskId := "not-present"

		url := "/rest/v1/tasks/" + taskId + "/status"

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusNotFound)

		Convey("response should contain a sensible error message", func() {
			var jsonBody map[string]any
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
	defer func() {
		assert.NoError(db.ClearCollections(task.Collection))
		assert.NoError(task.ClearTestResults(ctx, env))
	}()

	executionTaskId := "execution-task"
	displayTaskId := "display-task"
	versionId := "my-version"
	projectName := "project_test"

	testResult := testresult.TestResult{
		Status:        "success",
		TaskID:        executionTaskId,
		Execution:     0,
		TestName:      "some-test",
		LogURL:        "some-url",
		TestStartTime: time.Now().Add(-9 * time.Minute),
		TestEndTime:   time.Now().Add(-1 * time.Minute),
	}
	_, err = insertTaskForTesting(ctx, env, executionTaskId, versionId, projectName, []testresult.TestResult{testResult}, t.TempDir())
	assert.NoError(err)
	displayTask, err := insertTaskForTesting(ctx, env, displayTaskId, versionId, projectName, nil, t.TempDir())
	assert.NoError(err)
	displayTask.ExecutionTasks = []string{executionTaskId}
	err = db.UpdateContext(t.Context(), task.Collection,
		bson.M{task.IdKey: displayTaskId},
		bson.M{"$set": bson.M{
			task.ExecutionTasksKey: []string{executionTaskId},
			task.DisplayOnlyKey:    true,
		}})
	require.NoError(err)

	url := "/rest/v1/tasks/" + displayTaskId

	request, err := http.NewRequest("GET", url, nil)
	require.NoError(err)
	request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(http.StatusOK, response.Code)

	var jsonBody map[string]any
	err = json.Unmarshal(response.Body.Bytes(), &jsonBody)

	assert.NoError(err)
	assert.Equal(displayTask.Id, jsonBody["id"])
	found, ok := jsonBody["test_results"].(map[string]any)
	assert.True(ok)
	assert.Contains(found, "some-test")
}
