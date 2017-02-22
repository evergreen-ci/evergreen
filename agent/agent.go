package agent

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/shell"
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/slogger"
)

const (
	// DefaultCmdTimeout specifies the duration after which agent sends
	// an IdleTimeout signal if a task's command does not run to completion.
	DefaultCmdTimeout = 2 * time.Hour

	// DefaultExecTimeoutSecs specifies in seconds the maximum time a task is allowed to run
	// for, even if it is not idle. This default is used if exec_timeout_secs is not specified
	// in the project file.
	DefaultExecTimeoutSecs = 60 * 60 * 6 // six hours

	// DefaultIdleTimeout specifies the duration after which agent sends an
	// IdleTimeout signal if a task produces no logs.
	DefaultIdleTimeout = 20 * time.Minute
	// DefaultCallbackCmdTimeout specifies the duration after when the "post" or
	// "timeout" command sets should be shut down.
	DefaultCallbackCmdTimeout = 15 * time.Minute
	// DefaultHeartbeatInterval is interval after which agent sends a heartbeat
	// to API server.
	DefaultHeartbeatInterval = 30 * time.Second
	// DefaultStatsInterval is the interval after which agent sends system stats
	// to API server
	DefaultStatsInterval = time.Minute
)

var (
	// InitialSetupTimeout indicates the time allowed for the agent to collect
	// relevant information - for running a task - from the API server.
	InitialSetupTimeout = 20 * time.Minute
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
	HandleSignals(*Agent)
}

// ExecTracker exposes functions to update and get the current execution stage
// of the agent.
type ExecTracker interface {
	// Returns the current command being executed.
	CurrentCommand() *model.PluginCommandConf
	// Sets the current command being executed as well as a timeout for the command.
	CheckIn(command model.PluginCommandConf, timeout time.Duration)
}

// SignalHandler is an implementation of TerminateHandler which runs the post-run
// script when a task finishes, and reports its results back to the API server.
type SignalHandler struct {
	// signal channels for each background process
	directoryChan, heartbeatChan, idleTimeoutChan, execTimeoutChan, communicatorChan chan comm.Signal

	// a single channel for stopping all background processes
	stopBackgroundChan chan struct{}
}

// Agent controls the various components and background processes needed
// throughout the lifetime of the execution of the task.
type Agent struct {

	// TaskCommunicator handles all communication with the API server -
	// marking task started/ended, sending test results, logs, heartbeats, etc
	comm.TaskCommunicator

	// ExecTracker keeps track of the agent's current stage of execution.
	ExecTracker

	// KillChan is a channel which once closed, causes any in-progress commands to abort.
	KillChan chan bool

	// endChan holds a base set of task details that are populated during erroneous
	// execution behavior.
	endChan chan *apimodels.TaskEndDetail

	// signalHandler is used to process signals received by the agent during execution.
	signalHandler *SignalHandler

	// heartbeater handles triggering heartbeats at the correct intervals, and
	// raises a signal if too many heartbeats fail consecutively.
	heartbeater *comm.HeartbeatTicker

	// statsCollector handles sending vital host system stats at the correct
	// intervals, to the API server.
	statsCollector *StatsCollector

	// metrics collector collects and sends process and system
	// stats/metrics to the API server. This data is collected
	// using native-(gopsutil) via the logging package.
	metricsCollector *metricsCollector

	// logger handles all the logging (task, system, execution, local)
	// by appending log messages for each type to the correct stream.
	logger *comm.StreamLogger

	// timeoutWatcher maintains a timer, and raises a signal if the running task
	// does not produce output within a given time frame.
	idleTimeoutWatcher *comm.TimeoutWatcher

	// maxExecTimeoutWatcher maintains a timer, and raises a signal if the running task
	// does not return within a given time frame.
	maxExecTimeoutWatcher *comm.TimeoutWatcher

	// APILogger is a slogger.Appender which sends log messages
	// to the API server.
	APILogger *comm.APILogger

	// Holds the current command being executed by the agent.
	currentCommand      model.PluginCommandConf
	currentCommandMutex sync.RWMutex

	// taskConfig holds the project, distro and task objects for the agent's
	// assigned task.
	taskConfig *model.TaskConfig

	// hostId holds the identifier of the host the agent thinks it is running on,
	// for logging purposes.
	hostId string

	// Registry manages plugins available for the agent.
	Registry plugin.Registry

	// currentTaskDir holds the absolute path of the directory that the agent has
	// created for executing the current task.
	currentTaskDir string

	// location of the .pid lock file
	pidFilePath string
}

