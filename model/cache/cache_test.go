package cache

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestCache(t *testing.T) {
	defer func() { assert.NoError(t, db.Clear(collection)) }()

	cache := DBCache{}

	t.Run("GetNonexistentKey", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		val, ok, err := cache.Get(t.Context(), "not_here")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Nil(t, val)
	})

	t.Run("GetExistentKey", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		key := "https://api.github.com/repos/evergreen-ci/evergreen/contents/self-tests.yml?ref=86a2e50c93a2016f1b8a717f70343c4f9aeda961"
		value := []byte("testing")
		require.NoError(t, cache.Set(t.Context(), key, value))
		val, ok, err := cache.Get(t.Context(), key)
		require.NoError(t, err)
		assert.Equal(t, value, val)
		assert.True(t, ok)
	})

	t.Run("DeleteKey", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		key := "https://api.github.com/repos/evergreen-ci/evergreen/contents/self-tests.yml?ref=86a2e50c93a2016f1b8a717f70343c4f9aeda961"
		value := []byte("testing")
		require.NoError(t, cache.Set(t.Context(), key, value))
		val, ok, err := cache.Get(t.Context(), key)
		require.NoError(t, err)
		assert.Equal(t, value, val)
		assert.True(t, ok)

		require.NoError(t, cache.Delete(t.Context(), key))
		val, ok, err = cache.Get(t.Context(), key)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Nil(t, val)
	})

	t.Run("IncrementUpdated", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		key := "https://api.github.com/repos/evergreen-ci/evergreen/contents/self-tests.yml?ref=86a2e50c93a2016f1b8a717f70343c4f9aeda961"
		value := []byte("testing")
		require.NoError(t, cache.Set(t.Context(), key, value))
		firstVal, ok, err := cache.Get(t.Context(), key)
		require.NoError(t, err)
		assert.Equal(t, value, firstVal)
		assert.True(t, ok)

		before := cacheItem{}
		require.NoError(t, db.FindOneQContext(t.Context(), collection, db.Query(bson.M{IDKey: key}), &before))

		time.Sleep(100 * time.Millisecond)
		require.NoError(t, cache.Set(t.Context(), key, value))

		after := cacheItem{}
		require.NoError(t, db.FindOneQContext(t.Context(), collection, db.Query(bson.M{IDKey: key}), &after))

		assert.True(t, after.Updated.After(before.Updated))
	})
}
