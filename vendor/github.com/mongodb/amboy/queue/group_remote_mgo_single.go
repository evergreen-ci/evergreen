package queue

import (
	"context"
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
	cache    GroupCache
	opts     RemoteQueueGroupOptions
	dbOpts   MongoDBOptions
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
		cache:    NewGroupCache(opts.TTL),
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

func (g *remoteMgoQueueGroupSingle) Len() int { return g.cache.Len() }

func (g *remoteMgoQueueGroupSingle) Queues(ctx context.Context) []string {
	out, _ := g.getQueues(ctx) // nolint
	return out
}

func (g *remoteMgoQueueGroupSingle) getQueues(ctx context.Context) ([]string, error) {
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
		return nil, errors.WithStack(err)
	}

	switch len(out) {
	case 0:
		return nil, nil
	case 1:
		return out[0].Groups, nil
	default:
		return nil, errors.New("invalid inventory of existing queues")
	}
}

func (g *remoteMgoQueueGroupSingle) startQueues(ctx context.Context) error {
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

func (g *remoteMgoQueueGroupSingle) Get(ctx context.Context, id string) (amboy.Queue, error) {
	var queue Remote

	switch q := g.cache.Get(id).(type) {
	case Remote:
		return q, nil
	case nil:
		queue = g.opts.constructor(ctx, id)
	default:
		return q, nil
	}

	driver, err := OpenNewMgoGroupDriver(ctx, g.opts.Prefix, g.dbOpts, id, g.session.Clone())
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

func (g *remoteMgoQueueGroupSingle) Put(ctx context.Context, name string, queue amboy.Queue) error {
	return g.cache.Set(name, queue, g.opts.TTL)
}

func (g *remoteMgoQueueGroupSingle) Prune(ctx context.Context) error {
	return g.cache.Prune(ctx)
}

func (g *remoteMgoQueueGroupSingle) Close(ctx context.Context) error {
	defer g.canceler()
	return g.cache.Close(ctx)
}
