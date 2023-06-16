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

	t.Run("IncrementUpdated", func(t *testing.T) {
		require.NoError(t, db.Clear(collection))

		key := "https://api.github.com/repos/evergreen-ci/evergreen/contents/self-tests.yml?ref=86a2e50c93a2016f1b8a717f70343c4f9aeda961"
		value := []byte("testing")
		cache.Set(key, value)
		firstVal, ok := cache.Get(key)
		assert.Equal(t, value, firstVal)
		assert.True(t, ok)

		before := cacheItem{}
		require.NoError(t, db.FindOneQ(collection, db.Query(bson.M{IDKey: key}), &before))

		time.Sleep(100 * time.Millisecond)
		cache.Set(key, value)

		after := cacheItem{}
		require.NoError(t, db.FindOneQ(collection, db.Query(bson.M{IDKey: key}), &after))

		assert.True(t, after.Updated.After(before.Updated))
	})
}
