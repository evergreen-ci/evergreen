package agent

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/util"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Signal describes the various conditions under which the agent
// will complete execution of a task.
type Signal int64

// The FinalTaskFunc describes the expected return values for a given task run
// by the agent. The finishAndAwaitCleanup listens on a channel for this
// function and runs returns its values once it receives the function to run.
// This will typically be an HTTP call to the API server (to end the task).
type FinalTaskFunc func() (*apimodels.TaskEndResponse, error)

const APIVersion = 2

// Recognized agent signals.
const (
	// HeartbeatMaxFailed indicates that repeated attempts to send heartbeat to
	// the API server fails.
	HeartbeatMaxFailed Signal = iota
	// IncorrectSecret indicates that the secret for the task the agent is running
	// does not match the task secret held by API server.
	IncorrectSecret
	// AbortedByUser indicates a user decided to prematurely end the task.
	AbortedByUser
	// IdleTimeout indicates the task appears to be idle - e.g. no logs produced
	// for the duration indicated by DefaultIdleTimeout.
	IdleTimeout
	// CompletedSuccess indicates task successfully ran to completion and passed.
	CompletedSuccess
	// CompletedFailure indicates task successfully ran to completion but failed.
	CompletedFailure
)

const (
	// DefaultCmdTimeout specifies the duration after which agent sends
	// an IdleTimeout signal if a task's command does not run to completion.
	DefaultCmdTimeout = 2 * time.Hour
	// DefaultIdleTimeout specifies the duration after which agent sends an
	// IdleTimeout signal if a task produces no logs.
	DefaultIdleTimeout = 15 * time.Minute
	// DefaultHeartbeatInterval is interval after which agent sends a heartbeat
	// to API server.
	DefaultHeartbeatInterval = 30 * time.Second
	// DefaultStatsInterval is the interval after which agent sends system stats
	// to API server
	DefaultStatsInterval = 60 * time.Second
)

var (
	// InitialSetupTimeout indicates the time allowed for the agent to collect
	// relevant information - for running a task - from the API server.
	InitialSetupTimeout = 5 * time.Minute
	// InitialSetupCommand is a placeholder command for the period during which
	// the agent requests information for running a task
	InitialSetupCommand = model.PluginCommandConf{
		DisplayName: "initial task setup",
		Type:        model.SystemCommandType,
	}
)

// TerminateHandler is an interface which defines how the agent should respond
// to signals resulting in the end of the task (heartbeat fail, timeout, etc)
type TerminateHandler interface {
	HandleSignals(*Agent, chan FinalTaskFunc)
}

// ExecTracker exposes functions to update and get the current execution stage
// of the agent.
type ExecTracker interface {
	// Returns the current command being executed.
	CurrentCommand() *model.PluginCommandConf
	// Sets the current command being executed as well as a timeout for the command.
	CheckIn(command model.PluginCommandConf, timeout time.Duration)
}

// TaskCommunicator is an interface that handles the remote procedure calls
// between an agent and the remote server.
type TaskCommunicator interface {
	Start(pid string) error
	End(detail *apimodels.TaskEndDetail) (*apimodels.TaskEndResponse, error)
	GetTask() (*model.Task, error)
	GetProjectRef() (*model.ProjectRef, error)
	GetDistro() (*distro.Distro, error)
	GetProjectConfig() (*model.Project, error)
	Log([]model.LogMessage) error
	Heartbeat() (bool, error)
	FetchExpansionVars() (*apimodels.ExpansionVars, error)
	tryGet(path string) (*http.Response, error)
	tryPostJSON(path string, data interface{}) (*http.Response, error)
}

// SignalHandler is an implementation of TerminateHandler which runs the post-run
// script when a task finishes, and reports its results back to the API server.
type SignalHandler struct {
	// KillChan is a channel which once closed, causes any in-progress commands to abort.
	KillChan chan bool
	// Post is a set of commands to run after an agent completes a task execution.
	Post *model.YAMLCommandSet
	// Timeout is a set of commands to run if/when an IdleTimeout signal is received.
	Timeout *model.YAMLCommandSet
	// Channel on which to send/receive notifications from background tasks
	// (timeouts, heartbeat failures, abort signals, etc).
	signalChan chan Signal
}

