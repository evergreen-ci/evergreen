package taskexec

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/executor"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// localLoggerProducer implements the LoggerProducer interface for local execution
type localLoggerProducer struct {
	logger grip.Journaler
	closed bool
}

func (l *localLoggerProducer) Execution() grip.Journaler       { return l.logger }
func (l *localLoggerProducer) Task() grip.Journaler            { return l.logger }
func (l *localLoggerProducer) System() grip.Journaler          { return l.logger }
func (l *localLoggerProducer) Flush(ctx context.Context) error { return nil }
func (l *localLoggerProducer) Close() error                    { l.closed = true; return nil }
func (l *localLoggerProducer) Closed() bool                    { return l.closed }

type executorBlock struct {
	blockType   command.BlockType
	commands    *model.YAMLCommandSet
	startIndex  int
	endIndex    int
	canFailTask bool
}

// LocalExecutor implements task execution for local YAML files
type LocalExecutor struct {
	project        *model.Project
	parserProject  *model.ParserProject
	workDir        string
	expansions     *util.Expansions
	logger         grip.Journaler
	debugState     *DebugState
	commandBlocks  []executorBlock
	communicator   client.Communicator
	loggerProducer client.LoggerProducer
	taskConfig     *internal.TaskConfig
	jasperManager  jasper.Manager
}

// LocalExecutorOptions contains configuration for the local executor
type LocalExecutorOptions struct {
	WorkingDir string
	LogFile    string
	LogLevel   string
	Timeout    int
	Expansions map[string]string
}

// NewLocalExecutor creates a new local task executor
func NewLocalExecutor(opts LocalExecutorOptions) (*LocalExecutor, error) {
	logger := grip.NewJournaler("evergreen-local")

	expansions := util.Expansions{}
	if opts.WorkingDir != "" {
		expansions.Put("workdir", opts.WorkingDir)
	}
	for k, v := range opts.Expansions {
		expansions.Put(k, v)
	}

	// TODO: DEVPROD-24941 Send requests to app server
	comm := client.NewMock("http://localhost")

	loggerProducer := &localLoggerProducer{
		logger: logger,
	}

	taskConfig := &internal.TaskConfig{
		Expansions: expansions,
		WorkDir:    opts.WorkingDir,
	}

	jasperManager, err := jasper.NewSynchronizedManager(false)
	if err != nil {
		return nil, errors.Wrap(err, "creating jasper manager")
	}

	return &LocalExecutor{
		workDir:        opts.WorkingDir,
		expansions:     &expansions,
		logger:         logger,
		debugState:     NewDebugState(),
		communicator:   comm,
		loggerProducer: loggerProducer,
		taskConfig:     taskConfig,
		jasperManager:  jasperManager,
		commandBlocks:  []executorBlock{},
	}, nil
}

// LoadProject loads and parses an Evergreen project configuration from a file
func (e *LocalExecutor) LoadProject(configPath string) (*model.Project, error) {
	e.logger.Infof("Loading project from: %s", configPath)

	yamlBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading config file '%s'", configPath)
	}

	project := &model.Project{}
	pp := &model.ParserProject{}

	err = yaml.Unmarshal(yamlBytes, pp)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshaling YAML")
	}
	e.parserProject = pp

	project, err = model.TranslateProject(pp)
	if err != nil {
		return nil, errors.Wrap(err, "translating project")
	}
	e.project = project

	if e.taskConfig != nil {
		e.taskConfig.Project = *project
	}

	e.logger.Infof("Loaded project with %d tasks and %d build variants",
		len(project.Tasks), len(project.BuildVariants))

	return project, nil
}

// SetupWorkingDirectory prepares the working directory for task execution
func (e *LocalExecutor) SetupWorkingDirectory(path string) error {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "getting current directory")
		}
		path = cwd
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Wrapf(err, "creating working directory '%s'", path)
	}

	e.workDir = path
	e.expansions.Put("workdir", path)
	e.logger.Infof("Working directory set to: %s", path)

	return nil
}

// StepNext executes the current step and advances to the next
func (e *LocalExecutor) StepNext(ctx context.Context) error {
	if e.debugState.CurrentStepIndex >= len(e.debugState.CommandList) {
		return errors.New("no more steps to execute")
	}
	return e.stepNextWithBlocks(ctx)
}

