package agent

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Agent manages the data necessary to run tasks on a host.
type Agent struct {
	comm client.Communicator
	opts Options
}

// Options contains startup options for the Agent.
type Options struct {
	HostID              string
	HostSecret          string
	StatusPort          int
	LogPrefix           string
	HeartbeatInterval   time.Duration
	IdleTimeoutInterval time.Duration
	AgentSleepInterval  time.Duration
}

type taskContext struct {
	currentCommand command.Command
	logger         client.LoggerProducer
	statsCollector *StatsCollector
	task           client.TaskData
	taskConfig     *model.TaskConfig
	taskDirectory  string
}

// New creates a new Agent with some Options and a client.Communicator. Call the
// Agent's Start method to begin listening for tasks to run.
func New(opts Options, comm client.Communicator) (*Agent, error) {
	comm.SetHostID(opts.HostID)
	comm.SetHostSecret(opts.HostSecret)
	agent := &Agent{
		opts: opts,
		comm: comm,
	}

	sender, err := getSender(agent.opts.LogPrefix, "init")
	if err != nil {
		return nil, errors.Wrap(err, "problem configuring logger")
	}
	grip.SetSender(sender)
	grip.SetName("evergreen.agent")
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)

	return agent, nil
}

// Start starts the agent loop. The agent polls the API server for new tasks
// at interval agentSleepInterval and runs them.
func (a *Agent) Start(ctx context.Context) error {
	a.startStatusServer(a.opts.StatusPort)
	err := errors.Wrap(a.loop(ctx), "error in agent loop, exiting")
	grip.Emergency(err)
	return err
}

func (a *Agent) loop(ctx context.Context) error {
	agentSleepInterval := defaultAgentSleepInterval
	if a.opts.AgentSleepInterval != 0 {
		agentSleepInterval = a.opts.AgentSleepInterval
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			grip.Info("agent loop canceled")
			return nil
		case <-timer.C:
			nextTask, err := a.comm.GetNextTask(ctx)
			if err != nil {
				err = errors.Wrap(err, "error getting next task")
				grip.Error(err)
				return err
			}
			if nextTask.TaskId != "" {
				if nextTask.TaskSecret == "" {
					err = errors.New("task response missing secret")
					grip.Error(err)
					return err
				}
				tc := &taskContext{
					task: client.TaskData{
						ID:     nextTask.TaskId,
						Secret: nextTask.TaskSecret,
					},
				}
				if err := a.resetLogging(ctx, tc); err != nil {
					return err
				}
				if err := a.runTask(ctx, tc); err != nil {
					return err
				}
				timer.Reset(0)
				continue
			}
			grip.Debugf("Agent sleeping %s", agentSleepInterval)
			timer.Reset(agentSleepInterval)
		}
	}
}

func (a *Agent) resetLogging(ctx context.Context, tc *taskContext) error {
	tc.logger = a.comm.GetLoggerProducer(context.TODO(), tc.task)

	sender, err := getSender(a.opts.LogPrefix, tc.task.ID)
	if err != nil {
		err = errors.Wrap(err, "problem getting sender")
		grip.Error(err)
		return err
	}
	err = grip.SetSender(sender)
	if err != nil {
		err = errors.Wrap(err, "problem setting sender")
		grip.Error(err)
		return err
	}
	return nil
}

func (a *Agent) runTask(ctx context.Context, tc *taskContext) error {
	ctx, cancel := context.WithCancel(ctx)
	grip.Info(message.Fields{
		"message":     "running task",
		"task_id":     tc.task.ID,
		"task_secret": tc.task.Secret,
	})

	metrics := &metricsCollector{
		comm:     a.comm,
		taskData: tc.task,
	}

	if err := metrics.start(ctx); err != nil {
		return errors.Wrap(err, "problem setting up metrics collection")
	}

	// Defers are LIFO. We cancel all agent task threads, then any procs started by the agent, then remove the task directory.
	defer a.removeTaskDirectory(tc)
	defer a.killProcs(tc)
	defer cancel()

	// If the heartbeat aborts the task immediately, we should report that
	// the task failed during initial task setup, even though checkIn, which
	// will set tc.currentCommand in startTask has not yet run.
	factory, ok := command.GetCommandFactory("setup.initial")
	if !ok {
		return errors.New("problem during configuring initial state")
	}
	tc.currentCommand = factory()

	heartbeat := make(chan string)
	go a.startHeartbeat(ctx, tc, heartbeat)

	idleTimeout := make(chan struct{})
	resetIdleTimeout := make(chan time.Duration)
	go a.startIdleTimeoutWatch(ctx, tc, idleTimeout, resetIdleTimeout)

	complete := make(chan string)
	execTimeout := make(chan struct{})
	go a.startTask(ctx, tc, complete, execTimeout, resetIdleTimeout)

	status, timeout := a.wait(ctx, tc, heartbeat, idleTimeout, complete, execTimeout)

	resp, err := a.finishTask(ctx, tc, status, timeout)
	if err != nil {
		return errors.Wrap(err, "exiting due to error marking task complete")
	}
	if resp == nil {
		grip.Error("response was nil, indicating a 409 or an empty response from the API server")
		return nil
	}
	if resp.ShouldExit {
		return errors.New("task response indicates that agent should exit")
	}
	return nil
}

