package agent

import (
	"context"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
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
			if signalBeat == evergreen.TaskConflict {
				cancel()
			}
			if signalBeat != "" {
				heartbeat <- signalBeat
				return
			}
			if err != nil {
				failures++
				grip.Errorf("Error sending heartbeat (%d failed attempts): %s", failures, err)
			} else {
				failures = 0
			}
			if failures == maxHeartbeats {
				grip.Error("Hit max heartbeats, aborting task")
				// Presumably this won't work, but we should try to notify the user anyway
				tc.logger.Task().Error("Hit max heartbeats, aborting task")
				heartbeat <- evergreen.TaskFailed
				return
			}
		case <-ctx.Done():
			grip.Info("Heartbeat ticker canceled")
			heartbeat <- evergreen.TaskFailed
			return
		}
	}
}

func (a *Agent) doHeartbeat(ctx context.Context, tc *taskContext) (string, error) {
	abort, err := a.comm.Heartbeat(ctx, tc.task)
	if abort {
		grip.Info("Task aborted")
		return evergreen.TaskFailed, nil
	}
	if err != nil {
		if errors.Cause(err) == client.HTTPConflictError {
			return evergreen.TaskConflict, err
		}
		return "", err
	}
	grip.Debug("Sent heartbeat")
	return "", nil
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

func (a *Agent) startMaxExecTimeoutWatch(ctx context.Context, tc *taskContext, cancel context.CancelFunc) {
	defer recovery.LogStackTraceAndContinue("exec timeout watcher")
	defer cancel()
	ticker := time.NewTicker(time.Second)
	timeTickerStarted := time.Now()
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Exec timeout watch canceled")
			return
		case <-ticker.C:
			timeout := tc.getExecTimeout()
			timeSinceTickerStarted := time.Since(timeTickerStarted)

			if timeSinceTickerStarted > timeout {
				tc.logger.Execution().Errorf("Hit exec timeout (%s)", timeout)
				tc.reachTimeOut()
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

func (a *Agent) startSpotTerminationWatcher(ctx context.Context, cancel context.CancelFunc) {
	defer recovery.LogStackTraceAndContinue("spot termination watcher")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("Spot termination watcher canceled")
			return
		case <-ticker.C:
			if util.SpotHostWillTerminateSoon() {
				grip.Info("Spot instance terminating, so agent is exiting")
				os.Exit(1)
			}
		}
	}
}
