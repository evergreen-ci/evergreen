package queue

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
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

type mongoDriver struct {
	client     *mongo.Client
	name       string
	opts       MongoDBOptions
	instanceID string
	mu         sync.RWMutex
	canceler   context.CancelFunc
}

// NewMongoDriver constructs a MongoDB backed queue driver
// implementation using the go.mongodb.org/mongo-driver as the
// database interface.
func newMongoDriver(name string, opts MongoDBOptions) remoteQueueDriver {
	host, _ := os.Hostname() // nolint

	if !opts.Format.IsValid() {
		opts.Format = amboy.BSON
	}

	return &mongoDriver{
		name:       name,
		opts:       opts,
		instanceID: fmt.Sprintf("%s.%s.%s", name, host, uuid.NewV4()),
	}
}

// openNewMongoDriver constructs and opens a new MongoDB driver instance
// using the specified session. It is equivalent to calling
// NewMongoDriver() and calling driver.Open().
func openNewMongoDriver(ctx context.Context, name string, opts MongoDBOptions, client *mongo.Client) (remoteQueueDriver, error) {
	d := newMongoDriver(name, opts).(*mongoDriver)

	if err := d.start(ctx, client); err != nil {
		return nil, errors.Wrap(err, "problem starting driver")
	}

	return d, nil
}

// newMongoGroupDriver is similar to the MongoDriver, except it
// prefixes job ids with a prefix and adds the group field to the
// documents in the database which makes it possible to manage
// distinct queues with a single MongoDB collection.
func newMongoGroupDriver(name string, opts MongoDBOptions, group string) remoteQueueDriver {
	host, _ := os.Hostname() // nolint

	if !opts.Format.IsValid() {
		opts.Format = amboy.BSON
	}
	opts.UseGroups = true
	opts.GroupName = group

	return &mongoDriver{
		name:       name,
		opts:       opts,
		instanceID: fmt.Sprintf("%s.%s.%s.%s", name, group, host, uuid.NewV4()),
	}
}

// OpenNewMongoGroupDriver constructs and opens a new MongoDB driver instance
// using the specified session. It is equivalent to calling
// NewMongoGroupDriver() and calling driver.Open().
func openNewMongoGroupDriver(ctx context.Context, name string, opts MongoDBOptions, group string, client *mongo.Client) (remoteQueueDriver, error) {
	d, ok := newMongoGroupDriver(name, opts, group).(*mongoDriver)
	if !ok {
		return nil, errors.New("amboy programmer error: incorrect constructor")
	}

	opts.UseGroups = true
	opts.GroupName = group

	if err := d.start(ctx, client); err != nil {
		return nil, errors.Wrap(err, "problem starting driver")
	}

	return d, nil
}

func (d *mongoDriver) ID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceID
}

func (d *mongoDriver) Open(ctx context.Context) error {
	if d.canceler != nil {
		return nil
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(d.opts.URI))
	if err != nil {
		return errors.Wrapf(err, "problem opening connection to mongodb at '%s", d.opts.URI)
	}

	return errors.Wrap(d.start(ctx, client), "problem starting driver")
}

func (d *mongoDriver) start(ctx context.Context, client *mongo.Client) error {
	dCtx, cancel := context.WithCancel(ctx)
	d.canceler = cancel

	d.mu.Lock()
	d.client = client
	d.mu.Unlock()

	startAt := time.Now()
	go func() {
		<-dCtx.Done()
		grip.Info(message.Fields{
			"message":  "closing session for mongodb driver",
			"id":       d.instanceID,
			"uptime":   time.Since(startAt),
			"span":     time.Since(startAt).String(),
			"service":  "amboy.queue.mongodb",
			"is_group": d.opts.UseGroups,
			"group":    d.opts.GroupName,
		})
	}()

	if err := d.setupDB(ctx); err != nil {
		return errors.Wrap(err, "problem setting up database")
	}

	return nil
}

func (d *mongoDriver) getCollection() *mongo.Collection {
	db := d.client.Database(d.opts.DB)
	if d.opts.UseGroups {
		return db.Collection(addGroupSufix(d.name))
	}

	return db.Collection(addJobsSuffix(d.name))
}

