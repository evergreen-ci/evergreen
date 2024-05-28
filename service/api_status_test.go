package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func init() { testutil.Setup() }

// getEndPoint is a helper that creates a test API server,
// GETs the passed int endpoint, and returns
// the response.
func getEndPoint(url string, t *testing.T) *httptest.ResponseRecorder {
	env := evergreen.GetEnvironment()
	queue := env.LocalQueue()

	as, err := NewAPIServer(env, queue)
	require.NoError(t, err)
	app := as.GetServiceApp()
	app.AddMiddleware(gimlet.NewAuthenticationHandler(gimlet.NewBasicAuthenticator(nil, nil), env.UserManager()))
	handler, err := app.Handler()
	require.NoError(t, err)
	request, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	ctx := gimlet.AttachUser(request.Context(), &user.DBUser{Id: "octocat"})
	request = request.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
}

func TestServiceStatusEndPoints(t *testing.T) {
	Convey("Service Status endpoints should report the status of the service", t, func() {
		Convey("basic endpoint should have one key, that reports the build id", func() {
			resp := getEndPoint("/api/status/info", t)
			So(resp, ShouldNotBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)

			out := map[string]string{}

			So(json.NewDecoder(resp.Body).Decode(&out), ShouldBeNil)
			So(len(out), ShouldEqual, 2)
			_, ok := out["build_revision"]
			So(ok, ShouldBeTrue)
		})
	})
}
