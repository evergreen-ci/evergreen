package amboy

import (
	"context"
	"time"

	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/grip"
)

// LockTimeout describes the default period of time that a queue will respect
// a stale lock from another queue before beginning work on a job.
const LockTimeout = 10 * time.Minute

// Job describes a unit of work. Implementations of Job instances are
// the content of the Queue. The amboy/job package contains several
// general purpose and example implementations. Jobs are responsible,
// primarily via their Dependency property, for determining: if they
// need to run, and what Jobs they depend on. Actual use of the
// dependency system is the responsibility of the Queue implementation.
//
// In most cases, applications only need to implement the Run()
// method, all additional functionality is provided by the job.Base type,
// which can be embedded anonymously in implementations of the Job.
type Job interface {
	// Provides a unique identifier for a job. Queues may error if
	// two jobs have different IDs.
	ID() string

	// The primary execution method for the job. Should toggle the
	// completed state for the job.
	Run(context.Context)

	// Type returns a JobType object that Queue
	// implementations can use to de-serialize tasks.
	Type() JobType

	// Provides access to the job's dependency information, and
	// allows queues to override a dependency (e.g. in a force
	// build state, or as part of serializing dependency objects
	// with jobs.)
	Dependency() dependency.Manager
	SetDependency(dependency.Manager)

	// Provides access to the JobStatusInfo object for the job,
	// which reports the current state.
	Status() JobStatusInfo
	SetStatus(JobStatusInfo)

	// TimeInfo reports the start/end time of jobs, as well as "wait until" and
	// "dispatch by" options that queues can use to schedule jobs in the future.
	// UpdateTimeInfo only modifies non-zero fields.
	TimeInfo() JobTimeInfo
	UpdateTimeInfo(JobTimeInfo)
	// SetTimeInfo is like UpdateTimeInfo but overwrites all time info,
	// including zero fields.
	SetTimeInfo(JobTimeInfo)

	// RetryInfo reports information about the job's retry behavior.
	RetryInfo() JobRetryInfo
	// UpdateRetryInfo method modifies all set fields from the given options.
	UpdateRetryInfo(JobRetryOptions)

	// Provides access to the job's priority value, which some
	// queues may use to order job dispatching. Most Jobs
	// implement these values by composing the
	// amboy/priority.Value type.
	Priority() int
	SetPriority(int)

	// AddError allows another actor to annotate the job with an
	// error.
	AddError(error)
	// AddRetryableError annotates the job with an error and marks the job as
	// needing to retry.
	AddRetryableError(error)
	// Error returns an error object if the task was an
	// error. Typically if the job has not run, this is nil.
	Error() error

	// Lock and Unlock are responsible for handling the locking
	// behavor for the job. Lock is responsible for setting the
	// owner (its argument), incrementing the modification count
	// and marking the job in progress, and returning an error if
	// another worker has access to the job. Unlock is responsible
	// for unsetting the owner and marking the job as
	// not-in-progress, and should be a no-op if the job does not
	// belong to the owner. In general the owner should be the value
	// of queue.ID()
	Lock(owner string, lockTimeout time.Duration) error
	Unlock(owner string, lockTimeout time.Duration)

	// Scopes provide the ability to configure mutual exclusion for a job in a
	// queue. The Scopes method returns the current mutual exclusion locks for
	// the job.
	// SetScopes configures the mutually exclusive lock(s) that a job in a queue
	// should acquire. When called, it does not actually take a lock; rather, it
	// signals the intention to lock within the queue. This is typically called
	// when first initializing the job before enqueueing it; it is invalid for
	// end users to call SetScopes after the job has already dispatched.
	Scopes() []string
	SetScopes([]string)

	// ShouldApplyScopesOnEnqueue allows the scope exclusion functionality to be
	// configured so that exclusion occurs during job dispatch or enqueue.
	ShouldApplyScopesOnEnqueue() bool
	SetShouldApplyScopesOnEnqueue(bool)
}

// JobType contains information about the type of a job, which queues
// can use to serialize objects. All Job implementations must store
// and produce instances of this type that identify the type and
// implementation version.
type JobType struct {
	Name    string `json:"name" bson:"name" yaml:"name"`
	Version int    `json:"version" bson:"version" yaml:"version"`
}

