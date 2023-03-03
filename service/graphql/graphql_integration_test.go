package graphql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/require"
)

const apiKey = "testapikey"
const apiUser = "testuser"
const pathToTests = "../../graphql"

func TestAtomicGQLQueries(t *testing.T) {
	grip.Warning(grip.SetSender(send.MakePlainLogger()))
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, t.Name())
	testDirectories, err := os.ReadDir(filepath.Join(pathToTests, "tests"))
	require.NoError(t, err)
	server, err := service.CreateTestServer(settings, nil, true)
	require.NoError(t, err)
	defer server.Close()
	dir, _ := os.Getwd()
	fmt.Println("PATH: ", dir)

	for _, dir := range testDirectories {
		state := graphql.AtomicGraphQLState{
			TaskLogDB:   model.TaskLogDB,
			TaskLogColl: model.TaskLogCollection,
			Directory:   dir.Name(),
			Settings:    settings,
			ServerURL:   server.URL,
		}
		t.Run(state.Directory, graphql.MakeTestsInDirectory(&state, pathToTests))
	}
}

func TestGQLQueries(t *testing.T) {
	server, err := service.CreateTestServer(testutil.TestConfig(), nil, true)
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

	graphql.TestQueries(t, server.URL, pathToTests)
}
