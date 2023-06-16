package cache

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestCache(t *testing.T) {
	defer func() { db.Clear(collection) }()

	cache := DBCache{}

	t.Run("GetNonexistentKey", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		val, ok := cache.Get("not_here")
		assert.False(t, ok)
		assert.Nil(t, val)
	})

	t.Run("GetExistentKey", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		key := "https://api.github.com/repos/evergreen-ci/evergreen/contents/self-tests.yml?ref=86a2e50c93a2016f1b8a717f70343c4f9aeda961"
		value := []byte("testing")
		cache.Set(key, value)
		val, ok := cache.Get(key)
		assert.Equal(t, value, val)
		assert.True(t, ok)
	})

	t.Run("DeleteKey", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		key := "https://api.github.com/repos/evergreen-ci/evergreen/contents/self-tests.yml?ref=86a2e50c93a2016f1b8a717f70343c4f9aeda961"
		value := []byte("testing")
		cache.Set(key, value)
		val, ok := cache.Get(key)
		assert.Equal(t, value, val)
		assert.True(t, ok)

		cache.Delete(key)
		val, ok = cache.Get(key)
		assert.False(t, ok)
		assert.Nil(t, val)
	})

}
