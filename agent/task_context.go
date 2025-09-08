package agent

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/disk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Platform-specific mountpoints used for disk device detection.
	// Sync with devprod infrastructure before changing these values.
	linuxMountpoint   = "/data"
	windowsMountpoint = "Z:"
	darwinMountpoint  = "/System/Volumes/Data"
)

type taskContext struct {
	currentCommand command.Command
	// failingCommand keeps track of the command that caused the task to fail,
	// if any.
	failingCommand command.Command
	// otherFailingCommands keeps track of commands that have run and failed but
	// have not caused the task to fail (e.g. a post command that fails without
	// post_error_fails_task). Does not include commands that suppress errors,
	// such as s3.put with optional: true.
	otherFailingCommands []command.Command
	postErrored          bool
	logger               client.LoggerProducer
	task                 client.TaskData
	// ranSetupGroup is true during task setup if the task is a new standalone
	// task or if it's the first task in a task group.
	ranSetupGroup bool
	taskConfig    *internal.TaskConfig
	timeout       timeoutInfo
	oomTracker    jasper.OOMTracker
	traceID       string
	diskDevices   []string
	// taskCleanups and taskGroupCleanups store the cleanup commands for the
	// task and setup group, respectively.
	taskCleanups       []internal.CommandCleanup
	setupGroupCleanups []internal.CommandCleanup
	// userEndTaskResp is the end task response that the user can define, which
	// will overwrite the default end task response.
	userEndTaskResp                   *triggerEndTaskResp
	userEndTaskRespOriginatingCommand command.Command
	// addMetadataTagResp is a user-definable response that contains a failure
	// metadata tag payload, which can be appended to the final list of failure
	// metadata tags in the end task response.
	addMetadataTagResp *triggerAddMetadataTagResp
	sync.RWMutex
}

func (tc *taskContext) getPostErrored() bool {
	tc.RLock()
	defer tc.RUnlock()
	return tc.postErrored
}

func (tc *taskContext) setPostErrored(errored bool) {
	tc.Lock()
	defer tc.Unlock()
	tc.postErrored = errored
}

func (tc *taskContext) addTaskCommandCleanups(cleanups []internal.CommandCleanup) {
	tc.Lock()
	defer tc.Unlock()

	tc.taskCleanups = append(tc.taskCleanups, cleanups...)
}

func (tc *taskContext) addSetupGroupCommandCleanups(cleanups []internal.CommandCleanup) {
	tc.Lock()
	defer tc.Unlock()

	tc.setupGroupCleanups = append(tc.setupGroupCleanups, cleanups...)
}

// runTaskCommandCleanups runs the cleanup commands added throughout the normal execution of
// 'pre+main+post' or 'setup_task+main+teardown_task' and 'teardown_group'. For 'setup_group',
// use runSetupGroupCommandCleanups instead.
func (tc *taskContext) runTaskCommandCleanups(ctx context.Context, logger client.LoggerProducer, trace trace.Tracer) {
	// Noop on empty cleanup list.
	if len(tc.taskCleanups) == 0 {
		return
	}
	ctx, span := trace.Start(ctx, "task-command-cleanups")
	defer span.End()

	if err := errors.Wrap(runCommandCleanups(ctx, tc.taskCleanups, trace), "running setup group command cleanups"); err != nil {
		logger.Execution().Error(err)
	}
}

// runSetupGroupCommandCleanups runs the cleanup commands added throughout the execution of a setup group.
func (tc *taskContext) runSetupGroupCommandCleanups(ctx context.Context, logger client.LoggerProducer, trace trace.Tracer) {
	// Noop on empty cleanup list.
	if len(tc.setupGroupCleanups) == 0 {
		return
	}
	ctx, span := trace.Start(ctx, "setup-group-command-cleanups")
	defer span.End()

	if err := errors.Wrap(runCommandCleanups(ctx, tc.setupGroupCleanups, trace), "running setup group command cleanups"); err != nil {
		logger.Execution().Error(err)
	}
}

