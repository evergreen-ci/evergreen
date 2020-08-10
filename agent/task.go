package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func (a *Agent) startTask(ctx context.Context, tc *taskContext, complete chan<- string) {
	defer func() {
		if p := recover(); p != nil {
			var pmsg string
			if ps, ok := p.(string); ok {
				pmsg = ps
			} else {
				pmsg = fmt.Sprintf("%+v", p)
			}

			m := message.Fields{
				"operation": "running task",
				"panic":     pmsg,
				"stack":     message.NewStack(1, "").Raw(),
			}
			grip.Alert(m)
			select {
			case complete <- evergreen.TaskFailed:
				tc.getCurrentCommand().SetType(evergreen.CommandTypeSystem)
				grip.Debug("marked task as system-failed after panic")
			default:
				grip.Debug("marking task system failed during panic handling, but complete channel was blocked")
			}
			if tc.logger != nil && !tc.logger.Closed() {
				tc.logger.Execution().Error("Evergreen agent hit a runtime error, marking task system-failed")
			}
		}
	}()

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		tc.logger.Execution().Error("problem during configuring initial state")
		complete <- evergreen.TaskSystemFailed
		return
	}

	if ctx.Err() != nil {
		grip.Info("task canceled")
		return
	}
	tc.setCurrentCommand(factory())
	a.comm.UpdateLastMessageTime()

	if ctx.Err() != nil {
		grip.Info("task canceled")
		return
	}
	tc.logger.Task().Infof("Task logger initialized (agent version %s from %s).", evergreen.AgentVersion, evergreen.BuildRevision)
	tc.logger.Execution().Info("Execution logger initialized.")
	tc.logger.System().Info("System logger initialized.")

	if ctx.Err() != nil {
		grip.Info("task canceled")
		return
	}
	tc.logger.Task().Infof("Starting task %v, execution %v.", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)

	var innerCtx context.Context
	innerCtx, cancel = context.WithCancel(ctx)
	defer cancel()
	go a.startMaxExecTimeoutWatch(ctx, tc, cancel)

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

	conn, err := a.comm.GetCedarGRPCConn(ctx)
	if err != nil {
		tc.logger.System().Error(errors.Wrap(err, "error getting cedar client connection"))
	} else {
		tc.systemMetricsCollector, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
			task:       tc.taskModel,
			interval:   defaultStatsInterval,
			collectors: []metricCollector{},
			conn:       conn,
		})
		if err != nil {
			tc.logger.System().Error(errors.Wrap(err, "error initializing system metrics collector"))
		} else {
			err = tc.systemMetricsCollector.Start(ctx)
			if err != nil {
				tc.logger.System().Error(errors.Wrap(err, "error starting system metrics collection"))
			}
		}
	}

	if ctx.Err() != nil {
		tc.logger.Task().Info("task canceled")
		return
	}

	if tc.runGroupSetup {
		tc.taskDirectory, err = a.createTaskDirectory(tc)
		if err != nil {
			tc.logger.Execution().Errorf("error creating task directory: %s", err)
			complete <- evergreen.TaskFailed
			return
		}
	}
	tc.taskConfig.WorkDir = tc.taskDirectory
	tc.taskConfig.Expansions.Put("workdir", tc.taskConfig.WorkDir)

	// notify API server that the task has been started.
	tc.logger.Execution().Info("Reporting task started.")
	if err = a.comm.StartTask(ctx, tc.task); err != nil {
		tc.logger.Execution().Errorf("error marking task started: %v", err)
		complete <- evergreen.TaskFailed
		return
	}

	a.killProcs(ctx, tc, false)

	if err = a.runPreTaskCommands(innerCtx, tc); err != nil {
		complete <- evergreen.TaskFailed
		return
	}

	if tc.project.OomTracker {
		tc.logger.Execution().Info("OOM tracker clearing system messages")
		if err = tc.oomTracker.Clear(innerCtx); err != nil {
			tc.logger.Execution().Errorf("error clearing system messages: %s", err)
			complete <- evergreen.TaskFailed
			return
		}
	}

	if err = a.runTaskCommands(innerCtx, tc); err != nil {
		complete <- evergreen.TaskFailed
		return
	}
	complete <- evergreen.TaskSucceeded
}