// JobStatusInfo contains information about the current status of a
// job and is reported by the Status and set by the SetStatus methods
// in the Job interface.
type JobStatusInfo struct {
	Owner             string    `bson:"owner" json:"owner" yaml:"owner"`
	Completed         bool      `bson:"completed" json:"completed" yaml:"completed"`
	InProgress        bool      `bson:"in_prog" json:"in_progress" yaml:"in_progress"`
	ModificationTime  time.Time `bson:"mod_ts" json:"mod_time" yaml:"mod_time"`
	ModificationCount int       `bson:"mod_count" json:"mod_count" yaml:"mod_count"`
	ErrorCount        int       `bson:"err_count" json:"err_count" yaml:"err_count"`
	Errors            []string  `bson:"errors,omitempty" json:"errors,omitempty" yaml:"errors,omitempty"`
}

// JobTimeInfo stores timing information for a job and is used by both the
// Runner and Job implementations to track how long jobs take to execute.
type JobTimeInfo struct {
	Created time.Time `bson:"created,omitempty" json:"created,omitempty" yaml:"created,omitempty"`
	Start   time.Time `bson:"start,omitempty" json:"start,omitempty" yaml:"start,omitempty"`
	End     time.Time `bson:"end,omitempty" json:"end,omitempty" yaml:"end,omitempty"`
	// WaitUntil defers execution of a job until a particular time has elapsed.
	// Support for this feature in Queue implementations is optional.
	WaitUntil time.Time `bson:"wait_until" json:"wait_until,omitempty" yaml:"wait_until,omitempty"`
	// DispatchBy is a deadline before which the job must run. Support for this
	// feature in Queue implementations is optional. Queues that support this
	// feature may remove the job if the deadline has passed.
	DispatchBy time.Time `bson:"dispatch_by" json:"dispatch_by,omitempty" yaml:"dispatch_by,omitempty"`
	// MaxTime is the maximum time that the job is allowed to run. If the
	// runtime exceeds this duration, the Queue should abort the job.
	MaxTime time.Duration `bson:"max_time" json:"max_time,omitempty" yaml:"max_time,omitempty"`
}

