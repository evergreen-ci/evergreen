package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty/docker"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// Agent manages the data necessary to run tasks in a runtime environment.
type Agent struct {
	comm          client.Communicator
	jasper        jasper.Manager
	opts          Options
	defaultLogger send.Sender
	// ec2InstanceID is the instance ID from the instance metadata. This only
	// applies to EC2 hosts.
	ec2InstanceID string
	// setEndTaskResp sets the explicit task status, which can be set by the
	// user to override the final task status that would otherwise be used.
	setEndTaskResp      func(*triggerEndTaskResp)
	setEndTaskRespMutex sync.RWMutex
	tracer              trace.Tracer
	otelGrpcConn        *grpc.ClientConn
	closers             []closerOp
}

// Options contains startup options for an Agent.
type Options struct {
	// Mode determines which mode the agent will run in.
	Mode Mode
	// HostID and HostSecret only apply in host mode.
	HostID     string
	HostSecret string
	// PodID and PodSecret only apply in pod mode.
	PodID                  string
	PodSecret              string
	StatusPort             int
	LogPrefix              string
	LogOutput              LogOutputType
	WorkingDirectory       string
	HeartbeatInterval      time.Duration
	Cleanup                bool
	SetupData              apimodels.AgentSetupData
	CloudProvider          string
	TraceCollectorEndpoint string
	// SendTaskLogsToGlobalSender indicates whether task logs should also be
	// sent to the global agent file log.
	SendTaskLogsToGlobalSender bool
}

type timeoutInfo struct {
	// idleTimeoutDuration maintains the current idle timeout in the task context;
	// the exec timeout is maintained in the project data structure
	idleTimeoutDuration time.Duration
	timeoutType         timeoutType
	hadTimeout          bool
	// exceededDuration is the length of the timeout that was extended, if the task timed out
	exceededDuration time.Duration

	// heartbeatTimeoutOpts is used to determine when the heartbeat should time
	// out.
	heartbeatTimeoutOpts heartbeatTimeoutOptions
}

// heartbeatTimeoutOptions represent options for the heartbeat timeout.
type heartbeatTimeoutOptions struct {
	startAt    time.Time
	getTimeout func() time.Duration
	kind       timeoutType
}

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

	setupData, err := comm.GetAgentSetupData(ctx)
	grip.Alert(errors.Wrap(err, "getting agent setup data"))
	if setupData != nil {
		opts.SetupData = *setupData
		opts.TraceCollectorEndpoint = setupData.TraceCollectorEndpoint
	}

	a := &Agent{
		opts:           opts,
		comm:           comm,
		jasper:         jpm,
		setEndTaskResp: func(*triggerEndTaskResp) {},
	}

	a.closers = append(a.closers, closerOp{
		name: "communicator close",
		closerFn: func(ctx context.Context) error {
			if a.comm != nil {
				a.comm.Close()
			}
			return nil
		}})

	if err := a.initOtel(ctx); err != nil {
		grip.Error(errors.Wrap(err, "initializing otel"))
		a.tracer = otel.GetTracerProvider().Tracer("noop_tracer")
	}

	return a, nil
}

type closerOp struct {
	name     string
	closerFn func(context.Context) error
}

func (a *Agent) Close(ctx context.Context) {
	catcher := grip.NewBasicCatcher()
	for idx, closer := range a.closers {
		if closer.closerFn == nil {
			continue
		}

		grip.Info(message.Fields{
			"message": "calling closer",
			"index":   idx,
			"closer":  closer.name,
		})

		catcher.Add(closer.closerFn(ctx))
	}

	grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
		"message": "calling agent closers",
		"host_id": a.opts.HostID,
	}))
}

// Start starts the agent loop. The agent polls the API server for new tasks
// at interval agentSleepInterval and runs them.
func (a *Agent) Start(ctx context.Context) error {
	defer recovery.LogStackTraceAndExit("main agent thread")

	err := a.startStatusServer(ctx, a.opts.StatusPort)
	if err != nil {
		return errors.Wrap(err, "starting status server")
	}
	if a.opts.Cleanup {
		a.tryCleanupDirectory(a.opts.WorkingDirectory)
	}

	return errors.Wrap(a.loop(ctx), "executing main agent loop")
}

// populateEC2InstanceID sets the agent's instance ID based on the EC2 instance
// metadata service if it's an EC2 instance. If it's not an EC2 instance or the
// EC2 instance ID has already been populated, this is a no-op.
func (a *Agent) populateEC2InstanceID(ctx context.Context) {
	if a.ec2InstanceID != "" || !utility.StringSliceContains(evergreen.ProviderEc2Type, a.opts.CloudProvider) {
		return
	}

	instanceID, err := agentutil.GetEC2InstanceID(ctx)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":        "could not get EC2 instance ID",
			"host_id":        a.opts.HostID,
			"cloud_provider": a.opts.CloudProvider,
		}))
		return
	}

	a.ec2InstanceID = instanceID
}

