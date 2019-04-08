package queue

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/mgo.v2/bson"
)

// RemoteConstructor is a function passed by a client which makes a new remote queue for a QueueGroup.
type RemoteConstructor func(ctx context.Context) (Remote, error)

// remoteQueueGroup is a group of database-backed queues.
type remoteQueueGroup struct {
	canceler       context.CancelFunc
	client         *mongo.Client
	constructor    RemoteConstructor
	mongooptions   MongoDBOptions
	mu             sync.RWMutex
	prefix         string
	pruneFrequency time.Duration
	queues         map[string]amboy.Queue
	ttl            time.Duration
	ttlMap         map[string]time.Time
}

// RemoteQueueGroupOptions describe options passed to NewRemoteQueueGroup.
type RemoteQueueGroupOptions struct {
	// Client is a connection to a database.
	Client *mongo.Client

	// Constructor is a function passed by the client to construct a remote queue.
	Constructor RemoteConstructor

	// MongoOptions are connection options for the database.
	MongoOptions MongoDBOptions

	// Prefix is a string prepended to the queue collections.
	Prefix string

	// PruneFrequency is how often Prune runs.
	PruneFrequency time.Duration

	// TTL is how old the oldest task in the queue must be for the collection to be pruned.
	TTL time.Duration
}

type listCollectionsOutput struct {
	Name string `bson:"name"`
}

// NewRemoteQueueGroup constructs a new remote queue group. If ttl is 0, the queues will not be
// TTLed except when the client explicitly calls Prune.
func NewRemoteQueueGroup(ctx context.Context, opts RemoteQueueGroupOptions) (amboy.QueueGroup, error) {
	ctx, cancel := context.WithCancel(ctx)

	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid remote queue options")
	}

	g := &remoteQueueGroup{
		canceler:       cancel,
		client:         opts.Client,
		constructor:    opts.Constructor,
		mongooptions:   opts.MongoOptions,
		prefix:         opts.Prefix,
		pruneFrequency: opts.PruneFrequency,
		queues:         map[string]amboy.Queue{},
		ttl:            opts.TTL,
		ttlMap:         map[string]time.Time{},
	}

	colls, err := g.getExistingCollections(ctx, g.client, g.mongooptions.DB, g.prefix)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting existing collections")
	}

	catcher := grip.NewBasicCatcher()
	for _, coll := range colls {
		q, err := g.startProcessingRemoteQueue(ctx, coll)
		if err != nil {
			catcher.Add(errors.Wrap(err, "problem starting queue"))
		} else {
			g.queues[g.idFromCollection(coll)] = q
			g.ttlMap[g.idFromCollection(coll)] = time.Now()
		}
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
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
					_ = g.Prune(ctx)
				}
			}
		}()
	}
	return g, nil
}

func (g *remoteQueueGroup) startProcessingRemoteQueue(ctx context.Context, coll string) (Remote, error) {
	q, err := g.constructor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}
	d, err := OpenNewMongoDriver(ctx, coll, g.mongooptions, g.client)
	if err != nil {
		return nil, errors.Wrap(err, "problem opening driver")
	}
	if err := q.SetDriver(d); err != nil {
		return nil, errors.Wrap(err, "problem setting driver")
	}
	if err := q.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}
	return q, nil
}

func (opts RemoteQueueGroupOptions) validate() error {
	catcher := grip.NewBasicCatcher()
	if opts.Client == nil {
		catcher.New("must pass a client")
	}
	if opts.Constructor == nil {
		catcher.New("must pass a constructor")
	}
	if opts.TTL < 0 {
		catcher.New("ttl must be greater than or equal to 0")
	}
	if opts.TTL > 0 && opts.TTL < time.Second {
		catcher.New("ttl cannot be less than 1 second, unless it is 0")
	}
	if opts.PruneFrequency < 0 {
		catcher.New("prune frequency must be greater than or equal to 0")
	}
	if opts.PruneFrequency > 0 && opts.TTL < time.Second {
		catcher.New("prune frequency cannot be less than 1 second, unless it is 0")
	}
	if (opts.TTL == 0 && opts.PruneFrequency != 0) || (opts.TTL != 0 && opts.PruneFrequency == 0) {
		catcher.New("ttl and prune frequency must both be 0 or both be not 0")
	}
	if opts.MongoOptions.DB == "" {
		catcher.New("db must be set")
	}
	if opts.Prefix == "" {
		catcher.New("prefix must be set")
	}
	if opts.MongoOptions.URI == "" {
		catcher.New("uri must be set")
	}
	return catcher.Resolve()
}

func (g *remoteQueueGroup) getExistingCollections(ctx context.Context, client *mongo.Client, db, prefix string) ([]string, error) {
	c, err := client.Database(db).ListCollections(ctx, bson.M{"name": bson.M{"$regex": fmt.Sprintf("^%s.*", prefix)}})
	if err != nil {
		return nil, errors.Wrap(err, "problem calling listCollections")
	}
	defer c.Close(ctx)
	var collections []string
	for c.Next(ctx) {
		elem := listCollectionsOutput{}
		if err := c.Decode(&elem); err != nil {
			return nil, errors.Wrap(err, "problem parsing listCollections output")
		}
		collections = append(collections, elem.Name)
	}
	if c.Err() != nil {
		return nil, errors.Wrap(err, "problem iterating over list collections cursor")
	}
	return collections, nil
}

