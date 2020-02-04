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
	"strings"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/pkg/errors"
)

// Base is a type that all new checks should compose, and provides
// an implementation of most common Job methods which most jobs
// need not implement themselves.
type Base struct {
	TaskID         string        `bson:"name" json:"name" yaml:"name"`
	JobType        amboy.JobType `bson:"job_type" json:"job_type" yaml:"job_type"`
	RequiredScopes []string      `bson:"required_scopes" json:"required_scopes" yaml:"required_scopes"`

	priority int
	timeInfo amboy.JobTimeInfo
	status   amboy.JobStatusInfo
	dep      dependency.Manager
	mutex    sync.RWMutex
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

		b.status.Errors = append(b.status.Errors, err.Error())
	}
}

// HasErrors checks the stored errors in the object and reports if
// there are any stored errors. This operation is thread safe, but not
// part of the Job interface.
func (b *Base) HasErrors() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return len(b.status.Errors) > 0
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

// Lock allows pools to modify the state of a job before saving it to
// the queue to take the lock. The value of the argument should
// uniquely identify the runtime instance of the queue that holds the
// lock, and the method returns an error if the lock cannot be
// acquired.
func (b *Base) Lock(id string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.status.InProgress && time.Since(b.status.ModificationTime) < amboy.LockTimeout && b.status.Owner != id {
		return errors.Errorf("cannot take lock for '%s' because lock has been held for %s by %s",
			id, time.Since(b.status.ModificationTime), b.status.Owner)
	}
	b.status.InProgress = true
	b.status.Owner = id
	b.status.ModificationTime = time.Now()
	b.status.ModificationCount++
	return nil
}

// Unlock attempts to remove the current lock state in the job, if
// possible.
func (b *Base) Unlock(id string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.status.InProgress && time.Since(b.status.ModificationTime) < amboy.LockTimeout && b.status.Owner != id {
		return
	}

	b.status.InProgress = false
	b.status.ModificationTime = time.Now()
	b.status.ModificationCount++
	b.status.Owner = ""
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

	if len(b.status.Errors) == 0 {
		return nil
	}

	return errors.New(strings.Join(b.status.Errors, "\n"))
}

// Priority returns the priority value, and is part of the amboy.Job
// interface.
func (b *Base) Priority() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.priority
}

// SetPriority allows users to set the priority of a job, and is part
// of the amboy.Job interface.
func (b *Base) SetPriority(p int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.priority = p
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

	return b.timeInfo
}

// UpdateTimeInfo updates the stored value of time in the job, but
// does *not* modify fields that are unset in the input document.
func (b *Base) UpdateTimeInfo(i amboy.JobTimeInfo) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !i.Created.IsZero() {
		b.timeInfo.Created = i.Created
	}

	if !i.Start.IsZero() {
		b.timeInfo.Start = i.Start
	}

	if !i.End.IsZero() {
		b.timeInfo.End = i.End
	}

	if !i.WaitUntil.IsZero() {
		b.timeInfo.WaitUntil = i.WaitUntil
	}

	if !i.DispatchBy.IsZero() {
		b.timeInfo.DispatchBy = i.DispatchBy
	}

	if i.MaxTime != 0 {
		b.timeInfo.MaxTime = i.MaxTime
	}
}

// SetScopes overrides the jobs current scopes with those from the
// argument. To unset scopes, pass nil to this method.
func (b *Base) SetScopes(scopes []string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(scopes) == 0 {
		b.RequiredScopes = nil
		return
	}

	b.RequiredScopes = scopes
}

// Scopes returns the required scopes for the job.
func (b *Base) Scopes() []string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if len(b.RequiredScopes) == 0 {
		return nil
	}

	return b.RequiredScopes

}
