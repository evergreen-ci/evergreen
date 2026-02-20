package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PrefetchProjectContext gets the information related to the project that the request contains
// and fetches the associated project context and attaches that to the request context.
func PrefetchProjectContext(ctx context.Context, r *http.Request, input map[string]string) (context.Context, error) {
	r = r.WithContext(ctx)
	if input != nil {
		r = gimlet.SetURLVars(r, input)
	}
	rw := httptest.NewRecorder()
	NewProjectContextMiddleware().ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
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
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, patch.Collection, user.Collection))
	doc := &model.ProjectRef{
		Id: "mci",
	}
	require.NoError(t, doc.Insert(t.Context()))
	patchDoc := &patch.Patch{
		Id: bson.ObjectIdHex("aabbccddeeff112233445566"),
	}
	require.NoError(t, patchDoc.Insert(t.Context()))
	testUser := &user.DBUser{Id: "test_user"}
	require.NoError(t, testUser.Insert(t.Context()))
	Convey("When there is a data and a request", t, func() {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		So(err, ShouldBeNil)
		Convey("When fetching the project context", func() {
			ctx := context.Background()
			Convey("should error if project is private and no user is set", func() {
				ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"project_id": "mci"})
				So(ctx.Value(RequestContext), ShouldBeNil)

				errToResemble := gimlet.ErrorResponse{
					StatusCode: http.StatusNotFound,
					Message:    "not found",
				}
				So(err, ShouldResemble, errToResemble)
			})
			Convey("should error if patch exists and no user is set", func() {
				ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"patch_id": "aabbccddeeff112233445566"})
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
					Id: "mci",
				}
				ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "test_user"})
				ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"project_id": "mci"})
				So(err, ShouldBeNil)

				So(ctx.Value(RequestContext), ShouldResemble, &opCtx)
			})
		})
	})
}

func TestNewProjectAdminMiddleware(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

	opCtx := model.Context{}
	opCtx.ProjectRef = &model.ProjectRef{
		Id:     "orchard",
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Branch: "main",
		Admins: []string{"johnny.appleseed"},
	}
	adminRole := gimlet.Role{
		ID:          "r1",
		Scope:       "orchard",
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(t.Context(), adminRole))
	adminScope := gimlet.Scope{ID: "orchard", Resources: []string{"orchard"}, Type: evergreen.ProjectResourceType}
	assert.NoError(env.RoleManager().AddScope(t.Context(), adminScope))

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "not.admin"})
	r, err := http.NewRequest(http.MethodGet, "/projects/orchard", nil)
	assert.NoError(err)
	assert.NotNil(r)

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	mw := NewProjectAdminMiddleware()
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "johnny.appleseed", SystemRoles: []string{"r1"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	rw = httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestNewCanCreateMiddleware(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(evergreen.RoleCollection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	adminRole := gimlet.Role{
		ID:          "r1",
		Scope:       "anything",
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(t.Context(), adminRole))

	opCtx := model.Context{}

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "not.admin"})
	r, err := http.NewRequest(http.MethodPut, "/projects/makeFromRoute", nil)
	assert.NoError(err)
	assert.NotNil(r)
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	mw := NewCanCreateMiddleware()
	rw := httptest.NewRecorder()

	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "johnny.appleseed", SystemRoles: []string{"r1"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	rw = httptest.NewRecorder()
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)
}

