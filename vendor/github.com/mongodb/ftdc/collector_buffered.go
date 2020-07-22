package ftdc

import (
	"context"

	"github.com/mongodb/ftdc/util"
)

type bufferedCollector struct {
	Collector
	pipe    chan interface{}
	catcher util.Catcher
	ctx     context.Context
}

// NewBufferedCollector wraps an existing collector with a buffer to
// normalize throughput to an underlying collector implementation.
func NewBufferedCollector(ctx context.Context, size int, coll Collector) Collector {
	c := &bufferedCollector{
		Collector: coll,
		pipe:      make(chan interface{}, size),
		catcher:   util.NewCatcher(),
		ctx:       ctx,
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				close(c.pipe)
				if len(c.pipe) != 0 {
					for in := range c.pipe {
						c.catcher.Add(c.Collector.Add(in))
					}
				}

				return
			case in := <-c.pipe:
				c.catcher.Add(c.Collector.Add(in))
			}
		}
	}()
	return c
}

func (c *bufferedCollector) Add(in interface{}) error {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case c.pipe <- in:
		return nil
	}
}

func (c *bufferedCollector) Resolve() ([]byte, error) {
	if c.catcher.HasErrors() {
		return nil, c.catcher.Resolve()
	}

	return c.Collector.Resolve()
}
