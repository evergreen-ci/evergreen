package graphql

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

func init() {
	testutil.Setup()
}

func setupPermissions(t *testing.T) {
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))

	roleManager := env.RoleManager()

	roles, err := roleManager.GetAllRoles()
	require.NoError(t, err)
	require.Len(t, roles, 0)

	superUserRole := gimlet.Role{
		ID:          "superuser",
		Name:        "superuser",
		Scope:       "superuser_scope",
		Permissions: map[string]int{"project_create": 10, "distro_create": 10, "modify_roles": 10},
	}
	err = roleManager.UpdateRole(superUserRole)
	require.NoError(t, err)

	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{"super_user"},
	}
	err = roleManager.AddScope(superUserScope)
	require.NoError(t, err)

	projectAdminRole := gimlet.Role{
		ID:          "admin_project",
		Scope:       "project_scope",
		Permissions: map[string]int{"project_settings": 20, "project_tasks": 30, "project_patches": 10, "project_logs": 10},
	}
	err = roleManager.UpdateRole(projectAdminRole)
	require.NoError(t, err)

	projectViewRole := gimlet.Role{
		ID:          "view_project",
		Scope:       "project_scope",
		Permissions: map[string]int{"project_settings": 10, "project_tasks": 30, "project_patches": 10, "project_logs": 10},
	}
	err = roleManager.UpdateRole(projectViewRole)
	require.NoError(t, err)

	projectScope := gimlet.Scope{
		ID:        "project_scope",
		Name:      "project scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"project_id", "repo_id"},
	}
	err = roleManager.AddScope(projectScope)
	require.NoError(t, err)
}

func TestRequireSuperUser(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := interface{}(nil)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := user.GetOrCreateUser(apiUser, "Mohamed Khelif", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	res, err := config.Directives.RequireSuperUser(ctx, obj, next)
	require.Error(t, err, "user testuser does not have permission to access this resolver")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	err = usr.AddRole("superuser")
	require.NoError(t, err)

	res, err = config.Directives.RequireSuperUser(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)
}

func setupUser(t *testing.T) (*user.DBUser, error) {
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())
	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	return user.GetOrCreateUser(apiUser, "Evergreen User", email, accessToken, refreshToken, []string{})
}

func TestRequireProjectAccess(t *testing.T) {
	setupPermissions(t)
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := interface{}(nil)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}
	err = projectRef.Insert()
	require.NoError(t, err)

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id: "repo_id",
	}}
	err = repoRef.Upsert()
	require.NoError(t, err)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.EqualError(t, err, "input: Project not specified")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	obj = interface{}(map[string]interface{}(nil))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.EqualError(t, err, "input: Could not find project")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	obj = interface{}(map[string]interface{}{"identifier": "invalid_identifier"})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.EqualError(t, err, "input: Could not find project with identifier: invalid_identifier")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	obj = interface{}(map[string]interface{}{"identifier": "project_identifier"})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.EqualError(t, err, "input: user testuser does not have permission to access settings for the project project_id")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	obj = interface{}(map[string]interface{}{"id": "project_id"})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.EqualError(t, err, "input: user testuser does not have permission to access settings for the project project_id")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	err = usr.AddRole("view_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.EqualError(t, err, "input: user testuser does not have permission to access settings for the project project_id")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	err = usr.AddRole("admin_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)

	obj = interface{}(map[string]interface{}{"identifier": "project_identifier"})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	obj = interface{}(map[string]interface{}{"id": "repo_id"})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectSettingsAccessEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)
}

func TestRestrictProjectAccess(t *testing.T) {
	setupPermissions(t)
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
		Admins:     []string{"admin_1", "admin_2", "admin_3"},
	}
	err = projectRef.Insert()
	require.NoError(t, err)

	apiProjectRef := &restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(projectRef)
	require.NoError(t, err)

	fieldCtx := &graphql.FieldContext{
		Field: graphql.CollectedField{
			Field: &ast.Field{
				Alias: "admins",
			},
		},
	}
	ctx = graphql.WithFieldContext(ctx, fieldCtx)

	res, err := config.Directives.RestrictProjectAccess(ctx, apiProjectRef, next)
	require.Error(t, err, "user does not have permission to access the field admins for project with ID 'project_id")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	err = usr.AddRole("view_project")
	require.NoError(t, err)

	res, err = config.Directives.RestrictProjectAccess(ctx, apiProjectRef, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)
}
