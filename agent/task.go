package agent

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func (a *Agent) startTask(ctx context.Context, tc *taskContext, complete chan<- string, execTimeout chan<- struct{}, idleTimeout chan<- time.Duration) {
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		tc.logger.Execution().Error("problem during configuring initial state")
		complete <- evergreen.TaskSystemFailed
		return
	}

	a.checkIn(ctx, tc, factory(), initialSetupTimeout, idleTimeout)

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

	go a.startMaxExecTimeoutWatch(ctx, tc, a.getExecTimeoutSecs(taskConfig), execTimeout)

	tc.logger.Execution().Infof("Fetching expansions for project %s", taskConfig.Task.Project)
	expVars, err := a.comm.FetchExpansionVars(ctx, tc.task)
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
	tc.statsCollector.logStats(ctx, tc.taskConfig.Expansions)

	if ctx.Err() != nil {
		tc.logger.Task().Info("task canceled")
		return
	}
	newDir, err := a.createTaskDirectory(tc, taskConfig)
	tc.taskDirectory = newDir
	if err != nil {
		tc.logger.Execution().Errorf("error creating task directory: %s", err)
		complete <- evergreen.TaskFailed
		return
	}
	taskConfig.Expansions.Put("workdir", newDir)

	// notify API server that the task has been started.
	tc.logger.Execution().Info("Reporting task started.")
	if err = a.comm.StartTask(ctx, tc.task); err != nil {
		tc.logger.Execution().Errorf("error marking task started: %v", err)
		complete <- evergreen.TaskFailed
		return
	}

	a.killProcs(tc)
	a.runPreTaskCommands(ctx, tc)

	taskStatus := a.runTaskCommands(ctx, tc, idleTimeout)
	if taskStatus != nil {
		complete <- evergreen.TaskFailed
	}
	complete <- evergreen.TaskSucceeded
	return
}

func (a *Agent) runPreTaskCommands(ctx context.Context, tc *taskContext) {
	if tc.taskConfig.Project.Pre != nil {
		tc.logger.Execution().Info("Running pre-task commands.")
		var cancel context.CancelFunc
		ctx, cancel = a.withCallbackTimeout(ctx, tc)
		defer cancel()
		err := a.runCommands(ctx, tc, tc.taskConfig.Project.Pre.List(), false, nil)
		if err != nil {
			tc.logger.Execution().Errorf("Running pre-task script failed: %v", err)
		}
		tc.logger.Execution().Info("Finished running pre-task commands.")
	}
}

// CheckIn updates the agent's execution stage and current timeout duration,
// and resets its timer back to zero.
func (a *Agent) checkIn(ctx context.Context, tc *taskContext, command command.Command, duration time.Duration, idleTimeout chan<- time.Duration) {
	if ctx.Err() != nil {
		return
	}

	if idleTimeout == nil {
		return
	}

	select {
	case idleTimeout <- duration:
		tc.currentCommand = command
		tc.logger.Execution().Infof("Command timeout set to %v", duration.String())
	default:
	}
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

	tc.logger.Execution().Info("Constructing TaskConfig.")
	return model.NewTaskConfig(confDistro, confVersion, confProject, confTask, confRef)
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
