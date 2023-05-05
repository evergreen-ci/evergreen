package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestProjectRoutes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	env.SetUserManager(serviceutil.MockUserManager{})
	router, err := newAuthTestUIRouter(ctx, env)
	require.NoError(t, err, "error setting up router")

	Convey("When loading a public project, it should be found", t, func() {
		require.NoError(t, db.Clear(model.ProjectRefCollection), "Error clearing '%v' collection", model.ProjectRefCollection)

		publicId := "pub"
		public := &model.ProjectRef{
			Id:      publicId,
			Enabled: true,
			Repo:    "repo1",
			Admins:  []string{},
		}
		So(public.Insert(), ShouldBeNil)

		url := "/rest/v1/projects/" + publicId

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))

		response := httptest.NewRecorder()

		Convey("by a public user", func() {
			router.ServeHTTP(response, request)
			outRef := &model.ProjectRef{}
			So(response.Code, ShouldEqual, http.StatusOK)
			So(json.Unmarshal(response.Body.Bytes(), outRef), ShouldBeNil)
			So(outRef, ShouldResemble, public)
		})
		Convey("and a logged-in user", func() {
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			router.ServeHTTP(response, request)
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
			request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))
			router.ServeHTTP(response, request)
			out := struct {
				Projects []string `json:"projects"`
			}{}
			So(response.Code, ShouldEqual, http.StatusOK)
			So(json.Unmarshal(response.Body.Bytes(), &out), ShouldBeNil)
			So(len(out.Projects), ShouldEqual, 1)
			So(out.Projects[0], ShouldEqual, public.Id)
		})
	})

	Convey("When loading a private project", t, func() {
		require.NoError(t, db.Clear(model.ProjectRefCollection), "Error clearing '%v' collection", model.ProjectRefCollection)

		privateId := "priv"
		private := &model.ProjectRef{
			Id:      privateId,
			Enabled: true,
			Private: utility.TruePtr(),
			Repo:    "repo1",
			Admins:  []string{"testuser"},
		}
		So(private.Insert(), ShouldBeNil)
		response := httptest.NewRecorder()

		Convey("users who are not logged in should be denied with a 401", func() {
			url := "/rest/v1/projects/" + privateId

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			router.ServeHTTP(response, request)

			So(response.Code, ShouldEqual, http.StatusUnauthorized)
		})

		Convey("users who are logged in should be able to access the project", func() {
			url := "/rest/v1/projects/" + privateId
			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))
			// add auth cookie--this can be anything if we are using a MockUserManager
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
			router.ServeHTTP(response, request)

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
				request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))
				request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
				router.ServeHTTP(response, request)
				out := struct {
					Projects []string `json:"projects"`
				}{}
				So(response.Code, ShouldEqual, http.StatusOK)
				So(json.Unmarshal(response.Body.Bytes(), &out), ShouldBeNil)
				So(len(out.Projects), ShouldEqual, 1)
				So(out.Projects[0], ShouldEqual, private.Id)
			})
			Convey("but not public users", func() {
				router.ServeHTTP(response, request)
				So(response.Code, ShouldEqual, http.StatusUnauthorized)
			})
		})
	})

	Convey("When finding info on a nonexistent project", t, func() {
		url := "/rest/v1/projects/nope"

		request, err := http.NewRequest("GET", url, nil)
		So(err, ShouldBeNil)
		request = request.WithContext(gimlet.AttachUser(request.Context(), &user.DBUser{Id: "user"}))
		response := httptest.NewRecorder()

		Convey("response should contain a sensible error message", func() {
			Convey("for a public user", func() {
				router.ServeHTTP(response, request)
				So(response.Code, ShouldEqual, http.StatusNotFound)
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)
				So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
			})
			Convey("and a logged-in user", func() {
				request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
				router.ServeHTTP(response, request)
				So(response.Code, ShouldEqual, http.StatusNotFound)
				var jsonBody map[string]interface{}
				err = json.Unmarshal(response.Body.Bytes(), &jsonBody)
				So(err, ShouldBeNil)
				So(len(jsonBody["message"].(string)), ShouldBeGreaterThan, 0)
			})
		})
	})
}
