package internal

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
)

type TaskConfig struct {
	Distro             *apimodels.DistroView
	ProjectRef         *model.ProjectRef
	Project            *model.Project
	Task               *task.Task
	BuildVariant       *model.BuildVariant
	Expansions         *util.Expansions
	Redacted           map[string]bool
	WorkDir            string
	GithubPatchData    thirdparty.GithubPatch
	GithubMergeData    thirdparty.GithubMergeGroup
	Timeout            *Timeout
	TaskSync           evergreen.S3Credentials
	EC2Keys            []evergreen.EC2Key
	ModulePaths        map[string]string
	CedarTestResultsID string

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

func NewTaskConfig(workDir string, d *apimodels.DistroView, p *model.Project, t *task.Task, r *model.ProjectRef, patchDoc *patch.Patch, e util.Expansions) (*TaskConfig, error) {
	// do a check on if the project is empty
	if p == nil {
		return nil, errors.Errorf("project '%s' is nil", t.Project)
	}

	// check on if the project ref is empty
	if r == nil {
		return nil, errors.Errorf("project ref '%s' is nil", p.Identifier)
	}

	bv := p.FindBuildVariant(t.BuildVariant)
	if bv == nil {
		return nil, errors.Errorf("cannot find build variant '%s' for task in project '%s'", t.BuildVariant, t.Project)
	}

	taskConfig := &TaskConfig{
		Distro:       d,
		ProjectRef:   r,
		Project:      p,
		Task:         t,
		BuildVariant: bv,
		Expansions:   &e,
		WorkDir:      workDir,
	}
	if patchDoc != nil {
		taskConfig.GithubPatchData = patchDoc.GithubPatchData
		taskConfig.GithubMergeData = patchDoc.GithubMergeData
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
		return "", errors.Errorf("path '%s' does not exist", dir)
	} else if err != nil || stat == nil {
		return "", errors.Wrapf(err, "retrieving file info for path '%s'", dir)
	} else if !stat.IsDir() {
		return "", errors.Errorf("path '%s' is not a directory", dir)
	}

	return dir, nil
}

func (c *TaskConfig) GetCloneMethod() string {
	if c.Distro != nil {
		return c.Distro.CloneMethod
	}
	return evergreen.CloneMethodOAuth
}

func (tc *TaskConfig) GetTaskGroup(taskGroup string) (*model.TaskGroup, error) {
	if err := tc.validateTaskConfig(); err != nil {
		return nil, err
	}

	if taskGroup == "" {
		return nil, nil
	}
	tg := tc.Project.FindTaskGroup(taskGroup)
	if tg == nil {
		return nil, errors.Errorf("couldn't find task group '%s' in project '%s'", taskGroup, tc.Project.Identifier)
	}

	if tg.Timeout == nil {
		tg.Timeout = tc.Project.Timeout
	}
	return tg, nil
}

// GetTimeout returns the timeout defined on the taskGroup or project.
func (tc *TaskConfig) GetTimeout(taskGroup string) (*model.YAMLCommandSet, error) {
	if err := tc.validateTaskConfig(); err != nil {
		return nil, err
	}

	if taskGroup == "" {
		return tc.Project.Timeout, nil
	}

	tg := tc.Project.FindTaskGroup(taskGroup)
	if tg == nil {
		return nil, errors.Errorf("couldn't find task group '%s' in project '%s'", taskGroup, tc.Project.Identifier)
	}
	if tg.Timeout == nil {
		return tc.Project.Timeout, nil
	}

	return tg.Timeout, nil
}

// CommandBlock contains information for a block of commands.
type CommandBlock struct {
	Commands    *model.YAMLCommandSet
	CanFailTask bool
}

// GetPre returns a command block containing the pre task commands.
func (tc *TaskConfig) GetPre(taskGroup string) (*CommandBlock, error) {
	if err := tc.validateTaskConfig(); err != nil {
		return nil, err
	}

	canFailTask := tc.Project.Pre == nil || tc.Project.PreErrorFailsTask
	if taskGroup == "" {
		return &CommandBlock{Commands: tc.Project.Pre, CanFailTask: canFailTask}, nil
	}
	tg := tc.Project.FindTaskGroup(taskGroup)
	if tg == nil {
		return nil, errors.Errorf("couldn't find task group '%s' in project '%s'", taskGroup, tc.Project.Identifier)
	}

	return &CommandBlock{Commands: tg.SetupTask, CanFailTask: tg.SetupGroupFailTask}, nil
}

// GetPost returns a command block containing the post task commands.
func (tc *TaskConfig) GetPost(taskGroup string) (*CommandBlock, error) {
	if err := tc.validateTaskConfig(); err != nil {
		return nil, err
	}

	canFailTask := tc.Project.Post == nil || tc.Project.PostErrorFailsTask
	if taskGroup == "" {
		return &CommandBlock{Commands: tc.Project.Post, CanFailTask: canFailTask}, nil
	}
	tg := tc.Project.FindTaskGroup(taskGroup)
	if tg == nil {
		return nil, errors.Errorf("couldn't find task group '%s' in project '%s'", taskGroup, tc.Project.Identifier)
	}
	return &CommandBlock{Commands: tg.TeardownTask, CanFailTask: tg.TeardownTaskCanFailTask}, nil

}

func (tc *TaskConfig) TaskAttributeMap() map[string]string {
	return map[string]string{
		evergreen.TaskIDOtelAttribute:            tc.Task.Id,
		evergreen.TaskNameOtelAttribute:          tc.Task.DisplayName,
		evergreen.TaskExecutionOtelAttribute:     strconv.Itoa(tc.Task.Execution),
		evergreen.VersionIDOtelAttribute:         tc.Task.Version,
		evergreen.VersionRequesterOtelAttribute:  tc.Task.Requester,
		evergreen.BuildIDOtelAttribute:           tc.Task.BuildId,
		evergreen.BuildNameOtelAttribute:         tc.Task.BuildVariant,
		evergreen.ProjectIdentifierOtelAttribute: tc.ProjectRef.Identifier,
		evergreen.ProjectIDOtelAttribute:         tc.ProjectRef.Id,
		evergreen.DistroIDOtelAttribute:          tc.Task.DistroId,
	}
}

func (tc *TaskConfig) AddTaskBaggageToCtx(ctx context.Context) (context.Context, error) {
	catcher := grip.NewBasicCatcher()

	bag := baggage.FromContext(ctx)
	for key, val := range tc.TaskAttributeMap() {
		member, err := baggage.NewMember(key, val)
		if err != nil {
			catcher.Add(errors.Wrapf(err, "making member for key '%s' val '%s'", key, val))
			continue
		}
		bag, err = bag.SetMember(member)
		catcher.Add(err)
	}

	return baggage.ContextWithBaggage(ctx, bag), catcher.Resolve()
}

func (tc *TaskConfig) TaskAttributes() []attribute.KeyValue {
	var attributes []attribute.KeyValue
	for key, val := range tc.TaskAttributeMap() {
		attributes = append(attributes, attribute.String(key, val))
	}

	return attributes
}

func (tc *TaskConfig) validateTaskConfig() error {
	if tc == nil {
		return errors.New("unable to get task setup because task config is nil")
	}
	if tc.Task == nil {
		return errors.New("unable to get task setup because task is nil")
	}
	if tc.Task.Version == "" {
		return errors.New("task has no version")
	}
	if tc.Project == nil {
		return errors.New("project is nil")
	}
	return nil
}
