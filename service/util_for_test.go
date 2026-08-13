package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// newTestUIRouter is the same as makeAuthTestUIRouter but without any user
// authentication middleware configuration.
func newTestUIRouter(ctx context.Context, env evergreen.Environment) (http.Handler, error) {
	return makeAuthTestUIRouter(ctx, env, gimlet.UserMiddlewareConfiguration{})
}

// newAuthTestUIRouter is the same as makeAuthTestUIRouter but allows
// cookie-based and API key-based user authentication.
func newAuthTestUIRouter(ctx context.Context, env evergreen.Environment) (http.Handler, error) {
	return makeAuthTestUIRouter(ctx, env, gimlet.UserMiddlewareConfiguration{
		CookieName:     evergreen.AuthTokenCookie,
		HeaderKeyName:  evergreen.APIKeyHeader,
		HeaderUserName: evergreen.APIUserHeader,
	})
}

// makeAuthTestUIRouter creates a router for testing the UI server with a user
// middleware configuration.
func makeAuthTestUIRouter(ctx context.Context, env evergreen.Environment, umconf gimlet.UserMiddlewareConfiguration) (http.Handler, error) {
	settings := env.Settings()
	if env.UserManager() == nil {
		um, info, err := auth.LoadUserManager(settings)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		env.SetUserManager(um)
		env.SetUserManagerInfo(info)
	}
	uis := &UIServer{
		RootURL:  settings.Ui.Url,
		Settings: *settings,
		env:      env,
	}
	app := GetRESTv1App(uis)
	app.AddMiddleware(gimlet.UserMiddleware(ctx, env.UserManager(), umconf))
	return app.Handler()
}

// addViewTasksPermission registers a scope and role granting TasksView
// for the given project and returns a DBUser that holds the role. The
// caller must ensure the roles and scopes collections are cleaned up.
func addViewTasksPermission(t *testing.T, project string) *user.DBUser {
	t.Helper()
	ctx := t.Context()
	rm := evergreen.GetEnvironment().RoleManager()

	scope := gimlet.Scope{
		ID:        project + "_scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{project},
	}
	require.NoError(t, rm.AddScope(ctx, scope))

	role := gimlet.Role{
		ID:          project + "_view_tasks",
		Scope:       scope.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	require.NoError(t, rm.UpdateRole(ctx, role))

	return &user.DBUser{Id: "user", SystemRoles: []string{role.ID}}
}
