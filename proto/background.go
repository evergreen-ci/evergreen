package proto

import (
	"errors"
	"time"

	"golang.org/x/net/context"
)

func (a *Agent) startHeartbeat(ctx context.Context, tc taskContext, heartbeat chan<- struct{}) {
	heartbeatInterval := defaultHeartbeatInterval
	if a.opts.HeartbeatInterval != 0 {
		heartbeatInterval = a.opts.HeartbeatInterval
	}
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	failed := 0
	for {
		select {
		case <-ticker.C:
			abort, err := a.comm.Heartbeat(ctx, tc.task)
			if abort {
				tc.logger.Execution().Info("Task aborted")
				return
			}
			if err != nil {
				failed++
				tc.logger.Execution().Errorf("Error sending heartbeat (%d failed attempts): %s", failed, err)
			} else {
				tc.logger.Execution().Debug("Sent heartbeat")
				failed = 0
			}
			if failed > maxHeartbeats {
				err := errors.New("Exceeded max heartbeats")
				tc.logger.Execution().Error(err)
				a.killProcs(tc)
				heartbeat <- struct{}{}
				return
			}
		case <-ctx.Done():
			tc.logger.Execution().Info("Heartbeat ticker canceled")
			return
		}
	}
}

func (a *Agent) startIdleTimeoutWatch(ctx context.Context, tc taskContext, idleTimeout chan<- struct{}, resetIdleTimeout <-chan time.Duration) {
	timer := time.NewTimer(defaultIdleTimeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			tc.logger.Execution().Error("Hit idle timeout")
			idleTimeout <- struct{}{}
			return
		case d := <-resetIdleTimeout:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(d)
		case <-ctx.Done():
			tc.logger.Execution().Info("Idle timeout watch canceled")
			return
		}
	}
}

func (a *Agent) startMaxExecTimeoutWatch(ctx context.Context, tc taskContext, d time.Duration, execTimeout chan<- struct{}) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			tc.logger.Execution().Error("Hit exec timeout")
			execTimeout <- struct{}{}
			return
		case <-ctx.Done():
			tc.logger.Execution().Info("Exec timeout watch canceled")
			return
		}
	}
}

// withCallbackTimeout creates a context with a timeout set either to the project's
// callback timeout if it has one or to the defaultCallbackCmdTimeout.
func (a *Agent) withCallbackTimeout(ctx context.Context, tc taskContext) (context.Context, context.CancelFunc) {
	timeout := defaultCallbackCmdTimeout
	if tc.taskConfig.Project.CallbackTimeout != 0 {
		timeout = time.Duration(tc.taskConfig.Project.CallbackTimeout) * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
