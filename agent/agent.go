package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// Agent manages the data necessary to run tasks on a host.
type Agent struct {
	comm client.Communicator
	opts Options
}

// Options contains startup options for the Agent.
type Options struct {
	HostID             string
	HostSecret         string
	StatusPort         int
	LogPrefix          string
	LogkeeperURL       string
	S3BaseURL          string
	WorkingDirectory   string
	HeartbeatInterval  time.Duration
	AgentSleepInterval time.Duration
	Cleanup            bool
	S3Opts             pail.S3Options
}

type taskContext struct {
	currentCommand command.Command
	expansions     util.Expansions
	logger         client.LoggerProducer
	logs           *apimodels.TaskLogs
	statsCollector *StatsCollector
	task           client.TaskData
	taskGroup      string
	runGroupSetup  bool
	taskConfig     *model.TaskConfig
	taskDirectory  string
	logDirectories []string
	timeout        time.Duration
	timedOut       bool
	project        *model.Project
	taskModel      *task.Task
	version        *model.Version
	sync.RWMutex
}

// New creates a new Agent with some Options and a client.Communicator. Call the
// Agent's Start method to begin listening for tasks to run.
func New(opts Options, comm client.Communicator) *Agent {
	comm.SetHostID(opts.HostID)
	comm.SetHostSecret(opts.HostSecret)
	agent := &Agent{
		opts: opts,
		comm: comm,
	}

	return agent
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
	agentSleepInterval := defaultAgentSleepInterval
	if a.opts.AgentSleepInterval != 0 {
		agentSleepInterval = a.opts.AgentSleepInterval
	}

	// we want to have separate context trees for tasks and
	// loggers, so that when a task is canceled by a context, it
	// can log its clean up.
	var (
		lgrCtx        context.Context
		cancel        context.CancelFunc
		jitteredSleep time.Duration

		exit bool
	)
	lgrCtx, cancel = context.WithCancel(ctx)
	defer cancel()

	timer := time.NewTimer(0)
	defer timer.Stop()

	tc := &taskContext{}
	needPostGroup := false

LOOP:
	for {
		select {
		case <-ctx.Done():
			grip.Info("agent loop canceled")
			return nil
		case <-timer.C:
			nextTask, err := a.comm.GetNextTask(ctx, &apimodels.GetNextTaskDetails{TaskGroup: tc.taskGroup})
			if err != nil {
				// task secret doesn't match, get another task
				if errors.Cause(err) == client.HTTPConflictError {
					timer.Reset(0)
					continue LOOP
				}
				return errors.Wrap(err, "error getting next task")
			}
			if nextTask.ShouldExit {
				grip.Notice("Next task response indicates agent should exit")
				return nil
			}
			if nextTask.TaskId != "" {
				if nextTask.TaskSecret == "" {
					grip.Critical(message.WrapError(err, message.Fields{
						"message": "task response missing secret",
						"task":    tc.task.ID,
					}))
					timer.Reset(0)
					continue LOOP
				}
				tc, exit = a.prepareNextTask(ctx, nextTask, tc)
				if exit {
					// Query for next task, this time with an empty task group,
					// to get a ShouldExit from the API, and set NeedsNewAgent.
					timer.Reset(0)
					continue LOOP
				}
				if err := a.fetchProjectConfig(ctx, tc); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message": "error fetching project config; will attempt at a later point",
						"task":    tc.task.ID,
					}))
				}
				if err := a.resetLogging(lgrCtx, tc); err != nil {
					grip.Critical(message.WrapError(err, message.Fields{
						"message": "error setting up logger",
						"task":    tc.task.ID,
					}))
					timer.Reset(0)
					continue LOOP
				}
				tskCtx, tskCancel := context.WithCancel(ctx)
				defer tskCancel()
				shouldExit, err := a.runTask(tskCtx, tskCancel, tc)
				if err != nil {
					grip.Critical(message.WrapError(err, message.Fields{
						"message": "error running task",
						"task":    tc.task.ID,
					}))
					timer.Reset(0)
					continue LOOP
				}
				if shouldExit {
					return nil
				}
				needPostGroup = true
				timer.Reset(0)
				continue LOOP
			} else if needPostGroup {
				a.runPostGroupCommands(ctx, tc)
				needPostGroup = false
				// Running the post group commands implies exiting the group, so
				// destroy prior task information.
				tc = &taskContext{}
			}
			jitteredSleep = util.JitterInterval(agentSleepInterval)
			grip.Debugf("Agent sleeping %s", jitteredSleep)
			timer.Reset(jitteredSleep)
		}
	}
}

