package cli

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHLoggingCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager){
		"CreatePassesWithValidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &LoggingCacheCreateInput{}
			resp := &CachedLoggerResponse{
				OutcomeResponse: *makeOutcomeResponse(nil),
				Logger: options.CachedLogger{
					ID:        "id",
					ManagerID: "manager_id",
					Accessed:  time.Now(),
				},
			}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheCreateCommand},
				inputChecker,
				resp,
			)

			opts := validLoggingCacheOptions(t)
			logger, err := lc.Create(resp.Logger.ID, &opts)
			require.NoError(t, err)
			assert.Equal(t, resp.Logger.ID, logger.ID)
			assert.Equal(t, resp.Logger.ManagerID, logger.ManagerID)
		},
		"CreateFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheCreateCommand},
				nil,
				invalidResponse(),
			)

			opts := validLoggingCacheOptions(t)
			logger, err := lc.Create("id", &opts)
			assert.Error(t, err)
			assert.Zero(t, logger)
		},
		"GetPassesWithValidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			resp := &CachedLoggerResponse{
				OutcomeResponse: *makeOutcomeResponse(nil),
				Logger: options.CachedLogger{
					ID:        "id",
					ManagerID: "manager_id",
				},
			}
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheGetCommand},
				inputChecker,
				resp,
			)

			logger := lc.Get(resp.Logger.ID)
			assert.Equal(t, resp.Logger.ID, logger.ID)
			assert.Equal(t, resp.Logger.ManagerID, logger.ManagerID)
		},
		"GetFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheGetCommand},
				nil,
				invalidResponse(),
			)

			logger := lc.Get("foo")
			assert.Zero(t, logger)
		},
		"RemovePasses": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheRemoveCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			lc.Remove("foo")
		},
		"CloseAndRemovePasses": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheCloseAndRemoveCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			assert.NoError(t, lc.CloseAndRemove(ctx, "foo"))
		},
		"CloseAndRemoveFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheCloseAndRemoveCommand},
				inputChecker,
				invalidResponse(),
			)

			assert.Error(t, lc.CloseAndRemove(ctx, "foo"))
		},
		"ClearPasses": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheCloseAndRemoveCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			assert.NoError(t, lc.Clear(ctx))
		},
		"ClearFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheCloseAndRemoveCommand},
				inputChecker,
				invalidResponse(),
			)

			assert.Error(t, lc.Clear(ctx))
		},
		"PrunePasses": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &LoggingCachePruneInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCachePruneCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			lc.Prune(time.Now())
		},
		"LenPassesWithValidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			resp := &LoggingCacheLenResponse{
				OutcomeResponse: *makeOutcomeResponse(nil),
				Length:          50,
			}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheLenCommand},
				nil,
				resp,
			)

			assert.Equal(t, resp.Length, lc.Len())
		},
		"LenFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, lc *sshLoggingCache, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{LoggingCacheCommand, LoggingCacheLenCommand},
				nil,
				invalidResponse(),
			)

			assert.Equal(t, -1, lc.Len())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			client, err := NewSSHClient(mockClientOptions(), mockRemoteOptions())
			require.NoError(t, err)
			sshClient, ok := client.(*sshClient)
			require.True(t, ok)

			mockManager := &mock.Manager{}
			sshClient.client.manager = jasper.Manager(mockManager)

			tctx, cancel := context.WithTimeout(ctx, testutil.TestTimeout)
			defer cancel()

			lc := newSSHLoggingCache(ctx, sshClient.client)
			require.NotNil(t, lc)

			testCase(tctx, t, lc, sshClient, mockManager)
		})
	}
}
