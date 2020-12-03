package jasper

import (
	"context"
	"testing"
	"time"

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
			Name: "Fixture",
			Case: func(t *testing.T, cache LoggingCache) {
				assert.Equal(t, 0, cache.Len())
			},
		},
		{
			Name: "SafeOps",
			Case: func(t *testing.T, cache LoggingCache) {
				cache.Remove("")
				cache.Remove("what")
				assert.Nil(t, cache.Get("whatever"))
				assert.Error(t, cache.Put("foo", nil))
				assert.Equal(t, 0, cache.Len())
			},
		},
		{
			Name: "PutGet",
			Case: func(t *testing.T, cache LoggingCache) {
				assert.NoError(t, cache.Put("id", &options.CachedLogger{ID: "id"}))
				assert.Equal(t, 1, cache.Len())
				lg := cache.Get("id")
				require.NotNil(t, lg)
				assert.Equal(t, "id", lg.ID)
			},
		},
		{
			Name: "PutDuplicate",
			Case: func(t *testing.T, cache LoggingCache) {
				assert.NoError(t, cache.Put("id", &options.CachedLogger{ID: "id"}))
				assert.Error(t, cache.Put("id", &options.CachedLogger{ID: "id"}))
				assert.Equal(t, 1, cache.Len())
			},
		},
		{
			Name: "Prune",
			Case: func(t *testing.T, cache LoggingCache) {
				assert.NoError(t, cache.Put("id", &options.CachedLogger{ID: "id"}))
				assert.Equal(t, 1, cache.Len())
				cache.Prune(time.Now().Add(-time.Minute))
				assert.Equal(t, 1, cache.Len())
				cache.Prune(time.Now().Add(time.Minute))
				assert.Equal(t, 0, cache.Len())
			},
		},
		{
			Name: "CreateDuplicateProtection",
			Case: func(t *testing.T, cache LoggingCache) {
				cl, err := cache.Create("id", &options.Output{})
				require.NoError(t, err)
				require.NotNil(t, cl)

				cl, err = cache.Create("id", &options.Output{})
				require.Error(t, err)
				require.Nil(t, cl)
			},
		},
		{
			Name: "CreateAccessTime",
			Case: func(t *testing.T, cache LoggingCache) {
				cl, err := cache.Create("id", &options.Output{})
				require.NoError(t, err)

				assert.True(t, time.Since(cl.Accessed) <= time.Millisecond)
			},
		},
		{
			Name: "CloseAndRemove",
			Case: func(t *testing.T, cache LoggingCache) {
				ctx := context.TODO()
				sender := options.NewMockSender("output")

				require.NoError(t, cache.Put("id0", &options.CachedLogger{
					Output: sender,
				}))
				require.NoError(t, cache.Put("id1", &options.CachedLogger{}))
				require.NotNil(t, cache.Get("id0"))
				require.NoError(t, cache.CloseAndRemove(ctx, "id0"))
				require.Nil(t, cache.Get("id0"))
				assert.NotNil(t, cache.Get("id1"))
				require.True(t, sender.Closed)

				require.NoError(t, cache.Put("id0", &options.CachedLogger{
					Output: sender,
				}))
				require.NotNil(t, cache.Get("id0"))
				assert.Error(t, cache.CloseAndRemove(ctx, "id0"))
				require.Nil(t, cache.Get("id0"))
			},
		},
		{
			Name: "Clear",
			Case: func(t *testing.T, cache LoggingCache) {
				ctx := context.TODO()
				sender0 := options.NewMockSender("output")
				sender1 := options.NewMockSender("output")

				require.NoError(t, cache.Put("id0", &options.CachedLogger{
					Output: sender0,
				}))
				require.NoError(t, cache.Put("id1", &options.CachedLogger{
					Output: sender1,
				}))
				require.NotNil(t, cache.Get("id0"))
				require.NotNil(t, cache.Get("id1"))
				require.NoError(t, cache.Clear(ctx))
				require.Nil(t, cache.Get("id0"))
				require.Nil(t, cache.Get("id1"))
				require.True(t, sender0.Closed)
				require.True(t, sender1.Closed)

				require.NoError(t, cache.Put("id0", &options.CachedLogger{
					Output: sender0,
				}))
				require.NoError(t, cache.Put("id1", &options.CachedLogger{
					Output: sender1,
				}))
				require.NotNil(t, cache.Get("id0"))
				require.NotNil(t, cache.Get("id1"))
				assert.Error(t, cache.Clear(ctx))
				assert.Nil(t, cache.Get("id0"))
				assert.Nil(t, cache.Get("id1"))
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
