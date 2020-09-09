package jasper

import (
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
	} {
		t.Run(test.Name, func(t *testing.T) {
			require.NotPanics(t, func() {
				test.Case(t, NewLoggingCache())
			})
		})
	}
}
