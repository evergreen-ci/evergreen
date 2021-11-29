package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty/docker"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// Agent manages the data necessary to run tasks on a host.
type Agent struct {
	comm          client.Communicator
	jasper        jasper.Manager
	opts          Options
	defaultLogger send.Sender
}

// Options contains startup options for an Agent.
type Options struct {
	// Mode determines which mode the agent will run in.
	Mode Mode
	// HostID and HostSecret only apply in host mode.
	HostID     string
	HostSecret string
	// PodID and PodSecret only apply in pod mode.
	PodID                 string
	PodSecret             string
	StatusPort            int
	LogPrefix             string
	LogkeeperURL          string
	WorkingDirectory      string
	HeartbeatInterval     time.Duration
	AgentSleepInterval    time.Duration
	MaxAgentSleepInterval time.Duration
	Cleanup               bool
	S3Opts                pail.S3Options
	SetupData             apimodels.AgentSetupData
	CloudProvider         string
}

// Mode represents a mode that the agent will run in.
type Mode string

const (
	// HostMode indicates that the agent will run in a host.
	HostMode Mode = "host"
	// PodMode indicates that the agent will run in a pod's container.
	PodMode Mode = "pod"
)

type taskContext struct {
	currentCommand         command.Command
	expansions             util.Expansions
	expVars                *apimodels.ExpansionVars
	logger                 client.LoggerProducer
	jasper                 jasper.Manager
	logs                   *apimodels.TaskLogs
	statsCollector         *StatsCollector
	systemMetricsCollector *systemMetricsCollector
	task                   client.TaskData
	taskGroup              string
	ranSetupGroup          bool
	taskConfig             *internal.TaskConfig
	taskDirectory          string
	logDirectories         map[string]interface{}
	timeout                timeoutInfo
	project                *model.Project
	taskModel              *task.Task
	oomTracker             jasper.OOMTracker
	sync.RWMutex
}

type timeoutInfo struct {
	// idleTimeoutDuration maintains the current idle timeout in the task context;
	// the exec timeout is maintained in the project data structure
	idleTimeoutDuration time.Duration
	timeoutType         timeoutType
	hadTimeout          bool
	// exceededDuration is the length of the timeout that was extended, if the task timed out
	exceededDuration time.Duration
}

type timeoutType string

const (
	execTimeout timeoutType = "exec"
	idleTimeout timeoutType = "idle"
)

// New creates a new Agent with some Options and a client.Communicator. Call the
// Agent's Start method to begin listening for tasks to run. Users should call
// Close when the agent is finished.
func New(ctx context.Context, opts Options, serverURL string) (*Agent, error) {
	var comm client.Communicator
	switch opts.Mode {
	case HostMode:
		comm = client.NewHostCommunicator(serverURL, opts.HostID, opts.HostSecret)
	case PodMode:
		comm = client.NewPodCommunicator(serverURL, opts.PodID, opts.PodSecret)
	default:
		return nil, errors.Errorf("unrecognized agent mode '%s'", opts.Mode)
	}
	return newWithCommunicator(ctx, opts, comm)
}

func newWithCommunicator(ctx context.Context, opts Options, comm client.Communicator) (*Agent, error) {
	jpm, err := jasper.NewSynchronizedManager(false)
	if err != nil {
		comm.Close()
		return nil, errors.WithStack(err)
	}

	if setupData, err := comm.GetAgentSetupData(ctx); err == nil {
		opts.SetupData = *setupData
		opts.LogkeeperURL = setupData.LogkeeperURL
		opts.S3Opts = pail.S3Options{
			Credentials: pail.CreateAWSCredentials(setupData.S3Key, setupData.S3Secret, ""),
			Region:      endpoints.UsEast1RegionID,
			Name:        setupData.S3Bucket,
			Permissions: pail.S3PermissionsPublicRead,
			ContentType: "text/plain",
		}
	}

	return &Agent{
		opts:   opts,
		comm:   comm,
		jasper: jpm,
	}, nil
}

func (a *Agent) Close() {
	if a.comm != nil {
		a.comm.Close()
	}
}

// Start starts the agent loop. The agent polls the API server for new tasks
// at interval agentSleepInterval and runs them.
func (a *Agent) Start(ctx context.Context) error {
	defer recovery.LogStackTraceAndExit("main agent thread")

	err := a.startStatusServer(ctx, a.opts.StatusPort)
	if err != nil {
		return errors.WithStack(err)
	}
	if a.opts.Cleanup {
		tryCleanupDirectory(a.opts.WorkingDirectory)
	}

	return errors.Wrap(a.loop(ctx), "error in agent loop, exiting")
}