// finishAndAwaitCleanup sends the returned TaskEndResponse and error
// for processing by the main agent loop.
func (agt *Agent) finishAndAwaitCleanup(status string) (*apimodels.TaskEndResponse, error) {
	// Signal all background actions to stop. If HandleSignals is still running,
	// this will cause it to return.
	close(agt.signalHandler.stopBackgroundChan)
	var detail *apimodels.TaskEndDetail
	select {
	case detail = <-agt.endChan:
	default:
		// endChan will be empty if the task completed without error
		detail = agt.getTaskEndDetail()
	}
	if status == evergreen.TaskSucceeded {
		detail.Status = evergreen.TaskSucceeded
		agt.logger.LogTask(slogger.INFO, "Task completed - SUCCESS.")
	} else {
		agt.logger.LogTask(slogger.INFO, "Task completed - FAILURE.")
	}

	// run cleanup before and after the post operations.
	if err := shell.KillSpawnedProcs(agt.taskConfig.Task.Id, agt.logger); err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error cleaning up spawned processes (before-post): %v", err)
	}

	// run post commands
	if agt.taskConfig.Project.Post != nil {
		agt.logger.LogTask(slogger.INFO, "Running post-task commands.")
		start := time.Now()
		err := agt.RunCommands(agt.taskConfig.Project.Post.List(), false, agt.callbackTimeoutSignal())
		if err != nil {
			agt.logger.LogExecution(slogger.ERROR, "Error running post-task command: %v", err)
		}
		agt.logger.LogTask(slogger.INFO, "Finished running post-task commands in %v.", time.Since(start).String())
	}

	// run cleanup before and after the post operations.
	if err := shell.KillSpawnedProcs(agt.taskConfig.Task.Id, agt.logger); err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error cleaning up spawned processes (after-post): %v", err)
	}

	err := agt.removeTaskDirectory()
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error removing task directory: %v", err)
	}

	agt.logger.LogExecution(slogger.INFO, "Sending final status as: %v", detail.Status)
	agt.APILogger.FlushAndWait() // ensure that logs are sent before task ends.

	ret, err := agt.End(detail)
	if ret != nil && !ret.RunNext {
		agt.logger.LogExecution(slogger.INFO, "No new tasks to run. Agent will shut down.")
	}
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

// makeChannels allocates async channels for each background process.
func (sh *SignalHandler) makeChannels() {
	sh.heartbeatChan = make(chan comm.Signal, 1)
	sh.idleTimeoutChan = make(chan comm.Signal, 1)
	sh.execTimeoutChan = make(chan comm.Signal, 1)
	sh.communicatorChan = make(chan comm.Signal, 1)
	sh.directoryChan = make(chan comm.Signal, 1)
	sh.stopBackgroundChan = make(chan struct{})
}

// awaitSignal multiplexes inputs from the various background processes
func (sh *SignalHandler) awaitSignal() comm.Signal {
	var sig comm.Signal
	select {
	case sig = <-sh.heartbeatChan:
	case sig = <-sh.idleTimeoutChan:
	case sig = <-sh.execTimeoutChan:
	case sig = <-sh.communicatorChan:
	case sig = <-sh.directoryChan:
	case <-sh.stopBackgroundChan:
		return comm.Completed
	}
	return sig
}

