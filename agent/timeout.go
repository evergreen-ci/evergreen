package agent

import (
	"time"
)

// TimeoutWatcher tracks and handles command timeout within the agent.
type TimeoutWatcher struct {
	duration time.Duration
	timer    *time.Timer
	stop     chan bool
	disabled bool
}

// SetDuration sets the duration after which a timeout is triggered.
func (tw *TimeoutWatcher) SetDuration(duration time.Duration) {
	tw.duration = duration
}

// CheckIn resets the idle timer to zero.
func (tw *TimeoutWatcher) CheckIn() {
	if tw.timer != nil && !tw.disabled {
		tw.timer.Reset(tw.duration)
	}
}

// NotifyTimeouts sends a signal on sigChan whenever the timeout threshold of
// the current execution stage is reached.
func (tw *TimeoutWatcher) NotifyTimeouts(sigChan chan Signal) {
	go func() {
		if tw.duration <= 0 {
			panic("can't wait for timeouts with negative duration")
		}

		tw.timer = time.NewTimer(tw.duration)
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
