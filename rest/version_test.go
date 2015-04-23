package rest

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	"10gen.com/mci/web"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	. "github.com/smartystreets/goconvey/convey"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var (
	versionTestConfig = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(versionTestConfig))
}

func TestGetRecentVersions(t *testing.T) {

	app := web.NewApp()
	router := &mux.Router{}
	RegisterRoutes(app, router)

	Convey("When finding recent versions", t, func() {
		util.HandleTestingErr(db.Clear(model.VersionsCollection), t,
			"Error clearing '%v' collection", model.VersionsCollection)

		projectName := "my-project"
		otherProjectName := "my-other-project"
		So(projectName, ShouldNotEqual, otherProjectName) // sanity-check

		buildIdPreface := "build-id-for-version%v"

		So(NumRecentVersions, ShouldBeGreaterThan, 0)
		versions := make([]*model.Version, 0, NumRecentVersions)

		// Insert a bunch of versions into the database
		for i := 0; i < NumRecentVersions; i++ {
			version := &model.Version{
				Id:                  fmt.Sprintf("version%v", i),
				Project:             projectName,
				Author:              fmt.Sprintf("author%v", i),
				Revision:            fmt.Sprintf("%x", rand.Int()),
				Message:             fmt.Sprintf("message%v", i),
				RevisionOrderNumber: i + 1,
				Requester:           mci.RepotrackerVersionRequester,
			}
			So(version.Insert(), ShouldBeNil)
			versions = append(versions, version)
		}

		// Construct a version that should not be present in the response
		// since the length of the build ids slice is different than that
		// of the build variants slice
		earlyVersion := &model.Version{
			Id:                  "some-id",
			Project:             projectName,
			Author:              "some-author",
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Message:             "some-message",
			RevisionOrderNumber: 0,
			Requester:           mci.RepotrackerVersionRequester,
		}
		So(earlyVersion.Insert(), ShouldBeNil)

		// Construct a version that should not be present in the response
		// since it belongs to a different project
		otherVersion := &model.Version{
			Id:                  "some-other-id",
			Project:             otherProjectName,
			Author:              "some-other-author",
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Message:             "some-other-message",
			RevisionOrderNumber: NumRecentVersions + 1,
			Requester:           mci.RepotrackerVersionRequester,
		}
		So(otherVersion.Insert(), ShouldBeNil)

		builds := make([]*model.Build, 0, NumRecentVersions)
		task := model.TaskCache{
			Id:          "some-task-id",
			DisplayName: "some-task-name",
			Status:      "success",
			TimeTaken:   time.Duration(100 * time.Millisecond),
		}

		for i := 0; i < NumRecentVersions; i++ {
			build := &model.Build{
				Id:           fmt.Sprintf(buildIdPreface, i),
				Version:      versions[i].Id,
				BuildVariant: "some-build-variant",
				DisplayName:  "Some Build Variant",
				Tasks:        []model.TaskCache{task},
			}
			So(build.Insert(), ShouldBeNil)
			builds = append(builds, build)
		}

		url, err := router.Get("recent_versions").URL("project_name", projectName)
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

			So(jsonBody["project"], ShouldEqual, projectName)

			var jsonVersions []map[string]interface{}
			err = json.Unmarshal(*rawJsonBody["versions"], &jsonVersions)
			So(err, ShouldBeNil)
			So(len(jsonVersions), ShouldEqual, len(versions))

			for i, version := range versions {
				jsonVersion := jsonVersions[len(jsonVersions)-i-1] // reverse order

				So(jsonVersion["version_id"], ShouldEqual, version.Id)
				So(jsonVersion["author"], ShouldEqual, version.Author)
				So(jsonVersion["revision"], ShouldEqual, version.Revision)
				So(jsonVersion["message"], ShouldEqual, version.Message)

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

				_jsonTask, ok := jsonTasks[task.DisplayName]
				So(ok, ShouldBeTrue)
				jsonTask, ok := _jsonTask.(map[string]interface{})
				So(ok, ShouldBeTrue)

				So(jsonTask["task_id"], ShouldEqual, task.Id)
				So(jsonTask["status"], ShouldEqual, task.Status)
				So(jsonTask["time_taken"], ShouldEqual, task.TimeTaken)
			}
		})
	})

	Convey("When finding recent versions for a nonexistent project", t, func() {
		projectName := "not-present"

		url, err := router.Get("recent_versions").URL("project_name", projectName)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

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

	app := web.NewApp()
	router := &mux.Router{}
	RegisterRoutes(app, router)

	Convey("When finding info on a particular version", t, func() {
		util.HandleTestingErr(db.Clear(model.VersionsCollection), t,
			"Error clearing '%v' collection", model.VersionsCollection)

		versionId := "my-version"
		projectName := "my-project"

		version := &model.Version{
			Id:                  versionId,
			CreateTime:          time.Now().Add(-20 * time.Minute),
			StartTime:           time.Now().Add(-10 * time.Minute),
			FinishTime:          time.Now().Add(-5 * time.Second),
			Project:             projectName,
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Author:              "some-author",
			AuthorEmail:         "some-email",
			Message:             "some-message",
			Status:              "success",
			Activated:           true,
			BuildIds:            []string{"some-build-id"},
			BuildVariants:       []string{"some-build-variant"},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			RepoKind:            "github",
			BatchTime:           rand.Int(),
			Identifier:          versionId,
			Remote:              false,
			RemotePath:          "",
			Requester:           mci.RepotrackerVersionRequester,
		}
		So(version.Insert(), ShouldBeNil)

		url, err := router.Get("version_info").URL("version_id", versionId)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)
		validateVersionInfo(version, response)
	})

	Convey("When finding info on a nonexistent version", t, func() {
		versionId := "not-present"

		url, err := router.Get("version_info").URL("version_id", versionId)
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
				fmt.Sprintf("Error finding version '%v'", versionId))
		})
	})
}

