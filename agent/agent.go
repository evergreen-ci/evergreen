package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/internal/taskoutput"
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
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

const (
	hostAttribute = "evergreen.host"
	ps            = "ps"
)

var (
	shouldExitAttribute = fmt.Sprintf("%s.should_exit", hostAttribute)
	// globalFilesToCleanup are used for cleaning up at the end of the task; these files are appended to the home directory.
	globalFilesToCleanup = []string{".gitconfig", ".git-credentials", ".netrc"}
)

// Agent manages the data necessary to run tasks in a runtime environment.
type Agent struct {
	comm          client.Communicator
	jasper        jasper.Manager
	opts          Options
	defaultLogger send.Sender
	// setEndTaskResp sets the explicit task status, which can be set by the
	// user to override the final task status that would otherwise be used.
	setEndTaskResp      func(*triggerEndTaskResp)
	setEndTaskRespMutex sync.RWMutex
	// addMetadataTagResp adds a failure metadata tag to the task, which can be
	// appended to the final list of failure metadata tags that are set on task
	// completion.
	addMetadataTagResp  func(*triggerAddMetadataTagResp)
	addMetadataTagMutex sync.RWMutex
	tracer              trace.Tracer
	otelGrpcConn        *grpc.ClientConn
	closers             []closerOp
}

// Options contains startup options for an Agent.
type Options struct {
	// Mode determines which mode the agent will run in.
	Mode globals.Mode
	// HostID and HostSecret only apply in host mode.
	HostID     string
	HostSecret string
	// PodID and PodSecret only apply in pod mode.
	PodID                  string
	PodSecret              string
	StatusPort             int
	LogPrefix              string
	LogOutput              globals.LogOutputType
	WorkingDirectory       string
	HeartbeatInterval      time.Duration
	Cleanup                bool
	SetupData              apimodels.AgentSetupData
	CloudProvider          string
	TraceCollectorEndpoint string
	// SendTaskLogsToGlobalSender indicates whether task logs should also be
	// sent to the global agent file log.
	SendTaskLogsToGlobalSender bool
	HomeDirectory              string
	SingleTaskDistro           bool
}

// AddLoggableInfo is a helper to add relevant information about the agent
// runtime to the log message. This is typically to make high priority error
// logs more informative.
func (o *Options) AddLoggableInfo(msg message.Fields) message.Fields {
	if o.HostID != "" {
		msg["host_id"] = o.HostID
	}
	if o.PodID != "" {
		msg["pod_id"] = o.PodID
	}
	return msg
}

type timeoutInfo struct {
	// idleTimeoutDuration maintains the current idle timeout in the task context;
	// the exec timeout is maintained in the project data structure
	idleTimeoutDuration time.Duration
	timeoutType         globals.TimeoutType
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
	kind       globals.TimeoutType
}

