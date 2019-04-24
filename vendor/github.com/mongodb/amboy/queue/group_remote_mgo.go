package queue

import (
	"context"
	"runtime"
	"strings"
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

// remoteMgoQueueGroup is a group of database-backed queues.
type remoteMgoQueueGroup struct {
	canceler       context.CancelFunc
	session        *mgo.Session
	constructor    RemoteConstructor
	mu             sync.RWMutex
	mongooptions   MongoDBOptions
	prefix         string
	pruneFrequency time.Duration
	queues         map[string]amboy.Queue
	ttl            time.Duration
	ttlMap         map[string]time.Time
}

// NewMgoRemoteQueueGroup constructs a new remote queue group. If ttl is 0, the queues will not be
// TTLed except when the client explicitly calls Prune.
func NewMgoRemoteQueueGroup(ctx context.Context, opts RemoteQueueGroupOptions, session *mgo.Session, mdbopts MongoDBOptions) (amboy.QueueGroup, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid remote queue options")
	}

	if mdbopts.DB == "" {
		return nil, errors.New("no database name specified")
	}

	if mdbopts.URI == "" {
		return nil, errors.New("no mongodb uri specified")
	}

	session = session.Clone()
	ctx, cancel := context.WithCancel(ctx)
	g := &remoteMgoQueueGroup{
		canceler:       cancel,
		session:        session,
		mongooptions:   mdbopts,
		constructor:    opts.Constructor,
		prefix:         opts.Prefix,
		pruneFrequency: opts.PruneFrequency,
		queues:         map[string]amboy.Queue{},
		ttl:            opts.TTL,
		ttlMap:         map[string]time.Time{},
	}

	if opts.PruneFrequency > 0 && opts.TTL > 0 {
		if err := g.Prune(ctx); err != nil {
			return nil, errors.Wrap(err, "problem pruning queue")
		}
	}

	colls, err := g.getExistingCollections(ctx, g.session, g.mongooptions.DB, g.prefix)
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
			pruneCtx, pruneCancel := context.WithCancel(context.Background())
			defer pruneCancel()
			defer recovery.LogStackTraceAndContinue("panic in remote queue group ticker")
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
	return g, nil
}

func (g *remoteMgoQueueGroup) startProcessingRemoteQueue(ctx context.Context, coll string) (Remote, error) {
	coll = trimJobsSuffix(coll)
	q, err := g.constructor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem starting queue")
	}
	d, err := OpenNewMgoDriver(ctx, coll, g.mongooptions, g.session.Clone())
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
	if opts.Prefix == "" {
		catcher.New("prefix must be set")
	}
	return catcher.Resolve()
}

func (g *remoteMgoQueueGroup) getExistingCollections(ctx context.Context, session *mgo.Session, db, prefix string) ([]string, error) {
	colls, err := session.DB(db).CollectionNames()
	if err != nil {
		return nil, errors.Wrap(err, "problem calling listCollections")
	}
	var collections []string
	for _, c := range colls {
		if strings.HasPrefix(c, prefix) {
			collections = append(collections, c)
		}
	}

	return collections, nil
}

// Get a queue with the given index. Get sets the last accessed time to now. Note that this means
// that the caller must add a job to the queue within the TTL, or else it may have attempted to add
// a job to a closed queue.
func (g *remoteMgoQueueGroup) Get(ctx context.Context, id string) (amboy.Queue, error) {
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
func (g *remoteMgoQueueGroup) Put(ctx context.Context, id string, queue amboy.Queue) error {
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
func (g *remoteMgoQueueGroup) Prune(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	colls, err := g.getExistingCollections(ctx, g.session, g.mongooptions.DB, g.prefix)
	if err != nil {
		return errors.Wrap(err, "problem getting collections")
	}
	collsToCheck := []string{}
	for _, coll := range colls {
		// This is an optimization. If we've added to the queue recently enough, there's no
		// need to query its contents, since it cannot be old enough to prune.
		if t, ok := g.ttlMap[g.idFromCollection(coll)]; !ok || ok && time.Since(t) > g.ttl {
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
			session := g.session.Clone()
			defer session.Close()
			for nextColl := range collsDropChan {
				c := session.DB(g.mongooptions.DB).C(nextColl)
				count, err := c.Find(bson.M{
					"status.completed": true,
					"status.in_prog":   false,
					"status.mod_ts":    bson.M{"$gte": time.Now().Add(-g.ttl)},
				}).Count()
				if err != nil {
					catcher.Add(err)
					return
				}
				if count > 0 {
					return
				}
				count, err = c.Find(bson.M{"status.completed": false}).Count()
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
				catcher.Add(c.DropCollection())
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
func (g *remoteMgoQueueGroup) Close(ctx context.Context) {
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
		g.session.Close()
		close(waitCh)
	}()
	select {
	case <-waitCh:
		return
	case <-ctx.Done():
		return
	}
}

func (g *remoteMgoQueueGroup) collectionFromID(id string) string {
	return addJobsSuffix(g.prefix + id)
}

func (g *remoteMgoQueueGroup) idFromCollection(collection string) string {
	return trimJobsSuffix(strings.TrimPrefix(collection, g.prefix))
}
