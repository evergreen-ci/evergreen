package agent

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
)

func (a *Agent) startHeartbeat(ctx context.Context, cancel context.CancelFunc, tc *taskContext, heartbeat chan<- string) {
	defer recovery.LogStackTraceAndContinue("heartbeat background process")
	heartbeatInterval := defaultHeartbeatInterval
	if a.opts.HeartbeatInterval != 0 {
		heartbeatInterval = a.opts.HeartbeatInterval
	}

	var failures int
	var signalBeat string
	var err error
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			signalBeat, err = a.doHeartbeat(ctx, tc)
			if signalBeat == client.TaskConflict {
				tc.logger.Task().Error("Encountered task conflict while checking heartbeat, aborting task.")
				if err != nil {
					tc.logger.Task().Error(err.Error())
				}
				cancel()
			}
			if signalBeat == evergreen.TaskFailed {
				tc.logger.Task().Error("Heartbeat received signal to abort task.")
				heartbeat <- signalBeat
				return
			}
			if err != nil {
				failures++
			} else {
				failures = 0
			}
			if failures == maxHeartbeats {
				// Presumably this won't work, but we should try to notify the user anyway
				tc.logger.Task().Error("Hit max heartbeat attempts, aborting task.")
				heartbeat <- evergreen.TaskFailed
				return
			}
		case <-ctx.Done():
			heartbeat <- evergreen.TaskFailed
			return
		}
	}
}

func (a *Agent) doHeartbeat(ctx context.Context, tc *taskContext) (string, error) {
	resp, err := a.comm.Heartbeat(ctx, tc.task)
	if resp == evergreen.TaskFailed || resp == client.TaskConflict {
		return resp, err
	}
	return "", err
}

func (a *Agent) startIdleTimeoutWatch(ctx context.Context, tc *taskContext, cancel context.CancelFunc) {
	defer recovery.LogStackTraceAndContinue("idle timeout watcher")
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Idle timeout watcher canceled.")
			return
		case <-ticker.C:
			timeout := tc.getCurrentTimeout()
			timeSinceLastMessage := time.Since(a.comm.LastMessageAt())

			if timeSinceLastMessage > timeout {
				tc.logger.Execution().Errorf("Hit idle timeout (no message on stdout for more than %s).", timeout)
				tc.reachTimeOut(idleTimeout, timeout)
				return
			}
		}
	}
}

func (a *Agent) startMaxExecTimeoutWatch(ctx context.Context, tc *taskContext, cancel context.CancelFunc) {
	defer recovery.LogStackTraceAndContinue("exec timeout watcher")
	defer cancel()
	ticker := time.NewTicker(time.Second)
	timeTickerStarted := time.Now()
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Exec timeout watcher canceled.")
			return
		case <-ticker.C:
			timeout := tc.getExecTimeout()
			timeSinceTickerStarted := time.Since(timeTickerStarted)

			if timeSinceTickerStarted > timeout {
				tc.logger.Execution().Errorf("Hit exec timeout (%s).", timeout)
				tc.reachTimeOut(execTimeout, timeout)
				return
			}
		}
	}
}

// withCallbackTimeout creates a context with a timeout set either to the project's
// callback timeout if it has one or to the defaultCallbackCmdTimeout.
func (a *Agent) withCallbackTimeout(ctx context.Context, tc *taskContext) (context.Context, context.CancelFunc) {
	timeout := defaultCallbackCmdTimeout
	taskConfig := tc.getTaskConfig()
	if taskConfig != nil && taskConfig.Project != nil && taskConfig.Project.CallbackTimeout != 0 {
		timeout = time.Duration(taskConfig.Project.CallbackTimeout) * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
