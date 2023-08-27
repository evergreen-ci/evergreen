package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	endTaskResp   *TriggerEndTaskResp
	tracer        trace.Tracer
	otelGrpcConn  *grpc.ClientConn
	closers       []closerOp
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

// Mode represents a mode that the agent will run in.
type Mode string

const (
	// HostMode indicates that the agent will run in a host.
	HostMode Mode = "host"
	// PodMode indicates that the agent will run in a pod's container.
	PodMode Mode = "pod"

	endTaskMessageLimit = 500
)

// LogOutput represents the output locations for the agent's logs.
type LogOutputType string

const (
	// LogOutputFile indicates that the agent will log to a file.
	LogOutputFile LogOutputType = "file"
	// LogOutputStdout indicates that the agent will log to standard output.
	LogOutputStdout LogOutputType = "stdout"
)

type taskContext struct {
	currentCommand            command.Command
	expansions                util.Expansions
	privateVars               map[string]bool
	logger                    client.LoggerProducer
	task                      client.TaskData
	taskGroup                 string
	ranSetupGroup             bool
	taskConfig                *internal.TaskConfig
	taskDirectory             string
	timeout                   timeoutInfo
	project                   *model.Project
	taskModel                 *task.Task
	oomTracker                jasper.OOMTracker
	traceID                   string
	unsetFunctionVarsDisabled bool
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
	execTimeout       timeoutType = "exec"
	idleTimeout       timeoutType = "idle"
	callbackTimeout   timeoutType = "callback"
	preTimeout        timeoutType = "pre"
	postTimeout       timeoutType = "post"
	setupGroupTimeout timeoutType = "setup_group"
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

	setupData, err := comm.GetAgentSetupData(ctx)
	grip.Alert(errors.Wrap(err, "getting agent setup data"))
	if setupData != nil {
		opts.SetupData = *setupData
		opts.TraceCollectorEndpoint = setupData.TraceCollectorEndpoint
	}

	a := &Agent{
		opts:   opts,
		comm:   comm,
		jasper: jpm,
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
	needPostGroup := false
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
			a.endTaskResp = nil // reset this in case a previous task used this to trigger a response
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
			nextTask, err := a.comm.GetNextTask(ctx, &apimodels.GetNextTaskDetails{
				TaskGroup:     tc.taskGroup,
				AgentRevision: evergreen.AgentVersion,
				EC2InstanceID: a.ec2InstanceID,
			})
			if err != nil {
				return errors.Wrap(err, "getting next task")
			}
			ntr, err := a.processNextTask(ctx, nextTask, tc, needPostGroup)
			if err != nil {
				return errors.Wrap(err, "processing next task")
			}
			needPostGroup = ntr.needPostGroup
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
	shouldExit    bool
	noTaskToRun   bool
	needPostGroup bool
	tc            *taskContext
}

func (a *Agent) processNextTask(ctx context.Context, nt *apimodels.NextTaskResponse, tc *taskContext, needPostGroup bool) (processNextResponse, error) {
	if nt.ShouldExit {
		grip.Notice("Next task response indicates agent should exit.")
		return processNextResponse{shouldExit: true}, nil
	}
	// if the host's current task group is finished we teardown
	if nt.ShouldTeardownGroup {
		a.runPostGroupCommands(ctx, tc)
		return processNextResponse{
			// Running the post group commands implies exiting the group, so
			// destroy prior task information.
			tc:            &taskContext{},
			needPostGroup: false,
		}, nil
	}

	if nt.TaskId == "" && needPostGroup {
		a.runPostGroupCommands(ctx, tc)
		return processNextResponse{
			needPostGroup: false,
			// Running the post group commands implies exiting the group, so
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
		tc:            tc,
		needPostGroup: true,
	}, nil
}

// finishPrevTask finishes up the previous task and returns information needed for the next task.
func (a *Agent) finishPrevTask(ctx context.Context, nextTask *apimodels.NextTaskResponse, tc *taskContext) (bool, string) {
	shouldSetupGroup := false
	taskDirectory := tc.taskDirectory
	if shouldRunSetupGroup(nextTask, tc) {
		shouldSetupGroup = true
		taskDirectory = ""
		a.runPostGroupCommands(ctx, tc)
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
			taskGroup:                 nt.TaskGroup,
			ranSetupGroup:             !shouldSetupGroup,
			taskDirectory:             taskDirectory,
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

	taskConfig, err := a.makeTaskConfig(setupCtx, tc)
	if err != nil {
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "making task config"))
	}
	tc.setTaskConfig(taskConfig)
	if err := a.startLogging(agentCtx, tc); err != nil {
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "setting up logger producer"))
	}

	if !tc.ranSetupGroup {
		tc.taskDirectory, err = a.createTaskDirectory(tc)
		if err != nil {
			return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "creating task directory"))
		}
	}
	tc.taskConfig.WorkDir = tc.taskDirectory
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
	if !tc.ranSetupGroup { // we didn't run setup group yet
		return true
	} else if tc.taskConfig == nil ||
		nextTask.TaskGroup == "" ||
		nextTask.Build != tc.taskConfig.Task.BuildId { // next task has a standalone task or a new build
		return true
	} else if nextTask.TaskGroup != tc.taskGroup { // next task has a different task group
		if tc.logger != nil && nextTask.TaskGroup == tc.taskConfig.Task.TaskGroup {
			tc.logger.Task().Warning(message.Fields{
				"message":                 "programmer error: task group in task context doesn't match task",
				"task_config_task_group":  tc.taskConfig.Task.TaskGroup,
				"task_context_task_group": tc.taskGroup,
				"next_task_task_group":    nextTask.TaskGroup,
			})
		}
		return true
	}

	return false
}

