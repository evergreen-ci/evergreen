package remote

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := testutil.GetHTTPClient()
	defer testutil.PutHTTPClient(httpClient)

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

						logger := lc.Get(expectedLogger.ID)
						require.NotNil(t, logger)
						assert.Equal(t, expectedLogger.ID, logger.ID)
					},
				},
				{
					Name: "LoggingCacheGetFailsWithNonexistentLogger",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger := lc.Get("nonexistent")
						require.Nil(t, logger)
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

						require.NotNil(t, lc.Get(logger1.ID))
						require.NotNil(t, lc.Get(logger2.ID))
						lc.Remove(logger2.ID)
						require.NotNil(t, lc.Get(logger1.ID))
						require.Nil(t, lc.Get(logger2.ID))
					},
				},
				{
					Name: "LoggingCacheRemoveNoopsForNonexistentLogger",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						_, err := lc.Create("logger", &options.Output{})
						require.NoError(t, err)
						require.Equal(t, 1, lc.Len())
						lc.Remove("nonexistent")
						assert.Equal(t, 1, lc.Len())
					},
				},
				{
					Name: "LoggingCachePruneSucceeds",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						logger1, err := lc.Create("logger1", &options.Output{})
						require.NoError(t, err)
						require.NotNil(t, lc.Get(logger1.ID))
						// We have to sleep to force this logger to be
						// pruned.
						time.Sleep(2 * time.Second)

						logger2, err := lc.Create("logger2", &options.Output{})
						require.NoError(t, err)

						lc.Prune(time.Now().Add(-time.Second))
						require.Nil(t, lc.Get(logger1.ID))
						require.NotNil(t, lc.Get(logger2.ID))
					},
				},
				{
					Name: "LoggingCachePruneNoopsWithoutLoggers",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)

						assert.Zero(t, lc.Len())
						lc.Prune(time.Now())
						assert.Zero(t, lc.Len())
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

						assert.Equal(t, 2, lc.Len())
					},
				},
				{
					Name: "LoggingCacheLenIsZeroWithoutLoggers",
					Case: func(ctx context.Context, t *testing.T, client Manager) {
						lc := client.LoggingCache(ctx)
						assert.Zero(t, lc.Len())
					},
				},
			} {
				t.Run(test.Name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
					defer cancel()
					client := makeManager(tctx, t)
					defer func() {
						assert.NoError(t, client.CloseConnection())
					}()
					test.Case(tctx, t, client)
				})
			}
		})
	}
}
