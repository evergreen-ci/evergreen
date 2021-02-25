package jasper

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingCacheImplementation(t *testing.T) {
	for _, test := range []struct {
		Name string
		Case func(*testing.T, LoggingCache)
	}{
		{
			Name: "LenSucceedsWithNoLoggers",
			Case: func(t *testing.T, lc LoggingCache) {
				length, err := lc.Len()
				require.NoError(t, err)
				assert.Zero(t, length)
			},
		},
		{
			Name: "PutFollowedByGetSucceeds",
			Case: func(t *testing.T, lc LoggingCache) {
				assert.NoError(t, lc.Put("id", &options.CachedLogger{ID: "id"}))

				length, err := lc.Len()
				require.NoError(t, err)
				assert.Equal(t, 1, length)

				logger, err := lc.Get("id")
				require.NoError(t, err)
				require.NotNil(t, logger)
				assert.Equal(t, "id", logger.ID)
			},
		},
		{
			Name: "DuplicateCreateFails",
			Case: func(t *testing.T, lc LoggingCache) {
				logger, err := lc.Create("id", &options.Output{})
				require.NoError(t, err)
				require.NotNil(t, logger)

				logger, err = lc.Create("id", &options.Output{})
				require.Error(t, err)
				require.Nil(t, logger)
			},
		},
		{
			Name: "DuplicatePutFails",
			Case: func(t *testing.T, lc LoggingCache) {
				assert.NoError(t, lc.Put("id", &options.CachedLogger{ID: "id"}))
				assert.Error(t, lc.Put("id", &options.CachedLogger{ID: "id"}))
				length, err := lc.Len()
				require.NoError(t, err)
				assert.Equal(t, 1, length)
			},
		},
		{
			Name: "PruneSucceeds",
			Case: func(t *testing.T, lc LoggingCache) {
				assert.NoError(t, lc.Put("id", &options.CachedLogger{ID: "id"}))

				length, err := lc.Len()
				require.NoError(t, err)
				assert.Equal(t, 1, length)

				require.NoError(t, lc.Prune(time.Now().Add(-time.Minute)))

				length, err = lc.Len()
				require.NoError(t, err)
				assert.Equal(t, 1, length)

				require.NoError(t, lc.Prune(time.Now().Add(time.Minute)))

				length, err = lc.Len()
				require.NoError(t, err)
				assert.Equal(t, 0, length)
			},
		},
		{
			Name: "CreateAccessTimeIsSet",
			Case: func(t *testing.T, lc LoggingCache) {
				logger, err := lc.Create("id", &options.Output{})
				require.NoError(t, err)

				assert.True(t, time.Since(logger.Accessed) <= time.Second)
			},
		},
		{
			Name: "RemoveSucceeds",
			Case: func(t *testing.T, lc LoggingCache) {
				_, err := lc.Create("id", &options.Output{})
				require.NoError(t, err)

				require.NoError(t, lc.Remove("id"))

				_, err = lc.Get("id")
				assert.Error(t, err)
			},
		},
		{
			Name: "RemoveWithNonexistentLoggerFails",
			Case: func(t *testing.T, lc LoggingCache) {
				assert.Error(t, lc.Remove("foo"))
			},
		},
		{
			Name: "CloseAndRemoveSucceeds",
			Case: func(t *testing.T, lc LoggingCache) {
				ctx := context.Background()
				sender := send.NewMockSender("output")

				require.NoError(t, lc.Put("id0", &options.CachedLogger{
					Output: sender,
				}))
				require.NoError(t, lc.Put("id1", &options.CachedLogger{}))

				require.NoError(t, lc.CloseAndRemove(ctx, "id0"))

				logger, err := lc.Get("id0")
				require.Error(t, err)
				require.Zero(t, logger)
				require.True(t, sender.Closed)

				logger, err = lc.Get("id1")
				require.NoError(t, err)
				require.NotZero(t, logger)
			},
		},
		{
			Name: "ClearSucceedsAndCloses",
			Case: func(t *testing.T, lc LoggingCache) {
				ctx := context.Background()
				sender0 := send.NewMockSender("output")
				sender1 := send.NewMockSender("output")

				require.NoError(t, lc.Put("id0", &options.CachedLogger{
					Output: sender0,
				}))
				require.NoError(t, lc.Put("id1", &options.CachedLogger{
					Output: sender1,
				}))

				require.NoError(t, lc.Clear(ctx))

				logger1, err := lc.Get("id0")
				require.Error(t, err)
				require.Zero(t, logger1)
				logger2, err := lc.Get("id1")
				require.Error(t, err)
				require.Zero(t, logger2)
				require.True(t, sender0.Closed)
				require.True(t, sender1.Closed)
			},
		},
	} {
		t.Run(test.Name, func(t *testing.T) {
			require.NotPanics(t, func() {
				test.Case(t, NewLoggingCache())
			})
		})
	}
}
