package cache

import (
	"context"
	"fmt"
	"slices"
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
		var cache ttlcache.Cache[any] = ttlcache.WithOtel(ttlcache.NewWeakInMemory[any](), cacheName)
		ctx = context.WithValue(ctx, cacheContextKey(collections), cache)
	}

	return ctx
}

func GetFromCache(ctx context.Context, collection, id string) (any, bool) {
	cache, ok := getCache(ctx, cacheContextKey(collection))
	if !ok {
		return *new(any), false
	}

	return cache.Get(ctx, id, 0)
}

func SetInCache(ctx context.Context, collection, id string, value any) {
	cache, ok := getCache(ctx, cacheContextKey(collection))
	if !ok {
		return
	}

	cache.Put(ctx, id, value, time.Now().Add(lifetime))
}

func getCache(ctx context.Context, collection cacheContextKey) (ttlcache.Cache[any], bool) {
	if !validCache(collection) {
		return nil, false
	}

	cache, ok := ctx.Value(collection).(ttlcache.Cache[any])
	return cache, ok
}

func validCache(collection cacheContextKey) bool {
	return slices.Contains(validCaches, collection)
}
