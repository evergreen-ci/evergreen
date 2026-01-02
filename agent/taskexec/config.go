// Package taskexec provides simplified task execution infrastructure for local development.
// It provides a lightweight alternative to the full agent infrastructure while
// maintaining compatibility with existing command implementations.
package taskexec

import (
	"sync"

	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// TaskConfig is a simplified version of the agent's TaskConfig
// that can be used for local execution.
type TaskConfig struct {
	Project            model.Project
	Task               task.Task
	TaskName           string
	Expansions         util.Expansions
	NewExpansions      *agentutil.DynamicExpansions
	WorkDir            string
	Timeout            Timeout
	CommandCleanups    []CommandCleanup
	MaxExecTimeoutSecs int
	ModulePaths        map[string]string
	ProjectVars        map[string]string
	Redacted           []string
	RedactKeys         []string
	InternalRedactions *agentutil.DynamicExpansions
	BuildVariant       *model.BuildVariant
	TaskGroup          *model.TaskGroup
	mu                 sync.RWMutex
}

type Timeout struct {
	IdleTimeoutSecs int
	ExecTimeoutSecs int
}

type CommandCleanup struct {
	Command string
	Run     func() error
}

type LocalTaskConfigOptions struct {
	ProjectPath string
	TaskName    string
	VariantName string
	WorkDir     string
	Expansions  map[string]string
	TimeoutSecs int
	RedactKeys  []string
}

func NewLocalTaskConfig(opts LocalTaskConfigOptions) (*TaskConfig, error) {
	if opts.TaskName == "" {
		return nil, errors.New("task name is required")
	}
	if opts.WorkDir == "" {
		return nil, errors.New("working directory is required")
	}

	expansions := util.Expansions{}
	expansions.Put("workdir", opts.WorkDir)
	expansions.Put("task_name", opts.TaskName)
	if opts.VariantName != "" {
		expansions.Put("build_variant", opts.VariantName)
	}

	for k, v := range opts.Expansions {
		expansions.Put(k, v)
	}

	newExpansions := agentutil.NewDynamicExpansions(expansions)

	config := &TaskConfig{
		TaskName:      opts.TaskName,
		WorkDir:       opts.WorkDir,
		Expansions:    expansions,
		NewExpansions: newExpansions,
		ProjectVars:   make(map[string]string),
		ModulePaths:   make(map[string]string),
		RedactKeys:    opts.RedactKeys,
		Timeout: Timeout{
			ExecTimeoutSecs: opts.TimeoutSecs,
		},
		MaxExecTimeoutSecs: opts.TimeoutSecs,
		InternalRedactions: agentutil.NewDynamicExpansions(util.Expansions{}),
	}

	return config, nil
}

func (t *TaskConfig) SetIdleTimeout(timeout int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Timeout.IdleTimeoutSecs = timeout
}

func (t *TaskConfig) SetExecTimeout(timeout int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Timeout.ExecTimeoutSecs = timeout
}

func (t *TaskConfig) GetIdleTimeout() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Timeout.IdleTimeoutSecs
}

func (t *TaskConfig) GetExecTimeout() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Timeout.ExecTimeoutSecs
}

func (t *TaskConfig) AddCommandCleanup(cmd string, run func() error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.CommandCleanups = append(t.CommandCleanups, CommandCleanup{
		Command: cmd,
		Run:     run,
	})
}

func (t *TaskConfig) GetAndClearCommandCleanups() []CommandCleanup {
	t.mu.Lock()
	defer t.mu.Unlock()

	cleanups := t.CommandCleanups
	t.CommandCleanups = nil
	return cleanups
}

func (t *TaskConfig) GetWorkingDirectory() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.WorkDir
}

func (t *TaskConfig) GetTaskName() string {
	if t.Task.Id != "" {
		return t.Task.DisplayName
	}
	return t.TaskName
}

func (t *TaskConfig) GetExpansions() *util.Expansions {
	return &t.Expansions
}

func (t *TaskConfig) GetNewExpansions() *agentutil.DynamicExpansions {
	return t.NewExpansions
}

func (t *TaskConfig) FindProjectTask(taskName string) *model.ProjectTask {
	for _, pt := range t.Project.Tasks {
		if pt.Name == taskName {
			return &pt
		}
	}
	return nil
}

func (t *TaskConfig) SetProject(project *model.Project) error {
	if project == nil {
		return errors.New("project cannot be nil")
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.Project = *project
	return nil
}

func (t *TaskConfig) UpdateExpansions(updates map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for k, v := range updates {
		t.Expansions.Put(k, v)
	}
	if t.NewExpansions != nil {
		t.NewExpansions.Update(t.Expansions)
	}
}
