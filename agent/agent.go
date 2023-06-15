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
	LogkeeperURL           string
	WorkingDirectory       string
	HeartbeatInterval      time.Duration
	AgentSleepInterval     time.Duration
	MaxAgentSleepInterval  time.Duration
	Cleanup                bool
	SetupData              apimodels.AgentSetupData
	CloudProvider          string
	TraceCollectorEndpoint string
}

// Mode represents a mode that the agent will run in.
type Mode string

const (
	// HostMode indicates that the agent will run in a host.
	HostMode Mode = "host"
	// PodMode indicates that the agent will run in a pod's container.
	PodMode      Mode = "pod"
	MessageLimit      = 500

	taskTimeoutBlock = "task-timeout"
	preBlock         = "pre"
	postBlock        = "post"
	endTaskBlock     = "end-task"
	earlyTermBlock   = "early-termination"
)

type taskContext struct {
	currentCommand command.Command
	expansions     util.Expansions
	privateVars    map[string]bool
	logger         client.LoggerProducer
	jasper         jasper.Manager
	logs           *apimodels.TaskLogs
	statsCollector *StatsCollector
	task           client.TaskData
	taskGroup      string
	ranSetupGroup  bool
	taskConfig     *internal.TaskConfig
	taskDirectory  string
	timeout        timeoutInfo
	project        *model.Project
	taskModel      *task.Task
	oomTracker     jasper.OOMTracker
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

	setupData, err := comm.GetAgentSetupData(ctx)
	grip.Alert(errors.Wrap(err, "getting agent setup data"))
	if setupData != nil {
		opts.SetupData = *setupData
		opts.LogkeeperURL = setupData.LogkeeperURL
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
			grip.Error(errors.Wrap(tc.logger.Close(), "closing logger"))
		}
	}()

LOOP:
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
				// task secret doesn't match, get another task
				if errors.Cause(err) == client.HTTPConflictError {
					timer.Reset(0)
					agentSleepInterval = minAgentSleepInterval
					continue LOOP
				}
				return errors.Wrap(err, "getting next task")
			}
			if nextTask.ShouldExit {
				grip.Notice("Next task response indicates agent should exit.")
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
					grip.Error(errors.Wrap(prevLogger.Close(), "closing the previous logger producer"))
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
			grip.Debugf("Agent sleeping %s.", jitteredSleep)
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

	sender, err := a.GetSender(ctx, a.opts.LogPrefix)
	grip.Error(errors.Wrap(err, "getting sender"))
	grip.Error(errors.Wrap(grip.SetSender(sender), "setting sender"))

	return nil
}

