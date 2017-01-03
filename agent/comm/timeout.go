package comm

import (
	"sync"
	"time"
)

// TimeoutWatcher tracks and handles command timeout within the agent.
type TimeoutWatcher struct {
	duration time.Duration
	timer    *time.Timer
	stop     <-chan struct{}
	disabled bool

	mutex sync.Mutex
}

func NewTimeoutWatcher(stopChan <-chan struct{}) *TimeoutWatcher {
	// TODO: replace this with a context for cancellation, and be
	// able to eliminate the constructor
	return &TimeoutWatcher{stop: stopChan}
}

// SetDuration sets the duration after which a timeout is triggered.
func (tw *TimeoutWatcher) SetDuration(duration time.Duration) {
	tw.mutex.Lock()
	defer tw.mutex.Unlock()

	tw.duration = duration
}

// CheckIn resets the idle timer to zero.
func (tw *TimeoutWatcher) CheckIn() {
	tw.mutex.Lock()
	defer tw.mutex.Unlock()

	if tw.timer != nil && !tw.disabled {
		tw.timer.Reset(tw.duration)
	}
}

// NotifyTimeouts sends a signal on sigChan whenever the timeout threshold of
// the current execution stage is reached.
func (tw *TimeoutWatcher) NotifyTimeouts(sigChan chan<- Signal) {
	go func() {
		tw.mutex.Lock()
		if tw.duration <= 0 {
			tw.mutex.Unlock()
			panic("can't wait for timeouts with negative duration")
		}

		if tw.timer == nil {
			tw.timer = time.NewTimer(tw.duration)
		} else {
			tw.timer.Reset(tw.duration)
		}
		tw.mutex.Unlock()

		select {
		case <-tw.timer.C:
			// if execution reaches here, it's timed out.
			// send the time out signal on sigChan
			sigChan <- IdleTimeout
			return
		case <-tw.stop:
			return
		}
	}()
}
