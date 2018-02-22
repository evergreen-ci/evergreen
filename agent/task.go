package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
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
			case complete <- evergreen.TaskSystemFailed:
				grip.Debug("marked task as system-failed after panic")
			default:
				grip.Debug("marking task system failed during panic handling, but complete channel was blocked")
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
	tc.logger.Task().Infof("Task logger initialized (agent revision: %s).", evergreen.BuildRevision)
	tc.logger.Execution().Info("Execution logger initialized.")
	tc.logger.System().Info("System logger initialized.")

	taskConfig, err := a.getTaskConfig(ctx, tc)
	if err != nil {
		tc.logger.Execution().Errorf("Error fetching task configuration: %s", err)
		complete <- evergreen.TaskFailed
		return
	}

	if ctx.Err() != nil {
		grip.Info("task canceled")
		return
	}
	tc.logger.Task().Infof("Starting task %v, execution %v.", taskConfig.Task.Id, taskConfig.Task.Execution)

	var innerCtx context.Context
	innerCtx, cancel = context.WithCancel(ctx)
	defer cancel()

	go a.startMaxExecTimeoutWatch(ctx, tc, a.getExecTimeoutSecs(taskConfig), cancel)

	tc.logger.Execution().Infof("Fetching expansions for project %s", taskConfig.Task.Project)
	expVars, err := a.comm.FetchExpansionVars(innerCtx, tc.task)
	if err != nil {
		tc.logger.Execution().Errorf("error fetching project expansion variables: %s", err)
		complete <- evergreen.TaskFailed
		return
	}
	taskConfig.Expansions.Update(*expVars)
	tc.taskConfig = taskConfig

	// set up the system stats collector
	tc.statsCollector = NewSimpleStatsCollector(
		tc.logger,
		defaultStatsInterval,
		"uptime",
		"df -h",
		"${ps|ps}",
	)
	tc.statsCollector.logStats(innerCtx, tc.taskConfig.Expansions)

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
	taskConfig.Expansions.Put("workdir", tc.taskConfig.WorkDir)

	// notify API server that the task has been started.
	tc.logger.Execution().Info("Reporting task started.")
	if err = a.comm.StartTask(ctx, tc.task); err != nil {
		tc.logger.Execution().Errorf("error marking task started: %v", err)
		complete <- evergreen.TaskFailed
		return
	}

	a.killProcs(tc)
	a.runPreTaskCommands(innerCtx, tc)

	if err = a.runTaskCommands(innerCtx, tc); err != nil {
		complete <- evergreen.TaskFailed
		return
	}
	complete <- evergreen.TaskSucceeded
}

func (a *Agent) runPreTaskCommands(ctx context.Context, tc *taskContext) {
	var err error
	tc.logger.Task().Info("Running pre-task commands.")
	if tc.runGroupSetup {
		var cancel context.CancelFunc
		ctx, cancel = a.withCallbackTimeout(ctx, tc)
		defer cancel()
		taskGroup, err := model.GetTaskGroup(tc.taskGroup, tc.taskConfig)
		if err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for pre-group commands"))
			return
		}
		if taskGroup.SetupGroup != nil {
			err = a.runCommands(ctx, tc, taskGroup.SetupGroup.List(), false)
			if err != nil {
				tc.logger.Execution().Error(errors.Wrap(err, "error running task setup group"))
			}
		}
	}

	taskGroup, err := model.GetTaskGroup(tc.taskGroup, tc.taskConfig)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for pre-task commands"))
		return
	}
	if taskGroup.SetupTask != nil {
		err = a.runCommands(ctx, tc, taskGroup.SetupTask.List(), false)
	}
	tc.logger.Task().ErrorWhenf(err != nil, "Running pre-task commands failed: %v", err)
	tc.logger.Task().InfoWhen(err == nil, "Finished running pre-task commands.")
}

func (tc *taskContext) setCurrentCommand(command command.Command) {
	tc.Lock()
	defer tc.Unlock()
	tc.currentCommand = command
	tc.logger.Execution().Infof("Current command set to '%s' (%s)", tc.currentCommand.DisplayName(), tc.currentCommand.Type())
}

func (tc *taskContext) getCurrentCommand() command.Command {
	tc.RLock()
	defer tc.RUnlock()
	return tc.currentCommand
}

func (tc *taskContext) setCurrentTimeout(dur time.Duration) {
	tc.Lock()
	defer tc.Unlock()

	tc.timeout = dur
	tc.logger.Execution().Debugf("Set command timeout for '%s' (%s) to %s",
		tc.currentCommand.DisplayName(), tc.currentCommand.Type(), dur)
}

func (tc *taskContext) getCurrentTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.timeout > 0 {
		return tc.timeout
	}
	return defaultIdleTimeout
}

func (tc *taskContext) reachTimeOut() {
	tc.Lock()
	defer tc.Unlock()

	tc.timedOut = true
}

func (tc *taskContext) hadTimedOut() bool {
	tc.RLock()
	defer tc.RUnlock()

	return tc.timedOut
}

// getTaskConfig fetches task configuration data required to run the task from the API server.
func (a *Agent) getTaskConfig(ctx context.Context, tc *taskContext) (*model.TaskConfig, error) {
	tc.logger.Execution().Info("Fetching distro configuration.")
	confDistro, err := a.comm.GetDistro(ctx, tc.task)
	if err != nil {
		return nil, err
	}

	tc.logger.Execution().Info("Fetching version.")
	confVersion, err := a.comm.GetVersion(ctx, tc.task)
	if err != nil {
		return nil, err
	}

	confProject := &model.Project{}
	err = model.LoadProjectInto([]byte(confVersion.Config), confVersion.Identifier, confProject)
	if err != nil {
		return nil, errors.Wrapf(err, "reading project config")
	}

	tc.logger.Execution().Info("Fetching task configuration.")
	confTask, err := a.comm.GetTask(ctx, tc.task)
	if err != nil {
		return nil, err
	}

	tc.logger.Execution().Info("Fetching project ref.")
	confRef, err := a.comm.GetProjectRef(ctx, tc.task)
	if err != nil {
		return nil, err
	}
	if confRef == nil {
		return nil, errors.New("agent retrieved an empty project ref")
	}

	var confPatch *patch.Patch
	if confVersion.Requester == evergreen.GithubPRRequester {
		tc.logger.Execution().Info("Fetching patch document for Github PR request.")
		confPatch, err = a.comm.GetTaskPatch(ctx, tc.task)
		if err != nil {
			err = errors.Wrap(err, "couldn't fetch patch for Github PR request")
			tc.logger.Execution().Error(err.Error())
			return nil, err
		}
	}

	tc.logger.Execution().Info("Constructing TaskConfig.")
	return model.NewTaskConfig(confDistro, confVersion, confProject, confTask, confRef, confPatch)
}

func (a *Agent) getExecTimeoutSecs(taskConfig *model.TaskConfig) time.Duration {
	pt := taskConfig.Project.FindProjectTask(taskConfig.Task.DisplayName)
	if pt.ExecTimeoutSecs == 0 {
		// if unspecified in the project task and the project, use the default value
		if taskConfig.Project.ExecTimeoutSecs != 0 {
			pt.ExecTimeoutSecs = taskConfig.Project.ExecTimeoutSecs
		} else {
			pt.ExecTimeoutSecs = defaultExecTimeoutSecs
		}
	}
	return time.Duration(pt.ExecTimeoutSecs) * time.Second
}