// runTask runs a task. It returns true if the agent should exit.
func (a *Agent) runTask(ctx context.Context, tc *taskContext) (shouldExit bool, err error) {
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

	// If the heartbeat aborts the task immediately, we should report that
	// the task failed during initial task setup.
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		return false, errors.New("setup.initial command is not registered")
	}
	tc.setCurrentCommand(factory())

	var taskConfig *internal.TaskConfig
	taskConfig, err = a.makeTaskConfig(ctx, tc)
	if err != nil {
		err = errors.Wrap(err, "making task config")
		grip.Error(err)
		grip.Infof("Task complete: '%s'.", tc.task.ID)
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleTaskResponse(tskCtx, tc, evergreen.TaskFailed, err.Error())
	}
	tc.setTaskConfig(taskConfig)

	if err = a.startLogging(ctx, tc); err != nil {
		err = errors.Wrap(err, "setting up logger producer")
		grip.Error(err)
		grip.Infof("Task complete: '%s'.", tc.task.ID)
		tc.logger = client.NewSingleChannelLogHarness("agent.error", a.defaultLogger)
		return a.handleTaskResponse(tskCtx, tc, evergreen.TaskFailed, err.Error())
	}

	if !tc.ranSetupGroup {
		tc.taskDirectory, err = a.createTaskDirectory(tc)
		if err != nil {
			err = errors.Wrap(err, "creating task directory")
			grip.Error(err)
			grip.Infof("Task complete: '%s'.", tc.task.ID)
			tc.logger.Execution().Error(errors.Wrap(err, "creating task directory"))
			return a.handleTaskResponse(tskCtx, tc, evergreen.TaskFailed, err.Error())
		}
	}
	tc.taskConfig.WorkDir = tc.taskDirectory
	tc.taskConfig.Expansions.Put("workdir", tc.taskConfig.WorkDir)

	grip.Info(message.Fields{
		"message":     "running task",
		"task_id":     tc.task.ID,
		"task_secret": tc.task.Secret,
	})

	defer a.killProcs(ctx, tc, false)
	defer tskCancel()

	tskCtx = utility.ContextWithAttributes(tskCtx, tc.taskConfig.TaskAttributes())
	tskCtx, span := a.tracer.Start(tskCtx, fmt.Sprintf("task: '%s'", tc.taskConfig.Task.DisplayName))
	defer span.End()

	shutdown, err := a.startMetrics(tskCtx, tc.taskConfig)
	grip.Error(errors.Wrap(err, "starting metrics collection"))
	if shutdown != nil {
		defer shutdown(ctx)
	}

	defer func() {
		tc.logger.Execution().Error(errors.Wrap(a.uploadTraces(tskCtx, tc.taskConfig.WorkDir), "uploading traces"))
	}()

	innerCtx, innerCancel := context.WithCancel(tskCtx)

	// Pass in idle timeout context to heartbeat to enforce the idle timeout.
	// Pass in the task context canceller to heartbeat because it's responsible for aborting the task.
	heartbeat := make(chan string, 1)
	go a.startHeartbeat(innerCtx, tskCancel, tc, heartbeat)
	go a.startIdleTimeoutWatch(tskCtx, tc, innerCancel)

	complete := make(chan string, 2)
	go a.startTask(innerCtx, tc, complete)

	return a.handleTaskResponse(tskCtx, tc, a.wait(tskCtx, innerCtx, tc, heartbeat, complete), "")
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

func (a *Agent) wait(ctx, taskCtx context.Context, tc *taskContext, heartbeat chan string, complete chan string) string {
	status := evergreen.TaskFailed
	select {
	case <-taskCtx.Done():
		grip.Infof("Task canceled: '%s'.", tc.task.ID)
	case status = <-complete:
		grip.Infof("Task complete: '%s'.", tc.task.ID)
	case status = <-heartbeat:
		grip.Infof("Received signal from heartbeat channel for task: '%s'.", tc.task.ID)
	}

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
		tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for task timeout commands"))
		return
	}
	if taskGroup.Timeout != nil {
		err := a.runCommands(ctx, tc, taskGroup.Timeout.List(), runCommandsOptions{}, taskTimeoutBlock)
		tc.logger.Execution().Error(errors.Wrap(err, "running timeout commands"))
		tc.logger.Task().Infof("Finished running timeout commands in %s.", time.Since(start))
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
			tc.logger.Task().Error(errors.Wrap(err, "running post task commands"))
		}
		a.runEndTaskSync(ctx, tc, detail)
	case evergreen.TaskUndispatched:
		tc.logger.Task().Info("Task completed - ABORTED.")
	case client.TaskConflict:
		tc.logger.Task().Error("Task completed - CANCELED.")
		// If we receive a 409, return control to the loop (ask for a new task)
		return nil, nil
	case evergreen.TaskSystemFailed:
		// This is a special status indicating that the agent failed for reasons
		// outside of a task's control (e.g. due to a panic). Therefore, it
		// intentionally does not run the post task logic, because the task is
		// likely in an unrecoverable state and should just give up. Note that
		// this is a distinct case from a task failing and intentionally setting
		// its failure type to system failed, as that is within a task's
		// control.
		tc.logger.Task().Error("Task encountered unexpected task lifecycle system failure.")
	default:
		tc.logger.Task().Errorf("Programmer error: invalid task status '%s'.", detail.Status)
	}

	a.killProcs(ctx, tc, false)

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

