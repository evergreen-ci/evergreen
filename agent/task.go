package agent

import (
	"context"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

func (a *Agent) startTask(ctx context.Context, tc *taskContext, complete chan<- string) {
	defer func() {
		op := "running task pre and main blocks"
		pErr := recovery.HandlePanicWithError(recover(), nil, op)
		if pErr == nil {
			return
		}
		_ = a.logPanic(tc.logger, pErr, nil, op)
		trySendTaskComplete(tc.logger.Execution(), complete, evergreen.TaskSystemFailed)
	}()

	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		tc.logger.Execution().Error("Marking task as system-failed because setup.initial command is not registered.")
		trySendTaskComplete(tc.logger.Execution(), complete, evergreen.TaskFailed)
		return
	}

	if ctx.Err() != nil {
		tc.logger.Execution().Infof("Stopping task execution before setup: %s", ctx.Err())
		return
	}
	tc.setCurrentCommand(factory())
	a.comm.UpdateLastMessageTime()

	if ctx.Err() != nil {
		tc.logger.Execution().Infof("Stopping task execution during setup: %s", ctx.Err())
		return
	}
	tc.logger.Task().Infof("Task logger initialized (agent version '%s' from Evergreen build revision '%s').", evergreen.AgentVersion, evergreen.BuildRevision)
	tc.logger.Execution().Info("Execution logger initialized.")
	tc.logger.System().Info("System logger initialized.")

	if ctx.Err() != nil {
		tc.logger.Execution().Infof("Stopping task execution: %s", ctx.Err())
		return
	}
	hostname, err := os.Hostname()
	tc.logger.Execution().Info(errors.Wrap(err, "getting hostname"))
	if hostname != "" {
		tc.logger.Execution().Infof("Hostname is '%s'.", hostname)
	}
	tc.logger.Task().Infof("Starting task '%s', execution %d.", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)

	innerCtx, innerCancel := context.WithCancel(ctx)
	defer innerCancel()
	go a.startMaxExecTimeoutWatch(ctx, tc, innerCancel)

	// set up the system stats collector
	tc.statsCollector = NewSimpleStatsCollector(
		tc.logger,
		a.jasper,
		defaultStatsInterval,
		"uptime",
		"df -h",
		"${ps|ps}",
	)
	tc.statsCollector.logStats(innerCtx, tc.taskConfig.Expansions)

	if innerCtx.Err() != nil {
		tc.logger.Execution().Infof("Stopping task execution after setup: %s", innerCtx.Err())
		return
	}

	// notify API server that the task has been started.
	tc.logger.Execution().Info("Reporting task started.")
	if err = a.comm.StartTask(innerCtx, tc.task); err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "marking task started"))
		trySendTaskComplete(tc.logger.Execution(), complete, evergreen.TaskSystemFailed)
		return
	}

	a.killProcs(innerCtx, tc, false, "task is starting")

	if err = a.runPreTaskCommands(innerCtx, tc); err != nil {
		trySendTaskComplete(tc.logger.Execution(), complete, evergreen.TaskFailed)
		return
	}

	if tc.oomTrackerEnabled(a.opts.CloudProvider) {
		tc.logger.Execution().Info("OOM tracker clearing system messages.")
		if err = tc.oomTracker.Clear(innerCtx); err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "clearing OOM tracker system messages"))
		}
	}

	if err = a.runTaskCommands(innerCtx, tc); err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "running task commands"))
		trySendTaskComplete(tc.logger.Execution(), complete, evergreen.TaskFailed)
		return
	}

	trySendTaskComplete(tc.logger.Execution(), complete, evergreen.TaskSucceeded)
}

// trySendTaskComplete attempts to send a task status to the given channel. This
// is a non-blocking operation - if it tries to send the task status but the
// channel is already blocked (either because it is full or there is no consumer
// to receive the result), it will log an error and continue.
func trySendTaskComplete(logger grip.Journaler, complete chan<- string, status string) {
	select {
	case complete <- status:
	default:
		logger.Errorf("Tried sending task status '%s', but complete channel was blocked.", status)
	}
}

