package queue

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/mgo.v2/bson"
)

// remoteMongoQueueGroup is a group of database-backed queues.
type remoteMongoQueueGroup struct {
	canceler context.CancelFunc
	client   *mongo.Client
	mu       sync.RWMutex
	opts     MongoDBQueueGroupOptions
	dbOpts   MongoDBOptions
	queues   map[string]amboy.Queue
	ttlMap   map[string]time.Time
}

// MongoDBQueueGroupOptions describe options passed to NewRemoteQueueGroup.
type MongoDBQueueGroupOptions struct {
	// Prefix is a string prepended to the queue collections.
	Prefix string

	// Abortable controls if the queue will use an abortable pool
	// imlementation. The Ordered option controls if an
	// order-respecting queue will be created, while default
	// workers sets the defualt number of workers new queues will
	// have if the WorkerPoolSize function is not set.
	Abortable      bool
	Ordered        bool
	DefaultWorkers int

	// WorkerPoolSize determines how many works will be allocated
	// to each queue, based on the queue ID passed to it.
	WorkerPoolSize func(string) int

	// PruneFrequency is how often Prune runs by default.
	PruneFrequency time.Duration

	// BackgroundCreateFrequency is how often the background queue
	// creation runs, in the case that queues may be created in
	// the background without
	BackgroundCreateFrequency time.Duration

	// TTL is how old the oldest task in the queue must be for the collection to be pruned.
	TTL time.Duration
}

func (opts *MongoDBQueueGroupOptions) constructor(ctx context.Context, name string) remoteQueue {
	workers := opts.DefaultWorkers
	if opts.WorkerPoolSize != nil {
		workers = opts.WorkerPoolSize(name)
		if workers == 0 {
			workers = opts.DefaultWorkers
		}
	}

	var q remoteQueue
	if opts.Ordered {
		q = newSimpleRemoteOrdered(workers)
	} else {
		q = newRemoteUnordered(workers)
	}

	if opts.Abortable {
		p := pool.NewAbortablePool(workers, q)
		grip.Debug(q.SetRunner(p))
	}

	return q
}

func (opts MongoDBQueueGroupOptions) validate() error {
	catcher := grip.NewBasicCatcher()
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
	if opts.Prefix == "" {
		catcher.New("prefix must be set")
	}
	if opts.DefaultWorkers == 0 && opts.WorkerPoolSize == nil {
		catcher.New("must specify either a default worker pool size or a WorkerPoolSize function")
	}
	return catcher.Resolve()
}

type listCollectionsOutput struct {
	Name string `bson:"name"`
}

// NewMongoDBQueueGroup constructs a new remote queue group. If
// ttl is 0, the queues will not be TTLed except when the client
// explicitly calls Prune.
//
// The MongoDBRemoteQueue group creats a new collection for every queue,
// unlike the other remote queue group implementations. This is
// probably most viable for lower volume workloads; however, the
// caching mechanism may be more responsive in some situations.
func NewMongoDBQueueGroup(ctx context.Context, opts MongoDBQueueGroupOptions, client *mongo.Client, mdbopts MongoDBOptions) (amboy.QueueGroup, error) {
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
	g := &remoteMongoQueueGroup{
		canceler: cancel,
		client:   client,
		dbOpts:   mdbopts,
		opts:     opts,
		queues:   map[string]amboy.Queue{},
		ttlMap:   map[string]time.Time{},
	}

	if opts.PruneFrequency > 0 && opts.TTL > 0 {
		if err := g.Prune(ctx); err != nil {
			return nil, errors.Wrap(err, "problem pruning queue")
		}
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
			ticker := time.NewTicker(opts.PruneFrequency)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					grip.Error(message.WrapError(g.startQueues(ctx), "problem starting queues"))
				}
			}
		}()
	}

	if err := g.startQueues(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	return g, nil
}

func (g *remoteMongoQueueGroup) startQueues(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	colls, err := g.getExistingCollections(ctx, g.client, g.dbOpts.DB, g.opts.Prefix)
	if err != nil {
		return errors.Wrap(err, "problem getting existing collections")
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

	return catcher.Resolve()
}

func (g *remoteMongoQueueGroup) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return len(g.queues)
}

