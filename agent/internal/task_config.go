package internal

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/taskoutput"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
)

type TaskConfig struct {
	Distro          *apimodels.DistroView
	Host            *apimodels.HostView
	ProjectRef      model.ProjectRef
	Project         model.Project
	Task            task.Task
	DisplayTaskInfo *apimodels.DisplayTaskInfo
	BuildVariant    model.BuildVariant

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

	// AssumeRoleInformation holds the session tokens with their corresponding
	// expiration and role ARNs that have been assumed by the agent during the task's execution.
	// User's don't pass in their expiration, so we need to keep track of it when
	// we want to use their passed in credentials.
	AssumeRoleInformation map[string]AssumeRoleInformation

	ProjectVars map[string]string
	Redacted    []string
	RedactKeys  []string
	// InternalRedactions contain string values that should be redacted because
	// they are sensitive and for Evergreen-internal use only, but are not
	// expansions available to tasks. Having this allows redaction of strings
	// that cannot be exposed to tasks as expansions.
	// This only reuses agentutil.DynamicExpansions for thread safety.
	InternalRedactions *agentutil.DynamicExpansions
	WorkDir            string
	TaskOutputDir      *taskoutput.Directory
	GithubPatchData    thirdparty.GithubPatch
	GithubMergeData    thirdparty.GithubMergeGroup
	Timeout            Timeout
	TaskOutput         evergreen.S3Credentials
	ModulePaths        map[string]string
	// HasTestResults is true if the task has sent at least one test result.
	HasTestResults bool
	// HasFailingTestResult is true if the task has sent at least one test
	// result and at least one of those tests failed.
	HasFailingTestResult bool
	TaskGroup            *model.TaskGroup
	CommandCleanups      []CommandCleanup
	MaxExecTimeoutSecs   int

	// PatchOrVersionDescription holds the description of a patch or
	// message of a version to be used in the otel attributes.
	PatchOrVersionDescription string

	mu sync.RWMutex
}

func (tc *TaskConfig) TaskData() client.TaskData {
	return client.TaskData{
		ID:     tc.Task.Id,
		Secret: tc.Task.Secret,
	}
}

// CommandCleanup is a cleanup function associated with a command. As a command
// block is executed, the cleanup function(s) are added to the TaskConfig. When
// the command block is finished, the cleanup function(s) are collected by the
// TaskContext and executed depending on what command block was executed.
// For every command cleanup, a span is created with the Command as the name.
type CommandCleanup struct {
	// Command is the name of the command from (base).FullDisplayName().
	Command string
	// Run is the function that is called when the task is finished.
	Run func(context.Context) error
}

type AssumeRoleInformation struct {
	RoleARN    string
	Expiration time.Time
}

// AddCommandCleanup adds a cleanup function to the TaskConfig.
func (t *TaskConfig) AddCommandCleanup(cmd string, run func(context.Context) error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.CommandCleanups = append(t.CommandCleanups, CommandCleanup{
		Command: cmd,
		Run:     run,
	})
}

