package graphql_test

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()

}

func setupPermissions(t *testing.T, state *atomicGraphQLState) {
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))

	// Create scope and role collection to avoid RoleManager from trying to create them in a collection https://jira.mongodb.org/browse/EVG-15499
	require.NoError(t, env.DB().CreateCollection(ctx, evergreen.ScopeCollection))
	require.NoError(t, env.DB().CreateCollection(ctx, evergreen.RoleCollection))

	roleManager := env.RoleManager()

	roles, err := roleManager.GetAllRoles()
	require.NoError(t, err)
	require.Len(t, roles, 0)

	superUserRole := gimlet.Role{
		ID:          "superuser",
		Name:        "superuser",
		Scope:       "superuser_scope",
		Permissions: map[string]int{"admin_settings": 10, "project_create": 10, "distro_create": 10, "modify_roles": 10},
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
}

func TestSuperUser(t *testing.T) {
	setupPermissions(t, &atomicGraphQLState{})
	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := graphql.New("/graphql")
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
