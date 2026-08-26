package operations

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestEvaluateWithNoClientConfig(t *testing.T) {
	app := cli.NewApp()
	app.Commands = []cli.Command{Evaluate()}

	// Set ConfFlagName on the parent context to a nonexistent path so that
	// NewClientSettings fails gracefully (warn-and-continue) without touching
	// the real ~/.evergreen.yml on the host.
	parentSet := flag.NewFlagSet("parent", 0)
	parentSet.String(ConfFlagName, filepath.Join(t.TempDir(), "nonexistent.yml"), "")
	parentCtx := cli.NewContext(app, parentSet, nil)

	childSet := flag.NewFlagSet("test", 0)
	childSet.String(pathFlagName, filepath.Join("testdata", "sample.yml"), "")
	require.NoError(t, childSet.Parse([]string{"--path", filepath.Join("testdata", "sample.yml")}))
	ctx := cli.NewContext(app, childSet, parentCtx)

	require.NoError(t, Evaluate().Action.(func(*cli.Context) error)(ctx))
}