func (a *Agent) loop(ctx context.Context) error {
	minAgentSleepInterval := defaultAgentSleepInterval
	maxAgentSleepInterval := defaultMaxAgentSleepInterval
	if a.opts.AgentSleepInterval != 0 {
		minAgentSleepInterval = a.opts.AgentSleepInterval
	}
	if a.opts.MaxAgentSleepInterval != 0 {
		maxAgentSleepInterval = a.opts.MaxAgentSleepInterval
	}
	agentSleepInterval := minAgentSleepInterval

	var jitteredSleep time.Duration
	tskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	timer := time.NewTimer(0)
	defer timer.Stop()

	tc := &taskContext{}
	needPostGroup := false
	defer func() {
		if tc.logger != nil {
			grip.Error(tc.logger.Close())
		}
	}()

LOOP:
	for {
		select {
		case <-ctx.Done():
			grip.Info("agent loop canceled")
			return nil
		case <-timer.C:
			// Check the cedar GRPC connection so we can fail early
			// and avoid task system failures.
			err := utility.Retry(ctx, func() (bool, error) {
				_, err := a.comm.GetCedarGRPCConn(ctx)
				return true, err
			}, utility.RetryOptions{MaxAttempts: 5, MaxDelay: minAgentSleepInterval})
			if err != nil {
				if ctx.Err() != nil {
					// We don't want to return an error if
					// the agent loop is canceled.
					return nil
				}
				return errors.Wrap(err, "cannot connect to cedar")
			}

			nextTask, err := a.comm.GetNextTask(ctx, &apimodels.GetNextTaskDetails{
				TaskGroup:     tc.taskGroup,
				AgentRevision: evergreen.AgentVersion,
			})
			if err != nil {
				// task secret doesn't match, get another task
				if errors.Cause(err) == client.HTTPConflictError {
					timer.Reset(0)
					agentSleepInterval = minAgentSleepInterval
					continue LOOP
				}
				return errors.Wrap(err, "error getting next task")
			}
			if nextTask.ShouldExit {
				grip.Notice("Next task response indicates agent should exit")
				return nil
			}
			// if the host's current task group is finished we teardown
			if nextTask.ShouldTeardownGroup {
				a.runPostGroupCommands(ctx, tc)
				needPostGroup = false
				// Running the post group commands implies exiting the group, so
				// destroy prior task information.
				tc = &taskContext{}
				timer.Reset(0)
				agentSleepInterval = minAgentSleepInterval
				continue LOOP
			}
			if nextTask.TaskId != "" {
				if nextTask.TaskSecret == "" {
					grip.Critical(message.WrapError(err, message.Fields{
						"message": "task response missing secret",
						"task":    tc.task.ID,
					}))
					timer.Reset(0)
					agentSleepInterval = minAgentSleepInterval
					continue LOOP
				}
				prevLogger := tc.logger
				tc = a.prepareNextTask(ctx, nextTask, tc)
				if prevLogger != nil {
					grip.Error(errors.Wrap(prevLogger.Close(), "problem closing the logger producer"))
				}

				grip.Error(message.WrapError(a.fetchProjectConfig(ctx, tc), message.Fields{
					"message": "error fetching project config; will attempt at a later point",
					"task":    tc.task.ID,
				}))

				a.jasper.Clear(ctx)
				tc.jasper = a.jasper
				shouldExit, err := a.runTask(tskCtx, tc)
				if err != nil {
					grip.Critical(message.WrapError(err, message.Fields{
						"message": "error running task",
						"task":    tc.task.ID,
					}))
					timer.Reset(0)
					agentSleepInterval = minAgentSleepInterval
					continue LOOP
				}
				if shouldExit {
					return nil
				}
				needPostGroup = true
				timer.Reset(0)
				agentSleepInterval = minAgentSleepInterval
				continue LOOP
			} else if needPostGroup {
				a.runPostGroupCommands(ctx, tc)
				needPostGroup = false
				// Running the post group commands implies exiting the group, so
				// destroy prior task information.
				tc = &taskContext{}
			}

			jitteredSleep = utility.JitterInterval(agentSleepInterval)
			grip.Debugf("Agent sleeping %s", jitteredSleep)
			timer.Reset(jitteredSleep)
			agentSleepInterval = agentSleepInterval * 2
			if agentSleepInterval > maxAgentSleepInterval {
				agentSleepInterval = maxAgentSleepInterval
			}
		}
	}
}

