package agent

import "time"
import "github.com/10gen-labs/slogger/v1"

type HeartbeatTicker struct {
	//Number of consecutive failed heartbeats allowed before signaling a failure
	MaxFailedHeartbeats int

	//Period of time to wait between heartbeat attempts
	Interval time.Duration

	//Channel on which to notify of failed heartbeats or aborted task
	SignalChan chan AgentSignal

	//A channel which, when closed, tells the heartbeat ticker should stop.
	stop chan bool

	//The current count of how many heartbeats have failed consecutively.
	numFailed int

	//Interface which handles sending the actual heartbeat over the network
	TaskCommunicator

	Logger *slogger.Logger
}

func (self *HeartbeatTicker) StartHeartbeating() {
	if self.stop != nil {
		panic("Heartbeat goroutine already running!")
	}
	self.numFailed = 0
	self.stop = make(chan bool)
	go func() {
		ticker := time.NewTicker(self.Interval)
		for {
			select {
			case <-ticker.C:
				abort, err := self.TaskCommunicator.Heartbeat()
				if err != nil {
					self.numFailed++
					self.Logger.Logf(slogger.ERROR, "Error sending heartbeat "+
						"(%v): %v", self.numFailed, err)
				} else {
					self.numFailed = 0
				}
				if self.numFailed == self.MaxFailedHeartbeats+1 {
					self.Logger.Logf(slogger.ERROR, "Max heartbeats failed - trying to stop...")
					self.SignalChan <- HeartbeatMaxFailed
					ticker.Stop()
					self.stop = nil
					return
				}
				if abort {
					self.SignalChan <- AbortedByUser
					ticker.Stop()
					self.stop = nil
					return
				}
			case <-self.stop:
				self.Logger.Logf(slogger.INFO, "Heartbeat ticker stopping.")
				ticker.Stop()
				self.stop = nil
				return
			}
		}
	}()
}

func (self *HeartbeatTicker) Stop() {
	self.stop <- true
}
