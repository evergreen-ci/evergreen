package cache_test

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/util/cache"
	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {
	cache := cache.New[int]()

	id, ok := cache.Get("key", time.Minute)
	assert.False(t, ok)
	assert.Zero(t, id)

	cache.Put("key", 22, time.Now().Add(time.Second))

	id, ok = cache.Get("key", time.Minute)
	assert.False(t, ok)
	assert.Zero(t, id)

	id, ok = cache.Get("key", time.Millisecond)
	assert.True(t, ok)
	assert.Equal(t, 22, id)

	cache.Put("key", 33, time.Now().Add(time.Hour))

	id, ok = cache.Get("key", time.Minute)
	assert.True(t, ok)
	assert.Equal(t, 33, id)

	id, ok = cache.Get("key", 2*time.Hour)
	assert.False(t, ok)
	assert.Zero(t, id)
}
