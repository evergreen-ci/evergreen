package amboy

import (
	"context"
	"time"

	"github.com/mongodb/amboy/dependency"
)

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

	// Returns a pointer to a JobType object that Queue
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

	// TimeInfo reports the start/end time of jobs, as well as
	// providing for a "wait until" functionality that queues can
	// use to schedule jobs in the future. The update method, only
	// updates non-zero methods.
	TimeInfo() JobTimeInfo
	UpdateTimeInfo(JobTimeInfo)

	// Provides access to the job's priority value, which some
	// queues may use to order job dispatching. Most Jobs
	// implement these values by composing the
	// amboy/priority.Value type.
	Priority() int
	SetPriority(int)

	// AddError allows another actor to annotate the job with an
	// error.
	AddError(error)
	// Error returns an error object if the task was an
	// error. Typically if the job has not run, this is nil.
	Error() error
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
// in the Job interface.e
type JobStatusInfo struct {
	ID                string    `bson:"id,omitempty" json:"id,omitempty" yaml:"id,omitempty"`
	Owner             string    `bson:"owner" json:"owner" yaml:"owner"`
	Completed         bool      `bson:"completed" json:"completed" yaml:"completed"`
	InProgress        bool      `bson:"in_prog" json:"in_progress" yaml:"in_progress"`
	ModificationTime  time.Time `bson:"mod_ts" json:"mod_time" yaml:"mod_time"`
	ModificationCount int       `bson:"mod_count" json:"mod_count" yaml:"mod_count"`
	ErrorCount        int       `bson:"err_count" json:"err_count" yaml:"err_count"`
	Errors            []string  `bson:"errors,omitempty" json:"errors,omitempty" yaml:"errors,omitempty"`
}

// JobTimeInfo stores timing information for a job and is used by both
// the Runner and Job implementations to track how long jobs take to
// execute. Additionally, the Queue implementations __may__ use this
// data to delay execution of a job when WaitUntil refers to a time
// in the future.
type JobTimeInfo struct {
	Created   time.Time     `bson:"created,omitempty" json:"created,omitempty" yaml:"created,omitempty"`
	Start     time.Time     `bson:"start" json:"start,omitempty" yaml:"start,omitempty"`
	End       time.Time     `bson:"end" json:"end,omitempty" yaml:"end,omitempty"`
	WaitUntil time.Time     `bson:"wait_until" json:"wait_until,omitempty" yaml:"wait_until,omitempty"`
	MaxTime   time.Duration `bson:"max_time" json:"max_time,omitempty" yaml:"max_time,omitempty"`
}

// Duration is a convenience function to return a duration for a job.
func (j JobTimeInfo) Duration() time.Duration { return j.End.Sub(j.Start) }

// Queue describes a very simple Job queue interface that allows users
// to define Job objects, add them to a worker queue and execute tasks
// from that queue. Queue implementations may run locally or as part
// of a distributed application, with multiple workers and submitter
// Queue instances, which can support different job dispatching and
// organization properties.
type Queue interface {
	// Used to add a job to the queue. Should only error if the
	// Queue cannot accept jobs.
	Put(Job) error

	// Given a job id, get that job. The second return value is a
	// Boolean, which indicates if the named job had been
	// registered by a Queue.
	Get(string) (Job, bool)

	// Returns the next job in the queue. These calls are
	// blocking, but may be interrupted with a canceled context.
	Next(context.Context) Job

	// Makes it possible to detect if a Queue has started
	// dispatching jobs to runners.
	Started() bool

	// Used to mark a Job complete and remove it from the pending
	// work of the queue.
	Complete(context.Context, Job)

	// Returns a channel that produces completed Job objects.
	Results(context.Context) <-chan Job

	// Returns a channel that produces the status objects for all
	// jobs in the queue, completed and otherwise.
	JobStats(context.Context) <-chan JobStatusInfo

	// Returns an object that contains statistics about the
	// current state of the Queue.
	Stats() QueueStats

	// Getter for the Runner implementation embedded in the Queue
	// instance.
	Runner() Runner

	// Setter for the Runner implementation embedded in the Queue
	// instance. Permits runtime substitution of interfaces, but
	// implementations are not expected to permit users to change
	// runner implementations after starting the Queue.
	SetRunner(Runner) error

	// Begins the execution of the job Queue, using the embedded
	// Runner.
	Start(context.Context) error
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

	// Termaintes all in progress work and waits for processes to
	// return.
	Close()
}

// AbortableRunner provides a superset of the Runner interface but
// allows callers to abort jobs by ID.
type AbortableRunner interface {
	Runner

	IsRunning(string) bool
	RunningJobs() []string
	Abort(context.Context, string) error
	AbortAll(context.Context)
}