func TestTaskAuthMiddleware(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	assert.NoError(db.ClearCollections(host.Collection, task.Collection))
	task1 := task.Task{
		Id:     "task1",
		Secret: "abcdef",
	}
	completedTask := task.Task{
		Id:     "completedTask",
		Secret: "abcdef",
		Status: evergreen.TaskSucceeded,
	}
	host1 := &host.Host{
		Id:     "host1",
		Secret: "abcdef",
	}
	assert.NoError(task1.Insert(t.Context()))
	assert.NoError(completedTask.Insert(t.Context()))
	assert.NoError(host1.Insert(ctx))
	m := NewTaskAuthMiddleware()
	r := &http.Request{
		Header: http.Header{
			evergreen.HostHeader:       []string{"host1"},
			evergreen.HostSecretHeader: []string{"abcdef"},
			evergreen.TaskHeader:       []string{"task1"},
		},
	}

	rw := httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusConflict, rw.Code)

	r.Header.Set(evergreen.TaskSecretHeader, "ghijkl")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusConflict, rw.Code)

	r.Header.Set(evergreen.TaskSecretHeader, "abcdef")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)

	r.Header.Set(evergreen.TaskHeader, "completedTask")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.NotEqual(http.StatusOK, rw.Code)

	assert.NoError(task.UpdateOne(ctx, bson.M{task.IdKey: "completedTask"}, bson.M{"$set": bson.M{task.FinishTimeKey: time.Now().Add(-30 * time.Minute)}}))
	r.Header.Set(evergreen.TaskHeader, "completedTask")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusOK, rw.Code)

	assert.NoError(task.UpdateOne(ctx, bson.M{task.IdKey: "completedTask"}, bson.M{"$set": bson.M{task.FinishTimeKey: time.Now().Add(-90 * time.Minute)}}))
	r.Header.Set(evergreen.TaskHeader, "completedTask")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.Equal(http.StatusUnauthorized, rw.Code)

}

func TestHostAuthMiddleware(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewHostAuthMiddleware()
	for testName, testCase := range map[string]func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder){
		"Succeeds": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.Equal(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidSecret": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{"foo"},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithoutHostID": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithInvalidHostID": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{"foo"},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
		"FailsWithTerminatedHost": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			assert.NoError(t, h.SetStatus(ctx, evergreen.HostTerminated, "", ""))
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(host.Collection))
			defer func() {
				assert.NoError(t, db.Clear(host.Collection))
			}()
			h := &host.Host{
				Id:     "id",
				Secret: "secret",
			}
			require.NoError(t, h.Insert(ctx))

			testCase(t, h, httptest.NewRecorder())
		})
	}
}

func TestProjectViewPermission(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require := require.New(t)
	counter := 0
	counterFunc := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, model.ProjectRefCollection))
	require.NoError(db.CreateCollections(evergreen.ScopeCollection))
	restrictedRole := gimlet.Role{
		ID:          "restricted_role",
		Scope:       "restricted_scope",
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(t.Context(), restrictedRole))
	unrestrictedRole := gimlet.Role{
		ID:          "default_role",
		Scope:       "unrestricted_scope",
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(t.Context(), unrestrictedRole))
	restrictedScope := gimlet.Scope{
		ID:        "restricted_scope",
		Resources: []string{"restrictedProject"},
		Type:      "project",
	}
	assert.NoError(env.RoleManager().AddScope(t.Context(), restrictedScope))
	unrestrictedScope := gimlet.Scope{
		ID:        "unrestricted_scope",
		Resources: []string{"unrestrictedProject"},
		Type:      "project",
	}
	assert.NoError(env.RoleManager().AddScope(t.Context(), unrestrictedScope))
	restrictedProject := model.ProjectRef{
		Id: "restrictedProject",
	}
	unrestrictedProject := model.ProjectRef{
		Id: "unrestrictedProject",
	}
	assert.NoError(restrictedProject.Insert(t.Context()))
	assert.NoError(unrestrictedProject.Insert(t.Context()))
	permissionMiddleware := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView)
	checkPermission := func(rw http.ResponseWriter, r *http.Request) {
		permissionMiddleware.ServeHTTP(rw, r, counterFunc)
	}
	authenticator := gimlet.NewBasicAuthenticator(nil, nil)
	opts, err := gimlet.NewBasicUserOptions("user")
	require.NoError(err)

	um, err := gimlet.NewBasicUserManager([]gimlet.BasicUser{}, env.RoleManager())
	assert.NoError(err)
	authHandler := gimlet.NewAuthenticationHandler(authenticator, um)
	req := httptest.NewRequest(http.MethodGet, "http://foo.com/bar", nil)

	// no project should 404
	rw := httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusNotFound, rw.Code)
	assert.Equal(0, counter)

	// project with no user attached should 401
	req = gimlet.SetURLVars(req, map[string]string{"project_id": "restrictedProject"})
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// attach a user, but with no permissions yet
	usr := gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").RoleManager(env.RoleManager()))
	ctx = gimlet.AttachUser(req.Context(), usr)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// giving user permissions to unrestrictedProjects only should fail
	opts, err = gimlet.NewBasicUserOptions("user")
	require.NoError(err)
	usr = gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").
		Roles(unrestrictedRole.ID).RoleManager(env.RoleManager()))
	_, err = um.GetOrCreateUser(t.Context(), usr)
	assert.NoError(err)
	ctx = gimlet.AttachUser(req.Context(), usr)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)

	// give user permissions to both projects
	usr = gimlet.NewBasicUser(opts.Name("name").Email("email").Password("password").Key("key").
		Roles(unrestrictedRole.ID, restrictedRole.ID).RoleManager(env.RoleManager()))
	_, err = um.GetOrCreateUser(t.Context(), usr)
	assert.NoError(err)
	ctx = gimlet.AttachUser(req.Context(), usr)
	req = req.WithContext(ctx)
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusOK, rw.Code)
	assert.Equal(1, counter)
}