// New creates a new Agent with some Options and a client.Communicator. Call the
// Agent's Start method to begin listening for tasks to run. Users should call
// Close when the agent is finished.
func New(ctx context.Context, opts Options, serverURL string) (*Agent, error) {
	var comm client.Communicator
	switch opts.Mode {
	case globals.HostMode:
		comm = client.NewHostCommunicator(serverURL, opts.HostID, opts.HostSecret)
	case globals.PodMode:
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
	if err != nil {
		msg := opts.AddLoggableInfo(message.Fields{
			"message": "error getting agent setup data",
		})
		grip.Alert(message.WrapError(err, msg))
	}
	if setupData != nil {
		opts.SetupData = *setupData
		opts.TraceCollectorEndpoint = setupData.TraceCollectorEndpoint
	}

	a := &Agent{
		opts:               opts,
		comm:               comm,
		jasper:             jpm,
		setEndTaskResp:     func(*triggerEndTaskResp) {},
		addMetadataTagResp: func(*triggerAddMetadataTagResp) {},
	}

	a.closers = append(a.closers, closerOp{
		name: "communicator close",
		closerFn: func(ctx context.Context) error {
			if a.comm != nil {
				a.comm.Close()
			}
			return nil
		}})

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

	if err := a.initOtel(ctx); err != nil {
		a.tracer = otel.GetTracerProvider().Tracer("noop_tracer")
	}

	err := a.startStatusServer(ctx, a.opts.StatusPort)
	if err != nil {
		return errors.Wrap(err, "starting status server")
	}
	if a.opts.Cleanup {
		a.tryCleanupDirectory(ctx, a.opts.WorkingDirectory)
	}

	return errors.Wrap(a.loop(ctx), "executing main agent loop")
}

// loop is responsible for continually polling for new tasks and processing them.
// and then tries again.
func (a *Agent) loop(ctx context.Context) error {
	agentSleepInterval := globals.MinAgentSleepInterval

	lastIdleAt := time.Now()
	timer := time.NewTimer(0)
	defer timer.Stop()

	tc := &taskContext{
		logger: client.NewSingleChannelLogHarness("default", a.defaultLogger),
	}
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
			var previousTaskGroup string
			if tc.taskConfig != nil && tc.taskConfig.TaskGroup != nil {
				previousTaskGroup = tc.taskConfig.TaskGroup.Name
			}
			nextTask, err := a.comm.GetNextTask(ctx, &apimodels.GetNextTaskDetails{
				TaskGroup:     previousTaskGroup,
				AgentRevision: evergreen.AgentVersion,
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
			// Single task distros should exit after running a single task.
			// However, if the task group needs tearing down, we should continue
			// the loop so the teardown group can run in the next iteration.
			if !needTeardownGroup && a.opts.SingleTaskDistro {
				return a.comm.DisableHost(ctx, a.opts.HostID, apimodels.DisableInfo{Reason: "Single task distro ran a task"})
			}
			if ntr.shouldExit {
				return nil
			}

			if ntr.noTaskToRun {
				sleepTime := utility.JitterInterval(agentSleepInterval)
				if nextTask.EstimatedMaxIdleDuration != 0 {
					// This is a simplified estimate of the time remaining till this host is considered idle.
					estimatedDurationLeft := nextTask.EstimatedMaxIdleDuration - time.Since(lastIdleAt)
					if estimatedDurationLeft < sleepTime*2 && estimatedDurationLeft > 0 {
						// This guarantees that the agent will try to get a new task before the host is considered idle.
						sleepTime = estimatedDurationLeft / 2
					}
				}
				grip.Debugf("Agent found no task to run, sleeping %s.", sleepTime)
				timer.Reset(sleepTime)
				agentSleepInterval = agentSleepInterval * 2
				if agentSleepInterval > globals.MaxAgentSleepInterval {
					agentSleepInterval = globals.MaxAgentSleepInterval
				}
				continue
			}
			lastIdleAt = time.Now()
			timer.Reset(0)
			agentSleepInterval = globals.MinAgentSleepInterval
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
	_, span := a.tracer.Start(ctx, "process-next-task")
	defer span.End()
	if nt.ShouldExit {
		msg := "next task response indicates agent should exit"
		span.SetStatus(codes.Error, msg)
		span.RecordError(errors.New(msg), trace.WithAttributes(
			attribute.Bool(shouldExitAttribute, nt.ShouldExit),
		))
		grip.Notice(msg)
		return processNextResponse{shouldExit: true}, nil
	}
	if nt.ShouldTeardownGroup {
		// Tear down the task group if the task group is finished.
		a.runTeardownGroupCommands(ctx, tc)

		var err error
		// Terminate host if a task groups is complete in a single host distro.
		if a.opts.SingleTaskDistro {
			err = a.comm.DisableHost(ctx, a.opts.HostID, apimodels.DisableInfo{Reason: "Single task distro ran a task"})
			if err != nil {
				span.RecordError(err, trace.WithAttributes(attribute.String("task.id", tc.task.ID)), trace.WithStackTrace(true))
				grip.Critical(message.WrapError(err, message.Fields{
					"message":    "error disabling host after task group completion",
					"task":       tc.task.ID,
					"host":       a.opts.HostID,
					"task_group": nt.TaskGroup,
				}))
			}
		}

		return processNextResponse{
			// Running the teardown group commands implies exiting the group, so
			// destroy prior task information.
			tc: &taskContext{
				logger: client.NewSingleChannelLogHarness("default", a.defaultLogger),
			},
			needTeardownGroup: false,
		}, err
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
			tc: &taskContext{
				logger: client.NewSingleChannelLogHarness("default", a.defaultLogger),
			},
			noTaskToRun: true,
		}, nil
	}

	if nt.TaskId == "" {
		return processNextResponse{
			noTaskToRun: true,
		}, nil
	}

	if nt.TaskSecret == "" {
		msg := "task response missing secret"
		span.SetStatus(codes.Error, msg)
		span.RecordError(errors.New(msg), trace.WithAttributes(attribute.String("task.id", tc.task.ID)))
		grip.Critical(message.Fields{
			"message": msg,
			"task":    tc.task.ID,
		})
		return processNextResponse{
			noTaskToRun: true,
		}, nil
	}
	shouldSetupGroup, taskDirectory := a.finishPrevTask(ctx, nt, tc)

	tc, shouldExit, err := a.runTask(ctx, nil, nt, shouldSetupGroup, taskDirectory)
	if err != nil {
		span.SetStatus(codes.Error, "error running task")
		span.RecordError(err, trace.WithAttributes(attribute.String("task.id", tc.task.ID)), trace.WithStackTrace(true))
		grip.Critical(message.WrapError(err, message.Fields{
			"message": "error running task",
			"task":    tc.task.ID,
		}))
		return processNextResponse{
			tc: tc,
		}, nil
	}
	if shouldExit {
		msg := "run task indicates agent should exit"
		span.SetStatus(codes.Error, msg)
		span.RecordError(errors.New(msg), trace.WithAttributes(
			attribute.Bool(shouldExitAttribute, shouldExit),
		))
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
			ranSetupGroup: !shouldSetupGroup,
			oomTracker:    jasper.NewOOMTracker(),
			logger:        client.NewSingleChannelLogHarness("default", a.defaultLogger),
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

	a.addMetadataTagMutex.Lock()
	a.addMetadataTagResp = tc.setAddMetadataTagResponse
	a.addMetadataTagMutex.Unlock()

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

	var taskGroupDirMissing bool
	if tc.ranSetupGroup {
		if _, err := os.Stat(taskDirectory); os.IsNotExist(err) {
			taskGroupDirMissing = true
			tc.logger.Execution().Noticef("Task directory '%s' was already created by a previous task group task, but is missing for this task group task (possibly because it was deleted by a command in a previous task group task), re-creating it.", taskDirectory)
		}
		tmpDir := filepath.Join(taskDirectory, "tmp")
		if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
			taskGroupDirMissing = true
			tc.logger.Execution().Noticef("Task temporary directory '%s' was already created by a previous task group task, but is missing for this task group task (possibly because it was deleted by a command in a previous task group task), re-creating it.", tmpDir)
		}
	}
	if !tc.ranSetupGroup || taskGroupDirMissing {
		taskDirectory, err = a.createTaskDirectory(tc, taskDirectory)
		if err != nil {
			return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "creating task directory"))
		}
	}
	tc.taskConfig.WorkDir = taskDirectory
	tc.taskConfig.NewExpansions.Put("workdir", tc.taskConfig.WorkDir)

	traceClient := otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(a.otelGrpcConn))
	// Set up a new task output directory regardless if the task is part of
	// a task group.
	opts := taskoutput.DirectoryOpts{
		Root:        tc.taskConfig.WorkDir,
		Tsk:         &tc.taskConfig.Task,
		Logger:      tc.logger,
		TraceClient: traceClient,
		RedactorOpts: redactor.RedactionOptions{
			Expansions:         tc.taskConfig.NewExpansions,
			Redacted:           tc.taskConfig.Redacted,
			InternalRedactions: tc.taskConfig.InternalRedactions,
		},
	}
	tc.taskConfig.TaskOutputDir = taskoutput.NewDirectory(opts)
	if err := tc.taskConfig.TaskOutputDir.Setup(); err != nil {
		return a.handleSetupError(setupCtx, tc, errors.Wrap(err, "creating task output directory"))
	}

	// We are only calling this again to get the log for the current command after logging has been set up.
	if factory != nil {
		tc.setCurrentCommand(factory())
	}

	tc.logger.Task().Infof("Task logger initialized (agent version '%s' from Evergreen build revision '%s').", evergreen.AgentVersion, evergreen.BuildRevision)
	tc.logger.Execution().Info("Execution logger initialized.")
	tc.logger.System().Info("System logger initialized.")

	tc.logger.Execution().Error(errors.Wrap(tc.getDeviceNames(setupCtx), "getting device names for disks"))

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