func TestGetVersionInfoViaRevision(t *testing.T) {

	app := web.NewApp()
	router := &mux.Router{}
	RegisterRoutes(app, router)

	projectName := "my-project"

	Convey("When finding info on a particular version by its revision", t, func() {
		util.HandleTestingErr(db.Clear(model.VersionsCollection), t,
			"Error clearing '%v' collection", model.VersionsCollection)

		versionId := "my-version"
		revision := fmt.Sprintf("%x", rand.Int())

		version := &model.Version{
			Id:                  versionId,
			CreateTime:          time.Now().Add(-20 * time.Minute),
			StartTime:           time.Now().Add(-10 * time.Minute),
			FinishTime:          time.Now().Add(-5 * time.Second),
			Project:             projectName,
			Revision:            revision,
			Author:              "some-author",
			AuthorEmail:         "some-email",
			Message:             "some-message",
			Status:              "success",
			Activated:           true,
			BuildIds:            []string{"some-build-id"},
			BuildVariants:       []string{"some-build-variant"},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			RepoKind:            "github",
			BatchTime:           rand.Int(),
			Identifier:          versionId,
			Remote:              false,
			RemotePath:          "",
			Requester:           mci.RepotrackerVersionRequester,
		}
		So(version.Insert(), ShouldBeNil)

		url, err := router.Get("version_info_via_revision").URL(
			"project_name", projectName, "revision", revision)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)
		validateVersionInfo(version, response)
	})

	Convey("When finding info on a nonexistent version by its revision", t, func() {
		revision := "not-present"

		url, err := router.Get("version_info_via_revision").URL(
			"project_name", projectName, "revision", revision)
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
				fmt.Sprintf("Error finding revision '%v' for project '%v'", revision, projectName))
		})
	})
}