func (d *mongoDriver) setupDB(ctx context.Context) error {
	if d.opts.SkipIndexBuilds {
		return nil
	}

	keys := bsonx.Doc{}
	modKeys := bsonx.Doc{}

	if d.opts.UseGroups {
		keys = append(keys, bsonx.Elem{
			Key:   "group",
			Value: bsonx.Int32(1),
		})
		modKeys = append(modKeys, bsonx.Elem{
			Key:   "group",
			Value: bsonx.Int32(1),
		})
	}
	keys = append(keys,
		bsonx.Elem{
			Key:   "status.completed",
			Value: bsonx.Int32(1),
		},
		bsonx.Elem{
			Key:   "status.in_prog",
			Value: bsonx.Int32(1),
		})

	modKeys = append(modKeys, bsonx.Elem{
		Key:   "status.mod_ts",
		Value: bsonx.Int32(1),
	})

	if d.opts.CheckWaitUntil {
		keys = append(keys, bsonx.Elem{
			Key:   "time_info.wait_until",
			Value: bsonx.Int32(1),
		})
	}

	if d.opts.CheckDispatchBy {
		keys = append(keys, bsonx.Elem{
			Key:   "time_info.dispatch_by",
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

	indexes := []mongo.IndexModel{
		mongo.IndexModel{
			Keys: keys,
		},
		mongo.IndexModel{
			Keys: modKeys,
		},
	}
	if d.opts.TTL > 0 {
		ttl := int32(d.opts.TTL / time.Second)
		indexes = append(indexes, mongo.IndexModel{
			Keys: bsonx.Doc{
				{
					Key:   "time_info.created",
					Value: bsonx.Int32(1),
				},
			},
			Options: options.Index().SetExpireAfterSeconds(ttl),
		})
	}
	_, err := d.getCollection().Indexes().CreateMany(ctx, indexes)

	return errors.Wrap(err, "problem building indexes")
}

func (d *mongoDriver) Close() {
	if d.canceler != nil {
		d.canceler()
	}
}

func buildCompoundID(n, id string) string { return fmt.Sprintf("%s.%s", n, id) }

func (d *mongoDriver) getIDFromName(name string) string {
	if d.opts.UseGroups {
		return buildCompoundID(d.opts.GroupName, name)
	}

	return name
}

func (d *mongoDriver) processNameForUsers(j *registry.JobInterchange) {
	if !d.opts.UseGroups {
		return
	}

	j.Name = j.Name[len(d.opts.GroupName)+1:]
}

func (d *mongoDriver) processJobForGroup(j *registry.JobInterchange) {
	if !d.opts.UseGroups {
		return
	}

	j.Group = d.opts.GroupName
	j.Name = buildCompoundID(d.opts.GroupName, j.Name)
}

func (d *mongoDriver) modifyQueryForGroup(q bson.M) {
	if !d.opts.UseGroups {
		return
	}

	q["group"] = d.opts.GroupName
}

func (d *mongoDriver) Get(ctx context.Context, name string) (amboy.Job, error) {
	j := &registry.JobInterchange{}

	res := d.getCollection().FindOne(ctx, bson.M{"_id": d.getIDFromName(name)})
	if err := res.Err(); err != nil {
		return nil, errors.Wrapf(err, "GET problem fetching '%s'", name)
	}

	if err := res.Decode(j); err != nil {
		return nil, errors.Wrapf(err, "GET problem decoding '%s'", name)
	}

	d.processNameForUsers(j)

	output, err := j.Resolve(d.opts.Format)
	if err != nil {
		return nil, errors.Wrapf(err,
			"GET problem converting '%s' to job object", name)
	}

	return output, nil
}

func (d *mongoDriver) Put(ctx context.Context, j amboy.Job) error {
	job, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	d.processJobForGroup(job)

	if _, err = d.getCollection().InsertOne(ctx, job); err != nil {
		return errors.Wrapf(err, "problem saving new job %s", j.ID())
	}

	return nil
}

func getAtomicQuery(owner, jobName string, modCount int) bson.M {
	timeoutTs := time.Now().Add(-amboy.LockTimeout)

	return bson.M{
		"_id": jobName,
		"$or": []bson.M{
			// owner and modcount should match, which
			// means there's an active lock but we own it.
			//
			// The modcount is +1 in the case that we're
			// looking to update and update the modcount
			// (rather than just save, as in the Complete
			// case).
			{
				"status.owner":     owner,
				"status.mod_count": bson.M{"$in": []int{modCount, modCount - 1}},
				"status.mod_ts":    bson.M{"$gt": timeoutTs},
			},
			// modtime is older than the lock timeout,
			// regardless of what the other data is,
			{"status.mod_ts": bson.M{"$lte": timeoutTs}},
		},
	}
}

func isMongoDupKey(err error) bool {
	wce, ok := err.(mongo.WriteConcernError)
	if !ok {
		return false
	}
	return wce.Code == 11000 || wce.Code == 11001 || wce.Code == 12582 || wce.Code == 16460 && strings.Contains(wce.Message, " E11000 ")
}

func (d *mongoDriver) Save(ctx context.Context, j amboy.Job) error {
	name := j.ID()
	stat := j.Status()
	stat.ErrorCount = len(stat.Errors)
	stat.ModificationTime = time.Now()
	j.SetStatus(stat)

	job, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	d.processJobForGroup(job)
	query := getAtomicQuery(d.instanceID, job.Name, stat.ModificationCount)
	res, err := d.getCollection().ReplaceOne(ctx, query, job)
	if err != nil {
		if isMongoDupKey(err) {
			grip.Debug(message.Fields{
				"id":        d.instanceID,
				"service":   "amboy.queue.mongo",
				"operation": "save job",
				"name":      name,
				"is_group":  d.opts.UseGroups,
				"group":     d.opts.GroupName,
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

func (d *mongoDriver) Jobs(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)
	go func() {
		defer close(output)
		q := bson.M{}
		d.modifyQueryForGroup(q)

		iter, err := d.getCollection().Find(ctx, q, options.Find().SetSort(bson.M{"status.mod_ts": -1}))
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"id":        d.instanceID,
				"service":   "amboy.queue.mongo",
				"is_group":  d.opts.UseGroups,
				"group":     d.opts.GroupName,
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
					"service":   "amboy.queue.mongo",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
					"operation": "job iterator",
					"message":   "problem reading job from cursor",
				}))

				continue
			}

			d.processNameForUsers(j)

			job, err = j.Resolve(d.opts.Format)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mongo",
					"operation": "job iterator",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
					"message":   "problem converting job obj",
				}))
				continue
			}

			output <- job
		}

		grip.Error(message.WrapError(iter.Err(), message.Fields{
			"id":        d.instanceID,
			"service":   "amboy.queue.mongo",
			"is_group":  d.opts.UseGroups,
			"group":     d.opts.GroupName,
			"operation": "job iterator",
			"message":   "database interface error",
		}))
	}()
	return output
}

