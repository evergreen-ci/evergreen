package remote

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	for managerName, makeManager := range remoteManagerTestCases(httpClient) {
		t.Run(managerName, func(t *testing.T) {
			for _, test := range []clientTestCase{
				{
					Name: "LoggingCacheCreateSucceeds",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger, err := lc.Create("new_logger", &options.Output{})
						require.NoError(t, err)
						assert.Equal(t, "new_logger", logger.ID)

						// should fail with existing logger
						_, err = lc.Create("new_logger", &options.Output{})
						assert.Error(t, err)
					},
				},
				{
					Name: "LoggingCacheCreateFailsWithInvalidOptions",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger, err := lc.Create("new_logger", &options.Output{
							SendErrorToOutput: true,
							SendOutputToError: true,
						})
						assert.Error(t, err)
						assert.Zero(t, logger)
					},
				},
				{
					Name: "LoggingCachePutNotImplemented",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						assert.Error(t, lc.Put("logger", &options.CachedLogger{ID: "logger"}))
					},
				},
				{
					Name: "LoggingCacheGetSucceeds",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						expectedLogger, err := lc.Create("new_logger", &options.Output{})
						require.NoError(t, err)

						logger, err := lc.Get(expectedLogger.ID)
						require.NoError(t, err)
						require.NotZero(t, logger)
						assert.Equal(t, expectedLogger.ID, logger.ID)
					},
				},
				{
					Name: "LoggingCacheGetFailsWithNonexistentLogger",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger, err := lc.Get("nonexistent")
						assert.Error(t, err)
						assert.Zero(t, logger)
					},
				},
				{
					Name: "LoggingCacheClearSucceeds",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger1, err := lc.Create("logger1", &options.Output{})
						require.NoError(t, err)

						require.NoError(t, lc.Clear(ctx))

						logger2, err := lc.Create("logger2", &options.Output{})
						require.NoError(t, err)

						cachedLogger1, err := lc.Get(logger1.ID)
						assert.Error(t, err)
						assert.Zero(t, cachedLogger1)
						cachedLogger2, err := lc.Get(logger2.ID)
						assert.NoError(t, err)
						assert.NotZero(t, cachedLogger2)
					},
				},
				{
					Name: "LoggingCacheRemoveSucceeds",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger1, err := lc.Create("logger1", &options.Output{})
						require.NoError(t, err)
						logger2, err := lc.Create("logger2", &options.Output{})
						require.NoError(t, err)

						require.NoError(t, lc.Remove(logger2.ID))
						cachedLogger1, err := lc.Get(logger1.ID)
						assert.NoError(t, err)
						assert.NotZero(t, cachedLogger1)
						cachedLogger2, err := lc.Get(logger2.ID)
						assert.Error(t, err)
						assert.Zero(t, cachedLogger2)
					},
				},
				{
					Name: "LoggingCacheRemoveWithNonexistentLoggerFails",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						_, err := lc.Create("logger", &options.Output{})
						require.NoError(t, err)
						assert.Error(t, lc.Remove("foo"))
						length, err := lc.Len()
						require.NoError(t, err)
						assert.Equal(t, 1, length)
					},
				},
				{
					Name: "LoggingCachePruneSucceeds",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger1, err := lc.Create("logger1", &options.Output{})
						require.NoError(t, err)
						// We have to sleep to force this logger to be
						// pruned.
						time.Sleep(2 * time.Second)

						logger2, err := lc.Create("logger2", &options.Output{})
						require.NoError(t, err)

						require.NoError(t, lc.Prune(time.Now().Add(-time.Second)))
						_, err = lc.Get(logger1.ID)
						assert.Error(t, err)
						_, err = lc.Get(logger2.ID)
						assert.NoError(t, err)
					},
				},
				{
					Name: "LoggingCachePruneNoopsWithoutLoggers",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)

						length, err := lc.Len()
						require.NoError(t, err)
						require.Zero(t, length)

						require.NoError(t, lc.Prune(time.Now()))

						length, err = lc.Len()
						require.NoError(t, err)
						assert.Zero(t, length)
					},
				},
				{
					Name: "LoggingCacheLenIsNonzeroWithCachedLoggers",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						_, err := lc.Create("logger1", &options.Output{})
						require.NoError(t, err)
						_, err = lc.Create("logger2", &options.Output{})
						require.NoError(t, err)

						length, err := lc.Len()
						require.NoError(t, err)
						assert.Equal(t, 2, length)
					},
				},
				{
					Name: "LoggingCacheLenIsZeroWithoutLoggers",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						length, err := lc.Len()
						require.NoError(t, err)
						assert.Zero(t, length)
					},
				},
			} {
				t.Run(test.Name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
					defer cancel()
					client := makeManager(tctx, t)
					test.Case(tctx, t, client)
				})
			}
		})
	}
}
