package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
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

	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		tc.logger.Execution().Error("problem during configuring initial state")
		complete <- evergreen.TaskSystemFailed
		return
	}

	if taskCtx.Err() != nil {
		grip.Info("task canceled")
		return
	}
	tc.setCurrentCommand(factory())
	a.comm.UpdateLastMessageTime()

	if taskCtx.Err() != nil {
		grip.Info("task canceled")
		return
	}
	tc.logger.Task().Infof("Task logger initialized (agent version %s from %s).", evergreen.AgentVersion, evergreen.BuildRevision)
	tc.logger.Execution().Info("Execution logger initialized.")
	tc.logger.System().Info("System logger initialized.")

	if taskCtx.Err() != nil {
		grip.Info("task canceled")
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		tc.logger.Execution().Infof("Unable to get hostname: %s", err)
	} else {
		tc.logger.Execution().Infof("Hostname is %s", hostname)
	}
	tc.logger.Task().Infof("Starting task %v, execution %v.", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)

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

	if err := a.setupSystemMetricsCollector(ctx, tc); err != nil {
		tc.logger.System().Error(errors.Wrap(err, "setting up system metrics collector"))
	}

	if ctx.Err() != nil {
		tc.logger.Task().Info("task canceled")
		return
	}

	if !tc.ranSetupGroup {
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

	if tc.oomTrackerEnabled(a.opts.CloudProvider) {
		tc.logger.Execution().Info("OOM tracker clearing system messages")
		if err = tc.oomTracker.Clear(innerCtx); err != nil {
			tc.logger.Execution().Errorf("error clearing system messages: %s", err)
		}
	}

	if err = a.runTaskCommands(innerCtx, tc); err != nil {
		complete <- evergreen.TaskFailed
		return
	}
	complete <- evergreen.TaskSucceeded
}

func (a *Agent) setupSystemMetricsCollector(ctx context.Context, tc *taskContext) error {
	conn, err := a.comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrap(err, "getting cedar gRPC client connection")
	}

	tc.Lock()
	defer tc.Unlock()

	tc.systemMetricsCollector, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:     tc.taskModel,
		interval: defaultStatsInterval,
		collectors: []metricCollector{
			newUptimeCollector(),
			newProcessCollector(),
			newDiskUsageCollector(tc.taskConfig.WorkDir),
		},
		conn: conn,
	})
	if err != nil {
		return errors.Wrap(err, "initializing system metrics collector")
	}

	if err = tc.systemMetricsCollector.Start(ctx); err != nil {
		return errors.Wrap(err, "starting system metrics collection")
	}
	return nil
}

func (a *Agent) runPreTaskCommands(ctx context.Context, tc *taskContext) error {
	tc.logger.Task().Info("Running pre-task commands.")
	opts := runCommandsOptions{}

	if !tc.ranSetupGroup {
		var ctx2 context.Context
		var cancel context.CancelFunc
		taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
		if err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for pre-group commands"))
			return nil
		}
		if taskGroup.SetupGroup != nil {
			tc.logger.Task().Infof("Running setup_group for '%s'.", taskGroup.Name)
			opts.failPreAndPost = taskGroup.SetupGroupFailTask
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
			tc.logger.Task().Infof("Finished running setup_group for '%s'.", taskGroup.Name)
		}
		tc.ranSetupGroup = true
	}

	taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for pre-task commands"))
		return nil
	}

	if taskGroup.SetupTask != nil {
		tc.logger.Task().Infof("Running setup_task for '%s'.", taskGroup.Name)
		opts.failPreAndPost = taskGroup.SetupGroupFailTask
		err = a.runCommands(ctx, tc, taskGroup.SetupTask.List(), opts)
	}
	if err != nil {
		msg := fmt.Sprintf("Running pre-task commands failed: %v", err)
		tc.logger.Task().Error(msg)
		if opts.failPreAndPost {
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
	if tc.currentCommand != nil {
		tc.logger.Execution().Debugf("Set idle timeout for '%s' (%s) to %s",
			tc.currentCommand.DisplayName(), tc.currentCommand.Type(), tc.getIdleTimeout())
	} else {
		tc.logger.Execution().Debugf("Set current idle timeout to %s", tc.getIdleTimeout())
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
	confDistro, err := a.comm.GetDistroView(ctx, tc.task)
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
		confPatch, err = a.comm.GetTaskPatch(ctx, tc.task, "")
		if err != nil {
			err = errors.Wrap(err, "couldn't fetch patch for Github PR request")
			grip.Error(err.Error())
			return nil, err
		}
	}

	grip.Info("Constructing TaskConfig.")
	taskConfig, err := internal.NewTaskConfig(confDistro, tc.project, tc.taskModel, confRef, confPatch, tc.expansions)
	if err != nil {
		return nil, err
	}
	taskConfig.RestrictedExpansions = util.NewExpansions(tc.expVars.RestrictedVars)
	taskConfig.Redacted = tc.expVars.PrivateVars
	return taskConfig, nil
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