// Agent controls the various components and background processes needed
// throughout the lifetime of the execution of the task.
type Agent struct {

	// TaskCommunicator handles all communication with the API server -
	// marking task started/ended, sending test results, logs, heartbeats, etc
	TaskCommunicator

	// ExecTracker keeps track of the agent's current stage of execution.
	ExecTracker

	// Channel on which to send/receive notifications from background tasks
	// (timeouts, heartbeat failures, abort signals, etc).
	signalChan chan Signal

	// signalHandler is used to process signals received by the agent during execution.
	signalHandler *SignalHandler

	// heartbeater handles triggering heartbeats at the correct intervals, and
	// raises a signal if too many heartbeats fail consecutively.
	heartbeater *HeartbeatTicker

	// statsCollector handles sending vital host system stats at the correct
	// intervals, to the API server.
	statsCollector *StatsCollector

	// logger handles all the logging (task, system, execution, local)
	// by appending log messages for each type to the correct stream.
	logger *StreamLogger

	// timeoutWatcher maintains a timer, and raises a signal if the running task
	// does not produce output within a given time frame.
	idleTimeoutWatcher *TimeoutWatcher

	// maxExecTimeoutWatcher maintains a timer, and raises a signal if the running task
	// does not return within a given time frame.
	maxExecTimeoutWatcher *TimeoutWatcher

	// APILogger is a slogger.Appender which sends log messages
	// to the API server.
	APILogger *APILogger

	// Holds the current command being executed by the agent.
	currentCommand model.PluginCommandConf

	// taskConfig holds the project, distro and task objects for the agent's
	// assigned task.
	taskConfig *model.TaskConfig

	// Registry manages plugins available for the agent.
	Registry plugin.Registry
}

// finishAndAwaitCleanup sends the returned TaskEndResponse and error - as
// gotten from the FinalTaskFunc function - for processing by the main agent loop.
func (agt *Agent) finishAndAwaitCleanup(status Signal, completed chan FinalTaskFunc) (*apimodels.TaskEndResponse, error) {
	agt.signalChan <- status
	if agt.heartbeater.stop != nil {
		agt.heartbeater.stop <- true
	}
	if agt.statsCollector.stop != nil {
		agt.statsCollector.stop <- true
	}
	if agt.idleTimeoutWatcher.stop != nil {
		agt.idleTimeoutWatcher.stop <- true
	}
	if agt.maxExecTimeoutWatcher != nil && agt.maxExecTimeoutWatcher.stop != nil {
		agt.maxExecTimeoutWatcher.stop <- true
	}
	agt.APILogger.FlushAndWait()
	taskFinishFunc := <-completed // waiting for HandleSignals() to finish
	ret, err := taskFinishFunc()  // calling taskCom.End(), or similar
	agt.APILogger.FlushAndWait()  // any logs from HandleSignals() or End()
	return ret, err
}

// getTaskEndDetail returns a default TaskEndDetail struct based on the current
// command being run (or just completed).
func (agt *Agent) getTaskEndDetail() *apimodels.TaskEndDetail {
	cmd := agt.GetCurrentCommand()
	prj := agt.taskConfig.Project

	return &apimodels.TaskEndDetail{
		Type:        cmd.GetType(prj),
		Status:      evergreen.TaskFailed,
		Description: cmd.GetDisplayName(),
	}
}

// HandleSignals listens on its signal channel and properly handles any signal received.
func (sh *SignalHandler) HandleSignals(agt *Agent, completed chan FinalTaskFunc) {
	receivedSignal := <-sh.signalChan

	// Stop any running commands.
	close(sh.KillChan)

	detail := agt.getTaskEndDetail()

	switch receivedSignal {
	case IncorrectSecret:
		agt.logger.LogLocal(slogger.ERROR, "Secret doesn't match - exiting.")
		os.Exit(1)
	case HeartbeatMaxFailed:
		agt.logger.LogLocal(slogger.ERROR, "Max heartbeats failed - exiting.")
		os.Exit(1)
	case AbortedByUser:
		detail.Status = evergreen.TaskUndispatched
		agt.logger.LogTask(slogger.WARN, "Received abort signal - stopping.")
	case IdleTimeout:
		agt.logger.LogTask(slogger.ERROR, "Task timed out: '%v'", detail.Description)
		detail.TimedOut = true
		if sh.Timeout != nil {
			agt.logger.LogTask(slogger.INFO, "Running task-timeout commands.")
			start := time.Now()
			err := agt.RunCommands(sh.Timeout.List(), false, nil)
			if err != nil {
				agt.logger.LogExecution(slogger.ERROR, "Error running task-timeout command: %v", err)
			}
			agt.logger.LogTask(slogger.INFO, "Finished running task-timeout commands in %v.", time.Since(start).String())
		}
	case CompletedSuccess:
		detail.Status = evergreen.TaskSucceeded
		agt.logger.LogTask(slogger.INFO, "Task completed - SUCCESS.")
	case CompletedFailure:
		agt.logger.LogTask(slogger.INFO, "Task completed - FAILURE.")
	}

	if sh.Post != nil {
		agt.logger.LogTask(slogger.INFO, "Running post-task commands.")
		start := time.Now()
		err := agt.RunCommands(sh.Post.List(), false, nil)
		if err != nil {
			agt.logger.LogExecution(slogger.ERROR, "Error running post-task command: %v", err)
		}
		agt.logger.LogTask(slogger.INFO, "Finished running post-task commands in %v.", time.Since(start).String())
	}
	agt.logger.LogExecution(slogger.INFO, "Sending final status as: %v", detail.Status)

	// make the API call to end the task
	completed <- func() (*apimodels.TaskEndResponse, error) {
		return agt.End(detail)
	}
}

