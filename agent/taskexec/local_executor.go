package taskexec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/executor"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

var noOpCommands = map[string]string{
	evergreen.HostCreateCommandName:         "dynamic host creation is not supported in local execution",
	"host.list":                             "host listing is not supported in local execution",
	"generate.tasks":                        "dynamic task generation is not supported in local execution",
	"downstream_expansions.set":             "downstream expansions are not available in local execution",
	evergreen.AttachXUnitResultsCommandName: "test result attachment is not supported in local execution",
	evergreen.AttachResultsCommandName:      "result attachment is not supported in local execution",
	"gotest.parse_files":                    "result attachment is not supported in local execution",
	evergreen.AttachArtifactsCommandName:    "artifact attachment is not supported in local execution",
	"papertrail.trace":                      "papertrail tracing is not available in local execution",
	"keyval.inc":                            "key-value increment operations are not supported in local execution",
	"perf.send":                             "performance metrics submission is not supported in local execution",
	"s3.put":                                "S3 upload operations are not supported in local execution",
	"s3Copy.copy":                           "S3 copy operations are not supported in local execution",
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
	project           *model.Project
	parserProject     *model.ParserProject
	workDir           string
	expansions        *util.Expansions
	logger            grip.Journaler
	debugState        *DebugState
	commandBlocks     []executorBlock
	communicator      client.Communicator
	loggerProducer    client.LoggerProducer
	taskConfig        *internal.TaskConfig
	jasperManager     jasper.Manager
	opts              LocalExecutorOptions
	streamWriter      *streamWriter
	logManager        *logManager
	streamingProducer *streamingLoggerProducer
}

// LocalExecutorOptions contains configuration for the local executor
type LocalExecutorOptions struct {
	WorkingDir   string
	Expansions   map[string]string
	ServerURL    string
	TaskID       string
	OAuthToken   string
	SpawnHostID  string
	LocalModules map[string]string
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

	comm := client.NewDebugCommunicator(opts.ServerURL, opts.OAuthToken, opts.SpawnHostID)
	logger.Infof(ctx, "Using backend communication with server: %s", opts.ServerURL)

	loggerProducer := &localLoggerProducer{
		logger: logger,
	}

	taskConfig := &internal.TaskConfig{
		Expansions:            expansions,
		NewExpansions:         agentutil.NewDynamicExpansions(expansions),
		WorkDir:               opts.WorkingDir,
		AssumeRoleInformation: map[string]internal.AssumeRoleInformation{},
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

	if opts.TaskID != "" {
		if err := localExecutor.fetchTaskConfig(ctx, opts); err != nil {
			return nil, errors.Wrap(err, "fetching task config")
		}
	}

	return localExecutor, nil
}

// LoadProject loads and parses an Evergreen project configuration from a file
func (e *LocalExecutor) LoadProject(configPath string) (*model.Project, error) {
	e.logger.Infof(context.Background(), "Loading project from: %s", configPath)

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "resolving absolute path for '%s'", configPath)
	}

	yamlBytes, err := os.ReadFile(absPath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading config file '%s'", absPath)
	}

	project := &model.Project{}
	opts := &model.GetProjectOpts{
		ReadFileFrom:    model.ReadFromLocal,
		LocalIncludeDir: filepath.Dir(absPath),
		LocalModules:    e.opts.LocalModules,
	}

	pp, err := model.LoadProjectInto(context.Background(), yamlBytes, opts, "", project)
	if err != nil {
		return nil, errors.Wrap(err, "loading project")
	}
	e.parserProject = pp
	e.project = project

	if e.taskConfig != nil {
		e.taskConfig.Project = *project
	}

	e.logger.Infof(context.Background(), "Loaded project with %d tasks and %d build variants",
		len(project.Tasks), len(project.BuildVariants))

	return project, nil
}

