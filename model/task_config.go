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
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

type TaskConfig struct {
	Distro               *distro.Distro
	ProjectRef           *ProjectRef
	Project              *Project
	Task                 *task.Task
	BuildVariant         *BuildVariant
	Expansions           *util.Expansions
	RestrictedExpansions *util.Expansions
	Redacted             map[string]bool
	WorkDir              string
	GithubPatchData      thirdparty.GithubPatch
	Timeout              *Timeout
	TaskSync             evergreen.S3Credentials

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

func NewTaskConfig(d *distro.Distro, p *Project, t *task.Task, r *ProjectRef, patchDoc *patch.Patch, e util.Expansions) (*TaskConfig, error) {
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

func MakeConfigFromTask(t *task.Task) (*TaskConfig, error) {
	if t == nil {
		return nil, errors.New("no task to make a TaskConfig from")
	}
	dat, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return nil, errors.Wrap(err, "could not get distro lookup table")
	}
	distroIDs := dat.Expand([]string{t.DistroId})
	if len(distroIDs) == 0 {
		return nil, errors.Errorf("could not resolve distro '%s'", t.DistroId)
	}
	// If this distro name is aliased, it could resolve into multiple concrete
	// distros, so just pick one of them.
	d, err := distro.FindOne(distro.ById(distroIDs[0]))
	if err != nil {
		return nil, errors.Wrap(err, "error finding distro")
	}
	v, err := VersionFindOne(VersionById(t.Version))
	if err != nil {
		return nil, errors.Wrap(err, "error finding version")
	}
	if v == nil {
		return nil, errors.New("version doesn't exist")
	}
	proj, _, err := LoadProjectForVersion(v, v.Identifier, true)
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

	tc, err := NewTaskConfig(&d, proj, t, projRef, p, e)
	if err != nil {
		return nil, errors.Wrap(err, "error making TaskConfig")
	}
	// project parameters are listed first to give version parameters higher priority
	params := append(proj.GetParameters(), v.Parameters...)
	if err := tc.updateExpansions(t.Project, params); err != nil {
		return nil, errors.Wrap(err, "error updating expansions")
	}
	return tc, nil
}

// updateExpansions first updates expansions with project variables, and then updates
// parameters in list order to maintain priority.
func (tc *TaskConfig) updateExpansions(projectId string, params []patch.Parameter) error {
	projVars, err := FindOneProjectVars(projectId)
	if err != nil {
		return errors.Wrap(err, "error finding project vars")
	}
	if projVars == nil {
		return errors.New("project vars not found")
	}

	tc.Expansions.Update(projVars.GetUnrestrictedVars())
	tc.RestrictedExpansions.Update(projVars.GetRestrictedVars())
	tc.Redacted = projVars.PrivateVars

	for _, param := range params {
		tc.Expansions.Put(param.Key, param.Value)
		// params do not support restricted or redacted
		tc.RestrictedExpansions.Remove(param.Key)
		tc.Redacted[param.Key] = false
	}
	return nil
}
