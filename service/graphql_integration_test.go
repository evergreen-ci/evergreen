package service

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

const apiKey = "testapikey"
const apiUser = "testuser"

func TestAtomicGQLQueries(t *testing.T) {
	grip.Warning(grip.SetSender(send.MakePlainLogger()))
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestAtomicGQLQueries")
	testDirectories, err := ioutil.ReadDir("../graphql/tests")
	require.NoError(t, err)
	server, err := CreateTestServer(settings, nil, true)
	require.NoError(t, err)
	defer server.Close()

	for _, dir := range testDirectories {
		state := graphql.AtomicGraphQLState{
			TaskLogDB:   model.TaskLogDB,
			TaskLogColl: model.TaskLogCollection,
			Directory:   dir.Name(),
			Settings:    settings,
			ServerURL:   server.URL,
		}
		t.Run(state.Directory, graphql.MakeTestsInDirectory(&state))
	}
}

func TestGQLQueries(t *testing.T) {
	server, err := CreateTestServer(testutil.TestConfig(), nil, true)
	require.NoError(t, err)
	env := evergreen.GetEnvironment()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, env.DB().Drop(ctx))
	testUser := user.DBUser{
		Id:          apiUser,
		APIKey:      apiKey,
		Settings:    user.UserSettings{Timezone: "America/New_York"},
		SystemRoles: []string{"unrestrictedTaskAccess"},
	}
	require.NoError(t, testUser.Insert())

	require.NoError(t, db.EnsureIndex(testresult.Collection, mongo.IndexModel{
		Keys: testresult.TestResultsIndex}))

	graphql.TestQueries(t, server.URL)
}