// CreatePidFile checks that the pid file does not already exist with a different pid
// and creates one
func (agt *Agent) CreatePidFile(pidFilePath string) error {
	// create a file that will error out if there is another process writing to the file, add the read/write flag to
	// indicate that reading and writing can happen.
	pidFile, err := os.OpenFile(pidFilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		// try opening the file normally and error out with the contents of the pid file for error
		pidFile, err = os.OpenFile(pidFilePath, os.O_RDONLY, 0600)
		if err != nil {
			agt.logger.LogExecution(slogger.ERROR, "error opening agent pid file: %v", err)
			return err
		}
		if err == nil {
			defer pidFile.Close()
		}

		pidBytes := make([]byte, 64)
		_, err = pidFile.Read(pidBytes)
		if err != nil {
			agt.logger.LogExecution(slogger.ERROR, "error reading existing pid file: %v", err)
			return err
		}

		agt.logger.LogExecution(slogger.ERROR, "pid file already exists with contents: %v", string(pidBytes))
		return fmt.Errorf("host already has a process id file: %v", string(pidBytes))

	}

	defer pidFile.Close()
	pid := os.Getpid()
	// write to pid file
	_, err = pidFile.Write([]byte(strconv.Itoa(pid)))
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error writing pid file: %v", err.Error())
		return fmt.Errorf("Error writing pid file: %v", err.Error())
	}
	agt.logger.LogExecution(slogger.INFO, "pid file written for process: %v", pid)
	return nil
}

// HandleSignals listens on its signal channel and properly handles any signal received.
func (sh *SignalHandler) HandleSignals(agt *Agent) {
	receivedSignal := sh.awaitSignal()
	detail := agt.getTaskEndDetail()
	switch receivedSignal {
	case comm.Completed:
		agt.logger.LogLocal(slogger.INFO, "Task executed correctly - cleaning up")
		// everything went according to plan, so we just exit the signal handler routine
		return
	case comm.IncorrectSecret:
		agt.logger.LogLocal(slogger.ERROR, "Secret doesn't match - exiting.")
		ExitAgent(agt.logger, 1, agt.pidFilePath)
	case comm.HeartbeatMaxFailed:
		agt.logger.LogLocal(slogger.ERROR, "Max heartbeats failed - exiting.")
		ExitAgent(agt.logger, 1, agt.pidFilePath)
	case comm.AbortedByUser:
		detail.Status = evergreen.TaskUndispatched
		agt.logger.LogTask(slogger.WARN, "Received abort signal - stopping.")
	case comm.DirectoryFailure:
		detail.Status = evergreen.TaskFailed
		detail.Type = model.SystemCommandType
		agt.logger.LogTask(slogger.ERROR, "Directory creation failure - stopping.")
	case comm.IdleTimeout:
		agt.logger.LogTask(slogger.ERROR, "Task timed out: '%v'", detail.Description)
		detail.TimedOut = true
		if agt.taskConfig.Project.Timeout != nil {
			agt.logger.LogTask(slogger.INFO, "Running task-timeout commands.")
			start := time.Now()
			err := agt.RunCommands(agt.taskConfig.Project.Timeout.List(), false, agt.callbackTimeoutSignal())
			if err != nil {
				agt.logger.LogExecution(slogger.ERROR, "Error running task-timeout command: %v", err)
			}
			agt.logger.LogTask(slogger.INFO, "Finished running task-timeout commands in %v.", time.Since(start).String())
		}

	}

	// buffer the end details so that we can pick them up once the running command finishes
	agt.endChan <- detail

	// stop any running commands.
	close(agt.KillChan)

}

// GetCurrentCommand returns the current command being executed
// by the agent.
func (agt *Agent) GetCurrentCommand() model.PluginCommandConf {
	agt.currentCommandMutex.RLock()
	defer agt.currentCommandMutex.RUnlock()

	return agt.currentCommand
}

// CheckIn updates the agent's execution stage and current timeout duration,
// and resets its timer back to zero.
func (agt *Agent) CheckIn(command model.PluginCommandConf, duration time.Duration) {
	agt.currentCommandMutex.Lock()
	agt.currentCommand = command
	agt.currentCommandMutex.Unlock()

	agt.idleTimeoutWatcher.SetDuration(duration)
	agt.idleTimeoutWatcher.CheckIn()
	agt.logger.LogExecution(slogger.INFO, "Command timeout set to %v", duration.String())
}

