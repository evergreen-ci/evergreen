package graphql_test

import (
	"context"
	"testing"
	"time"

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
	const apiKey = "testapikey"
	const slackUsername = "testslackuser"
	const email = "testuser@mongodb.com"
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))
	testUser := user.DBUser{
		Id:           apiUser,
		APIKey:       apiKey,
		EmailAddress: email,
		Settings:     user.UserSettings{Timezone: "America/New_York", SlackUsername: slackUsername},
		SystemRoles:  []string{"unrestrictedTaskAccess", "modify_host"},
		PubKeys: []user.PubKey{
			{Name: "z", Key: "zKey", CreatedAt: time.Time{}},
			{Name: "c", Key: "cKey", CreatedAt: time.Time{}},
			{Name: "d", Key: "dKey", CreatedAt: time.Time{}},
			{Name: "a", Key: "aKey", CreatedAt: time.Time{}},
			{Name: "b", Key: "bKey", CreatedAt: time.Time{}},
		}}
	require.NoError(t, testUser.Insert())
	modifyHostRole := gimlet.Role{
		ID:          "modify_host",
		Name:        "modify host",
		Scope:       "modify_host_scope",
		Permissions: map[string]int{"distro_hosts": 20},
	}
	_, err := env.DB().Collection("roles").InsertOne(ctx, modifyHostRole)
	require.NoError(t, err)

	modifyHostScope := gimlet.Scope{
		ID:        "modify_host_scope",
		Name:      "modify host scope",
		Type:      "distro",
		Resources: []string{"ubuntu1604-small", "ubuntu1604-large"},
	}
	_, err = env.DB().Collection("scopes").InsertOne(ctx, modifyHostScope)
	require.NoError(t, err)
	state.apiKey = apiKey
	state.apiUser = apiUser

	require.NoError(t, setupData(*evergreen.GetEnvironment().DB(), *evergreen.GetEnvironment().Client().Database(state.taskLogDB), state.testData, *state))
}

func XTestDirectives(t *testing.T) {
	// settings := testutil.TestConfig()
	// testutil.ConfigureIntegrationTest(t, settings, "TestDirectives")
	// server, err := service.CreateTestServer(settings, nil, true)
	// require.NoError(t, err)
	// defer server.Close()
	config := graphql.New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := interface{}(nil)
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		return nil, nil
	}
	res, err := config.Directives.SuperUserOnly(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestSuperUser(t *testing.T) {
	setupPermissions(t, &atomicGraphQLState{})
	config := graphql.New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := interface{}(nil)
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		return nil, nil
	}
	res, err := config.Directives.SuperUserOnly(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
}
