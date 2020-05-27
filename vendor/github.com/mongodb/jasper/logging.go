package jasper

import (
	"sync"
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// LoggingCache provides an interface to a cache of loggers.
type LoggingCache interface {
	Create(id string, opts *options.Output) (*options.CachedLogger, error)
	Put(id string, logger *options.CachedLogger) error
	Get(id string) *options.CachedLogger
	Remove(id string)
	Prune(lastAccessed time.Time)
	Len() int
}

// NewLoggingCache produces a thread-safe implementation of a logging
// cache for use in manager implementations.
func NewLoggingCache() LoggingCache {
	return &loggingCacheImpl{
		cache: map[string]*options.CachedLogger{},
	}
}

type loggingCacheImpl struct {
	cache map[string]*options.CachedLogger
	mu    sync.RWMutex
}

func (c *loggingCacheImpl) Create(id string, opts *options.Output) (*options.CachedLogger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[id]; ok {
		return nil, errors.Errorf("logger named %s exists", id)
	}
	logger := opts.CachedLogger(id)

	c.cache[id] = logger

	return logger, nil
}

func (c *loggingCacheImpl) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.cache)
}

func (c *loggingCacheImpl) Prune(ts time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range c.cache {
		if v.Accessed.Before(ts) {
			delete(c.cache, k)
		}
	}
}

func (c *loggingCacheImpl) Get(id string) *options.CachedLogger {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[id]; !ok {
		return nil
	}

	item := c.cache[id]
	item.Accessed = time.Now()
	c.cache[id] = item
	return item
}

func (c *loggingCacheImpl) Put(id string, logger *options.CachedLogger) error {
	if logger == nil {
		return errors.New("cannot cache nil logger")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[id]; ok {
		return errors.Errorf("cannot cache with existing logger '%s'", id)
	}

	logger.Accessed = time.Now()

	c.cache[id] = logger

	return nil
}

func (c *loggingCacheImpl) Remove(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, id)
}