// loop is responsible for continually polling for new tasks and processing them.
// and then tries again.
func (a *Agent) loop(ctx context.Context) error {
	agentSleepInterval := minAgentSleepInterval

	timer := time.NewTimer(0)
	defer timer.Stop()

	tc := &taskContext{}
	needTeardownGroup := false
	defer func() {
		if tc.logger != nil {
			// If the logger from the task is still open and the agent is
			// shutting down, close the logger to flush the remaining logs.
			grip.Error(errors.Wrap(tc.logger.Close(), "closing logger"))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			grip.Info("Agent loop canceled.")
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
				return errors.Wrap(err, "connecting to Cedar")
			}

			a.populateEC2InstanceID(ctx)
			var previousTaskGroup string
			if tc.taskConfig != nil && tc.taskConfig.TaskGroup != nil {
				previousTaskGroup = tc.taskConfig.TaskGroup.Name
			}
			nextTask, err := a.comm.GetNextTask(ctx, &apimodels.GetNextTaskDetails{
				TaskGroup:     previousTaskGroup,
				AgentRevision: evergreen.AgentVersion,
				EC2InstanceID: a.ec2InstanceID,
			})
			if err != nil {
				return errors.Wrap(err, "getting next task")
			}
			ntr, err := a.processNextTask(ctx, nextTask, tc, needTeardownGroup)
			if err != nil {
				return errors.Wrap(err, "processing next task")
			}
			needTeardownGroup = ntr.needTeardownGroup
			if ntr.tc != nil {
				tc = ntr.tc
			}

			if ntr.shouldExit {
				return nil
			}

			if ntr.noTaskToRun {
				jitteredSleep := utility.JitterInterval(agentSleepInterval)
				grip.Debugf("Agent found no task to run, sleeping %s.", jitteredSleep)
				timer.Reset(jitteredSleep)
				agentSleepInterval = agentSleepInterval * 2
				if agentSleepInterval > maxAgentSleepInterval {
					agentSleepInterval = maxAgentSleepInterval
				}
				continue
			}
			timer.Reset(0)
			agentSleepInterval = minAgentSleepInterval

		}
	}
}

type processNextResponse struct {
	shouldExit        bool
	noTaskToRun       bool
	needTeardownGroup bool
	tc                *taskContext
}

func (a *Agent) processNextTask(ctx context.Context, nt *apimodels.NextTaskResponse, tc *taskContext, needTeardownGroup bool) (processNextResponse, error) {
	if nt.ShouldExit {
		grip.Notice("Next task response indicates agent should exit.")
		return processNextResponse{shouldExit: true}, nil
	}
	if nt.ShouldTeardownGroup {
		// Tear down the task group if the task group is finished.
		a.runTeardownGroupCommands(ctx, tc)
		return processNextResponse{
			// Running the teardown group commands implies exiting the group, so
			// destroy prior task information.
			tc:                &taskContext{},
			needTeardownGroup: false,
		}, nil
	}

	if nt.TaskId == "" && needTeardownGroup {
		// Tear down the task group if there's no next task to run (i.e. there's
		// no more tasks in the task group), and the agent just finished a task
		// or task group.
		a.runTeardownGroupCommands(ctx, tc)
		return processNextResponse{
			needTeardownGroup: false,
			// Running the teardown group commands implies exiting the group, so
			// destroy prior task information.
			tc:          &taskContext{},
			noTaskToRun: true,
		}, nil
	}

	if nt.TaskId == "" {
		return processNextResponse{
			noTaskToRun: true,
		}, nil
	}

	if nt.TaskSecret == "" {
		grip.Critical(message.Fields{
			"message": "task response missing secret",
			"task":    tc.task.ID,
		})
		return processNextResponse{
			noTaskToRun: true,
		}, nil
	}
	shouldSetupGroup, taskDirectory := a.finishPrevTask(ctx, nt, tc)

	tc, shouldExit, err := a.runTask(ctx, nil, nt, shouldSetupGroup, taskDirectory)
	if err != nil {
		grip.Critical(message.WrapError(err, message.Fields{
			"message": "error running task",
			"task":    tc.task.ID,
		}))
		return processNextResponse{
			tc: tc,
		}, nil
	}
	if shouldExit {
		return processNextResponse{
			shouldExit: true,
			tc:         tc,
		}, nil
	}
	return processNextResponse{
		tc:                tc,
		needTeardownGroup: true,
	}, nil
}

