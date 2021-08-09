package internal

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

type TaskConfig struct {
	Distro               *apimodels.DistroView
	ProjectRef           *model.ProjectRef
	Project              *model.Project
	Task                 *task.Task
	BuildVariant         *model.BuildVariant
	Expansions           *util.Expansions
	RestrictedExpansions *util.Expansions
	Redacted             map[string]bool
	WorkDir              string
	GithubPatchData      thirdparty.GithubPatch
	Timeout              *Timeout
	TaskSync             evergreen.S3Credentials
	ModulePaths          map[string]string
	CedarTestResultsID   string

	mu sync.RWMutex
}

type Timeout struct {
	IdleTimeoutSecs int
	ExecTimeoutSecs int
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

func NewTaskConfig(d *apimodels.DistroView, p *model.Project, t *task.Task, r *model.ProjectRef, patchDoc *patch.Patch, e util.Expansions) (*TaskConfig, error) {
	// do a check on if the project is empty
	if p == nil {
		return nil, errors.Errorf("project for task with project_id %v is empty", t.Project)
	}

	// check on if the project ref is empty
	if r == nil {
		return nil, errors.Errorf("Project ref with identifier: %v was empty", p.Identifier)
	}

	bv := p.FindBuildVariant(t.BuildVariant)
	if bv == nil {
		return nil, errors.Errorf("couldn't find buildvariant: '%v'", t.BuildVariant)
	}

	taskConfig := &TaskConfig{
		Distro:       d,
		ProjectRef:   r,
		Project:      p,
		Task:         t,
		BuildVariant: bv,
		Expansions:   &e,
		WorkDir:      d.WorkDir,
	}
	if patchDoc != nil {
		taskConfig.GithubPatchData = patchDoc.GithubPatchData
	}

	taskConfig.Timeout = &Timeout{}

	return taskConfig, nil
}

func (c *TaskConfig) GetWorkingDirectory(dir string) (string, error) {
	if dir == "" {
		dir = c.WorkDir
	} else if strings.HasPrefix(dir, c.WorkDir) {
		// pass
	} else {
		dir = filepath.Join(c.WorkDir, dir)
	}

	if stat, err := os.Stat(dir); os.IsNotExist(err) {
		return "", errors.Errorf("directory %s does not exist", dir)
	} else if err != nil || stat == nil {
		return "", errors.Wrapf(err, "error retrieving file info for %s", dir)
	} else if !stat.IsDir() {
		return "", errors.Errorf("path %s is not a directory", dir)
	}

	return dir, nil
}

// GetExpansionsWithRestricted should ONLY be used for operations that won't leak restricted expansions.
// Otherwise, use taskConfig.Expansions directly.
func (c *TaskConfig) GetExpansionsWithRestricted() *util.Expansions {
	allExpansions := *c.Expansions
	if c.RestrictedExpansions != nil {
		allExpansions.Update(c.RestrictedExpansions.Map())
	}
	return &allExpansions
}

func (tc *TaskConfig) GetTaskGroup(taskGroup string) (*model.TaskGroup, error) {
	if tc == nil {
		return nil, errors.New("unable to get task group: TaskConfig is nil")
	}
	if tc.Task == nil {
		return nil, errors.New("unable to get task group: task is nil")
	}
	if tc.Task.Version == "" {
		return nil, errors.New("task has no version")
	}
	if tc.Project == nil {
		return nil, errors.New("project is nil")
	}

	if taskGroup == "" {
		// if there is no named task group, fall back to project definitions
		return &model.TaskGroup{
			SetupTask:               tc.Project.Pre,
			TeardownTask:            tc.Project.Post,
			Timeout:                 tc.Project.Timeout,
			SetupGroupFailTask:      tc.Project.Pre == nil || tc.Project.PreErrorFailsTask,
			TeardownTaskCanFailTask: tc.Project.Post == nil || tc.Project.PostErrorFailsTask,
		}, nil
	}
	tg := tc.Project.FindTaskGroup(taskGroup)
	if tg == nil {
		return nil, errors.Errorf("couldn't find task group %s", tc.Task.TaskGroup)
	}
	return tg, nil
}
