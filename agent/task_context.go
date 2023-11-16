package agent

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/disk"
)

type taskContext struct {
	currentCommand command.Command
	logger         client.LoggerProducer
	task           client.TaskData
	ranSetupGroup  bool
	taskConfig     *internal.TaskConfig
	timeout        timeoutInfo
	oomTracker     jasper.OOMTracker
	traceID        string
	diskDevices    []string
	// userEndTaskResp is the end task response that the user can define, which
	// will overwrite the default end task response.
	userEndTaskResp *triggerEndTaskResp
	sync.RWMutex
}

func (tc *taskContext) setCurrentCommand(command command.Command) {
	tc.Lock()
	defer tc.Unlock()
	tc.currentCommand = command
	if tc.logger != nil {
		tc.logger.Execution().Infof("Current command set to %s (%s).", tc.currentCommand.FullDisplayName(), tc.currentCommand.Type())
	}
}

func (tc *taskContext) getCurrentCommand() command.Command {
	tc.RLock()
	defer tc.RUnlock()
	return tc.currentCommand
}

// setCurrentIdleTimeout sets the idle timeout for the current running command.
// This timeout only applies to commands running in specific blocks where idle
// timeout is allowed.
func (tc *taskContext) setCurrentIdleTimeout(cmd command.Command, block command.BlockType) {
	tc.Lock()
	defer tc.Unlock()

	var timeout time.Duration
	if dynamicTimeout := tc.taskConfig.GetIdleTimeout(); dynamicTimeout != 0 {
		timeout = time.Duration(dynamicTimeout) * time.Second
	} else if cmd.IdleTimeout() > 0 {
		timeout = cmd.IdleTimeout()
	} else {
		timeout = defaultIdleTimeout
	}

	tc.setIdleTimeout(timeout)

	tc.logger.Execution().Debugf("Set idle timeout for %s (%s) to %s.",
		cmd.FullDisplayName(), cmd.Type(), tc.getIdleTimeout())
}

// getCurrentIdleTimeout returns the idle timeout for the current running
// command.
func (tc *taskContext) getCurrentIdleTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	timeout := tc.getIdleTimeout()
	if timeout > 0 {
		return timeout
	}
	return defaultIdleTimeout
}

func (tc *taskContext) setHeartbeatTimeout(opts heartbeatTimeoutOptions) {
	if opts.getTimeout == nil {
		opts.getTimeout = func() time.Duration { return defaultHeartbeatTimeout }
	}
	if utility.IsZeroTime(opts.startAt) {
		opts.startAt = time.Now()
	}

	tc.Lock()
	defer tc.Unlock()

	tc.timeout.heartbeatTimeoutOpts = opts
}

func (tc *taskContext) getHeartbeatTimeout() heartbeatTimeoutOptions {
	tc.RLock()
	defer tc.RUnlock()

	return tc.timeout.heartbeatTimeoutOpts
}

// hadHeartbeatTimeout returns whether the task has hit the heartbeat timeout.
// If this returns true, the heartbeat should time out to indicate the task is
// stuck in a way that's not recoverable. Hitting the heartbeat timeout is
// generally a strong sign of a bug and should never occur during normal
// operation.
func (tc *taskContext) hadHeartbeatTimeout() bool {
	timeoutOpts := tc.getHeartbeatTimeout()

	// Give the agent some extra time just in case it's being a bit slow but is
	// making progress.
	timeoutWithGracePeriod := timeoutOpts.getTimeout() + evergreen.HeartbeatTimeoutThreshold
	return time.Since(timeoutOpts.startAt) > timeoutWithGracePeriod
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
	return tc.taskConfig.Project.OomTracker && !utility.StringSliceContains(evergreen.ProviderContainer, cloudProvider)
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

func (tc *taskContext) getExecTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()
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

// makeTaskConfig fetches task configuration data required to run the task from the API server.
func (a *Agent) makeTaskConfig(ctx context.Context, tc *taskContext) (*internal.TaskConfig, error) {
	if tc.taskConfig != nil {
		// This is only relevant in tests. For convenience, tests can
		// pre-initialize a task config to use instead of fetching the task
		// setup data using the communicator.
		return tc.taskConfig, nil
	}

	grip.Info("Fetching task info.")
	tsk, project, expansions, redacted, err := a.fetchTaskInfo(ctx, tc)
	if err != nil {
		return nil, errors.Wrap(err, "fetching task info")
	}

	grip.Info("Fetching distro configuration.")
	var confDistro *apimodels.DistroView
	if a.opts.Mode == HostMode {
		var err error
		confDistro, err = a.comm.GetDistroView(ctx, tc.task)
		if err != nil {
			return nil, errors.Wrap(err, "fetching distro view")
		}
	}

	grip.Info("Fetching project ref.")
	confRef, err := a.comm.GetProjectRef(ctx, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "getting project ref")
	}
	if confRef == nil {
		return nil, errors.New("agent retrieved an empty project ref")
	}

	var confPatch *patch.Patch
	if evergreen.IsGitHubPatchRequester(tsk.Requester) {
		grip.Info("Fetching patch document for GitHub PR request.")
		confPatch, err = a.comm.GetTaskPatch(ctx, tc.task, "")
		if err != nil {
			return nil, errors.Wrap(err, "fetching patch for GitHub PR request")
		}
	}

	grip.Info("Constructing task config.")
	taskConfig, err := internal.NewTaskConfig(a.opts.WorkingDirectory, confDistro, project, tsk, confRef, confPatch, expansions)
	if err != nil {
		return nil, err
	}
	taskConfig.Redacted = redacted
	taskConfig.TaskSync = a.opts.SetupData.TaskSync
	taskConfig.EC2Keys = a.opts.SetupData.EC2Keys

	return taskConfig, nil
}