// finishPrevTask finishes up the previous task and returns information needed for the next task.
func (a *Agent) finishPrevTask(ctx context.Context, nextTask *apimodels.NextTaskResponse, tc *taskContext) (bool, string) {
	var shouldSetupGroup bool
	var taskDirectory string
	if tc.taskConfig != nil {
		taskDirectory = tc.taskConfig.WorkDir
	}

	if shouldRunSetupGroup(nextTask, tc) {
		shouldSetupGroup = true
		taskDirectory = ""
		a.runTeardownGroupCommands(ctx, tc)
	}
	if tc.logger != nil {
		grip.Error(errors.Wrap(tc.logger.Close(), "closing the previous logger producer"))
	}
	a.jasper.Clear(ctx)
	return shouldSetupGroup, taskDirectory
}

// setupTask does some initial setup that the task needs before running such as initializing the logger, loading the task config
// data and setting the task directory.
func (a *Agent) setupTask(agentCtx, setupCtx context.Context, initialTC *taskContext, nt *apimodels.NextTaskResponse, shouldSetupGroup bool, taskDirectory string) (tc *taskContext, shouldExit bool, err error) {
	if initialTC == nil {
		tc = &taskContext{
			task: client.TaskData{
				ID:     nt.TaskId,
				Secret: nt.TaskSecret,
			},
			ranSetupGroup:             !shouldSetupGroup,
			oomTracker:                jasper.NewOOMTracker(),
			unsetFunctionVarsDisabled: nt.UnsetFunctionVarsDisabled,
		}
	} else {
		tc = initialTC
	}

	// If the heartbeat aborts the task immediately, we should report that
	// the task failed during initial task setup.
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		grip.Alert(errors.New("setup.initial command is not registered"))
	}
	if factory != nil {
		tc.setCurrentCommand(factory())
	}

	a.comm.UpdateLastMessageTime()

	a.setEndTaskRespMutex.Lock()
	a.setEndTaskResp = tc.setUserEndTaskResponse
	a.setEndTaskRespMutex.Unlock()

	taskConfig, err := a.makeTaskConfig(setupCtx, tc)
	if err != nil {
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "making task config"))
	}
	tc.taskConfig = taskConfig

	if err := a.startLogging(agentCtx, tc); err != nil {
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "setting up logger producer"))
	}

	if !tc.ranSetupGroup {
		taskDirectory, err = a.createTaskDirectory(tc)
		if err != nil {
			return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "creating task directory"))
		}
	}

	tc.taskConfig.WorkDir = taskDirectory
	tc.taskConfig.Expansions.Put("workdir", tc.taskConfig.WorkDir)

	// We are only calling this again to get the log for the current command after logging has been set up.
	if factory != nil {
		tc.setCurrentCommand(factory())
	}

	tc.logger.Task().Infof("Task logger initialized (agent version '%s' from Evergreen build revision '%s').", evergreen.AgentVersion, evergreen.BuildRevision)
	tc.logger.Execution().Info("Execution logger initialized.")
	tc.logger.System().Info("System logger initialized.")

	if err := setupCtx.Err(); err != nil {
		return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "making task config"))
	}

	hostname, err := os.Hostname()
	tc.logger.Execution().Info(errors.Wrap(err, "getting hostname"))
	if hostname != "" {
		tc.logger.Execution().Infof("Hostname is '%s'.", hostname)
	}
	tc.logger.Task().Infof("Starting task '%s', execution %d.", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)

	return tc, shouldExit, nil
}

func (a *Agent) handleSetupError(ctx context.Context, tc *taskContext, err error) (*taskContext, bool, error) {
	catcher := grip.NewBasicCatcher()
	grip.Error(err)
	catcher.Wrap(err, "handling setup error")
	tc.logger.Execution().Error(err)
	grip.Infof("Task complete: '%s'.", tc.task.ID)
	shouldExit, err := a.handleTaskResponse(ctx, tc, evergreen.TaskSystemFailed, err.Error())
	catcher.Wrap(err, "handling task response")
	return tc, shouldExit, catcher.Resolve()
}

func shouldRunSetupGroup(nextTask *apimodels.NextTaskResponse, tc *taskContext) bool {
	var previousTaskGroup string
	if tc.taskConfig != nil && tc.taskConfig.TaskGroup != nil {
		previousTaskGroup = tc.taskConfig.TaskGroup.Name
	}
	if !tc.ranSetupGroup { // we didn't run setup group yet
		return true
	} else if tc.taskConfig == nil ||
		nextTask.TaskGroup == "" ||
		nextTask.Build != tc.taskConfig.Task.BuildId { // next task has a standalone task or a new build
		return true
	} else if nextTask.TaskGroup != previousTaskGroup { // next task has a different task group
		return true
	}
	return false
}