func (a *Agent) prepareNextTask(ctx context.Context, nextTask *apimodels.NextTaskResponse, tc *taskContext) *taskContext {
	shouldSetupGroup := false
	taskDirectory := tc.taskDirectory
	if shouldRunSetupGroup(nextTask, tc) {
		shouldSetupGroup = true
		taskDirectory = ""
		a.runPostGroupCommands(ctx, tc)
	}
	return &taskContext{
		task: client.TaskData{
			ID:     nextTask.TaskId,
			Secret: nextTask.TaskSecret,
		},
		taskGroup:     nextTask.TaskGroup,
		ranSetupGroup: !shouldSetupGroup,
		taskDirectory: taskDirectory,
		oomTracker:    jasper.NewOOMTracker(),
	}
}

func shouldRunSetupGroup(nextTask *apimodels.NextTaskResponse, tc *taskContext) bool {
	setupGroup := false
	var msg string
	if !tc.ranSetupGroup { // we didn't run setup group yet
		msg = "running setup group because we haven't yet"
		setupGroup = true
	} else if tc.taskConfig == nil ||
		nextTask.TaskGroup == "" ||
		nextTask.Build != tc.taskConfig.Task.BuildId { // next task has a standalone task or a new build
		msg = "running setup group because we have a new independent task"
		setupGroup = true
	} else if nextTask.TaskGroup != tc.taskGroup { // next task has a different task group
		if tc.logger != nil && nextTask.TaskGroup == tc.taskConfig.Task.TaskGroup {
			tc.logger.Task().Warning(message.Fields{
				"message":                 "programmer error: task group in task context doesn't match task",
				"task_config_task_group":  tc.taskConfig.Task.TaskGroup,
				"task_context_task_group": tc.taskGroup,
				"next_task_task_group":    nextTask.TaskGroup,
			})
		}
		msg = "running setup group because new task group"
		setupGroup = true
	}

	if tc.logger != nil {
		tc.logger.Task().DebugWhen(setupGroup, msg)
	}
	return setupGroup
}

func (a *Agent) fetchProjectConfig(ctx context.Context, tc *taskContext) error {
	project, err := a.comm.GetProject(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "error getting project")
	}

	taskModel, err := a.comm.GetTask(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "error getting task")
	}
	exp, err := a.comm.GetExpansions(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "error getting expansions")
	}
	expVars, err := a.comm.FetchExpansionVars(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "error getting task-specific expansions")
	}
	exp.Update(expVars.Vars)
	tc.taskModel = taskModel
	tc.project = project
	tc.expansions = exp
	tc.expVars = expVars
	return nil
}

func (a *Agent) startLogging(ctx context.Context, tc *taskContext) error {
	var err error

	if tc.logger != nil {
		grip.Error(errors.Wrap(tc.logger.Close(), "problem closing the logger producer"))
	}
	grip.Error(os.RemoveAll(filepath.Join(a.opts.WorkingDirectory, taskLogDirectory)))
	if tc.project != nil && tc.project.Loggers != nil {
		tc.logger, err = a.makeLoggerProducer(ctx, tc, tc.project.Loggers, "")
	} else {
		tc.logger, err = a.makeLoggerProducer(ctx, tc, &model.LoggerConfig{}, "")
	}
	if err != nil {
		return errors.WithStack(err)
	}

	sender, err := a.GetSender(ctx, a.opts.LogPrefix)
	grip.Error(errors.Wrap(err, "problem getting sender"))
	grip.Error(errors.Wrap(grip.SetSender(sender), "problem setting sender"))

	return nil
}