func (a *Agent) wait(ctx context.Context, tc *taskContext, heartbeat chan string, idleTimeout chan struct{}, complete chan string, execTimeout chan struct{}) (string, bool) {
	status := evergreen.TaskFailed
	timeout := false
	select {
	case status = <-complete:
		grip.Infof("task complete: %s", tc.task.ID)
	case <-execTimeout:
		grip.Infof("recevied signal from execTimeout channel: %s", tc.task.ID)
	case status = <-heartbeat:
		grip.Infof("received signal from heartbeat channel: %s", tc.task.ID)
	case <-idleTimeout:
		grip.Infof("received signal on idleTimeout channel: %s", tc.task.ID)
		timeout = true
		if tc.taskConfig.Project.Timeout != nil {
			tc.logger.Task().Info("Running task-timeout commands.")
			start := time.Now()
			err := a.runCommands(ctx, tc, tc.taskConfig.Project.Timeout.List(), false, nil)
			if err != nil {
				tc.logger.Execution().Errorf("Error running task-timeout command: %v", err)
			}
			tc.logger.Task().Infof("Finished running task-timeout commands in %v.", time.Since(start).String())
		}
	case <-ctx.Done():
		grip.Infof("task canceled: %s", tc.task.ID)
	}
	return status, timeout
}

// finishTask sends the returned TaskEndResponse and error
func (a *Agent) finishTask(ctx context.Context, tc *taskContext, status string, timeout bool) (*apimodels.EndTaskResponse, error) {
	detail := a.endTaskResponse(tc, status, timeout)
	switch detail.Status {
	case evergreen.TaskSucceeded:
		tc.logger.Task().Info("Task completed - SUCCESS.")
		grip.Info("Running post task commands")
		a.runPostTaskCommands(ctx, tc)
		grip.Info("Finished running post task commands")
	case evergreen.TaskFailed:
		tc.logger.Task().Info("Task completed - FAILURE.")
		grip.Info("Running post task commands")
		a.runPostTaskCommands(ctx, tc)
		grip.Info("Finished running post task commands")
	case evergreen.TaskUndispatched:
		tc.logger.Task().Info("Task completed - ABORTED.")
	case evergreen.TaskConflict:
		tc.logger.Task().Error("Task completed - CANCELED.")
		// If we receive a 409, return control to the loop (ask for a new task)
		return nil, nil
	}

	tc.logger.Execution().Infof("Sending final status as: %v", detail.Status)
	err := tc.logger.Close()
	grip.Errorf("Error closing logger: %v", err)
	grip.Infof("Sending final status as: %v", detail.Status)
	resp, err := a.comm.EndTask(ctx, detail, tc.task)
	grip.Infof("Sent final status as: %v", detail.Status)
	if err != nil {
		return nil, errors.Wrap(err, "problem marking task complete")
	}

	return resp, nil
}

func (a *Agent) getCurrentCommandType(tc *taskContext) string {
	if tc.taskConfig != nil {
		return tc.currentCommand.Type()
	}
	return model.DefaultCommandType
}

func (a *Agent) endTaskResponse(tc *taskContext, status string, timeout bool) *apimodels.TaskEndDetail {
	return &apimodels.TaskEndDetail{
		Description: tc.currentCommand.DisplayName(),
		Type:        tc.currentCommand.Type(),
		TimedOut:    timeout,
		Status:      status,
	}
}

func (a *Agent) runPostTaskCommands(ctx context.Context, tc *taskContext) {
	if tc.taskConfig != nil && tc.taskConfig.Project.Post != nil {
		a.killProcs(tc)
		tc.logger.Task().Info("Running post-task commands.")
		start := time.Now()
		var cancel context.CancelFunc
		ctx, cancel = a.withCallbackTimeout(ctx, tc)
		defer cancel()
		err := a.runCommands(ctx, tc, tc.taskConfig.Project.Post.List(), false, nil)
		if err != nil {
			tc.logger.Execution().Errorf("Error running post-task command: %v", err)
		} else {
			tc.logger.Task().Infof("Finished running post-task commands in %v.", time.Since(start).String())
		}
		a.killProcs(tc)
	}
}

func (a *Agent) killProcs(tc *taskContext) {
	grip.Infof("cleaning up processes for task: %s", tc.task.ID)

	if tc.task.ID != "" {
		if err := subprocess.KillSpawnedProcs(tc.task.ID, tc.logger.Task()); err != nil {
			msg := fmt.Sprintf("Error cleaning up spawned processes (agent-exit): %v", err)
			grip.Critical(msg)
		}
	}
	grip.Infof("processes cleaned up for task %s", tc.task.ID)
}
