package service

import (
	"io/ioutil"
	"testing"

	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/require"
)

func TestAtomicGQLQueries(t *testing.T) {
	grip.Warning(grip.SetSender(send.MakePlainLogger()))
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestAtomicGQLQueries")
	testDirectories, err := ioutil.ReadDir("tests")
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
