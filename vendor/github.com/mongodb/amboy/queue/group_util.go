package queue

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// GroupCache provides a common mechanism for managing collections of
// queues, for use in specific group cache situations
type GroupCache interface {
	Set(string, amboy.Queue, time.Duration) error
	Get(string) amboy.Queue
	Remove(context.Context, string) error
	Prune(context.Context) error
	Close(context.Context) error
	Names() []string
	Len() int
}

// NewGroupCache produces a GroupCache implementation that supports a
// default TTL setting, and supports cloning and closing operations.
func NewGroupCache(ttl time.Duration) GroupCache {
	return &cacheImpl{
		ttl:  ttl,
		mu:   &sync.RWMutex{},
		q:    map[string]cacheItem{},
		hook: func(_ context.Context, _ string) error { return nil },
	}
}

// NewCacheWithCleanupHook defines a cache but allows implementations
// to add additional cleanup logic to the prune and Close operations.
func NewCacheWithCleanupHook(ttl time.Duration, hook func(ctx context.Context, id string) error) GroupCache {
	return &cacheImpl{
		ttl:  ttl,
		hook: hook,
		mu:   &sync.RWMutex{},
		q:    map[string]cacheItem{},
	}
}

type cacheImpl struct {
	ttl  time.Duration
	mu   *sync.RWMutex
	q    map[string]cacheItem
	hook func(ctx context.Context, id string) error
}

type cacheItem struct {
	q    amboy.Queue
	ttl  time.Duration
	ts   time.Time
	name string
}

func (c *cacheImpl) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.q)
}

func (c *cacheImpl) Names() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]string, len(c.q))
	idx := 0
	for name := range c.q {
		out[idx] = name
	}

	return out
}

func (c *cacheImpl) Set(name string, q amboy.Queue, ttl time.Duration) error {
	if q == nil {
		return errors.New("cannot cache nil queue")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.q[name]; ok {
		return errors.Errorf("queue named '%s' is already cached", name)
	}

	if ttl <= 0 {
		ttl = c.ttl
	}

	c.q[name] = cacheItem{
		q:    q,
		ttl:  ttl,
		ts:   time.Now(),
		name: name,
	}

	return nil
}

func (c *cacheImpl) Get(name string) amboy.Queue {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.q[name]; !ok {
		return nil
	}

	item := c.q[name]
	item.ts = time.Now()
	c.q[name] = item

	return item.q
}

func (c *cacheImpl) Remove(ctx context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.q[name]; !ok {
		return nil
	}

	queue := c.q[name].q
	if !queue.Stats(ctx).IsComplete() {
		return errors.Errorf("cannot delete in progress queue, '%s'", name)
	}

	queue.Runner().Close(ctx)
	delete(c.q, name)
	return nil
}

func (c *cacheImpl) getCacheIterUnsafe() <-chan *cacheItem {
	out := make(chan *cacheItem, len(c.q))
	for _, v := range c.q {
		out <- &v
	}
	close(out)
	return out
}

func (c *cacheImpl) getCacheIterSafe() <-chan *cacheItem {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.getCacheIterUnsafe()
}

func (c *cacheImpl) Prune(ctx context.Context) error {
	wg := &sync.WaitGroup{}
	num := runtime.NumCPU()
	catcher := grip.NewBasicCatcher()
	work := c.getCacheIterSafe()
	wg.Add(num)

	for i := 0; i < num; i++ {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic in local queue pruning")
			defer wg.Done()
		outer:
			for {
				select {
				case <-ctx.Done():
					return
				case item := <-work:
					if item == nil {
						return
					}

					if item.ttl > 0 && item.ttl > time.Since(item.ts) {
						continue outer
					}

					if item.q.Stats(ctx).IsComplete() {
						wait := make(chan struct{})
						go func() {
							defer recovery.LogStackTraceAndContinue("panic in queue waiting")
							defer close(wait)

							item.q.Runner().Close(ctx)
							c.mu.Lock()
							defer c.mu.Unlock()
							catcher.Add(c.hook(ctx, item.name))
							delete(c.q, item.name)
						}()
						select {
						case <-ctx.Done():
							return
						case <-wait:
							continue
						}
					}
				}
			}
		}()
	}
	wg.Wait()
	catcher.Add(ctx.Err())
	return catcher.Resolve()
}

func (c *cacheImpl) Close(ctx context.Context) error {
	wg := &sync.WaitGroup{}
	num := runtime.NumCPU()
	wg.Add(num)

	c.mu.Lock()
	defer c.mu.Unlock()

	work := c.getCacheIterUnsafe()

	for i := 0; i < num; i++ {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic in local queue closing")
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item := <-work:
					if item == nil {
						return
					}

					wait := make(chan struct{})
					go func() {
						defer recovery.LogStackTraceAndContinue("panic in queue waiting")
						defer close(wait)
						item.q.Runner().Close(ctx)
					}()
					select {
					case <-ctx.Done():
						return
					case <-wait:
						continue
					}
				}
			}
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return errors.WithStack(err)
	}

	c.q = map[string]cacheItem{}

	return nil
}