func (d *mongoDriver) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	output := make(chan amboy.JobStatusInfo)
	go func() {
		defer close(output)
		q := bson.M{}
		d.modifyQueryForGroup(q)

		iter, err := d.getCollection().Find(ctx,
			q,
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
				"service":   "amboy.queue.mongo",
				"operation": "job status iterator",
				"message":   "problem with query",
				"is_group":  d.opts.UseGroups,
				"group":     d.opts.GroupName,
			}))
			return
		}

		for iter.Next(ctx) {
			j := &registry.JobInterchange{}
			if err := iter.Decode(j); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.monto",
					"operation": "job status iterator",
					"message":   "problem converting job obj",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
				}))
				continue
			}
			d.processNameForUsers(j)
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

func (d *mongoDriver) getNextQuery() bson.M {
	now := time.Now()
	qd := bson.M{
		"$or": []bson.M{
			{
				"status.completed": false,
				"status.in_prog":   false,
			},
			{
				"status.completed": false,
				"status.mod_ts":    bson.M{"$lte": now.Add(-amboy.LockTimeout)},
				"status.in_prog":   true,
			},
		},
	}

	d.modifyQueryForGroup(qd)

	timeLimits := bson.M{}
	if d.opts.CheckWaitUntil {
		timeLimits["time_info.wait_until"] = bson.M{"$lte": now}
	}
	if d.opts.CheckDispatchBy {
		timeLimits["$or"] = []bson.M{
			{"time_info.dispatch_by": bson.M{"$gt": now}},
			{"time_info.dispatch_by": time.Time{}},
		}
	}
	if len(timeLimits) > 0 {
		qd = bson.M{"$and": []bson.M{qd, timeLimits}}
	}
	return qd
}

