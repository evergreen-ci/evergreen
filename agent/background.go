package agent

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (a *Agent) startHeartbeat(ctx context.Context, tc *taskContext, heartbeat chan<- string) {
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
				grip.Info("Task aborted")
				heartbeat <- evergreen.TaskUndispatched
				return
			}
			if err != nil {
				if err.Error() == client.HTTPConflictError {
					heartbeat <- evergreen.TaskConflict
					return
				}
				failed++
				grip.Errorf("Error sending heartbeat (%d failed attempts): %s", failed, err)
			} else {
				grip.Debug("Sent heartbeat")
				failed = 0
			}
			if failed > maxHeartbeats {
				grip.Error(errors.New("Exceeded max heartbeats"))
				heartbeat <- evergreen.TaskFailed
				return
			}
		case <-ctx.Done():
			grip.Info("Heartbeat ticker canceled")
			return
		}
	}
}

func (a *Agent) startIdleTimeoutWatch(ctx context.Context, tc *taskContext, cancel context.CancelFunc) {
	timeoutInterval := defaultIdleTimeout
	if a.opts.IdleTimeoutInterval != 0 {
		timeoutInterval = a.opts.IdleTimeoutInterval
	}

	timer := time.NewTimer(timeoutInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Idle timeout watch canceled context")
			return
		case <-timer.C:
			if taskTimeout := tc.getCurrentTimeout(); taskTimeout != 0 {
				timeoutInterval = taskTimeout
			}

			// check the last time the idle timeout was updated.
			nextTimeout := timeoutInterval - time.Since(a.comm.LastMessageAt())
			if nextTimeout <= 0 {
				tc.logger.Execution().Error("Hit idle timeout")
				tc.reachTimeOut()
				cancel()
			}
			timer.Reset(nextTimeout)
		}
	}
}

func (a *Agent) startMaxExecTimeoutWatch(ctx context.Context, tc *taskContext, d time.Duration, cancel context.CancelFunc) {
	defer cancel()
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Exec timeout watch canceled")
			return
		case <-timer.C:
			tc.logger.Execution().Error("Hit exec timeout")
			tc.reachTimeOut()
			return
		}
	}
}

// withCallbackTimeout creates a context with a timeout set either to the project's
// callback timeout if it has one or to the defaultCallbackCmdTimeout.
func (a *Agent) withCallbackTimeout(ctx context.Context, tc *taskContext) (context.Context, context.CancelFunc) {
	timeout := defaultCallbackCmdTimeout
	if tc.taskConfig.Project.CallbackTimeout != 0 {
		timeout = time.Duration(tc.taskConfig.Project.CallbackTimeout) * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