func (a *Agent) fetchTaskInfo(ctx context.Context, tc *taskContext) (*task.Task, *model.Project, util.Expansions, map[string]bool, error) {
	project, err := a.comm.GetProject(ctx, tc.task)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "getting project")
	}

	taskModel, err := a.comm.GetTask(ctx, tc.task)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "getting task")
	}

	expAndVars, err := a.comm.GetExpansionsAndVars(ctx, tc.task)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "getting expansions and variables")
	}

	// GetExpansionsAndVars does not include build variant expansions or project
	// parameters, so load them from the project.
	for _, bv := range project.BuildVariants {
		if bv.Name == taskModel.BuildVariant {
			expAndVars.Expansions.Update(bv.Expansions)
			break
		}
	}
	expAndVars.Expansions.Update(expAndVars.Vars)
	for _, param := range project.Parameters {
		// If the key doesn't exist, the value will default to "" anyway; this
		// prevents an un-specified project parameter from overwriting
		// lower-priority expansions.
		if param.Value != "" {
			expAndVars.Expansions.Put(param.Key, param.Value)
		}
	}
	// Overwrite any empty values here since these parameters were explicitly
	// user-specified.
	expAndVars.Expansions.Update(expAndVars.Parameters)

	return taskModel, project, expAndVars.Expansions, expAndVars.PrivateVars, nil
}

func (a *Agent) startLogging(ctx context.Context, tc *taskContext) error {
	var err error

	// If the agent is logging to a file, this will re-initialize the sender to
	// log to a new file for the new task.
	sender, err := a.GetSender(ctx, a.opts.LogOutput, a.opts.LogPrefix, tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution)
	grip.Error(errors.Wrap(err, "getting sender"))
	grip.Error(errors.Wrap(grip.SetSender(sender), "setting sender"))

	if tc.logger != nil {
		grip.Error(errors.Wrap(tc.logger.Close(), "closing the logger producer"))
	}
	taskLogDir := filepath.Join(a.opts.WorkingDirectory, taskLogDirectory)
	grip.Error(errors.Wrapf(os.RemoveAll(taskLogDir), "removing task log directory '%s'", taskLogDir))
	if tc.taskConfig.Project.Loggers != nil {
		tc.logger, err = a.makeLoggerProducer(ctx, tc, tc.taskConfig.Project.Loggers, "")
	} else {
		tc.logger, err = a.makeLoggerProducer(ctx, tc, &model.LoggerConfig{}, "")
	}
	if err != nil {
		return errors.Wrap(err, "making the logger producer")
	}

	return nil
}

// runTask runs a task. It returns true if the agent should exit.
func (a *Agent) runTask(ctx context.Context, tcInput *taskContext, nt *apimodels.NextTaskResponse, shouldSetupGroup bool, taskDirectory string) (tc *taskContext, shouldExit bool, err error) {
	// we want to have separate context trees for tasks and loggers, so
	// when a task is canceled by a context, it can log its clean up.
	tskCtx, tskCancel := context.WithCancel(ctx)
	defer tskCancel()

	defer func() {
		op := "running task"
		pErr := recovery.HandlePanicWithError(recover(), nil, op)
		if pErr == nil {
			return
		}
		err = a.logPanic(tc.logger, pErr, err, op)
	}()

	// Setup occurs before the task is actually running, so it's not abortable. If setup is taking
	// a long time, timing it out after the heartbeat timeout ensures the operation will give up within
	// the same timeout as the usual amount of time a task is allowed to run before it must heartbeat.
	setupCtx, setupCancel := context.WithTimeout(tskCtx, evergreen.HeartbeatTimeoutThreshold)
	defer setupCancel()
	tc, shouldExit, err = a.setupTask(ctx, setupCtx, tcInput, nt, shouldSetupGroup, taskDirectory)
	if err != nil {
		return tc, shouldExit, errors.Wrap(err, "setting up task")
	}

	defer a.killProcs(ctx, tc, false, "task is finished")

	grip.Info(message.Fields{
		"message": "running task",
		"task_id": tc.task.ID,
	})

	tskCtx = utility.ContextWithAttributes(tskCtx, tc.taskConfig.TaskAttributes())
	tskCtx, span := a.tracer.Start(tskCtx, fmt.Sprintf("task: '%s'", tc.taskConfig.Task.DisplayName))
	defer span.End()
	tc.traceID = span.SpanContext().TraceID().String()

	tc.dataDisk, err = dataPartition(ctx)
	grip.Error(errors.Wrapf(err, "getting device name for '%s' partition", dataMountpoint))

	shutdown, err := a.startMetrics(tskCtx, tc.taskConfig)
	grip.Error(errors.Wrap(err, "starting metrics collection"))
	if shutdown != nil {
		defer shutdown(ctx)
	}

	defer func() {
		tc.logger.Execution().Error(errors.Wrap(a.uploadTraces(tskCtx, tc.taskConfig.WorkDir), "uploading traces"))
	}()

	tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	preAndMainCtx, preAndMainCancel := context.WithCancel(tskCtx)
	go a.startHeartbeat(tskCtx, preAndMainCancel, tc)

	status := a.runPreAndMain(preAndMainCtx, tc)
	shouldExit, err = a.handleTaskResponse(tskCtx, tc, status, "")
	return tc, shouldExit, err
}