func TestActivateVersion(t *testing.T) {

	app := web.NewApp()
	router := &mux.Router{}
	RegisterRoutes(app, router)

	Convey("When marking a particular version as active", t, func() {
		util.HandleTestingErr(db.Clear(model.VersionsCollection), t,
			"Error clearing '%v' collection", model.VersionsCollection)

		versionId := "my-version"
		projectName := "my-project"

		build := &model.Build{
			Id:           "some-build-id",
			BuildVariant: "some-build-variant",
		}
		So(build.Insert(), ShouldBeNil)

		version := &model.Version{
			Id:                  versionId,
			CreateTime:          time.Now().Add(-20 * time.Minute),
			StartTime:           time.Now().Add(-10 * time.Minute),
			FinishTime:          time.Now().Add(-5 * time.Second),
			Project:             projectName,
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Author:              "some-author",
			AuthorEmail:         "some-email",
			Message:             "some-message",
			Status:              "success",
			Activated:           false,
			BuildIds:            []string{build.Id},
			BuildVariants:       []string{build.BuildVariant},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			RepoKind:            "github",
			BatchTime:           rand.Int(),
			Identifier:          versionId,
			Remote:              false,
			RemotePath:          "",
			Requester:           mci.RepotrackerVersionRequester,
		}
		So(version.Activated, ShouldBeFalse)
		So(version.Insert(), ShouldBeNil)

		url, err := router.Get("version_info").URL("version_id", versionId)
		So(err, ShouldBeNil)

		var body = map[string]interface{}{
			"activated": true,
		}
		jsonBytes, err := json.Marshal(body)
		So(err, ShouldBeNil)
		bodyReader := bytes.NewReader(jsonBytes)

		request, err := http.NewRequest("PATCH", url.String(), bodyReader)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		version.Activated = true // version should have been activated
		validateVersionInfo(version, response)
	})

	Convey("When marking a nonexistent version as active", t, func() {
		versionId := "not-present"

		url, err := router.Get("version_info").URL("version_id", versionId)
		So(err, ShouldBeNil)

		var body = map[string]interface{}{
			"activated": true,
		}
		jsonBytes, err := json.Marshal(body)
		So(err, ShouldBeNil)
		bodyReader := bytes.NewReader(jsonBytes)

		request, err := http.NewRequest("PATCH", url.String(), bodyReader)
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
				fmt.Sprintf("Error finding version '%v'", versionId))
		})
	})
}

