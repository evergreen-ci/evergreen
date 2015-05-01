package agent

import "time"
import "github.com/10gen-labs/slogger/v1"

type HeartbeatTicker struct {
	// Number of consecutive failed heartbeats allowed before signaling a failure
	MaxFailedHeartbeats int

	// Period of time to wait between heartbeat attempts
	Interval time.Duration

	// Channel on which to notify of failed heartbeats or aborted task
	SignalChan chan Signal

	// A channel which, when closed, tells the heartbeat ticker should stop.
	stop chan bool

	// The current count of how many heartbeats have failed consecutively.
	numFailed int

	// Interface which handles sending the actual heartbeat over the network
	TaskCommunicator

	Logger *slogger.Logger
}

func (hbt *HeartbeatTicker) StartHeartbeating() {
	if hbt.stop != nil {
		panic("Heartbeat goroutine already running!")
	}
	hbt.numFailed = 0
	hbt.stop = make(chan bool)

	go func() {
		ticker := time.NewTicker(hbt.Interval)
		for {
			select {
			case <-ticker.C:
				abort, err := hbt.TaskCommunicator.Heartbeat()
				if err != nil {
					hbt.numFailed++
					hbt.Logger.Logf(slogger.ERROR, "Error sending heartbeat "+
						"(%v): %v", hbt.numFailed, err)
				} else {
					hbt.numFailed = 0
				}
				if hbt.numFailed == hbt.MaxFailedHeartbeats+1 {
					hbt.Logger.Logf(slogger.ERROR, "Max heartbeats failed - trying to stop...")
					hbt.SignalChan <- HeartbeatMaxFailed
					ticker.Stop()
					hbt.stop = nil
					return
				}
				if abort {
					hbt.SignalChan <- AbortedByUser
					ticker.Stop()
					hbt.stop = nil
					return
				}
			case <-hbt.stop:
				hbt.Logger.Logf(slogger.INFO, "Heartbeat ticker stopping.")
				ticker.Stop()
				hbt.stop = nil
				return
			}
		}
	}()
}

func (hbt *HeartbeatTicker) Stop() {
	hbt.stop <- true
}