func (a *Agent) runPreTaskCommands(ctx context.Context, tc *taskContext) error {
	tc.logger.Task().Info("Running pre-task commands.")
	ctx, preTaskSpan := a.tracer.Start(ctx, "pre-task-commands")
	defer preTaskSpan.End()

	opts := runCommandsOptions{}

	if !tc.ranSetupGroup {
		taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
		if err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for task setup group commands"))
			return nil
		}
		if taskGroup.SetupGroup != nil {
			tc.logger.Task().Infof("Running setup group for task group '%s'.", taskGroup.Name)
			opts.failPreAndPost = taskGroup.SetupGroupFailTask

			var setupGroupCtx context.Context
			var setupGroupCancel context.CancelFunc
			if taskGroup.SetupGroupTimeoutSecs > 0 {
				setupGroupCtx, setupGroupCancel = context.WithTimeout(ctx, time.Duration(taskGroup.SetupGroupTimeoutSecs)*time.Second)
			} else {
				setupGroupCtx, setupGroupCancel = a.withCallbackTimeout(ctx, tc)
			}
			defer setupGroupCancel()

			err = a.runCommandsInBlock(setupGroupCtx, tc, taskGroup.SetupGroup.List(), opts, setupGroupBlock)
			if err != nil {
				tc.logger.Execution().Error(errors.Wrap(err, "running task setup group"))
				if taskGroup.SetupGroupFailTask {
					return err
				}
			}
			tc.logger.Task().Infof("Finished running setup group for task group '%s'.", taskGroup.Name)
		}
		tc.ranSetupGroup = true
	}

	taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for pre-task commands"))
		return nil
	}

	if taskGroup.SetupTask != nil {
		tc.logger.Task().Infof("Running setup task for task group '%s'.", taskGroup.Name)
		opts.failPreAndPost = taskGroup.SetupGroupFailTask
		block := preBlock
		if tc.taskGroup != "" {
			block = setupTaskBlock
		}
		err = a.runCommandsInBlock(ctx, tc, taskGroup.SetupTask.List(), opts, block)
	}
	if err != nil {
		err = errors.Wrap(err, "Running pre-task commands failed")
		tc.logger.Task().Error(err)
		if opts.failPreAndPost {
			return err
		}
	}
	tc.logger.Task().InfoWhen(err == nil, "Finished running pre-task commands.")
	return nil
}

func (tc *taskContext) setCurrentCommand(command command.Command) {
	tc.Lock()
	defer tc.Unlock()
	tc.currentCommand = command

	if tc.logger != nil {
		tc.logger.Execution().Infof("Current command set to %s (%s).", tc.currentCommand.DisplayName(), tc.currentCommand.Type())
	}
}

func (tc *taskContext) getCurrentCommand() command.Command {
	tc.RLock()
	defer tc.RUnlock()
	return tc.currentCommand
}

func (tc *taskContext) setCurrentIdleTimeout(cmd command.Command) {
	tc.Lock()
	defer tc.Unlock()

	var timeout time.Duration
	if cmd == nil {
		timeout = defaultIdleTimeout
	} else if dynamicTimeout := tc.taskConfig.GetIdleTimeout(); dynamicTimeout != 0 {
		timeout = time.Duration(dynamicTimeout) * time.Second
	} else if cmd.IdleTimeout() > 0 {
		timeout = cmd.IdleTimeout()
	} else {
		timeout = defaultIdleTimeout
	}

	tc.setIdleTimeout(timeout)
	if tc.currentCommand != nil {
		tc.logger.Execution().Debugf("Set idle timeout for %s (%s) to %s.",
			tc.currentCommand.DisplayName(), tc.currentCommand.Type(), tc.getIdleTimeout())
	} else {
		tc.logger.Execution().Debugf("Set current idle timeout to %s.", tc.getIdleTimeout())
	}
}

func (tc *taskContext) getCurrentTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	timeout := tc.getIdleTimeout()
	if timeout > 0 {
		return timeout
	}
	return defaultIdleTimeout
}

func (tc *taskContext) reachTimeOut(kind timeoutType, dur time.Duration) {
	tc.Lock()
	defer tc.Unlock()

	tc.setTimedOut(true, kind)
	tc.setTimeoutDuration(dur)
}

func (tc *taskContext) hadTimedOut() bool {
	tc.RLock()
	defer tc.RUnlock()

	return tc.timedOut()
}

