package route

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mongobson "go.mongodb.org/mongo-driver/bson"
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
	collections := []string{
		model.ProjectRefCollection,
		model.RepoRefCollection,
		patch.Collection,
		user.Collection,
	}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(collections...))
	})

	projectRef := model.ProjectRef{Id: "mci"}
	require.NoError(t, projectRef.Insert(t.Context()))

	patch := patch.Patch{Id: bson.ObjectIdHex("aabbccddeeff112233445566")}
	require.NoError(t, patch.Insert(t.Context()))

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:    "my-repo",
		Owner: "evergreen-ci",
		Repo:  "evergreen",
	}}
	require.NoError(t, repoRef.Replace(t.Context()))

	testUser := &user.DBUser{Id: "test_user"}
	require.NoError(t, testUser.Insert(t.Context()))

	t.Run("ProjectWithNoUserShouldError", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		ctx, err := PrefetchProjectContext(t.Context(), req, map[string]string{"project_id": "mci"})
		assert.Equal(t, gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: "not found"}, err)
		assert.Nil(t, ctx.Value(RequestContext))
	})

	t.Run("PatchWithNoUserShouldError", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		ctx, err := PrefetchProjectContext(t.Context(), req, map[string]string{"patch_id": "aabbccddeeff112233445566"})
		assert.Equal(t, gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: "not found"}, err)
		assert.Nil(t, ctx.Value(RequestContext))
	})

	t.Run("ProjectWithUserShouldSucceed", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		ctx := gimlet.AttachUser(t.Context(), testUser)
		ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"project_id": "mci"})
		require.NoError(t, err)

		opCtx, ok := ctx.Value(RequestContext).(*model.Context)
		require.True(t, ok)
		require.NotNil(t, opCtx.ProjectRef)
		assert.Equal(t, "mci", opCtx.ProjectRef.Id)
	})

	t.Run("RepoIDPopulatesRepoRefInContext", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		ctx := gimlet.AttachUser(t.Context(), testUser)
		ctx, err = PrefetchProjectContext(ctx, req, map[string]string{"repo_id": "my-repo"})
		require.NoError(t, err)

		opCtx, ok := ctx.Value(RequestContext).(*model.Context)
		require.True(t, ok)
		require.NotNil(t, opCtx.RepoRef)
		assert.Equal(t, "my-repo", opCtx.RepoRef.Id)
	})
}

func TestNewProjectAdminMiddleware(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection))
	ctx := t.Context()
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
	ctx := t.Context()
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