func runCommandCleanups(ctx context.Context, cleanups []internal.CommandCleanup, trace trace.Tracer) error {
	catcher := grip.NewBasicCatcher()
	for _, cleanup := range cleanups {
		ctx, span := trace.Start(ctx, cleanup.Command)
		if err := cleanup.Run(ctx); err != nil {
			catcher.Wrapf(err, "running clean up from command '%s'", cleanup.Command)
			span.SetAttributes(
				attribute.String("evergreen.command.error", err.Error()),
			)
		}
		span.End()
	}
	return catcher.Resolve()
}

func (tc *taskContext) setFailingCommand(cmd command.Command) {
	tc.Lock()
	defer tc.Unlock()
	tc.failingCommand = cmd
}

func (tc *taskContext) getFailingCommand() command.Command {
	tc.RLock()
	defer tc.RUnlock()
	return tc.failingCommand
}

func (tc *taskContext) addFailingCommand(cmd command.Command) {
	tc.Lock()
	defer tc.Unlock()
	tc.otherFailingCommands = append(tc.otherFailingCommands, cmd)
}

func (tc *taskContext) getOtherFailingCommands() []apimodels.FailingCommand {
	tc.RLock()
	defer tc.RUnlock()
	var otherFailingCmds []apimodels.FailingCommand
	for _, failingCmd := range tc.otherFailingCommands {
		if tc.failingCommand != nil && failingCmd.FullDisplayName() == tc.failingCommand.FullDisplayName() {
			// Do not include the command that failed the task in the other
			// failing commands.
			continue
		}
		otherFailingCmds = append(otherFailingCmds, apimodels.FailingCommand{
			FullDisplayName:     failingCmd.FullDisplayName(),
			FailureMetadataTags: utility.UniqueStrings(failingCmd.FailureMetadataTags()),
		})
	}
	return otherFailingCmds
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
func (tc *taskContext) setCurrentIdleTimeout(cmd command.Command) {
	tc.Lock()
	defer tc.Unlock()

	var timeout time.Duration
	if dynamicTimeout := tc.taskConfig.GetIdleTimeout(); dynamicTimeout != 0 {
		timeout = time.Duration(dynamicTimeout) * time.Second
	} else if cmd.IdleTimeout() > 0 {
		timeout = cmd.IdleTimeout()
	} else if tc.taskConfig.Project.TimeoutSecs > 0 {
		timeout = time.Duration(tc.taskConfig.Project.TimeoutSecs) * time.Second
	} else {
		timeout = globals.DefaultIdleTimeout
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
	return globals.DefaultIdleTimeout
}

func (tc *taskContext) setHeartbeatTimeout(opts heartbeatTimeoutOptions) {
	if opts.getTimeout == nil {
		opts.getTimeout = func() time.Duration { return globals.DefaultHeartbeatTimeout }
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

func (tc *taskContext) reachTimeOut(kind globals.TimeoutType, dur time.Duration) {
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

func (tc *taskContext) oomTrackerEnabled(cloudProvider string) bool {
	return tc.taskConfig.Project.OomTracker && !utility.StringSliceContains(evergreen.ProviderContainer, cloudProvider)
}

func (tc *taskContext) setIdleTimeout(dur time.Duration) {
	tc.timeout.idleTimeoutDuration = dur
}

func (tc *taskContext) getIdleTimeout() time.Duration {
	return tc.timeout.idleTimeoutDuration
}

func (tc *taskContext) setTimedOut(timeout bool, kind globals.TimeoutType) {
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

func (tc *taskContext) getTimeoutType() globals.TimeoutType {
	return tc.timeout.timeoutType
}

func (tc *taskContext) getExecTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()
	if dynamicTimeout := tc.taskConfig.GetExecTimeout(); dynamicTimeout > 0 {
		if tc.taskConfig.MaxExecTimeoutSecs != 0 && dynamicTimeout > tc.taskConfig.MaxExecTimeoutSecs {
			return time.Duration(tc.taskConfig.MaxExecTimeoutSecs) * time.Second
		}
		return time.Duration(dynamicTimeout) * time.Second
	}
	if pt := tc.taskConfig.Project.FindProjectTask(tc.taskConfig.Task.DisplayName); pt != nil && pt.ExecTimeoutSecs > 0 {
		return time.Duration(pt.ExecTimeoutSecs) * time.Second
	}
	if tc.taskConfig.Project.ExecTimeoutSecs > 0 {
		return time.Duration(tc.taskConfig.Project.ExecTimeoutSecs) * time.Second
	}
	return globals.DefaultExecTimeout
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
	taskInfo, err := a.fetchTaskInfo(ctx, tc)
	if err != nil {
		return nil, errors.Wrap(err, "fetching task info")
	}

	grip.Info("Fetching distro configuration.")
	confDistro := &apimodels.DistroView{}
	confHost := &apimodels.HostView{}
	if a.opts.Mode == globals.HostMode {
		var err error
		confDistro, err = a.comm.GetDistroView(ctx, tc.task)
		if err != nil {
			return nil, errors.Wrap(err, "fetching distro view")
		}
		confHost, err = a.comm.GetHostView(ctx, tc.task)
		if err != nil {
			return nil, errors.Wrap(err, "fetching host view")
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
	if evergreen.IsGitHubPatchRequester(taskInfo.task.Requester) {
		grip.Info("Fetching patch document for GitHub PR request.")
		confPatch, err = a.comm.GetTaskPatch(ctx, tc.task)
		if err != nil {
			return nil, errors.Wrap(err, "fetching patch for GitHub PR request")
		}
	}

	var versionDoc *model.Version
	if confPatch == nil {
		grip.Info("Fetching version document for description.")
		versionDoc, err = a.comm.GetTaskVersion(ctx, tc.task)
		if err != nil {
			// Don't return an error since it's not essential to have the version.
			grip.Error("Error fetching version document for description.")
		}
	}

	grip.Info("Constructing task config.")
	tcOpts := internal.TaskConfigOptions{
		WorkDir:           a.opts.WorkingDirectory,
		Distro:            confDistro,
		Host:              confHost,
		Project:           taskInfo.project,
		Task:              taskInfo.task,
		DisplayTaskInfo:   taskInfo.displayTaskInfo,
		ProjectRef:        confRef,
		Patch:             confPatch,
		Version:           versionDoc,
		ExpansionsAndVars: taskInfo.expansionsAndVars,
	}
	taskConfig, err := internal.NewTaskConfig(tcOpts)
	if err != nil {
		return nil, err
	}
	taskConfig.TaskOutput = a.opts.SetupData.TaskOutput
	taskConfig.MaxExecTimeoutSecs = a.opts.SetupData.MaxExecTimeoutSecs

	// Set AWS credentials for task output buckets.
	awsCreds := pail.CreateAWSStaticCredentials(taskConfig.TaskOutput.Key, taskConfig.TaskOutput.Secret, "")
	if taskConfig.Task.TaskOutputInfo == nil {
		return nil, errors.New("agent retrieved a task with no task output info")
	}
	taskConfig.Task.TaskOutputInfo.TaskLogs.AWSCredentials = awsCreds
	taskConfig.Task.TaskOutputInfo.TestLogs.AWSCredentials = awsCreds
	taskConfig.Task.TaskOutputInfo.TestResults.AWSCredentials = awsCreds

	if a.opts.HostSecret != "" {
		// Redact the host secret from the logs. While the host secret isn't
		// supposed to be logged anywhere nor is it accessible by the task, this
		// is additional protection against leaking the secret due to situations
		// like a bug that unintentionally logs it.
		taskConfig.InternalRedactions.PutAndRedact(globals.HostSecret, a.opts.HostSecret)
	}

	return taskConfig, nil
}

// commandBlock contains information for a block of commands.
type commandBlock struct {
	block               command.BlockType
	commands            *model.YAMLCommandSet
	timeoutKind         globals.TimeoutType
	getTimeout          func() time.Duration
	canTimeOutHeartbeat bool
	canFailTask         bool
}

// getPre returns a command block containing the pre task commands.
func (tc *taskContext) getPre() (*commandBlock, error) {
	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{
			block:       command.PreBlock,
			commands:    tc.taskConfig.Project.Pre,
			timeoutKind: globals.PreTimeout,
			getTimeout:  tc.getPreTimeout,
			canFailTask: tc.taskConfig.Project.PreErrorFailsTask,
		}, nil
	}

	return &commandBlock{
		block:       command.SetupTaskBlock,
		commands:    tg.SetupTask,
		timeoutKind: globals.SetupTaskTimeout,
		getTimeout:  tc.getSetupTaskTimeout(),
		canFailTask: tg.SetupTaskCanFailTask,
	}, nil
}

// getPost returns a command block containing the post task commands.
func (tc *taskContext) getPost() (*commandBlock, error) {
	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{
			block:               command.PostBlock,
			commands:            tc.taskConfig.Project.Post,
			timeoutKind:         globals.PostTimeout,
			getTimeout:          tc.getPostTimeout,
			canTimeOutHeartbeat: true,
			canFailTask:         tc.taskConfig.Project.PostErrorFailsTask,
		}, nil
	}

	return &commandBlock{
		block:               command.TeardownTaskBlock,
		commands:            tg.TeardownTask,
		timeoutKind:         globals.TeardownTaskTimeout,
		getTimeout:          tc.getTeardownTaskTimeout(),
		canTimeOutHeartbeat: true,
		canFailTask:         tg.TeardownTaskCanFailTask,
	}, nil
}

// getSetupGroup returns the setup group for a task group task.
func (tc *taskContext) getSetupGroup() (*commandBlock, error) {
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
		timeoutKind: globals.SetupGroupTimeout,
		getTimeout:  tc.getSetupGroupTimeout(),
		canFailTask: tg.SetupGroupCanFailTask,
	}, nil
}

// getTeardownGroup returns the teardown group for a task group task.
func (tc *taskContext) getTeardownGroup() (*commandBlock, error) {
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
		timeoutKind: globals.TeardownGroupTimeout,
		getTimeout:  tc.getTeardownGroupTimeout(),
		canFailTask: false,
	}, nil
}

// getTimeout returns a command block containing the timeout handler commands.
func (tc *taskContext) getTimeout() (*commandBlock, error) {
	tg := tc.taskConfig.TaskGroup
	if tg == nil {
		return &commandBlock{
			block:               command.TaskTimeoutBlock,
			commands:            tc.taskConfig.Project.Timeout,
			timeoutKind:         globals.CallbackTimeout,
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
			timeoutKind:         globals.CallbackTimeout,
			getTimeout:          tc.getCallbackTimeout,
			canFailTask:         false,
			canTimeOutHeartbeat: true,
		}, nil
	}

	return &commandBlock{
		block:       command.TaskTimeoutBlock,
		commands:    tg.Timeout,
		timeoutKind: globals.CallbackTimeout,
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

	return globals.DefaultPreTimeout
}

// getPostTimeout returns the timeout for the post block.
func (tc *taskContext) getPostTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig.Project.PostTimeoutSecs != 0 {
		return time.Duration(tc.taskConfig.Project.PostTimeoutSecs) * time.Second
	}
	return globals.DefaultPostTimeout
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
		return globals.DefaultPreTimeout
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
		return globals.DefaultPreTimeout
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
		return globals.DefaultPostTimeout
	}
}

// getTeardownGroupTimeout returns the timeout for the teardown group block.
func (tc *taskContext) getTeardownGroupTimeout() func() time.Duration {
	return func() time.Duration {
		tc.RLock()
		defer tc.RUnlock()

		// if the task group timeout is over the max, use the max timeout
		tg := tc.taskConfig.TaskGroup
		if tg != nil && tg.TeardownGroupTimeoutSecs != 0 && tg.TeardownGroupTimeoutSecs < int(globals.MaxTeardownGroupTimeout.Seconds()) {
			return time.Duration(tg.TeardownGroupTimeoutSecs) * time.Second
		}
		return globals.MaxTeardownGroupTimeout
	}
}

// getCallbackTimeout returns the callback timeout for the timeout handler.
func (tc *taskContext) getCallbackTimeout() time.Duration {
	tc.RLock()
	defer tc.RUnlock()

	if tc.taskConfig.Project.CallbackTimeout != 0 {
		return time.Duration(tc.taskConfig.Project.CallbackTimeout) * time.Second
	}
	return globals.DefaultCallbackTimeout
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
		return globals.DefaultCallbackTimeout
	}
}

// setUserEndTaskResponse sets the user-defined end task response data.
func (tc *taskContext) setUserEndTaskResponse(resp *triggerEndTaskResp) {
	tc.Lock()
	defer tc.Unlock()

	tc.userEndTaskResp = resp
	tc.userEndTaskRespOriginatingCommand = tc.currentCommand
}

// getUserEndTaskResponse gets the user-defined end task response data.
func (tc *taskContext) getUserEndTaskResponse() *triggerEndTaskResp {
	tc.RLock()
	defer tc.RUnlock()

	return tc.userEndTaskResp
}

// setAddMetadataTagResponse sets the user-defined failure metadata tag data.
func (tc *taskContext) setAddMetadataTagResponse(resp *triggerAddMetadataTagResp) {
	tc.Lock()
	defer tc.Unlock()
	var existingTags []string
	if tc.addMetadataTagResp != nil {
		existingTags = tc.addMetadataTagResp.AddFailureMetadataTags
	}
	newTags := utility.UniqueStrings(append(existingTags, resp.AddFailureMetadataTags...))
	tc.addMetadataTagResp = &triggerAddMetadataTagResp{
		AddFailureMetadataTags: newTags,
	}
}

// getAddMetadataTagResponse gets the user-defined failure metadata tag data.
func (tc *taskContext) getAddMetadataTagResponse() *triggerAddMetadataTagResp {
	tc.RLock()
	defer tc.RUnlock()
	return tc.addMetadataTagResp
}

// getPlatformMountpoint returns the standard mountpoint used for the current OS.
func getPlatformMountpoint() string {
	switch runtime.GOOS {
	case "linux":
		return linuxMountpoint
	case "windows":
		return windowsMountpoint
	case "darwin":
		return darwinMountpoint
	default:
		return ""
	}
}

// getMountpoints returns the list of mountpoints to check for disk devices.
// Always includes "/" as the first mountpoint, followed by platform-specific mountpoint if available.
func (tc *taskContext) getMountpoints() []string {
	mountpoints := []string{"/"}
	if platformMountpoint := getPlatformMountpoint(); platformMountpoint != "" {
		mountpoints = append(mountpoints, platformMountpoint)
	}
	return mountpoints
}

// isWindowsDriveLetter checks if the device represents a Windows drive letter (e.g., "C:")
func isWindowsDriveLetter(device string) bool {
	return runtime.GOOS == "windows" &&
		len(device) == 2 &&
		device[1] == ':' &&
		device[0] >= 'A' && device[0] <= 'Z'
}

// getDeviceName extracts and sanitizes a device name from a device path.
// Returns the sanitized device name and a boolean indicating if it's valid.
func getDeviceName(device string) (string, bool) {
	deviceName := filepath.Base(device)

	// Handle edge cases where filepath.Base returns empty or path separators
	if deviceName == "" || deviceName == "/" || deviceName == "\\" {
		// Special case: Windows drive letters (e.g., "C:")
		if isWindowsDriveLetter(device) {
			deviceName = device
		} else {
			return "", false
		}
	}

	sanitizedDeviceName := instrumentNameDisallowedCharacters.ReplaceAllString(deviceName, "")
	return sanitizedDeviceName, sanitizedDeviceName != ""
}

// getDeviceNames returns the names of the devices mounted.
func (tc *taskContext) getDeviceNames(ctx context.Context) error {
	mountpoints := tc.getMountpoints()

	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return errors.Wrap(err, "getting partitions")
	}

	for _, partition := range partitions {
		if slices.Contains(mountpoints, partition.Mountpoint) {
			deviceName, valid := getDeviceName(partition.Device)
			if valid {
				tc.diskDevices = append(tc.diskDevices, deviceName)
			}
		}
	}
	return nil
}