func (a *Agent) runPreTaskCommands(ctx context.Context, tc *taskContext) error {
	tc.logger.Task().Info("Running pre-task commands.")
	opts := runCommandsOptions{}

	if tc.runGroupSetup {
		var ctx2 context.Context
		var cancel context.CancelFunc
		taskGroup, err := model.GetTaskGroup(tc.taskGroup, tc.taskConfig)
		if err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for pre-group commands"))
			return nil
		}
		if taskGroup.SetupGroup != nil {
			opts.shouldSetupFail = taskGroup.SetupGroupFailTask
			if taskGroup.SetupGroupTimeoutSecs > 0 {
				ctx2, cancel = context.WithTimeout(ctx, time.Duration(taskGroup.SetupGroupTimeoutSecs)*time.Second)
			} else {
				ctx2, cancel = a.withCallbackTimeout(ctx, tc)
			}
			defer cancel()
			err = a.runCommands(ctx2, tc, taskGroup.SetupGroup.List(), opts)
			if err != nil {
				tc.logger.Execution().Error(errors.Wrap(err, "error running task setup group"))
				if taskGroup.SetupGroupFailTask {
					return err
				}
			}
		}
	}

	taskGroup, err := model.GetTaskGroup(tc.taskGroup, tc.taskConfig)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for pre-task commands"))
		return nil
	}

	if taskGroup.SetupTask != nil {
		opts.shouldSetupFail = taskGroup.SetupGroupFailTask
		err = a.runCommands(ctx, tc, taskGroup.SetupTask.List(), opts)
	}
	if err != nil {
		msg := fmt.Sprintf("Running pre-task commands failed: %v", err)
		tc.logger.Task().Error(msg)
		if opts.shouldSetupFail {
			return errors.New(msg)
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
		tc.logger.Execution().Infof("Current command set to '%s' (%s)", tc.currentCommand.DisplayName(), tc.currentCommand.Type())
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
	tc.logger.Execution().Debugf("Set idle timeout for '%s' (%s) to %s",
		tc.currentCommand.DisplayName(), tc.currentCommand.Type(), tc.getIdleTimeout())
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

func (tc *taskContext) getOomTrackerInfo() apimodels.OOMTrackerInfo {
	detected, pids := tc.oomTracker.Report()
	return apimodels.OOMTrackerInfo{
		Detected: detected,
		Pids:     pids,
	}
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
func (a *Agent) makeTaskConfig(ctx context.Context, tc *taskContext) (*model.TaskConfig, error) {
	if tc.project == nil {
		grip.Info("Fetching project config.")
		err := a.fetchProjectConfig(ctx, tc)
		if err != nil {
			return nil, err
		}
	}
	grip.Info("Fetching distro configuration.")
	confDistro, err := a.comm.GetDistro(ctx, tc.task)
	if err != nil {
		return nil, err
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
		grip.Info("Fetching patch document for Github PR request.")
		confPatch, err = a.comm.GetTaskPatch(ctx, tc.task)
		if err != nil {
			err = errors.Wrap(err, "couldn't fetch patch for Github PR request")
			grip.Error(err.Error())
			return nil, err
		}
	}

	grip.Info("Constructing TaskConfig.")
	return model.NewTaskConfig(confDistro, tc.project, tc.taskModel, confRef, confPatch, tc.expansions)
}

func (tc *taskContext) getExecTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()
	if tc.taskConfig == nil {
		return defaultExecTimeout
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
	return defaultExecTimeout
}

func (tc *taskContext) setTaskConfig(taskConfig *model.TaskConfig) {
	tc.Lock()
	defer tc.Unlock()
	tc.taskConfig = taskConfig
}

func (tc *taskContext) getTaskConfig() *model.TaskConfig {
	tc.RLock()
	defer tc.RUnlock()
	return tc.taskConfig
}
