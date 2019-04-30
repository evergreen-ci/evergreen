package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// localQueueGroup is a group of in-memory queues.
type localQueueGroup struct {
	canceler context.CancelFunc
	opts     LocalQueueGroupOptions
	cache    GroupCache
}

// LocalQueueGroupOptions describe options passed to NewLocalQueueGroup.
type LocalQueueGroupOptions struct {
	Constructor func(ctx context.Context) (amboy.Queue, error)
	TTL         time.Duration
}

// NewLocalQueueGroup constructs a new local queue group. If ttl is 0, the queues will not be
// TTLed except when the client explicitly calls Prune.
func NewLocalQueueGroup(ctx context.Context, opts LocalQueueGroupOptions) (amboy.QueueGroup, error) {
	if opts.Constructor == nil {
		return nil, errors.New("must pass a constructor")
	}
	if opts.TTL < 0 {
		return nil, errors.New("ttl must be greater than or equal to 0")
	}
	if opts.TTL > 0 && opts.TTL < time.Second {
		return nil, errors.New("ttl cannot be less than 1 second, unless it is 0")
	}
	g := &localQueueGroup{
		opts:  opts,
		cache: NewGroupCache(opts.TTL),
	}
	ctx, g.canceler = context.WithCancel(ctx)

	if opts.TTL > 0 {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic in local queue group ticker")
			ticker := time.NewTicker(opts.TTL)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					grip.Error(message.WrapError(g.Prune(ctx),
						message.Fields{
							"group": "local queue group background pruning",
							"ttl":   opts.TTL,
						}))
				}
			}
		}()
	}
	return g, nil
}

func (g *localQueueGroup) Len() int { return g.cache.Len() }

func (g *localQueueGroup) Queues(_ context.Context) []string {
	return g.cache.Names()
}

// Get a queue with the given index. Get sets the last accessed time to now. Note that this means
// that the caller must add a job to the queue within the TTL, or else it may have attempted to add
// a job to a closed queue.
func (g *localQueueGroup) Get(ctx context.Context, id string) (amboy.Queue, error) {
	q := g.cache.Get(id)
	if q != nil {
		return q, nil
	}

	queue, err := g.opts.Constructor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}

	if err = g.cache.Set(id, queue, g.opts.TTL); err != nil {
		// safe to throw away the partially constructed
		// here, because another won and we  haven't started the workers.
		if q := g.cache.Get(id); q != nil {
			return q, nil
		}

		return nil, errors.Wrap(err, "problem caching queue")
	}

	if err = queue.Start(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	return queue, nil
}

// Put a queue at the given index.
func (g *localQueueGroup) Put(ctx context.Context, id string, queue amboy.Queue) error {
	return errors.WithStack(g.cache.Set(id, queue, g.opts.TTL))
}

// Prune old queues.
func (g *localQueueGroup) Prune(ctx context.Context) error { return g.cache.Prune(ctx) }

// Close the queues.
func (g *localQueueGroup) Close(ctx context.Context) error { return g.cache.Close(ctx) }
