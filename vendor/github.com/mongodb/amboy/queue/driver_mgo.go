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
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// mgoDriver is a type that represents and wraps a queues
// persistence of jobs *and* locks to a mgoDriver instance.
type mgoDriver struct {
	session    *mgo.Session
	opts       MongoDBOptions
	name       string
	instanceID string
	canceler   context.CancelFunc
	mu         sync.RWMutex
}

// NewMgoDriver creates a driver object given a name, which
// serves as a prefix for collection names, and a MongoDB connection
func NewMgoDriver(name string, opts MongoDBOptions) Driver {
	host, _ := os.Hostname() // nolint

	if !opts.Format.IsValid() {
		opts.Format = amboy.BSON
	}

	return &mgoDriver{
		name:       name,
		opts:       opts,
		instanceID: fmt.Sprintf("%s.%s.%s", name, host, uuid.NewV4()),
	}
}

// OpenNewMgoDriver constructs and opens a new MongoDB driver instance
// using the specified session. It is equivalent to calling
// NewMgo() and calling *MongoDB.Open().
func OpenNewMgoDriver(ctx context.Context, name string, opts MongoDBOptions, session *mgo.Session) (Driver, error) {
	d := NewMgoDriver(name, opts).(*mgoDriver)

	if err := d.start(ctx, session.Clone()); err != nil {
		return nil, errors.Wrap(err, "problem starting driver")
	}

	return d, nil
}

func (d *mgoDriver) ID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceID
}

// Open creates a connection to mgoDriver, and returns an error if
// there's a problem connecting.
func (d *mgoDriver) Open(ctx context.Context) error {
	if d.canceler != nil {
		return nil
	}

	session, err := mgo.Dial(d.opts.URI)
	if err != nil {
		return errors.Wrapf(err, "problem opening connection to mongodb at '%s", d.opts.URI)
	}

	return errors.Wrap(d.start(ctx, session), "problem starting driver")
}

func (d *mgoDriver) start(ctx context.Context, session *mgo.Session) error {
	dCtx, cancel := context.WithCancel(ctx)
	d.canceler = cancel

	session.SetSafe(&mgo.Safe{WMode: "majority"})

	d.mu.Lock()
	d.session = session
	d.mu.Unlock()

	startAt := time.Now()
	go func() {
		<-dCtx.Done()
		grip.Info(message.Fields{
			"message": "closing session for mongodb driver",
			"id":      d.instanceID,
			"uptime":  time.Since(startAt),
			"span":    time.Since(startAt).String(),
			"service": "amboy.queue.mgo",
		})
	}()

	if err := d.setupDB(); err != nil {
		return errors.Wrap(err, "problem setting up database")
	}

	return nil
}

func (d *mgoDriver) getJobsCollection() (*mgo.Session, *mgo.Collection) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	session := d.session.Copy()

	return session, session.DB(d.opts.DB).C(addJobsSuffix(d.name))
}

func (d *mgoDriver) setupDB() error {
	if d.opts.SkipIndexBuilds {
		return nil
	}

	catcher := grip.NewCatcher()
	session, jobs := d.getJobsCollection()
	defer session.Close()

	indexKey := []string{
		"status.completed",
		"status.in_prog",
	}
	if d.opts.CheckWaitUntil {
		indexKey = append(indexKey, "time_info.wait_until")
	}
	if d.opts.CheckDispatchBy {
		indexKey = append(indexKey, "time_info.dispatch_by")
	}

	// priority must be at the end for the sort
	if d.opts.Priority {
		indexKey = append(indexKey, "priority")
	}

	catcher.Add(jobs.EnsureIndexKey(indexKey...))
	catcher.Add(jobs.EnsureIndexKey("status.mod_ts"))
	if d.opts.TTL > 0 {
		catcher.Add(jobs.EnsureIndex(mgo.Index{
			Key:         []string{"time_info.created"},
			ExpireAfter: d.opts.TTL,
		}))
	}

	return errors.Wrap(catcher.Resolve(), "problem building indexes")
}

