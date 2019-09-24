package queue

import (
	"context"
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
	opts     MongoDBQueueGroupOptions
	dbOpts   MongoDBOptions
	cache    GroupCache
}

// NewMongoDBSingleQueueGroup constructs a new remote queue group. If ttl is 0, the queues will not be
// TTLed except when the client explicitly calls Prune.
func NewMongoDBSingleQueueGroup(ctx context.Context, opts MongoDBQueueGroupOptions, client *mongo.Client, mdbopts MongoDBOptions) (amboy.QueueGroup, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid remote queue options")
	}

	if mdbopts.DB == "" {
		return nil, errors.New("no database name specified")
	}

	if mdbopts.URI == "" {
		return nil, errors.New("no mongodb uri specified")
	}

	mdbopts.UseGroups = true
	mdbopts.GroupName = opts.Prefix

	ctx, cancel := context.WithCancel(ctx)
	g := &remoteMongoQueueGroupSingle{
		canceler: cancel,
		client:   client,
		dbOpts:   mdbopts,
		opts:     opts,
		cache:    NewGroupCache(opts.TTL),
	}

	if opts.PruneFrequency > 0 && opts.TTL > 0 {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic in remote queue group ticker")
			ticker := time.NewTicker(opts.PruneFrequency)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					grip.Error(message.WrapError(g.Prune(ctx), "problem pruning remote queue group database"))
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

func (g *remoteMongoQueueGroupSingle) getQueues(ctx context.Context) ([]string, error) {
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
		return nil, errors.WithStack(err)
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

	return out.Groups, catcher.Resolve()
}

func (g *remoteMongoQueueGroupSingle) startQueues(ctx context.Context) error {
	queues, err := g.getQueues(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	catcher := grip.NewBasicCatcher()
	for _, id := range queues {
		_, err := g.Get(ctx, id)
		catcher.Add(err)
	}

	return catcher.Resolve()
}

func (g *remoteMongoQueueGroupSingle) Get(ctx context.Context, id string) (amboy.Queue, error) {
	var queue remoteQueue

	switch q := g.cache.Get(id).(type) {
	case remoteQueue:
		return q, nil
	case nil:
		queue = g.opts.constructor(ctx, id)
	default:
		return q, nil
	}

	driver, err := openNewMongoGroupDriver(ctx, g.opts.Prefix, g.dbOpts, id, g.client)
	if err != nil {
		return nil, errors.Wrap(err, "problem opening driver for queue")
	}

	if err = queue.SetDriver(driver); err != nil {
		return nil, errors.Wrap(err, "problem setting driver")
	}

	if err = g.cache.Set(id, queue, g.opts.TTL); err != nil {
		// safe to throw away the partially constructed
		// here, because another won and we  haven't started the workers.
		if q := g.cache.Get(id); q != nil {
			return q, nil
		}

		return nil, errors.Wrap(err, "problem caching queue")
	}

	if err := queue.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}

	return queue, nil
}

func (g *remoteMongoQueueGroupSingle) Put(ctx context.Context, name string, queue amboy.Queue) error {
	return g.cache.Set(name, queue, 0)
}

func (g *remoteMongoQueueGroupSingle) Len() int { return g.cache.Len() }

func (g *remoteMongoQueueGroupSingle) Queues(ctx context.Context) []string {
	queues, _ := g.getQueues(ctx) // nolint
	return queues
}

func (g *remoteMongoQueueGroupSingle) Prune(ctx context.Context) error {
	return g.cache.Prune(ctx)
}

func (g *remoteMongoQueueGroupSingle) Close(ctx context.Context) error {
	defer g.canceler()
	return g.cache.Close(ctx)
}
