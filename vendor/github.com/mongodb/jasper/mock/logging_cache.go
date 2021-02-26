package mock

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
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

// Get returns an object from the in-memory logging cache. It returns an error
// if it does not exist in the cache.
func (c *LoggingCache) Get(id string) (*options.CachedLogger, error) {
	logger, ok := c.Cache[id]
	if !ok {
		return nil, jasper.ErrCachedLoggerNotFound
	}
	return logger, nil
}

// Remove removes an object from the in-memory logging cache. It returns an
// error if it does not exist in the cache.
func (c *LoggingCache) Remove(id string) error {
	if _, ok := c.Cache[id]; !ok {
		return jasper.ErrCachedLoggerNotFound
	}

	delete(c.Cache, id)

	return nil
}

// CloseAndRemove closes the cached logger and removes it from the in-memory
// logging cache.
func (c *LoggingCache) CloseAndRemove(_ context.Context, id string) error {
	logger, ok := c.Cache[id]
	if !ok {
		return jasper.ErrCachedLoggerNotFound
	}
	if err := logger.Close(); err != nil {
		return errors.Wrapf(err, "closing logger")
	}

	delete(c.Cache, id)

	return nil
}

// Clear closes and removes all objects in the in-memory logging cache.
func (c *LoggingCache) Clear(_ context.Context) error {
	catcher := grip.NewBasicCatcher()
	var closed []string
	for id, logger := range c.Cache {
		if err := logger.Close(); err != nil {
			catcher.Wrapf(err, "closing logger with id '%s'", id)
			continue
		}
		closed = append(closed, id)
	}
	for _, id := range closed {
		delete(c.Cache, id)
	}

	return catcher.Resolve()
}

// Prune removes all items from the cache whose most recent access time is older
// than lastAccessed.
func (c *LoggingCache) Prune(lastAccessed time.Time) error {
	catcher := grip.NewBasicCatcher()
	for id, logger := range c.Cache {
		if logger.Accessed.Before(lastAccessed) {
			catcher.Wrapf(c.CloseAndRemove(context.Background(), id), "pruning logger with id '%s'", id)
		}
	}
	return catcher.Resolve()
}

// Len returns the size of the in-memory logging cache.
func (c *LoggingCache) Len() (int, error) {
	return len(c.Cache), nil
}
