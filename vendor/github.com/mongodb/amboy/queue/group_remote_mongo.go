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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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

// MongoDBQueueGroupOptions describe options to create a queue group.
type MongoDBQueueGroupOptions struct {
	// Prefix is a string prepended to the queue collections.
	Prefix string

	// Abortable controls if the queue will use an abortable pool
	// imlementation. The Ordered option controls if an
	// order-respecting queue will be created, while default
	// workers sets the default number of workers new queues will
	// have if the WorkerPoolSize function is not set.
	Abortable      bool
	Ordered        bool
	DefaultWorkers int

	// WorkerPoolSize determines how many works will be allocated
	// to each queue, based on the queue ID passed to it.
	WorkerPoolSize func(string) int

	// Retryable represents the options to configure retrying jobs in the queue.
	Retryable RetryableQueueOptions

	// PruneFrequency is how often inactive queues are checked to see if they
	// can be pruned.
	PruneFrequency time.Duration

	// BackgroundCreateFrequency is how often active queues can have their
	// TTLs periodically refreshed in the background. A queue is active as long
	// as it either still has jobs to complete or the most recently completed
	// job finished within the TTL. This is useful in case a queue still has
	// jobs to process but a user does not explicitly access the queue - if the
	// goal is to ensure a queue is never pruned when it still has jobs to
	// complete, this should be set to a value lower than the TTL.
	BackgroundCreateFrequency time.Duration

	// TTL determines how long a queue is considered active without performing
	// useful work for being accessed by a user. After the TTL has elapsed, the
	// queue is allowed to be pruned.
	TTL time.Duration
}

func (opts *MongoDBQueueGroupOptions) constructor(ctx context.Context, name string) (remoteQueue, error) {
	workers := opts.DefaultWorkers
	if opts.WorkerPoolSize != nil {
		workers = opts.WorkerPoolSize(name)
		if workers == 0 {
			workers = opts.DefaultWorkers
		}
	}

	var q remoteQueue
	var err error
	qOpts := remoteOptions{
		numWorkers: workers,
		retryable:  opts.Retryable,
	}
	if opts.Ordered {
		if q, err = newRemoteSimpleOrderedWithOptions(qOpts); err != nil {
			return nil, errors.Wrap(err, "initializing ordered queue")
		}
	} else {
		if q, err = newRemoteUnorderedWithOptions(qOpts); err != nil {
			return nil, errors.Wrap(err, "initializing unordered queue")
		}
	}

	if opts.Abortable {
		p := pool.NewAbortablePool(workers, q)
		if err = q.SetRunner(p); err != nil {
			return nil, errors.Wrap(err, "configuring queue with runner")
		}
	}

	return q, nil
}

func (opts MongoDBQueueGroupOptions) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.TTL < 0, "ttl must be greater than or equal to 0")
	catcher.NewWhen(opts.TTL > 0 && opts.TTL < time.Second, "ttl cannot be less than 1 second, unless it is 0")
	catcher.NewWhen(opts.PruneFrequency < 0, "prune frequency must be greater than or equal to 0")
	catcher.NewWhen(opts.PruneFrequency > 0 && opts.TTL < time.Second, "prune frequency cannot be less than 1 second, unless it is 0")
	catcher.NewWhen((opts.TTL == 0 && opts.PruneFrequency != 0) || (opts.TTL != 0 && opts.PruneFrequency == 0), "ttl and prune frequency must both be 0 or both be not 0")
	catcher.NewWhen(opts.Prefix == "", "prefix must be set")
	catcher.NewWhen(opts.DefaultWorkers == 0 && opts.WorkerPoolSize == nil, "must specify either a default worker pool size or a WorkerPoolSize function")
	catcher.Wrap(opts.Retryable.Validate(), "invalid retryable queue options")
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
			return nil, errors.Wrap(err, "pruning queue")
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
					grip.Error(message.WrapError(g.Prune(ctx), "pruning remote queue group database"))
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
					grip.Error(message.WrapError(g.startQueues(ctx), "starting queues"))
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
		return errors.Wrap(err, "getting existing collections")
	}

	catcher := grip.NewBasicCatcher()
	for _, coll := range colls {
		q, err := g.startProcessingRemoteQueue(ctx, coll)
		if err != nil {
			catcher.Add(errors.Wrap(err, "starting queue"))
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
	q, err := g.opts.constructor(ctx, coll)
	if err != nil {
		return nil, errors.Wrap(err, "constructing queue")
	}

	var d remoteQueueDriver
	if g.client != nil {
		d, err = openNewMongoDriver(ctx, coll, g.dbOpts, g.client)
		if err != nil {
			return nil, errors.Wrap(err, "creating and opening driver")
		}
	} else {
		d, err = newMongoDriver(coll, g.dbOpts)
		if err != nil {
			return nil, errors.Wrap(err, "creating and opening driver")
		}
	}
	if err := q.SetDriver(d); err != nil {
		return nil, errors.Wrap(err, "setting driver")
	}
	if err := q.Start(ctx); err != nil {
		return nil, errors.Wrap(err, "starting queue")
	}
	return q, nil
}

func (g *remoteMongoQueueGroup) getExistingCollections(ctx context.Context, client *mongo.Client, db, prefix string) ([]string, error) {
	c, err := client.Database(db).ListCollections(ctx, bson.M{"name": bson.M{"$regex": fmt.Sprintf("^%s.*", prefix)}})
	if err != nil {
		return nil, errors.Wrap(err, "calling listCollections")
	}
	defer c.Close(ctx)
	var collections []string
	for c.Next(ctx) {
		elem := listCollectionsOutput{}
		if err := c.Decode(&elem); err != nil {
			return nil, errors.Wrap(err, "parsing listCollections output")
		}
		collections = append(collections, elem.Name)
	}
	if err := c.Err(); err != nil {
		return nil, errors.Wrap(err, "iterating over list collections cursor")
	}
	if err := c.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "closing cursor")
	}
	return collections, nil
}

// Get a queue with the given index. Get sets the last accessed time to now. Note that this means
// that the caller must add a job to the queue within the TTL, or else it may have attempted to add
// a job to a closed queue.
func (g *remoteMongoQueueGroup) Get(ctx context.Context, id string) (amboy.Queue, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if queue, ok := g.queues[id]; ok {
		g.ttlMap[id] = time.Now()
		return queue, nil
	}

	queue, err := g.startProcessingRemoteQueue(ctx, g.collectionFromID(id))
	if err != nil {
		return nil, errors.Wrap(err, "starting queue")
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
		return errors.Wrap(err, "getting collections")
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
					"$or": []bson.M{
						{"status.mod_ts": bson.M{"$gte": time.Now().Add(-g.opts.TTL)}},
						{
							"retry_info.retryable":   true,
							"retry_info.needs_retry": true,
						},
					},
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
					queue.Close(ctx)
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
				queue.Close(ctx)
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
