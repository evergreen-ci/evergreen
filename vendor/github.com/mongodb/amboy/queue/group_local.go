package queue

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// Constructor is a function passed by the client which makes a new queue for a QueueGroup.
type Constructor func(ctx context.Context) (amboy.Queue, error)

// localQueueGroup is a group of in-memory queues.
type localQueueGroup struct {
	mu          sync.RWMutex
	canceler    context.CancelFunc
	queues      map[string]amboy.Queue
	constructor Constructor
	ttlMap      map[string]time.Time
	ttl         time.Duration
}

// LocalQueueGroupOptions describe options passed to NewLocalQueueGroup.
type LocalQueueGroupOptions struct {
	Constructor Constructor
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
	ctx, cancel := context.WithCancel(ctx)
	g := &localQueueGroup{
		canceler:    cancel,
		queues:      map[string]amboy.Queue{},
		constructor: opts.Constructor,
		ttlMap:      map[string]time.Time{},
		ttl:         opts.TTL,
	}
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

// Get a queue with the given index. Get sets the last accessed time to now. Note that this means
// that the caller must add a job to the queue within the TTL, or else it may have attempted to add
// a job to a closed queue.
func (g *localQueueGroup) Get(ctx context.Context, id string) (amboy.Queue, error) {
	g.mu.RLock()
	if queue, ok := g.queues[id]; ok {
		g.ttlMap[id] = time.Now()
		g.mu.RUnlock()
		return queue, nil
	}
	g.mu.RUnlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	// Check again in case the map was modified after we released the read lock.
	if queue, ok := g.queues[id]; ok {
		g.ttlMap[id] = time.Now()
		return queue, nil
	}

	queue, err := g.constructor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}
	if err := queue.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}
	g.queues[id] = queue
	g.ttlMap[id] = time.Now()
	return queue, nil
}

// Put a queue at the given index.
func (g *localQueueGroup) Put(ctx context.Context, id string, queue amboy.Queue) error {
	g.mu.RLock()
	if _, ok := g.queues[id]; ok {
		g.mu.RUnlock()
		return errors.New("a queue already exists at this index")
	}
	g.mu.RUnlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	// Check again in case the map was modified after we released the read lock.
	if _, ok := g.queues[id]; ok {
		return errors.New("a queue already exists at this index")
	}

	g.queues[id] = queue
	g.ttlMap[id] = time.Now()
	return nil
}

// Prune old queues.
func (g *localQueueGroup) Prune(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	queues := make([]amboy.Queue, 0, len(g.ttlMap))
	for queueID, t := range g.ttlMap {
		if time.Since(t) > g.ttl {
			if q, ok := g.queues[queueID]; ok {
				delete(g.queues, queueID)
				queues = append(queues, q)
			}
			delete(g.ttlMap, queueID)
		}
	}
	wg := &sync.WaitGroup{}
	for _, queue := range queues {
		wg.Add(1)
		go func(queue amboy.Queue) {
			queue.Runner().Close(ctx)
			wg.Done()
		}(queue)
	}
	wg.Wait()
	return nil
}

// Close the queues.
func (g *localQueueGroup) Close(ctx context.Context) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.canceler()
	waitCh := make(chan struct{})
	wg := &sync.WaitGroup{}
	go func() {
		for _, queue := range g.queues {
			wg.Add(1)
			go func(queue amboy.Queue) {
				defer recovery.LogStackTraceAndContinue("panic in local queue group closer")
				defer wg.Done()
				queue.Runner().Close(ctx)
			}(queue)
		}
		wg.Wait()
		close(waitCh)
	}()
	select {
	case <-waitCh:
		return
	case <-ctx.Done():
		return
	}
}
