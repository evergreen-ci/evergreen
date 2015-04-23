package agent

import "time"

const (
	// execution timeouts
	InitialSetupTimeout = time.Minute * 5
	InitialSetupStage   = "initial-setup-stage"
)

type TimeoutWatcher struct {
	executionStage  string
	timeoutDuration time.Duration
	timer           *time.Timer
	stop            chan bool
	disabled        bool
}

func (self *TimeoutWatcher) SetTimeoutDuration(timeoutDur time.Duration) {
	self.timeoutDuration = timeoutDur
}

//CheckIn() resets the idle timer to zero.
func (self *TimeoutWatcher) CheckIn() {
	if self.timer != nil && !self.disabled {
		self.timer.Reset(self.timeoutDuration)
	}
}

//NotifyTimeouts() sends a signal on sigChan whenever the timeout threshold of
//the current execution stage is reached.
func (self *TimeoutWatcher) NotifyTimeouts(sigChan chan AgentSignal) {
	go func() {
		if self.timeoutDuration <= 0 {
			panic("can't wait for timeouts with negative duration")
		}

		self.timer = time.NewTimer(self.timeoutDuration)
		select {
		case <-self.timer.C:
			//if execution reaches here, it's timed out.
			//send the time out signal on sigChan
			sigChan <- IdleTimeout
			return
		case <-self.stop:
			return
		}
	}()
}
