package remote

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper/scripting"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScripting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	for managerName, makeManager := range remoteManagerTestCases(httpClient) {
		t.Run(managerName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, client Manager, tmpDir string){
				"SetupSucceeds": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)
					assert.NoError(t, harness.Setup(ctx))
				},
				"SetupFails": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					if os.Geteuid() == 0 {
						t.Skip("running as root will prevent setup from failing")
					}
					if runtime.GOOS == "windows" {
						t.Skip("chmod does not work on Windows")
					}
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)
					require.NoError(t, os.Chmod(tmpDir, 0555))
					assert.Error(t, harness.Setup(ctx))
					require.NoError(t, os.Chmod(tmpDir, 0777))
				},
				"CleanupSucceeds": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)
					assert.NoError(t, harness.Cleanup(ctx))
				},
				"CleanupFails": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					if os.Geteuid() == 0 {
						t.Skip("running as root will prevent setup from failing")
					}
					if runtime.GOOS == "windows" {
						t.Skip("chmod does not work on Windows")
					}
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)
					require.NoError(t, harness.Setup(ctx))
					require.NoError(t, os.Chmod(tmpDir, 0555))
					assert.Error(t, harness.Cleanup(ctx))
					require.NoError(t, os.Chmod(tmpDir, 0777))
				},
				"RunSucceeds": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)
					tmpFile := filepath.Join(tmpDir, "fake_script.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangMainSuccess()), 0755))
					assert.NoError(t, harness.Run(ctx, []string{tmpFile}))
				},
				"RunFails": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)

					tmpFile := filepath.Join(tmpDir, "fake_script.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangMainFail()), 0755))
					assert.Error(t, harness.Run(ctx, []string{tmpFile}))
				},
				"RunScriptSucceeds": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)
					assert.NoError(t, harness.RunScript(ctx, testutil.GolangMainSuccess()))
				},
				"RunScriptFails": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {

					harness := createTestScriptingHarness(ctx, t, client, tmpDir)
					require.Error(t, harness.RunScript(ctx, testutil.GolangMainFail()))
				},
				"BuildSucceeds": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)

					tmpFile := filepath.Join(tmpDir, "fake_script.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangMainSuccess()), 0755))
					buildFile := filepath.Join(tmpDir, "fake_script")
					_, err := harness.Build(ctx, tmpDir, []string{
						"-o",
						buildFile,
						tmpFile,
					})
					require.NoError(t, err)
					_, err = os.Stat(buildFile)
					require.NoError(t, err)
				},
				"BuildFails": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)

					tmpFile := filepath.Join(tmpDir, "fake_script.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; func main() { "bad syntax" }`), 0755))
					buildFile := filepath.Join(tmpDir, "fake_script")
					_, err := harness.Build(ctx, tmpDir, []string{
						"-o",
						buildFile,
						tmpFile,
					})
					require.Error(t, err)
					_, err = os.Stat(buildFile)
					assert.Error(t, err)
				},
				"TestSucceeds": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)

					tmpFile := filepath.Join(tmpDir, "fake_script_test.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangTestSuccess()), 0755))
					results, err := harness.Test(ctx, tmpDir, scripting.TestOptions{Name: "dummy"})
					require.NoError(t, err)
					require.Len(t, results, 1)
					assert.Equal(t, scripting.TestOutcomeSuccess, results[0].Outcome)
				},
				"TestFails": func(ctx context.Context, t *testing.T, client Manager, tmpDir string) {
					harness := createTestScriptingHarness(ctx, t, client, tmpDir)

					tmpFile := filepath.Join(tmpDir, "fake_script_test.go")
					require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangTestFail()), 0755))
					results, err := harness.Test(ctx, tmpDir, scripting.TestOptions{Name: "dummy"})
					assert.Error(t, err)
					require.Len(t, results, 1)
					assert.Equal(t, scripting.TestOutcomeFailure, results[0].Outcome)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
					defer cancel()
					client := makeManager(tctx, t)
					tmpDir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, os.RemoveAll(tmpDir))
					}()
					testCase(tctx, t, client, tmpDir)
				})

			}
		})
	}
}

func createTestScriptingHarness(ctx context.Context, t *testing.T, client Manager, dir string) scripting.Harness {
	opts := testoptions.ValidGolangScriptingHarnessOptions(dir)
	sh, err := client.CreateScripting(ctx, opts)
	require.NoError(t, err)
	return sh
}
