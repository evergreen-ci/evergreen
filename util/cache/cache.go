package cache

import (
	"sync"
	"time"
)

type cachedValue[T any] struct {
	value     T
	expiresAt time.Time
}

func (c *cachedValue[T]) isExpired(lifetime time.Duration) bool {
	return time.Until(c.expiresAt) < lifetime
}

type cache[T any] struct {
	cache map[string]cachedValue[T]
	mu    sync.RWMutex
}

// New creates a new in-memory cache.
func New[T any]() *cache[T] {
	return &cache[T]{
		cache: make(map[string]cachedValue[T]),
		mu:    sync.RWMutex{},
	}
}

// Get returns the value for the given key if it exists and a boolean indicating
// if the value was found.
func (c *cache[T]) Get(id string, lifetime time.Duration) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cachedToken, ok := c.cache[id]
	if !ok {
		var value T
		return value, false
	}
	if cachedToken.isExpired(lifetime) {
		var value T
		return value, false
	}

	return cachedToken.value, true
}

// Put adds a value to the cache with the given expiration time.
func (c *cache[T]) Put(id string, value T, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[id] = cachedValue[T]{
		value:     value,
		expiresAt: expiresAt,
	}
}