// ReloadProject reloads the project configuration from a file while preserving
// the current debug session state. If a task is currently selected, it
// re-prepares the task with the new config.
func (e *LocalExecutor) ReloadProject(ctx context.Context, configPath string) (*model.Project, error) {
	savedIndex := e.debugState.CurrentStepIndex
	savedVars := make(map[string]string, len(e.debugState.CustomVars))
	for k, v := range e.debugState.CustomVars {
		savedVars[k] = v
	}
	savedHistory := make([]executionRecord, len(e.debugState.ExecutionHistory))
	copy(savedHistory, e.debugState.ExecutionHistory)

	project, err := e.LoadProject(configPath)
	if err != nil {
		return nil, errors.Wrap(err, "loading project")
	}

	if e.debugState.SelectedTask != "" {
		if err := e.PrepareTask(ctx, e.debugState.SelectedTask, e.debugState.SelectedVariant); err != nil {
			return nil, errors.Wrap(err, "re-preparing task after reload")
		}
	}

	e.debugState.CustomVars = savedVars
	e.debugState.ExecutionHistory = savedHistory
	if savedIndex > len(e.debugState.CommandList) {
		savedIndex = len(e.debugState.CommandList)
	}
	e.debugState.CurrentStepIndex = savedIndex

	for k, v := range savedVars {
		e.expansions.Put(k, v)
	}

	e.logger.Infof(ctx, "Reloaded project, preserved step index at %d", e.debugState.CurrentStepIndex)

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
	e.logger.Infof(context.Background(), "Working directory set to: %s", path)

	return nil
}

// RunUntil executes steps up until the given input index.
func (e *LocalExecutor) RunUntil(ctx context.Context, untilIndex int) error {
	if len(e.commandBlocks) == 0 {
		return errors.New("no commands available, please ensure a task has been selected")
	}
	maxIndex := e.commandBlocks[len(e.commandBlocks)-1].endIndex
	if untilIndex >= maxIndex {
		e.logger.Warningf(ctx, "Running until step out of range, falling back to %s", e.debugState.CommandList[maxIndex].FullStepNumber())
		untilIndex = maxIndex
	}

	for e.debugState.CurrentStepIndex <= untilIndex {
		if err := e.StepNext(ctx); err != nil {
			e.logger.Errorf(ctx, "Step %s failed: %v", e.debugState.CommandList[e.debugState.CurrentStepIndex].FullStepNumber(), err)
			return err
		}
		if e.debugState.CurrentStepIndex > untilIndex {
			break
		}
	}
	return nil
}

// JumpTo moves to the specified step without executing
func (e *LocalExecutor) JumpTo(index int) error {
	if len(e.commandBlocks) == 0 {
		return errors.New("no commands available, please ensure a task has been selected")
	}
	maxIndex := e.commandBlocks[len(e.commandBlocks)-1].endIndex
	if index >= maxIndex || index < 0 {
		return errors.Errorf("invalid step index %d (valid range: 0-%d)", index, maxIndex)
	}
	e.debugState.CurrentStepIndex = index
	e.logger.Infof(context.Background(), "Jumped to step %s", e.debugState.CommandList[index].FullStepNumber())
	return nil
}

