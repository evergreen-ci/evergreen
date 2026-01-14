package taskexec

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
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

// LocalExecutor implements task execution for local YAML files
type LocalExecutor struct {
	project       *model.Project
	parserProject *model.ParserProject
	workDir       string
	expansions    *util.Expansions
	logger        grip.Journaler
	debugState    *DebugState
	commandList   []model.PluginCommandConf

	// Dependencies for command execution
	communicator   client.Communicator
	loggerProducer client.LoggerProducer
	taskConfig     *internal.TaskConfig
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

	comm := client.NewMock("http://localhost")

	loggerProducer := &localLoggerProducer{
		logger: logger,
	}

	taskConfig := &internal.TaskConfig{
		Expansions: expansions,
		WorkDir:    opts.WorkingDir,
		Task: task.Task{
			Id:     "local-task",
			Secret: "local-secret",
		},
	}

	return &LocalExecutor{
		workDir:        opts.WorkingDir,
		expansions:     &expansions,
		logger:         logger,
		debugState:     NewDebugState(),
		communicator:   comm,
		loggerProducer: loggerProducer,
		taskConfig:     taskConfig,
	}, nil
}

// LoadProject loads and parses an Evergreen project configuration from a file
func (e *LocalExecutor) LoadProject(configPath string) (*model.Project, error) {
	e.logger.Infof("Loading project from: %s", configPath)

	yamlBytes, err := ioutil.ReadFile(configPath)
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

// stepNext executes the current step and advances to the next
func (e *LocalExecutor) stepNext(ctx context.Context) error {
	if e.debugState.CurrentStepIndex >= len(e.commandList) {
		return errors.New("no more steps to execute")
	}

	cmd := e.commandList[e.debugState.CurrentStepIndex]
	cmdInfo := e.debugState.CommandList[e.debugState.CurrentStepIndex]

	e.logger.Infof("Executing step %d: %s", e.debugState.CurrentStepIndex, cmdInfo.DisplayName)

	err := e.executeCommand(ctx, cmd)
	if err != nil {
		e.logger.Errorf("Step %d failed: %v", e.debugState.CurrentStepIndex, err)
	} else {
		e.logger.Infof("Step %d completed successfully", e.debugState.CurrentStepIndex)
	}

	e.debugState.CurrentStepIndex++

	return nil
}

// executeCommand executes a single command using the agent command registry
func (e *LocalExecutor) executeCommand(ctx context.Context, cmd model.PluginCommandConf) error {
	factory, ok := command.GetCommandFactory(cmd.Command)
	if !ok {
		e.logger.Warningf("Command '%s' not found in registry, skipping", cmd.Command)
		return errors.Errorf("command '%s' is not registered", cmd.Command)
	}
	cmdInstance := factory()
	if err := cmdInstance.ParseParams(cmd.Params); err != nil {
		return errors.Wrapf(err, "parsing parameters for command '%s'", cmd.Command)
	}

	cmdInstance.SetType(cmd.GetType(e.project))
	cmdInstance.SetFullDisplayName(cmd.DisplayName)
	if cmd.TimeoutSecs > 0 {
		cmdInstance.SetIdleTimeout(time.Duration(cmd.TimeoutSecs) * time.Second)
	}
	cmdInstance.SetRetryOnFailure(cmd.RetryOnFailure)
	cmdInstance.SetFailureMetadataTags(cmd.FailureMetadataTags)

	e.taskConfig.Expansions = *e.expansions
	e.taskConfig.WorkDir = e.workDir

	e.logger.Infof("Executing command: %s", cmd.Command)
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

// RunAll executes all remaining steps
func (e *LocalExecutor) RunAll(ctx context.Context) error {
	for e.debugState.hasMoreSteps() {
		if err := e.stepNext(ctx); err != nil {
			e.logger.Warningf("Step %d failed, continuing", e.debugState.CurrentStepIndex-1)
		}
	}
	return nil
}

// GetDebugState returns the current debug state
func (e *LocalExecutor) GetDebugState() *DebugState {
	return e.debugState
}