// shouldRunSetupGroup determines if the next task that's about to run is part
// of a new task group or is a standalone task.
// If so, then the task or task group is new to the host and should therefore
// perform task initialization, including setting up a new task directory and
// running setup group (for task groups only).
// If not, then the task is part of a task group, and it shares the task
// directory with the previous task in the task group that the agent ran.
func shouldRunSetupGroup(nextTask *apimodels.NextTaskResponse, tc *taskContext) bool {
	var previousTaskGroup string
	if tc.taskConfig != nil && tc.taskConfig.TaskGroup != nil {
		previousTaskGroup = tc.taskConfig.TaskGroup.Name
	}
	if !tc.ranSetupGroup { // The agent hasn't run setup group before.
		return true
	} else if tc.taskConfig == nil || // The agent hasn't run any task previously.
		nextTask.TaskGroup == "" || // The agent's previous task wasn't part of a task group.
		nextTask.Build != tc.taskConfig.Task.BuildId { // The next task is in a different build.
		return true
	} else if nextTask.TaskGroup != previousTaskGroup { // The next task has a different task group.
		return true
	} else if nextTask.TaskExecution > tc.taskConfig.Task.Execution { // The previous and next task are in the same task group but the next task has a higher execution number.
		return true
	}
	return false
}

type taskInfo struct {
	project           *model.Project
	task              *task.Task
	displayTaskInfo   *apimodels.DisplayTaskInfo
	expansionsAndVars *apimodels.ExpansionsAndVars
}

