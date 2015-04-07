package agent

import (
	"10gen.com/mci"
	"10gen.com/mci/apimodels"
	"10gen.com/mci/model"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/patch"
	"10gen.com/mci/plugin"
	_ "10gen.com/mci/plugin/config"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AgentSignal int64

// The FinalTaskFunc describes the expected return values for a given task run
// by the agent. The finishAndAwaitCleanup listens on a channel for this
// function and runs returns its values once it receives the function to run.
// This will typically be an HTTP call to the API server (to end the task).
type FinalTaskFunc func() (*apimodels.TaskEndResponse, error)

const APIVersion = 2

const (
	HeartbeatMaxFailed AgentSignal = iota
	IncorrectSecret
	AbortedByUser
	IdleTimeout
	CompletedSuccess
	CompletedFailure
)

const DefaultCmdTimeout = 2 * time.Hour

//TaskCommunicator is an interface that handles the remote procedure calls
//between an agent and the remote server.
type TaskCommunicator interface {
	Start(pid string) error
	End(status string, details *apimodels.TaskEndDetails) (*apimodels.TaskEndResponse, error)
	GetTask() (*model.Task, error)
	GetDistro() (*distro.Distro, error)
	GetProjectConfig() (*model.Project, error)
	GetPatch() (*patch.Patch, error)
	Log([]model.LogMessage) error
	Heartbeat() (bool, error)
	FetchExpansionVars() (*apimodels.ExpansionVars, error)
	tryGet(path string) (*http.Response, error)
	tryPostJSON(path string, data interface{}) (*http.Response, error)
}

//Agent controls the various components and background processes needed
//throughout the lifetime of the execution of the task.
type Agent struct {

	//Interface which handles all communication with MOTU -
	//marking task started/ended, sending test results, logs, heartbeats, etc
	TaskCommunicator

	//Channel on which to send/receive notifications from background tasks
	//(timeouts, heartbeat failures, abort signals, etc).
	signalChan chan AgentSignal

	//heartbeater handles triggering heartbeats at the correct intervals, and
	//raises a signal if too many heartbeats fail consecutively
	heartbeater *HeartbeatTicker

	//statsCollector handles sending vital host system stats at the correct
	//intervals, to the API server
	statsCollector *StatsCollector

	//agentLogger handles all the logging (task, system, execution, local)
	//by appending log messages for each type to the correct place
	agentLogger *AgentLogger

	//timeoutWatcher maintains a timer, and raises a signal if the timer exceeds
	//its current threshold duration
	timeoutWatcher *TimeoutWatcher

	//RemoteAppender is a slogger.Appender which sends log messages
	//to an external server
	RemoteAppender *TaskCommunicatorAppender

	//execTracker keeps track of the current stage of execution that the agent
	//is working on (git clone, s3 push/pull, running test, etc)
	execTracker ExecTracker

	currentStage string

	//taskConfig is a pointer to the task config the agent is currently using
	taskConfig *model.TaskConfig
}

//TerminateHandler is an interface which defines how the agent should respond
//to signals resulting in the end of the task (heartbeat fail, timeout, etc)
type TerminateHandler interface {
	HandleSignals(taskCom TaskCommunicator,
		signalChannel chan AgentSignal,
		completed chan FinalTaskFunc)
}

//AgentSignalHandler is an implementation of TerminateHandler which
//runs the post-run script when a task finishes, and reports its results back to
//the remote server
type AgentSignalHandler struct {
	AgentLogger *AgentLogger

	//Channel to close to abort any in-progress commands
	KillChannel chan bool
	WorkDir     string
	PostRun     *AgentCommand
	Post        *model.YAMLCommandSet
	Timeout     *model.YAMLCommandSet
	execTracker ExecTracker
	Registry    plugin.Registry
	TaskConfig  *model.TaskConfig
}

type ExecTracker interface {
	GetCurrentStage() string
	CheckIn(stageName string, duration time.Duration)
}

// finishAndAwaitCleanup sends the returned TaskEndResponse and error - as
// gotten from the FinalTaskFunc function - for processing by the main agent
// loop
func (self *Agent) finishAndAwaitCleanup(status AgentSignal,
	completed chan FinalTaskFunc) (*apimodels.TaskEndResponse, error) {
	self.signalChan <- status
	if self.heartbeater.stop != nil {
		self.heartbeater.stop <- true
	}
	if self.statsCollector.stop != nil {
		self.statsCollector.stop <- true
	}
	if self.timeoutWatcher.stop != nil {
		self.timeoutWatcher.stop <- true
	}
	self.RemoteAppender.FlushAndWait()
	taskFinishFunc := <-completed      // waiting for HandleSignals() to finish
	ret, err := taskFinishFunc()       // calling the taskCom.End(), or similar
	self.RemoteAppender.FlushAndWait() // any logs from HandleSignals() or End()
	return ret, err
}

func (self *AgentSignalHandler) HandleSignals(taskCom TaskCommunicator,
	signalChannel chan AgentSignal, completed chan FinalTaskFunc) {
	receivedSignal := <-signalChannel

	//Stop any running commands.
	close(self.KillChannel)

	var finalStatus string
	var endDetails *apimodels.TaskEndDetails

	switch receivedSignal {
	case IncorrectSecret:
		self.AgentLogger.LogLocal(slogger.ERROR, "Secret doesn't match.")
		os.Exit(1)
	case HeartbeatMaxFailed:
		finalStatus = mci.TaskFailed
		self.AgentLogger.LogExecution(slogger.ERROR, "Max heartbeats failed - stopping.")
	case AbortedByUser:
		finalStatus = mci.TaskUndispatched
		self.AgentLogger.LogTask(slogger.WARN, "Received abort signal - stopping.")
	case IdleTimeout:
		currentStage := self.execTracker.GetCurrentStage()
		self.AgentLogger.LogTask(slogger.ERROR, "Task timed out during: %v",
			currentStage)
		finalStatus = mci.TaskFailed
		endDetails = &apimodels.TaskEndDetails{
			TimeoutStage: currentStage,
			TimedOut:     true,
		}
		if self.Timeout != nil {
			self.AgentLogger.LogTask(slogger.INFO, "Executing task-timeout commands...")
			err := RunAllCommands(self.Timeout.List(),
				self.Registry,
				taskCom,
				self.execTracker,
				self.AgentLogger,
				self.TaskConfig,
				false,
				nil,
			)
			if err != nil {
				self.AgentLogger.LogExecution(slogger.ERROR, "Running task-timeout "+
					"command failed: %v", err)
			}
		}
	case CompletedSuccess:
		finalStatus = mci.TaskSucceeded
		self.AgentLogger.LogTask(slogger.INFO, "Task completed - SUCCESS.")
	case CompletedFailure:
		finalStatus = mci.TaskFailed
		self.AgentLogger.LogTask(slogger.INFO, "Task completed - FAILURE.")
	}

	if self.Post != nil {
		self.AgentLogger.LogTask(slogger.INFO, "Executing post-task commands...")
		err := RunAllCommands(self.Post.List(),
			self.Registry,
			taskCom,
			self.execTracker,
			self.AgentLogger,
			self.TaskConfig,
			false,
			nil,
		)
		if err != nil {
			self.AgentLogger.LogExecution(slogger.ERROR, "Running post-run "+
				"command failed: %v", err)
		}
	}
	self.AgentLogger.LogExecution(slogger.INFO, "Sending final status as: "+
		"%v", finalStatus)
	// make the API call to end the task
	completed <- func() (*apimodels.TaskEndResponse, error) {
		return taskCom.End(finalStatus, endDetails)
	}
}

//StartBackgroundActions spawns goroutines that monitor various parts of the
//execution - heartbeats, timeouts, logging, etc.
func (self *Agent) StartBackgroundActions(signalHandler TerminateHandler) chan FinalTaskFunc {
	completed := make(chan FinalTaskFunc)
	self.heartbeater.StartHeartbeating()
	self.timeoutWatcher.NotifyTimeouts(self.signalChan)
	go signalHandler.HandleSignals(self.TaskCommunicator, self.signalChan, completed)

	// Listen for SIGQUIT and dump a stack trace to system logs if received.
	go util.DumpStackOnSIGQUIT(mci.NewInfoLoggingWriter(self.agentLogger.SystemLogger))
	return completed
}

func NewAgent(motuRootUrl string, taskId string, taskSecret string,
	verboseLogging bool, cert string) (*Agent, error) {
	signalChan := make(chan AgentSignal, 1)
	taskCom, err := NewHTTPAgentCommunicator(motuRootUrl, taskId,
		taskSecret, cert)
	if err != nil {
		return nil, err
	}
	taskCom.SignalChan = signalChan

	timeoutWatcher := &TimeoutWatcher{timeoutDuration: 15 * time.Minute}
	remoteAppender := NewTaskCommunicatorAppender(taskCom, signalChan)
	agentLogger, err := NewAgentLogger(timeoutWatcher, remoteAppender, verboseLogging)
	if err != nil {
		return nil, err
	}

	heartbeater := &HeartbeatTicker{
		MaxFailedHeartbeats: 10,
		Interval:            30 * time.Second,
		SignalChan:          signalChan,
		TaskCommunicator:    taskCom,
		Logger:              agentLogger.ExecLogger,
	}

	taskCom.Logger = agentLogger.ExecLogger
	return &Agent{
		TaskCommunicator: taskCom,
		signalChan:       signalChan,
		heartbeater:      heartbeater,
		agentLogger:      agentLogger,
		timeoutWatcher:   timeoutWatcher,
		RemoteAppender:   remoteAppender,
	}, nil
}

func (self *Agent) GetCurrentStage() string {
	return self.currentStage
}

//CheckIn updates the agent's execution stage and current timeout duration,
//and resets its timer back to zero
func (self *Agent) CheckIn(stageName string, duration time.Duration) {
	self.currentStage = stageName
	self.timeoutWatcher.SetTimeoutDuration(duration)
	self.timeoutWatcher.CheckIn()
	self.agentLogger.LogExecution(slogger.INFO, "Beginning to execute stage %v",
		stageName)
}

//PrepareTask parses the necessary config files, fetches metadata from MOTU,
//and returns a TaskConfig which contains all the config params
//needed to run the task.
func PrepareTask(agent *Agent, configRoot, workDir string) (*model.TaskConfig,
	error) {
	agent.agentLogger.LogExecution(slogger.INFO, "Fetching task info...")
	// get the task info
	task, err := agent.GetTask()
	if err != nil {
		return nil, err
	}
	var configRootAbs string
	if filepath.IsAbs(configRoot) {
		configRootAbs = configRoot
	} else {
		configRootAbs = filepath.Join(workDir, configRoot)
	}

	// get the project's configuration
	agent.agentLogger.LogExecution(slogger.INFO, "Getting project config...")
	project, err := agent.GetProjectConfig()
	if err != nil {
		return nil, err
	}

	agent.agentLogger.LogExecution(slogger.INFO, "Getting distro config...")
	distro, err := agent.GetDistro()
	if err != nil {
		return nil, err
	}

	taskConfig, err := model.NewTaskConfig(distro, project, task, workDir)
	if err != nil {
		return nil, err
	}

	configRootAbs = strings.Replace(configRootAbs, "\\", "/", -1)

	taskConfig.Expansions.Put("config_root", configRootAbs)
	return taskConfig, nil
}

func registerAll(pluginRegistry plugin.Registry,
	plugins []plugin.Plugin, agentLogger *AgentLogger) error {
	for _, pl := range plugins {
		if err := pluginRegistry.Register(pl); err != nil {
			return fmt.Errorf("Failed to register plugin %v: %v", pl.Name(), err)
		}
		agentLogger.LogExecution(slogger.INFO, "Registered plugin %v", pl.Name())
	}
	return nil
}

func RunAllCommands(
	commands []model.PluginCommandConf,
	pluginRegistry plugin.Registry,
	taskCom TaskCommunicator,
	execTracker ExecTracker,
	agentLogger *AgentLogger,
	taskConfig *model.TaskConfig,
	breakOnError bool,
	stopChan chan bool) error {

	for index, commandInfo := range commands {
		execCmds, err := pluginRegistry.GetCommands(commandInfo, taskConfig.Project.Functions)
		if err != nil {
			agentLogger.LogTask(slogger.ERROR, "Don't know how to run plugin action %s: %v", commandInfo.Command, err)
			if breakOnError {
				return err
			}
			continue
		}

		for _, cmd := range execCmds {
			fullCommandName := cmd.Plugin() + "." + cmd.Name()

			// TODO: add validation for this once new config's in place/use
			if !commandInfo.RunOnVariant(taskConfig.BuildVariant.Name) {
				agentLogger.LogTask(slogger.INFO, "Skipping command %v on variant %v (step %v of %v)",
					fullCommandName, taskConfig.BuildVariant.Name, index+1, len(commands))
				continue
			}

			agentLogger.LogTask(slogger.INFO, "Running command %v (step %v of %v)",
				fullCommandName, index+1, len(commands))
			pluginCom := &TaskJSONCommunicator{cmd.Plugin(), taskCom}

			var timeoutPeriod = DefaultCmdTimeout
			if commandInfo.TimeoutSecs > 0 {
				timeoutPeriod = time.Duration(commandInfo.TimeoutSecs) * time.Second
			}

			execTracker.CheckIn(fullCommandName, timeoutPeriod)

			cmdStartTime := time.Now()
			agentLogger.LogExecution(slogger.INFO, "Starting %v...", fullCommandName)

			// create a new command logger to wrap the agent logger
			commandLogger := &AgentCommandLogger{
				commandName: fullCommandName,
				agentLogger: agentLogger,
			}

			if len(commandInfo.Vars) > 0 {
				keysToMerge := make([]string, 0, len(commandInfo.Vars))
				valsToMerge := make([]string, 0, len(commandInfo.Vars))
				for key, val := range commandInfo.Vars {
					keysToMerge = append(keysToMerge, key)
					newVal, err := taskConfig.Expansions.ExpandString(val)
					if err != nil {
						return fmt.Errorf("Can't expand '%v': %v", val, err)
					}
					valsToMerge = append(valsToMerge, newVal)
				}
				//Now take keys/vals specified in vars and merge into expansions
				for i, key := range keysToMerge {
					taskConfig.Expansions.Put(key, valsToMerge[i])
				}
			}

			// run the command
			err = cmd.Execute(commandLogger, pluginCom, taskConfig, stopChan)

			cmdEndTime := time.Now()
			agentLogger.LogExecution(slogger.INFO, "Finished %v in %v",
				fullCommandName, (cmdEndTime.Sub(cmdStartTime)).String())
			if err != nil {
				agentLogger.LogTask(slogger.ERROR, "Command failed: %v", err)
				if breakOnError {
					return err
				}
				continue
			}
		}
	}
	return nil
}

func RunTask(agent *Agent, configRoot string, workDir string) (*apimodels.TaskEndResponse, error) {
	agent.agentLogger.LogLocal(slogger.INFO, "Local logger initialized.")
	cmdKiller := make(chan bool)

	pluginRegistry := plugin.NewSimpleRegistry()
	signalHandler := &AgentSignalHandler{
		AgentLogger: agent.agentLogger,
		KillChannel: cmdKiller,
		PostRun:     nil,
		execTracker: agent,
		WorkDir:     workDir,
		TaskConfig:  nil,
		Registry:    pluginRegistry,
	}

	completed := agent.StartBackgroundActions(signalHandler)

	agent.CheckIn(InitialSetupStage, InitialSetupTimeout)
	agent.agentLogger.LogTask(slogger.INFO, "Task logger initialized.")
	agent.agentLogger.LogExecution(slogger.INFO, "Execution logger initialized.")
	agent.agentLogger.LogExecution(slogger.INFO, "Reporting task started...")

	// really this should be logged much earlier, but our current logger is
	// not usable until we've run the agent.StartBackgroundActions()
	httpAgentComm, ok := agent.TaskCommunicator.(*HTTPAgentCommunicator)
	if ok && len(httpAgentComm.HttpsCert) == 0 {
		agent.agentLogger.LogTask(slogger.WARN, "Running Agent without an https cert")
	}

	//notify MOTU that the task has been started.
	err := agent.Start(strconv.Itoa(os.Getpid()))
	if err != nil {
		agent.agentLogger.LogExecution(slogger.ERROR, "Error occurred marking "+
			"task started: %v", err)
		return agent.finishAndAwaitCleanup(CompletedFailure, completed)
	}

	//register plugins needed for execution
	err = registerAll(pluginRegistry, plugin.Published, agent.agentLogger)

	if err != nil {
		agent.agentLogger.LogExecution(slogger.ERROR, "Error initializing "+
			"agent plugins: %v", err)
		return agent.finishAndAwaitCleanup(CompletedFailure, completed)
	}

	agent.agentLogger.LogExecution(slogger.INFO, "Loading task config...")
	taskConfig, err := PrepareTask(agent, configRoot, workDir)
	if err != nil {
		agent.agentLogger.LogExecution(slogger.ERROR, "error occurred "+
			"preparing task %v", err)
		return agent.finishAndAwaitCleanup(CompletedFailure, completed)
	}
	signalHandler.TaskConfig = taskConfig
	statsCollector := NewSimpleStatsCollector(
		agent.agentLogger.SystemLogger,
		60*time.Second,
		taskConfig.Expansions,
		"df -h",
		"${ps|ps}",
	)
	agent.statsCollector = statsCollector
	agent.statsCollector.LogStats()
	agent.taskConfig = taskConfig

	signalHandler.WorkDir = taskConfig.SourceDir
	signalHandler.Post = taskConfig.Project.Post
	signalHandler.Timeout = taskConfig.Project.Timeout

	agent.agentLogger.LogExecution(slogger.INFO, "Fetching expansions for project: %v", taskConfig.Task.Project)
	//fetch the project vars for the project every time
	resultVars, err := agent.TaskCommunicator.FetchExpansionVars()
	if err != nil {
		agent.agentLogger.LogExecution(slogger.ERROR, "Fetching vars failed: %v", err)
		return nil, err
	}

	agent.taskConfig.Expansions.Update(*resultVars)

	if taskConfig.Project.Pre != nil {
		agent.agentLogger.LogExecution(slogger.INFO, "Running pre-task commands")
		err = RunAllCommands(taskConfig.Project.Pre.List(), pluginRegistry,
			agent.TaskCommunicator, agent, agent.agentLogger, taskConfig, false, nil)
		if err != nil {
			agent.agentLogger.LogExecution(slogger.ERROR, "Running pre-task script failed: %v", err)
		}
	}
	return RunPluginCommands(agent, pluginRegistry, taskConfig, cmdKiller, completed)
}

func RunPluginCommands(agent *Agent, pluginRegistry plugin.Registry,
	conf *model.TaskConfig, cmdKiller chan bool, completed chan FinalTaskFunc) (
	*apimodels.TaskEndResponse, error) {
	projectTask := conf.Project.FindProjectTask(conf.Task.DisplayName)
	if projectTask == nil {
		agent.agentLogger.LogExecution(slogger.ERROR, "Can't find task: %v", conf.Task.DisplayName)
		return agent.finishAndAwaitCleanup(CompletedFailure, completed)
	}

	err := RunAllCommands(
		projectTask.Commands,
		pluginRegistry,
		agent.TaskCommunicator,
		agent,
		agent.agentLogger,
		conf,
		true,
		cmdKiller,
	)
	if err != nil {
		agent.agentLogger.LogExecution(slogger.ERROR, "Command failed: %v", err)
		return agent.finishAndAwaitCleanup(CompletedFailure, completed)
	}
	return agent.finishAndAwaitCleanup(CompletedSuccess, completed)
}