// Close terminates the connection to the database server.
func (d *mgoDriver) Close() {
	if d.canceler != nil {
		d.canceler()
	}
}

// Get takes the name of a job and returns an amboy.Job object from
// the persistence layer for the job matching that unique id.
func (d *mgoDriver) Get(_ context.Context, name string) (amboy.Job, error) {
	session, jobs := d.getJobsCollection()
	defer session.Close()

	j := &registry.JobInterchange{}

	err := jobs.FindId(name).One(j)

	if err != nil {
		return nil, errors.Wrapf(err, "GET problem fetching '%s'", name)
	}

	output, err := j.Resolve(d.opts.Format)
	if err != nil {
		return nil, errors.Wrapf(err,
			"GET problem converting '%s' to job object", name)
	}

	return output, nil
}

func getAtomicQuery(owner, jobName string, modCount int) bson.M {
	timeoutTs := time.Now().Add(-amboy.LockTimeout)

	return bson.M{
		"_id": jobName,
		"$or": []bson.M{
			// owner and modcount should match, which
			// means there's an active lock but we own it.
			{
				"status.owner":     owner,
				"status.mod_count": modCount,
				"status.mod_ts":    bson.M{"$gt": timeoutTs},
			},
			// modtime is older than the lock timeout,
			// regardless of what the other data is,
			{"status.mod_ts": bson.M{"$lte": timeoutTs}},
		},
	}
}

// Put inserts the job into the collection, returning an error when that job already exists.
func (d *mgoDriver) Put(_ context.Context, j amboy.Job) error {
	job, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	name := j.ID()
	session, jobs := d.getJobsCollection()
	defer session.Close()

	if err = jobs.Insert(job); err != nil {
		return errors.Wrapf(err, "problem saving new job %s", name)
	}

	return nil
}

// Save takes a job object and updates that job in the persistence
// layer. Replaces or updates an existing job with the same ID.
func (d *mgoDriver) Save(_ context.Context, j amboy.Job) error {
	name := j.ID()
	session, jobs := d.getJobsCollection()
	defer session.Close()

	stat := j.Status()
	stat.ErrorCount = len(stat.Errors)
	stat.ModificationTime = time.Now()
	j.SetStatus(stat)

	job, err := registry.MakeJobInterchange(j, d.opts.Format)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	query := getAtomicQuery(d.instanceID, name, stat.ModificationCount)
	err = jobs.Update(query, job)
	if err != nil {
		if mgo.IsDup(errors.Cause(err)) {
			grip.Debug(message.Fields{
				"id":        d.instanceID,
				"service":   "amboy.queue.mgo",
				"operation": "save job",
				"name":      name,
				"outcome":   "duplicate key error, ignoring stale job",
			})

			return nil
		}

		return errors.Wrapf(err, "problem saving document %s", name)
	}

	return nil
}

// Jobs returns a channel containing all jobs persisted by this
// driver. This includes all completed, pending, and locked
// jobs. Errors, including those with connections to MongoDB or with
// corrupt job documents, are logged.
func (d *mgoDriver) Jobs(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)
	go func() {
		defer close(output)

		session, jobs := d.getJobsCollection()
		defer session.Close()

		results := jobs.Find(nil).Sort("-status.mod_ts").Iter()
		defer results.Close()
		j := &registry.JobInterchange{}
		for results.Next(j) {
			job, err := j.Resolve(d.opts.Format)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mgo",
					"operation": "job iterator",
					"message":   "problem converting job obj",
				}))

				if ctx.Err() != nil {
					return
				}

				continue
			}
			select {
			case <-ctx.Done():
				return
			case output <- job:
				continue
			}
		}

		grip.Error(message.WrapError(results.Err(), message.Fields{
			"id":        d.instanceID,
			"service":   "amboy.queue.mgo",
			"operation": "job iterator",
			"message":   "database interface error",
		}))
	}()
	return output
}