func (tc *taskContext) getOomTrackerInfo() *apimodels.OOMTrackerInfo {
	lines, pids := tc.oomTracker.Report()
	if len(lines) == 0 {
		return nil
	}

	return &apimodels.OOMTrackerInfo{
		Detected: true,
		Pids:     pids,
	}
}

func (tc *taskContext) oomTrackerEnabled(cloudProvider string) bool {
	return tc.project.OomTracker && !utility.StringSliceContains(evergreen.ProviderContainer, cloudProvider)
}

func (tc *taskContext) setIdleTimeout(dur time.Duration) {
	tc.timeout.idleTimeoutDuration = dur
}

func (tc *taskContext) getIdleTimeout() time.Duration {
	return tc.timeout.idleTimeoutDuration
}

func (tc *taskContext) setTimedOut(timeout bool, kind timeoutType) {
	tc.timeout.hadTimeout = timeout
	tc.timeout.timeoutType = kind
}

func (tc *taskContext) timedOut() bool {
	return tc.timeout.hadTimeout
}

func (tc *taskContext) setTimeoutDuration(dur time.Duration) {
	tc.timeout.exceededDuration = dur
}

func (tc *taskContext) getTimeoutDuration() time.Duration {
	return tc.timeout.exceededDuration
}

func (tc *taskContext) getTimeoutType() timeoutType {
	return tc.timeout.timeoutType
}

// makeTaskConfig fetches task configuration data required to run the task from the API server.
func (a *Agent) makeTaskConfig(ctx context.Context, tc *taskContext) (*internal.TaskConfig, error) {
	if tc.project == nil {
		grip.Info("Fetching project config.")
		err := a.fetchProjectConfig(ctx, tc)
		if err != nil {
			return nil, err
		}
	}
	grip.Info("Fetching distro configuration.")
	var confDistro *apimodels.DistroView
	var err error
	if a.opts.Mode == HostMode {
		confDistro, err = a.comm.GetDistroView(ctx, tc.task)
		if err != nil {
			return nil, err
		}
	}

	grip.Info("Fetching project ref.")
	confRef, err := a.comm.GetProjectRef(ctx, tc.task)
	if err != nil {
		return nil, err
	}
	if confRef == nil {
		return nil, errors.New("agent retrieved an empty project ref")
	}

	var confPatch *patch.Patch
	if evergreen.IsGitHubPatchRequester(tc.taskModel.Requester) {
		grip.Info("Fetching patch document for GitHub PR request.")
		confPatch, err = a.comm.GetTaskPatch(ctx, tc.task, "")
		if err != nil {
			return nil, errors.Wrap(err, "fetching patch for GitHub PR request")
		}
	}

	grip.Info("Constructing task config.")
	taskConfig, err := internal.NewTaskConfig(a.opts.WorkingDirectory, confDistro, tc.project, tc.taskModel, confRef, confPatch, tc.expansions)
	if err != nil {
		return nil, err
	}
	taskConfig.Redacted = tc.privateVars
	taskConfig.TaskSync = a.opts.SetupData.TaskSync
	taskConfig.EC2Keys = a.opts.SetupData.EC2Keys

	return taskConfig, nil
}

func (tc *taskContext) getExecTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()
	if tc.taskConfig == nil {
		return DefaultExecTimeout
	}
	if dynamicTimeout := tc.taskConfig.GetExecTimeout(); dynamicTimeout > 0 {
		return time.Duration(dynamicTimeout) * time.Second
	}
	if pt := tc.taskConfig.Project.FindProjectTask(tc.taskConfig.Task.DisplayName); pt != nil && pt.ExecTimeoutSecs > 0 {
		return time.Duration(pt.ExecTimeoutSecs) * time.Second
	}
	if tc.taskConfig.Project.ExecTimeoutSecs > 0 {
		return time.Duration(tc.taskConfig.Project.ExecTimeoutSecs) * time.Second
	}
	return DefaultExecTimeout
}

func (tc *taskContext) setTaskConfig(taskConfig *internal.TaskConfig) {
	tc.Lock()
	defer tc.Unlock()
	tc.taskConfig = taskConfig
}

func (tc *taskContext) getTaskConfig() *internal.TaskConfig {
	tc.RLock()
	defer tc.RUnlock()
	return tc.taskConfig
}
