package mock

import (
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// LoggingCache implements the LoggingCache interfaces based on an in-memory
// logging cache with exported fields to configure and introspect the mock's
// behavior.
type LoggingCache struct {
	Cache                map[string]*options.CachedLogger
	AllowCreateOverwrite bool
	FailCreate           bool
	AllowPutOverwrite    bool
	FailPut              bool
}

// Create creates a cached logger from the given options.Output and stores it in
// the cache. If AllowCreateOverwrite is set, the newly created logger will
// overwrite an existing one with the same ID if it already exists. If
// FailCreate is set, it returns an error.
func (c *LoggingCache) Create(id string, opts *options.Output) (*options.CachedLogger, error) {
	if c.FailCreate {
		return nil, mockFail()
	}

	if !c.AllowCreateOverwrite {
		if _, ok := c.Cache[id]; ok {
			return nil, errors.New("duplicate error")
		}
	}

	c.Cache[id] = opts.CachedLogger(id)
	return c.Cache[id], nil
}

// Put adds the given cached logger with the given ID to the cache. If
// AllowPutOverwrite is set, the given logger will overwrite an existing one
// with the same ID if it already exists. If FailPut is set, it returns an
// error.
func (c *LoggingCache) Put(id string, logger *options.CachedLogger) error {
	if c.FailPut {
		return mockFail()
	}

	if !c.AllowPutOverwrite {
		if _, ok := c.Cache[id]; ok {
			return errors.New("duplicate error")
		}
	}

	c.Cache[id] = logger
	return nil
}

// Get returns an object from the in-memory logging cache.
func (c *LoggingCache) Get(id string) *options.CachedLogger { return c.Cache[id] }

// Remove removes an object from the in-memory logging cache.
func (c *LoggingCache) Remove(id string) { delete(c.Cache, id) }

// Prune removes all items from the cache whose most recent access time is older
// than lastAccessed.
func (c *LoggingCache) Prune(lastAccessed time.Time) {
	for k, v := range c.Cache {
		if v.Accessed.Before(lastAccessed) {
			delete(c.Cache, k)
		}
	}
}

// Len returns the size of the in-memory logging cache.
func (c *LoggingCache) Len() int { return len(c.Cache) }
