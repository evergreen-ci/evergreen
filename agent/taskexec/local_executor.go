package taskexec

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/executor"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

var noOpCommands = map[string]bool{
	evergreen.HostCreateCommandName:         true,
	"host.list":                             true,
	"generate.tasks":                        true,
	"downstream_expansions.set":             true,
	evergreen.AttachXUnitResultsCommandName: true,
	evergreen.AttachResultsCommandName:      true,
	evergreen.AttachArtifactsCommandName:    true,
	"papertrail.trace":                      true,
	"keyval.inc":                            true,
	"perf.send":                             true,
	"s3.put":                                true,
	"s3Copy.copy":                           true,
}

// mockSecret is required to make agent request formation validation pass but it's not used in
// debug sessions so we hard code it to a mock
const mockSecret = "mock_secret"

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
	opts           LocalExecutorOptions
}

// LocalExecutorOptions contains configuration for the local executor
type LocalExecutorOptions struct {
	WorkingDir string
	Expansions map[string]string
	ServerURL  string
	TaskID     string
	OAuthToken string
}

// NewLocalExecutor creates a new local task executor
func NewLocalExecutor(ctx context.Context, opts LocalExecutorOptions) (*LocalExecutor, error) {
	logger := grip.NewJournaler("evergreen-local")

	expansions := util.Expansions{}
	if opts.WorkingDir != "" {
		expansions.Put("workdir", opts.WorkingDir)
	}
	for k, v := range opts.Expansions {
		expansions.Put(k, v)
	}

	comm := client.NewDebugCommunicator(opts.ServerURL, opts.OAuthToken)
	logger.Infof("Using backend communication with server: %s", opts.ServerURL)

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

	localExecutor := &LocalExecutor{
		workDir:        opts.WorkingDir,
		expansions:     &expansions,
		logger:         logger,
		debugState:     NewDebugState(),
		communicator:   comm,
		loggerProducer: loggerProducer,
		taskConfig:     taskConfig,
		jasperManager:  jasperManager,
		commandBlocks:  []executorBlock{},
		opts:           opts,
	}

	if err := localExecutor.fetchTaskConfig(ctx, opts); err != nil {
		return nil, errors.Wrap(err, "fetching task config")
	}

	return localExecutor, nil
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

// RunUntil executes steps up until the given input index.
func (e *LocalExecutor) RunUntil(ctx context.Context, untilIndex int) error {
	if len(e.commandBlocks) == 0 {
		return nil
	}
	maxIndex := e.commandBlocks[len(e.commandBlocks)-1].endIndex
	if untilIndex >= maxIndex {
		e.logger.Warningf("Running until index %d out of range, falling back to %d", untilIndex, maxIndex)
		untilIndex = maxIndex
	}

	for e.debugState.CurrentStepIndex <= untilIndex {
		if err := e.StepNext(ctx); err != nil {
			e.logger.Errorf("Step %d failed: %v", e.debugState.CurrentStepIndex, err)
			return err
		}
		if e.debugState.CurrentStepIndex > untilIndex {
			break
		}
	}
	return nil
}

// StepNext executes the current step and advances to the next
func (e *LocalExecutor) StepNext(ctx context.Context) error {
	if !e.debugState.HasMoreSteps() {
		return errors.New("no more steps to execute")
	}
	return e.stepNext(ctx)
}

// stepNext executes the current step using RunCommandsInBlock.
// This implementation uses Direct Block Targeting to avoid unnecessary iteration
// by leveraging the metadata in CommandList to directly target the specific
// block and command that needs to be executed.
func (e *LocalExecutor) stepNext(ctx context.Context) error {
	if !e.debugState.HasMoreSteps() {
		return errors.New("no more steps to execute")
	}

	targetCmd := e.debugState.CommandList[e.debugState.CurrentStepIndex]
	targetBlockIdx := targetCmd.BlockIndex

	if targetBlockIdx >= len(e.commandBlocks) {
		return errors.Errorf("invalid block index %d", targetBlockIdx)
	}

	// Only process the specific block (i.e. pre, main, post) containing our target command
	block := e.commandBlocks[targetBlockIdx]
	cmdBlock := executor.CommandBlock{
		Block:    block.blockType,
		Commands: block.commands,
	}
	deps := e.createBlockDeps()

	// Track which command within the block we're currently processing
	// This counter matches the BlockCmdNum field (1-indexed)
	currentCmdInBlock := 0

	// Use the pre-calculated startIndex for this block
	// This gives us the global command index offset for commands before this block
	globalCmdIndex := block.startIndex

	// Flag to track if we've executed our target command
	executed := false

	// Override the RunCommandOrFunc callback to intercept and execute
	// only the specific command we're targeting
	deps.RunCommandOrFunc = func(
		ctx context.Context,
		commandInfo model.PluginCommandConf,
		cmds []command.Command,
		blockType command.BlockType,
		canFailTask bool,
	) error {
		currentCmdInBlock++

		// Skip commands until we reach the target command in this block
		// BlockCmdNum is 1-indexed, so we compare directly
		if currentCmdInBlock != targetCmd.BlockCmdNum {
			// Update global index for skipped commands
			globalCmdIndex += len(cmds)
			return nil
		}

		// We've found the right command in the block
		// Now determine which specific expanded command to execute
		targetIndexWithinExpanded := e.debugState.CurrentStepIndex - globalCmdIndex

		if targetIndexWithinExpanded >= 0 && targetIndexWithinExpanded < len(cmds) {
			cmd := cmds[targetIndexWithinExpanded]
			cmd.SetJasperManager(e.jasperManager)

			err := cmd.Execute(ctx, e.communicator, e.loggerProducer, e.taskConfig)
			if err != nil {
				e.logger.Errorf("Step %d failed: %v", e.debugState.CurrentStepIndex, err)
				return err
			}

			e.logger.Infof("Step %d completed successfully", e.debugState.CurrentStepIndex)
			e.debugState.CurrentStepIndex++
			executed = true
		}
		return nil
	}
	if err := executor.RunCommandsInBlock(ctx, deps, cmdBlock); err != nil {
		return err
	}
	if !executed {
		return errors.Errorf("failed to execute step %d", e.debugState.CurrentStepIndex)
	}

	return nil
}

// RunAll executes all steps in a task
func (e *LocalExecutor) RunAll(ctx context.Context) error {
	for e.debugState.HasMoreSteps() {
		if err := e.StepNext(ctx); err != nil {
			e.logger.Warningf("Step %d failed, continuing", e.debugState.CurrentStepIndex-1)
			return err
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
		e.debugState.CurrentStepIndex++
		if e.isLocalNoOpCommand(cmd) {
			e.handleNoOpCommand(cmd)
			continue
		}

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
	return nil
}

// isLocalNoOpCommand checks if a command should be no-op in local execution
func (e *LocalExecutor) isLocalNoOpCommand(cmd command.Command) bool {
	return noOpCommands[cmd.Name()]
}

// handleNoOpCommand logs a message for commands that are no-op in local execution
func (e *LocalExecutor) handleNoOpCommand(cmd command.Command) {
	messages := map[string]string{
		evergreen.HostCreateCommandName:         "host.create: Skipping - dynamic host creation is not supported in local execution",
		"host.list":                             "host.list: Skipping - host listing is not supported in local execution",
		"generate.tasks":                        "generate.tasks: Skipping - dynamic task generation is not supported in local execution",
		"downstream_expansions.set":             "downstream_expansions.set: Skipping - downstream expansions are not available in local execution",
		evergreen.AttachXUnitResultsCommandName: "attach.xunit_results: Skipping - test result attachment is not supported in local execution",
		evergreen.AttachResultsCommandName:      "attach.results: Skipping - result attachment is not supported in local execution",
		evergreen.AttachArtifactsCommandName:    "attach.artifacts: Skipping - artifact attachment is not supported in local execution",
		"papertrail.trace":                      "papertrail.trace: Skipping - papertrail tracing is not available in local execution",
		"keyval.inc":                            "keyval.inc: Skipping - key-value increment operations are not supported in local execution",
		"perf.send":                             "perf.send: Skipping - performance metrics submission is not supported in local execution",
		"s3.put":                                "s3.put: Skipping - S3 upload operations are not supported in local execution",
		"s3Copy.copy":                           "s3Copy.copy: Skipping - S3 copy operations are not supported in local execution",
	}
	message, ok := messages[cmd.Name()]
	if !ok {
		message = cmd.Name() + ": Skipping - command is not supported in local execution"
	}
	e.logger.Infof(message)
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

	// Build command blocks array with pre, main, and post blocks
	var blocks []executorBlock

	if e.project.Pre != nil && len(e.project.Pre.List()) > 0 {
		blocks = append(blocks, executorBlock{
			blockType:   command.PreBlock,
			commands:    e.project.Pre,
			canFailTask: e.project.PreErrorFailsTask,
		})
	}
	blocks = append(blocks, executorBlock{
		blockType: command.MainTaskBlock,
		commands: &model.YAMLCommandSet{
			MultiCommand: task.Commands,
		},
		canFailTask: true,
	})
	if e.project.Post != nil && len(e.project.Post.List()) > 0 {
		blocks = append(blocks, executorBlock{
			blockType:   command.PostBlock,
			commands:    e.project.Post,
			canFailTask: e.project.PostErrorFailsTask,
		})
	}
	e.commandBlocks = blocks
	if err := e.rebuildCommandList(); err != nil {
		return errors.Wrap(err, "rebuilding command list")
	}

	// This variable tracks the starting command index for each execution block,
	// so we can index it quickly later via the startIndex and endIndex fields.
	currentIdx := 0
	for i := range e.commandBlocks {
		e.commandBlocks[i].startIndex = currentIdx
		blockCommands := 0
		for _, cmd := range e.debugState.CommandList {
			if cmd.BlockIndex == i {
				blockCommands++
			}
		}
		if blockCommands > 0 {
			e.commandBlocks[i].endIndex = currentIdx + blockCommands - 1
			currentIdx += blockCommands
		} else {
			e.commandBlocks[i].endIndex = currentIdx - 1
		}
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

func (e *LocalExecutor) fetchTaskConfig(ctx context.Context, opts LocalExecutorOptions) error {
	taskData := client.TaskData{
		ID:                 opts.TaskID,
		OverrideValidation: true,
	}
	if opts.TaskID == "" {
		e.createSyntheticTask("local_task")
		return nil
	}

	projectRef, err := e.communicator.GetProjectRef(ctx, taskData)
	if err != nil {
		return errors.Wrap(err, "getting project ref")
	}
	if projectRef == nil {
		return errors.New("project ref not found")
	}
	e.taskConfig.ProjectRef = *projectRef

	tsk, err := e.communicator.GetTask(ctx, taskData)
	if err != nil {
		e.logger.Errorf("Failed to fetch task from backend: %v", err)
		return err
	}
	if tsk == nil {
		return errors.Errorf("task '%s' not found", opts.TaskID)
	}
	tsk.Secret = mockSecret
	e.taskConfig.Task = *tsk

	project, err := e.communicator.GetProject(ctx, taskData)
	if err != nil {
		return errors.Wrapf(err, "getting parser project '%s'", tsk.Revision)
	}
	if project == nil {
		return errors.Errorf("project '%s' not found", tsk.Revision)
	}
	e.taskConfig.Project = *project
	e.project = project

	expansionsAndVars, err := e.communicator.GetExpansionsAndVars(ctx, taskData)
	if err != nil {
		return errors.Wrapf(err, "getting expansions and vars from task '%s'", tsk.Id)
	}
	if expansionsAndVars == nil {
		return nil
	}
	for k, v := range expansionsAndVars.Expansions {
		e.taskConfig.Expansions.Put(k, v)
	}
	for k, v := range expansionsAndVars.Vars {
		e.taskConfig.Expansions.Put(k, v)
	}
	e.taskConfig.NewExpansions = agentutil.NewDynamicExpansions(expansionsAndVars.Expansions)
	return nil
}

func (e *LocalExecutor) createSyntheticTask(taskName string) {
	taskID := fmt.Sprintf("local_%s_%d", taskName, time.Now().Unix())
	e.taskConfig.Task = task.Task{
		Id:          taskID,
		Project:     e.taskConfig.ProjectRef.Identifier,
		DisplayName: taskName,
	}
}
