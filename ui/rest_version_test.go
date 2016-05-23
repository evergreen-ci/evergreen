package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/render"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	versionTestConfig = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(versionTestConfig))
}

func TestGetRecentVersions(t *testing.T) {

	userManager, err := auth.LoadUserManager(versionTestConfig.AuthConfig)
	testutil.HandleTestingErr(err, t, "Failure in loading UserManager from config")

	uis := UIServer{
		RootURL:     versionTestConfig.Ui.Url,
		Settings:    *versionTestConfig,
		UserManager: userManager,
	}

	home := evergreen.FindEvergreenHome()

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: true,
	})
	uis.InitPlugins()

	router, err := uis.NewRouter()
	testutil.HandleTestingErr(err, t, "Failure in uis.NewRouter()")

	err = testutil.CreateTestLocalConfig(buildTestConfig, "mci-test", "")
	testutil.HandleTestingErr(err, t, "Error loading local config mci-test")

	err = testutil.CreateTestLocalConfig(buildTestConfig, "render", "")
	testutil.HandleTestingErr(err, t, "Error loading local config render")

	Convey("When finding recent versions", t, func() {
		testutil.HandleTestingErr(db.ClearCollections(version.Collection, build.Collection), t,
			"Error clearing '%v' collection", version.Collection)

		projectName := "project_test"

		err = testutil.CreateTestLocalConfig(buildTestConfig, projectName, "")
		So(err, ShouldBeNil)
		otherProjectName := "my-other-project"
		So(projectName, ShouldNotEqual, otherProjectName) // sanity-check

		buildIdPreface := "build-id-for-version%v"

		So(NumRecentVersions, ShouldBeGreaterThan, 0)
		versions := make([]*version.Version, 0, NumRecentVersions)

		// Insert a bunch of versions into the database
		for i := 0; i < NumRecentVersions; i++ {
			v := &version.Version{
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
		earlyVersion := &version.Version{
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
		otherVersion := &version.Version{
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
		task := build.TaskCache{
			Id:          "some-task-id",
			DisplayName: "some-task-name",
			Status:      "success",
			TimeTaken:   time.Duration(100 * time.Millisecond),
		}

		for i := 0; i < NumRecentVersions; i++ {
			build := &build.Build{
				Id:           fmt.Sprintf(buildIdPreface, i),
				Version:      versions[i].Id,
				BuildVariant: "some-build-variant",
				DisplayName:  "Some Build Variant",
				Tasks:        []build.TaskCache{task},
			}
			So(build.Insert(), ShouldBeNil)
			builds = append(builds, build)
		}

		url, err := router.Get("recent_versions").URL("project_id", projectName)
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

		url, err := router.Get("recent_versions").URL("project_id", projectName)
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

	uis := UIServer{
		RootURL:     versionTestConfig.Ui.Url,
		Settings:    *versionTestConfig,
		UserManager: testutil.MockUserManager{},
	}
	home := evergreen.FindEvergreenHome()

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: true,
	})
	uis.InitPlugins()

	router, err := uis.NewRouter()
	testutil.HandleTestingErr(err, t, "Failure in uis.NewRouter()")

	err = testutil.CreateTestLocalConfig(buildTestConfig, "mci-test", "")
	testutil.HandleTestingErr(err, t, "Error loading local config mci-test")

	err = testutil.CreateTestLocalConfig(buildTestConfig, "render", "")
	testutil.HandleTestingErr(err, t, "Error loading local config render")

	Convey("When finding info on a particular version", t, func() {
		testutil.HandleTestingErr(db.Clear(version.Collection), t,
			"Error clearing '%v' collection", version.Collection)

		versionId := "my-version"
		projectName := "project_test"

		err = testutil.CreateTestLocalConfig(buildTestConfig, projectName, "")
		So(err, ShouldBeNil)

		v := &version.Version{
			Id:                  versionId,
			CreateTime:          time.Now().Add(-20 * time.Minute),
			StartTime:           time.Now().Add(-10 * time.Minute),
			FinishTime:          time.Now().Add(-5 * time.Second),
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Author:              "some-author",
			AuthorEmail:         "some-email",
			Message:             "some-message",
			Status:              "success",
			BuildIds:            []string{"some-build-id"},
			BuildVariants:       []version.BuildStatus{{"some-build-variant", true, time.Now().Add(-20 * time.Minute), "some-build-id"}},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			RepoKind:            "github",
			Identifier:          versionId,
			Remote:              false,
			RemotePath:          "",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(v.Insert(), ShouldBeNil)

		url, err := router.Get("version_info").URL("version_id", versionId)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		Println(response.Body)

		So(response.Code, ShouldEqual, http.StatusOK)
		validateVersionInfo(v, response)
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
			So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
		})
	})
}

func TestGetVersionInfoViaRevision(t *testing.T) {

	userManager, err := auth.LoadUserManager(versionTestConfig.AuthConfig)
	testutil.HandleTestingErr(err, t, "Failure in loading UserManager from config")

	uis := UIServer{
		RootURL:     versionTestConfig.Ui.Url,
		Settings:    *versionTestConfig,
		UserManager: userManager,
	}

	home := evergreen.FindEvergreenHome()

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: true,
	})
	uis.InitPlugins()

	router, err := uis.NewRouter()
	testutil.HandleTestingErr(err, t, "Failure in uis.NewRouter()")

	projectName := "project_test"

	Convey("When finding info on a particular version by its revision", t, func() {
		testutil.HandleTestingErr(db.Clear(version.Collection), t,
			"Error clearing '%v' collection", version.Collection)

		versionId := "my-version"
		revision := fmt.Sprintf("%x", rand.Int())

		v := &version.Version{
			Id:                  versionId,
			CreateTime:          time.Now().Add(-20 * time.Minute),
			StartTime:           time.Now().Add(-10 * time.Minute),
			FinishTime:          time.Now().Add(-5 * time.Second),
			Revision:            revision,
			Author:              "some-author",
			AuthorEmail:         "some-email",
			Message:             "some-message",
			Status:              "success",
			BuildIds:            []string{"some-build-id"},
			BuildVariants:       []version.BuildStatus{{"some-build-variant", true, time.Now().Add(-20 * time.Minute), "some-build-id"}},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			RepoKind:            "github",
			Identifier:          projectName,
			Remote:              false,
			RemotePath:          "",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(v.Insert(), ShouldBeNil)

		url, err := router.Get("version_info_via_revision").URL(
			"project_id", projectName, "revision", revision)
		So(err, ShouldBeNil)

		request, err := http.NewRequest("GET", url.String(), nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		router.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)
		validateVersionInfo(v, response)
	})

	Convey("When finding info on a nonexistent version by its revision", t, func() {
		revision := "not-present"

		url, err := router.Get("version_info_via_revision").URL(
			"project_id", projectName, "revision", revision)
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
			So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
		})
	})
}

