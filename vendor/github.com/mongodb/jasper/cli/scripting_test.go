package cli

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/mongodb/jasper/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestCLIScripting(t *testing.T) {
	for remoteType, makeService := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) util.CloseFunc{
		RESTService: makeTestRESTService,
		RPCService:  makeTestRPCService,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string){
				"SetupSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					id := createScriptingFromCLI(t, c, testoptions.ValidGolangScriptingHarnessOptions(tmpDir))
					setupScriptingFromCLI(t, c, id)
				},
				"SetupWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(IDInput{})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, scriptingSetup(), input, resp))
					assert.False(t, resp.Successful())
				},
				"SetupWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(IDInput{ID: "foo"})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingSetup(), input, resp))
					assert.False(t, resp.Successful())
				},
				"RunSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					id := createScriptingFromCLI(t, c, testoptions.ValidGolangScriptingHarnessOptions(tmpDir))
					setupScriptingFromCLI(t, c, id)
					tmpFile := filepath.Join(tmpDir, "main.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangMainSuccess()), 0755))
					input, err := json.Marshal(ScriptingRunInput{
						ID:   id,
						Args: []string{tmpFile},
					})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingRun(), input, resp))
					assert.True(t, resp.Successful())
				},
				"RunWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingRunInput{Args: []string{"./"}})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, scriptingRun(), input, resp))
					assert.False(t, resp.Successful())
				},
				"RunWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingRunInput{
						ID:   "foo",
						Args: []string{"./"},
					})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingRun(), input, resp))
					assert.False(t, resp.Successful())
				},
				"RunScriptSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					id := createScriptingFromCLI(t, c, testoptions.ValidGolangScriptingHarnessOptions(tmpDir))
					setupScriptingFromCLI(t, c, id)
					input, err := json.Marshal(ScriptingRunScriptInput{
						ID:     id,
						Script: testutil.GolangMainSuccess(),
					})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingRunScript(), input, resp))
					assert.True(t, resp.Successful())
				},
				"RunScriptWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingRunScriptInput{Script: testutil.GolangMainSuccess()})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, scriptingRunScript(), input, resp))
					assert.False(t, resp.Successful())
				},
				"RunScriptWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingRunScriptInput{
						ID:     "foo",
						Script: testutil.GolangMainSuccess(),
					})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingRunScript(), input, resp))
					assert.False(t, resp.Successful())
				},
				"BuildSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					id := createScriptingFromCLI(t, c, testoptions.ValidGolangScriptingHarnessOptions(tmpDir))
					setupScriptingFromCLI(t, c, id)
					tmpFile := filepath.Join(tmpDir, "main.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangMainSuccess()), 0755))
					input, err := json.Marshal(ScriptingBuildInput{
						ID:        id,
						Directory: tmpDir,
					})
					require.NoError(t, err)
					resp := &ScriptingBuildResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingBuild(), input, resp))
					assert.True(t, resp.Successful())
				},
				"BuildWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingBuildInput{
						Directory: tmpDir,
					})
					require.NoError(t, err)
					resp := &ScriptingBuildResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, scriptingBuild(), input, resp))
					assert.False(t, resp.Successful())
				},
				"BuildWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingBuildInput{
						ID:        "foo",
						Directory: tmpDir,
					})
					require.NoError(t, err)
					resp := &ScriptingBuildResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingBuild(), input, resp))
					assert.False(t, resp.Successful())
				},
				"TestSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					id := createScriptingFromCLI(t, c, testoptions.ValidGolangScriptingHarnessOptions(tmpDir))
					setupScriptingFromCLI(t, c, id)
					tmpFile := filepath.Join(tmpDir, "main.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangTestSuccess()), 0755))
					input, err := json.Marshal(ScriptingTestInput{
						ID:        id,
						Directory: tmpDir,
					})
					require.NoError(t, err)
					resp := &ScriptingTestResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingTest(), input, resp))
					assert.True(t, resp.Successful())
				},
				"TestWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingTestInput{
						Directory: tmpDir,
					})
					require.NoError(t, err)
					resp := &ScriptingTestResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, scriptingTest(), input, resp))
					assert.False(t, resp.Successful())
				},
				"TestWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(ScriptingTestInput{
						ID:        "foo",
						Directory: tmpDir,
					})
					require.NoError(t, err)
					resp := &ScriptingTestResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingTest(), input, resp))
					assert.False(t, resp.Successful())
				},
				"CleanupSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					id := createScriptingFromCLI(t, c, testoptions.ValidGolangScriptingHarnessOptions(tmpDir))
					setupScriptingFromCLI(t, c, id)
					input, err := json.Marshal(IDInput{ID: id})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingCleanup(), input, resp))
					assert.True(t, resp.Successful())
				},
				"CleanupWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(IDInput{})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, scriptingCleanup(), input, resp))
					assert.False(t, resp.Successful())
				},
				"CleanupWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, tmpDir string) {
					input, err := json.Marshal(IDInput{ID: "foo"})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, scriptingCleanup(), input, resp))
					assert.False(t, resp.Successful())
				},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.RPCTestTimeout)
					defer cancel()
					port := testutil.GetPortNumber()
					c := mockCLIContext(remoteType, port)
					manager, err := jasper.NewSynchronizedManager(false)
					require.NoError(t, err)
					closeService := makeService(ctx, t, port, manager)
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, closeService())
					}()

					tmpDir, err := ioutil.TempDir(testutil.BuildDirectory(), "cli-scripting")
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, os.RemoveAll(tmpDir))
					}()

					testCase(ctx, t, c, tmpDir)
				})
			}
		})
	}
}

// createScriptingFromCLI creates a scripting harness on a remote service
// using the CLI.
func createScriptingFromCLI(t *testing.T, c *cli.Context, opts options.ScriptingHarness) string {
	input, err := BuildScriptingCreateInput(opts)
	require.NoError(t, err)
	jsonInput, err := json.Marshal(input)
	require.NoError(t, err)
	resp := &IDResponse{}
	require.NoError(t, execCLICommandInputOutput(t, c, remoteCreateScripting(), jsonInput, resp))
	require.True(t, resp.Successful())
	require.NotZero(t, resp.ID)

	return resp.ID
}

// setupScriptingFromCLI sets up the scripting harness with the given ID on a
// remote service using the CLI.
func setupScriptingFromCLI(t *testing.T, c *cli.Context, id string) {
	input, err := json.Marshal(IDInput{ID: id})
	require.NoError(t, err)
	resp := &OutcomeResponse{}
	require.NoError(t, execCLICommandInputOutput(t, c, scriptingSetup(), input, resp))
	require.True(t, resp.Successful())
}
