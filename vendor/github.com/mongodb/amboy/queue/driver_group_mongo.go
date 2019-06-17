package queue

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

type mongoGroupDriver struct {
	client     *mongo.Client
	name       string
	group      string
	opts       MongoDBOptions
	instanceID string
	mu         sync.RWMutex
	canceler   context.CancelFunc

	LockManager
}

// NewMongoGroupDriver is similar to the MongoDriver, except it
// prefixes job ids with a prefix and adds the group field to the
// documents in the database which makes it possible to manage
// distinct queues with a single MongoDB collection.
func NewMongoGroupDriver(name string, opts MongoDBOptions, group string) Driver {
	host, _ := os.Hostname() // nolint

	if !opts.Format.IsValid() {
		opts.Format = amboy.BSON
	}

	return &mongoGroupDriver{
		name:       name,
		group:      group,
		opts:       opts,
		instanceID: fmt.Sprintf("%s.%s.%s.%s", name, group, host, uuid.NewV4()),
	}
}

// OpenNewMongoGroupDriver constructs and opens a new MongoDB driver instance
// using the specified session. It is equivalent to calling
// NewMongoGroupDriver() and calling driver.Open().
func OpenNewMongoGroupDriver(ctx context.Context, name string, opts MongoDBOptions, group string, client *mongo.Client) (Driver, error) {
	d, ok := NewMongoGroupDriver(name, opts, group).(*mongoGroupDriver)
	if !ok {
		return nil, errors.New("amboy programmer error: incorrect constructor")
	}

	if err := d.start(ctx, client); err != nil {
		return nil, errors.Wrap(err, "problem starting driver")
	}

	return d, nil
}

func (d *mongoGroupDriver) ID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceID
}

func (d *mongoGroupDriver) Open(ctx context.Context) error {
	if d.canceler != nil {
		return nil
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(d.opts.URI))
	if err != nil {
		return errors.Wrapf(err, "problem opening connection to mongodb at '%s", d.opts.URI)
	}

	return errors.Wrap(d.start(ctx, client), "problem starting driver")
}

func (d *mongoGroupDriver) start(ctx context.Context, client *mongo.Client) error {
	d.LockManager = NewLockManager(ctx, d)

	dCtx, cancel := context.WithCancel(ctx)
	d.canceler = cancel

	d.mu.Lock()
	d.client = client
	d.mu.Unlock()

	startAt := time.Now()
	go func() {
		<-dCtx.Done()
		grip.Info(message.Fields{
			"message": "closing session for mongodb driver",
			"group":   d.group,
			"id":      d.instanceID,
			"uptime":  time.Since(startAt),
			"span":    time.Since(startAt).String(),
			"service": "amboy.queue.group.mongodb",
		})
	}()

	if err := d.setupDB(ctx); err != nil {
		return errors.Wrap(err, "problem setting up database")
	}

	return nil
}

func (d *mongoGroupDriver) getCollection() *mongo.Collection {
	return d.client.Database(d.opts.DB).Collection(addGroupSufix(d.name))
}