// SetVariable sets a custom variable.
func (e *LocalExecutor) SetVariable(key, value string) {
	e.debugState.CustomVars[key] = value
	e.expansions.Put(key, value)
	e.logger.Infof(context.Background(), "Set variable %s=%s", key, value)
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

	stepIndex := e.debugState.CurrentStepIndex
	targetCmd := e.debugState.CommandList[stepIndex]
	targetBlockIdx := targetCmd.BlockIndex

	if targetBlockIdx >= len(e.commandBlocks) {
		return errors.Errorf("invalid block index %d", targetBlockIdx)
	}

	if e.streamWriter != nil {
		e.streamWriter.SetStep(stepIndex)
		e.streamWriter.SetStepNumber(targetCmd.FullStepNumber())
	}

	startTime := time.Now()

	_, isNoOp := noOpCommands[targetCmd.CommandName]
	if isNoOp {
		noOpMsg := e.getNoOpMessage(targetCmd.CommandName)
		if e.streamWriter != nil {
			e.streamWriter.WriteChannelMessage(ExecChannel, noOpMsg)
		}
		e.logger.Infof(ctx, noOpMsg)
		e.debugState.CurrentStepIndex++
		durationMs := time.Since(startTime).Milliseconds()
		record := executionRecord{
			stepIndex:  stepIndex,
			success:    true,
			durationMs: durationMs,
		}
		e.debugState.addExecutionRecord(record)

		if e.streamWriter != nil {
			nextStep := e.debugState.CurrentStepIndex
			e.streamWriter.WriteDone(true, durationMs, nextStep, "")
		}

		if e.logManager != nil {
			lf := e.logManager.LogFile()
			stepNum := targetCmd.FullStepNumber()
			lf.WriteStepStart(stepNum, targetCmd.DisplayName, string(targetCmd.BlockType))
			lf.WriteLogLine(stepNum, noOpMsg)
			lf.WriteStepEnd(stepNum, true, getDurationStr(startTime))
		}
		return nil
	}

	if e.logManager != nil {
		lf := e.logManager.LogFile()
		lf.WriteStepStart(targetCmd.FullStepNumber(), targetCmd.DisplayName, string(targetCmd.BlockType))
	}

	if e.streamWriter != nil {
		msg := fmt.Sprintf("Running command %s.", targetCmd.DisplayName)
		e.streamWriter.WriteChannelMessage(ExecChannel, msg)
	}

	// Only process the specific block (i.e. pre, main, post) containing our target command
	block := e.commandBlocks[targetBlockIdx]
	cmdBlock := executor.CommandBlock{
		Block:       block.blockType,
		Commands:    block.commands,
		CanFailTask: block.canFailTask,
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

			cleanup, err := e.taskConfig.ApplyFunctionVarsToExpansions(commandInfo.Vars, cmd.Name())
			if err != nil {
				return err
			}
			defer cleanup()

			err = cmd.Execute(ctx, e.communicator, e.loggerProducer, e.taskConfig)
			if err != nil {
				e.logger.Errorf(ctx, "Step %s failed: %v", targetCmd.FullStepNumber(), err)
				if canFailTask {
					return err
				}
				blockName := executor.BlockToLegacyName(blockType)
				e.logger.Warningf(ctx, "Continuing after non-fatal error in %s block: %v", blockName, err)
			} else {
				e.logger.Infof(ctx, "Step %s completed successfully", targetCmd.FullStepNumber())
			}
			e.debugState.CurrentStepIndex++
			executed = true
		}
		return nil
	}
	record := executionRecord{
		stepIndex: stepIndex,
		success:   true,
	}
	if err := executor.RunCommandsInBlock(ctx, deps, cmdBlock); err != nil {
		record.success = false
		durationMs := time.Since(startTime).Milliseconds()
		record.durationMs = durationMs
		record.errMsg = err.Error()
		e.debugState.addExecutionRecord(record)

		if e.streamWriter != nil {
			e.streamWriter.WriteDone(false, durationMs, e.debugState.CurrentStepIndex, err.Error())
		}
		if e.logManager != nil {
			lf := e.logManager.LogFile()
			lf.WriteStepEnd(targetCmd.FullStepNumber(), false, getDurationStr(startTime))
		}
		return err
	}
	if !executed {
		return errors.Errorf("failed to execute step %s", targetCmd.FullStepNumber())
	}
	durationMs := time.Since(startTime).Milliseconds()
	record.durationMs = durationMs
	e.debugState.addExecutionRecord(record)

	if e.streamWriter != nil {
		e.streamWriter.WriteDone(true, durationMs, e.debugState.CurrentStepIndex, "")
	}
	if e.logManager != nil {
		lf := e.logManager.LogFile()
		lf.WriteStepEnd(targetCmd.FullStepNumber(), true, getDurationStr(startTime))
	}
	return nil
}

func (e *LocalExecutor) getNoOpMessage(cmdName string) string {
	if reason, ok := noOpCommands[cmdName]; ok {
		return fmt.Sprintf("%s: Skipping - %s", cmdName, reason)
	}
	return cmdName + ": Skipping - command is not supported in local execution"
}

// RunAll executes all steps in a task
func (e *LocalExecutor) RunAll(ctx context.Context) error {
	for e.debugState.HasMoreSteps() {
		if err := e.StepNext(ctx); err != nil {
			e.logger.Warningf(ctx, "Step failed, continuing")
			return err
		}
	}
	return nil
}

// GetDebugState returns the current debug state
func (e *LocalExecutor) GetDebugState() *DebugState {
	return e.debugState
}

// GetProject returns the loaded project.
func (e *LocalExecutor) GetProject() *model.Project {
	return e.project
}

// GetFetchedTaskName returns the display name of the task fetched from
// the server during initialization.
func (e *LocalExecutor) GetFetchedTaskName() string {
	return e.taskConfig.Task.DisplayName
}

// GetFetchedBuildVariant returns the build variant of the task fetched
// from the server during initialization.
func (e *LocalExecutor) GetFetchedBuildVariant() string {
	return e.taskConfig.Task.BuildVariant
}