func (g *remoteMongoQueueGroup) Queues(ctx context.Context) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	out, _ := g.getExistingCollections(ctx, g.client, g.dbOpts.DB, g.opts.Prefix) // nolint

	return out
}

func (g *remoteMongoQueueGroup) startProcessingRemoteQueue(ctx context.Context, coll string) (amboy.Queue, error) {
	coll = trimJobsSuffix(coll)
	q := g.opts.constructor(ctx, coll)

	d, err := openNewMongoDriver(ctx, coll, g.dbOpts, g.client)
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

func (g *remoteMongoQueueGroup) getExistingCollections(ctx context.Context, client *mongo.Client, db, prefix string) ([]string, error) {
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
	if err := c.Err(); err != nil {
		return nil, errors.Wrap(err, "problem iterating over list collections cursor")
	}
	if err := c.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "problem closing cursor")
	}
	return collections, nil
}

// Get a queue with the given index. Get sets the last accessed time to now. Note that this means
// that the caller must add a job to the queue within the TTL, or else it may have attempted to add
// a job to a closed queue.
func (g *remoteMongoQueueGroup) Get(ctx context.Context, id string) (amboy.Queue, error) {
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
func (g *remoteMongoQueueGroup) Put(ctx context.Context, id string, queue amboy.Queue) error {
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
func (g *remoteMongoQueueGroup) Prune(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	colls, err := g.getExistingCollections(ctx, g.client, g.dbOpts.DB, g.opts.Prefix)
	if err != nil {
		return errors.Wrap(err, "problem getting collections")
	}
	collsToCheck := []string{}
	for _, coll := range colls {
		// This is an optimization. If we've added to the queue recently enough, there's no
		// need to query its contents, since it cannot be old enough to prune.
		if t, ok := g.ttlMap[g.idFromCollection(coll)]; !ok || ok && time.Since(t) > g.opts.TTL {
			collsToCheck = append(collsToCheck, coll)
		}
	}
	catcher := grip.NewBasicCatcher()
	wg := &sync.WaitGroup{}
	collsDeleteChan := make(chan string, len(collsToCheck))
	collsDropChan := make(chan string, len(collsToCheck))

	for _, coll := range collsToCheck {
		collsDropChan <- coll
	}
	close(collsDropChan)

	wg = &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func(ch chan string) {
			defer recovery.LogStackTraceAndContinue("panic in pruning collections")
			defer wg.Done()
			for nextColl := range collsDropChan {
				c := g.client.Database(g.dbOpts.DB).Collection(nextColl)
				count, err := c.CountDocuments(ctx, bson.M{
					"status.completed": true,
					"status.in_prog":   false,
					"status.mod_ts":    bson.M{"$gte": time.Now().Add(-g.opts.TTL)},
				})
				if err != nil {
					catcher.Add(err)
					return
				}
				if count > 0 {
					return
				}
				count, err = c.CountDocuments(ctx, bson.M{"status.completed": false})
				if err != nil {
					catcher.Add(err)
					return
				}
				if count > 0 {
					return
				}
				if queue, ok := g.queues[g.idFromCollection(nextColl)]; ok {
					queue.Runner().Close(ctx)
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
		for _, coll := range collsToCheck {
			if id == g.idFromCollection(coll) {
				continue outer
			}
		}
		wg.Add(1)
		go func(queueID string, ch chan string, qu amboy.Queue) {
			defer recovery.LogStackTraceAndContinue("panic in pruning queues")
			defer wg.Done()
			qu.Runner().Close(ctx)
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
func (g *remoteMongoQueueGroup) Close(ctx context.Context) error {
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
				queue.Runner().Close(ctx)
			}(queue)
		}
		wg.Wait()
		close(waitCh)
	}()
	select {
	case <-waitCh:
		return nil
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	}
}

func (g *remoteMongoQueueGroup) collectionFromID(id string) string {
	return addJobsSuffix(g.opts.Prefix + id)
}

func (g *remoteMongoQueueGroup) idFromCollection(collection string) string {
	return trimJobsSuffix(strings.TrimPrefix(collection, g.opts.Prefix))
}