func (d *mongoGroupDriver) setupDB(ctx context.Context) error {
	if d.opts.SkipIndexBuilds {
		return nil
	}

	keys := bsonx.Doc{
		{
			Key:   "group",
			Value: bsonx.Int32(1),
		},
		{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		{
			Key:   "status.in_prog",
			Value: bsonx.Int32(1),
		},
	}
	if d.opts.CheckWaitUntil {
		keys = append(keys, bsonx.Elem{
			Key:   "time_info.wait_until",
			Value: bsonx.Int32(1),
		})
	}

	// priority must be at the end for the sort
	if d.opts.Priority {
		keys = append(keys, bsonx.Elem{
			Key:   "priority",
			Value: bsonx.Int32(1),
		})
	}

	_, err := d.getCollection().Indexes().CreateMany(ctx, []mongo.IndexModel{
		mongo.IndexModel{
			Keys: keys,
		},
		mongo.IndexModel{
			Keys: bsonx.Doc{
				{
					Key:   "group",
					Value: bsonx.Int32(1),
				},
				{
					Key:   "status.mod_ts",
					Value: bsonx.Int32(1),
				},
			},
		},
	})

	return errors.Wrap(err, "problem building indexes")
}

func (d *mongoGroupDriver) Close() {
	if d.canceler != nil {
		d.canceler()
	}
}

func (d *mongoGroupDriver) Get(ctx context.Context, name string) (amboy.Job, error) {
	res := d.getCollection().FindOne(ctx, bson.M{"_id": buildCompoundID(d.group, name)})
	if err := res.Err(); err != nil {
		return nil, errors.Wrapf(err, "GET problem fetching '%s'", name)
	}

	j := &registry.JobInterchange{}
	if err := res.Decode(j); err != nil {
		return nil, errors.Wrapf(err, "GET problem decoding '%s'", name)
	}

	j.Name = j.Name[len(d.group)+1:]

	output, err := j.Resolve(d.opts.Format)
	if err != nil {
		return nil, errors.Wrapf(err,
			"GET problem converting '%s' to job object", name)
	}

	return output, nil
}

func (d *mongoGroupDriver) Put(ctx context.Context, j amboy.Job) error {
	job, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	job.Group = d.group
	job.Name = buildCompoundJobID(d.group, j)

	if _, err := d.getCollection().InsertOne(ctx, job); err != nil {
		return errors.Wrapf(err, "problem saving new job %s", j.ID())
	}

	return nil
}

func buildCompoundJobID(n string, job amboy.Job) string { return buildCompoundID(n, job.ID()) }
func buildCompoundID(n, id string) string               { return fmt.Sprintf("%s.%s", n, id) }

func (d *mongoGroupDriver) Save(ctx context.Context, j amboy.Job) error {
	name := j.ID()
	stat := j.Status()
	stat.Owner = d.instanceID
	stat.ModificationCount++
	stat.ModificationTime = time.Now()
	j.SetStatus(stat)

	job, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	job.Group = d.group
	job.Name = buildCompoundJobID(d.group, j)

	query := getAtomicQuery(d.instanceID, job.Name, stat.ModificationCount)
	res, err := d.getCollection().ReplaceOne(ctx, query, job)
	if err != nil {
		if isMongoDupKey(err) {
			grip.Debug(message.Fields{
				"id":        d.instanceID,
				"group":     d.group,
				"service":   "amboy.queue.group.mongo",
				"operation": "save job",
				"name":      name,
				"outcome":   "duplicate key error, ignoring stale job",
			})
			return nil
		}
		return errors.Wrapf(err, "problem saving document %s: %+v", name, res)
	}

	if res.MatchedCount == 0 {
		return errors.Errorf("problem saving job [id=%s, matched=%d, modified=%d]", name, res.MatchedCount, res.ModifiedCount)
	}

	return nil
}

func (d *mongoGroupDriver) SaveStatus(ctx context.Context, j amboy.Job, stat amboy.JobStatusInfo) error {
	query := getAtomicQuery(d.instanceID, buildCompoundJobID(d.group, j), stat.ModificationCount)
	stat.Owner = d.instanceID
	stat.ModificationCount++
	stat.ModificationTime = time.Now()
	timeInfo := j.TimeInfo()

	res, err := d.getCollection().UpdateOne(ctx, query, bson.M{"$set": bson.M{"status": stat, "time_info": timeInfo}})
	if err != nil {
		return errors.Wrapf(err, "problem updating status document for %s", j.ID())
	}

	if res.MatchedCount == 0 {
		return errors.Errorf("did not update any status documents [id=%s, matched=%d, modified=%d]", j.ID(), res.MatchedCount, res.ModifiedCount)
	}

	j.SetStatus(stat)

	return nil
}

func (d *mongoGroupDriver) Jobs(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)
	go func() {
		defer close(output)
		iter, err := d.getCollection().Find(ctx, bson.M{"group": d.group}, options.Find().SetSort(bson.M{"status.mod_ts": -1}))
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":        d.instanceID,
				"group":     d.group,
				"service":   "amboy.queue.group.mongo",
				"operation": "job iterator",
				"message":   "problem with query",
			}))
			return
		}
		var job amboy.Job
		for iter.Next(ctx) {
			j := &registry.JobInterchange{}
			if err = iter.Decode(j); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"group":     d.group,
					"service":   "amboy.queue.group.mongo",
					"operation": "job iterator",
					"message":   "problem reading job from cursor",
				}))

				continue
			}

			j.Name = j.Name[len(d.group)+1:]

			job, err = j.Resolve(d.opts.Format)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"group":     d.group,
					"service":   "amboy.queue.group.mongo",
					"operation": "job iterator",
					"message":   "problem converting job obj",
				}))
				continue
			}

			output <- job
		}

		grip.Error(message.WrapError(iter.Err(), message.Fields{
			"id":        d.instanceID,
			"group":     d.group,
			"service":   "amboy.queue.group.mongo",
			"operation": "job iterator",
			"message":   "database interface error",
		}))
	}()
	return output
}

