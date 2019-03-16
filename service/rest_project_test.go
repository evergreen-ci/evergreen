package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	. "github.com/smartystreets/goconvey/convey"
)

var projectTestConfig = testutil.TestConfig()

func TestProjectRoutes(t *testing.T) {
	uis := UIServer{
		RootURL:     projectTestConfig.Ui.Url,
		Settings:    *projectTestConfig,
		UserManager: serviceutil.MockUserManager{},
	}
	home := evergreen.FindEvergreenHome()
	uis.render = gimlet.NewHTMLRenderer(gimlet.RendererOptions{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: true,
	})

	app := GetRESTv1App(&uis)
	app.AddMiddleware(gimlet.UserMiddleware(uis.UserManager, gimlet.UserMiddlewareConfiguration{
		CookieName:     evergreen.AuthTokenCookie,
		HeaderKeyName:  evergreen.APIKeyHeader,
		HeaderUserName: evergreen.APIUserHeader,
	}))
	n, err := app.Handler()
	testutil.HandleTestingErr(err, t, "error setting up router")

	Convey("When loading a public project, it should be found", t, func() {
		testutil.HandleTestingErr(db.Clear(model.ProjectRefCollection), t,
			"Error clearing '%v' collection", model.ProjectRefCollection)

		publicId := "pub"
		public := &model.ProjectRef{
			Identifier:  publicId,
			Enabled:     true,
			Repo:        "repo1",
			LocalConfig: "buildvariants:\n - name: ubuntu",
			Admins:      []string{},
		}
		So(public.Insert(), ShouldBeNil)

		url := "/rest/v1/projects/" + publicId

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)

		response := httptest.NewRecorder()

		Convey("by a public user", func() {
			n.ServeHTTP(response, request)
			outRef := &model.ProjectRef{}
			So(response.Code, ShouldEqual, http.StatusOK)
			So(json.Unmarshal(response.Body.Bytes(), outRef), ShouldBeNil)
			So(outRef, ShouldResemble, public)
		})
		Convey("and a logged-in user", func() {
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			n.ServeHTTP(response, request)
			outRef := &model.ProjectRef{}
			So(response.Code, ShouldEqual, http.StatusOK)
			So(json.Unmarshal(response.Body.Bytes(), outRef), ShouldBeNil)
			So(outRef, ShouldResemble, public)
		})
		Convey("and be visible to the project_list route", func() {
			url := "/rest/v1/projects"

			So(err, ShouldBeNil)
			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			n.ServeHTTP(response, request)
			out := struct {
				Projects []string `json:"projects"`
			}{}
			So(response.Code, ShouldEqual, http.StatusOK)
			So(json.Unmarshal(response.Body.Bytes(), &out), ShouldBeNil)
			So(len(out.Projects), ShouldEqual, 1)
			So(out.Projects[0], ShouldEqual, public.Identifier)
		})
	})

	Convey("When loading a private project", t, func() {
		testutil.HandleTestingErr(db.Clear(model.ProjectRefCollection), t,
			"Error clearing '%v' collection", model.ProjectRefCollection)

		privateId := "priv"
		private := &model.ProjectRef{
			Identifier: privateId,
			Enabled:    true,
			Private:    true,
			Repo:       "repo1",
			Admins:     []string{"testuser"},
		}
		So(private.Insert(), ShouldBeNil)
		response := httptest.NewRecorder()

		Convey("users who are not logged in should be denied with a 401", func() {
			url := "/rest/v1/projects/" + privateId

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			n.ServeHTTP(response, request)

			So(response.Code, ShouldEqual, http.StatusUnauthorized)
		})

		Convey("users who are logged in should be able to access the project", func() {
			url := "/rest/v1/projects/" + privateId
			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			// add auth cookie--this can be anything if we are using a MockUserManager
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			n.ServeHTTP(response, request)

			outRef := &model.ProjectRef{}
			So(response.Code, ShouldEqual, http.StatusOK)
			So(json.Unmarshal(response.Body.Bytes(), outRef), ShouldBeNil)
			So(outRef, ShouldResemble, private)
		})
		Convey("and it should be visible to the project_list route", func() {
			url := "/rest/v1/projects"

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			Convey("for credentialed users", func() {
				request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
				n.ServeHTTP(response, request)
				out := struct {
					Projects []string `json:"projects"`
				}{}
				So(response.Code, ShouldEqual, http.StatusOK)
				So(json.Unmarshal(response.Body.Bytes(), &out), ShouldBeNil)
				So(len(out.Projects), ShouldEqual, 1)
				So(out.Projects[0], ShouldEqual, private.Identifier)
			})
			Convey("but not public users", func() {
				n.ServeHTTP(response, request)
				out := struct {
					Projects []string `json:"projects"`
				}{}
				So(response.Code, ShouldEqual, http.StatusOK)
				So(json.Unmarshal(response.Body.Bytes(), &out), ShouldBeNil)
				So(len(out.Projects), ShouldEqual, 0)
			})
		})
	})

	Convey("When finding info on a nonexistent project", t, func() {
		url := "/rest/v1/projects/nope"

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		response := httptest.NewRecorder()

		Convey("response should contain a sensible error message", func() {
			Convey("for a public user", func() {
				n.ServeHTTP(response, request)
				So(response.Code, ShouldEqual, http.StatusNotFound)
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)
				So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
			})
			Convey("and a logged-in user", func() {
				request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
				n.ServeHTTP(response, request)
				So(response.Code, ShouldEqual, http.StatusNotFound)
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)
				So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
			})
		})
	})
}