// fetchTaskInfo gets the Project, Task, ExpansionAndVars, and DisplayTaskInfo. It stores them inside
// a TaskConfigOptions struct- it does not set any of its other fields.
func (a *Agent) fetchTaskInfo(ctx context.Context, tc *taskContext) (*taskInfo, error) {
	opts := &taskInfo{}
	var err error
	opts.project, err = a.comm.GetProject(ctx, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "getting project")
	}

	opts.task, err = a.comm.GetTask(ctx, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "getting task")
	}

	// Reset S3Usage for this execution to avoid accumulating from previous restarts
	opts.task.S3Usage = task.S3Usage{}

	opts.expansionsAndVars, err = a.comm.GetExpansionsAndVars(ctx, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "getting expansions and variables")
	}

	opts.displayTaskInfo, err = a.comm.GetDisplayTaskInfoFromExecution(ctx, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "getting task's display task info")
	}

	// GetExpansionsAndVars does not include build variant expansions or project
	// parameters, so load them from the project.
	for _, bv := range opts.project.BuildVariants {
		if bv.Name == opts.task.BuildVariant {
			opts.expansionsAndVars.Expansions.Update(bv.Expansions)
			break
		}
	}
	opts.expansionsAndVars.Expansions.Update(opts.expansionsAndVars.Vars)
	for _, param := range opts.project.Parameters {
		// If the key doesn't exist, the value will default to "" anyway; this
		// prevents an un-specified project parameter from overwriting
		// lower-priority expansions.
		if param.Value != "" {
			opts.expansionsAndVars.Expansions.Put(param.Key, param.Value)
		}
	}
	// Overwrite any empty values here since these parameters were explicitly
	// user-specified.
	opts.expansionsAndVars.Expansions.Update(opts.expansionsAndVars.Parameters)

	return opts, nil
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
	tc.logger, err = a.makeLoggerProducer(ctx, tc, "")

	return errors.Wrap(err, "making the logger producer")
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
		err = a.logPanic(tc, pErr, err, op)
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

	defer func() {
		_ = a.killProcs(ctx, tc, false, "task is finished")
	}()

	grip.Info(message.Fields{
		"message": "running task",
		"task_id": tc.task.ID,
	})

	tskCtx = utility.ContextWithAttributes(tskCtx, tc.taskConfig.TaskAttributes())
	tskCtx, span := a.tracer.Start(tskCtx, "task")
	defer span.End()
	tc.traceID = span.SpanContext().TraceID().String()

	shutdown, err := a.startMetrics(tskCtx, tc.taskConfig)
	grip.Error(errors.Wrap(err, "starting metrics collection"))
	if shutdown != nil {
		defer shutdown(ctx)
	}

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
		_ = a.logPanic(tc, pErr, nil, op)
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
		kind:                  globals.ExecTimeout,
		getTimeout:            tc.getExecTimeout,
		canMarkTimeoutFailure: true,
	}
	go a.startTimeoutWatcher(timeoutWatcherCtx, execTimeoutCancel, timeoutOpts)

	tc.logger.Execution().Infof("Setting heartbeat timeout to type '%s'.", globals.ExecTimeout)
	tc.setHeartbeatTimeout(heartbeatTimeoutOptions{
		startAt:    time.Now(),
		getTimeout: tc.getExecTimeout,
		kind:       globals.ExecTimeout,
	})
	defer func() {
		tc.logger.Execution().Infof("Resetting heartbeat timeout from type '%s' back to default.", globals.ExecTimeout)
		tc.setHeartbeatTimeout(heartbeatTimeoutOptions{})
	}()

	// Set up the system stats collector.
	statsCmds := []string{"uptime", "df -h"}

	// Add ps command if configured in YAML or expansion (for backward compatibility) when default ps logging is not disabled.
	if psCmd := tc.getPSCommand(); psCmd != "" {
		statsCmds = append(statsCmds, psCmd)
	}

	statsCollector := NewSimpleStatsCollector(
		tc.logger,
		a.jasper,
		globals.DefaultStatsInterval,
		statsCmds...,
	)
	// Running the `df` command on Unix systems displays inode
	// statistics without the `-i` flag by default. However, we need
	// to pass the flag explicitly for Linux, hence the conditional.
	// We do not include Windows in the conditional because running
	// `df -h -i` on Cygwin does not report these statistics.
	if runtime.GOOS == "linux" {
		statsCollector.Cmds = append(statsCollector.Cmds, "df -h -i")
	}
	// On Linux and macOS, use lsof to display network connections with process information.
	// +c 0: display full command name without truncation
	// -i: show all internet connections (TCP, UDP, etc.)
	// -n: numeric addresses (no DNS resolution)
	// -P: numeric ports (no service name resolution)
	// If lsof is not available, the command will fail gracefully due to ContinueOnError.
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		statsCollector.Cmds = append(statsCollector.Cmds, "lsof +c 0 -i -n -P")
	}

	statsCollector.logStats(execTimeoutCtx, tc.taskConfig.Expansions)

	if execTimeoutCtx.Err() != nil {
		tc.logger.Execution().Infof("Stopping task execution after setup: %s", execTimeoutCtx.Err())
		return evergreen.TaskSystemFailed
	}

	// notify API server that the task has been started.
	tc.logger.Execution().Info("Reporting task started.")
	if err := a.comm.StartTask(execTimeoutCtx, tc.task, tc.traceID, tc.diskDevices); err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "marking task started"))
		return evergreen.TaskSystemFailed
	}

	_ = a.killProcs(execTimeoutCtx, tc, false, "task is starting")

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
		err := errors.Errorf("unable to find task '%s' in project '%s'", tc.taskConfig.Task.DisplayName, tc.taskConfig.Task.Project)
		tc.logger.Execution().Error(err)
		return err
	}

	if err := ctx.Err(); err != nil {
		err = errors.Wrap(err, "canceled before running task commands")
		tc.logger.Execution().Error(err)
		return err
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

// runDefaultTimeoutHandler extracts and logs PIDs of running processes when a task times out.
func (a *Agent) runDefaultTimeoutHandler(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail) {
	tc.logger.Execution().Info("Running default timeout handler to collect process information.")

	var currentCmdPID int
	var currentCmdName string
	if currentCmd := tc.getCurrentCommand(); currentCmd != nil {
		currentCmdName = currentCmd.FullDisplayName()
		tc.logger.Execution().Infof("Current command at timeout: %s", currentCmdName)
	}

	var runningPIDs []int
	var processDetails []string

	if a.jasper == nil {
		return
	}
	procs, err := a.jasper.List(ctx, options.Running)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "listing running processes during timeout"))
		return
	}
	tc.logger.Execution().Infof("Found %d running processes managed by Jasper", len(procs))
	for _, proc := range procs {
		info := proc.Info(ctx)
		if info.PID > 0 {
			runningPIDs = append(runningPIDs, info.PID)

			detail := fmt.Sprintf("PID %d: ID=%s, Running=%v",
				info.PID, info.ID, info.IsRunning)
			if !info.StartAt.IsZero() {
				detail += fmt.Sprintf(", StartTime=%s", info.StartAt.Format(time.RFC3339))
				detail += fmt.Sprintf(", RunningFor=%v", time.Since(info.StartAt))
			}
			if info.Complete {
				detail += fmt.Sprintf(", Complete=true, ExitCode=%d", info.ExitCode)
			}
			if info.Options.WorkingDirectory != "" {
				detail += fmt.Sprintf(", WorkingDir=%s", info.Options.WorkingDirectory)
			}
			if tags := proc.GetTags(); len(tags) > 0 {
				detail += fmt.Sprintf(", Tags=%v", tags)
			}
			if len(info.Options.Args) > 0 {
				detail += fmt.Sprintf(", Command Args=%v", info.Options.Args)
			}

			processDetails = append(processDetails, detail)

			if currentCmd := tc.getCurrentCommand(); currentCmd != nil && info.IsRunning {
				currentCmdPID = info.PID
			}
		}
	}
	if len(runningPIDs) > 0 {
		tc.logger.Execution().Infof("Process PIDs at timeout: %v", runningPIDs)
		tc.logger.Execution().Info("Detailed process information:")
		for _, detail := range processDetails {
			tc.logger.Execution().Info(detail)
		}

		if currentCmdPID > 0 {
			tc.logger.Execution().Infof("Suspected current command PID: %d", currentCmdPID)
		}

		tc.logger.Task().Infof("Default timeout handler collected %d process PIDs for debugging.", len(runningPIDs))
	} else {
		tc.logger.Execution().Info("No running processes found during timeout")
	}
	if detail.TimeoutProcessInfo == nil {
		detail.TimeoutProcessInfo = &apimodels.TimeoutProcessInfo{
			CurrentCommand:    currentCmdName,
			CurrentCommandPID: currentCmdPID,
			RunningPIDs:       runningPIDs,
			Timestamp:         time.Now(),
		}
		pidStrings := make([]string, len(runningPIDs))
		for i, n := range runningPIDs {
			pidStrings[i] = strconv.Itoa(n)
		}
		delimitedPids := strings.Join(pidStrings, ",")
		tc.taskConfig.NewExpansions.Put("timed_out_command_pid", strconv.Itoa(currentCmdPID))
		tc.taskConfig.NewExpansions.Put("timed_out_pids", delimitedPids)
	}
}

