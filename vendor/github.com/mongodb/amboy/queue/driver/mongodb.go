package driver

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
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const lockTimeout = 5 * time.Minute

// MongoDB is a type that represents and wraps a queues
// persistence of jobs *and* locks to a MongoDB instance.
type MongoDB struct {
	name       string
	mongodbURI string
	dbName     string
	session    *mgo.Session
	canceler   context.CancelFunc
	instanceID string
	priority   bool
	mu         sync.RWMutex
	*LockManager
}

// MongoDBOptions is a struct passed to the NewMongoDB constructor to
// communicate MongoDB specific settings about the driver's behavior
// and operation.
type MongoDBOptions struct {
	URI      string
	DB       string
	Priority bool
}

// DefaultMongoDBOptions constructs a new options object with default
// values: connecting to a MongoDB instance on localhost, using the
// "amboy" database, and *not* using priority ordering of jobs.
func DefaultMongoDBOptions() MongoDBOptions {
	return MongoDBOptions{
		URI:      "mongodb://localhost:27017",
		DB:       "amboy",
		Priority: false,
	}

}

// NewMongoDB creates a driver object given a name, which
// serves as a prefix for collection names, and a MongoDB connection
func NewMongoDB(name string, opts MongoDBOptions) *MongoDB {
	host, _ := os.Hostname()
	d := &MongoDB{
		name:       name,
		dbName:     opts.DB,
		mongodbURI: opts.URI,
		priority:   opts.Priority,
		instanceID: fmt.Sprintf("%s.%s.%s", name, host, uuid.NewV4()),
	}

	d.LockManager = NewLockManager(d.instanceID, d)

	return d
}

// OpenNewMongoDB constructs and opens a new MongoDB driver instance
// using the specified session. It is equivalent to calling
// NewMongoDB() and calling *MongoDB.Open().
func OpenNewMongoDB(ctx context.Context, name string, opts MongoDBOptions, session *mgo.Session) (*MongoDB, error) {
	d := NewMongoDB(name, opts)

	if err := d.start(ctx, session.Copy()); err != nil {
		return nil, errors.Wrap(err, "problem starting driver")
	}

	return d, nil
}

// Open creates a connection to MongoDB, and returns an error if
// there's a problem connecting.
func (d *MongoDB) Open(ctx context.Context) error {
	if d.canceler != nil {
		return nil
	}

	session, err := mgo.Dial(d.mongodbURI)
	if err != nil {
		return errors.Wrapf(err, "problem opening connection to mongodb at '%s", d.mongodbURI)
	}

	return errors.Wrap(d.start(ctx, session), "problem starting driver")
}

func (d *MongoDB) start(ctx context.Context, session *mgo.Session) error {
	dCtx, cancel := context.WithCancel(ctx)
	d.canceler = cancel

	session.SetSafe(&mgo.Safe{WMode: "majority"})

	d.mu.Lock()
	d.session = session
	d.mu.Unlock()

	d.LockManager.Open(ctx)

	go func() {
		<-dCtx.Done()
		grip.Info("closing session for mongodb driver")
	}()

	if err := d.setupDB(); err != nil {
		return errors.Wrap(err, "problem setting up database")
	}

	return nil
}

func (d *MongoDB) getJobsCollection() (*mgo.Session, *mgo.Collection) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	session := d.session.Copy()

	return session, session.DB(d.dbName).C(d.name + ".jobs")
}

func (d *MongoDB) setupDB() error {
	catcher := grip.NewCatcher()
	session, jobs := d.getJobsCollection()
	defer session.Close()

	if d.priority {
		catcher.Add(jobs.EnsureIndexKey("status.completed", "status.in_prog", "priority"))
	} else {
		catcher.Add(jobs.EnsureIndexKey("status.completed", "status.in_prog"))
	}
	catcher.Add(jobs.EnsureIndexKey("status.mod_ts"))

	return errors.Wrap(catcher.Resolve(), "problem building indexes")
}

// Close terminates the connection to the database server.
func (d *MongoDB) Close() {
	if d.canceler != nil {
		d.canceler()
	}
}

// Get takes the name of a job and returns an amboy.Job object from
// the persistence layer for the job matching that unique id.
func (d *MongoDB) Get(name string) (amboy.Job, error) {
	session, jobs := d.getJobsCollection()
	defer session.Close()

	j := &registry.JobInterchange{}

	err := jobs.FindId(name).One(j)
	grip.Debugf("GET operation: [name='%s', payload='%+v' error='%v']", name, j, err)

	if err != nil {
		return nil, errors.Wrapf(err, "GET problem fetching '%s'", name)
	}

	output, err := registry.ConvertToJob(j)
	if err != nil {
		return nil, errors.Wrapf(err,
			"GET problem converting '%s' to job object", name)
	}

	return output, nil
}

func (d *MongoDB) getAtomicQuery(jobName string, stat amboy.JobStatusInfo) bson.M {
	timeoutTs := time.Now().Add(-lockTimeout)

	return bson.M{
		"_id": jobName,
		"$or": []bson.M{
			// if there is no owner, then there can be no lock,
			bson.M{"status.owner": ""},
			// owner and modcount should match, which
			// means there's an active lock but we own it.
			bson.M{
				"status.owner":     d.instanceID,
				"status.mod_count": stat.ModificationCount,
				"status.mod_ts":    bson.M{"$lte": timeoutTs},
			},
			// modtime is older than the lock timeout,
			// regardless of what the other data is,
			bson.M{"status.mod_ts": bson.M{"$gt": timeoutTs}},
		},
	}
}