// GetTaskConfig fetches task configuration data required to run the task from the API server.
func (agt *Agent) GetTaskConfig() (*model.TaskConfig, error) {
	agt.logger.LogExecution(slogger.INFO, "Fetching distro configuration.")
	confDistro, err := agt.GetDistro()
	if err != nil {
		return nil, err
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching version.")
	confVersion, err := agt.GetVersion()
	if err != nil {
		return nil, err
	}

	confProject := &model.Project{}
	err = model.LoadProjectInto([]byte(confVersion.Config), confVersion.Identifier, confProject)
	if err != nil {
		return nil, fmt.Errorf("reading project config: %v", err)
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching task configuration.")
	confTask, err := agt.GetTask()
	if err != nil {
		return nil, err
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching project ref.")
	confRef, err := agt.GetProjectRef()
	if err != nil {
		return nil, err
	}
	if confRef == nil {
		return nil, fmt.Errorf("agent retrieved an empty project ref")
	}

	agt.logger.LogExecution(slogger.INFO, "Constructing TaskConfig.")
	return model.NewTaskConfig(confDistro, confVersion, confProject, confTask, confRef)
}

// Options represents an agent configuration.
type Options struct {
	APIURL      string
	TaskId      string // TODO remove (EVG-1283)
	TaskSecret  string
	HostId      string
	HostSecret  string
	PIDFilePath string
	Certificate string
}

// New creates a new agent to run a given task.
func New(opts Options) (*Agent, error) {
	sh := &SignalHandler{}
	sh.makeChannels()

	// set up communicator with API server
	httpCommunicator, err := comm.NewHTTPCommunicator(
		opts.APIURL, opts.TaskId, opts.TaskSecret, opts.HostId, opts.HostSecret,
		opts.Certificate, sh.communicatorChan)
	if err != nil {
		return nil, err
	}

	// set up logger to API server
	apiLogger := comm.NewAPILogger(httpCommunicator)
	idleTimeoutWatcher := comm.NewTimeoutWatcher(sh.stopBackgroundChan)
	idleTimeoutWatcher.SetDuration(DefaultIdleTimeout)

	// set up timeout logger, local and API logger streams
	streamLogger, err := comm.NewStreamLogger(idleTimeoutWatcher, apiLogger)
	if err != nil {
		return nil, err
	}
	httpCommunicator.Logger = streamLogger.Execution

	// set up the heartbeat ticker
	hbTicker := comm.NewHeartbeatTicker(sh.stopBackgroundChan)
	hbTicker.MaxFailedHeartbeats = 10
	hbTicker.SignalChan = sh.heartbeatChan
	hbTicker.TaskCommunicator = httpCommunicator
	hbTicker.Logger = httpCommunicator.Logger
	hbTicker.Interval = DefaultHeartbeatInterval

	// set up the system stats collector
	statsCollector := NewSimpleStatsCollector(
		streamLogger.System,
		DefaultStatsInterval,
		sh.stopBackgroundChan,
		"uptime",
		"df -h",
		"${ps|ps}",
	)
	killChan := make(chan bool)

	agt := &Agent{
		signalHandler:    sh,
		logger:           streamLogger,
		TaskCommunicator: httpCommunicator,
		heartbeater:      hbTicker,
		statsCollector:   statsCollector,
		metricsCollector: &metricsCollector{
			comm: httpCommunicator,
			stop: killChan,
		},
		idleTimeoutWatcher: idleTimeoutWatcher,
		APILogger:          apiLogger,
		Registry:           plugin.NewSimpleRegistry(),
		KillChan:           killChan,
		endChan:            make(chan *apimodels.TaskEndDetail, 1),
		pidFilePath:        opts.PIDFilePath,
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

	httpAgentComm, ok := agt.TaskCommunicator.(*comm.HTTPCommunicator)
	if ok && len(httpAgentComm.HttpsCert) == 0 {
		agt.logger.LogTask(slogger.WARN, "Running agent without a https certificate.")
	}

	taskConfig, err := agt.GetTaskConfig()
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error fetching task configuration: %v", err)
		return nil, err
	}

	agt.logger.LogTask(slogger.INFO,
		"Starting task %v, execution %v.", taskConfig.Task.Id, taskConfig.Task.Execution)

	pt := taskConfig.Project.FindProjectTask(taskConfig.Task.DisplayName)
	if pt.ExecTimeoutSecs == 0 {
		// if unspecified in the project task and the project, use the default value
		if taskConfig.Project.ExecTimeoutSecs != 0 {
			pt.ExecTimeoutSecs = taskConfig.Project.ExecTimeoutSecs
		} else {
			pt.ExecTimeoutSecs = DefaultExecTimeoutSecs
		}
	}
	execTimeout := time.Duration(pt.ExecTimeoutSecs) * time.Second
	// Set master task timeout, only if included in the taskConfig
	if execTimeout != 0 {
		agt.maxExecTimeoutWatcher = comm.NewTimeoutWatcher(
			agt.signalHandler.stopBackgroundChan)
		agt.maxExecTimeoutWatcher.SetDuration(execTimeout)
	}

	agt.logger.LogExecution(slogger.INFO, "Fetching expansions for project %v...", taskConfig.Task.Project)
	expVars, err := agt.FetchExpansionVars()
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error fetching project expansion variables: %v", err)
		return nil, err
	}
	taskConfig.Expansions.Update(*expVars)
	agt.taskConfig = taskConfig

	// start the heartbeater, timeout watcher, system stats collector, and signal listener
	agt.StartBackgroundActions(agt.signalHandler)

	err = agt.createTaskDirectory(taskConfig)
	if err != nil {
		agt.signalHandler.directoryChan <- comm.DirectoryFailure
		return nil, err
	}
	taskConfig.Expansions.Put("workdir", taskConfig.WorkDir)

	// register plugins needed for execution
	if err = registerPlugins(agt.Registry, plugin.CommandPlugins, agt.logger); err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error initializing agent plugins: %v", err)
		return agt.finishAndAwaitCleanup(evergreen.TaskFailed)
	}

	// notify API server that the task has been started.
	agt.logger.LogExecution(slogger.INFO, "Reporting task started.")
	if err = agt.Start(strconv.Itoa(os.Getpid())); err != nil {
		agt.logger.LogExecution(slogger.ERROR, "error marking task started: %v", err)
		return agt.finishAndAwaitCleanup(evergreen.TaskFailed)
	}

	if taskConfig.Project.Pre != nil {
		agt.logger.LogExecution(slogger.INFO, "Running pre-task commands.")
		err = agt.RunCommands(taskConfig.Project.Pre.List(), false, agt.callbackTimeoutSignal())
		if err != nil {
			agt.logger.LogExecution(slogger.ERROR, "Running pre-task script failed: %v", err)
		}
		agt.logger.LogExecution(slogger.INFO, "Finished running pre-task commands.")
	}

	return agt.RunTaskCommands()
}

// RunTaskCommands runs all commands for the task currently assigend to the agent.
func (agt *Agent) RunTaskCommands() (*apimodels.TaskEndResponse, error) {
	conf := agt.taskConfig
	task := conf.Project.FindProjectTask(conf.Task.DisplayName)

	if task == nil {
		agt.logger.LogExecution(slogger.ERROR, "Can't find task: %v", conf.Task.DisplayName)
		return agt.finishAndAwaitCleanup(evergreen.TaskFailed)
	}

	agt.logger.LogExecution(slogger.INFO, "Running task commands.")
	start := time.Now()
	err := agt.RunCommands(task.Commands, true, agt.KillChan)
	agt.logger.LogExecution(slogger.INFO, "Finished running task commands in %v.", time.Since(start).String())
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Task failed: %v", err)
		return agt.finishAndAwaitCleanup(evergreen.TaskFailed)
	}
	return agt.finishAndAwaitCleanup(evergreen.TaskSucceeded)
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
			if !commandInfo.RunOnVariant(agt.taskConfig.BuildVariant.Name) ||
				!parsedCommand.RunOnVariant(agt.taskConfig.BuildVariant.Name) {
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
			commandLogger := comm.NewCommandLogger(fullCommandName, agt.logger)

			if len(commandInfo.Vars) > 0 {
				for key, val := range commandInfo.Vars {
					newVal, err := agt.taskConfig.Expansions.ExpandString(val)
					if err != nil {
						return fmt.Errorf("Can't expand '%v': %v", val, err)
					}
					agt.taskConfig.Expansions.Put(key, newVal)
				}
			}

			pluginCom := &comm.TaskJSONCommunicator{cmd.Plugin(), agt.TaskCommunicator}

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
func registerPlugins(registry plugin.Registry, plugins []plugin.CommandPlugin, logger *comm.StreamLogger) error {
	for _, pl := range plugins {
		if err := registry.Register(pl); err != nil {
			return fmt.Errorf("Failed to register plugin %v: %v", pl.Name(), err)
		}
		logger.LogExecution(slogger.INFO, "Registered plugin %v", pl.Name())
	}
	return nil
}

// callbackTimeoutSignal creates a stop channel that closes after
// the the project's CallbackTimeout has passed. Uses the default
// value if none is set.
func (agt *Agent) callbackTimeoutSignal() chan bool {
	timeout := DefaultCallbackCmdTimeout
	if agt.taskConfig.Project.CallbackTimeout != 0 {
		timeout = time.Duration(agt.taskConfig.Project.CallbackTimeout) * time.Second
	}
	stop := make(chan bool)
	go func() {
		time.Sleep(timeout)
		close(stop)
	}()
	return stop
}

// StartBackgroundActions spawns goroutines that monitor various parts of the
// execution - heartbeats, timeouts, logging, etc.
func (agt *Agent) StartBackgroundActions(signalHandler TerminateHandler) {
	agt.heartbeater.StartHeartbeating()
	agt.statsCollector.LogStats(agt.taskConfig.Expansions)
	agt.idleTimeoutWatcher.NotifyTimeouts(agt.signalHandler.idleTimeoutChan)
	grip.CatchError(agt.metricsCollector.start())
	if agt.maxExecTimeoutWatcher != nil {
		// default action is not to include a master timeout
		agt.maxExecTimeoutWatcher.NotifyTimeouts(agt.signalHandler.execTimeoutChan)
	}
	go signalHandler.HandleSignals(agt)
}

// createTaskDirectory makes a directory for the agent to execute
// the current task within. It changes the necessary variables
// so that all of the agent's operations will use this folder.
func (agt *Agent) createTaskDirectory(taskConfig *model.TaskConfig) error {
	h := md5.New()

	h.Write([]byte(
		fmt.Sprintf("%s_%d_%d", taskConfig.Task.Id, taskConfig.Task.Execution, os.Getpid())))
	dirName := hex.EncodeToString(h.Sum(nil))
	newDir := filepath.Join(taskConfig.Distro.WorkDir, dirName)

	agt.logger.LogExecution(slogger.INFO, "Making new folder for task execution: %v", newDir)
	err := os.Mkdir(newDir, 0777)
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error creating task directory: %v", err)
		return err
	}

	agt.logger.LogExecution(slogger.INFO, "Changing into task directory: %v", newDir)
	err = os.Chdir(newDir)
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error changing into task directory: %v", err)
		return err
	}
	agt.currentTaskDir = newDir

	taskConfig.WorkDir = agt.currentTaskDir
	return nil
}

