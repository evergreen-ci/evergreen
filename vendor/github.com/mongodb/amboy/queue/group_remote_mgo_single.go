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
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type remoteMgoQueueGroupSingle struct {
	canceler context.CancelFunc
	session  *mgo.Session
	mu       sync.RWMutex
	opts     RemoteQueueGroupOptions
	dbOpts   MongoDBOptions
	queues   map[string]amboy.Queue
}

// NewMgoRemoteSingleQueueGroup constructs a new remote queue group
// where all queues are stored in a single collection, using the
// legacy driver. If ttl is 0, the queues will not be
// TTLed except when the client explicitly calls Prune.
func NewMgoRemoteSingleQueueGroup(ctx context.Context, opts RemoteQueueGroupOptions, session *mgo.Session, mdbopts MongoDBOptions) (amboy.QueueGroup, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid remote queue options")
	}

	if mdbopts.DB == "" {
		return nil, errors.New("no database name specified")
	}

	if mdbopts.URI == "" {
		return nil, errors.New("no mongodb uri specified")
	}

	ctx, cancel := context.WithCancel(ctx)
	g := &remoteMgoQueueGroupSingle{
		canceler: cancel,
		session:  session,
		dbOpts:   mdbopts,
		opts:     opts,
		queues:   map[string]amboy.Queue{},
	}

	if opts.PruneFrequency > 0 {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic in remote queue group ticker")
			pruneCtx, pruneCancel := context.WithCancel(context.Background())
			defer pruneCancel()
			ticker := time.NewTicker(opts.PruneFrequency)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					grip.Error(message.WrapError(g.Prune(pruneCtx), "problem pruning remote queue group database"))
				}
			}
		}()
	}

	if opts.BackgroundCreateFrequency > 0 {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic in remote queue group ticker")
			ticker := time.NewTicker(opts.BackgroundCreateFrequency)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					grip.Error(message.WrapError(g.startQueues(ctx), "problem starting external queues"))
				}
			}
		}()
	}

	if err := g.startQueues(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	return g, nil
}

func (g *remoteMgoQueueGroupSingle) startQueues(ctx context.Context) error {
	session := g.session.Clone()
	defer session.Close()

	out := []struct {
		Groups []string `bson:"groups"`
	}{}

	err := session.DB(g.dbOpts.DB).C(addGroupSufix(g.opts.Prefix)).Pipe([]bson.M{
		{
			"$match": bson.M{
				"$or": []bson.M{
					{
						"status.completed": false,
					},
					{
						"status.completed": true,
						"status.mod_ts":    bson.M{"$gte": time.Now().Add(-g.opts.TTL)},
					},
				},
			},
		},
		{

			"$group": bson.M{
				"_id": nil,
				"groups": bson.M{
					"$addToSet": "$group",
				},
			},
		},
	}).AllowDiskUse().All(&out)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(out) == 0 {
		return nil
	}

	if len(out) != 1 {
		return errors.New("invalid inventory of existing queues")
	}

	catcher := grip.NewBasicCatcher()
	for _, id := range out[0].Groups {
		_, err := g.Get(ctx, id)
		catcher.Add(err)
	}

	return catcher.Resolve()
}

func (g *remoteMgoQueueGroupSingle) Get(ctx context.Context, id string) (amboy.Queue, error) {
	g.mu.RLock()
	if queue, ok := g.queues[id]; ok {
		g.mu.RUnlock()
		return queue, nil
	}
	g.mu.RUnlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	// Check again in case the map was modified after we released the read lock.
	if queue, ok := g.queues[id]; ok {
		return queue, nil
	}

	driver, err := OpenNewMgoGroupDriver(ctx, g.opts.Prefix, g.dbOpts, id, g.session.Clone())
	if err != nil {
		return nil, errors.Wrap(err, "problem opening driver for queue")
	}

	queue := g.opts.constructor(ctx, id)
	if err := queue.SetDriver(driver); err != nil {
		return nil, errors.Wrap(err, "problem setting driver")

	}
	g.queues[id] = queue

	if err := queue.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}

	return queue, nil
}

func (g *remoteMgoQueueGroupSingle) Put(ctx context.Context, name string, queue amboy.Queue) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.queues[name]; ok {
		return errors.New("cannot put a queue into group with existing name")
	}

	g.queues[name] = queue
	return nil
}

func (g *remoteMgoQueueGroupSingle) Prune(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Minute)
	defer cancel()

	for name, queue := range g.queues {
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		if queue.Stats().IsComplete() {
			queue.Runner().Close(ctx)
			delete(g.queues, name)
		}
	}
	return nil
}

func (g *remoteMgoQueueGroupSingle) Close(ctx context.Context) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for name, queue := range g.queues {
		if err := ctx.Err(); err != nil {
			return
		}
		queue.Runner().Close(ctx)
		delete(g.queues, name)
	}
	g.canceler()
}