func (a *Agent) runPostOrTeardownTaskCommands(ctx context.Context, tc *taskContext) error {
	// We run the command cleanups in a defer in case any of the post commands add cleanups.
	// This will clean up anything added from commands running in pre, main, or post or the
	// task group's setup task, main, and teardown task. As well, if the task timed out,
	// it will run any cleanup commands that were added from it as well.
	// We also run it as the first defer since they are handled in LIFO order.
	defer tc.runTaskCommandCleanups(ctx, tc.logger, a.tracer)

	ctx, span := a.tracer.Start(ctx, "post-task-commands")
	defer span.End()

	_ = a.killProcs(ctx, tc, false, "post-task or teardown-task commands are starting")
	defer func() {
		_ = a.killProcs(ctx, tc, false, "post-task or teardown-task commands are finished")
	}()

	post, err := tc.getPost()
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "fetching post-task or teardown-task commands"))
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
	defer a.removeTaskDirectory(ctx, tc)
	if tc.taskConfig == nil {
		return
	}
	// Only killProcs if tc.taskConfig is not nil. This avoids passing an
	// empty working directory to killProcs, and is okay because this
	// killProcs is only for the processes run in runTeardownGroupCommands.
	defer func() {
		_ = a.killProcs(ctx, tc, true, "teardown group commands are finished")
	}()

	defer func() {
		if tc.logger != nil {
			// Flush teardown logs (e.g. global file cleanup) emitted after the
			// last explicit flush in finishTask (after rotating the logger) before closing.
			flushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_ = tc.logger.Flush(flushCtx)
			cancel()
			grip.Error(tc.logger.Close())
		}
	}()
	defer a.clearGlobalFiles(tc)

	teardownGroup, err := tc.getTeardownGroup()
	if err != nil {
		if tc.logger != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "fetching teardown-group commands"))
		}
		return
	}

	if teardownGroup.commands != nil {
		_ = a.killProcs(ctx, tc, true, "teardown group commands are starting")
		ctx = utility.ContextWithAttributes(ctx, tc.taskConfig.TaskAttributes())
		ctx, span := a.tracer.Start(ctx, "teardown_group")
		defer span.End()
		// The teardown group commands don't defer the span as to not include the
		// cleanups below, which add their own spans.
		cmdCtx, span := a.tracer.Start(ctx, "commands")
		_ = a.runCommandsInBlock(cmdCtx, tc, *teardownGroup)
		span.End()
		// Teardown groups should run all the remaining command cleanups.
		tc.runTaskCommandCleanups(ctx, tc.logger, a.tracer)
		tc.runSetupGroupCommandCleanups(ctx, tc.logger, a.tracer)
	}
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

func (a *Agent) handleTimeoutAndOOM(ctx context.Context, tc *taskContext, detail *apimodels.TaskEndDetail, status string) {
	if tc.hadTimedOut() && ctx.Err() == nil {
		status = evergreen.TaskFailed
		a.runTaskTimeoutCommands(ctx, tc)
	}

	if tc.oomTrackerEnabled(a.opts.CloudProvider) && status == evergreen.TaskFailed {
		startTime := time.Now()
		oomCtx, oomCancel := context.WithTimeout(ctx, 10*time.Second)
		defer oomCancel()
		tc.logger.Execution().Error(errors.Wrap(tc.oomTracker.Check(oomCtx), "checking for OOM killed processes"))
		if lines, pids := tc.oomTracker.Report(); len(lines) > 0 {
			tc.logger.Execution().Debugf("Found an OOM kill (in %.3f seconds).", time.Since(startTime).Seconds())
			tc.logger.Execution().Debug(strings.Join(lines, "\n"))
			detail.OOMTracker = &apimodels.OOMTrackerInfo{
				Detected: true,
				Pids:     pids,
			}
		} else {
			tc.logger.Execution().Debugf("Found no OOM kill (in %.3f seconds).", time.Since(startTime).Seconds())
		}
	}
}

