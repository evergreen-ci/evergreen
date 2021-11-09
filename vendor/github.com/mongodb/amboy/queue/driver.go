package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
)

// remoteQueueDriver describes the interface between a queue and an out of
// process persistence layer, like a database.
type remoteQueueDriver interface {
	ID() string
	Open(context.Context) error
	Close(context.Context) error

	// Get finds a job by job ID. For retryable jobs, this returns the latest
	// job attempt.
	Get(context.Context, string) (amboy.Job, error)
	// GetAttempt returns a retryable job by job ID and attempt number. If used
	// to find a non-retryable job, this should return nil job and an error.
	GetAttempt(ctx context.Context, id string, attempt int) (amboy.Job, error)
	// GetAllAttempts returns all attempts of a retryable job by job ID. If used
	// to find a non-retryable job, this should return no jobs and an error.
	GetAllAttempts(ctx context.Context, id string) ([]amboy.Job, error)
	// Put inserts a new job in the backing storage.
	Put(context.Context, amboy.Job) error
	// Save updates an existing job in the backing storage. Implementations may
	// not allow calls to Save to run concurrently.
	Save(context.Context, amboy.Job) error
	// CompleteAndPut updates an existing job toComplete and inserts a new job
	// toPut atomically. Implementations may not allow calls to CompleteAndPut
	// to run concurrently.
	CompleteAndPut(ctx context.Context, toComplete amboy.Job, toPut amboy.Job) error

	Jobs(context.Context) <-chan amboy.Job
	// RetryableJobs returns retryable jobs, subject to a filter.
	RetryableJobs(context.Context, retryableJobFilter) <-chan amboy.Job
	Next(context.Context) amboy.Job

	Stats(context.Context) amboy.QueueStats
	JobInfo(context.Context) <-chan amboy.JobInfo
	Complete(context.Context, amboy.Job) error

	LockTimeout() time.Duration

	SetDispatcher(Dispatcher)
	Dispatcher() Dispatcher
}

// retryableJobFilter represents a query filter on retryable jobs.
type retryableJobFilter string

const (
	// RetryableJobAll refers to all retryable jobs.
	retryableJobAll retryableJobFilter = "all-retryable"
	// RetryableJobAllRetrying refers to all retryable jobs that are currently
	// waiting to retry.
	retryableJobAllRetrying retryableJobFilter = "all-retrying"
	// RetryableJobActiveRetrying refers to retryable jobs that have recently
	// retried.
	retryableJobActiveRetrying retryableJobFilter = "active-retrying"
	// RetryableJobStaleRetrying refers to retryable jobs that should be
	// retrying but have not done so recently.
	retryableJobStaleRetrying retryableJobFilter = "stale-retrying"
)

// MongoDBOptions is a struct passed to the MongoDB driver constructor to
// communicate MongoDB-specific settings about the driver's behavior and
// operation.
type MongoDBOptions struct {
	URI                      string
	DB                       string
	GroupName                string
	UseGroups                bool
	Priority                 bool
	CheckWaitUntil           bool
	CheckDispatchBy          bool
	SkipQueueIndexBuilds     bool
	SkipReportingIndexBuilds bool
	Format                   amboy.Format
	WaitInterval             time.Duration
	// TTL sets the number of seconds for a TTL index on the "info.created"
	// field. If set to zero, the TTL index will not be created and
	// and documents may live forever in the database.
	TTL time.Duration
	// LockTimeout overrides the default job lock timeout if set.
	LockTimeout time.Duration
	// SampleSize is the number of jobs that the driver will consider from the
	// next available ones. If it samples from the available jobs, the order of
	// next jobs are randomized. By default, the driver does not sample from the
	// next available jobs. SampleSize cannot be used if Priority is true.
	SampleSize int
}

// defaultMongoDBURI is the default URI to connect to a MongoDB instance.
const defaultMongoDBURI = "mongodb://localhost:27017"

// DefaultMongoDBOptions constructs a new options object with default
// values: connecting to a MongoDB instance on localhost, using the
// "amboy" database, and *not* using priority ordering of jobs.
func DefaultMongoDBOptions() MongoDBOptions {
	return MongoDBOptions{
		URI:                      defaultMongoDBURI,
		DB:                       "amboy",
		Priority:                 false,
		UseGroups:                false,
		CheckWaitUntil:           true,
		CheckDispatchBy:          false,
		SkipQueueIndexBuilds:     false,
		SkipReportingIndexBuilds: false,
		WaitInterval:             time.Second,
		Format:                   amboy.BSON,
		LockTimeout:              amboy.LockTimeout,
		SampleSize:               0,
	}
}

// Validate validates that the required options are given and sets fields that
// are unspecified and have a default value.
func (opts *MongoDBOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.URI == "", "must specify connection URI")
	catcher.NewWhen(opts.DB == "", "must specify database")
	catcher.NewWhen(opts.SampleSize < 0, "sample rate cannot be negative")
	catcher.NewWhen(opts.Priority && opts.SampleSize > 0, "cannot sample next jobs when ordering them by priority")
	catcher.NewWhen(opts.LockTimeout < 0, "lock timeout cannot be negative")
	if opts.LockTimeout == 0 {
		opts.LockTimeout = amboy.LockTimeout
	}
	if !opts.Format.IsValid() {
		opts.Format = amboy.BSON
	}
	return catcher.Resolve()
}
