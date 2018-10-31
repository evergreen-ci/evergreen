package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

// PrefetchProjectContext gets the information related to the project that the request contains
// and fetches the associated project context and attaches that to the request context.
func PrefetchProjectContext(ctx context.Context, sc data.Connector, r *http.Request) (context.Context, error) {
	r = r.WithContext(ctx)

	rw := httptest.NewRecorder()
	NewProjectContextMiddleware(sc).ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	})

	if rw.Code != http.StatusOK {
		return ctx, gimlet.ErrorResponse{
			StatusCode: rw.Code,
			Message:    "not found",
		}
	}

	return ctx, nil
}

func TestPrefetchProject(t *testing.T) {
	Convey("When there is a data and a request", t, func() {
		serviceContext := &data.MockConnector{}
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		Convey("When fetching the project context", func() {
			ctx := context.Background()
			Convey("should error if project is private and no user is set", func() {
				opCtx := model.Context{}
				opCtx.ProjectRef = &model.ProjectRef{
					Private: true,
				}
				serviceContext.MockContextConnector.CachedContext = opCtx
				ctx, err = PrefetchProjectContext(ctx, serviceContext, req)
				So(ctx.Value(RequestContext), ShouldBeNil)

				errToResemble := gimlet.ErrorResponse{
					StatusCode: http.StatusNotFound,
					Message:    "not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should error if patch exists and no user is set", func() {
				opCtx := model.Context{}
				opCtx.Patch = &patch.Patch{}
				serviceContext.MockContextConnector.CachedContext = opCtx
				ctx, err = PrefetchProjectContext(ctx, serviceContext, req)
				So(ctx.Value(RequestContext), ShouldBeNil)

				errToResemble := gimlet.ErrorResponse{
					StatusCode: http.StatusNotFound,
					Message:    "not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should succeed if project ref exists and user is set", func() {
				opCtx := model.Context{}
				opCtx.ProjectRef = &model.ProjectRef{
					Private: true,
				}
				ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})
				serviceContext.MockContextConnector.CachedContext = opCtx
				ctx, err = PrefetchProjectContext(ctx, serviceContext, req)
				So(err, ShouldBeNil)

				So(ctx.Value(RequestContext), ShouldResemble, &opCtx)
			})
		})
	})
}

var noopHandlerFunc = func(rw http.ResponseWriter, r *http.Request) {}

func TestTaskMiddleware(t *testing.T) {
	assert := assert.New(t)
	rw := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "foo", nil)
	assert.NoError(err)
	m := RequireTaskContext()

	// since we can't mock mux.vars, we can only test an empty task
	m.ServeHTTP(rw, req, noopHandlerFunc)
	assert.Equal(http.StatusBadRequest, rw.Code)
	assert.Contains(string(rw.Body.Bytes()), "missing task id")
}

func TestHostMiddleware(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(host.Collection))
	rw := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "foo", nil)
	assert.NoError(err)
	m := RequireHostContext()

	// no host
	m.ServeHTTP(rw, req, noopHandlerFunc)
	assert.Equal(http.StatusBadRequest, rw.Code)
	assert.Contains(string(rw.Body.Bytes()), "is missing host information")

	// nonexistent host
	rw = httptest.NewRecorder()
	req.Header.Set(evergreen.HostHeader, "h")
	m.ServeHTTP(rw, req, noopHandlerFunc)
	assert.Equal(http.StatusBadRequest, rw.Code)
	assert.Contains(string(rw.Body.Bytes()), "Host h not found")

	// invalid secret
	h := host.Host{
		Id:     "h",
		Secret: "123",
	}
	assert.NoError(h.Insert())
	rw = httptest.NewRecorder()
	req.Header.Set(evergreen.HostHeader, "h")
	req.Header.Set(evergreen.HostSecretHeader, "321")
	m.ServeHTTP(rw, req, noopHandlerFunc)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Contains(string(rw.Body.Bytes()), "Invalid host secret")

	// correct secret
	rw = httptest.NewRecorder()
	req.Header.Set(evergreen.HostHeader, "h")
	req.Header.Set(evergreen.HostSecretHeader, "123")
	m.ServeHTTP(rw, req, noopHandlerFunc)
	assert.Equal(http.StatusOK, rw.Code)
	grip.Info(rw.Body.Bytes())
}
