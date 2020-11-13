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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		Private: true,
		Id:      "orchard",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "master",
		Admins:  []string{"johnny.appleseed"},
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
		Private: true,
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "master",
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
		Private: true,
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "master",
		Admins:  []string{"admin"},
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
		Private: true,
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "master",
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
		Private: true,
		Id:      "mci",
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Branch:  "master",
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
	p, err = patch.FindOne(patch.ByUserAndCommitQueue("octocat", false))
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

func TestProjectViewPermission(t *testing.T) {
	//setup
	assert := assert.New(t)
	require := require.New(t)
	counter := 0
	counterFunc := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	env := evergreen.GetEnvironment()
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, model.ProjectRefCollection))
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	role1 := gimlet.Role{
		ID:          "r1",
		Scope:       "proj1",
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(role1))
	defaultRole := gimlet.Role{
		ID:          evergreen.UnauthedUserRoles[0],
		Scope:       "all",
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(defaultRole))
	scope1 := gimlet.Scope{
		ID:        "proj1",
		Resources: []string{"proj1"},
		Type:      "project",
	}
	assert.NoError(env.RoleManager().AddScope(scope1))
	scopeAll := gimlet.Scope{
		ID:        "all",
		Resources: []string{"proj1", "proj2"},
		Type:      "project",
	}
	assert.NoError(env.RoleManager().AddScope(scopeAll))
	proj1 := model.ProjectRef{
		Id:      "proj1",
		Private: true,
	}
	proj2 := model.ProjectRef{
		Id: "proj2",
	}
	assert.NoError(proj1.Insert())
	assert.NoError(proj2.Insert())
	permissionMiddleware := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView)
	checkPermission := func(rw http.ResponseWriter, r *http.Request) {
		permissionMiddleware.ServeHTTP(rw, r, counterFunc)
	}
	authenticator := gimlet.NewBasicAuthenticator(nil, nil)
	opts, err := gimlet.NewBasicUserOptions("user")
	require.NoError(err)
	user := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").RoleManager(env.RoleManager()))
	um, err := gimlet.NewBasicUserManager([]gimlet.BasicUser{*user}, env.RoleManager())
	assert.NoError(err)
	authHandler := gimlet.NewAuthenticationHandler(authenticator, um)
	req := httptest.NewRequest("GET", "http://foo.com/bar", nil)

	// no project should 404
	rw := httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusNotFound, rw.Code)
	assert.Equal(0, counter)

	// public project should return 200 even with no user
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "proj2"})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)

	// private project with no user attached should 404
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "proj1"})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusNotFound, rw.Code)
	assert.Equal(1, counter)

	// attach a user, but with no permissions yet
	ctx := gimlet.AttachUser(req.Context(), user)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(1, counter)

	// give user the right permissions
	opts, err = gimlet.NewBasicUserOptions("user")
	require.NoError(err)
	user = gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").Roles(role1.ID).RoleManager(env.RoleManager()))
	_, err = um.GetOrCreateUser(user)
	assert.NoError(err)
	ctx = gimlet.AttachUser(req.Context(), user)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(2, counter)
}

func TestEventLogPermission(t *testing.T) {
	//setup
	assert := assert.New(t)
	require := require.New(t)
	counter := 0
	counterFunc := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	env := evergreen.GetEnvironment()
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, model.ProjectRefCollection, distro.Collection))
	_ = env.DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	projRole := gimlet.Role{
		ID:          "proj",
		Scope:       "proj1",
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(projRole))
	distroRole := gimlet.Role{
		ID:          "distro",
		Scope:       "distro1",
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(distroRole))
	superuserRole := gimlet.Role{
		ID:          "superuser",
		Scope:       "superuser",
		Permissions: map[string]int{evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(superuserRole))
	scope1 := gimlet.Scope{
		ID:        "proj1",
		Resources: []string{"proj1"},
		Type:      evergreen.ProjectResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(scope1))
	scope2 := gimlet.Scope{
		ID:        "distro1",
		Resources: []string{"distro1"},
		Type:      evergreen.DistroResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(scope2))
	scope3 := gimlet.Scope{
		ID:        "superuser",
		Resources: []string{evergreen.SuperUserPermissionsID},
		Type:      evergreen.SuperUserResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(scope3))
	proj1 := model.ProjectRef{
		Id:      "proj1",
		Private: true,
	}
	assert.NoError(proj1.Insert())
	distro1 := distro.Distro{
		Id: "distro1",
	}
	assert.NoError(distro1.Insert())
	permissionMiddleware := EventLogPermissionsMiddleware{}
	checkPermission := func(rw http.ResponseWriter, r *http.Request) {
		permissionMiddleware.ServeHTTP(rw, r, counterFunc)
	}
	authenticator := gimlet.NewBasicAuthenticator(nil, nil)
	opts, err := gimlet.NewBasicUserOptions("user")
	require.NoError(err)
	user := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").Roles(projRole.ID, distroRole.ID, superuserRole.ID).RoleManager(env.RoleManager()))
	um, err := gimlet.NewBasicUserManager([]gimlet.BasicUser{*user}, env.RoleManager())
	assert.NoError(err)
	authHandler := gimlet.NewAuthenticationHandler(authenticator, um)
	req := httptest.NewRequest("GET", "http://foo.com/bar", nil)

	// no user + private project should 404
	rw := httptest.NewRecorder()
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": model.EventResourceTypeProject, "resource_id": proj1.Id})
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusNotFound, rw.Code)
	assert.Equal(0, counter)

	// have user, project event
	req = req.WithContext(gimlet.AttachUser(req.Context(), user))
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": model.EventResourceTypeProject, "resource_id": proj1.Id})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)

	// distro event
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.ResourceTypeDistro, "resource_id": distro1.Id})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(2, counter)

	// superuser event
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.ResourceTypeAdmin, "resource_id": evergreen.SuperUserPermissionsID})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(3, counter)
}