func (a *Agent) fetchProjectConfig(ctx context.Context, tc *taskContext) error {
	project, err := a.comm.GetProject(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "getting project")
	}

	taskModel, err := a.comm.GetTask(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "getting task")
	}

	expAndVars, err := a.comm.GetExpansionsAndVars(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "getting expansions and variables")
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

	tc.taskModel = taskModel
	tc.project = project
	tc.expansions = expAndVars.Expansions
	tc.privateVars = expAndVars.PrivateVars
	return nil
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
	if tc.project != nil && tc.project.Loggers != nil {
		tc.logger, err = a.makeLoggerProducer(ctx, tc, tc.project.Loggers, "")
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

	grip.Info(message.Fields{
		"message": "running task",
		"task_id": tc.task.ID,
	})

	tskCtx = utility.ContextWithAttributes(tskCtx, tc.taskConfig.TaskAttributes())
	tskCtx, span := a.tracer.Start(tskCtx, fmt.Sprintf("task: '%s'", tc.taskConfig.Task.DisplayName))
	defer span.End()
	tc.traceID = span.SpanContext().TraceID().String()

	shutdown, err := a.startMetrics(tskCtx, tc.taskConfig)
	grip.Error(errors.Wrap(err, "starting metrics collection"))
	if shutdown != nil {
		defer shutdown(ctx)
	}

	defer func() {
		tc.logger.Execution().Error(errors.Wrap(a.uploadTraces(tskCtx, tc.taskConfig.WorkDir), "uploading traces"))
	}()

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
	defer a.killProcs(ctx, tc, false, "task is finished")

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

	// set up the system stats collector
	statsCollector := NewSimpleStatsCollector(
		tc.logger,
		a.jasper,
		defaultStatsInterval,
		"uptime",
		"df -h",
		"${ps|ps}",
	)

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
	tc.logger.Task().Info("Running pre-task commands.")
	ctx, preTaskSpan := a.tracer.Start(ctx, "pre-task-commands")
	defer preTaskSpan.End()

	if !tc.ranSetupGroup {
		taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
		if err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for task setup group commands"))
			return nil
		}
		if taskGroup != nil && taskGroup.SetupGroup != nil {
			tc.logger.Task().Infof("Running setup group for task group '%s'.", taskGroup.Name)
			setupGroupCtx, setupGroupCancel := context.WithCancel(ctx)
			defer setupGroupCancel()

			var timeout time.Duration
			if taskGroup.SetupGroupTimeoutSecs > 0 {
				timeout = time.Duration(taskGroup.SetupGroupTimeoutSecs) * time.Second
			} else {
				timeout = tc.getCallbackTimeout()
			}
			timeoutOpts := timeoutWatcherOptions{
				tc:                    tc,
				kind:                  setupGroupTimeout,
				getTimeout:            func() time.Duration { return timeout },
				canMarkTimeoutFailure: taskGroup.SetupGroupFailTask,
			}
			go a.startTimeoutWatcher(setupGroupCtx, setupGroupCancel, timeoutOpts)

			opts := runCommandsOptions{
				block:       command.SetupGroupBlock,
				canFailTask: taskGroup.SetupGroupFailTask,
			}
			err = a.runCommandsInBlock(setupGroupCtx, tc, taskGroup.SetupGroup.List(), opts)
			if err != nil {
				tc.logger.Task().Error(errors.Wrap(err, "Running task setup group commands failed"))
				if taskGroup.SetupGroupFailTask {
					return err
				}
			}
			tc.logger.Task().Infof("Finished running setup group for task group '%s'.", taskGroup.Name)
		}
		tc.ranSetupGroup = true
	}

	pre, err := tc.taskConfig.GetPre(tc.taskGroup)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for pre-task commands"))
		return nil
	}

	if pre.Commands != nil {
		preCtx, preCancel := context.WithCancel(ctx)
		defer preCancel()
		timeoutOpts := timeoutWatcherOptions{
			tc:                    tc,
			kind:                  preTimeout,
			getTimeout:            tc.getPreTimeout,
			canMarkTimeoutFailure: pre.CanFailTask,
		}
		go a.startTimeoutWatcher(ctx, preCancel, timeoutOpts)

		block := command.PreBlock
		if tc.taskGroup != "" {
			block = command.SetupTaskBlock
		}
		opts := runCommandsOptions{
			canFailTask: pre.CanFailTask,
			block:       block,
		}
		err = a.runCommandsInBlock(preCtx, tc, pre.Commands.List(), opts)
		if err != nil {
			tc.logger.Task().Error(errors.Wrap(err, "Running pre-task commands failed"))
			if opts.canFailTask {
				return err
			}
		}
	}

	tc.logger.Task().InfoWhen(err == nil, "Finished running pre-task commands.")
	return nil
}