// GetCurrentCommand returns the current command being executed
// by the agent.
func (agt *Agent) GetCurrentCommand() model.PluginCommandConf {
	return agt.currentCommand
}

// CheckIn updates the agent's execution stage and current timeout duration,
// and resets its timer back to zero.
func (agt *Agent) CheckIn(command model.PluginCommandConf, duration time.Duration) {
	agt.currentCommand = command
	agt.idleTimeoutWatcher.SetDuration(duration)
	agt.idleTimeoutWatcher.CheckIn()
	agt.logger.LogExecution(slogger.INFO, "Command timeout set to %v", duration.String())
}

// GetTaskConfig fetches task configuration data required to run the task from the API server.
func (agt *Agent) GetTaskConfig() (*model.TaskConfig, error) {
	agt.logger.LogExecution(slogger.INFO, "Fetching distro configuration.")
	distro, err := agt.GetDistro()
	if err != nil {
		return nil, err
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching project configuration.")
	project, err := agt.GetProjectConfig()
	if err != nil {
		return nil, err
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching task configuration.")
	task, err := agt.GetTask()
	if err != nil {
		return nil, err
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching project ref.")
	ref, err := agt.GetProjectRef()
	if err != nil {
		return nil, err
	}

	if ref == nil {
		return nil, fmt.Errorf("Agent retrieved an empty project ref")
	}

	agt.logger.LogExecution(slogger.INFO, "Constructing TaskConfig.")
	return model.NewTaskConfig(distro, project, task, ref)

}

// New creates a new agent to run a given task.
func New(apiServerURL, taskId, taskSecret, logFile, cert string) (*Agent, error) {
	sigChan := make(chan Signal, 1)

	// set up communicator with API server
	httpCommunicator, err := NewHTTPCommunicator(apiServerURL, taskId, taskSecret, cert, sigChan)
	if err != nil {
		return nil, err
	}

	// set up logger to API server
	apiLogger := NewAPILogger(httpCommunicator)
	idleTimeoutWatcher := &TimeoutWatcher{duration: DefaultIdleTimeout}

	// set up timeout logger, local and API logger streams
	streamLogger, err := NewStreamLogger(idleTimeoutWatcher, apiLogger, logFile)
	if err != nil {
		return nil, err
	}
	httpCommunicator.Logger = streamLogger.Execution

	// set up the heartbeat ticker
	hbTicker := &HeartbeatTicker{
		MaxFailedHeartbeats: 10,
		SignalChan:          sigChan,
		TaskCommunicator:    httpCommunicator,
		Logger:              httpCommunicator.Logger,
		Interval:            DefaultHeartbeatInterval,
	}

	// set up the system stats collector
	statsCollector := NewSimpleStatsCollector(
		streamLogger.System,
		DefaultStatsInterval,
		"df -h",
		"${ps|ps}",
	)

	agt := &Agent{
		logger:             streamLogger,
		TaskCommunicator:   httpCommunicator,
		heartbeater:        hbTicker,
		statsCollector:     statsCollector,
		idleTimeoutWatcher: idleTimeoutWatcher,
		APILogger:          apiLogger,
		signalChan:         sigChan,
		Registry:           plugin.NewSimpleRegistry(),
	}

	return agt, nil
}

// RunTask manages the process of running a task. It returns a response
// indicating the end result of the task.
func (agt *Agent) RunTask() (*apimodels.TaskEndResponse, error) {
	agt.CheckIn(InitialSetupCommand, InitialSetupTimeout)

	agt.logger.LogLocal(slogger.INFO, "Local logger initialized.")
	agt.logger.LogTask(slogger.INFO, "Task logger initialized (agent revision: %v)", evergreen.BuildRevision)
	agt.logger.LogExecution(slogger.INFO, "Execution logger initialized.")
	agt.logger.LogSystem(slogger.INFO, "System logger initialized.")

	httpAgentComm, ok := agt.TaskCommunicator.(*HTTPCommunicator)
	if ok && len(httpAgentComm.HttpsCert) == 0 {
		agt.logger.LogTask(slogger.WARN, "Running agent without a https certificate.")
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching task configuration.")

	taskConfig, err := agt.GetTaskConfig()
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error fetching task configuration: %v", err)
		return nil, err
	}

	name := taskConfig.Task.DisplayName
	pt := taskConfig.Project.FindProjectTask(name)
	execTimeout := time.Duration(pt.ExecTimeout) * time.Second
	// Set master task timeout, only if included in the taskConfig
	if execTimeout != 0 {
		agt.maxExecTimeoutWatcher = &TimeoutWatcher{duration: execTimeout}
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching expansions for project %v...", taskConfig.Task.Project)

	expVars, err := agt.FetchExpansionVars()
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error fetching project expansion variables: %v", err)
		return nil, err
	}

	taskConfig.Expansions.Update(*expVars)

	agt.taskConfig = taskConfig

	// initialize agent's signal handler to listen for signals
	signalHandler := &SignalHandler{
		KillChan:   make(chan bool),
		signalChan: agt.signalChan,
		Post:       agt.taskConfig.Project.Post,
		Timeout:    agt.taskConfig.Project.Timeout,
	}

	agt.signalHandler = signalHandler

	// start the heartbeater, timeout watcher, system stats collector
	// and signal listener
	completed := agt.StartBackgroundActions(signalHandler)

	// register plugins needed for execution
	if err = registerPlugins(agt.Registry, plugin.CommandPlugins, agt.logger); err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error initializing agent plugins: %v", err)
		return agt.finishAndAwaitCleanup(CompletedFailure, completed)
	}

	// notify API server that the task has been started.
	agt.logger.LogExecution(slogger.INFO, "Reporting task started.")
	if err = agt.Start(strconv.Itoa(os.Getpid())); err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error marking task started: %v", err)
		return agt.finishAndAwaitCleanup(CompletedFailure, completed)
	}

	if agt.taskConfig.Project.Pre != nil {
		agt.logger.LogExecution(slogger.INFO, "Running pre-task commands.")
		err = agt.RunCommands(agt.taskConfig.Project.Pre.List(), false, nil)
		if err != nil {
			agt.logger.LogExecution(slogger.ERROR, "Running pre-task script failed: %v", err)
		}
		agt.logger.LogExecution(slogger.INFO, "Finished running pre-task commands.")
	}
	return agt.RunTaskCommands(completed)
}

// RunTaskCommands runs all commands for the task currently assigend to the agent.
func (agt *Agent) RunTaskCommands(completed chan FinalTaskFunc) (*apimodels.TaskEndResponse, error) {
	conf := agt.taskConfig
	task := conf.Project.FindProjectTask(conf.Task.DisplayName)
	if task == nil {
		agt.logger.LogExecution(slogger.ERROR, "Can't find task: %v", conf.Task.DisplayName)
		return agt.finishAndAwaitCleanup(CompletedFailure, completed)
	}

	agt.logger.LogExecution(slogger.INFO, "Running task commands.")
	start := time.Now()
	err := agt.RunCommands(task.Commands, true, agt.signalHandler.KillChan)
	agt.logger.LogExecution(slogger.INFO, "Finished running task commands in %v.", time.Since(start).String())

	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Task failed: %v", err)
		return agt.finishAndAwaitCleanup(CompletedFailure, completed)
	}
	return agt.finishAndAwaitCleanup(CompletedSuccess, completed)
}