func (d *mongoDriver) Next(ctx context.Context) amboy.Job {
	var (
		qd     bson.M
		job    amboy.Job
		misses int64
	)

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
			qd = d.getNextQuery()
			iter, err := d.getCollection().Find(ctx, qd, opts)
			if err != nil {
				grip.Debug(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mongo",
					"operation": "retrieving next job",
					"message":   "problem generating query",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
				}))
				return nil
			}

		CURSOR:
			for iter.Next(ctx) {
				if err = iter.Decode(j); err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"id":        d.instanceID,
						"service":   "amboy.queue.mongo",
						"operation": "converting next job",
						"message":   "problem reading document from cursor",
						"is_group":  d.opts.UseGroups,
						"group":     d.opts.GroupName,
					}))
					// try for the next thing in the iterator if we can
					continue CURSOR
				}

				job, err = j.Resolve(d.opts.Format)
				if err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"id":        d.instanceID,
						"service":   "amboy.queue.mongo",
						"operation": "converting document",
						"message":   "problem converting job from intermediate form",
						"is_group":  d.opts.UseGroups,
						"group":     d.opts.GroupName,
					}))
					// try for the next thing in the iterator if we can
					continue CURSOR
				}

				if job.TimeInfo().IsStale() {
					var res *mongo.DeleteResult

					res, err = d.getCollection().DeleteOne(ctx, bson.M{"_id": job.ID()})
					msg := message.Fields{
						"id":          d.instanceID,
						"service":     "amboy.queue.mongo",
						"num_deleted": res.DeletedCount,
						"message":     "found stale job",
						"operation":   "job staleness check",
						"job":         job.ID(),
						"job_type":    job.Type().Name,
						"is_group":    d.opts.UseGroups,
						"group":       d.opts.GroupName,
					}
					grip.Warning(message.WrapError(err, msg))
					grip.NoticeWhen(err == nil, msg)
					continue CURSOR
				}

				break CURSOR
			}

			if err = iter.Err(); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mongo",
					"message":   "problem reported by iterator",
					"operation": "retrieving next job",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
				}))
				return nil
			}

			if err = iter.Close(ctx); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mongo",
					"message":   "problem closing iterator",
					"operation": "retrieving next job",
					"is_group":  d.opts.UseGroups,
					"group":     d.opts.GroupName,
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

func (d *mongoDriver) Stats(ctx context.Context) amboy.QueueStats {
	coll := d.getCollection()

	var numJobs int64
	var err error
	if d.opts.UseGroups {
		numJobs, err = coll.CountDocuments(ctx, bson.M{"group": d.opts.GroupName})
	} else {
		numJobs, err = coll.EstimatedDocumentCount(ctx)
	}

	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"service":    "amboy.queue.mongo",
		"collection": coll.Name(),
		"operation":  "queue stats",
		"is_group":   d.opts.UseGroups,
		"group":      d.opts.GroupName,
		"message":    "problem counting all jobs",
	}))

	pendingQuery := bson.M{"status.completed": false}
	d.modifyQueryForGroup(pendingQuery)
	pending, err := coll.CountDocuments(ctx, pendingQuery)
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"service":    "amboy.queue.mongo",
		"collection": coll.Name(),
		"operation":  "queue stats",
		"is_group":   d.opts.UseGroups,
		"group":      d.opts.GroupName,
		"message":    "problem counting pending jobs",
	}))

	lockedQuery := bson.M{"status.completed": false, "status.in_prog": true}
	d.modifyQueryForGroup(lockedQuery)
	numLocked, err := coll.CountDocuments(ctx, lockedQuery)
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"service":    "amboy.queue.mongo",
		"collection": coll.Name(),
		"is_group":   d.opts.UseGroups,
		"group":      d.opts.GroupName,
		"operation":  "queue stats",
		"message":    "problem counting locked jobs",
	}))

	return amboy.QueueStats{
		Total:     int(numJobs),
		Pending:   int(pending),
		Completed: int(numJobs - pending),
		Running:   int(numLocked),
	}
}