func (d *mongoGroupDriver) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	output := make(chan amboy.JobStatusInfo)
	go func() {
		defer close(output)

		iter, err := d.getCollection().Find(ctx,
			bson.M{"group": d.group},
			&options.FindOptions{
				Sort: bson.M{"status.mod_ts": -1},
				Projection: bson.M{
					"_id":    1,
					"status": 1,
				},
			})
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":        d.instanceID,
				"group":     d.group,
				"service":   "amboy.queue.group.mongo",
				"operation": "job status iterator",
				"message":   "problem with query",
			}))
			return
		}

		for iter.Next(ctx) {
			j := &registry.JobInterchange{}
			if err := iter.Decode(j); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.monto",
					"group":     d.group,
					"operation": "job status iterator",
					"message":   "problem converting job obj",
				}))
				continue
			}
			j.Name = j.Name[len(d.group)+1:]
			j.Status.ID = j.Name
			select {
			case <-ctx.Done():
				return
			case output <- j.Status:
			}
		}
	}()

	return output
}

func (d *mongoGroupDriver) Next(ctx context.Context) amboy.Job {
	var (
		qd     bson.M
		job    amboy.Job
		misses int64
	)

	qd = bson.M{
		"group": d.group,
		"$or": []bson.M{
			{
				"status.completed": false,
				"status.in_prog":   false,
			},
			{
				"status.completed": false,
				"status.mod_ts":    bson.M{"$lte": time.Now().Add(-LockTimeout)},
				"status.in_prog":   true,
			},
		},
	}

	if d.opts.CheckWaitUntil {
		qd = bson.M{
			"$and": []bson.M{
				qd,
				{"$or": []bson.M{
					{"time_info.wait_until": bson.M{"$lte": time.Now()}},
					{"time_info.wait_until": bson.M{"$exists": false}}},
				},
			}}
	}

	opts := options.Find().SetBatchSize(4)
	if d.opts.Priority {
		opts.SetSort(bson.M{"priority": -1})
	}

	j := &registry.JobInterchange{}
	timer := time.NewTimer(0)
	defer timer.Stop()

RETRY:
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			misses++
			iter, err := d.getCollection().Find(ctx, qd, opts)
			if err != nil {
				grip.Debug(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"group":     d.group,
					"service":   "amboy.queue.group.mongo",
					"operation": "retrieving next job",
					"message":   "problem generating query",
				}))
				return nil
			}

		CURSOR:
			for iter.Next(ctx) {
				if err = iter.Decode(j); err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"id":        d.instanceID,
						"group":     d.group,
						"service":   "amboy.queue.group.mongo",
						"operation": "converting next job",
						"message":   "problem reading document from cursor",
					}))
					// try for the next thing in the iterator if we can
					continue CURSOR
				}

				job, err = j.Resolve(d.opts.Format)
				if err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"id":        d.instanceID,
						"group":     d.group,
						"service":   "amboy.queue.group.mongo",
						"operation": "converting document",
						"message":   "problem converting job from intermediate form",
					}))
					// try for the next thing in the iterator if we can
					continue CURSOR
				}
				break CURSOR
			}

			if err = iter.Err(); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"group":     d.group,
					"service":   "amboy.queue.group.mongo",
					"message":   "problem reported by iterator",
					"operation": "retrieving next job",
				}))
				return nil
			}

			if err = iter.Close(ctx); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"group":     d.group,
					"service":   "amboy.queue.group.mongo",
					"message":   "problem closing iterator",
					"operation": "retrieving next job",
				}))
				return nil
			}

			if job != nil {
				break RETRY
			}

			timer.Reset(time.Duration(misses * rand.Int63n(int64(d.opts.WaitInterval))))
			continue RETRY
		}
	}

	return job
}

func (d *mongoGroupDriver) Stats(ctx context.Context) amboy.QueueStats {
	coll := d.getCollection()
	total, err := coll.CountDocuments(ctx, bson.M{"group": d.group})
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"group":      d.group,
		"service":    "amboy.queue.group.mongo",
		"collection": coll.Name(),
		"operation":  "queue stats",
		"message":    "problem counting all jobs jobs",
	}))

	pending, err := coll.CountDocuments(ctx, bson.M{"group": d.group, "status.completed": false})
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"group":      d.group,
		"service":    "amboy.queue.group.mongo",
		"collection": coll.Name(),
		"operation":  "queue stats",
		"message":    "problem counting pending jobs",
	}))

	numLocked, err := coll.CountDocuments(ctx, bson.M{"group": d.group, "status.completed": false, "status.in_prog": true})
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"group":      d.group,
		"service":    "amboy.queue.group.mongo",
		"collection": coll.Name(),
		"operation":  "queue stats",
		"message":    "problem counting locked jobs",
	}))

	return amboy.QueueStats{
		Total:     int(total),
		Pending:   int(pending),
		Completed: int(total - pending),
		Running:   int(numLocked),
	}
}
