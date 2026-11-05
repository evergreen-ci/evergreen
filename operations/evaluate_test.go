package operations

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestEvaluate(t *testing.T) {
	samplePath := filepath.Join("testdata", "sample.yml")
	crossFilePath := filepath.Join("testdata", "cross_file_anchor_main.yml")

	for _, tc := range []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name: "SucceedsWithValidFile",
			args: []string{"--" + pathFlagName, samplePath},
		},
		{
			name: "SucceedsWithCrossFileAnchors",
			args: []string{"--" + pathFlagName, crossFilePath},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Keep the command hermetic: without a mock client any server
			// communication hits the default API server.
			mockClient = &client.Mock{}
			defer func() { mockClient = nil }()

			app := cli.NewApp()
			app.Commands = []cli.Command{Evaluate()}

			parentSet := flag.NewFlagSet("parent", 0)
			parentSet.String(ConfFlagName, filepath.Join(t.TempDir(), "nonexistent.yml"), "")

			childSet := flag.NewFlagSet("evaluate", 0)
			childSet.String(pathFlagName, "", "")
			require.NoError(t, childSet.Parse(tc.args))

			ctx := cli.NewContext(app, childSet, cli.NewContext(app, parentSet, nil))
			err := Evaluate().Action.(func(*cli.Context) error)(ctx)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
