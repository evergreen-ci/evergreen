package internal

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/taskoutput"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
)

type TaskConfig struct {
	Distro       *apimodels.DistroView
	ProjectRef   model.ProjectRef
	Project      model.Project
	Task         task.Task
	BuildVariant model.BuildVariant

	// Expansions store the fundamental expansions set by Evergreen.
	// e.g. execution, project_id, task_id, etc. It also stores
	// expansions that are set by the user by expansion.update.
	Expansions util.Expansions

	// NewExpansions is a thread safe way to access Expansions.
	// It also exposes a way to redact expansions from logs.
	NewExpansions *agentutil.DynamicExpansions

	// DynamicExpansions holds expansions that were set from 'expansions.update'
	// and should persist throughout the task's execution.
	DynamicExpansions util.Expansions

	ProjectVars        map[string]string
	Redacted           []string
	RedactKeys         []string
	WorkDir            string
	TaskOutputDir      *taskoutput.Directory
	GithubPatchData    thirdparty.GithubPatch
	GithubMergeData    thirdparty.GithubMergeGroup
	Timeout            Timeout
	TaskOutput         evergreen.S3Credentials
	TaskSync           evergreen.S3Credentials
	EC2Keys            []evergreen.EC2Key
	ModulePaths        map[string]string
	CedarTestResultsID string
	TaskGroup          *model.TaskGroup
	CommandCleanups    CommandCleanups

	mu sync.RWMutex
}

// CommandCleanups is a list of cleanup functions that are added dynamically
// during task execution. These functions are called when the task is
// finished. They are then purged from the list.
type CommandCleanup struct {
	// Command is the name of the command from (base).FullDisplayName().
	Command string
	// Run is the function that is called when the task is finished.
	Run func(context.Context) error
}

type CommandCleanups []CommandCleanup

func (c CommandCleanups) RunAll(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()
	for _, cleanup := range c {
		catcher.Wrapf(cleanup.Run(ctx), "running clean up from command '%s'", cleanup.Command)
	}
	return errors.Wrap(catcher.Resolve(), "running command cleanups")
}

// Timeout records dynamic timeout information that has been explicitly set by
// the user during task runtime.
type Timeout struct {
	IdleTimeoutSecs int
	ExecTimeoutSecs int
}

// SetIdleTimeout sets the dynamic idle timeout explicitly set by the user
// during task runtime (e.g. via timeout.update).
func (t *TaskConfig) SetIdleTimeout(timeout int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Timeout.IdleTimeoutSecs = timeout
}

// SetIdleTimeout sets the dynamic idle timeout explicitly set by the user
// during task runtime (e.g. via timeout.update).
func (t *TaskConfig) SetExecTimeout(timeout int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Timeout.ExecTimeoutSecs = timeout
}

// GetIdleTimeout returns the dynamic idle timeout explicitly set by the user
// during task runtime (e.g. via timeout.update).
func (t *TaskConfig) GetIdleTimeout() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Timeout.IdleTimeoutSecs
}

// GetExecTimeout returns the dynamic execution timeout explicitly set by the
// user during task runtime (e.g. via timeout.update).
func (t *TaskConfig) GetExecTimeout() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Timeout.ExecTimeoutSecs
}