// RunCommands takes a slice of commands and executes then sequentially.
// If returnOnError is set, it returns immediately if one of the commands fails.
// All plugins listen on the stop channel and must terminate immediately when a
// value is received.
func (agt *Agent) RunCommands(commands []model.PluginCommandConf, returnOnError bool, stop chan bool) error {
	for i, commandInfo := range commands {
		parsedCommands, err := agt.Registry.ParseCommandConf(commandInfo, agt.taskConfig.Project.Functions)
		if err != nil {
			agt.logger.LogTask(slogger.ERROR, "Couldn't parse plugin command '%v': %v", commandInfo.Command, err)
			if returnOnError {
				return err
			}
			continue
		}

		cmds, err := agt.Registry.GetCommands(commandInfo, agt.taskConfig.Project.Functions)
		if err != nil {
			agt.logger.LogTask(slogger.ERROR, "Don't know how to run plugin action %s: %v", commandInfo.Command, err)
			if returnOnError {
				return err
			}
			continue
		}

		for j, cmd := range cmds {
			fullCommandName := cmd.Plugin() + "." + cmd.Name()

			parsedCommand := parsedCommands[j]

			if commandInfo.Function != "" {
				fullCommandName = fmt.Sprintf(`'%v' in "%v"`, fullCommandName, commandInfo.Function)
			} else if parsedCommand.DisplayName != "" {
				fullCommandName = fmt.Sprintf(`("%v") %v`, parsedCommand.DisplayName, fullCommandName)
			} else {
				fullCommandName = fmt.Sprintf("'%v'", fullCommandName)
			}

			// TODO: add validation for this once new config's in place/use
			if !commandInfo.RunOnVariant(agt.taskConfig.BuildVariant.Name) {
				agt.logger.LogTask(slogger.INFO, "Skipping command %v on variant %v (step %v of %v)",
					fullCommandName, agt.taskConfig.BuildVariant.Name, i+1, len(commands))
				continue
			}

			if len(cmds) == 1 {
				agt.logger.LogTask(slogger.INFO, "Running command %v (step %v of %v)", fullCommandName, i+1, len(commands))
			} else {
				// for functions with more than one command
				agt.logger.LogTask(slogger.INFO, "Running command %v (step %v.%v of %v)", fullCommandName, i+1, j+1, len(commands))
			}

			var timeoutPeriod = DefaultCmdTimeout
			if commandInfo.TimeoutSecs > 0 {
				timeoutPeriod = time.Duration(commandInfo.TimeoutSecs) * time.Second
			}

			// override function timeout with command specific timeout
			if parsedCommand.TimeoutSecs > 0 {
				timeoutPeriod = time.Duration(parsedCommand.TimeoutSecs) * time.Second
			}

			// create a new command logger to wrap the agent logger
			commandLogger := &CommandLogger{
				commandName: fullCommandName,
				logger:      agt.logger,
			}

			if len(commandInfo.Vars) > 0 {
				for key, val := range commandInfo.Vars {
					newVal, err := agt.taskConfig.Expansions.ExpandString(val)
					if err != nil {
						return fmt.Errorf("Can't expand '%v': %v", val, err)
					}
					agt.taskConfig.Expansions.Put(key, newVal)
				}
			}

			pluginCom := &TaskJSONCommunicator{cmd.Plugin(), agt.TaskCommunicator}

			agt.CheckIn(parsedCommand, timeoutPeriod)

			start := time.Now()
			err = cmd.Execute(commandLogger, pluginCom, agt.taskConfig, stop)

			agt.logger.LogExecution(slogger.INFO, "Finished %v in %v", fullCommandName, time.Since(start).String())

			if err != nil {
				agt.logger.LogTask(slogger.ERROR, "Command failed: %v", err)
				if returnOnError {
					return err
				}
				continue
			}
		}
	}
	return nil
}