// JobRetryInfo stores configuration and information for a job that can retry.
// Support for retrying jobs is only supported by RetryableQueues.
type JobRetryInfo struct {
	// Retryable indicates whether the job can use Amboy's built-in retry
	// mechanism. This should typically be set when first initializing the job;
	// it is invalid for end users to modify Retryable once the job has already
	// been dispatched.
	Retryable bool `bson:"retryable" json:"retryable,omitempty" yaml:"retryable,omitempty"`
	// NeedsRetry indicates whether the job is supposed to retry when it is
	// complete. This will only be considered if Retryable is true.
	NeedsRetry bool `bson:"needs_retry" json:"needs_retry,omitempty" yaml:"needs_retry,omitempty"`
	// BaseJobID is the job ID of the original job that was retried, ignoring
	// any additional retry metadata.
	BaseJobID string `bson:"base_job_id,omitempty" json:"base_job_id,omitempty" yaml:"base_job_id,omitempty"`
	// CurrentAttempt is the current attempt number. This is zero-indexed
	// (unless otherwise set on enqueue), so the first time the job attempts to
	// run, its value is 0. Each subsequent retry increments this value.
	CurrentAttempt int `bson:"current_attempt" json:"current_attempt,omitempty" yaml:"current_attempt,omitempty"`
	// MaxAttempts is the maximum number of attempts for a job. This is
	// 1-indexed since it is a count. For example, if this is set to 3, the job
	// will be allowed to run 3 times at most. If unset, the default maximum
	// attempts is 10.
	MaxAttempts int `bson:"max_attempts,omitempty" json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	// DispatchBy reflects the amount of time (relative to when the job is
	// retried) that the retried job has to dispatch for execution. If this
	// deadline elapses, the job will not run. This is analogous to
	// (JobTimeInfo).DispatchBy.
	DispatchBy time.Duration `bson:"dispatch_by,omitempty" json:"dispatch_by,omitempty" yaml:"dispatch_by,omitempty"`
	// WaitUntil reflects the amount of time (relative to when the job is
	// retried) that the retried job has to wait before it can be dispatched for
	// execution. The job will not run until this waiting period elapses. This
	// is analogous to (JobTimeInfo).WaitUntil.
	WaitUntil time.Duration `bson:"wait_until,omitempty" json:"wait_until,omitempty" yaml:"wait_until,omitempty"`
	// Start is the time that the job began retrying.
	Start time.Time `bson:"start,omitempty" json:"start,omitempty" yaml:"start,omitempty"`
	// End is the time that the job finished retrying.
	End time.Time `bson:"end,omitempty" json:"end,omitempty" yaml:"end,omitempty"`
}

// Options returns a JobRetryInfo as its equivalent JobRetryOptions. In other
// words, if the returned result is used with Job.UpdateRetryInfo(), the job
// will be populated with the same information as this JobRetryInfo.
func (i *JobRetryInfo) Options() JobRetryOptions {
	return JobRetryOptions{
		Retryable:      &i.Retryable,
		NeedsRetry:     &i.NeedsRetry,
		CurrentAttempt: &i.CurrentAttempt,
		MaxAttempts:    &i.MaxAttempts,
		DispatchBy:     &i.DispatchBy,
		WaitUntil:      &i.WaitUntil,
		Start:          &i.Start,
		End:            &i.End,
	}
}

// ShouldRetry returns whether or not the associated job is supposed to retry
// upon completion.
func (i JobRetryInfo) ShouldRetry() bool {
	return i.Retryable && i.NeedsRetry
}

const defaultRetryableMaxAttempts = 10

// GetMaxAttempts returns the maximum number of times a job is allowed to
// attempt. It defaults the maximum attempts if it's unset.
func (info JobRetryInfo) GetMaxAttempts() int {
	if info.MaxAttempts <= 0 {
		return defaultRetryableMaxAttempts
	}
	return info.MaxAttempts
}

// GetRemainingAttempts returns the number of times this job is still allow to
// attempt, excluding the current attempt.
func (info JobRetryInfo) GetRemainingAttempts() int {
	remainder := info.GetMaxAttempts() - info.CurrentAttempt - 1
	if remainder < 0 {
		return 0
	}
	return remainder
}

// JobRetryOptions represents configuration options for a job that can retry.
// Their meaning corresponds to the fields in JobRetryInfo, but is more amenable
// to optional input values.
type JobRetryOptions struct {
	Retryable      *bool          `bson:"-" json:"-" yaml:"-"`
	NeedsRetry     *bool          `bson:"-" json:"-" yaml:"-"`
	CurrentAttempt *int           `bson:"-" json:"-" yaml:"-"`
	MaxAttempts    *int           `bson:"-" json:"-" yaml:"-"`
	DispatchBy     *time.Duration `bson:"-" json:"-" yaml:"-"`
	WaitUntil      *time.Duration `bson:"-" json:"-" yaml:"-"`
	Start          *time.Time     `bson:"-" json:"-" yaml:"-"`
	End            *time.Time     `bson:"-" json:"-" yaml:"-"`
}

// Duration is a convenience function to return a duration for a job.
func (j JobTimeInfo) Duration() time.Duration { return j.End.Sub(j.Start) }

// IsStale determines if the job is too old to be dispatched, and if
// so, queues may remove or drop the job entirely.
func (j JobTimeInfo) IsStale() bool {
	if j.DispatchBy.IsZero() {
		return false
	}

	return j.DispatchBy.Before(time.Now())
}

// IsDispatchable determines if the job should be dispatched based on
// the value of WaitUntil.
func (j JobTimeInfo) IsDispatchable() bool {
	return time.Now().After(j.WaitUntil)
}

// Validate ensures that the structure has reasonable values set.
func (j JobTimeInfo) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(!j.DispatchBy.IsZero() && j.WaitUntil.After(j.DispatchBy), "invalid for wait_until to be after dispatch_by")
	catcher.NewWhen(j.Created.IsZero(), "must specify non-zero created timestamp")
	catcher.NewWhen(j.MaxTime < 0, "must specify 0 or positive max_time")

	return catcher.Resolve()
}

// JobInfo provides a view of information for a Job.
type JobInfo struct {
	ID     string
	Type   JobType
	Status JobStatusInfo
	Time   JobTimeInfo
	Retry  JobRetryInfo
}

// NewJobInfo creates a JobInfo from a Job.
func NewJobInfo(j Job) JobInfo {
	return JobInfo{
		ID:     j.ID(),
		Status: j.Status(),
		Time:   j.TimeInfo(),
		Retry:  j.RetryInfo(),
		Type:   j.Type(),
	}
}

// Queue describes a very simple Job queue interface that allows users
// to define Job objects, add them to a worker queue and execute tasks
// from that queue. Queue implementations may run locally or as part
// of a distributed application, with multiple workers and submitter
// Queue instances, which can support different job dispatching and
// organization properties.
type Queue interface {
	// ID returns a unique identifier for the instance of the queue.
	ID() string

	// Put adds a job to the queue.
	Put(context.Context, Job) error

	// Get finds a Job by ID. The boolean return value indicates if the Job was
	// found or not.
	Get(context.Context, string) (Job, bool)

	// Next returns the next available Job to run in the Queue.
	Next(context.Context) Job

	// Info returns information related to management of the Queue.
	Info() QueueInfo

	// Complete marks a Job as completed executing.
	Complete(context.Context, Job) error

	// Save persists the state of a current Job to the underlying storage,
	// generally in support of locking and incremental persistence.
	// Implementations should error if the job does not exist in the Queue or if
	// the Job state within the Queue has been modified to invalidate the
	// in-memory ownership of the Job.
	Save(context.Context, Job) error

	// Results returns a channel that produces completed Job objects.
	Results(context.Context) <-chan Job

	// JobInfo returns a channel that produces the information for all Jobs in
	// the Queue.
	JobInfo(context.Context) <-chan JobInfo

	// Stats returns statistics about the current state of the Queue.
	Stats(context.Context) QueueStats

	// Runner returns the Runner implementation that is running Jobs for the
	// Queue.
	Runner() Runner

	// SetRunner sets the Runner that is running Jobs for the Queue. This
	// permits runtime substitution of Runner implementations. However, Queue
	// implementations are not expected to permit users to change Runner
	// implementations after starting the Queue.
	SetRunner(Runner) error

	// Start begins the execution of Jobs in the Queue.
	Start(context.Context) error

	// Close cleans up all resources used by the Queue.
	Close(context.Context)
}

// QueueInfo describes runtime information associated with a Queue.
type QueueInfo struct {
	Started     bool
	LockTimeout time.Duration
}

// QueueGroup describes a group of queues. Each queue is indexed by a
// string. Users can use these queues if there are many different types
// of work or if the types of work are only knowable at runtime.
type QueueGroup interface {
	// Get a queue with the given index.
	Get(context.Context, string) (Queue, error)

	// Put a queue at the given index.
	Put(context.Context, string, Queue) error

	// Prune old queues.
	Prune(context.Context) error

	// Close the queues.
	Close(context.Context) error

	// Len returns the number of active queues managed in the
	// group.
	Len() int

	// Queues returns all currently registered and running queues
	Queues(context.Context) []string
}

// RetryableQueue is the same as a Queue but supports additional operations for
// retryable jobs.
type RetryableQueue interface {
	// Queue is identical to the standard queue interface, except:
	// For retryable jobs, Get will retrieve the latest attempt of a job by ID.
	// Results will only return completed jobs that are not retrying.
	Queue

	// RetryHandler returns the handler for retrying a job in this queue.
	RetryHandler() RetryHandler
	// SetRetryHandler permits runtime substitution of RetryHandler
	// implementations. Queue implementations are not expected to permit users
	// to change RetryHandler implementations after starting the Queue.
	SetRetryHandler(RetryHandler) error

	// GetAttempt returns the retryable job associated with the given ID and
	// execution attempt. If it cannot find a matching job, it will return
	// ErrJobNotFound. This will only return retryable jobs.
	GetAttempt(ctx context.Context, id string, attempt int) (Job, error)

	// GetAllAttempts returns all execution attempts of a retryable job
	// associated with the given job ID. If it cannot find a matching job, it
	// will return ErrJobNotFound.This will only return retryable jobs.
	GetAllAttempts(ctx context.Context, id string) ([]Job, error)

	// CompleteRetryingAndPut marks an existing job toComplete in the queue (see
	// CompleteRetrying) as finished retrying and inserts a new job toPut in the
	// queue (see Put). Implementations must make this operation atomic.
	CompleteRetryingAndPut(ctx context.Context, toComplete, toPut Job) error

	// CompleteRetrying marks a job that is retrying as finished processing, so
	// that it will no longer retry.
	CompleteRetrying(ctx context.Context, j Job) error
}

// RetryHandler provides a means to retry jobs (see JobRetryOptions) within a
// RetryableQueue.
type RetryHandler interface {
	// SetQueue provides a method to change the RetryableQueue where the job
	// should be retried. Implementations may not be able to change their Queue
	// association after starting.
	SetQueue(RetryableQueue) error
	// Start prepares the RetryHandler to begin processing jobs to retry.
	Start(context.Context) error
	// Started reports if the RetryHandler has started.
	Started() bool
	// Put adds a job that must be retried into the RetryHandler.
	Put(context.Context, Job) error
	// Close aborts all retry work in progress and waits for all work to finish.
	Close(context.Context)
}

// RetryHandlerOptions configures the behavior of a RetryHandler.
type RetryHandlerOptions struct {
	// MaxRetryAttempts is the maximum number of times that the retry handler is
	// allowed to attempt to retry a job before it gives up. This is referring
	// to the retry handler's attempts to internally retry the job and is
	// unrelated to the job's particular max attempt setting.
	MaxRetryAttempts int
	// MaxRetryAttempts is the maximum time that the retry handler is allowed to
	// attempt to retry a job before it gives up.
	MaxRetryTime time.Duration
	// RetryBackoff is how long the retry handler will back off after a failed
	// retry attempt.
	RetryBackoff time.Duration
	// NumWorkers is the maximum number of jobs that are allowed to retry in
	// parallel.
	NumWorkers int
	// MaxCapacity is the total number of jobs that the RetryHandler is allowed
	// to hold in preparation to retry. If MaxCapacity is 0, it will be set to a
	// default maximum capacity. If MaxCapacity is -1, it will have unlimited
	// capacity.
	MaxCapacity int
}

// Validate checks that all retry handler options are valid.
func (opts *RetryHandlerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.MaxRetryAttempts < 0, "cannot have negative max retry attempts")
	catcher.NewWhen(opts.MaxRetryTime < 0, "cannot have negative max retry time")
	catcher.NewWhen(opts.NumWorkers < 0, "cannot have negative worker thread count")
	catcher.NewWhen(opts.MaxCapacity < -1, "cannot have negative max capacity, unless it is -1 for unlimited")
	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	if opts.MaxRetryAttempts == 0 {
		opts.MaxRetryAttempts = 10
	}
	if opts.MaxRetryTime == 0 {
		opts.MaxRetryTime = time.Minute
	}
	if opts.NumWorkers == 0 {
		opts.NumWorkers = 1
	}
	if opts.MaxCapacity == 0 {
		opts.MaxCapacity = 4096
	}
	return nil
}

// IsUnlimitedMaxCapacity returns whether or not the options specify unlimited
// capacity.
func (opts *RetryHandlerOptions) IsUnlimitedMaxCapacity() bool {
	return opts.MaxCapacity == -1
}

// Runner describes a simple worker interface for executing jobs in
// the context of a Queue. Used by queue implementations to run
// tasks. Generally Queue implementations will spawn a runner as part
// of their constructor or Start() methods, but client code can inject
// alternate Runner implementations, as required.
type Runner interface {
	// Reports if the pool has started.
	Started() bool

	// Provides a method to change or set the pointer to the
	// enclosing Queue object after instance creation. Runner
	// implementations may not be able to change their Queue
	// association after starting.
	SetQueue(Queue) error

	// Prepares the runner implementation to begin doing work, if
	// any is required (e.g. starting workers.) Typically called
	// by the enclosing Queue object's Start() method.
	Start(context.Context) error

	// Terminates all in progress work and waits for processes to
	// return.
	Close(context.Context)
}

// AbortableRunner provides a superset of the Runner interface but
// allows callers to abort jobs by ID.
type AbortableRunner interface {
	Runner

	IsRunning(string) bool
	RunningJobs() []string
	Abort(context.Context, string) error
	AbortAll(context.Context) error
}
