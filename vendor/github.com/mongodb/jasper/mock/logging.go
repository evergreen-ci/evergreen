package mock

import (
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

type LoggingCache struct {
	Cache                map[string]*options.CachedLogger
	PutError             error
	AllowPutOverwrite    bool
	AllowCreateOverwrite bool
	CreateError          error
}

func (c *LoggingCache) Create(id string, opts *options.Output) (*options.CachedLogger, error) {
	if c.CreateError != nil {
		return nil, c.CreateError
	}

	if !c.AllowCreateOverwrite {
		if _, ok := c.Cache[id]; ok {
			return nil, errors.New("duplicate error")
		}
	}

	c.Cache[id] = opts.CachedLogger(id)
	return c.Cache[id], nil
}

func (c *LoggingCache) Prune(lastAccessed time.Time) {
	for k, v := range c.Cache {
		if v.Accessed.Before(lastAccessed) {
			delete(c.Cache, k)
		}
	}
}

func (c *LoggingCache) Put(id string, logger *options.CachedLogger) error {
	if c.PutError != nil {
		return c.PutError
	}

	if !c.AllowPutOverwrite {
		if _, ok := c.Cache[id]; ok {
			return errors.New("duplicate error")
		}
	}

	c.Cache[id] = logger
	return nil
}

func (c *LoggingCache) Get(id string) *options.CachedLogger { return c.Cache[id] }
func (c *LoggingCache) Remove(id string)                    { delete(c.Cache, id) }
func (c *LoggingCache) Len() int                            { return len(c.Cache) }