func TestNotificationSendMiddleware(t *testing.T) {
	assert.NoError(t, db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection))

	adminRole := gimlet.Role{
		ID:          "notification_send",
		Scope:       "superuser_scope",
		Permissions: map[string]int{evergreen.PermissionNotificationsSend: evergreen.NotificationsSend.Value},
	}
	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{evergreen.SuperUserPermissionsID},
	}
	require.NoError(t, evergreen.GetEnvironment().RoleManager().UpdateRole(t.Context(), adminRole))
	require.NoError(t, evergreen.GetEnvironment().RoleManager().AddScope(t.Context(), superUserScope))

	// Create a middleware that requires the notifications send permission.
	permission := RequiresSuperUserPermission(evergreen.PermissionNotificationsSend, evergreen.NotificationsSend)
	checkPermission := func(rw http.ResponseWriter, r *http.Request) {
		permission.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
	}
	opCtx := model.Context{}
	um, err := gimlet.NewBasicUserManager([]gimlet.BasicUser{}, evergreen.GetEnvironment().RoleManager())
	assert.NoError(t, err)
	authenticator := gimlet.NewBasicAuthenticator(nil, nil)
	authHandler := gimlet.NewAuthenticationHandler(authenticator, um)

	// Check that a regular user can't use the route.
	r, err := http.NewRequest(http.MethodPut, "/notifications/email", nil)
	assert.NoError(t, err)
	assert.NotNil(t, r)
	ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "regular.user"})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	rw := httptest.NewRecorder()
	authHandler.ServeHTTP(rw, r, checkPermission)
	assert.Equal(t, http.StatusUnauthorized, rw.Code)

	// Check that an authenticated user can use the route.
	ctx = gimlet.AttachUser(t.Context(), &user.DBUser{Id: "notification.user", SystemRoles: []string{"notification_send"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	rw = httptest.NewRecorder()
	authHandler.ServeHTTP(rw, r, checkPermission)
	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestSendNotificationMiddleware(t *testing.T) {
	require.NoError(t, db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection))

	adminRole := gimlet.Role{
		ID:          "notification_send",
		Scope:       "superuser_scope",
		Permissions: map[string]int{evergreen.PermissionNotificationsSend: evergreen.NotificationsSend.Value},
	}
	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{evergreen.SuperUserPermissionsID},
	}
	require.NoError(t, evergreen.GetEnvironment().RoleManager().UpdateRole(t.Context(), adminRole))
	require.NoError(t, evergreen.GetEnvironment().RoleManager().AddScope(t.Context(), superUserScope))

	mw := NewSendNotificationMiddleware()
	opCtx := model.Context{}

	makeRequest := func(target string) *http.Request {
		body := strings.NewReader(`{"target":"` + target + `"}`)
		r, err := http.NewRequest(http.MethodPost, "/notifications/slack", body)
		require.NoError(t, err)
		return r
	}

	serveMiddleware := func(rw http.ResponseWriter, r *http.Request) {
		mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
	}

	// Superuser with notification_send role can send to any target.
	r := makeRequest("@other.user")
	ctx := gimlet.AttachUser(t.Context(), &user.DBUser{Id: "superuser", SystemRoles: []string{"notification_send"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	rw := httptest.NewRecorder()
	serveMiddleware(rw, r)
	assert.Equal(t, http.StatusOK, rw.Code)

	// Regular user sending to themselves (target matches their Slack username).
	r = makeRequest("@myslack")
	ctx = gimlet.AttachUser(t.Context(), &user.DBUser{Id: "self.user", Settings: user.UserSettings{SlackUsername: "myslack"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	rw = httptest.NewRecorder()
	serveMiddleware(rw, r)
	assert.Equal(t, http.StatusOK, rw.Code)

	// Regular user sending to a different Slack username gets 401.
	r = makeRequest("@someone.else")
	ctx = gimlet.AttachUser(t.Context(), &user.DBUser{Id: "other.user", Settings: user.UserSettings{SlackUsername: "myslack"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	rw = httptest.NewRecorder()
	serveMiddleware(rw, r)
	assert.Equal(t, http.StatusUnauthorized, rw.Code)

	// Regular user with no Slack username set gets 401 (check enabled).
	r = makeRequest("@myslack")
	ctx = gimlet.AttachUser(t.Context(), &user.DBUser{Id: "no.slack.user"})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	rw = httptest.NewRecorder()
	serveMiddleware(rw, r)
	assert.Equal(t, http.StatusUnauthorized, rw.Code)

	// A correctly-formatted email body on the email route returns 401 for a regular user.
	body := strings.NewReader(`{"recipients":["someone@example.com"],"subject":"hi"}`)
	r, err := http.NewRequest(http.MethodPost, "/notifications/email", body)
	require.NoError(t, err)
	ctx = gimlet.AttachUser(t.Context(), &user.DBUser{Id: "self.user", Settings: user.UserSettings{SlackUsername: "myslack"}})
	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))
	rw = httptest.NewRecorder()
	serveMiddleware(rw, r)
	assert.Equal(t, http.StatusUnauthorized, rw.Code)
}

func TestSNSAuthMiddlewareCapsBodySize(t *testing.T) {
	mw := NewSNSAuthMiddleware()

	oversized := strings.NewReader(strings.Repeat("a", maxWebhookBodySize+1))
	r, err := http.NewRequest(http.MethodPost, "/hooks/aws", oversized)
	require.NoError(t, err)
	rw := httptest.NewRecorder()
	called := false
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
		called = true
	})
	assert.False(t, called, "route handler should not be called when huge webhook request body is sent")
	assert.NotEqual(t, http.StatusOK, rw.Code)
}

func TestGithubAuthMiddlewareCapsBodySize(t *testing.T) {
	mw := NewGithubAuthMiddleware()

	oversized := strings.NewReader(strings.Repeat("a", maxWebhookBodySize+1))
	r, err := http.NewRequest(http.MethodPost, "/hooks/github", oversized)
	require.NoError(t, err)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rw := httptest.NewRecorder()
	called := false
	mw.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
		called = true
	})
	assert.False(t, called, "route handler should not be called when huge webhook request body is sent")
	assert.NotEqual(t, http.StatusOK, rw.Code)
}

func TestTaskAuthMiddleware(t *testing.T) {
	ctx := t.Context()

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
		Id:          "host1",
		Secret:      "abcdef",
		RunningTask: "task1",
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
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
		// Verify that the task and host are stored in the request context.
		foundTask := GetTask(r.Context())
		assert.NotNil(foundTask)
		assert.Equal("task1", foundTask.Id)
		foundHost := GetHost(r.Context())
		assert.NotNil(foundHost)
		assert.Equal("host1", foundHost.Id)
	})
	assert.Equal(http.StatusOK, rw.Code)

	r.Header.Set(evergreen.TaskHeader, "completedTask")
	rw = httptest.NewRecorder()
	m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
	assert.NotEqual(http.StatusOK, rw.Code)

	assert.NoError(task.UpdateOne(ctx, bson.M{task.IdKey: "completedTask"}, bson.M{"$set": bson.M{task.FinishTimeKey: time.Now().Add(-30 * time.Minute)}}))
	assert.NoError(host.UpdateOne(ctx, mongobson.M{host.IdKey: "host1"}, mongobson.M{"$set": mongobson.M{host.RunningTaskKey: "completedTask"}}))
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
	ctx := t.Context()

	m := NewHostAuthMiddleware()
	for testName, testCase := range map[string]func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder){
		"Succeeds": func(t *testing.T, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
				// Verify that the host is stored in the request context.
				foundHost := GetHost(r.Context())
				assert.NotNil(t, foundHost)
				assert.Equal(t, h.Id, foundHost.Id)
			})
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

func TestReadOnlyHostAuthMiddleware(t *testing.T) {
	m := NewReadOnlyHostAuthMiddleware()
	for testName, testCase := range map[string]func(t *testing.T, ctx context.Context, h *host.Host, rw *httptest.ResponseRecorder){
		"SucceedsWithoutWritingCommunicationTime": func(t *testing.T, ctx context.Context, h *host.Host, rw *httptest.ResponseRecorder) {
			stale := time.Now().Add(-time.Hour)
			require.NoError(t, host.UpdateOne(ctx, mongobson.M{host.IdKey: h.Id}, mongobson.M{"$set": mongobson.M{host.LastCommunicationTimeKey: stale}}))

			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{h.Secret},
				},
			}
			called := false
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
				called = true
				foundHost := GetHost(r.Context())
				require.NotNil(t, foundHost)
				assert.Equal(t, h.Id, foundHost.Id)
			})
			assert.Equal(t, http.StatusOK, rw.Code)
			assert.True(t, called)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.WithinDuration(t, stale, dbHost.LastCommunicationTime, time.Millisecond)
		},
		"FailsWithInvalidSecret": func(t *testing.T, ctx context.Context, h *host.Host, rw *httptest.ResponseRecorder) {
			r := &http.Request{
				Header: http.Header{
					evergreen.HostHeader:       []string{h.Id},
					evergreen.HostSecretHeader: []string{"foo"},
				},
			}
			m.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {})
			assert.NotEqual(t, http.StatusOK, rw.Code)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx := t.Context()
			require.NoError(t, db.Clear(host.Collection))
			t.Cleanup(func() {
				assert.NoError(t, db.Clear(host.Collection))
			})
			h := &host.Host{
				Id:     "id",
				Secret: "secret",
			}
			require.NoError(t, h.Insert(ctx))

			testCase(t, ctx, h, httptest.NewRecorder())
		})
	}
}

