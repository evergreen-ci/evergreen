/*Package job provides tools and generic implementations of jobs for
amboy Queues.

Base Metadata

The Base type provides an implementation of the amboy.Job interface
that does *not* have a Run method, and can be embedded in your own job
implementations to avoid implemented duplicated common
functionality. The type also implements several methods which are not
part of the Job interface for error handling (e.g. AddError and
HasErrors), and methods for marking tasks complete and setting the ID
(e.g. MarkComplete and SetID).

All job implementations should use this functionality, although there
are some situations where jobs may want independent implementation of
the Job interface, including: easier construction for use from the
REST interface, needing or wanting a more constrained public
interface, or needing more constrained options for some values
(e.g. Dependency, Priority).
*/
package job

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
)

// Base is a type that all new checks should compose, and provides
// an implementation of most common Job methods which most jobs
// need not implement themselves.
type Base struct {
	TaskID        string            `bson:"name" json:"name" yaml:"name"`
	JobType       amboy.JobType     `bson:"job_type" json:"job_type" yaml:"job_type"`
	Errors        []string          `bson:"errors" json:"errors" yaml:"errors"`
	PriorityValue int               `bson:"priority" json:"priority" yaml:"priority"`
	TaskTimeInfo  amboy.JobTimeInfo `bson:"time_info" json:"time_info" yaml:"time_info"`

	status amboy.JobStatusInfo
	dep    dependency.Manager
	mutex  sync.RWMutex
}

////////////////////////////////////////////////////////////////////////
//
// Safe methods for manipulating the object.
//
////////////////////////////////////////////////////////////////////////

// MarkComplete signals that the job is complete, and is not part of
// the Job interface.
func (b *Base) MarkComplete() {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.status.Completed = true
}

// AddError takes an error object and if it is non-nil, tracks it
// internally. This operation is thread safe, but not part of the Job
// interface.
func (b *Base) AddError(err error) {
	if err != nil {
		b.mutex.Lock()
		defer b.mutex.Unlock()

		b.Errors = append(b.Errors, fmt.Sprintf("%+v", err))
	}
}

// HasErrors checks the stored errors in the object and reports if
// there are any stored errors. This operation is thread safe, but not
// part of the Job interface.
func (b *Base) HasErrors() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return len(b.Errors) > 0
}

// SetID makes it possible to change the ID of an amboy.Job. It is not
// part of the amboy.Job interface.
func (b *Base) SetID(n string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.TaskID = n
}

////////////////////////////////////////////////////////////////////////
//
// Implementation of common interface members.
//
////////////////////////////////////////////////////////////////////////

// ID returns the name of the job, and is a component of the Job
// interface.
func (b *Base) ID() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.TaskID
}

// Type returns the JobType specification for this object, and
// is a component of the Job interface.
func (b *Base) Type() amboy.JobType {
	return b.JobType
}

// Dependency returns an amboy Job dependency interface object, and is
// a component of the Job interface.
func (b *Base) Dependency() dependency.Manager {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.dep
}

// SetDependency allows you to inject a different Job dependency
// object, and is a component of the Job interface.
func (b *Base) SetDependency(d dependency.Manager) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.dep = d
}

// Error returns all of the error objects produced by the job.
func (b *Base) Error() error {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if len(b.Errors) == 0 {
		return nil
	}

	return errors.New(strings.Join(b.Errors, "\n"))
}

// Priority returns the priority value, and is part of the amboy.Job
// interface.
func (b *Base) Priority() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.PriorityValue
}

// SetPriority allows users to set the priority of a job, and is part
// of the amboy.Job interface.
func (b *Base) SetPriority(p int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.PriorityValue = p
}

// Status returns the current state of the job including information
// useful for locking for compatibility with remote queues that
// require managing exclusive access to a job.
func (b *Base) Status() amboy.JobStatusInfo {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.status
}

// SetStatus resets the Status object of a Job document without. It is
// part of the Job interface and used by remote queues.
func (b *Base) SetStatus(s amboy.JobStatusInfo) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.status = s

}

// TimeInfo returns the job's TimeInfo object. The runner
// implementations are responsible for updating these values.
func (b *Base) TimeInfo() amboy.JobTimeInfo {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.TaskTimeInfo
}

// UpdateTimeInfo updates the stored value of time in the job, but
// does *not* modify fields that are unset in the input document.
func (b *Base) UpdateTimeInfo(i amboy.JobTimeInfo) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !i.Start.IsZero() {
		b.TaskTimeInfo.Start = i.Start
	}

	if !i.End.IsZero() {
		b.TaskTimeInfo.End = i.End
	}

	if !i.WaitUntil.IsZero() {
		b.TaskTimeInfo.WaitUntil = i.WaitUntil
	}
}