// stepNextWithBlocks executes the current step using RunCommandsInBlock
func (e *LocalExecutor) stepNextWithBlocks(ctx context.Context) error {
	if e.debugState.CurrentStepIndex >= len(e.debugState.CommandList) {
		return errors.New("no more steps to execute")
	}

	for _, block := range e.commandBlocks {
		cmdBlock := executor.CommandBlock{
			Block:    block.blockType,
			Commands: block.commands,
		}

		deps := e.createBlockDeps()

		originalStepIndex := e.debugState.CurrentStepIndex
		executed := false

		deps.RunCommandOrFunc = func(
			ctx context.Context,
			commandInfo model.PluginCommandConf,
			cmds []command.Command,
			blockType command.BlockType,
			canFailTask bool,
		) error {
			// Skip commands that are not the current step
			if e.debugState.CurrentStepIndex != originalStepIndex || executed {
				return nil
			}

			for _, cmd := range cmds {
				cmd.SetJasperManager(e.jasperManager)
				err := cmd.Execute(ctx, e.communicator, e.loggerProducer, e.taskConfig)
				if err != nil {
					e.logger.Errorf("Step %d failed: %v", e.debugState.CurrentStepIndex, err)
					return err
				}
				e.logger.Infof("Step %d completed successfully", e.debugState.CurrentStepIndex)
			}

			executed = true
			e.debugState.CurrentStepIndex++
			return nil
		}
		if err := executor.RunCommandsInBlock(ctx, deps, cmdBlock); err != nil {
			return err
		}
		if executed {
			return nil
		}
	}

	return nil
}

// executeCommand executes a single command using the agent command registry
func (e *LocalExecutor) executeCommand(ctx context.Context, cmdInfo CommandInfo) error {
	cmd := cmdInfo.Command.Command
	factory, ok := command.GetCommandFactory(cmd)
	if !ok {
		e.logger.Warningf("Command '%s' not found in registry, skipping", cmd)
		return errors.Errorf("command '%s' is not registered", cmd)
	}
	cmdInstance := factory()
	if err := cmdInstance.ParseParams(cmdInfo.Command.Params); err != nil {
		return errors.Wrapf(err, "parsing parameters for command '%s'", cmd)
	}

	cmdInstance.SetType(cmdInfo.Command.GetType(e.project))
	cmdInstance.SetFullDisplayName(cmdInfo.DisplayName)
	if cmdInfo.Command.TimeoutSecs > 0 {
		cmdInstance.SetIdleTimeout(time.Duration(cmdInfo.Command.TimeoutSecs) * time.Second)
	}
	cmdInstance.SetRetryOnFailure(cmdInfo.Command.RetryOnFailure)
	cmdInstance.SetFailureMetadataTags(cmdInfo.Command.FailureMetadataTags)

	cmdInstance.SetJasperManager(e.jasperManager)

	e.taskConfig.Expansions = *e.expansions
	e.taskConfig.WorkDir = e.workDir

	e.logger.Infof("Executing command: %s", cmdInfo.Command.Command)
	return cmdInstance.Execute(ctx, e.communicator, e.loggerProducer, e.taskConfig)
}