// finishTask finishes up a running task. It runs any post-task command blocks
// such as timeout and post, then sends the final end task response.
func (a *Agent) finishTask(ctx context.Context, tc *taskContext, status string, systemFailureDescription string) (*apimodels.EndTaskResponse, error) {
	detail := a.endTaskResponse(ctx, tc, status, systemFailureDescription)
	if detail.TimedOut {
		a.runDefaultTimeoutHandler(ctx, tc, detail)
	}
	switch detail.Status {
	case evergreen.TaskSucceeded:
		a.handleTimeoutAndOOM(ctx, tc, detail, status)

		tc.logger.Task().Info("Task completed - SUCCESS.")
		if err := a.runPostOrTeardownTaskCommands(ctx, tc); err != nil {
			tc.logger.Task().Info("Post task completed - FAILURE. Overall task status changed to FAILED.")
			setEndTaskFailureDetails(tc, detail, evergreen.TaskFailed, "", "", nil)
		}

		detail.PostErrored = tc.getPostErrored()
		detail.OtherFailingCommands = tc.getOtherFailingCommands()
		updateEndTaskFailureDetailsForTestResults(tc, detail)

	case evergreen.TaskFailed:
		a.handleTimeoutAndOOM(ctx, tc, detail, status)

		tc.logger.Task().Info("Task completed - FAILURE.")
		// If the post commands error, ignore the error. runCommandsInBlock
		// already logged the error, and the post commands cannot cause the
		// task to fail since the task already failed.
		_ = a.runPostOrTeardownTaskCommands(ctx, tc)

		detail.PostErrored = tc.getPostErrored()
		detail.OtherFailingCommands = tc.getOtherFailingCommands()

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

	if addedMetadataTagResp := tc.getAddMetadataTagResponse(); addedMetadataTagResp != nil {
		tc.logger.Task().Infof("Appending extra failure metadata tags set with HTTP endpoint.")
		detail.FailureMetadataTags = utility.UniqueStrings(append(detail.FailureMetadataTags, addedMetadataTagResp.AddFailureMetadataTags...))
	}

	// Attempt automatic task output ingestion if the task output directory
	// was setup, regardless of the task status.
	if tc.taskConfig != nil && tc.taskConfig.TaskOutputDir != nil {
		toCtx, span := a.tracer.Start(ctx, "task-output-ingestion")
		tc.logger.Execution().Error(errors.Wrap(tc.taskConfig.TaskOutputDir.Run(toCtx), "ingesting task output"))
		span.End()
	}

	if err := a.killProcs(ctx, tc, false, "task is ending"); err != nil {
		// If the task is finished but the agent can't clean up all the
		// processes/Docker artifacts, disable the host because the next task
		// will start with lingering state from the prior task.
		tc.logger.Execution().Criticalf("Unable to clean up processes/Docker artifacts for finished task, disabling this host. Error: %s", err.Error())
		if disableErr := a.comm.DisableHost(ctx, a.opts.HostID, apimodels.DisableInfo{
			Reason: "could not clean up processes/Docker artifacts after task is finished",
		}); disableErr != nil {
			tc.logger.Execution().Criticalf("Unable to disable unhealthy host that has leftover processes/Docker artifacts. Error: %s", disableErr.Error())
		}
	}

	if tc.logger != nil {
		tc.logger.Execution().Infof("Sending final task status: '%s'.", detail.Status)
		flushCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		grip.Error(errors.Wrap(tc.logger.Flush(flushCtx), "flushing logs"))
	}

	if tc.logger != nil && tc.taskConfig != nil {
		tc.logger.Task().Infof("Task tracked %d S3 PUT requests during execution.", tc.taskConfig.Task.S3Usage.NumPutRequests)
	}

	grip.Infof("Sending final task status: '%s'.", detail.Status)
	resp, err := a.comm.EndTask(ctx, detail, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "marking task complete")
	}
	grip.Infof("Successfully sent final task status: '%s'.", detail.Status)

	if detail.Status != evergreen.TaskSucceeded {
		// In end task, the server may modify the task's TaskOutputInfo to
		// specify a failed bucket config for task and test logs.
		// If so, we need to rotate the logger to pick up the new bucket.
		if err := a.rotateLoggerToFailedBucket(ctx, tc); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{"message": "rotating logger to failed bucket"}))
		}
	}

	err = a.upsertCheckRun(ctx, tc)
	if err != nil {
		grip.Error(errors.Wrap(err, "upserting check run"))
		if tc.logger != nil {
			tc.logger.Task().Errorf("Error upserting check run: '%s'", err.Error())
		}
	}

	if tc.logger != nil {
		flushCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		grip.Error(errors.Wrap(tc.logger.Flush(flushCtx), "flushing logs"))
	}

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(evergreen.TaskStatusOtelAttribute, detail.Status))
	if detail.Status != evergreen.TaskSucceeded {
		span.SetStatus(codes.Error, fmt.Sprintf("failing status '%s'", detail.Status))
	}
	if detail.Type != "" {
		span.SetAttributes(attribute.String(evergreen.TaskFailureTypeOtelAttribute, detail.Type))
	}
	if detail.FailingCommand != "" {
		span.SetAttributes(attribute.String(evergreen.TaskFailingCommandOtelAttribute, detail.FailingCommand))
	}
	if detail.Description != "" {
		span.SetAttributes(attribute.String(evergreen.TaskDescriptionOtelAttribute, detail.Description))
	}

	return resp, nil
}

// bucketConfigsChanged returns true if either the task or test bucket config
// name changed between two TaskOutput snapshots.
func bucketConfigsChanged(oldOut, newOut task.TaskOutput) bool {
	return oldOut.TaskLogs.BucketConfig.Name != newOut.TaskLogs.BucketConfig.Name ||
		oldOut.TestLogs.BucketConfig.Name != newOut.TestLogs.BucketConfig.Name
}