func (a *Agent) endTaskResponse(tc *taskContext, status string, message string) *apimodels.TaskEndDetail {
	var description string
	var failureType string
	if a.endTaskResp != nil { // if the user indicated a task response, use this instead
		tc.logger.Task().Infof("Task status set with HTTP endpoint.")
		if !evergreen.IsValidTaskEndStatus(a.endTaskResp.Status) {
			tc.logger.Task().Errorf("'%s' is not a valid task status.", a.endTaskResp.Status)
			status = evergreen.TaskFailed
			failureType = evergreen.CommandTypeSystem
		} else {
			status = a.endTaskResp.Status
			if len(a.endTaskResp.Description) > MessageLimit {
				tc.logger.Task().Warningf("Description from endpoint is too long to set (%d character limit), defaulting to command display name.", MessageLimit)
			} else {
				description = a.endTaskResp.Description
			}

			if a.endTaskResp.Type != "" && !utility.StringSliceContains(evergreen.ValidCommandTypes, a.endTaskResp.Type) {
				tc.logger.Task().Warningf("'%s' is not a valid failure type, defaulting to command failure type.", a.endTaskResp.Type)
			} else {
				failureType = a.endTaskResp.Type
			}
		}
	}

	if tc.getCurrentCommand() != nil {
		if description == "" {
			description = tc.getCurrentCommand().DisplayName()
		}
		if failureType == "" {
			failureType = tc.getCurrentCommand().Type()
		}
	}
	detail := &apimodels.TaskEndDetail{
		Description:     description,
		Type:            failureType,
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
	ctx, span := a.tracer.Start(ctx, "post-task-commands")
	defer span.End()

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
		tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for post-task commands"))
		return nil
	}
	if taskGroup.TeardownTask != nil {
		opts.failPreAndPost = taskGroup.TeardownTaskCanFailTask
		err = a.runCommands(postCtx, tc, taskGroup.TeardownTask.List(), opts, postBlock)
		if err != nil {
			tc.logger.Task().Error(errors.Wrap(err, "running post-task commands"))
			if taskGroup.TeardownTaskCanFailTask {
				return err
			}
		}
		tc.logger.Task().Infof("Finished running post-task commands in %s.", time.Since(start))
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
			tc.logger.Execution().Error(errors.Wrap(err, "fetching task group for post-group commands"))
		}
		return
	}
	if taskGroup.TeardownGroup != nil {
		grip.Info("Running post-group commands.")
		a.killProcs(ctx, tc, true)
		var cancel context.CancelFunc
		ctx, cancel = a.withCallbackTimeout(ctx, tc)
		defer cancel()
		err := a.runCommands(ctx, tc, taskGroup.TeardownGroup.List(), runCommandsOptions{}, postBlock)
		grip.Error(errors.Wrap(err, "running post-group commands"))
		grip.Info("Finished running post-group commands.")
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

	if err := a.runCommands(syncCtx, tc, taskSyncCmds.List(), runCommandsOptions{}, endTaskBlock); err != nil {
		tc.logger.Task().Error(message.WrapError(err, message.Fields{
			"message":    "error running task sync",
			"total_time": time.Since(start).String(),
		}))
		return
	}
	tc.logger.Task().Infof("Finished running task sync in %s.", time.Since(start))
}

func (a *Agent) killProcs(ctx context.Context, tc *taskContext, ignoreTaskGroupCheck bool) {
	logger := grip.NewJournaler("killProcs")
	if tc.logger != nil && !tc.logger.Closed() {
		logger = tc.logger.Execution()
	}

	if a.shouldKill(tc, ignoreTaskGroupCheck) {
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

// logPanic logs and returns a panic error, along with the original error (if
// any). If there was no panic error, this is a no-op.
func (a *Agent) logPanic(logger client.LoggerProducer, pErr, originalErr error, op string) error {
	if pErr == nil {
		return nil
	}

	msg := message.Fields{
		"message":   "programmatic error: agent panicked",
		"operation": op,
		"stack":     message.NewStack(2, "").Raw(),
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(originalErr)
	catcher.Add(pErr)
	grip.Alert(message.WrapError(catcher.Resolve(), msg))
	if logger != nil && !logger.Closed() {
		logMsg := message.Fields{
			"message":   "programmatic error: Evergreen agent hit a runtime panic",
			"operation": op,
		}
		logger.Execution().Error(logMsg)
	}

	return catcher.Resolve()
}