// stop is only called in deferred statements in testing, but makes it
// possible to kill the background process in an agent
func (agt *Agent) stop() {
	grip.Notice("intending to forcibly stop the agent")
	select {
	case agt.KillChan <- true:
		grip.Info("sent agent stop signal")
		close(agt.KillChan)
	default:
		grip.Info("couldn't stop agent because it was already stopped")
	}
}

// removeTaskDirectory removes the folder the agent created for the
// task it was executing.
func (agt *Agent) removeTaskDirectory() error {
	agt.logger.LogExecution(slogger.INFO, "Changing directory back to distro working directory.")
	err := os.Chdir(agt.taskConfig.Distro.WorkDir)
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error changing directory out of task directory: %v", err)
		return err
	}

	agt.logger.LogExecution(slogger.INFO, "Deleting directory for completed task.")
	err = os.RemoveAll(agt.currentTaskDir)
	if err != nil {
		agt.logger.LogExecution(slogger.ERROR, "Error removing working directory for the task: %v", err)
		return err
	}
	agt.currentTaskDir = ""
	return nil
}

// ExitAgent removes the pid file and exits the process with the given exit code.
func ExitAgent(logger *comm.StreamLogger, exitCode int, pidFile string) {
	err := os.Remove(pidFile)
	if err != nil {
		if logger != nil {
			logger.LogLocal(slogger.ERROR, "Error removing .pid file: %v", err)
			logger.Flush()
		} else {
			fmt.Printf("Error removing .pid file: %v", err)
		}
		// exit with code 2 to indicate pid file removal error
		os.Exit(2)
	}
	// if the pid file is removed also flush if necessary
	if logger != nil {
		logger.Flush()
	}
	os.Exit(exitCode)
}
