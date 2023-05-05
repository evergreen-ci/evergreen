package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/user"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/gimlet"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestGetRecentVersions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	router, err := newTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	err = modelutil.CreateTestLocalConfig(buildTestConfig, "mci-test", "")
	require.NoError(t, err, "Error loading local config mci-test")

	err = modelutil.CreateTestLocalConfig(buildTestConfig, "render", "")
	require.NoError(t, err, "Error loading local config render")

	Convey("When finding recent versions", t, func() {
		require.NoError(t, db.ClearCollections(model.VersionCollection, build.Collection, task.Collection, model.ProjectRefCollection))

		projectName := "project_test"

		err = modelutil.CreateTestLocalConfig(buildTestConfig, projectName, "")
		So(err, ShouldBeNil)
		otherProjectName := "my-other-project"
		So(projectName, ShouldNotEqual, otherProjectName) // sanity-check

		buildIdPreface := "build-id-for-version%v"
		taskIdPreface := "task-id-for-version%v"

		So(NumRecentVersions, ShouldBeGreaterThan, 0)
		versions := make([]*model.Version, 0, NumRecentVersions)

		// Insert a bunch of versions into the database
		for i := 0; i < NumRecentVersions; i++ {
			v := &model.Version{
				Id:                  fmt.Sprintf("version%v", i),
				Identifier:          projectName,
				Author:              fmt.Sprintf("author%v", i),
				Revision:            fmt.Sprintf("%x", rand.Int()),
				Message:             fmt.Sprintf("message%v", i),
				RevisionOrderNumber: i + 1,
				Requester:           evergreen.RepotrackerVersionRequester,
			}
			So(v.Insert(), ShouldBeNil)
			versions = append(versions, v)
		}

		// Construct a version that should not be present in the response
		// since the length of the build ids slice is different than that
		// of the build variants slice
		earlyVersion := &model.Version{
			Id:                  "some-id",
			Identifier:          projectName,
			Author:              "some-author",
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Message:             "some-message",
			RevisionOrderNumber: 0,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(earlyVersion.Insert(), ShouldBeNil)

		// Construct a version that should not be present in the response
		// since it belongs to a different project
		otherVersion := &model.Version{
			Id:                  "some-other-id",
			Identifier:          otherProjectName,
			Author:              "some-other-author",
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Message:             "some-other-message",
			RevisionOrderNumber: NumRecentVersions + 1,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(otherVersion.Insert(), ShouldBeNil)

		builds := make([]*build.Build, 0, NumRecentVersions)
		tasks := make([]*task.Task, 0, NumRecentVersions)

		for i := 0; i < NumRecentVersions; i++ {
			build := &build.Build{
				Id:           fmt.Sprintf(buildIdPreface, i),
				Version:      versions[i].Id,
				BuildVariant: "some-build-variant",
				DisplayName:  "Some Build Variant",
			}
			So(build.Insert(), ShouldBeNil)
			builds = append(builds, build)

			task := &task.Task{
				Id:           fmt.Sprintf(taskIdPreface, i),
				Version:      versions[i].Id,
				DisplayName:  "some-task-name",
				Status:       "success",
				TimeTaken:    100 * time.Millisecond,
				BuildVariant: build.BuildVariant,
			}
			So(task.Insert(), ShouldBeNil)
			tasks = append(tasks, task)
		}

		url := "/rest/v1/projects/" + projectName + "/versions"

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)
		So(response.Code, ShouldEqual, http.StatusOK)

		Convey("response should match contents of database", func() {
			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)
			link := response.Header().Get("Link")
			So(link, ShouldNotBeEmpty)
			So(link, ShouldContainSubstring, "/versions")
			So(link, ShouldContainSubstring, "limit=10&start=0")

			var rawJsonBody map[string]*json.RawMessage
			err = json.Unmarshal(response.Body.Bytes(), &rawJsonBody)
			So(err, ShouldBeNil)

			So(jsonBody["project"], ShouldEqual, projectName)

			var jsonVersions []map[string]interface{}
			err = json.Unmarshal(*rawJsonBody["versions"], &jsonVersions)
			So(err, ShouldBeNil)
			So(len(jsonVersions), ShouldEqual, len(versions))

			for i, v := range versions {
				jsonVersion := jsonVersions[len(jsonVersions)-i-1] // reverse order

				So(jsonVersion["version_id"], ShouldEqual, v.Id)
				So(jsonVersion["author"], ShouldEqual, v.Author)
				So(jsonVersion["revision"], ShouldEqual, v.Revision)
				So(jsonVersion["message"], ShouldEqual, v.Message)

				_jsonBuilds, ok := jsonVersion["builds"]
				So(ok, ShouldBeTrue)
				jsonBuilds, ok := _jsonBuilds.(map[string]interface{})
				So(ok, ShouldBeTrue)
				So(len(jsonBuilds), ShouldEqual, 1)

				_jsonBuild, ok := jsonBuilds[builds[i].BuildVariant]
				So(ok, ShouldBeTrue)
				jsonBuild, ok := _jsonBuild.(map[string]interface{})
				So(ok, ShouldBeTrue)

				So(jsonBuild["build_id"], ShouldEqual, builds[i].Id)
				So(jsonBuild["name"], ShouldEqual, builds[i].DisplayName)

				_jsonTasks, ok := jsonBuild["tasks"]
				So(ok, ShouldBeTrue)
				jsonTasks, ok := _jsonTasks.(map[string]interface{})
				So(ok, ShouldBeTrue)
				So(len(jsonTasks), ShouldEqual, 1)

				_jsonTask, ok := jsonTasks[tasks[i].DisplayName]
				So(ok, ShouldBeTrue)
				jsonTask, ok := _jsonTask.(map[string]interface{})
				So(ok, ShouldBeTrue)

				So(jsonTask["task_id"], ShouldEqual, tasks[i].Id)
				So(jsonTask["status"], ShouldEqual, tasks[i].Status)
				So(jsonTask["time_taken"], ShouldEqual, tasks[i].TimeTaken)
			}
		})
	})

	Convey("When finding recent versions for a nonexistent project", t, func() {
		projectName := "not-present"

		url := "/rest/v1/projects/" + projectName + "/versions"

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		Convey("response should contain no versions", func() {
			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)

			var rawJsonBody map[string]*json.RawMessage
			err = json.Unmarshal(response.Body.Bytes(), &rawJsonBody)
			So(err, ShouldBeNil)

			So(jsonBody["project"], ShouldEqual, projectName)

			var jsonVersions []map[string]interface{}
			err = json.Unmarshal(*rawJsonBody["versions"], &jsonVersions)
			So(err, ShouldBeNil)
			So(jsonVersions, ShouldBeEmpty)
		})
	})
}

func TestGetVersionInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	env.SetUserManager(serviceutil.MockUserManager{})
	router, err := newTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	err = modelutil.CreateTestLocalConfig(buildTestConfig, "mci-test", "")
	require.NoError(t, err, "Error loading local config mci-test")

	err = modelutil.CreateTestLocalConfig(buildTestConfig, "render", "")
	require.NoError(t, err, "Error loading local config render")

	Convey("When finding info on a particular version", t, func() {
		require.NoError(t, db.Clear(model.VersionCollection),
			"Error clearing '%v' collection", model.VersionCollection)

		versionId := "my-version"
		projectName := "project_test"

		err = modelutil.CreateTestLocalConfig(buildTestConfig, projectName, "")
		So(err, ShouldBeNil)

		v := &model.Version{
			Id:          versionId,
			CreateTime:  time.Now().Add(-20 * time.Minute),
			StartTime:   time.Now().Add(-10 * time.Minute),
			FinishTime:  time.Now().Add(-5 * time.Second),
			Revision:    fmt.Sprintf("%x", rand.Int()),
			Author:      "some-author",
			AuthorEmail: "some-email",
			Message:     "some-message",
			Status:      "success",
			BuildIds:    []string{"some-build-id"},
			BuildVariants: []model.VersionBuildStatus{{
				BuildVariant: "some-build-variant",
				ActivationStatus: model.ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now().Add(-20 * time.Minute),
				},
				BuildId: "some-build-id"}},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			Identifier:          versionId,
			Remote:              false,
			RemotePath:          "",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(v.Insert(), ShouldBeNil)

		url := "/rest/v1/versions/" + versionId

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)
		validateVersionInfo(v, response)
	})

	Convey("When finding info on a nonexistent version", t, func() {
		versionId := "not-present"

		url := "/rest/v1/versions/" + versionId
		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

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

func TestGetVersionInfoViaRevision(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	router, err := newTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	projectName := "project_test"

	Convey("When finding info on a particular version by its revision", t, func() {
		require.NoError(t, db.Clear(model.VersionCollection),
			"Error clearing '%v' collection", model.VersionCollection)

		versionId := "my-version"
		revision := fmt.Sprintf("%x", rand.Int())

		v := &model.Version{
			Id:          versionId,
			CreateTime:  time.Now().Add(-20 * time.Minute),
			StartTime:   time.Now().Add(-10 * time.Minute),
			FinishTime:  time.Now().Add(-5 * time.Second),
			Revision:    revision,
			Author:      "some-author",
			AuthorEmail: "some-email",
			Message:     "some-message",
			Status:      "success",
			BuildIds:    []string{"some-build-id"},
			BuildVariants: []model.VersionBuildStatus{{
				BuildVariant: "some-build-variant",
				ActivationStatus: model.ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now().Add(-20 * time.Minute),
				},
				BuildId: "some-build-id"}},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			Identifier:          projectName,
			Remote:              false,
			RemotePath:          "",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(v.Insert(), ShouldBeNil)

		url := fmt.Sprintf("/rest/v1/projects/%s/revisions/%s", projectName, revision)

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)
		validateVersionInfo(v, response)
	})

	Convey("When finding info on a nonexistent version by its revision", t, func() {
		revision := "not-present"

		url := fmt.Sprintf("/rest/v1/projects/%s/revisions/%s", projectName, revision)

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

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

func TestActivateVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	env.SetUserManager(serviceutil.MockUserManager{})
	router, err := newAuthTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	Convey("When marking a particular version as active", t, func() {
		require.NoError(t, db.ClearCollections(model.VersionCollection, build.Collection),
			"Error clearing collections")

		versionId := "my-version"
		projectName := "project_test"

		build := &build.Build{
			Id:           "some-build-id",
			BuildVariant: "some-build-variant",
		}
		So(build.Insert(), ShouldBeNil)

		v := &model.Version{
			Id:          versionId,
			CreateTime:  time.Now().Add(-20 * time.Minute),
			StartTime:   time.Now().Add(-10 * time.Minute),
			FinishTime:  time.Now().Add(-5 * time.Second),
			Revision:    fmt.Sprintf("%x", rand.Int()),
			Author:      "some-author",
			AuthorEmail: "some-email",
			Message:     "some-message",
			Status:      "success",
			BuildIds:    []string{build.Id},
			BuildVariants: []model.VersionBuildStatus{{
				BuildVariant: "some-build-variant",
				ActivationStatus: model.ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now().Add(-20 * time.Minute),
				},
				BuildId: "some-build-id"},
			}, // nolint
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			Identifier:          projectName,
			Remote:              false,
			RemotePath:          "",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(v.Insert(), ShouldBeNil)

		url := "/rest/v1/versions/" + versionId

		var body = map[string]interface{}{
			"activated": true,
		}
		jsonBytes, err := json.Marshal(body)
		So(err, ShouldBeNil)
		bodyReader := bytes.NewReader(jsonBytes)

		request, err := http.NewRequest("PATCH", url, bodyReader)
		So(err, ShouldBeNil)
		// add auth cookie--this can be anything if we are using a MockUserManager
		request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		validateVersionInfo(v, response)
	})

	Convey("When marking a nonexistent version as active", t, func() {
		versionId := "not-present"

		url := "/rest/v1/versions/" + versionId

		var body = map[string]interface{}{
			"activated": true,
		}
		jsonBytes, err := json.Marshal(body)
		So(err, ShouldBeNil)
		bodyReader := bytes.NewReader(jsonBytes)

		request, err := http.NewRequest("PATCH", url, bodyReader)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// add auth cookie--this can be anything if we are using a MockUserManager
		request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusNotFound)

		Convey("response should contain a sensible error message", func() {
			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)
			So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
		})
	})

	Convey("When modifying a version without credentials", t, func() {
		versionId := "not-present"

		url := "/rest/v1/versions/" + versionId

		var body = map[string]interface{}{
			"activated": true,
		}
		jsonBytes, err := json.Marshal(body)
		So(err, ShouldBeNil)
		bodyReader := bytes.NewReader(jsonBytes)

		request, err := http.NewRequest("PATCH", url, bodyReader)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		Convey("response should indicate a permission error", func() {
			So(response.Code, ShouldEqual, http.StatusUnauthorized)
		})
	})
}

func TestGetVersionStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	router, err := newTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	Convey("When finding the status of a particular version", t, func() {
		require.NoError(t, db.ClearCollections(build.Collection, task.Collection),
			"Error clearing '%v' collection", build.Collection)

		versionId := "my-version"

		task := task.Task{
			Id:           "some-task-id",
			DisplayName:  "some-task-name",
			Status:       "success",
			TimeTaken:    100 * time.Millisecond,
			BuildVariant: "some-build-variant",
			Version:      versionId,
		}
		So(task.Insert(), ShouldBeNil)
		build := &build.Build{
			Id:           "some-build-id",
			Version:      versionId,
			BuildVariant: "some-build-variant",
			DisplayName:  "Some Build Variant",
			Tasks:        []build.TaskCache{{Id: task.Id}},
		}
		So(build.Insert(), ShouldBeNil)

		Convey("grouped by tasks", func() {
			groupBy := "tasks"

			url := "/rest/v1/versions/" + versionId + "/status?groupby=" + groupBy

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)

			So(response.Code, ShouldEqual, http.StatusOK)

			Convey("response should match contents of database", func() {
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)

				So(jsonBody["version_id"], ShouldEqual, versionId)

				_jsonTasks, ok := jsonBody["tasks"]
				So(ok, ShouldBeTrue)
				jsonTasks, ok := _jsonTasks.(map[string]interface{})
				So(ok, ShouldBeTrue)
				So(len(jsonTasks), ShouldEqual, 1)

				_jsonTask, ok := jsonTasks[task.DisplayName]
				So(ok, ShouldBeTrue)
				jsonTask, ok := _jsonTask.(map[string]interface{})
				So(ok, ShouldBeTrue)

				_jsonBuild, ok := jsonTask[task.BuildVariant]
				So(ok, ShouldBeTrue)
				jsonBuild, ok := _jsonBuild.(map[string]interface{})
				So(ok, ShouldBeTrue)

				So(jsonBuild["task_id"], ShouldEqual, task.Id)
				So(jsonBuild["status"], ShouldEqual, task.Status)
				So(jsonBuild["time_taken"], ShouldEqual, task.TimeTaken)
			})

			Convey("is the default option", func() {

				url := "/rest/v1/versions/" + versionId + "/status"

				request, err := http.NewRequest("GET", url, nil)
				So(err, ShouldBeNil)
				request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

				_response := httptest.NewRecorder()
				// Need match variables to be set so can call mux.Vars(request)
				// in the actual handler function
				router.ServeHTTP(_response, request)

				So(_response, ShouldResemble, response)
			})
		})

		Convey("grouped by builds", func() {
			groupBy := "builds"

			url := "/rest/v1/versions/" + versionId + "/status?groupby=" + groupBy

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)

			So(response.Code, ShouldEqual, http.StatusOK)

			Convey("response should match contents of database", func() {
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)

				So(jsonBody["version_id"], ShouldEqual, versionId)

				_jsonBuilds, ok := jsonBody["builds"]
				So(ok, ShouldBeTrue)
				jsonBuilds, ok := _jsonBuilds.(map[string]interface{})
				So(ok, ShouldBeTrue)
				So(len(jsonBuilds), ShouldEqual, 1)

				_jsonBuild, ok := jsonBuilds[build.BuildVariant]
				So(ok, ShouldBeTrue)
				jsonBuild, ok := _jsonBuild.(map[string]interface{})
				So(ok, ShouldBeTrue)

				_jsonTask, ok := jsonBuild[task.DisplayName]
				So(ok, ShouldBeTrue)
				jsonTask, ok := _jsonTask.(map[string]interface{})
				So(ok, ShouldBeTrue)

				So(jsonTask["task_id"], ShouldEqual, task.Id)
				So(jsonTask["status"], ShouldEqual, task.Status)
				So(jsonTask["time_taken"], ShouldEqual, task.TimeTaken)
			})
		})

		Convey("grouped by an invalid option", func() {
			groupBy := "invalidOption"

			url := "/rest/v1/versions/" + versionId + "/status?groupby=" + groupBy

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)

			So(response.Code, ShouldEqual, http.StatusBadRequest)

			var jsonBody map[string]interface{}
			err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
			So(err, ShouldBeNil)

			So(jsonBody["message"], ShouldEqual,
				fmt.Sprintf("Invalid groupby parameter '%v'", groupBy))
		})
	})

	Convey("When finding the status of a nonexistent version", t, func() {
		versionId := "not-present"

		Convey("grouped by tasks", func() {
			groupBy := "tasks"

			url := "/rest/v1/versions/" + versionId + "/status?groupby=" + groupBy

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)

			So(response.Code, ShouldEqual, http.StatusOK)

			Convey("response should contain a sensible error message", func() {
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)

				_jsonTasks, ok := jsonBody["tasks"]
				So(ok, ShouldBeTrue)
				jsonTasks, ok := _jsonTasks.(map[string]interface{})
				So(ok, ShouldBeTrue)
				So(jsonTasks, ShouldBeEmpty)
			})
		})

		Convey("grouped by builds", func() {
			versionId := "not-present"
			groupBy := "builds"

			url := "/rest/v1/versions/" + versionId + "/status?groupby=" + groupBy

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

			response := httptest.NewRecorder()
			// Need match variables to be set so can call mux.Vars(request)
			// in the actual handler function
			router.ServeHTTP(response, request)

			So(response.Code, ShouldEqual, http.StatusOK)

			Convey("response should contain a sensible error message", func() {
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)

				_jsonBuilds, ok := jsonBody["builds"]
				So(ok, ShouldBeTrue)
				jsonBuilds, ok := _jsonBuilds.(map[string]interface{})
				So(ok, ShouldBeTrue)
				So(jsonBuilds, ShouldBeEmpty)
			})
		})
	})
}