// SetStreamWriter sets the stream writer for streaming output during execution.
// When set, command output is sent as NDJSON to the stream writer.
func (e *LocalExecutor) SetStreamWriter(sw *streamWriter) {
	e.streamWriter = sw
	producer := newStreamingLoggerProducer(sw)

	baseSender := producer.Execution()
	redactedSender := redactor.NewRedactingSender(baseSender, redactor.RedactionOptions{
		Expansions:         e.taskConfig.NewExpansions,
		Redacted:           e.taskConfig.Redacted,
		InternalRedactions: e.taskConfig.InternalRedactions,
		PreloadRedactions:  true,
	})

	producer.setSender(redactedSender)
	e.streamingProducer = producer
	e.loggerProducer = newStreamingLoggerProducerAdapter(producer)
}

// ClearStreamWriter removes the stream writer after execution completes.
// It resets the logger producer back to the default local logger so that
// commands executed outside a streaming session still have a valid
// logger producer to write to.
func (e *LocalExecutor) ClearStreamWriter() {
	if e.streamWriter != nil {
		e.streamWriter.Close()
	}
	e.streamWriter = nil
	e.streamingProducer = nil
	e.loggerProducer = &localLoggerProducer{logger: e.logger}
}

// SetupLogManager initializes local log file management.
func (e *LocalExecutor) SetupLogManager(isSetupPhase bool) error {
	lm, err := newLogManager(isSetupPhase)
	if err != nil {
		return err
	}
	e.logManager = lm
	return nil
}

// CloseLogManager closes the log manager.
func (e *LocalExecutor) CloseLogManager() {
	if e.logManager != nil {
		e.logManager.Close()
		e.logManager = nil
	}
}

// GetLogManager returns the log manager.
func (e *LocalExecutor) GetLogManager() *logManager {
	return e.logManager
}

// streamingLoggerProducerAdapter wraps streamingLoggerProducer to satisfy client.LoggerProducer.
type streamingLoggerProducerAdapter struct {
	producer *streamingLoggerProducer
	logger   grip.Journaler
}

func newStreamingLoggerProducerAdapter(producer *streamingLoggerProducer) *streamingLoggerProducerAdapter {
	return &streamingLoggerProducerAdapter{
		producer: producer,
		logger:   logging.MakeGrip(producer.Task()),
	}
}

func (a *streamingLoggerProducerAdapter) Execution() grip.Journaler { return a.logger }
func (a *streamingLoggerProducerAdapter) Task() grip.Journaler      { return a.logger }
func (a *streamingLoggerProducerAdapter) System() grip.Journaler    { return a.logger }
func (a *streamingLoggerProducerAdapter) Flush(ctx context.Context) error {
	return a.producer.Flush(ctx)
}
func (a *streamingLoggerProducerAdapter) Close() error { return a.producer.Close() }
func (a *streamingLoggerProducerAdapter) Closed() bool { return a.producer.Closed() }

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
	e.logger.Errorf(context.Background(), "Panic in %s: %v", op, panicErr)
	if originalErr != nil {
		e.logger.Errorf(context.Background(), "Original error: %v", originalErr)
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
			e.handleNoOpCommand(ctx, cmd)
			continue
		}

		cmd.SetJasperManager(e.jasperManager)
		err := cmd.Execute(ctx, e.communicator, e.loggerProducer, e.taskConfig)
		if err != nil {
			e.logger.Errorf(ctx, "Command failed: %v", err)
			if canFailTask {
				return err
			}
			e.logger.Warningf(ctx, "Continuing after non-fatal error: %v", err)
		}
	}
	return nil
}

// isLocalNoOpCommand checks if a command should be no-op in local execution
func (e *LocalExecutor) isLocalNoOpCommand(cmd command.Command) bool {
	_, ok := noOpCommands[cmd.Name()]
	return ok
}

// handleNoOpCommand logs a message for commands that are no-op in local execution
func (e *LocalExecutor) handleNoOpCommand(ctx context.Context, cmd command.Command) {
	e.logger.Infof(ctx, e.getNoOpMessage(cmd.Name()))
}