// GetAndClearCommandCleanups returns the command cleanups that have been added
// to the TaskConfig and clears the list of command cleanups.
func (t *TaskConfig) GetAndClearCommandCleanups() []CommandCleanup {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cleanups := t.CommandCleanups
	t.CommandCleanups = nil
	return cleanups
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

type TaskConfigOptions struct {
	WorkDir           string
	Distro            *apimodels.DistroView
	Host              *apimodels.HostView
	Project           *model.Project
	Task              *task.Task
	DisplayTaskInfo   *apimodels.DisplayTaskInfo
	ProjectRef        *model.ProjectRef
	Patch             *patch.Patch
	Version           *model.Version
	ExpansionsAndVars *apimodels.ExpansionsAndVars
}

// NewTaskConfig validates that the required inputs are given and populates the
// information necessary for a task to run. It is generally preferred to use
// this function over initializing the TaskConfig struct manually.
// The patchDoc is optional and should be provided if the task is a PR or
// github merge queue task. It also propagates the patch description to
// the task config and otel attributes.
// If the patchDoc is nil, a versionDoc should be provided (optional as well)
// to get the version description for the otel attributes.
func NewTaskConfig(opts TaskConfigOptions) (*TaskConfig, error) {
	if opts.Task == nil {
		return nil, errors.Errorf("task cannot be nil")
	}
	if opts.Project == nil {
		return nil, errors.Errorf("project '%s' is nil", opts.Task.Project)
	}
	if opts.ProjectRef == nil {
		return nil, errors.Errorf("project ref '%s' is nil", opts.Project.Identifier)
	}

	bv := opts.Project.FindBuildVariant(opts.Task.BuildVariant)
	if bv == nil {
		return nil, errors.Errorf("cannot find build variant '%s' for task in project '%s'", opts.Task.BuildVariant, opts.Task.Project)
	}

	var taskGroup *model.TaskGroup
	if opts.Task.TaskGroup != "" {
		taskGroup = opts.Project.FindTaskGroup(opts.Task.TaskGroup)
		if taskGroup == nil {
			return nil, errors.Errorf("task is part of task group '%s' but no such task group is defined in the project", opts.Task.TaskGroup)
		}
	}

	// Add keys matching redact patterns to private vars.
	for key := range opts.ExpansionsAndVars.Vars {
		if ok := opts.ExpansionsAndVars.PrivateVars[key]; ok {
			// Skip since the key is already private.
			continue
		}

		for _, pattern := range opts.ExpansionsAndVars.RedactKeys {
			if strings.Contains(strings.ToLower(key), pattern) {
				opts.ExpansionsAndVars.PrivateVars[key] = true
				break
			}
		}
	}

	var redacted []string
	for key := range opts.ExpansionsAndVars.PrivateVars {
		redacted = append(redacted, key)
	}

	internalRedactions := opts.ExpansionsAndVars.InternalRedactions
	if internalRedactions == nil {
		internalRedactions = map[string]string{}
	}

	taskConfig := &TaskConfig{
		Distro:                opts.Distro,
		Host:                  opts.Host,
		ProjectRef:            *opts.ProjectRef,
		Project:               *opts.Project,
		Task:                  *opts.Task,
		DisplayTaskInfo:       opts.DisplayTaskInfo,
		BuildVariant:          *bv,
		Expansions:            opts.ExpansionsAndVars.Expansions,
		NewExpansions:         agentutil.NewDynamicExpansions(opts.ExpansionsAndVars.Expansions),
		DynamicExpansions:     util.Expansions{},
		AssumeRoleInformation: map[string]AssumeRoleInformation{},
		InternalRedactions:    agentutil.NewDynamicExpansions(internalRedactions),
		ProjectVars:           opts.ExpansionsAndVars.Vars,
		Redacted:              redacted,
		WorkDir:               opts.WorkDir,
		TaskGroup:             taskGroup,
	}
	if opts.Patch != nil {
		taskConfig.GithubPatchData = opts.Patch.GithubPatchData
		taskConfig.GithubMergeData = opts.Patch.GithubMergeData
		taskConfig.PatchOrVersionDescription = opts.Patch.Description
	} else if opts.Version != nil {
		taskConfig.PatchOrVersionDescription = opts.Version.Message
	}

	return taskConfig, nil
}

func (tc *TaskConfig) TaskAttributeMap() map[string]string {
	attributes := map[string]string{
		evergreen.TaskIDOtelAttribute:             tc.Task.Id,
		evergreen.TaskNameOtelAttribute:           tc.Task.DisplayName,
		evergreen.TaskExecutionOtelAttribute:      strconv.Itoa(tc.Task.Execution),
		evergreen.VersionIDOtelAttribute:          tc.Task.Version,
		evergreen.VersionRequesterOtelAttribute:   tc.Task.Requester,
		evergreen.BuildIDOtelAttribute:            tc.Task.BuildId,
		evergreen.BuildNameOtelAttribute:          tc.Task.BuildVariant,
		evergreen.ProjectIdentifierOtelAttribute:  tc.ProjectRef.Identifier,
		evergreen.ProjectOrgOtelAttribute:         tc.ProjectRef.Owner,
		evergreen.ProjectRepoOtelAttribute:        tc.ProjectRef.Repo,
		evergreen.ProjectIDOtelAttribute:          tc.ProjectRef.Id,
		evergreen.RepoRefIDOtelAttribute:          tc.ProjectRef.RepoRefId,
		evergreen.DistroIDOtelAttribute:           tc.Task.DistroId,
		evergreen.VersionDescriptionOtelAttribute: tc.PatchOrVersionDescription,
	}
	if tc.GithubPatchData.PRNumber != 0 {
		attributes[evergreen.VersionPRNumOtelAttribute] = strconv.Itoa(tc.GithubPatchData.PRNumber)
	}
	if tc.Host != nil && tc.Host.Hostname != "" {
		attributes[evergreen.HostnameOtelAttribute] = tc.Host.Hostname
	}
	if tc.DisplayTaskInfo != nil {
		attributes[evergreen.DisplayTaskIDOtelAttribute] = tc.DisplayTaskInfo.ID
		attributes[evergreen.DisplayTaskNameOtelAttribute] = tc.DisplayTaskInfo.Name
	}
	if !utility.IsZeroTime(tc.Task.ActivatedTime) {
		attributes[evergreen.TaskActivatedTimeOtelAttribute] = tc.Task.ActivatedTime.Format(time.RFC3339)
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