// JobStats returns job status documents for all jobs in the storage layer.
//
// This implementation returns documents in reverse modification time.
func (d *mgoDriver) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	output := make(chan amboy.JobStatusInfo)
	go func() {
		defer close(output)
		session, jobs := d.getJobsCollection()
		defer session.Close()

		results := jobs.Find(nil).Select(bson.M{
			"_id":    1,
			"status": 1,
		}).Sort("-status.mod_ts").Iter()
		defer results.Close()

		j := &registry.JobInterchange{}
		for results.Next(j) {
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

func (d *mgoDriver) getNextQuery() bson.M {
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

// Next returns one job, not marked complete from the database.
func (d *mgoDriver) Next(ctx context.Context) amboy.Job {
	session, jobs := d.getJobsCollection()
	if session == nil || jobs == nil {
		return nil
	}
	defer session.Close()

	j := &registry.JobInterchange{}

	var (
		qd     bson.M
		err    error
		misses int64
		job    amboy.Job
	)

	qd = d.getNextQuery()
	query := jobs.Find(qd).Batch(4)
	if d.opts.Priority {
		query = query.Sort("-priority")
	}
	iter := query.Iter()

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if !iter.Next(j) {
				misses++
				if err = iter.Close(); err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"id":        d.instanceID,
						"service":   "amboy.queue.mgo",
						"message":   "problem closing iterator",
						"operation": "retrieving next job",
						"misses":    misses,
					}))
					return nil
				}

				timer.Reset(time.Duration(misses * rand.Int63n(int64(d.opts.WaitInterval))))
				qd = d.getNextQuery()
				query := jobs.Find(qd).Batch(4)
				if d.opts.Priority {
					query = query.Sort("-priority")
				}
				iter = query.Iter()
				continue
			}

			job, err = j.Resolve(d.opts.Format)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mgo",
					"operation": "converting next job",
					"message":   "problem converting job object from mongodb",
					"misses":    misses,
				}))

				// try for the next thing in the iterator if we can
				timer.Reset(time.Nanosecond)
				continue
			}

			if job.TimeInfo().IsStale() {
				err = jobs.RemoveId(job.ID())
				msg := message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mgo",
					"message":   "found stale job",
					"operation": "job staleness check",
					"job":       job.ID(),
					"job_type":  job.Type().Name,
				}
				grip.Warning(message.WrapError(err, msg))
				grip.NoticeWhen(err == nil, msg)
				continue
			}

			if err = iter.Close(); err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"id":        d.instanceID,
					"service":   "amboy.queue.mgo",
					"message":   "problem closing iterator",
					"operation": "returning next job",
					"misses":    misses,
					"job_id":    job.ID(),
				}))
				return nil
			}

			return job
		}

	}
}

// Stats returns a Stats object that contains information about the
// state of the queue in the persistence layer. This operation
// performs a number of asynchronous queries to collect data, and in
// an active system with a number of active queues, stats may report
// incongruous data.
func (d *mgoDriver) Stats(_ context.Context) amboy.QueueStats {
	session, jobs := d.getJobsCollection()
	defer session.Close()

	numJobs, err := jobs.Count()
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"service":    "amboy.queue.mgo",
		"collection": jobs.Name,
		"operation":  "queue stats",
		"message":    "problem counting all jobs",
	}))

	pending, err := jobs.Find(bson.M{"status.completed": false}).Count()
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"service":    "amboy.queue.mgo",
		"collection": jobs.Name,
		"operation":  "queue stats",
		"message":    "problem counting pending jobs",
	}))

	numLocked, err := jobs.Find(bson.M{"status.completed": false, "status.in_prog": true}).Count()
	grip.Warning(message.WrapError(err, message.Fields{
		"id":         d.instanceID,
		"service":    "amboy.queue.mgo",
		"collection": jobs.Name,
		"operation":  "queue stats",
		"message":    "problem counting locked jobs",
	}))

	return amboy.QueueStats{
		Total:     numJobs,
		Pending:   pending,
		Completed: numJobs - pending,
		Running:   numLocked,
	}
}