// commandBlock contains information for a block of commands.
type commandBlock struct {
	block               command.BlockType
	commands            *model.YAMLCommandSet
	timeoutKind         timeoutType
	getTimeout          func() time.Duration
	canTimeOutHeartbeat bool
	canFailTask         bool
}

// getPre returns a command block containing the pre task commands.
func (tc *taskContext) getPre() (*commandBlock, error) {
	if err := tc.taskConfig.Validate(); err != nil {
		return nil, err
	}

	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{
			block:       command.PreBlock,
			commands:    tc.taskConfig.Project.Pre,
			timeoutKind: preTimeout,
			getTimeout:  tc.getPreTimeout,
			canFailTask: tc.taskConfig.Project.PreErrorFailsTask,
		}, nil
	}

	return &commandBlock{
		block:       command.SetupTaskBlock,
		commands:    tg.SetupTask,
		timeoutKind: setupTaskTimeout,
		getTimeout:  tc.getSetupTaskTimeout(),
		canFailTask: tg.SetupTaskCanFailTask,
	}, nil
}

// getPost returns a command block containing the post task commands.
func (tc *taskContext) getPost() (*commandBlock, error) {
	if err := tc.taskConfig.Validate(); err != nil {
		return nil, err
	}

	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{
			block:               command.PostBlock,
			commands:            tc.taskConfig.Project.Post,
			timeoutKind:         postTimeout,
			getTimeout:          tc.getPostTimeout,
			canTimeOutHeartbeat: true,
			canFailTask:         tc.taskConfig.Project.PostErrorFailsTask,
		}, nil
	}

	return &commandBlock{
		block:               command.TeardownTaskBlock,
		commands:            tg.TeardownTask,
		timeoutKind:         teardownTaskTimeout,
		getTimeout:          tc.getTeardownTaskTimeout(),
		canTimeOutHeartbeat: true,
		canFailTask:         tg.TeardownTaskCanFailTask,
	}, nil
}

// getSetupGroup returns the setup group for a task group task.
func (tc *taskContext) getSetupGroup() (*commandBlock, error) {
	if err := tc.taskConfig.Validate(); err != nil {
		return nil, err
	}

	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{}, nil
	}
	if tg.SetupGroup == nil {
		return &commandBlock{}, nil
	}

	return &commandBlock{
		block:       command.SetupGroupBlock,
		commands:    tg.SetupGroup,
		timeoutKind: setupGroupTimeout,
		getTimeout:  tc.getSetupGroupTimeout(),
		canFailTask: tg.SetupGroupCanFailTask,
	}, nil
}

// getTeardownGroup returns the teardown group for a task group task.
func (tc *taskContext) getTeardownGroup() (*commandBlock, error) {
	if err := tc.taskConfig.Validate(); err != nil {
		return nil, err
	}

	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{}, nil
	}
	if tg.TeardownGroup == nil {
		return &commandBlock{}, nil
	}

	return &commandBlock{
		block:       command.TeardownGroupBlock,
		commands:    tg.TeardownGroup,
		timeoutKind: teardownGroupTimeout,
		getTimeout:  tc.getTeardownGroupTimeout(),
		canFailTask: false,
	}, nil
}

// getTimeout returns a command block containing the timeout handler commands.
func (tc *taskContext) getTimeout() (*commandBlock, error) {
	if err := tc.taskConfig.Validate(); err != nil {
		return nil, err
	}

	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{
			block:               command.TaskTimeoutBlock,
			commands:            tc.taskConfig.Project.Timeout,
			timeoutKind:         callbackTimeout,
			getTimeout:          tc.getCallbackTimeout,
			canFailTask:         false,
			canTimeOutHeartbeat: true,
		}, nil
	}
	if tg.Timeout == nil {
		// Task group timeout defaults to the project timeout settings if not
		// explicitly set.
		return &commandBlock{
			block:               command.TaskTimeoutBlock,
			commands:            tc.taskConfig.Project.Timeout,
			timeoutKind:         callbackTimeout,
			getTimeout:          tc.getCallbackTimeout,
			canFailTask:         false,
			canTimeOutHeartbeat: true,
		}, nil
	}

	return &commandBlock{
		block:       command.TaskTimeoutBlock,
		commands:    tg.Timeout,
		timeoutKind: callbackTimeout,
		getTimeout:  tc.getTaskGroupCallbackTimeout(),
		canFailTask: false,
	}, nil
}