func TestGetVersionStatus(t *testing.T) {

	app := web.NewApp()
	router := &mux.Router{}
	RegisterRoutes(app, router)

	Convey("When finding the status of a particular version", t, func() {
		util.HandleTestingErr(db.Clear(model.BuildsCollection), t,
			"Error clearing '%v' collection", model.BuildsCollection)

		versionId := "my-version"

		task := model.TaskCache{
			Id:          "some-task-id",
			DisplayName: "some-task-name",
			Status:      "success",
			TimeTaken:   time.Duration(100 * time.Millisecond),
		}
		build := &model.Build{
			Id:           "some-build-id",
			Version:      versionId,
			BuildVariant: "some-build-variant",
			DisplayName:  "Some Build Variant",
			Tasks:        []model.TaskCache{task},
		}
		So(build.Insert(), ShouldBeNil)

		Convey("grouped by tasks", func() {
			groupBy := "tasks"

			url, err := router.Get("version_status").URL("version_id", versionId)
			So(err, ShouldBeNil)

			query := url.Query()
			query.Set("groupby", groupBy)
			url.RawQuery = query.Encode()

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

				_jsonBuild, ok := jsonTask[build.BuildVariant]
				So(ok, ShouldBeTrue)
				jsonBuild, ok := _jsonBuild.(map[string]interface{})
				So(ok, ShouldBeTrue)

				So(jsonBuild["task_id"], ShouldEqual, task.Id)
				So(jsonBuild["status"], ShouldEqual, task.Status)
				So(jsonBuild["time_taken"], ShouldEqual, task.TimeTaken)
			})

			Convey("is the default option", func() {
				url, err := router.Get("version_status").URL("version_id", versionId)
				So(err, ShouldBeNil)

				request, err := http.NewRequest("GET", url.String(), nil)
				So(err, ShouldBeNil)

				_response := httptest.NewRecorder()
				// Need match variables to be set so can call mux.Vars(request)
				// in the actual handler function
				router.ServeHTTP(_response, request)

				So(_response, ShouldResemble, response)
			})
		})

		Convey("grouped by builds", func() {
			groupBy := "builds"

			url, err := router.Get("version_status").URL("version_id", versionId)
			So(err, ShouldBeNil)

			query := url.Query()
			query.Set("groupby", groupBy)
			url.RawQuery = query.Encode()

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

			url, err := router.Get("version_status").URL("version_id", versionId)
			So(err, ShouldBeNil)

			query := url.Query()
			query.Set("groupby", groupBy)
			url.RawQuery = query.Encode()

			request, err := http.NewRequest("GET", url.String(), nil)
			So(err, ShouldBeNil)

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

			url, err := router.Get("version_status").URL("version_id", versionId)
			So(err, ShouldBeNil)

			query := url.Query()
			query.Set("groupby", groupBy)
			url.RawQuery = query.Encode()

			request, err := http.NewRequest("GET", url.String(), nil)
			So(err, ShouldBeNil)

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

			url, err := router.Get("version_status").URL("version_id", versionId)
			So(err, ShouldBeNil)

			query := url.Query()
			query.Set("groupby", groupBy)
			url.RawQuery = query.Encode()

			request, err := http.NewRequest("GET", url.String(), nil)
			So(err, ShouldBeNil)

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

func validateVersionInfo(version *model.Version, response *httptest.ResponseRecorder) {
	Convey("response should match contents of database", func() {
		var jsonBody map[string]interface{}
		err := json.Unmarshal(response.Body.Bytes(), &jsonBody)
		So(err, ShouldBeNil)

		var rawJsonBody map[string]*json.RawMessage
		err = json.Unmarshal(response.Body.Bytes(), &rawJsonBody)
		So(err, ShouldBeNil)

		So(jsonBody["id"], ShouldEqual, version.Id)

		var createTime time.Time
		err = json.Unmarshal(*rawJsonBody["create_time"], &createTime)
		So(err, ShouldBeNil)
		So(createTime, ShouldHappenWithin, timePrecision, version.CreateTime)

		var startTime time.Time
		err = json.Unmarshal(*rawJsonBody["start_time"], &startTime)
		So(err, ShouldBeNil)
		So(startTime, ShouldHappenWithin, timePrecision, version.StartTime)

		var finishTime time.Time
		err = json.Unmarshal(*rawJsonBody["finish_time"], &finishTime)
		So(err, ShouldBeNil)
		So(finishTime, ShouldHappenWithin, timePrecision, version.FinishTime)

		So(jsonBody["project"], ShouldEqual, version.Project)
		So(jsonBody["revision"], ShouldEqual, version.Revision)
		So(jsonBody["author"], ShouldEqual, version.Author)
		So(jsonBody["author_email"], ShouldEqual, version.AuthorEmail)
		So(jsonBody["message"], ShouldEqual, version.Message)
		So(jsonBody["status"], ShouldEqual, version.Status)
		So(jsonBody["activated"], ShouldEqual, version.Activated)

		var buildIds []string
		err = json.Unmarshal(*rawJsonBody["builds"], &buildIds)
		So(err, ShouldBeNil)
		So(buildIds, ShouldResemble, version.BuildIds)

		var buildVariants []string
		err = json.Unmarshal(*rawJsonBody["build_variants"], &buildVariants)
		So(err, ShouldBeNil)
		So(buildVariants, ShouldResemble, version.BuildVariants)

		So(jsonBody["order"], ShouldEqual, version.RevisionOrderNumber)
		So(jsonBody["owner_name"], ShouldEqual, version.Owner)
		So(jsonBody["repo_name"], ShouldEqual, version.Repo)
		So(jsonBody["branch_name"], ShouldEqual, version.Branch)
		So(jsonBody["repo_kind"], ShouldEqual, version.RepoKind)
		So(jsonBody["batch_time"], ShouldEqual, version.BatchTime)
		So(jsonBody["identifier"], ShouldEqual, version.Identifier)
		So(jsonBody["remote"], ShouldEqual, version.Remote)
		So(jsonBody["remote_path"], ShouldEqual, version.RemotePath)
		So(jsonBody["requester"], ShouldEqual, version.Requester)
	})
}