// runTask returns true if the agent should exit, and separate an error if relevant
func (a *Agent) runTask(ctx context.Context, tc *taskContext) (bool, error) {
	// we want to have separate context trees for tasks and loggers, so
	// when a task is canceled by a context, it can log its clean up.
	tskCtx, tskCancel := context.WithCancel(ctx)
	defer tskCancel()

	var err error
	defer func() {
		err = recovery.HandlePanicWithError(recover(), err, "running task")
	}()

	// If the heartbeat aborts the task immediately, we should report that
	// the task failed during initial task setup.
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		return false, errors.New("problem during configuring initial state")
	}
	tc.setCurrentCommand(factory())

	var taskConfig *internal.TaskConfig
	taskConfig, err = a.makeTaskConfig(ctx, tc)
	if err != nil {
		grip.Errorf("error fetching task configuration: %s", err)
		grip.Infof("task complete: %s", tc.task.ID)
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleTaskResponse(tskCtx, tc, evergreen.TaskFailed, "")
	}
	taskConfig.TaskSync = a.opts.SetupData.TaskSync
	tc.setTaskConfig(taskConfig)

	if err = a.startLogging(ctx, tc); err != nil {
		msg := errors.Wrap(err, "error setting up logger producer").Error()
		grip.Error(msg)
		grip.Infof("task complete: %s", tc.task.ID)
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleTaskResponse(tskCtx, tc, evergreen.TaskFailed, msg)
	}

	grip.Info(message.Fields{
		"message":     "running task",
		"task_id":     tc.task.ID,
		"task_secret": tc.task.Secret,
	})

	defer a.killProcs(ctx, tc, false)
	defer tskCancel()

	heartbeat := make(chan string, 1)
	go a.startHeartbeat(tskCtx, tskCancel, tc, heartbeat)

	innerCtx, innerCancel := context.WithCancel(tskCtx)

	go a.startIdleTimeoutWatch(tskCtx, tc, innerCancel)
	if utility.StringSliceContains(evergreen.ProviderSpotEc2Type, a.opts.CloudProvider) {
		go a.startEarlyTerminationWatcher(tskCtx, tc, agentutil.SpotHostWillTerminateSoon, agentutil.ExitAgent, nil)
	}

	complete := make(chan string)
	go a.startTask(innerCtx, tc, complete)

	return a.handleTaskResponse(tskCtx, tc, a.wait(tskCtx, innerCtx, tc, heartbeat, complete), "")
}

func (a *Agent) handleTaskResponse(ctx context.Context, tc *taskContext, status string, message string) (bool, error) {
	resp, err := a.finishTask(ctx, tc, status, message)
	if err != nil {
		return false, errors.Wrap(err, "error marking task complete")
	}
	if resp == nil {
		grip.Error("response was nil, indicating a 409 or an empty response from the API server")
		return false, nil
	}
	if resp.ShouldExit {
		return true, nil
	}
	return false, errors.WithStack(err)
}

func (a *Agent) wait(ctx, taskCtx context.Context, tc *taskContext, heartbeat chan string, complete chan string) string {
	status := evergreen.TaskFailed
	select {
	case <-taskCtx.Done():
		grip.Infof("task canceled: %s", tc.task.ID)
	case status = <-complete:
		grip.Infof("task complete: %s", tc.task.ID)
	case status = <-heartbeat:
		grip.Infof("received signal from heartbeat channel: %s", tc.task.ID)
	}

	if tc.hadTimedOut() && ctx.Err() == nil {
		status = evergreen.TaskFailed
		a.runTaskTimeoutCommands(ctx, tc)
	}

	if tc.oomTrackerEnabled(a.opts.CloudProvider) && status == evergreen.TaskFailed {
		startTime := time.Now()
		oomCtx, oomCancel := context.WithTimeout(ctx, time.Second*10)
		defer oomCancel()
		if err := tc.oomTracker.Check(oomCtx); err != nil {
			tc.logger.Execution().Errorf("error checking for OOM killed processes: %s", err)
		}
		if lines, _ := tc.oomTracker.Report(); len(lines) > 0 {
			tc.logger.Execution().Debugf("found an OOM kill (in %.3f seconds)", time.Now().Sub(startTime).Seconds())
			tc.logger.Execution().Debug(strings.Join(lines, "\n"))
		} else {
			tc.logger.Execution().Debugf("found no OOM kill (in %.3f seconds)", time.Now().Sub(startTime).Seconds())
		}
	}

	return status
}

func (a *Agent) runTaskTimeoutCommands(ctx context.Context, tc *taskContext) {
	tc.logger.Task().Info("Running task-timeout commands.")
	start := time.Now()
	var cancel context.CancelFunc
	ctx, cancel = a.withCallbackTimeout(ctx, tc)
	defer cancel()

	taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for task timeout commands"))
		return
	}
	if taskGroup.Timeout != nil {
		err := a.runCommands(ctx, tc, taskGroup.Timeout.List(), runCommandsOptions{})
		tc.logger.Execution().Error(message.WrapError(err, message.Fields{
			"message": "Error running timeout command",
		}))
		tc.logger.Task().InfoWhen(err == nil, message.Fields{
			"message":    "Finished running timeout commands",
			"total_time": time.Since(start).String(),
		})
	}
}