// Get a queue with the given index. Get sets the last accessed time to now. Note that this means
// that the caller must add a job to the queue within the TTL, or else it may have attempted to add
// a job to a closed queue.
func (g *remoteQueueGroup) Get(ctx context.Context, id string) (amboy.Queue, error) {
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

	queue, err := g.startProcessingRemoteQueue(ctx, g.collectionFromID(id))
	if err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}
	g.queues[id] = queue
	g.ttlMap[id] = time.Now()
	return queue, nil
}

// Put a queue at the given index. The caller is responsible for starting thq queue.
func (g *remoteQueueGroup) Put(ctx context.Context, id string, queue amboy.Queue) error {
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

// Prune queues that have no pending work, and have completed work older than the TTL.
func (g *remoteQueueGroup) Prune(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	colls, err := g.getExistingCollections(ctx, g.client, g.mongooptions.DB, g.prefix)
	if err != nil {
		return errors.Wrap(err, "problem getting collections")
	}
	for i, coll := range colls {
		// This is an optimization. If we've added to the queue recently enough, there's no
		// need to query its contents, since it cannot be old enough to prune.
		if t, ok := g.ttlMap[coll]; ok && time.Since(t) < g.ttl {
			g.remove(colls, i)
		}
	}
	catcher := grip.NewBasicCatcher()
	wg := &sync.WaitGroup{}
	collsDeleteChan := make(chan string, len(colls))
	collsDropChan := make(chan string)

	go func() {
		defer recovery.LogStackTraceAndContinue("panic in pruning collections")
		for _, coll := range colls {
			collsDropChan <- coll
		}
		close(collsDropChan)
	}()
	wg = &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func(ch chan string) {
			defer recovery.LogStackTraceAndContinue("panic in pruning collections")
			defer wg.Done()
			for nextColl := range collsDropChan {
				c := g.client.Database(g.mongooptions.DB).Collection(nextColl)
				one := c.FindOne(ctx, bson.M{"status.mod_ts": bson.M{"$gte": time.Now().Add(-g.ttl)}})
				if err := one.Err(); err != nil {
					catcher.Add(err)
					return
				}
				if err := one.Decode(struct{}{}); err != nil {
					if err != mongo.ErrNoDocuments {
						catcher.Add(err)
						return
					}
				} else {
					return
				}
				one = c.FindOne(ctx, bson.M{"status.completed": false})
				if err := one.Err(); err != nil {
					catcher.Add(err)
					return
				}
				if err := one.Decode(struct{}{}); err != nil {
					if err != mongo.ErrNoDocuments {
						catcher.Add(err)
						return
					}
				} else {
					return
				}
				if queue, ok := g.queues[g.idFromCollection(nextColl)]; ok {
					queue.Runner().Close()
					select {
					case <-ctx.Done():
						return
					case ch <- g.idFromCollection(nextColl):
						// pass
					}
				}
				if err := c.Drop(ctx); err != nil {
					catcher.Add(err)
				}
			}
		}(collsDeleteChan)
	}
	wg.Wait()
	close(collsDeleteChan)
	for id := range collsDeleteChan {
		delete(g.queues, id)
		delete(g.ttlMap, id)
	}

	// Another prune may have gotten to the collection first, so we should close the queue.
	queuesDeleteChan := make(chan string, len(g.queues))
	wg = &sync.WaitGroup{}
outer:
	for id, q := range g.queues {
		for _, coll := range colls {
			if id == g.idFromCollection(coll) {
				continue outer
			}
		}
		wg.Add(1)
		go func(queueID string, ch chan string, qu amboy.Queue) {
			defer recovery.LogStackTraceAndContinue("panic in pruning queues")
			defer wg.Done()
			qu.Runner().Close()
			select {
			case <-ctx.Done():
				return
			case ch <- queueID:
				// pass
			}
		}(id, queuesDeleteChan, q)
	}
	wg.Wait()
	close(queuesDeleteChan)
	for id := range queuesDeleteChan {
		delete(g.queues, id)
		delete(g.ttlMap, id)
	}
	return catcher.Resolve()
}

// Close the queues.
func (g *remoteQueueGroup) Close(ctx context.Context) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.canceler()
	waitCh := make(chan struct{})
	wg := &sync.WaitGroup{}
	go func() {
		defer recovery.LogStackTraceAndContinue("panic in remote queue group closer")
		for _, queue := range g.queues {
			wg.Add(1)
			go func(queue amboy.Queue) {
				defer recovery.LogStackTraceAndContinue("panic in remote queue group closer")
				defer wg.Done()
				queue.Runner().Close()
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

func (g *remoteQueueGroup) collectionFromID(id string) string {
	return g.prefix + id
}

func (g *remoteQueueGroup) idFromCollection(collection string) string {
	return strings.TrimPrefix(collection, g.prefix)
}

// remove efficiently from a slice if order doesn't matter https://stackoverflow.com/a/37335777.
func (g *remoteQueueGroup) remove(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
