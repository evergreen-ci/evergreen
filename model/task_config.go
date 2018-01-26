package model

import (
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

type TaskConfig struct {
	Distro       *distro.Distro
	Version      *version.Version
	ProjectRef   *ProjectRef
	Project      *Project
	Task         *task.Task
	BuildVariant *BuildVariant
	Expansions   *util.Expansions
	WorkDir      string
}

func NewTaskConfig(d *distro.Distro, v *version.Version, p *Project, t *task.Task, r *ProjectRef, patchDoc *patch.Patch) (*TaskConfig, error) {
	// do a check on if the project is empty
	if p == nil {
		return nil, errors.Errorf("project for task with branch %v is empty", t.Project)
	}

	// check on if the project ref is empty
	if r == nil {
		return nil, errors.Errorf("Project ref with identifier: %v was empty", p.Identifier)
	}

	bv := p.FindBuildVariant(t.BuildVariant)
	if bv == nil {
		return nil, errors.Errorf("couldn't find buildvariant: '%v'", t.BuildVariant)
	}

	e := populateExpansions(d, v, bv, t, patchDoc)
	return &TaskConfig{d, v, r, p, t, bv, e, d.WorkDir}, nil
}

func (c *TaskConfig) GetWorkingDirectory(dir string) (string, error) {
	if dir == "" {
		dir = c.WorkDir
	} else {
		dir = filepath.Join(c.WorkDir, dir)
	}

	if stat, err := os.Stat(dir); os.IsNotExist(err) {
		return "", errors.Errorf("directory %s does not exist", dir)
	} else if !stat.IsDir() {
		return "", errors.Errorf("path %s is not a directory", dir)
	}

	return dir, nil
}
