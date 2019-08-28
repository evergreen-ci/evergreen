package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
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

func TestNewProjectAdminMiddleware(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private:    true,
		Identifier: "orchard",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
		Admins:     []string{"johnny.appleseed"},
	}

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "not.admin"})
	r, err := http.NewRequest("GET", "/projects/orchard", nil)
	assert.NoError(err)
	assert.NotNil(r)

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	mockConnector := &data.MockConnector{}
	mw := NewProjectAdminMiddleware(mockConnector)
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "johnny.appleseed"})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	rw = httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewarePROwner(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private:    true,
		Identifier: "mci",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
		CommitQueue: model.CommitQueueParams{
			Enabled: true,
		},
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID: 1234,
			},
		},
	})

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       "1234",
	})

	mockDataConnector := &data.MockConnector{}
	mw := NewCommitQueueItemOwnerMiddleware(mockDataConnector)
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewareProjectAdmin(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private:    true,
		Identifier: "mci",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
		Admins:     []string{"admin"},
		CommitQueue: model.CommitQueueParams{
			Enabled: true,
		},
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID: 1234,
			},
		},
	})

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Id: "admin",
	})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       "1234",
	})

	mockDataConnector := &data.MockConnector{}
	mw := NewCommitQueueItemOwnerMiddleware(mockDataConnector)
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewareUnauthorizedUserGitHub(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private:    true,
		Identifier: "mci",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
		CommitQueue: model.CommitQueueParams{
			PatchType: commitqueue.PRPatchType,
			Enabled:   true,
		},
	}

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID: 4321,
			},
		},
	})

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       "1234",
	})

	mockDataConnector := &data.MockConnector{}
	mw := NewCommitQueueItemOwnerMiddleware(mockDataConnector)
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)
}

func TestCommitQueueItemOwnerMiddlewareUserPatch(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(patch.Collection))

	ctx := context.Background()
	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Private:    true,
		Identifier: "mci",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "master",
		CommitQueue: model.CommitQueueParams{
			PatchType: commitqueue.CLIPatchType,
			Enabled:   true,
		},
	}

	r, err := http.NewRequest(http.MethodDelete, "/", nil)
	assert.NoError(err)
	assert.NotNil(r)

	p := &patch.Patch{
		Author: "octocat",
	}
	assert.NoError(p.Insert())
	p, err = patch.FindOne(patch.ByUser("octocat"))
	assert.NoError(err)
	assert.NotNil(p)

	// not authorized
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "me"})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       p.Id.Hex(),
	})

	dataConnector := &data.DBConnector{}
	mw := NewCommitQueueItemOwnerMiddleware(dataConnector)

	rw := httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	// authorized
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "octocat"})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	r = gimlet.SetURLVars(r, map[string]string{
		"project_id": "mci",
		"item":       p.Id.Hex(),
	})
	rw = httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestTaskAuthMiddleware(t *testing.T) {
	assert := assert.New(t)
	m := NewTaskAuthMiddleware(&data.MockConnector{})
	r := &http.Request{
		Header: http.Header{
			evergreen.HostHeader:       []string{"host1"},
			evergreen.HostSecretHeader: []string{"abcdef"},
			evergreen.TaskHeader:       []string{"task1"},
		},
	}

	rw := httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	r.Header.Set(evergreen.TaskSecretHeader, "abcdef")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}