func TestUpdateHostAccessTime(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, ctx context.Context, h *host.Host){
		"StaleCommunicationTimeWritesUpdate": func(t *testing.T, ctx context.Context, h *host.Host) {
			stale := time.Now().Add(-time.Hour)
			require.NoError(t, host.UpdateOne(ctx, mongobson.M{host.IdKey: h.Id}, mongobson.M{"$set": mongobson.M{host.LastCommunicationTimeKey: stale}}))
			h.LastCommunicationTime = stale

			updateHostAccessTime(ctx, h)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.True(t, dbHost.LastCommunicationTime.After(stale))
		},
		"RecentCommunicationTimeSkipsWrite": func(t *testing.T, ctx context.Context, h *host.Host) {
			recent := time.Now().Add(-time.Second)
			require.NoError(t, host.UpdateOne(ctx, mongobson.M{host.IdKey: h.Id}, mongobson.M{"$set": mongobson.M{host.LastCommunicationTimeKey: recent}}))
			h.LastCommunicationTime = recent

			updateHostAccessTime(ctx, h)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.WithinDuration(t, recent, dbHost.LastCommunicationTime, time.Millisecond)
		},
		"CommunicationTimeWithinIntervalButOverThirtySecondsSkipsWrite": func(t *testing.T, ctx context.Context, h *host.Host) {
			// Pins the interval above the agent's 30s heartbeat.
			require.Greater(t, hostCommunicationWriteInterval, 45*time.Second)
			recent := time.Now().Add(-45 * time.Second)
			require.NoError(t, host.UpdateOne(ctx, mongobson.M{host.IdKey: h.Id}, mongobson.M{"$set": mongobson.M{host.LastCommunicationTimeKey: recent}}))
			h.LastCommunicationTime = recent

			updateHostAccessTime(ctx, h)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.WithinDuration(t, recent, dbHost.LastCommunicationTime, time.Millisecond)
		},
		"RecentCommunicationTimeStillClearsAgentFlags": func(t *testing.T, ctx context.Context, h *host.Host) {
			recent := time.Now().Add(-time.Second)
			require.NoError(t, host.UpdateOne(ctx, mongobson.M{host.IdKey: h.Id}, mongobson.M{"$set": mongobson.M{
				host.LastCommunicationTimeKey: recent,
				host.NeedsNewAgentKey:         true,
				host.NeedsNewAgentMonitorKey:  true,
			}}))
			h.LastCommunicationTime = recent
			h.NeedsNewAgent = true
			h.NeedsNewAgentMonitor = true

			updateHostAccessTime(ctx, h)

			dbHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.False(t, dbHost.NeedsNewAgent)
			assert.False(t, dbHost.NeedsNewAgentMonitor)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx := t.Context()
			require.NoError(t, db.Clear(host.Collection))
			t.Cleanup(func() {
				assert.NoError(t, db.Clear(host.Collection))
			})
			h := &host.Host{
				Id:     "id",
				Secret: "secret",
			}
			require.NoError(t, h.Insert(ctx))

			testCase(t, ctx, h)
		})
	}
}

