package proto

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/mongodb/grip"
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
	APIURL            string
	HostID            string
	HostSecret        string
	StatusPort        int
	LogPrefix         string
	HeartbeatInterval time.Duration
}

type taskContext struct {
	currentCommand model.PluginCommandConf
	logger         client.LoggerProducer
	statsCollector *StatsCollector
	task           client.TaskData
	taskConfig     *model.TaskConfig
	taskDirectory  string
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
	a.startStatusServer(a.opts.StatusPort)
	defer grip.GetSender().Close()
	err := errors.Wrap(a.loop(ctx), "error in agent loop, exiting")
	grip.Emergency(err)
	return err
}

func (a *Agent) loop(ctx context.Context) error {
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
				if err := a.startNextTask(ctx, tc); err != nil {
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

func (a *Agent) startNextTask(ctx context.Context, tc *taskContext) error {
	ctx, cancel := context.WithCancel(ctx)
	tc.logger = a.comm.GetLoggerProducer(tc.task)

	// TODO: This is commenting out pending investigation of races caused by the metrics collector (EVG-1948).
	// metrics := &metricsCollector{
	//	comm:     a.comm,
	//	taskData: tc.task,
	// }

	// if err := metrics.start(ctx); err != nil {
	//	return errors.Wrap(err, "problem setting up metrics collection")
	// }

	sender, err := getSender(a.opts.LogPrefix, tc.task.ID)
	if err != nil {
		err = errors.Wrap(err, "problem setting up logging")
		grip.Error(err)
		return err
	}
	existingSender := grip.GetSender()
	grip.SetSender(sender)
	existingSender.Close()

	grip.Info(message.Fields{
		"message":     "running task",
		"task_id":     tc.task.ID,
		"task_secret": tc.task.Secret,
	})

	// Defers are LIFO. We cancel all agent task threads, then any procs started by the agent, then remove the task directory.
	defer a.removeTaskDirectory(tc)
	defer a.killProcs(tc)
	defer cancel()

	heartbeat := make(chan struct{})
	go a.startHeartbeat(ctx, tc, heartbeat)

	idleTimeout := make(chan struct{})
	resetIdleTimeout := make(chan time.Duration)
	go a.startIdleTimeoutWatch(ctx, tc, idleTimeout, resetIdleTimeout)

	complete := make(chan string)
	execTimeout := make(chan struct{})
	go a.runTask(ctx, tc, complete, execTimeout, resetIdleTimeout)

	status := evergreen.TaskFailed
	timeout := false
	select {
	case status = <-complete:
		grip.Infof("task complete: %s", tc.task.ID)
	case <-execTimeout:
		grip.Infof("recevied signal from execTimeout channel: %s", tc.task.ID)
	case <-heartbeat:
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

	resp, err := a.finishTask(ctx, tc, status, timeout)
	if err != nil {
		return errors.Wrap(err, "exiting due to error marking task complete")
	}
	if resp == nil {
		return errors.New("received nil response from API server")
	}
	if resp.ShouldExit {
		return errors.New("task response indicates that agent should exit")
	}
	return nil
}

// finishTask sends the returned TaskEndResponse and error
func (a *Agent) finishTask(ctx context.Context, tc *taskContext, status string, timeout bool) (*apimodels.EndTaskResponse, error) {
	detail := a.endTaskResponse(tc, status, timeout)
	a.runPostTaskCommands(ctx, tc)

	tc.logger.Execution().Infof("Sending final status as: %v", detail.Status)
	tc.logger.Close()
	resp, err := a.comm.EndTask(ctx, detail, tc.task)
	if err != nil {
		return nil, errors.Wrap(err, "problem marking task complete")
	}

	return resp, nil
}

func (a *Agent) endTaskResponse(tc *taskContext, status string, timeout bool) *apimodels.TaskEndDetail {
	detail := &apimodels.TaskEndDetail{
		Description: tc.currentCommand.GetDisplayName(),
		TimedOut:    timeout,
	}
	if tc.taskConfig != nil {
		detail.Type = tc.currentCommand.GetType(tc.taskConfig.Project)
	}

	if status == evergreen.TaskSucceeded {
		detail.Status = evergreen.TaskSucceeded
		tc.logger.Task().Info("Task completed - SUCCESS.")
	} else {
		detail.Status = evergreen.TaskFailed
		tc.logger.Task().Info("Task completed - FAILURE.")
	}
	return detail
}

func (a *Agent) runPostTaskCommands(ctx context.Context, tc *taskContext) {
	if tc.taskConfig != nil && tc.taskConfig.Project.Post != nil {
		a.killProcs(tc)
		tc.logger.Task().Info("Running post-task commands.")
		start := time.Now()
		ctx, cancel := a.withCallbackTimeout(ctx, tc)
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
