package operations

import (
	"flag"
	"path/filepath"
	"testing"

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
			name:        "CrossFileAnchorYAMLFailsWithoutFlag",
			args:        []string{"--" + pathFlagName, crossFilePath},
			expectError: true,
		},
		{
			name: "CrossFileAnchorYAMLSucceedsWithFlag",
			args: []string{"--" + pathFlagName, crossFilePath, "--" + crossFileAnchorsFlagName},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := cli.NewApp()
			app.Commands = []cli.Command{Evaluate()}

			parentSet := flag.NewFlagSet("parent", 0)
			parentSet.String(ConfFlagName, filepath.Join(t.TempDir(), "nonexistent.yml"), "")

			childSet := flag.NewFlagSet("evaluate", 0)
			childSet.String(pathFlagName, "", "")
			childSet.Bool(crossFileAnchorsFlagName, false, "")
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