// expandString expands variables in a string
func (e *LocalExecutor) expandString(s string) string {
	if e.expansions == nil {
		return s
	}
	result := s
	expansions := e.expansions.Map()
	for key, value := range expansions {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	for key, value := range e.debugState.CustomVars {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	return result
}

// RunAll executes all steps in a task
func (e *LocalExecutor) RunAll(ctx context.Context) error {
	for e.debugState.HasMoreSteps() {
		if err := e.StepNext(ctx); err != nil {
			e.logger.Warningf("Step %d failed, continuing", e.debugState.CurrentStepIndex-1)
			return nil
		}
	}
	return nil
}

// GetDebugState returns the current debug state
func (e *LocalExecutor) GetDebugState() *DebugState {
	return e.debugState
}

// createBlockDeps creates BlockExecutorDeps for use with RunCommandsInBlock
func (e *LocalExecutor) createBlockDeps() executor.BlockExecutorDeps {
	return executor.BlockExecutorDeps{
		JasperManager:         e.jasperManager,
		TaskLogger:            e.logger,
		ExecLogger:            e.logger,
		TaskConfig:            e.taskConfig,
		Tracer:                nil,
		StartTimeoutWatcher:   nil,
		SetHeartbeatTimeout:   nil,
		ResetHeartbeatTimeout: nil,
		HandlePanic:           e.handleLocalPanic,
		RunCommandOrFunc:      e.runCommandWithTracking,
	}
}

// handleLocalPanic handles panics that occur during command execution
func (e *LocalExecutor) handleLocalPanic(panicErr error, originalErr error, op string) error {
	e.logger.Errorf("Panic in %s: %v", op, panicErr)
	if originalErr != nil {
		e.logger.Errorf("Original error: %v", originalErr)
		return errors.Wrapf(panicErr, "panic during %s (original error: %v)", op, originalErr)
	}
	return errors.Wrapf(panicErr, "panic during %s", op)
}

// runCommandWithTracking executes commands with step tracking for debug mode
func (e *LocalExecutor) runCommandWithTracking(
	ctx context.Context,
	_ model.PluginCommandConf,
	cmds []command.Command,
	_ command.BlockType,
	canFailTask bool,
) error {
	for _, cmd := range cmds {
		cmd.SetJasperManager(e.jasperManager)
		err := cmd.Execute(ctx, e.communicator, e.loggerProducer, e.taskConfig)
		if err != nil {
			e.logger.Errorf("Command failed: %v", err)
			if canFailTask {
				return err
			}
			e.logger.Warningf("Continuing after non-fatal error: %v", err)
		}
	}

	e.debugState.CurrentStepIndex++
	return nil
}

// PrepareTask prepares a task for execution by creating command blocks
func (e *LocalExecutor) PrepareTask(taskName string) error {
	if e.project == nil {
		return errors.New("project not loaded")
	}

	task := e.project.FindProjectTask(taskName)
	if task == nil {
		return errors.Errorf("task '%s' not found in project", taskName)
	}

	e.debugState.SelectedTask = taskName
	e.logger.Infof("Preparing task: %s", taskName)

	mainBlock := executorBlock{
		blockType: command.MainTaskBlock,
		commands: &model.YAMLCommandSet{
			MultiCommand: task.Commands,
		},
		startIndex:  0,
		canFailTask: true,
	}

	e.commandBlocks = []executorBlock{mainBlock}
	if err := e.rebuildCommandList(); err != nil {
		return errors.Wrap(err, "rebuilding command list")
	}

	if len(e.commandBlocks) > 0 && len(e.debugState.CommandList) > 0 {
		e.commandBlocks[0].endIndex = len(e.debugState.CommandList) - 1
	}
	e.logger.Infof("Task prepared with %d commands in %d blocks",
		len(e.debugState.CommandList), len(e.commandBlocks))

	return nil
}

// rebuildCommandList rebuilds the flattened command list from command blocks
func (e *LocalExecutor) rebuildCommandList() error {
	e.debugState.CommandList = []CommandInfo{}
	globalIndex := 0

	for blockIdx, block := range e.commandBlocks {
		commands := block.commands.List()

		for cmdIdx, cmd := range commands {
			blockInfo := command.BlockInfo{
				Block:     block.blockType,
				CmdNum:    cmdIdx + 1,
				TotalCmds: len(commands),
			}

			renderedCmds, err := command.Render(cmd, e.project, blockInfo)
			if err != nil {
				e.logger.Warningf("Failed to render command '%s': %v", cmd.Command, err)
				e.debugState.CommandList = append(e.debugState.CommandList, CommandInfo{
					Index:        globalIndex,
					Command:      cmd,
					DisplayName:  cmd.GetDisplayName(),
					IsFunction:   cmd.Function != "",
					FunctionName: cmd.Function,
					BlockType:    block.blockType,
					BlockIndex:   blockIdx,
					BlockCmdNum:  cmdIdx + 1,
				})
				globalIndex++
				continue
			}

			for _, rcmd := range renderedCmds {
				displayName := rcmd.FullDisplayName()
				if displayName == "" {
					displayName = cmd.GetDisplayName()
				}

				e.debugState.CommandList = append(e.debugState.CommandList, CommandInfo{
					Index:        globalIndex,
					Command:      cmd,
					DisplayName:  displayName,
					IsFunction:   cmd.Function != "",
					FunctionName: cmd.Function,
					BlockType:    block.blockType,
					BlockIndex:   blockIdx,
					BlockCmdNum:  cmdIdx + 1,
				})
				globalIndex++
			}
		}
	}

	e.logger.Infof("Rebuilt command list with %d total commands", len(e.debugState.CommandList))
	return nil
}
