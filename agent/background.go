package agent

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

func (a *Agent) startHeartbeat(ctx context.Context, tc *taskContext, heartbeat chan<- string) {
	defer recovery.LogStackTraceAndContinue("heartbeat background process")
	heartbeatInterval := defaultHeartbeatInterval
	if a.opts.HeartbeatInterval != 0 {
		heartbeatInterval = a.opts.HeartbeatInterval
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	failed, signalBeat := a.doHeartbeat(ctx, tc, 0)
	if signalBeat != "" {
		heartbeat <- signalBeat
		return
	}

	for {
		select {
		case <-ticker.C:
			failed, signalBeat = a.doHeartbeat(ctx, tc, failed)
			if signalBeat != "" {
				heartbeat <- signalBeat
				return
			}
		case <-ctx.Done():
			grip.Info("Heartbeat ticker canceled")
			heartbeat <- evergreen.TaskFailed
			return
		}
	}
}

func (a *Agent) doHeartbeat(ctx context.Context, tc *taskContext, failed int) (int, string) {
	abort, err := a.comm.Heartbeat(ctx, tc.task)
	if abort {
		grip.Info("Task aborted")
		return failed, evergreen.TaskFailed
	}
	if err != nil {
		if err.Error() == client.HTTPConflictError {
			return failed, evergreen.TaskConflict
		}
		failed++
		grip.Errorf("Error sending heartbeat (%d failed attempts): %s", failed, err)
	} else {
		grip.Debug("Sent heartbeat")
	}

	if failed > maxHeartbeats {
		grip.Error(errors.New("Exceeded max heartbeats"))
		return failed, evergreen.TaskFailed

	}

	return failed, ""
}

func (a *Agent) startIdleTimeoutWatch(ctx context.Context, tc *taskContext, cancel context.CancelFunc) {
	defer recovery.LogStackTraceAndContinue("idle timeout watcher")
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Idle timeout watch canceled")
			return
		case <-ticker.C:
			timeout := tc.getCurrentTimeout()
			timeSinceLastMessage := time.Since(a.comm.LastMessageAt())

			if timeSinceLastMessage > timeout {
				tc.logger.Execution().Errorf("Hit idle timeout (no message on stdout for more than %s)", timeout)
				tc.reachTimeOut()
				return
			}
		}
	}
}

func (a *Agent) startMaxExecTimeoutWatch(ctx context.Context, tc *taskContext, d time.Duration, cancel context.CancelFunc) {
	timer := time.NewTimer(d)
	defer recovery.LogStackTraceAndContinue("exec timeout watcher")
	defer timer.Stop()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Exec timeout watch canceled")
			return
		case <-timer.C:
			tc.logger.Execution().Errorf("Hit exec timeout (%s)", d)
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