func validateVersionInfo(v *model.Version, response *httptest.ResponseRecorder) {
	Convey("response should match contents of database", func() {
		var jsonBody map[string]interface{}
		err := json.Unmarshal(response.Body.Bytes(), &jsonBody)
		So(err, ShouldBeNil)

		var rawJsonBody map[string]*json.RawMessage
		err = json.Unmarshal(response.Body.Bytes(), &rawJsonBody)
		So(err, ShouldBeNil)

		So(jsonBody["id"], ShouldEqual, v.Id)

		var createTime time.Time
		err = json.Unmarshal(*rawJsonBody["create_time"], &createTime)
		So(err, ShouldBeNil)
		So(createTime, ShouldHappenWithin, TimePrecision, v.CreateTime)

		var startTime time.Time
		err = json.Unmarshal(*rawJsonBody["start_time"], &startTime)
		So(err, ShouldBeNil)
		So(startTime, ShouldHappenWithin, TimePrecision, v.StartTime)

		var finishTime time.Time
		err = json.Unmarshal(*rawJsonBody["finish_time"], &finishTime)
		So(err, ShouldBeNil)
		So(finishTime, ShouldHappenWithin, TimePrecision, v.FinishTime)

		So(jsonBody["project"], ShouldEqual, v.Identifier)
		So(jsonBody["revision"], ShouldEqual, v.Revision)
		So(jsonBody["author"], ShouldEqual, v.Author)
		So(jsonBody["author_email"], ShouldEqual, v.AuthorEmail)
		So(jsonBody["message"], ShouldEqual, v.Message)
		So(jsonBody["status"], ShouldEqual, v.Status)

		var buildIds []string
		err = json.Unmarshal(*rawJsonBody["builds"], &buildIds)
		So(err, ShouldBeNil)
		So(buildIds, ShouldResemble, v.BuildIds)

		var buildVariants []string
		err = json.Unmarshal(*rawJsonBody["build_variants"], &buildVariants)
		So(err, ShouldBeNil)
		So(buildVariants[0], ShouldResemble, v.BuildVariants[0].BuildVariant)

		So(jsonBody["order"], ShouldEqual, v.RevisionOrderNumber)
		So(jsonBody["owner_name"], ShouldEqual, v.Owner)
		So(jsonBody["repo_name"], ShouldEqual, v.Repo)
		So(jsonBody["branch_name"], ShouldEqual, v.Branch)
		So(jsonBody["identifier"], ShouldEqual, v.Identifier)
		So(jsonBody["remote"], ShouldEqual, v.Remote)
		So(jsonBody["remote_path"], ShouldEqual, v.RemotePath)
		So(jsonBody["requester"], ShouldEqual, v.Requester)
	})
}
