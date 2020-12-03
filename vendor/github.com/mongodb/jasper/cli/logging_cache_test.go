package cli

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/mongodb/jasper/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestCLILoggingCache(t *testing.T) {
	for remoteType, makeService := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) util.CloseFunc{
		RESTService: makeTestRESTService,
		RPCService:  makeTestRPCService,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context){
				"CreateSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context) {
					id := "id"
					logger := createCachedLoggerFromCLI(t, c, id)
					assert.Equal(t, id, logger.ID)
					assert.NotZero(t, logger.Accessed)
					assert.NotZero(t, logger.ManagerID)
				},
				"CreateWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context) {
					logger, err := jasper.NewInMemoryLogger(100)
					require.NoError(t, err)
					input, err := json.Marshal(LoggingCacheCreateInput{
						Output: options.Output{
							Loggers: []*options.LoggerConfig{logger},
						},
					})
					require.NoError(t, err)
					resp := &CachedLoggerResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, loggingCacheCreate(), input, resp))
					assert.False(t, resp.Successful())
					assert.Zero(t, resp.Logger)
				},
				"GetSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context) {
					logger := createCachedLoggerFromCLI(t, c, "id")

					input, err := json.Marshal(IDInput{ID: logger.ID})
					require.NoError(t, err)
					resp := &CachedLoggerResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheGet(), input, resp))
					require.True(t, resp.Successful())
					assert.Equal(t, logger.ID, resp.Logger.ID)
					assert.NotZero(t, resp.Logger.Accessed)
				},
				"GetWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context) {
					_ = createCachedLoggerFromCLI(t, c, "id")

					input, err := json.Marshal(IDInput{})
					require.NoError(t, err)
					resp := &CachedLoggerResponse{}
					assert.Error(t, execCLICommandInputOutput(t, c, loggingCacheGet(), input, resp))
					assert.False(t, resp.Successful())
				},
				"GetWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context) {
					_ = createCachedLoggerFromCLI(t, c, "id")

					input, err := json.Marshal(IDInput{ID: "foo"})
					require.NoError(t, err)
					resp := &CachedLoggerResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheGet(), input, resp))
					assert.False(t, resp.Successful())
				},
				"RemoveSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context) {
					logger := createCachedLoggerFromCLI(t, c, "id")

					input, err := json.Marshal(IDInput{ID: logger.ID})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheRemove(), input, resp))
					require.True(t, resp.Successful())

					getResp := &CachedLoggerResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheGet(), input, getResp))
					assert.False(t, getResp.Successful())
					assert.Zero(t, getResp.Logger)
				},
				"CloseAndRemoveSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context) {
					logger := createCachedLoggerFromCLI(t, c, "id")

					input, err := json.Marshal(IDInput{ID: logger.ID})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheCloseAndRemove(), input, resp))
					require.True(t, resp.Successful())

					getResp := &CachedLoggerResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheGet(), input, getResp))
					assert.False(t, getResp.Successful())
					assert.Zero(t, getResp.Logger)
				},
				"ClearSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context) {
					_ = createCachedLoggerFromCLI(t, c, "id0")
					_ = createCachedLoggerFromCLI(t, c, "id1")

					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandOutput(t, c, loggingCacheClear(), resp))
					require.True(t, resp.Successful())

					getResp := &LoggingCacheLenResponse{}
					require.NoError(t, execCLICommandOutput(t, c, loggingCacheLen(), getResp))
					assert.True(t, getResp.Successful())
					assert.Zero(t, getResp.Length)
				},
				"RemoveWithNonexistentIDNoops": func(ctx context.Context, t *testing.T, c *cli.Context) {
					logger := createCachedLoggerFromCLI(t, c, "id")

					input, err := json.Marshal(IDInput{ID: "foo"})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheRemove(), input, resp))
					assert.True(t, resp.Successful())

					getResp := &CachedLoggerResponse{}
					getInput, err := json.Marshal(IDInput{ID: logger.ID})
					require.NoError(t, err)
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheGet(), getInput, getResp))
					assert.True(t, getResp.Successful())
					assert.NotZero(t, getResp.Logger)
				},
				"LenSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context) {
					_ = createCachedLoggerFromCLI(t, c, "id")

					resp := &LoggingCacheLenResponse{}
					require.NoError(t, execCLICommandOutput(t, c, loggingCacheLen(), resp))
					require.True(t, resp.Successful())
					assert.Equal(t, 1, resp.Length)
				},
				"PruneSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context) {
					_ = createCachedLoggerFromCLI(t, c, "id")

					lenResp := &LoggingCacheLenResponse{}
					require.NoError(t, execCLICommandOutput(t, c, loggingCacheLen(), lenResp))
					require.True(t, lenResp.Successful())
					assert.Equal(t, 1, lenResp.Length)

					input, err := json.Marshal(LoggingCachePruneInput{LastAccessed: time.Now().Add(time.Hour)})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, loggingCachePrune(), input, resp))
					assert.True(t, resp.Successful())

					lenResp = &LoggingCacheLenResponse{}
					require.NoError(t, execCLICommandOutput(t, c, loggingCacheLen(), lenResp))
					require.True(t, lenResp.Successful())
					assert.Zero(t, lenResp.Length)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
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

					testCase(ctx, t, c)
				})
			}
		})
	}
}

// validLoggingCacheOptions returns valid options for creating a cached logger.
func validLoggingCacheOptions(t *testing.T) options.Output {
	logger, err := jasper.NewInMemoryLogger(100)
	require.NoError(t, err)
	return options.Output{
		Loggers: []*options.LoggerConfig{logger},
	}
}

// createCachedLoggerFromCLI creates a cached logger on a remote service using
// the CLI.
func createCachedLoggerFromCLI(t *testing.T, c *cli.Context, id string) options.CachedLogger {
	input, err := json.Marshal(LoggingCacheCreateInput{
		ID:     id,
		Output: validLoggingCacheOptions(t),
	})
	require.NoError(t, err)
	resp := &CachedLoggerResponse{}
	require.NoError(t, execCLICommandInputOutput(t, c, loggingCacheCreate(), input, resp))
	require.True(t, resp.Successful())
	require.NotZero(t, resp.Logger.ManagerID)
	require.Equal(t, id, resp.Logger.ID)

	return resp.Logger
}