func (a *Agent) prepareNextTask(ctx context.Context, nextTask *apimodels.NextTaskResponse, tc *taskContext) (*taskContext, bool) {
	setupGroup := false
	taskDirectory := tc.taskDirectory
	if nextTaskHasDifferentTaskGroupOrBuild(nextTask, tc) {
		setupGroup = true
		taskDirectory = ""
		a.runPostGroupCommands(ctx, tc)
		if nextTask.NewAgent {
			return &taskContext{}, true
		}
	}
	return &taskContext{
		task: client.TaskData{
			ID:     nextTask.TaskId,
			Secret: nextTask.TaskSecret,
		},
		taskGroup:     nextTask.TaskGroup,
		runGroupSetup: setupGroup,
		taskDirectory: taskDirectory,
	}, false
}

func nextTaskHasDifferentTaskGroupOrBuild(nextTask *apimodels.NextTaskResponse, tc *taskContext) bool {
	if tc.taskConfig == nil ||
		nextTask.TaskGroup == "" ||
		nextTask.TaskGroup != tc.taskGroup ||
		// TODO The Version case is redundant, and can be removed after agents roll over after a deploy.
		nextTask.Version != tc.taskConfig.Task.Version ||
		nextTask.Build != tc.taskConfig.Task.BuildId {
		return true
	}
	return false
}

func (a *Agent) fetchProjectConfig(ctx context.Context, tc *taskContext) error {
	v, err := a.comm.GetVersion(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "error getting version")
	}
	project := &model.Project{}
	err = model.LoadProjectInto([]byte(v.Config), v.Identifier, project)
	if err != nil {
		return errors.Wrapf(err, "error reading project config")
	}
	taskModel, err := a.comm.GetTask(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "error getting task")
	}
	exp, err := a.comm.GetExpansions(ctx, tc.task)
	if err != nil {
		return errors.Wrap(err, "error getting expansions")
	}
	tc.version = v
	tc.taskModel = taskModel
	tc.project = project
	tc.expansions = exp
	return nil
}

func (a *Agent) resetLogging(ctx context.Context, tc *taskContext) error {
	grip.Error(os.RemoveAll(filepath.Join(a.opts.WorkingDirectory, taskLogDirectory)))
	if tc.project != nil && tc.project.Loggers != nil {
		tc.logger = a.makeLoggerProducer(ctx, tc, tc.project.Loggers, "")
	} else {
		tc.logger = a.comm.GetLoggerProducer(ctx, tc.task, nil)
	}

	sender, err := GetSender(ctx, a.opts.LogPrefix, tc.task.ID)
	if err != nil {
		return errors.Wrap(err, "problem getting sender")
	}
	err = grip.SetSender(sender)
	if err != nil {
		return errors.Wrap(err, "problem setting sender")
	}
	return nil
}

// runTask returns true if the agent should exit, and separate an error if relevant
func (a *Agent) runTask(ctx context.Context, cancel context.CancelFunc, tc *taskContext) (bool, error) {
	var err error
	defer func() { err = recovery.HandlePanicWithError(recover(), err, "running task") }()

	grip.Info(message.Fields{
		"message":     "running task",
		"task_id":     tc.task.ID,
		"task_secret": tc.task.Secret,
	})

	// Defers are LIFO. We cancel all agent task threads, then any procs started by the agent, then remove the task directory.
	defer a.killProcs(tc, false)
	defer cancel()

	// If the heartbeat aborts the task immediately, we should report that
	// the task failed during initial task setup.
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		return false, errors.New("problem during configuring initial state")
	}
	tc.setCurrentCommand(factory())

	heartbeat := make(chan string, 1)
	go a.startHeartbeat(ctx, cancel, tc, heartbeat)

	innerCtx, innerCancel := context.WithCancel(ctx)

	go a.startIdleTimeoutWatch(ctx, tc, innerCancel)

	complete := make(chan string)
	go a.startTask(innerCtx, tc, complete)

	status := a.wait(ctx, innerCtx, tc, heartbeat, complete)
	var resp *apimodels.EndTaskResponse
	resp, err = a.finishTask(ctx, tc, status)
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

	return status
}

func (a *Agent) runTaskTimeoutCommands(ctx context.Context, tc *taskContext) {
	tc.logger.Task().Info("Running task-timeout commands.")
	start := time.Now()
	var cancel context.CancelFunc
	ctx, cancel = a.withCallbackTimeout(ctx, tc)
	defer cancel()

	taskGroup, err := model.GetTaskGroup(tc.taskGroup, tc.taskConfig)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for task timeout commands"))
		return
	}
	if taskGroup.Timeout != nil {

		err := a.runCommands(ctx, tc, taskGroup.Timeout.List(), runCommandsOptions{})
		tc.logger.Execution().ErrorWhenf(err != nil, "Error running timeout command: %v", err)
		tc.logger.Task().InfoWhenf(err == nil, "Finished running timeout commands in %v.", time.Since(start).String())
	}
}

