package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/utility/ttlcache"
)

type (
	// Using a custom type to avoid collisions with other context keys.
	cacheContextKey string
)

const (
	tasksCache    cacheContextKey = "tasks"
	versionsCache cacheContextKey = "versions"
	projectsCache cacheContextKey = "projects"
	patchesCache  cacheContextKey = "patches"

	lifetime = time.Second
)

var validCaches = []cacheContextKey{
	tasksCache,
	versionsCache,
	projectsCache,
	patchesCache,
}

func Embed(ctx context.Context, namePrefix string) context.Context {
	for _, collections := range validCaches {
		if ctx.Value(cacheContextKey(collections)) != nil {
			continue
		}
		cacheName := fmt.Sprintf("%s-db-cache-%s", namePrefix, collections)
		cache := ttlcache.WithOtel(ttlcache.NewWeakInMemory[any](), cacheName)
		ctx = context.WithValue(ctx, cacheContextKey(collections), cache)
	}

	return ctx
}

func GetFromCache[T any](ctx context.Context, collection, id string) (T, bool) {
	cache, ok := getCache[T](ctx, cacheContextKey(collection))
	if !ok {
		return *new(T), false
	}

	return cache.Get(ctx, id, 0)
}

func SetInCache[T any](ctx context.Context, collection, id string, value T) {
	cache, ok := getCache[T](ctx, cacheContextKey(collection))
	if !ok {
		return
	}

	cache.Put(ctx, id, value, time.Now().Add(lifetime))
}

func getCache[T any](ctx context.Context, collection cacheContextKey) (ttlcache.Cache[T], bool) {
	if !validCache(collection) {
		return nil, false
	}

	cache, ok := ctx.Value(cacheContextKey(collection)).(ttlcache.Cache[T])
	return cache, ok
}

func validCache(collection cacheContextKey) bool {
	for _, validCollection := range validCaches {
		if collection == validCollection {
			return true
		}
	}
	return false
}