func TestEventLogPermission(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	require := require.New(t)
	counter := 0
	counterFunc := func(rw http.ResponseWriter, r *http.Request) {
		counter++
		rw.WriteHeader(http.StatusOK)
	}
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, model.ProjectRefCollection, distro.Collection))
	require.NoError(db.CreateCollections(evergreen.ScopeCollection))
	projRole := gimlet.Role{
		ID:          "proj",
		Scope:       "proj1",
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(t.Context(), projRole))
	distroRole := gimlet.Role{
		ID:          "distro",
		Scope:       "distro1",
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsView.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(t.Context(), distroRole))
	superuserRole := gimlet.Role{
		ID:          "superuser",
		Scope:       "superuser",
		Permissions: map[string]int{evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value},
	}
	assert.NoError(env.RoleManager().UpdateRole(t.Context(), superuserRole))
	scope1 := gimlet.Scope{
		ID:        "proj1",
		Resources: []string{"proj1"},
		Type:      evergreen.ProjectResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(t.Context(), scope1))
	scope2 := gimlet.Scope{
		ID:        "distro1",
		Resources: []string{"distro1"},
		Type:      evergreen.DistroResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(t.Context(), scope2))
	scope3 := gimlet.Scope{
		ID:        "superuser",
		Resources: []string{evergreen.SuperUserPermissionsID},
		Type:      evergreen.SuperUserResourceType,
	}
	assert.NoError(env.RoleManager().AddScope(t.Context(), scope3))
	proj1 := model.ProjectRef{
		Id: "proj1",
	}
	assert.NoError(proj1.Insert(t.Context()))
	distro1 := distro.Distro{
		Id: "distro1",
	}
	assert.NoError(distro1.Insert(ctx))
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
	req := httptest.NewRequest(http.MethodGet, "http://foo.com/bar", nil)

	// no user should 401
	rw := httptest.NewRecorder()
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.EventResourceTypeProject, "resource_id": proj1.Id})
	authHandler.ServeHTTP(rw, req, checkPermission)
	assert.Equal(http.StatusUnauthorized, rw.Code)
	assert.Equal(0, counter)

	// have user, project event
	req = req.WithContext(gimlet.AttachUser(req.Context(), user))
	req = gimlet.SetURLVars(req, map[string]string{"resource_type": event.EventResourceTypeProject, "resource_id": proj1.Id})
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