// rotateLoggerToFailedBucket closes the current task logger and reinitializes with
// the current bucket config name for task and test logs.
func (a *Agent) rotateLoggerToFailedBucket(ctx context.Context, tc *taskContext) error {
	if tc == nil || tc.taskConfig == nil || tc.taskConfig.Task.TaskOutputInfo == nil {
		return errors.New("cannot rotate logger: missing task output info")
	}

	oldSnapshot := *tc.taskConfig.Task.TaskOutputInfo

	if tc.task.ID != "" {
		if updated, err := a.comm.GetTask(ctx, tc.task); err == nil && updated != nil {
			updated := updated.TaskOutputInfo
			if updated != nil {
				if name := updated.TaskLogs.BucketConfig.Name; name != "" {
					tc.taskConfig.Task.TaskOutputInfo.TaskLogs.BucketConfig.Name = name
				}
				if name := updated.TestLogs.BucketConfig.Name; name != "" {
					tc.taskConfig.Task.TaskOutputInfo.TestLogs.BucketConfig.Name = name
				}
			}
		} else if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "refreshing task after failure to get updated failed bucket config",
				"task_id": tc.task.ID,
			}))
		}
	}

	newSnapshot := *tc.taskConfig.Task.TaskOutputInfo
	if !bucketConfigsChanged(oldSnapshot, newSnapshot) {
		return nil
	}

	if tc.logger != nil && !tc.logger.Closed() {
		flushCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		_ = tc.logger.Flush(flushCtx)
		cancel()
		_ = tc.logger.Close()
	}
	return errors.Wrap(a.startLogging(ctx, tc), "restarting logger for failed bucket")
}

func (a *Agent) upsertCheckRun(ctx context.Context, tc *taskContext) error {
	if tc.taskConfig == nil {
		return nil
	}

	checkRunOutput, err := buildCheckRun(ctx, tc)
	if err != nil {
		return err
	}
	if checkRunOutput == nil {
		return nil
	}

	if err = a.comm.UpsertCheckRun(ctx, tc.task, *checkRunOutput); err != nil {
		return err
	}

	tc.logger.Task().Infof("Successfully upserted checkRun.")
	return nil
}

func buildCheckRun(ctx context.Context, tc *taskContext) (*apimodels.CheckRunOutput, error) {
	fileNamePointer := tc.taskConfig.Task.CheckRunPath
	// no checkRun specified
	if fileNamePointer == nil || !evergreen.IsGithubPRRequester(tc.taskConfig.Task.Requester) {
		return nil, nil
	}

	fileName := utility.FromStringPtr(fileNamePointer)
	checkRunOutput := apimodels.CheckRunOutput{}
	if fileName == "" {
		tc.logger.Task().Infof("Upserting check run with no output file specified.")
		return &checkRunOutput, nil
	}

	fileName, err := tc.taskConfig.Expansions.ExpandString(fileName)
	if err != nil {
		return nil, errors.New("Error expanding check run output file")
	}

	fileName = command.GetWorkingDirectory(tc.taskConfig, fileName)

	_, err = os.Stat(fileName)
	if os.IsNotExist(err) {
		checkRunOutput.Title = "Error getting check run output"
		checkRunOutput.Summary = "Evergreen couldn't find the check run output file"
		tc.logger.Task().Errorf("Attempting to create check run but file '%s' does not exist", fileName)
		return &checkRunOutput, errors.Wrap(err, "getting check run output")
	}

	err = utility.ReadJSONFile(fileName, &checkRunOutput)
	if err != nil {
		return nil, errors.Wrap(err, "reading checkRun output file")
	}

	if err := util.ExpandValues(&checkRunOutput, &tc.taskConfig.Expansions); err != nil {
		return nil, errors.New("Error expanding values for check run output")
	}

	return &checkRunOutput, nil

}