// registerPlugins makes plugins available for use by the agent.
func registerPlugins(registry plugin.Registry, plugins []plugin.CommandPlugin, logger *StreamLogger) error {
	for _, pl := range plugins {
		if err := registry.Register(pl); err != nil {
			return fmt.Errorf("Failed to register plugin %v: %v", pl.Name(), err)
		}
		logger.LogExecution(slogger.INFO, "Registered plugin %v", pl.Name())
	}
	return nil
}

// StartBackgroundActions spawns goroutines that monitor various parts of the
// execution - heartbeats, timeouts, logging, etc.
func (agt *Agent) StartBackgroundActions(signalHandler TerminateHandler) chan FinalTaskFunc {
	completed := make(chan FinalTaskFunc)
	agt.heartbeater.StartHeartbeating()
	agt.statsCollector.LogStats(agt.taskConfig.Expansions)
	agt.idleTimeoutWatcher.NotifyTimeouts(agt.signalChan)

	// Default action is not to include a master timeout
	if agt.maxExecTimeoutWatcher != nil {
		agt.maxExecTimeoutWatcher.NotifyTimeouts(agt.signalChan)
	}
	go signalHandler.HandleSignals(agt, completed)

	// listen for SIGQUIT and dump a stack trace to system logs if received.
	go util.DumpStackOnSIGQUIT(evergreen.NewInfoLoggingWriter(agt.logger.System))
	return completed
}