// finishTask sends the returned EndTaskResponse and error
func (a *Agent) finishTask(ctx context.Context, tc *taskContext, status string, message string) (*apimodels.EndTaskResponse, error) {
	detail := a.endTaskResponse(tc, status, message)
	switch detail.Status {
	case evergreen.TaskSucceeded:
		tc.logger.Task().Info("Task completed - SUCCESS.")
		if err := a.runPostTaskCommands(ctx, tc); err != nil {
			tc.logger.Task().Info("Post task completed -- FAILURE. Overall task status changed to FAILED.")
			detail.Status = evergreen.TaskFailed
		}
		a.runEndTaskSync(ctx, tc, detail)
	case evergreen.TaskFailed:
		tc.logger.Task().Info("Task completed - FAILURE.")
		if err := a.runPostTaskCommands(ctx, tc); err != nil {
			tc.logger.Task().Error(errors.Wrap(err, "error running post task commands"))
		}
		a.runEndTaskSync(ctx, tc, detail)
	case evergreen.TaskUndispatched:
		tc.logger.Task().Info("Task completed - ABORTED.")
	case evergreen.TaskConflict:
		tc.logger.Task().Error("Task completed - CANCELED.")
		// If we receive a 409, return control to the loop (ask for a new task)
		return nil, nil
	case evergreen.TaskSystemFailed:
		grip.Error("Task system failure")
	default:
		tc.logger.Task().Errorf("Programmer error: Invalid task status %s", detail.Status)
	}

	tc.Lock()
	if tc.systemMetricsCollector != nil {
		err := tc.systemMetricsCollector.Close()
		tc.logger.System().Error(errors.Wrap(err, "error closing system metrics collector"))
	}
	tc.Unlock()

	a.killProcs(ctx, tc, false)

	if tc.logger != nil {
		err := a.uploadToS3(ctx, tc)
		tc.logger.Execution().Error(errors.Wrap(err, "error uploading log files"))
		tc.logger.Execution().Infof("Sending final status as: %v", detail.Status)
		flush_ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		grip.Error(tc.logger.Flush(flush_ctx))
	}
	grip.Infof("Sending final status as: %v", detail.Status)
	resp, err := a.comm.EndTask(ctx, detail, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "problem marking task complete")
	}
	grip.Infof("Sent final status as: %v", detail.Status)

	return resp, nil
}

func (a *Agent) endTaskResponse(tc *taskContext, status string, message string) *apimodels.TaskEndDetail {
	var description string
	var cmdType string
	if tc.getCurrentCommand() != nil {
		description = tc.getCurrentCommand().DisplayName()
		cmdType = tc.getCurrentCommand().Type()
	}
	detail := &apimodels.TaskEndDetail{
		Description:     description,
		Type:            cmdType,
		TimedOut:        tc.hadTimedOut(),
		TimeoutType:     string(tc.getTimeoutType()),
		TimeoutDuration: tc.getTimeoutDuration(),
		OOMTracker:      tc.getOomTrackerInfo(),
		Status:          status,
		Message:         message,
		Logs:            tc.logs,
	}
	if tc.taskConfig != nil {
		detail.Modules.Prefixes = tc.taskConfig.ModulePaths
	}
	return detail
}

func (a *Agent) runPostTaskCommands(ctx context.Context, tc *taskContext) error {
	start := time.Now()
	a.killProcs(ctx, tc, false)
	defer a.killProcs(ctx, tc, false)
	tc.logger.Task().Info("Running post-task commands.")
	opts := runCommandsOptions{}
	postCtx, cancel := a.withCallbackTimeout(ctx, tc)
	defer cancel()
	taskConfig := tc.getTaskConfig()
	taskGroup, err := taskConfig.GetTaskGroup(tc.taskGroup)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for post-task commands"))
		return nil
	}
	if taskGroup.TeardownTask != nil {
		opts.failPreAndPost = taskGroup.TeardownTaskCanFailTask
		err = a.runCommands(postCtx, tc, taskGroup.TeardownTask.List(), opts)
		if err != nil {
			tc.logger.Task().Error(message.WrapError(err, message.Fields{
				"message": "Error running post-task command.",
			}))
			if taskGroup.TeardownTaskCanFailTask {
				return err
			}
		}
		tc.logger.Task().InfoWhen(err == nil, message.Fields{
			"message":    "Finished running post-task commands.",
			"total_time": time.Since(start).String(),
		})
	}
	return nil
}