// Put inserts the job into the collection, returning an error when that job already exists.
func (d *MongoDB) Put(j amboy.Job) error {
	job, err := registry.MakeJobInterchange(j)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	name := j.ID()
	session, jobs := d.getJobsCollection()
	defer session.Close()

	if err = jobs.Insert(job); err != nil {
		return errors.Wrapf(err, "problem saving new job %s", name)
	}

	grip.Debugf("saved job '%s'", name)

	return nil
}

// Save takes a job object and updates that job in the persistence
// layer. Replaces or updates an existing job with the same ID.
func (d *MongoDB) Save(j amboy.Job) error {
	job, err := registry.MakeJobInterchange(j)
	if err != nil {
		return errors.Wrap(err, "problem converting job to interchange format")
	}

	name := j.ID()
	session, jobs := d.getJobsCollection()
	defer session.Close()

	info, err := jobs.Upsert(d.getAtomicQuery(j.ID(), j.Status()), job)
	if err != nil {
		err = errors.Wrapf(err, "problem updating %s: %+v", name, info)
		grip.Warning(err)
		return err
	}

	grip.Debugf("saved job '%s': %+v", name, info)

	return nil
}

// SaveStatus persists only the status document in the job in the
// persistence layer. If the job does not exist, or the underlying
// status document has changed incompatibly this operation produces
// an error.
func (d *MongoDB) SaveStatus(j amboy.Job, stat amboy.JobStatusInfo) error {
	session, jobs := d.getJobsCollection()
	defer session.Close()

	err := jobs.Update(d.getAtomicQuery(j.ID(), j.Status()),
		bson.M{"$set": bson.M{"status": stat}})

	return errors.Wrapf(err, "problem updating status document for %s", j.ID())
}

// Jobs returns a channel containing all jobs persisted by this
// driver. This includes all completed, pending, and locked
// jobs. Errors, including those with connections to MongoDB or with
// corrupt job documents, are logged.
func (d *MongoDB) Jobs() <-chan amboy.Job {
	output := make(chan amboy.Job)
	go func() {
		defer close(output)

		session, jobs := d.getJobsCollection()
		defer session.Close()

		results := jobs.Find(nil).Sort("-status.mod_ts").Iter()
		defer results.Close()
		j := &registry.JobInterchange{}
		for results.Next(j) {
			job, err := registry.ConvertToJob(j)
			if err != nil {
				grip.CatchError(err)
				continue
			}
			output <- job
		}
		grip.CatchError(results.Err())
	}()
	return output
}

// JobStats returns job status documents for all jobs in the storage layer.
//
// This implementation returns documents in reverse modification time.
func (d *MongoDB) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
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
			if ctx.Err() != nil {
				return
			}

			j.Status.ID = j.Name
			output <- j.Status
		}
	}()
	return output
}

// Next returns one job, not marked complete from the database.
func (d *MongoDB) Next(ctx context.Context) amboy.Job {
	session, jobs := d.getJobsCollection()
	if session == nil || jobs == nil {
		return nil
	}
	defer session.Close()
	j := &registry.JobInterchange{}

	query := jobs.Find(bson.M{"status.completed": false, "status.in_prog": false})
	if d.priority {
		query = query.Sort("-priority")
	}

	timer := time.NewTimer(0)
	defer timer.Stop()

	var misses int64
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			err := query.One(j)
			if err != nil {
				if misses < 30 {
					misses++
				}

				if err == mgo.ErrNotFound {
					timer.Reset(time.Duration(misses * rand.Int63n(int64(time.Second))))
					continue
				}
				grip.Warning(message.Fields{
					"message": "problem retreiving jobs from MongoDB",
					"error":   err.Error(),
				})
				return nil
			}

			job, err := registry.ConvertToJob(j)
			if err != nil {
				grip.Warning(message.Fields{
					"message": "problem converting job object from mongodb",
					"error":   err.Error(),
				})
				timer.Reset(time.Duration(misses * rand.Int63n(int64(time.Second))))
				continue
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
func (d *MongoDB) Stats() amboy.QueueStats {
	session, jobs := d.getJobsCollection()
	defer session.Close()

	numJobs, err := jobs.Count()
	grip.ErrorWhenf(err != nil,
		"problem getting count from jobs collection (%s): %+v ",
		jobs.Name, err)

	completed, err := jobs.Find(bson.M{"status.completed": true}).Count()
	grip.ErrorWhenf(err != nil, "problem getting count of pending jobs (%s): %+v ",
		jobs.Name, err)

	numLocked, err := jobs.Find(bson.M{"status.in_prog": true}).Count()
	grip.ErrorWhenf(err != nil,
		"problem getting count of locked Jobs (%s): %+v",
		jobs.Name, err)

	return amboy.QueueStats{
		Total:     numJobs,
		Pending:   numJobs - completed,
		Completed: completed,
		Running:   numLocked,
	}
}