// startTask performs initial task setup and then runs the pre and main blocks
// for the task. This method returns the status of the task after pre and main
// have run. Also note that it's critical that all operations in this method
// must respect the context. If the context errors, this must eventually return.
func (a *Agent) runPreAndMain(ctx context.Context, tc *taskContext) (status string) {
	defer func() {
		op := "running task pre and main blocks"
		pErr := recovery.HandlePanicWithError(recover(), nil, op)
		if pErr == nil {
			return
		}
		_ = a.logPanic(tc.logger, pErr, nil, op)
		status = evergreen.TaskSystemFailed
	}()

	if ctx.Err() != nil {
		tc.logger.Execution().Infof("Stopping task execution during setup: %s", ctx.Err())
		return evergreen.TaskSystemFailed
	}

	timeoutWatcherCtx, timeoutWatcherCancel := context.WithCancel(ctx)
	defer timeoutWatcherCancel()

	idleTimeoutCtx, idleTimeoutCancel := context.WithCancel(timeoutWatcherCtx)
	go a.startIdleTimeoutWatcher(timeoutWatcherCtx, idleTimeoutCancel, tc)

	execTimeoutCtx, execTimeoutCancel := context.WithCancel(idleTimeoutCtx)
	defer execTimeoutCancel()
	timeoutOpts := timeoutWatcherOptions{
		tc:                    tc,
		kind:                  execTimeout,
		getTimeout:            tc.getExecTimeout,
		canMarkTimeoutFailure: true,
	}
	go a.startTimeoutWatcher(timeoutWatcherCtx, execTimeoutCancel, timeoutOpts)

	tc.logger.Execution().Infof("Setting heartbeat timeout to type '%s'.", execTimeout)
	tc.setHeartbeatTimeout(heartbeatTimeoutOptions{
		startAt:    time.Now(),
		getTimeout: tc.getExecTimeout,
		kind:       execTimeout,
	})
	defer func() {
		tc.logger.Execution().Infof("Resetting heartbeat timeout from type '%s' back to default.", execTimeout)
		tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	}()

	// set up the system stats collector
	statsCollector := NewSimpleStatsCollector(
		tc.logger,
		a.jasper,
		defaultStatsInterval,
		"uptime",
		"df -h",
		"${ps|ps}",
	)
	// Running the `df` command on Unix systems displays inode
	// statistics without the `-i` flag by default. However, we need
	// to pass the flag explicitly for Linux, hence the conditional.
	// We do not include Windows in the conditional because running
	// `df -h -i` on Cygwin does not report these statistics.
	if runtime.GOOS == "linux" {
		statsCollector.Cmds = append(statsCollector.Cmds, "df -h -i")
	}

	statsCollector.logStats(execTimeoutCtx, tc.taskConfig.Expansions)

	if execTimeoutCtx.Err() != nil {
		tc.logger.Execution().Infof("Stopping task execution after setup: %s", execTimeoutCtx.Err())
		return evergreen.TaskSystemFailed
	}

	// notify API server that the task has been started.
	tc.logger.Execution().Info("Reporting task started.")
	if err := a.comm.StartTask(execTimeoutCtx, tc.task); err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "marking task started"))
		return evergreen.TaskSystemFailed
	}

	a.killProcs(execTimeoutCtx, tc, false, "task is starting")

	if err := a.runPreTaskCommands(execTimeoutCtx, tc); err != nil {
		return evergreen.TaskFailed
	}

	if tc.oomTrackerEnabled(a.opts.CloudProvider) {
		tc.logger.Execution().Info("OOM tracker clearing system messages.")
		if err := tc.oomTracker.Clear(execTimeoutCtx); err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "clearing OOM tracker system messages"))
		}
	}

	if err := a.runTaskCommands(execTimeoutCtx, tc); err != nil {
		return evergreen.TaskFailed
	}

	return evergreen.TaskSucceeded
}

func (a *Agent) runPreTaskCommands(ctx context.Context, tc *taskContext) error {
	ctx, preTaskSpan := a.tracer.Start(ctx, "pre-task-commands")
	defer preTaskSpan.End()

	if !tc.ranSetupGroup {
		setupGroup, err := tc.getSetupGroup()
		if err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "fetching setup-group commands"))
			return nil
		}

		if setupGroup.commands != nil {
			err = a.runCommandsInBlock(ctx, tc, *setupGroup)
			if err != nil && setupGroup.canFailTask {
				return err
			}
		}
		tc.ranSetupGroup = true
	}

	pre, err := tc.getPre()
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching pre-task commands"))
		return nil
	}

	if pre.commands != nil {
		err = a.runCommandsInBlock(ctx, tc, *pre)
		if err != nil && pre.canFailTask {
			return err
		}
	}

	return nil
}