func (a *Agent) runPostGroupCommands(ctx context.Context, tc *taskContext) {
	defer a.removeTaskDirectory(tc)
	if tc.taskConfig == nil {
		return
	}
	// Only killProcs if tc.taskConfig is not nil. This avoids passing an
	// empty working directory to killProcs, and is okay because this
	// killProcs is only for the processes run in runPostGroupCommands.
	defer a.killProcs(ctx, tc, true)
	defer func() {
		if tc.logger != nil {
			grip.Error(tc.logger.Close())
		}
	}()
	taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
	if err != nil {
		if tc.logger != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for post-group commands"))
		}
		return
	}
	if taskGroup.TeardownGroup != nil {
		grip.Info("Running post-group commands")
		a.killProcs(ctx, tc, true)
		var cancel context.CancelFunc
		ctx, cancel = a.withCallbackTimeout(ctx, tc)
		defer cancel()
		err := a.runCommands(ctx, tc, taskGroup.TeardownGroup.List(), runCommandsOptions{})
		grip.Error(message.WrapError(err, message.Fields{
			"message": "Error running post-task command.",
		}))
		grip.InfoWhen(err == nil, "Finished running post-group commands")
	}
}

// runEndTaskSync runs task sync if it was requested for the end of this task.
func (a *Agent) runEndTaskSync(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail) {
	if tc.taskModel == nil {
		tc.logger.Task().Error("Task model not found for running task sync.")
		return
	}
	start := time.Now()
	taskSyncCmds := endTaskSyncCommands(tc, detail)
	if taskSyncCmds == nil {
		return
	}

	var syncCtx context.Context
	var cancel context.CancelFunc
	if timeout := tc.taskModel.SyncAtEndOpts.Timeout; timeout != 0 {
		syncCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		// Default to a generously long timeout if none is specified, since
		// the sync could be a long operation.
		syncCtx, cancel = context.WithTimeout(ctx, evergreen.DefaultTaskSyncAtEndTimeout)
	}
	defer cancel()

	if err := a.runCommands(syncCtx, tc, taskSyncCmds.List(), runCommandsOptions{}); err != nil {
		tc.logger.Task().Error(message.WrapError(err, message.Fields{
			"message":    "Error running task sync.",
			"total_time": time.Since(start).String(),
		}))
		return
	}
	tc.logger.Task().Info(message.Fields{
		"message":    "Finished running task sync.",
		"total_time": time.Since(start).String(),
	})
}

func (a *Agent) killProcs(ctx context.Context, tc *taskContext, ignoreTaskGroupCheck bool) {
	logger := grip.NewJournaler("killProcs")
	if tc.logger != nil && !tc.logger.Closed() {
		logger = tc.logger.Execution()
	}

	if a.shouldKill(tc, ignoreTaskGroupCheck) {
		if tc.task.ID != "" && tc.taskConfig != nil {
			logger.Infof("cleaning up processes for task: %s", tc.task.ID)
			if err := agentutil.KillSpawnedProcs(ctx, tc.task.ID, tc.taskConfig.WorkDir, logger); err != nil {
				// If the host is in a state where ps is timing out we need human intervention.
				if psErr := errors.Cause(err); psErr == agentutil.ErrPSTimeout {
					disableErr := a.comm.DisableHost(ctx, a.opts.HostID, apimodels.DisableInfo{Reason: psErr.Error()})
					logger.CriticalWhen(disableErr != nil, fmt.Sprintf("error disabling host: %s", disableErr))
				}
				logger.Critical(fmt.Sprintf("error cleaning up spawned processes: %v", err))
			}
			logger.Infof("cleaned up processes for task: '%s'", tc.task.ID)
		}

		logger.Info("cleaning up Docker artifacts")
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dockerTimeout)
		defer cancel()
		if err := docker.Cleanup(ctx, logger); err != nil {
			logger.Critical(fmt.Sprintf("error cleaning up Docker artifacts: %s", err))
		}
		logger.Info("cleaned up Docker artifacts")
	}
}

func (a *Agent) shouldKill(tc *taskContext, ignoreTaskGroupCheck bool) bool {
	// never kill if cleanup is false
	if !a.opts.Cleanup {
		return false
	}
	// kill if the task is not in a task group
	if tc.taskGroup == "" {
		return true
	}
	// kill if ignoreTaskGroupCheck is true
	if ignoreTaskGroupCheck {
		return true
	}
	taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
	if err != nil {
		return false
	}
	// do not kill if share_processes is set
	if taskGroup.ShareProcs {
		return false
	}
	// return true otherwise
	return true
}