func TestProjectViewPermission(t *testing.T) {
	assert := assert.New(t)
	ctx := t.Context()
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

func TestRequiresRepoPermission(t *testing.T) {
	collections := []string{
		model.ProjectRefCollection,
		model.RepoRefCollection,
		evergreen.RoleCollection,
		evergreen.ScopeCollection,
	}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(collections...))
	})

	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:    "my-repo",
		Owner: "evergreen-ci",
		Repo:  "evergreen",
	}}
	require.NoError(t, repoRef.Replace(ctx))

	branchProject := model.ProjectRef{
		Id:         "branch-project",
		RepoRefId:  "my-repo",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "main",
		Identifier: "branch-project",
	}
	require.NoError(t, branchProject.Insert(ctx))

	repoScope := gimlet.Scope{
		ID:        "repo-scope",
		Resources: []string{repoRef.Id},
		Type:      evergreen.ProjectResourceType,
	}
	require.NoError(t, env.RoleManager().AddScope(ctx, repoScope))

	repoEditRole := gimlet.Role{
		ID:          "edit-repo-role",
		Scope:       repoScope.ID,
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value},
	}
	require.NoError(t, env.RoleManager().UpdateRole(ctx, repoEditRole))

	branchProjectScope := gimlet.Scope{
		ID:        "project-scope",
		Resources: []string{branchProject.Id},
		Type:      evergreen.ProjectResourceType,
	}
	require.NoError(t, env.RoleManager().AddScope(ctx, branchProjectScope))

	branchProjectViewRole := gimlet.Role{
		ID:          "view-project-role",
		Scope:       branchProjectScope.ID,
		Permissions: map[string]int{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value},
	}
	require.NoError(t, env.RoleManager().UpdateRole(ctx, branchProjectViewRole))

	viewMiddleware := RequiresRepoPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsView)
	editMiddleware := RequiresRepoPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsEdit)

	// Helper to build a request with user and project context populated (as addProject would do).
	makeRepoRequest := func(t *testing.T, usr *user.DBUser, repoID string) *http.Request {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/repos/"+repoID, nil)
		require.NoError(t, err)
		userCtx := gimlet.AttachUser(r.Context(), usr)
		userCtx, err = PrefetchProjectContext(userCtx, r, map[string]string{"repo_id": repoID})
		require.NoError(t, err)
		return r.WithContext(userCtx)
	}

	t.Run("NonexistentRepoReturnsNotFound", func(t *testing.T) {
		r := makeRepoRequest(t, &user.DBUser{Id: "some-user"}, "nonexistent")
		rw := httptest.NewRecorder()
		viewMiddleware.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("UserWithoutViewPermissionReturnsForbidden", func(t *testing.T) {
		r := makeRepoRequest(t, &user.DBUser{Id: "unauthorized-user"}, "my-repo")
		rw := httptest.NewRecorder()
		viewMiddleware.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
		assert.Equal(t, http.StatusUnauthorized, rw.Code)
	})

	t.Run("UserWithViewPermissionSucceeds", func(t *testing.T) {
		r := makeRepoRequest(t, &user.DBUser{Id: "view-user", SystemRoles: []string{"view-project-role"}}, "my-repo")
		rw := httptest.NewRecorder()
		viewMiddleware.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
		assert.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("ViewOnlyUserCannotEdit", func(t *testing.T) {
		r := makeRepoRequest(t, &user.DBUser{Id: "view-only-user", SystemRoles: []string{"view-project-role"}}, "my-repo")
		rw := httptest.NewRecorder()
		editMiddleware.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
		assert.Equal(t, http.StatusUnauthorized, rw.Code)
	})

	t.Run("UserWithoutEditPermissionReturnsUnauthorized", func(t *testing.T) {
		r := makeRepoRequest(t, &user.DBUser{Id: "unauthorized-user"}, "my-repo")
		rw := httptest.NewRecorder()
		editMiddleware.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
		assert.Equal(t, http.StatusUnauthorized, rw.Code)
	})

	t.Run("UserWithEditPermissionSucceeds", func(t *testing.T) {
		r := makeRepoRequest(t, &user.DBUser{Id: "edit-user", SystemRoles: []string{"edit-repo-role"}}, "my-repo")
		rw := httptest.NewRecorder()
		editMiddleware.ServeHTTP(rw, r, func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})
		assert.Equal(t, http.StatusOK, rw.Code)
	})

}

func TestURLVarsToDistroScopes(t *testing.T) {
	require.NoError(t, db.ClearCollections(distro.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(distro.Collection))
	})

	targetDistro := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, targetDistro.Insert(t.Context()))
	otherDistro := distro.Distro{
		Id: "other-distro",
	}
	require.NoError(t, otherDistro.Insert(t.Context()))

	for tName, tCase := range map[string]struct {
		pathVars           map[string]string
		queryString        string
		expectedDistroIDs  []string
		expectedStatusCode int
	}{
		"ResolvesDistroFromPath": {
			pathVars:           map[string]string{"distro_id": targetDistro.Id},
			expectedDistroIDs:  []string{targetDistro.Id},
			expectedStatusCode: http.StatusOK,
		},
		"IgnoresQueryStringDistroWhenPathHasDistro": {
			pathVars:           map[string]string{"distro_id": targetDistro.Id},
			queryString:        fmt.Sprintf("distro_id=%s", otherDistro.Id),
			expectedDistroIDs:  []string{targetDistro.Id},
			expectedStatusCode: http.StatusOK,
		},
		"QueryStringOnlyDistroIsNotFound": {
			queryString:        fmt.Sprintf("distro_id=%s", otherDistro.Id),
			expectedStatusCode: http.StatusNotFound,
		},
		"NonexistentDistroInPathIsNotFound": {
			pathVars:           map[string]string{"distro_id": "nonexistent-distro"},
			expectedStatusCode: http.StatusNotFound,
		},
		"NoDistroIsNotFound": {
			expectedStatusCode: http.StatusNotFound,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			url := "/rest/v2/distros/some-distro"
			if tCase.queryString != "" {
				url += "?" + tCase.queryString
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, tCase.pathVars)

			distroIDs, statusCode, err := urlVarsToDistroScopes(req)

			assert.Equal(t, tCase.expectedStatusCode, statusCode)
			if tCase.expectedStatusCode != http.StatusOK {
				assert.Error(t, err)
				assert.Empty(t, distroIDs)
				return
			}
			assert.NoError(t, err)
			assert.ElementsMatch(t, tCase.expectedDistroIDs, distroIDs)
		})
	}
}