// finishTask sends the returned EndTaskResponse and error
func (a *Agent) finishTask(ctx context.Context, tc *taskContext, status string) (*apimodels.EndTaskResponse, error) {
	err := a.uploadToS3(ctx, tc)
	tc.logger.Execution().Error(errors.Wrap(err, "error uploading log files"))

	detail := a.endTaskResponse(tc, status)
	switch detail.Status {
	case evergreen.TaskSucceeded:
		tc.logger.Task().Info("Task completed - SUCCESS.")
		a.runPostTaskCommands(ctx, tc)
	case evergreen.TaskFailed:
		tc.logger.Task().Info("Task completed - FAILURE.")
		a.runPostTaskCommands(ctx, tc)
	case evergreen.TaskUndispatched:
		tc.logger.Task().Info("Task completed - ABORTED.")
	case evergreen.TaskConflict:
		tc.logger.Task().Error("Task completed - CANCELED.")
		// If we receive a 409, return control to the loop (ask for a new task)
		return nil, nil
	}

	tc.logger.Execution().Infof("Sending final status as: %v", detail.Status)
	if err = tc.logger.Close(); err != nil {
		grip.Errorf("Error closing logger: %v", err)
	}
	grip.Infof("Sending final status as: %v", detail.Status)
	resp, err := a.comm.EndTask(ctx, detail, tc.task)
	grip.Infof("Sent final status as: %v", detail.Status)
	if err != nil {
		return nil, errors.Wrap(err, "problem marking task complete")
	}

	return resp, nil
}

func (a *Agent) endTaskResponse(tc *taskContext, status string) *apimodels.TaskEndDetail {
	return &apimodels.TaskEndDetail{
		Description: tc.getCurrentCommand().DisplayName(),
		Type:        tc.getCurrentCommand().Type(),
		TimedOut:    tc.hadTimedOut(),
		Status:      status,
		Logs:        tc.logs,
	}
}

func (a *Agent) runPostTaskCommands(ctx context.Context, tc *taskContext) {
	start := time.Now()
	a.killProcs(tc, false)
	defer a.killProcs(tc, false)
	tc.logger.Task().Info("Running post-task commands.")
	var cancel context.CancelFunc
	ctx, cancel = a.withCallbackTimeout(ctx, tc)
	defer cancel()
	taskConfig := tc.getTaskConfig()
	taskGroup, err := model.GetTaskGroup(tc.taskGroup, taskConfig)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for post-task commands"))
		return
	}
	if taskGroup.TeardownTask != nil {
		err := a.runCommands(ctx, tc, taskGroup.TeardownTask.List(), runCommandsOptions{})
		tc.logger.Task().ErrorWhenf(err != nil, "Error running post-task command: %v", err)
		tc.logger.Task().InfoWhenf(err == nil, "Finished running post-task commands in %v.", time.Since(start).String())
	}
}

func (a *Agent) runPostGroupCommands(ctx context.Context, tc *taskContext) {
	defer a.removeTaskDirectory(tc)
	defer a.killProcs(tc, true)
	if tc.taskConfig == nil {
		return
	}
	if err := a.resetLogging(ctx, tc); err != nil {
		grip.Error(errors.Wrap(err, "error resetting logging"))
		return
	}
	defer tc.logger.Close()
	taskGroup, err := model.GetTaskGroup(tc.taskGroup, tc.taskConfig)
	if err != nil {
		tc.logger.Execution().Error(errors.Wrap(err, "error fetching task group for post-group commands"))
		return
	}
	if taskGroup.TeardownGroup != nil {
		grip.Info("Running post-group commands")
		a.killProcs(tc, true)
		var cancel context.CancelFunc
		ctx, cancel = a.withCallbackTimeout(ctx, tc)
		defer cancel()
		err := a.runCommands(ctx, tc, taskGroup.TeardownGroup.List(), runCommandsOptions{})
		grip.ErrorWhenf(err != nil, "Error running post-task command: %v", err)
		grip.InfoWhen(err == nil, "Finished running post-group commands")
	}
}

func (a *Agent) killProcs(tc *taskContext, ignoreTaskGroupCheck bool) {
	if a.shouldKill(tc, ignoreTaskGroupCheck) {
		grip.Infof("cleaning up processes for task: %s", tc.task.ID)

		if tc.task.ID != "" {
			if err := subprocess.KillSpawnedProcs(tc.task.ID, tc.logger.Task()); err != nil {
				msg := fmt.Sprintf("Error cleaning up spawned processes (agent-exit): %v", err)
				grip.Critical(msg)
			}
		}
		grip.Infof("processes cleaned up for task %s", tc.task.ID)
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
	taskGroup, err := model.GetTaskGroup(tc.taskGroup, tc.taskConfig)
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