// runTaskCommands runs all commands for the task currently assigned to the agent.
func (a *Agent) runTaskCommands(ctx context.Context, tc *taskContext) error {
	ctx, span := a.tracer.Start(ctx, "task-commands")
	defer span.End()

	task := tc.taskConfig.Project.FindProjectTask(tc.taskConfig.Task.DisplayName)

	if task == nil {
		return errors.Errorf("unable to find task '%s' in project '%s'", tc.taskConfig.Task.DisplayName, tc.taskConfig.Task.Project)
	}

	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "canceled while running task commands")
	}

	mainTask := commandBlock{
		block:       command.MainTaskBlock,
		commands:    &model.YAMLCommandSet{MultiCommand: task.Commands},
		canFailTask: true,
	}
	err := a.runCommandsInBlock(ctx, tc, mainTask)
	if err != nil {
		return err
	}

	return nil
}

func (a *Agent) runTaskTimeoutCommands(ctx context.Context, tc *taskContext) {
	timeout, err := tc.getTimeout()
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching task-timeout commands"))
		return
	}
	if timeout.commands != nil {
		// If the timeout commands error, ignore the error. runCommandsInBlock
		// already logged the error, and the timeout commands cannot cause the
		// task to fail since the task has already timed out.
		_ = a.runCommandsInBlock(ctx, tc, *timeout)
	}
}

func (a *Agent) runPostTaskCommands(ctx context.Context, tc *taskContext) error {
	ctx, span := a.tracer.Start(ctx, "post-task-commands")
	defer span.End()

	a.killProcs(ctx, tc, false, "post task commands are starting")
	defer a.killProcs(ctx, tc, false, "post task commands are finished")

	post, err := tc.getPost()
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching post-task commands"))
		return nil
	}

	if post.commands != nil {
		err = a.runCommandsInBlock(ctx, tc, *post)
		if err != nil && post.canFailTask {
			return err
		}
	}
	return nil
}

func (a *Agent) runTeardownGroupCommands(ctx context.Context, tc *taskContext) {
	defer a.removeTaskDirectory(tc)
	if tc.taskConfig == nil {
		return
	}
	// Only killProcs if tc.taskConfig is not nil. This avoids passing an
	// empty working directory to killProcs, and is okay because this
	// killProcs is only for the processes run in runTeardownGroupCommands.
	defer a.killProcs(ctx, tc, true, "teardown group commands are finished")

	defer func() {
		if tc.logger != nil {
			// If the logger from the task is still open, running the teardown
			// group is the last thing that a task can do, so close the logger
			// to indicate logging is complete for the task.
			grip.Error(tc.logger.Close())
		}
	}()

	teardownGroup, err := tc.getTeardownGroup()
	if err != nil {
		if tc.logger != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "fetching teardown-group commands"))
		}
		return
	}

	if teardownGroup.commands != nil {
		a.killProcs(ctx, tc, true, "teardown group commands are starting")

		_ = a.runCommandsInBlock(ctx, tc, *teardownGroup)
	}
}

// runEndTaskSync runs task sync if it was requested for the end of this task.
func (a *Agent) runEndTaskSync(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail) {
	taskSyncCmds := endTaskSyncCommands(tc, detail)
	if taskSyncCmds == nil {
		return
	}

	var timeout time.Duration
	if tc.taskConfig.Task.SyncAtEndOpts.Timeout != 0 {
		timeout = tc.taskConfig.Task.SyncAtEndOpts.Timeout
	} else {
		timeout = evergreen.DefaultTaskSyncAtEndTimeout
	}

	taskSync := commandBlock{
		block:       command.TaskSyncBlock,
		commands:    taskSyncCmds,
		timeoutKind: taskSyncTimeout,
		getTimeout: func() time.Duration {
			return timeout
		},
		canTimeOutHeartbeat: true,
	}

	// If the task sync commands error, ignore the error. runCommandsInBlock
	// already logged the error and the task sync commands cannot cause the task
	// to fail since they're optional.
	_ = a.runCommandsInBlock(ctx, tc, taskSync)
}

func (a *Agent) handleTaskResponse(ctx context.Context, tc *taskContext, status string, systemFailureDescription string) (bool, error) {
	resp, err := a.finishTask(ctx, tc, status, systemFailureDescription)
	if err != nil {
		return false, errors.Wrap(err, "marking task complete")
	}
	if resp == nil {
		grip.Error("Response was nil, indicating a 409 or an empty response from the API server.")
		return false, nil
	}
	if resp.ShouldExit {
		return true, nil
	}
	return false, errors.WithStack(err)
}

