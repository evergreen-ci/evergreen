package model

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

type TaskConfig struct {
	Distro          *distro.Distro
	Version         *Version
	ProjectRef      *ProjectRef
	Project         *Project
	Task            *task.Task
	BuildVariant    *BuildVariant
	Expansions      *util.Expansions
	Redacted        map[string]bool
	WorkDir         string
	GithubPatchData patch.GithubPatch
	Timeout         *Timeout

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

func NewTaskConfig(d *distro.Distro, v *Version, p *Project, t *task.Task, r *ProjectRef, patchDoc *patch.Patch, e util.Expansions) (*TaskConfig, error) {
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
		Version:      v,
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

func MakeConfigFromTask(t *task.Task) (*TaskConfig, error) {
	if t == nil {
		return nil, errors.New("no task to make a TaskConfig from")
	}
	v, err := VersionFindOne(VersionById(t.Version))
	if err != nil {
		return nil, errors.Wrap(err, "error finding version")
	}
	d, err := distro.FindOne(distro.ById(t.DistroId))
	if err != nil {
		return nil, errors.Wrap(err, "error finding distro")
	}
	proj := &Project{}
	err = LoadProjectInto([]byte(v.Config), v.Identifier, proj)
	if err != nil {
		return nil, errors.Wrap(err, "error loading project")
	}
	projRef, err := FindOneProjectRef(t.Project)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project ref")
	}
	var p *patch.Patch
	if v.Requester == evergreen.PatchVersionRequester {
		p, err = patch.FindOne(patch.ByVersion(v.Id))
		if err != nil {
			return nil, errors.Wrap(err, "error finding patch")
		}
	}
	h, err := host.FindOne(host.ById(t.HostId))
	if err != nil {
		return nil, errors.Wrap(err, "error finding host")
	}

	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error getting evergreen config")
	}
	oauthToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "error getting oauth token")
	}
	e, err := PopulateExpansions(t, h, oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "error populating expansions")
	}

	tc, err := NewTaskConfig(&d, v, proj, t, projRef, p, e)
	if err != nil {
		return nil, errors.Wrap(err, "error making TaskConfig")
	}
	projVars, err := FindOneProjectVars(t.Project)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project vars")
	}
	tc.Expansions.Update(projVars.Vars)
	tc.Redacted = projVars.PrivateVars
	return tc, nil
}