func (a *Agent) handleTaskResponse(ctx context.Context, tc *taskContext, status string, message string) (bool, error) {
	resp, err := a.finishTask(ctx, tc, status, message)
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

func (a *Agent) runTaskTimeoutCommands(ctx context.Context, tc *taskContext) {
	tc.logger.Task().Info("Running task-timeout commands.")
	start := time.Now()

	timeout, err := tc.taskConfig.GetTimeout(tc.taskGroup)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for task timeout commands"))
		return
	}
	if timeout != nil {
		timeoutCtx, timeoutCancel := context.WithCancel(ctx)
		defer timeoutCancel()
		timeoutOpts := timeoutWatcherOptions{
			tc:         tc,
			kind:       callbackTimeout,
			getTimeout: tc.getCallbackTimeout,
		}
		go a.startTimeoutWatcher(timeoutCtx, timeoutCancel, timeoutOpts)

		err := a.runCommandsInBlock(timeoutCtx, tc, timeout.List(), runCommandsOptions{block: command.TaskTimeoutBlock})
		tc.logger.Task().Error(errors.Wrap(err, "Running timeout commands failed"))
		tc.logger.Task().Infof("Finished running timeout commands in %s.", time.Since(start))
	}
}

// finishTask sends the returned EndTaskResponse and error
func (a *Agent) finishTask(ctx context.Context, tc *taskContext, status string, message string) (*apimodels.EndTaskResponse, error) {
	detail := a.endTaskResponse(ctx, tc, status, message)
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
		if err := a.runPostTaskCommands(ctx, tc); err != nil {
			tc.logger.Task().Error(errors.Wrap(err, "running post task commands"))
		}
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

func (a *Agent) endTaskResponse(ctx context.Context, tc *taskContext, status string, message string) *apimodels.TaskEndDetail {
	var userDefinedDescription string
	var userDefinedFailureType string
	if a.endTaskResp != nil { // if the user indicated a task response, use this instead
		tc.logger.Task().Infof("Task status set to '%s' with HTTP endpoint.", a.endTaskResp.Status)
		if !evergreen.IsValidTaskEndStatus(a.endTaskResp.Status) {
			tc.logger.Task().Errorf("'%s' is not a valid task status, defaulting to system failure.", a.endTaskResp.Status)
			status = evergreen.TaskFailed
			userDefinedFailureType = evergreen.CommandTypeSystem
		} else {
			status = a.endTaskResp.Status
			if len(a.endTaskResp.Description) > endTaskMessageLimit {
				tc.logger.Task().Warningf("Description from endpoint is too long to set (%d character limit), defaulting to command display name.", endTaskMessageLimit)
			} else {
				userDefinedDescription = a.endTaskResp.Description
			}

			if a.endTaskResp.Type != "" && !utility.StringSliceContains(evergreen.ValidCommandTypes, a.endTaskResp.Type) {
				tc.logger.Task().Warningf("'%s' is not a valid failure type, defaulting to command failure type.", a.endTaskResp.Type)
			} else {
				userDefinedFailureType = a.endTaskResp.Type
			}
		}
	}

	detail := &apimodels.TaskEndDetail{
		OOMTracker: tc.getOomTrackerInfo(),
		Message:    message,
		TraceID:    tc.traceID,
	}
	setEndTaskFailureDetails(tc, detail, status, userDefinedDescription, userDefinedFailureType)
	if tc.taskConfig != nil {
		detail.Modules.Prefixes = tc.taskConfig.ModulePaths
	}
	return detail
}

func setEndTaskFailureDetails(tc *taskContext, detail *apimodels.TaskEndDetail, status, description, failureType string) {
	var isDefaultInfo bool
	if tc.getCurrentCommand() != nil {
		// If there is no explicit user-defined description or failure type,
		// infer that information from the last command that ran.
		if description == "" {
			description = tc.getCurrentCommand().DisplayName()
			isDefaultInfo = true
		}
		if failureType == "" {
			failureType = tc.getCurrentCommand().Type()
			isDefaultInfo = true
		}
	}

	detail.Status = status
	if status != evergreen.TaskSucceeded || status == evergreen.TaskSucceeded && !isDefaultInfo {
		// If the task failed, always set the task failure information, because
		// the user will want to see which command failed. If the task
		// succeeded, the additional information is only necessary if a user
		// explicitly defined custom info to display. This avoids a potentially
		// confusing scenario where a task succeeds but has unnecesssary
		// information about the last command that succeeded.
		detail.Description = description
		detail.Type = failureType
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

func (a *Agent) runPostTaskCommands(ctx context.Context, tc *taskContext) error {
	ctx, span := a.tracer.Start(ctx, "post-task-commands")
	defer span.End()

	start := time.Now()
	a.killProcs(ctx, tc, false, "post task commands are starting")
	defer a.killProcs(ctx, tc, false, "post task commands are finished")
	tc.logger.Task().Info("Running post-task commands.")

	taskConfig := tc.getTaskConfig()
	post, err := taskConfig.GetPost(tc.taskGroup)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for post-task commands"))
		return nil
	}
	if post.Commands != nil {
		block := command.PostBlock
		if tc.taskGroup != "" {
			block = command.TeardownTaskBlock
		}

		postCtx, postCancel := context.WithCancel(ctx)
		defer postCancel()
		timeoutOpts := timeoutWatcherOptions{
			tc:                    tc,
			kind:                  postTimeout,
			getTimeout:            tc.getPostTimeout,
			canMarkTimeoutFailure: post.CanFailTask,
		}
		go a.startTimeoutWatcher(ctx, postCancel, timeoutOpts)

		opts := runCommandsOptions{
			canFailTask: post.CanFailTask,
			block:       block,
		}
		err = a.runCommandsInBlock(postCtx, tc, post.Commands.List(), opts)
		if err != nil {
			tc.logger.Task().Error(errors.Wrap(err, "Running post-task commands failed"))
			if post.CanFailTask {
				return err
			}
		}
	}
	tc.logger.Task().Infof("Finished running post-task commands in %s.", time.Since(start))
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
	defer a.killProcs(ctx, tc, true, "teardown group commands are finished")

	defer func() {
		if tc.logger != nil {
			// If the logger from the task is still open, running the teardown
			// group is the last thing that a task can do, so close the logger
			// to indicate logging is complete for the task.
			grip.Error(tc.logger.Close())
		}
	}()

	var logger grip.Journaler
	if tc.logger != nil {
		logger = tc.logger.Task()
	} else {
		logger = grip.GetDefaultJournaler()
	}

	taskGroup, err := tc.taskConfig.GetTaskGroup(tc.taskGroup)
	if err != nil {
		if tc.logger != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for post-group commands"))
		}
		return
	}
	if taskGroup != nil && taskGroup.TeardownGroup != nil {
		logger.Info("Running post-group commands")

		a.killProcs(ctx, tc, true, "teardown group commands are starting")

		teardownGroupCtx, teardownGroupCancel := context.WithCancel(ctx)
		defer teardownGroupCancel()
		timeoutOpts := timeoutWatcherOptions{
			tc:         tc,
			kind:       callbackTimeout,
			getTimeout: tc.getCallbackTimeout,
		}
		go a.startTimeoutWatcher(teardownGroupCtx, teardownGroupCancel, timeoutOpts)

		err := a.runCommandsInBlock(teardownGroupCtx, tc, taskGroup.TeardownGroup.List(), runCommandsOptions{block: command.TeardownGroupBlock})
		logger.Error(errors.Wrap(err, "Running post-group commands failed"))

		logger.Info("Finished running post-group commands.")
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

	err := a.runCommandsInBlock(syncCtx, tc, taskSyncCmds.List(), runCommandsOptions{block: command.TaskSyncBlock})
	tc.logger.Task().Error(errors.Wrap(err, "Running task sync commands failed"))
	tc.logger.Task().Infof("Finished running task sync in %s.", time.Since(start))
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
	if taskGroup != nil && taskGroup.ShareProcs {
		return false
	}
	// return true otherwise
	return true
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