// PrepareTask prepares a task for execution by creating command blocks.
// If variantName is provided, it validates the task exists on that variant
// and applies the variant's expansions.
func (e *LocalExecutor) PrepareTask(ctx context.Context, taskName, variantName string) error {
	if e.project == nil {
		return errors.New("project not loaded")
	}

	task := e.project.FindProjectTask(taskName)
	if task == nil {
		return errors.Errorf("task '%s' not found in project", taskName)
	}

	e.debugState.SelectedTask = taskName
	e.logger.Infof(ctx, "Preparing task: %s", taskName)

	if variantName != "" {
		bv := e.project.FindBuildVariant(variantName)
		if bv == nil {
			return errors.Errorf("build variant '%s' not found in project", variantName)
		}
		if _, err := bv.Get(taskName); err != nil {
			return errors.Wrapf(err, "task '%s' is not defined on build variant '%s'", taskName, variantName)
		}
		e.debugState.SelectedVariant = variantName
		e.taskConfig.Expansions.Put("build_variant", variantName)
		e.taskConfig.Expansions.Update(bv.Expansions)
		e.logger.Infof(ctx, "Applied expansions from build variant: %s", variantName)
	}

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
	e.logger.Infof(ctx, "Task prepared with %d commands in %d blocks",
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
				e.logger.Warningf(context.Background(), "Failed to render command '%s': %v", cmd.Command, err)
				e.debugState.CommandList = append(e.debugState.CommandList, CommandInfo{
					Index:          globalIndex,
					Command:        cmd,
					CommandName:    cmd.Command,
					DisplayName:    cmd.GetDisplayName(),
					IsFunction:     cmd.Function != "",
					FunctionName:   cmd.Function,
					BlockType:      block.blockType,
					BlockIndex:     blockIdx,
					BlockCmdNum:    cmdIdx + 1,
					BlockTotalCmds: len(commands),
				})
				globalIndex++
				continue
			}

			for rcmdIdx, rcmd := range renderedCmds {
				displayName := rcmd.FullDisplayName()
				if displayName == "" {
					displayName = cmd.GetDisplayName()
				}

				info := CommandInfo{
					Index:          globalIndex,
					Command:        cmd,
					CommandName:    rcmd.Name(),
					DisplayName:    displayName,
					IsFunction:     cmd.Function != "",
					FunctionName:   cmd.Function,
					BlockType:      block.blockType,
					BlockIndex:     blockIdx,
					BlockCmdNum:    cmdIdx + 1,
					BlockTotalCmds: len(commands),
				}
				if cmd.Function != "" {
					// rcmdIdx is 0-indexed but step numbers are 1-indexed to match
					// the step number notation used in the task logs
					info.FuncSubCmdNum = rcmdIdx + 1
					info.FuncTotalSubCmds = len(renderedCmds)
				}
				e.debugState.CommandList = append(e.debugState.CommandList, info)
				globalIndex++
			}
		}
	}

	e.logger.Infof(context.Background(), "Rebuilt command list with %d total commands", len(e.debugState.CommandList))
	return nil
}

func (e *LocalExecutor) fetchTaskConfig(ctx context.Context, opts LocalExecutorOptions) error {
	taskData := client.TaskData{
		ID:                 opts.TaskID,
		OverrideValidation: true,
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
		e.logger.Errorf(ctx, "Failed to fetch task from backend: %v", err)
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

	agentutil.AddVariantAndParameterExpansions(expansionsAndVars, e.project, e.taskConfig.Task.BuildVariant)
	for k, v := range expansionsAndVars.Expansions {
		e.taskConfig.Expansions.Put(k, v)
	}
	for k, v := range expansionsAndVars.Vars {
		e.taskConfig.Expansions.Put(k, v)
	}

	if expansionsAndVars.PrivateVars == nil {
		expansionsAndVars.PrivateVars = map[string]bool{}
	}
	for key := range expansionsAndVars.Vars {
		if expansionsAndVars.PrivateVars[key] {
			continue
		}
		for _, pattern := range expansionsAndVars.RedactKeys {
			if strings.Contains(strings.ToLower(key), pattern) {
				expansionsAndVars.PrivateVars[key] = true
				break
			}
		}
	}

	var redacted []string
	for key := range expansionsAndVars.PrivateVars {
		redacted = append(redacted, key)
	}
	e.taskConfig.Redacted = redacted

	internalRedactions := expansionsAndVars.InternalRedactions
	if internalRedactions == nil {
		internalRedactions = map[string]string{}
	}
	e.taskConfig.InternalRedactions = agentutil.NewDynamicExpansions(internalRedactions)

	return nil
}

func getDurationStr(startTime time.Time) string {
	d := time.Since(startTime)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
