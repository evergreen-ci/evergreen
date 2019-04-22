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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type remoteMongoQueueGroupSingle struct {
	canceler context.CancelFunc
	client   *mongo.Client
	mu       sync.RWMutex
	opts     RemoteQueueGroupOptions
	dbOpts   MongoDBOptions
	queues   map[string]amboy.Queue
}

// NewMongoRemoteSingleQueueGroup constructs a new remote queue group. If ttl is 0, the queues will not be
// TTLed except when the client explicitly calls Prune.
func NewMongoRemoteSingleQueueGroup(ctx context.Context, opts RemoteQueueGroupOptions, client *mongo.Client, mdbopts MongoDBOptions) (amboy.QueueGroup, error) {
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
	g := &remoteMongoQueueGroupSingle{
		canceler: cancel,
		client:   client,
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

func (g *remoteMongoQueueGroupSingle) startQueues(ctx context.Context) error {
	cursor, err := g.client.Database(g.dbOpts.DB).Collection(addGroupSufix(g.opts.Prefix)).Aggregate(ctx,
		[]bson.M{
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
		},
	)
	if err != nil {
		return errors.WithStack(err)
	}

	out := struct {
		Groups []string `bson:"groups"`
	}{}

	catcher := grip.NewBasicCatcher()
	for cursor.Next(ctx) {
		if err = cursor.Decode(&out); err != nil {
			catcher.Add(err)
		} else {
			break
		}
	}
	catcher.Add(cursor.Err())
	catcher.Add(cursor.Close(ctx))
	grip.NoticeWhen(len(out.Groups) == 0, "no queue groups with active tasks")
	for _, id := range out.Groups {
		_, err := g.Get(ctx, id)
		catcher.Add(err)
	}

	return catcher.Resolve()
}

func (g *remoteMongoQueueGroupSingle) Get(ctx context.Context, id string) (amboy.Queue, error) {
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

	driver, err := OpenNewMongoGroupDriver(ctx, g.opts.Prefix, g.dbOpts, id, g.client)
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

func (g *remoteMongoQueueGroupSingle) Put(ctx context.Context, name string, queue amboy.Queue) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.queues[name]; ok {
		return errors.New("cannot put a queue into group with existing name")
	}

	g.queues[name] = queue
	return nil
}

func (g *remoteMongoQueueGroupSingle) Prune(ctx context.Context) error {
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

func (g *remoteMongoQueueGroupSingle) Close(ctx context.Context) {
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
