package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
)

// startHeartbeat runs the task heartbeat. The heartbeat is responsible for two
// things:
//  1. It communicates with the app server to indicate that the agent is still
//     alive and running its task. If the app server does not receive a
//     heartbeat for a long time while the task is still running, then it can
//     assume the task has somehow stopped running and can choose to system-fail
//     the task.
//  2. It decides if/when to abort a task. If it receives an explicit message
//     from the app server to abort (e.g. the user requested the task to abort),
//     then it triggers the running task to abort by cancelling
//     preAndMainCancel. It may also choose to abort in certain edge cases such
//     as repeatedly failing to heartbeat.
func (a *Agent) startHeartbeat(ctx context.Context, preAndMainCancel context.CancelFunc, tc *taskContext) {
	defer recovery.LogStackTraceAndContinue("heartbeat background process")
	heartbeatInterval := defaultHeartbeatInterval
	if a.opts.HeartbeatInterval != 0 {
		heartbeatInterval = a.opts.HeartbeatInterval
	}

	var numRepeatedFailures int
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	var hasSentAbort bool
	for {
		select {
		case <-ticker.C:
			signalBeat, err := a.doHeartbeat(ctx, tc)
			if err != nil {
				numRepeatedFailures++
			} else {
				numRepeatedFailures = 0
			}
			if hasSentAbort {
				// Once abort has been received and passed along to the task
				// running in the foreground, the heartbeat should continue to
				// signal to the app server that post tasks are still running.
				// This is a best-effort attempt, since there's no graceful way
				// to handle heartbeat failures when abort was already sent.
				if numRepeatedFailures == maxHeartbeats {
					tc.logger.Task().Error("Hit max heartbeat attempts when task is already aborted, task is at risk of timing out if it runs for much longer.")
				}
				continue
			}
			if signalBeat == evergreen.TaskFailed {
				tc.logger.Task().Error("Heartbeat received signal to abort task.")
				preAndMainCancel()
				hasSentAbort = true
				continue
			}
			if numRepeatedFailures == maxHeartbeats {
				tc.logger.Task().Error("Hit max heartbeat attempts, aborting task.")
				preAndMainCancel()
				hasSentAbort = true
			}
		case <-ctx.Done():
			if !hasSentAbort {
				preAndMainCancel()
			}
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

type timeoutWatcherOptions struct {
	// tc is the task context for the current running task.
	tc *taskContext
	// kind is the kind of timeout that's being waited for.
	kind timeoutType
	// getTimeout returns the timeout.
	getTimeout func() time.Duration
	// canMarkTimeoutFailure indicates whether the timeout watcher can mark the
	// task as having hit a timeout that can fail the task.
	canMarkTimeoutFailure bool
}

// startTimeoutWatcher waits until the given timeout is hit for an operation. If
// the watcher has run for longer than the timeout, then it marks the task as
// having hit the timeout and cancels the running operation.
func (a *Agent) startTimeoutWatcher(ctx context.Context, operationCancel context.CancelFunc, opts timeoutWatcherOptions) {
	defer recovery.LogStackTraceAndContinue(fmt.Sprintf("%s timeout watcher", opts.kind))
	defer operationCancel()
	ticker := time.NewTicker(time.Second)
	timeTickerStarted := time.Now()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			grip.Infof("%s timeout watcher canceled.", opts.kind)
			return
		case <-ticker.C:
			timeout := opts.getTimeout()
			timeSinceTickerStarted := time.Since(timeTickerStarted)

			if timeSinceTickerStarted > timeout {
				opts.tc.logger.Task().Errorf("Hit %s timeout (%s).", opts.kind, timeout)
				if opts.canMarkTimeoutFailure {
					opts.tc.reachTimeOut(opts.kind, timeout)
				}
				return
			}
		}
	}
}

// getCallbackTimeout returns the callback timeout for the task.
func (tc *taskContext) getCallbackTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig != nil && tc.taskConfig.Project != nil && tc.taskConfig.Project.CallbackTimeout != 0 {
		return time.Duration(tc.taskConfig.Project.CallbackTimeout) * time.Second
	}
	return defaultCallbackCmdTimeout
}

// getPreTimeout returns the timeout for the pre block.
func (tc *taskContext) getPreTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig != nil && tc.taskConfig.Project != nil && tc.taskConfig.Project.PreTimeoutSecs != 0 {
		return time.Duration(tc.taskConfig.Project.PreTimeoutSecs) * time.Second
	}

	return defaultPreTimeout
}

// getPostTimeout returns the timeout for the post block.
func (tc *taskContext) getPostTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig != nil && tc.taskConfig.Project != nil && tc.taskConfig.Project.PostTimeoutSecs != 0 {
		return time.Duration(tc.taskConfig.Project.PostTimeoutSecs) * time.Second
	}
	return defaultPostTimeout
}