func TestActivateVersion(t *testing.T) {

	uis := UIServer{
		RootURL:     versionTestConfig.Ui.Url,
		Settings:    *versionTestConfig,
		UserManager: testutil.MockUserManager{},
	}

	home := evergreen.FindEvergreenHome()

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: true,
	})
	uis.InitPlugins()

	router, err := uis.NewRouter()
	testutil.HandleTestingErr(err, t, "Failure in uis.NewRouter()")

	n := negroni.New()
	n.Use(negroni.HandlerFunc(UserMiddleware(uis.UserManager)))
	n.UseHandler(router)

	Convey("When marking a particular version as active", t, func() {
		testutil.HandleTestingErr(db.Clear(version.Collection), t,
			"Error clearing '%v' collection", version.Collection)

		versionId := "my-version"
		projectName := "project_test"

		build := &build.Build{
			Id:           "some-build-id",
			BuildVariant: "some-build-variant",
		}
		So(build.Insert(), ShouldBeNil)

		v := &version.Version{
			Id:                  versionId,
			CreateTime:          time.Now().Add(-20 * time.Minute),
			StartTime:           time.Now().Add(-10 * time.Minute),
			FinishTime:          time.Now().Add(-5 * time.Second),
			Revision:            fmt.Sprintf("%x", rand.Int()),
			Author:              "some-author",
			AuthorEmail:         "some-email",
			Message:             "some-message",
			Status:              "success",
			BuildIds:            []string{build.Id},
			BuildVariants:       []version.BuildStatus{{"some-build-variant", true, time.Now().Add(-20 * time.Minute), "some-build-id"}},
			RevisionOrderNumber: rand.Int(),
			Owner:               "some-owner",
			Repo:                "some-repo",
			Branch:              "some-branch",
			RepoKind:            "github",
			Identifier:          projectName,
			Remote:              false,
			RemotePath:          "",
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(v.Insert(), ShouldBeNil)

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
		// add auth cookie--this can be anything if we are using a MockUserManager
		request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})

		response := httptest.NewRecorder()
		// Need match variables to be set so can call mux.Vars(request)
		// in the actual handler function
		n.ServeHTTP(response, request)

		So(response.Code, ShouldEqual, http.StatusOK)

		validateVersionInfo(v, response)
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
		// add auth cookie--this can be anything if we are using a MockUserManager
		request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
		n.ServeHTTP(response, request)

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
		n.ServeHTTP(response, request)

		Convey("response should indicate a permission error", func() {
			So(response.Code, ShouldEqual, http.StatusFound)
		})
	})
}

func TestGetVersionStatus(t *testing.T) {

	userManager, err := auth.LoadUserManager(versionTestConfig.AuthConfig)
	testutil.HandleTestingErr(err, t, "Failure in loading UserManager from config")

	uis := UIServer{
		RootURL:     versionTestConfig.Ui.Url,
		Settings:    *versionTestConfig,
		UserManager: userManager,
	}

	home := evergreen.FindEvergreenHome()

	uis.Render = render.New(render.Options{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: true,
	})
	uis.InitPlugins()

	router, err := uis.NewRouter()
	testutil.HandleTestingErr(err, t, "Failure in uis.NewRouter()")

	Convey("When finding the status of a particular version", t, func() {
		testutil.HandleTestingErr(db.Clear(build.Collection), t,
			"Error clearing '%v' collection", build.Collection)

		versionId := "my-version"

		task := build.TaskCache{
			Id:          "some-task-id",
			DisplayName: "some-task-name",
			Status:      "success",
			TimeTaken:   time.Duration(100 * time.Millisecond),
		}
		build := &build.Build{
			Id:           "some-build-id",
			Version:      versionId,
			BuildVariant: "some-build-variant",
			DisplayName:  "Some Build Variant",
			Tasks:        []build.TaskCache{task},
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

func validateVersionInfo(v *version.Version, response *httptest.ResponseRecorder) {
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
		So(jsonBody["repo_kind"], ShouldEqual, v.RepoKind)
		So(jsonBody["identifier"], ShouldEqual, v.Identifier)
		So(jsonBody["remote"], ShouldEqual, v.Remote)
		So(jsonBody["remote_path"], ShouldEqual, v.RemotePath)
		So(jsonBody["requester"], ShouldEqual, v.Requester)
	})
}
