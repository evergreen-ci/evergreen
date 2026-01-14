package taskexec

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// LocalExecutor implements task execution for local YAML files
type LocalExecutor struct {
	project       *model.Project
	parserProject *model.ParserProject
	workDir       string
	expansions    *util.Expansions
	logger        grip.Journaler
	debugState    *DebugState
	commandList   []model.PluginCommandConf
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

	return &LocalExecutor{
		workDir:    opts.WorkingDir,
		expansions: &expansions,
		logger:     logger,
		debugState: NewDebugState(),
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

// executeCommand executes a single command using a simplified approach
func (e *LocalExecutor) executeCommand(ctx context.Context, cmd model.PluginCommandConf) error {
	// For now, we'll only handle shell.exec commands directly
	// This is a simplified implementation that doesn't use the agent's command registry
	if cmd.Command != "shell.exec" {
		e.logger.Infof("Skipping non-shell command: %s", cmd.Command)
		return nil
	}

	scriptParam, ok := cmd.Params["script"]
	if !ok {
		return errors.New("no script parameter found")
	}

	script, ok := scriptParam.(string)
	if !ok {
		return errors.New("script parameter is not a string")
	}

	script = e.expandString(script)
	workDir := e.workDir
	if workDirParam, ok := cmd.Params["working_dir"]; ok {
		if wd, ok := workDirParam.(string); ok {
			workDir = e.expandString(wd)
		}
	}

	shell := "sh"
	if shellParam, ok := cmd.Params["shell"]; ok {
		if s, ok := shellParam.(string); ok {
			shell = s
		}
	}

	var execCmd *exec.Cmd
	if shell == "bash" {
		execCmd = exec.CommandContext(ctx, "bash", "-c", script)
	} else {
		execCmd = exec.CommandContext(ctx, "sh", "-c", script)
	}

	execCmd.Dir = workDir

	execCmd.Env = os.Environ()
	if envParams, ok := cmd.Params["env"]; ok {
		if envMap, ok := envParams.(map[string]interface{}); ok {
			for k, v := range envMap {
				if vStr, ok := v.(string); ok {
					execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, e.expandString(vStr)))
				}
			}
		}
	}

	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	e.logger.Infof("Executing shell command in %s", workDir)
	err := execCmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	if output != "" {
		e.logger.Info(output)
	}

	return err
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