func (a *Agent) handleTimeoutAndOOM(ctx context.Context, tc *taskContext, status string) {
	if tc.hadTimedOut() && ctx.Err() == nil {
		status = evergreen.TaskFailed
		a.runTaskTimeoutCommands(ctx, tc)
	}

	if tc.oomTrackerEnabled(a.opts.CloudProvider) && status == evergreen.TaskFailed {
		startTime := time.Now()
		oomCtx, oomCancel := context.WithTimeout(ctx, 10*time.Second)
		defer oomCancel()
		tc.logger.Execution().Error(errors.Wrap(tc.oomTracker.Check(oomCtx), "checking for OOM killed processes"))
		if lines, _ := tc.oomTracker.Report(); len(lines) > 0 {
			tc.logger.Execution().Debugf("Found an OOM kill (in %.3f seconds).", time.Since(startTime).Seconds())
			tc.logger.Execution().Debug(strings.Join(lines, "\n"))
		} else {
			tc.logger.Execution().Debugf("Found no OOM kill (in %.3f seconds).", time.Since(startTime).Seconds())
		}
	}
}

// finishTask finishes up a running task. It runs any post-task command blocks
// such as timeout and post, then sends the final end task response.
func (a *Agent) finishTask(ctx context.Context, tc *taskContext, status string, systemFailureDescription string) (*apimodels.EndTaskResponse, error) {
	detail := a.endTaskResponse(ctx, tc, status, systemFailureDescription)
	switch detail.Status {
	case evergreen.TaskSucceeded:
		a.handleTimeoutAndOOM(ctx, tc, status)
		tc.logger.Task().Info("Task completed - SUCCESS.")
		if err := a.runPostTaskCommands(ctx, tc); err != nil {
			tc.logger.Task().Info("Post task completed - FAILURE. Overall task status changed to FAILED.")
			setEndTaskFailureDetails(tc, detail, evergreen.TaskFailed, "", "")
		}
		a.runEndTaskSync(ctx, tc, detail)
	case evergreen.TaskFailed:
		a.handleTimeoutAndOOM(ctx, tc, status)
		tc.logger.Task().Info("Task completed - FAILURE.")
		// If the post commands error, ignore the error. runCommandsInBlock
		// already logged the error, and the post commands cannot cause the
		// task to fail since the task already failed.
		_ = a.runPostTaskCommands(ctx, tc)
		a.runEndTaskSync(ctx, tc, detail)
	case evergreen.TaskSystemFailed:
		// This is a special status indicating that the agent failed for reasons
		// outside of a task's control (e.g. due to a panic). Therefore, it
		// intentionally does not run the post task logic, because the task is
		// likely in an unrecoverable state and should just give up. Note that
		// this is a distinct case from a task failing and intentionally setting
		// its failure type to system failed, as that is within a task's
		// control.
		tc.logger.Task().Error("Task encountered unexpected task lifecycle system failure.")
		detail.Status = evergreen.TaskFailed
		detail.Type = evergreen.CommandTypeSystem
		if detail.Description == "" {
			detail.Description = "task system-failed for unknown reasons"
		}
	default:
		tc.logger.Task().Errorf("Programmatic error: ending task with invalid task status '%s', defaulting to system failure.", detail.Status)
		detail.Status = evergreen.TaskFailed
		detail.Type = evergreen.CommandTypeSystem
		if detail.Description == "" {
			detail.Description = "task has invalid status"
		}
	}

	a.killProcs(ctx, tc, false, "task is ending")

	if tc.logger != nil {
		tc.logger.Execution().Infof("Sending final task status: '%s'.", detail.Status)
		flushCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		grip.Error(errors.Wrap(tc.logger.Flush(flushCtx), "flushing logs"))
	}
	grip.Infof("Sending final task status: '%s'.", detail.Status)
	resp, err := a.comm.EndTask(ctx, detail, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "marking task complete")
	}
	grip.Infof("Successfully sent final task status: '%s'.", detail.Status)

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(evergreen.TaskStatusOtelAttribute, detail.Status))
	if detail.Status != evergreen.TaskSucceeded {
		span.SetStatus(codes.Error, fmt.Sprintf("failing status '%s'", detail.Status))
	}

	return resp, nil
}

func (a *Agent) endTaskResponse(ctx context.Context, tc *taskContext, status string, systemFailureDescription string) *apimodels.TaskEndDetail {
	highestPriorityDescription := systemFailureDescription
	var userDefinedFailureType string
	if userEndTaskResp := tc.getUserEndTaskResponse(); userEndTaskResp != nil {
		tc.logger.Task().Infof("Task status set to '%s' with HTTP endpoint.", userEndTaskResp.Status)
		if !evergreen.IsValidTaskEndStatus(userEndTaskResp.Status) {
			tc.logger.Task().Errorf("'%s' is not a valid task status, defaulting to system failure.", userEndTaskResp.Status)
			status = evergreen.TaskFailed
			userDefinedFailureType = evergreen.CommandTypeSystem
		} else {
			status = userEndTaskResp.Status

			if len(userEndTaskResp.Description) > endTaskMessageLimit {
				tc.logger.Task().Warningf("Description from endpoint is too long to set (%d character limit), using default description.", endTaskMessageLimit)
			} else {
				highestPriorityDescription = userEndTaskResp.Description
			}

			if userEndTaskResp.Type != "" && !utility.StringSliceContains(evergreen.ValidCommandTypes, userEndTaskResp.Type) {
				tc.logger.Task().Warningf("'%s' is not a valid failure type, defaulting to command failure type.", userEndTaskResp.Type)
			} else {
				userDefinedFailureType = userEndTaskResp.Type
			}
		}
	}

	detail := &apimodels.TaskEndDetail{
		OOMTracker: tc.getOomTrackerInfo(),
		TraceID:    tc.traceID,
		DataDisk:   tc.dataDisk,
	}
	setEndTaskFailureDetails(tc, detail, status, highestPriorityDescription, userDefinedFailureType)
	if tc.taskConfig != nil {
		detail.Modules.Prefixes = tc.taskConfig.ModulePaths
	}
	return detail
}