// getPreTimeout returns the timeout for the pre block.
func (tc *taskContext) getPreTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig.Project.PreTimeoutSecs != 0 {
		return time.Duration(tc.taskConfig.Project.PreTimeoutSecs) * time.Second
	}

	return defaultPreTimeout
}

// getPostTimeout returns the timeout for the post block.
func (tc *taskContext) getPostTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig.Project.PostTimeoutSecs != 0 {
		return time.Duration(tc.taskConfig.Project.PostTimeoutSecs) * time.Second
	}
	return defaultPostTimeout
}

// getSetupGroupTimeout returns the timeout for the setup group block.
func (tc *taskContext) getSetupGroupTimeout() func() time.Duration {
	return func() time.Duration {
		tc.RLock()
		defer tc.RUnlock()

		tg := tc.taskConfig.TaskGroup
		if tg != nil && tg.SetupGroupTimeoutSecs != 0 {
			return time.Duration(tg.SetupGroupTimeoutSecs) * time.Second
		}
		return defaultPreTimeout
	}
}

// getSetupTaskTimeout returns the timeout for the setup task block.
func (tc *taskContext) getSetupTaskTimeout() func() time.Duration {
	return func() time.Duration {
		tc.RLock()
		defer tc.RUnlock()

		tg := tc.taskConfig.TaskGroup
		if tg != nil && tg.SetupTaskTimeoutSecs != 0 {
			return time.Duration(tg.SetupTaskTimeoutSecs) * time.Second
		}
		return defaultPreTimeout
	}
}

// getTeardownTaskTimeout returns the timeout for the teardown task block.
func (tc *taskContext) getTeardownTaskTimeout() func() time.Duration {
	return func() time.Duration {
		tc.RLock()
		defer tc.RUnlock()

		tg := tc.taskConfig.TaskGroup
		if tg != nil && tg.TeardownTaskTimeoutSecs != 0 {
			return time.Duration(tg.TeardownTaskTimeoutSecs) * time.Second
		}
		return defaultPostTimeout
	}
}

// getTeardownGroupTimeout returns the timeout for the teardown group block.
func (tc *taskContext) getTeardownGroupTimeout() func() time.Duration {
	return func() time.Duration {
		tc.RLock()
		defer tc.RUnlock()

		tg := tc.taskConfig.TaskGroup
		if tg != nil && tg.TeardownGroupTimeoutSecs != 0 {
			return time.Duration(tg.TeardownGroupTimeoutSecs) * time.Second
		}
		return defaultTeardownGroupTimeout
	}
}

// getCallbackTimeout returns the callback timeout for the timeout handler.
func (tc *taskContext) getCallbackTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig.Project.CallbackTimeout != 0 {
		return time.Duration(tc.taskConfig.Project.CallbackTimeout) * time.Second
	}
	return defaultCallbackTimeout
}

// getTaskGroupCallbackTimeout returns the callback timeout for a task group
// task.
func (tc *taskContext) getTaskGroupCallbackTimeout() func() time.Duration {
	return func() time.Duration {
		tc.RLock()
		defer tc.RUnlock()

		tg := tc.taskConfig.TaskGroup
		if tg != nil && tg.CallbackTimeoutSecs != 0 {
			return time.Duration(tg.CallbackTimeoutSecs) * time.Second
		}
		return defaultCallbackTimeout
	}
}

// setUserEndTaskResponse sets the user-defined end task response data.
func (tc *taskContext) setUserEndTaskResponse(resp *triggerEndTaskResp) {
	tc.Lock()
	defer tc.Unlock()

	tc.userEndTaskResp = resp
}

// getUserEndTaskResponse gets the user-defined end task response data.
func (tc *taskContext) getUserEndTaskResponse() *triggerEndTaskResp {
	tc.RLock()
	defer tc.RUnlock()

	return tc.userEndTaskResp
}

func (tc *taskContext) getDeviceNames(ctx context.Context) error {
	if tc.taskConfig == nil || tc.taskConfig.Distro == nil || len(tc.taskConfig.Distro.Mountpoints) == 0 {
		return nil
	}

	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return errors.Wrap(err, "getting partitions")
	}
	for _, partition := range partitions {
		if utility.StringSliceContains(tc.taskConfig.Distro.Mountpoints, partition.Mountpoint) {
			tc.diskDevices = append(tc.diskDevices, filepath.Base(partition.Device))
		}
	}
	return nil
}
