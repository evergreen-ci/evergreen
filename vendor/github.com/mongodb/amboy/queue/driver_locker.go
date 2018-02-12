package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type lockPings map[string]time.Time

// LockManager describes the component of the Driver interface that
// handles job mutexing.
type LockManager interface {
	Lock(amboy.Job) error
	Unlock(amboy.Job) error
}

// lockManager provides an implementation of the Lock and Unlock
// methods to be composed by amboy/queue.Driver implementations.
//
// lockManagers open a single background process that updates all
// tracked locks at an interval, less than the configured lockTimeout
// to avoid locks growing stale.
type lockManager struct {
	name    string
	d       Driver
	timeout time.Duration
	ops     chan func(lockPings)
}

// NewLockManager configures a Lock manager for use in Driver
// implementations. This operation does *not* start the background
// thread. The name *must* be unique per driver/queue combination, to
// ensure that each driver/queue can have exclusive locks over jobs.
func NewLockManager(ctx context.Context, name string, d Driver) LockManager {
	l := newLockManager(name, d)
	l.start(ctx)
	return l
}

func newLockManager(name string, d Driver) *lockManager {
	return &lockManager{
		name:    name,
		d:       d,
		ops:     make(chan func(lockPings)),
		timeout: lockTimeout,
	}
}

func (l *lockManager) start(ctx context.Context) { go l.lockPinger(ctx) }

func (l *lockManager) lockPinger(ctx context.Context) {
	activeLocks := lockPings{}

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case op := <-l.ops:
			op(activeLocks)
		case <-timer.C:
			startAt := time.Now()
			nextLoopAt := time.Now().Add(l.timeout / 2)
			for name, ts := range activeLocks {
				if nextLoopAt.After(ts) {
					// make sure that we loop
					// again when at least one of the
					// locks will be ready for an update.
					nextLoopAt = ts
				}

				if ts.Before(startAt) {
					// don't update locks that are too fresh.
					continue
				}

				j, err := l.d.Get(name)
				if err != nil {
					// remove locks from the
					// current queue if they no
					// longer exist
					delete(activeLocks, name)
					continue
				}

				// Update the important metadata
				stat := j.Status()
				stat.ModificationCount++
				stat.ModificationTime = time.Now()

				if ctx.Err() != nil {
					return
				}

				if err := l.d.SaveStatus(j, stat); err != nil {
					continue
				}

				activeLocks[name] = time.Now().Add(l.timeout / 2)
			}

			timer.Reset(-time.Since(nextLoopAt))
		}
	}
}

func (l *lockManager) addPing(name string) {
	wait := make(chan struct{})
	l.ops <- func(pings lockPings) {
		pings[name] = time.Now().Add(l.timeout)
		close(wait)
	}
	<-wait
}

func (l *lockManager) removePing(name string) {
	wait := make(chan struct{})
	l.ops <- func(pings lockPings) {
		delete(pings, name)
		close(wait)
	}
	<-wait
}

// Lock takes an exclusive lock on the specified job and instructs a
// background process to update it continually.
//
// Returns an error if the Lock is already locked or if there's a
// problem updating the document.
func (l *lockManager) Lock(j amboy.Job) error {
	if j == nil {
		return errors.New("cannot unlock nil job")
	}

	// We get the status object, modify it, and then let the save
	// function and the query handle the "do we own this? is the
	// lock active? has it changed since we last saw it?"

	stat := j.Status()

	// previous versions of this allowed operation allowed one
	// client to "take" the lock more than once. This covered a
	// deadlock/bug in queue implementations in marking jobs
	// complete, *and* allowed queues implementations with more
	// than one worker, to potentially repeat work.
	if stat.InProgress && stat.ModificationTime.Add(l.timeout).After(time.Now()) {
		return errors.Errorf("cannot take lock, job locked at %s by %s, for job: '%s'",
			stat.ModificationTime, stat.Owner, j.ID())
	}

	stat.ModificationTime = time.Now()
	stat.ModificationCount++
	stat.Owner = l.name
	stat.InProgress = true
	j.SetStatus(stat)

	if err := l.d.SaveStatus(j, stat); err != nil {
		return errors.Wrap(err, "problem saving stat")
	}

	l.addPing(j.ID())

	return nil
}

// Unlock removes this process' exclusive lock on the specified job
// and instructs the background job to begin updating the lock
// regularly. Returns an error if no lock exists or if there was a
// problem updating the lock in the persistence layer.
func (l *lockManager) Unlock(j amboy.Job) error {
	if j == nil {
		return errors.New("cannot unlock nil job")
	}

	stat := j.Status()
	stat.ModificationCount++
	stat.ModificationTime = time.Now()
	stat.InProgress = false
	stat.Owner = ""
	j.SetStatus(stat)

	l.removePing(j.ID())

	if err := l.d.SaveStatus(j, stat); err != nil {
		grip.Info(message.WrapError(err, message.Fields{
			"job":  j.ID(),
			"stat": stat,
		}))

		return errors.Wrapf(err, "problem unlocking '%s'", j.ID())
	}

	return nil
}