func (a *Agent) endTaskResponse(ctx context.Context, tc *taskContext, status string, systemFailureDescription string) *apimodels.TaskEndDetail {
	highestPriorityDescription := systemFailureDescription
	var userDefinedFailureType string
	var userDefinedFailureMetadataTags []string
	if userEndTaskResp := tc.getUserEndTaskResponse(); userEndTaskResp != nil {
		tc.logger.Task().Infof("Task status set to '%s' with HTTP endpoint.", userEndTaskResp.Status)
		if !evergreen.IsValidTaskEndStatus(userEndTaskResp.Status) {
			tc.logger.Task().Errorf("'%s' is not a valid task status, defaulting to system failure.", userEndTaskResp.Status)
			status = evergreen.TaskFailed
			userDefinedFailureType = evergreen.CommandTypeSystem
		} else {
			status = userEndTaskResp.Status
			userDefinedFailureMetadataTags = userEndTaskResp.AddFailureMetadataTags

			if len(userEndTaskResp.Description) > globals.EndTaskMessageLimit {
				tc.logger.Task().Warningf("Description from endpoint is too long to set (%d character limit), using default description.", globals.EndTaskMessageLimit)
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
		TraceID:     tc.traceID,
		DiskDevices: tc.diskDevices,
	}
	setEndTaskFailureDetails(tc, detail, status, highestPriorityDescription, userDefinedFailureType, userDefinedFailureMetadataTags)
	if tc.taskConfig != nil {
		detail.Modules.Prefixes = tc.taskConfig.ModulePaths
	}
	return detail
}

func setEndTaskFailureDetails(tc *taskContext, detail *apimodels.TaskEndDetail, status, description, failureType string, failureMetadataTagsToAdd []string) {
	currCmd := tc.getCurrentCommand()
	if currCmd != nil && failureType == "" {
		// If there is no explicit user-defined failure type,
		// infer that information from the last command that ran.
		failureType = currCmd.Type()
	}

	detail.Status = status
	detail.Description = description
	if status != evergreen.TaskSucceeded {
		if tc.userEndTaskRespOriginatingCommand != nil {
			detail.FailingCommand = tc.userEndTaskRespOriginatingCommand.FullDisplayName()
			tc.setFailingCommand(tc.userEndTaskRespOriginatingCommand)
		} else {
			detail.FailingCommand = currCmd.FullDisplayName()
			tc.setFailingCommand(currCmd)
		}
		detail.Type = failureType
		detail.FailureMetadataTags = utility.UniqueStrings(append(tc.getFailingCommand().FailureMetadataTags(), failureMetadataTagsToAdd...))
	}

	detail.OtherFailingCommands = tc.getOtherFailingCommands()

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

// updateEndTaskFailureDetailsForTestResults checks and updates the task failure
// details for missing or failed test results.
func updateEndTaskFailureDetailsForTestResults(tc *taskContext, detail *apimodels.TaskEndDetail) {
	if detail.Status == evergreen.TaskFailed {
		// If the task has already failed for another reason, do not overwrite
		// it with a test result-related failure. Test results failures are
		// lower priority.
		return
	}

	if tc.taskConfig.Task.MustHaveResults && !tc.taskConfig.HasTestResults {
		tc.logger.Task().Info("Test results are missing and this task must have attached test results. Overall task status changed to FAILED.")
		detail.Type = evergreen.CommandTypeTest
		detail.Status = evergreen.TaskFailed
		detail.Description = evergreen.TaskDescriptionNoResults
		return
	}

	if tc.taskConfig.HasFailingTestResult {
		tc.logger.Task().Info("Test results contain at least one failure. Overall task status changed to FAILED.")
		detail.Type = evergreen.CommandTypeTest
		detail.Status = evergreen.TaskFailed
		detail.Description = evergreen.TaskDescriptionResultsFailed
	}
}

func (a *Agent) killProcs(ctx context.Context, tc *taskContext, ignoreTaskGroupCheck bool, reason string) error {
	logger := grip.NewJournaler("killProcs")
	if tc.logger != nil && !tc.logger.Closed() {
		logger = tc.logger.Execution()
	}

	if !a.shouldKill(tc, ignoreTaskGroupCheck) {
		return nil
	}

	logger.Infof("Cleaning up task because %s", reason)

	catcher := grip.NewBasicCatcher()
	if tc.task.ID != "" && tc.taskConfig != nil && tc.taskConfig.Distro != nil {
		logger.Infof("Cleaning up processes for task: '%s'.", tc.task.ID)
		if err := agentutil.KillSpawnedProcs(ctx, tc.task.ID, tc.taskConfig.WorkDir, tc.taskConfig.Distro.ExecUser, logger); err != nil {
			catcher.Wrap(err, "cleaning up spawned processes")
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
	if a.opts.Mode != globals.PodMode {
		logger.Info("Cleaning up Docker artifacts.")
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, globals.DockerTimeout)
		defer cancel()
		if err := docker.Cleanup(ctx, logger); err != nil {
			catcher.Wrap(err, "cleaning up Docker artifacts")
			logger.Critical(errors.Wrap(err, "cleaning up Docker artifacts"))
		}
		logger.Info("Cleaned up Docker artifacts.")
	}

	return catcher.Resolve()
}

// clearGlobalFiles cleans up certain files that were created in the home directory, including
// the global git config file, git credentials file, and netrc file.
func (a *Agent) clearGlobalFiles(tc *taskContext) {
	logger := grip.GetDefaultJournaler()
	if tc.logger != nil && !tc.logger.Closed() {
		logger = tc.logger.Execution()
	}

	for _, file := range globalFilesToCleanup {
		logger.Infof("Clearing '%s'.", file)
		globalPath := filepath.Join(a.opts.HomeDirectory, file)
		if _, err := os.Stat(globalPath); os.IsNotExist(err) {
			logger.Infof("Global '%s' file does not exist.", file)
		} else if err := os.Remove(globalPath); err != nil {
			logger.Error(errors.Wrapf(err, "removing global '%s' file", file))
		} else {
			logger.Infof("Cleared '%s'.", file)
		}
	}
}

func (a *Agent) shouldKill(tc *taskContext, ignoreTaskGroupCheck bool) bool {
	// Never kill if the agent is not configured to clean up.
	if !a.opts.Cleanup {
		return false
	}
	// Kill if the task is not in a task group.
	if tc.taskConfig != nil && tc.taskConfig.TaskGroup == nil {
		return true
	}
	// This is a task group, kill if ignoreTaskGroupCheck is true
	if ignoreTaskGroupCheck {
		return true
	}
	// This is a task group, kill if not sharing processes between tasks in the
	// task group.
	return tc.taskConfig != nil && !tc.taskConfig.TaskGroup.ShareProcs
}

// logPanic logs a panic to the task log and returns the panic error, along with
// the original error (if any). If there was no panic error, this is a no-op.
func (a *Agent) logPanic(tc *taskContext, pErr, originalErr error, op string) error {
	if pErr == nil {
		return nil
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(originalErr)
	catcher.Add(pErr)
	logMsg := a.opts.AddLoggableInfo(message.Fields{
		"message":   "programmatic error: Evergreen agent hit panic",
		"operation": op,
	})
	if tc.logger != nil && !tc.logger.Closed() {
		tc.logger.Task().Error(logMsg)
	}
	logMsg["task_id"] = tc.task.ID
	if tc.taskConfig != nil {
		logMsg["task_execution"] = tc.taskConfig.Task.Execution
	}
	grip.Alert(message.WrapError(errors.WithStack(pErr), logMsg))

	return catcher.Resolve()
}