func setEndTaskFailureDetails(tc *taskContext, detail *apimodels.TaskEndDetail, status, description, failureType string) {
	var isDefaultDescription bool
	if tc.getCurrentCommand() != nil {
		// If there is no explicit user-defined description or failure type,
		// infer that information from the last command that ran.
		if description == "" {
			description = tc.getCurrentCommand().DisplayName()
			isDefaultDescription = true
		}
		if failureType == "" {
			failureType = tc.getCurrentCommand().Type()
		}
	}

	detail.Status = status
	if status != evergreen.TaskSucceeded {
		detail.Type = failureType
		detail.Description = description
	}
	if !isDefaultDescription {
		detail.Description = description
	}

	if !detail.TimedOut {
		// Only set timeout details if a prior command in the task hasn't
		// already recorded a timeout. For example, if a command times out in
		// the main block, then another command times out in the post block, the
		// main block command should preserve the main block timeout.
		detail.TimedOut = tc.hadTimedOut()
		detail.TimeoutType = string(tc.getTimeoutType())
		detail.TimeoutDuration = tc.getTimeoutDuration()
	}

}

func (a *Agent) killProcs(ctx context.Context, tc *taskContext, ignoreTaskGroupCheck bool, reason string) {
	logger := grip.NewJournaler("killProcs")
	if tc.logger != nil && !tc.logger.Closed() {
		logger = tc.logger.Execution()
	}

	if !a.shouldKill(tc, ignoreTaskGroupCheck) {
		return
	}

	logger.Infof("Cleaning up task because %s", reason)

	if tc.task.ID != "" && tc.taskConfig != nil {
		logger.Infof("Cleaning up processes for task: '%s'.", tc.task.ID)
		if err := agentutil.KillSpawnedProcs(ctx, tc.task.ID, tc.taskConfig.WorkDir, logger); err != nil {
			// If the host is in a state where ps is timing out we need human intervention.
			if psErr := errors.Cause(err); psErr == agentutil.ErrPSTimeout {
				disableErr := a.comm.DisableHost(ctx, a.opts.HostID, apimodels.DisableInfo{Reason: psErr.Error()})
				logger.CriticalWhen(disableErr != nil, errors.Wrap(err, "disabling host due to ps timeout"))
			}
			logger.Critical(errors.Wrap(err, "cleaning up spawned processes"))
		}
		logger.Infof("Cleaned up processes for task: '%s'.", tc.task.ID)
	}

	// Agents running in containers don't have Docker available, so skip
	// Docker cleanup for them.
	if a.opts.Mode != PodMode {
		logger.Info("Cleaning up Docker artifacts.")
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dockerTimeout)
		defer cancel()
		if err := docker.Cleanup(ctx, logger); err != nil {
			logger.Critical(errors.Wrap(err, "cleaning up Docker artifacts"))
		}
		logger.Info("Cleaned up Docker artifacts.")
	}
}

func (a *Agent) shouldKill(tc *taskContext, ignoreTaskGroupCheck bool) bool {
	// Never kill if the agent is not configured to clean up.
	if !a.opts.Cleanup {
		return false
	}
	// Kill if the task is not in a task group.
	if tc.taskConfig.TaskGroup == nil {
		return true
	}
	// This is a task group, kill if ignoreTaskGroupCheck is true
	if ignoreTaskGroupCheck {
		return true
	}
	// This is a task group, kill if not sharing processes between tasks in the
	// task group.
	return !tc.taskConfig.TaskGroup.ShareProcs
}

// logPanic logs a panic to the task log and returns the panic error, along with
// the original error (if any). If there was no panic error, this is a no-op.
func (a *Agent) logPanic(logger client.LoggerProducer, pErr, originalErr error, op string) error {
	if pErr == nil {
		return nil
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(originalErr)
	catcher.Add(pErr)
	if logger != nil && !logger.Closed() {
		logMsg := message.Fields{
			"message":   "programmatic error: Evergreen agent hit panic",
			"operation": op,
		}
		logger.Task().Error(logMsg)
	}

	return catcher.Resolve()
}
