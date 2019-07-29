package cli

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tychoish/bond"
	"github.com/urfave/cli"
)

func validMongoDBDownloadOptions() jasper.MongoDBDownloadOptions {
	target := runtime.GOOS
	if target == "darwin" {
		target = "osx"
	}

	edition := "enterprise"
	if target == "linux" {
		edition = "base"
	}

	return jasper.MongoDBDownloadOptions{
		BuildOpts: bond.BuildOptions{
			Target:  target,
			Arch:    bond.MongoDBArch("x86_64"),
			Edition: bond.MongoDBEdition(edition),
			Debug:   false,
		},
		Releases: []string{"4.0-current"},
	}
}

func TestCLIRemote(t *testing.T) {
	for remoteType, makeService := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) jasper.CloseFunc{
		RESTService: makeTestRESTService,
		RPCService:  makeTestRPCService,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context){
				"ConfigureCachePasses": func(ctx context.Context, t *testing.T, c *cli.Context) {
					input, err := json.Marshal(jasper.CacheOptions{})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, remoteConfigureCache(), input, resp))
					assert.True(t, resp.Successful())
				},
				"DownloadFilePasses": func(ctx context.Context, t *testing.T, c *cli.Context) {
					cwd, err := os.Getwd()
					require.NoError(t, err)

					tmpFile, err := ioutil.TempFile(filepath.Join(filepath.Dir(cwd), "build"), "out.txt")
					require.NoError(t, err)
					defer func() {
						require.NoError(t, tmpFile.Close())
						os.Remove(tmpFile.Name())
					}()

					input, err := json.Marshal(jasper.DownloadInfo{
						URL:  "https://example.com",
						Path: tmpFile.Name(),
					})
					grip.Infof("tmpfile name = %s", tmpFile.Name())
					require.NoError(t, err)

					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, remoteDownloadFile(), input, resp))

					info, err := os.Stat(tmpFile.Name())
					require.NoError(t, err)
					assert.NotZero(t, info.Size)
				},
				"DownloadMongoDBPasses": func(ctx context.Context, t *testing.T, c *cli.Context) {
					cwd, err := os.Getwd()
					require.NoError(t, err)

					tmpDir, err := ioutil.TempDir(filepath.Join(filepath.Dir(cwd), "build"), "out")
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, os.RemoveAll(tmpDir))
					}()

					opts := validMongoDBDownloadOptions()
					opts.Path = tmpDir
					input, err := json.Marshal(opts)
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, remoteDownloadMongoDB(), input, resp))
				},
				"GetBuildloggerURLsFailsWithNonexistentProcess": func(ctx context.Context, t *testing.T, c *cli.Context) {
					input, err := json.Marshal(IDInput{ID: "foo"})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, remoteGetBuildloggerURLs(), input, resp))
					assert.False(t, resp.Successful())
				},
				"GetLogStreamPasses": func(ctx context.Context, t *testing.T, c *cli.Context) {
					opts := trueCreateOpts()
					opts.Output.Loggers = []jasper.Logger{jasper.NewInMemoryLogger(10)}
					createInput, err := json.Marshal(opts)
					require.NoError(t, err)
					createResp := &InfoResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerCreateProcess(), createInput, createResp))

					input, err := json.Marshal(LogStreamInput{ID: createResp.Info.ID, Count: 100})
					require.NoError(t, err)
					resp := &LogStreamResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, remoteGetLogStream(), input, resp))

					assert.True(t, resp.Successful())
				},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
					defer cancel()

					port := getNextPort()
					c := mockCLIContext(remoteType, port)
					manager, err := jasper.NewLocalManager(false)
					require.NoError(t, err)
					closeService := makeService(ctx, t, port, manager)
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, closeService())
					}()

					testCase(ctx, t, c)
				})
			}
		})
	}
}
