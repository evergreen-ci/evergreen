package graphql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/require"
)

const pathToTests = "../../graphql"

func TestAtomicGQLQueries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	grip.Warning(grip.SetSender(send.MakePlainLogger()))
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings)
	testDirectories, err := os.ReadDir(filepath.Join(pathToTests, "tests"))
	require.NoError(t, err)
	server, err := service.CreateTestServer(ctx, settings, nil, true)
	require.NoError(t, err)
	defer server.Close()
	dir, _ := os.Getwd()
	fmt.Println("PATH: ", dir)

	for _, dir := range testDirectories {
		dirContents, err := os.ReadDir(filepath.Join(pathToTests, "tests", dir.Name()))
		require.NoError(t, err)

		for _, entry := range dirContents {
			if entry.IsDir() {
				var state graphql.AtomicGraphQLState
				if entry.Name() == "queries" {
					// The existence of a queries folder suggests that there are old format tests to run.
					state = graphql.AtomicGraphQLState{
						Directory: dir.Name(),
						Settings:  settings,
						ServerURL: server.URL,
					}
				} else {
					// A nested directory that isn't a queries folder suggests that there are new format tests to run.
					state = graphql.AtomicGraphQLState{
						Directory: filepath.Join(dir.Name(), entry.Name()),
						Settings:  settings,
						ServerURL: server.URL,
					}
				}
				t.Run(state.Directory, graphql.MakeTestsInDirectory(&state, pathToTests))
			}
		}
	}
}
