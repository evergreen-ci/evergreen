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
	tasksCache cacheContextKey = "tasks"

	lifetime = time.Second
)

var validCaches = []string{
	string(tasksCache),
}

func Embed(ctx context.Context) context.Context {
	for _, collections := range validCaches {
		if ctx.Value(cacheContextKey(collections)) != nil {
			continue
		}
		cacheName := fmt.Sprintf("db-cache-%s", collections)
		cache := ttlcache.WithOtel(ttlcache.NewWeakInMemory[any](), cacheName)
		ctx = context.WithValue(ctx, cacheContextKey(collections), cache)
	}

	return ctx
}

func GetFromCache[T any](ctx context.Context, collection, id string) (T, bool) {
	if !validCache(collection) {
		return *new(T), false
	}

	cache, ok := getCache[T](ctx, cacheContextKey(collection))
	if !ok {
		return *new(T), false
	}

	return cache.Get(ctx, id, 0)
}

func SetInCache[T any](ctx context.Context, collection, id string, value T) {
	if !validCache(collection) {
		return
	}

	cache, ok := getCache[T](ctx, cacheContextKey(collection))
	if !ok {
		return
	}

	cache.Put(ctx, id, value, time.Now().Add(lifetime))
}

func getCache[T any](ctx context.Context, collection cacheContextKey) (ttlcache.Cache[T], bool) {
	cache, ok := ctx.Value(cacheContextKey(collection)).(ttlcache.Cache[T])
	return cache, ok
}

func validCache(collection string) bool {
	for _, validCollection := range validCaches {
		if collection == validCollection {
			return true
		}
	}
	return false
}