// NewTaskConfig validates that the required inputs are given and populates the
// information necessary for a task to run. It is generally preferred to use
// this function over initializing the TaskConfig struct manually.
func NewTaskConfig(workDir string, d *apimodels.DistroView, p *model.Project, t *task.Task, r *model.ProjectRef, patchDoc *patch.Patch, e *apimodels.ExpansionsAndVars) (*TaskConfig, error) {
	if p == nil {
		return nil, errors.Errorf("project '%s' is nil", t.Project)
	}
	if r == nil {
		return nil, errors.Errorf("project ref '%s' is nil", p.Identifier)
	}
	if t == nil {
		return nil, errors.Errorf("task cannot be nil")
	}

	bv := p.FindBuildVariant(t.BuildVariant)
	if bv == nil {
		return nil, errors.Errorf("cannot find build variant '%s' for task in project '%s'", t.BuildVariant, t.Project)
	}

	var taskGroup *model.TaskGroup
	if t.TaskGroup != "" {
		taskGroup = p.FindTaskGroup(t.TaskGroup)
		if taskGroup == nil {
			return nil, errors.Errorf("task is part of task group '%s' but no such task group is defined in the project", t.TaskGroup)
		}
	}

	// Add keys matching redact patterns to private vars.
	for key := range e.Vars {
		if ok := e.PrivateVars[key]; ok {
			// Skip since the key is already private.
			continue
		}

		for _, pattern := range e.RedactKeys {
			if strings.Contains(strings.ToLower(key), pattern) {
				e.PrivateVars[key] = true
				break
			}
		}
	}

	var redacted []string
	for key := range e.PrivateVars {
		redacted = append(redacted, key)
	}

	taskConfig := &TaskConfig{
		Distro:            d,
		ProjectRef:        *r,
		Project:           *p,
		Task:              *t,
		BuildVariant:      *bv,
		Expansions:        e.Expansions,
		NewExpansions:     agentutil.NewDynamicExpansions(e.Expansions),
		DynamicExpansions: util.Expansions{},
		ProjectVars:       e.Vars,
		Redacted:          redacted,
		WorkDir:           workDir,
		TaskGroup:         taskGroup,
	}
	if patchDoc != nil {
		taskConfig.GithubPatchData = patchDoc.GithubPatchData
		taskConfig.GithubMergeData = patchDoc.GithubMergeData
	}

	return taskConfig, nil
}

func (tc *TaskConfig) TaskAttributeMap() map[string]string {
	attributes := map[string]string{
		evergreen.TaskIDOtelAttribute:            tc.Task.Id,
		evergreen.TaskNameOtelAttribute:          tc.Task.DisplayName,
		evergreen.TaskExecutionOtelAttribute:     strconv.Itoa(tc.Task.Execution),
		evergreen.VersionIDOtelAttribute:         tc.Task.Version,
		evergreen.VersionRequesterOtelAttribute:  tc.Task.Requester,
		evergreen.BuildIDOtelAttribute:           tc.Task.BuildId,
		evergreen.BuildNameOtelAttribute:         tc.Task.BuildVariant,
		evergreen.ProjectIdentifierOtelAttribute: tc.ProjectRef.Identifier,
		evergreen.ProjectOrgOtelAttribute:        tc.ProjectRef.Owner,
		evergreen.ProjectRepoOtelAttribute:       tc.ProjectRef.Repo,
		evergreen.ProjectIDOtelAttribute:         tc.ProjectRef.Id,
		evergreen.DistroIDOtelAttribute:          tc.Task.DistroId,
	}
	if tc.GithubPatchData.PRNumber != 0 {
		attributes[evergreen.VersionPRNumOtelAttribute] = strconv.Itoa(tc.GithubPatchData.PRNumber)
	}
	return attributes
}

// TaskAttributes returns a list of common otel attributes for tasks.
func (tc *TaskConfig) TaskAttributes() []attribute.KeyValue {
	var attributes []attribute.KeyValue
	for key, val := range tc.TaskAttributeMap() {
		attributes = append(attributes, attribute.String(key, val))
	}
	if len(tc.Task.Tags) > 0 {
		attributes = append(attributes, attribute.StringSlice(evergreen.TaskTagsOtelAttribute, tc.Task.Tags))
	}

	return attributes
}

// createsCheckRun returns a boolean indicating if the current task creates a checkRun
func (tc *TaskConfig) createsCheckRun() bool {
	for _, tu := range tc.BuildVariant.Tasks {
		if tu.Name == tc.Task.DisplayName {
			return tu.CreateCheckRun != nil
		}
	}
	return false
}
