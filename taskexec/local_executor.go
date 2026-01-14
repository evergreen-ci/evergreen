package taskexec

import (
	"io/ioutil"
	"os"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
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
	// Set up logging
	var logger grip.Journaler
	if opts.LogFile != "" {
		sender, err := send.MakeFileLogger(opts.LogFile)
		if err != nil {
			return nil, errors.Wrap(err, "creating file logger")
		}
		logger = grip.NewJournaler("evergreen-local-file")
		grip.SetSender(sender)
	} else {
		logger = grip.NewJournaler("evergreen-local")
	}

	// Initialize expansions
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

// GetDebugState returns the current debug state
func (e *LocalExecutor) GetDebugState() *DebugState {
	return e.debugState
}
