package agent

import "time"
import "github.com/10gen-labs/slogger/v1"

// HeartbeatTicker manages heartbeat communication with the API server
type HeartbeatTicker struct {
	// Number of consecutive failed heartbeats allowed before signaling a failure
	MaxFailedHeartbeats int

	// Period of time to wait between heartbeat attempts
	Interval time.Duration

	// Channel on which to notify of failed heartbeats or aborted task
	SignalChan chan<- Signal

	// A channel which, when closed, tells the heartbeat ticker should stop.
	stop <-chan struct{}

	// The current count of how many heartbeats have failed consecutively.
	numFailed int

	// Interface which handles sending the actual heartbeat over the network
	TaskCommunicator

	Logger *slogger.Logger
}

func (hbt *HeartbeatTicker) StartHeartbeating() {
	hbt.numFailed = 0

	go func() {
		ticker := time.NewTicker(hbt.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				abort, err := hbt.TaskCommunicator.Heartbeat()
				if err != nil {
					hbt.numFailed++
					hbt.Logger.Logf(slogger.ERROR, "Error sending heartbeat (%v): %v", hbt.numFailed, err)
				} else {
					hbt.numFailed = 0
				}
				if hbt.numFailed == hbt.MaxFailedHeartbeats+1 {
					hbt.Logger.Logf(slogger.ERROR, "Max heartbeats failed - trying to stop...")
					hbt.SignalChan <- HeartbeatMaxFailed
					return
				}
				if abort {
					hbt.SignalChan <- AbortedByUser
					return
				}
			case <-hbt.stop:
				hbt.Logger.Logf(slogger.INFO, "Heartbeat ticker stopping.")
				return
			}
		}
	}()
}
