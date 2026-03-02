package task

import (
	"context"
	"fmt"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

const (
	dependencyKey = "dependencies"

	oneWeek = 7 * 24 * time.Hour

	// UnschedulableThreshold is the threshold after which a task waiting to
	// dispatch should be unscheduled due to staleness.
	UnschedulableThreshold = oneWeek

	// indicates the window of completed tasks we want to use in computing
	// average task duration. By default we use tasks that have
	// completed within the last 7 days
	taskCompletionEstimateWindow = oneWeek

	// if we have no data on a given task, default to 10 minutes so we
	// have some new hosts spawned
	defaultTaskDuration = 10 * time.Minute

	// length of time to cache the expected duration in the task document
	predictionTTL = 8 * time.Hour
)

var (
	// A regex that matches either / or \ for splitting directory paths
	// on either windows or linux paths.
	eitherSlash = regexp.MustCompile(`[/\\]`)
)

type Task struct {
	Id     string `bson:"_id" json:"id"`
	Secret string `bson:"secret" json:"secret"`
	// time information for task
	// CreateTime - the creation time for the task, derived from the commit time or the patch creation time.
	// DispatchTime - the time the task runner starts up the agent on the host.
	// ScheduledTime - the time the task is scheduled.
	// StartTime - the time the agent starts the task on the host after spinning it up.
	// FinishTime - the time the task was completed on the remote host.
	// ActivatedTime - the time the task was marked as available to be scheduled, automatically or by a developer.
	// DependenciesMet - for tasks that have dependencies, the time all dependencies are met.
	CreateTime          time.Time `bson:"create_time" json:"create_time"`
	IngestTime          time.Time `bson:"injest_time" json:"ingest_time"`
	DispatchTime        time.Time `bson:"dispatch_time" json:"dispatch_time"`
	ScheduledTime       time.Time `bson:"scheduled_time" json:"scheduled_time"`
	StartTime           time.Time `bson:"start_time" json:"start_time"`
	FinishTime          time.Time `bson:"finish_time" json:"finish_time"`
	ActivatedTime       time.Time `bson:"activated_time" json:"activated_time"`
	DependenciesMetTime time.Time `bson:"dependencies_met_time,omitempty" json:"dependencies_met_time,omitempty"`

	Version string `bson:"version" json:"version,omitempty"`
	// Project is the project id of the task.
	Project  string `bson:"branch" json:"branch,omitempty"`
	Revision string `bson:"gitspec" json:"gitspec"`
	// Priority is a specifiable value that adds weight to the prioritization that task will be given in its
	// corresponding distro task queue.
	Priority int64 `bson:"priority" json:"priority"`
	// SortingValueBreakdown is not persisted to the db, but stored in memory and passed to the task queue document.
	// It contains information on what factors led to the overall queue ranking value for the task.
	SortingValueBreakdown SortingValueBreakdown `bson:"-" json:"sorting_value_breakdown"`
	TaskGroup             string                `bson:"task_group" json:"task_group"`
	TaskGroupMaxHosts     int                   `bson:"task_group_max_hosts,omitempty" json:"task_group_max_hosts,omitempty"`
	TaskGroupOrder        int                   `bson:"task_group_order,omitempty" json:"task_group_order,omitempty"`
	ResultsService        string                `bson:"results_service,omitempty" json:"results_service,omitempty"`
	HasTestResults        bool                  `bson:"has_test_results,omitempty" json:"has_test_results,omitempty"`
	ResultsFailed         bool                  `bson:"results_failed,omitempty" json:"results_failed,omitempty"`
	MustHaveResults       bool                  `bson:"must_have_results,omitempty" json:"must_have_results,omitempty"`
	// only relevant if the task is running.  the time of the last heartbeat
	// sent back by the agent
	LastHeartbeat time.Time `bson:"last_heartbeat" json:"last_heartbeat"`

	// Activated indicates whether the task should be scheduled to run or not.
	Activated                bool   `bson:"activated" json:"activated"`
	ActivatedBy              string `bson:"activated_by" json:"activated_by"`
	DeactivatedForDependency bool   `bson:"deactivated_for_dependency" json:"deactivated_for_dependency"`

	BuildId                 string       `bson:"build_id" json:"build_id"`
	DistroId                string       `bson:"distro" json:"distro"`
	BuildVariant            string       `bson:"build_variant" json:"build_variant"`
	BuildVariantDisplayName string       `bson:"build_variant_display_name" json:"-"`
	DependsOn               []Dependency `bson:"depends_on" json:"depends_on"`
	// UnattainableDependency caches the contents of DependsOn for more
	// efficient querying. It is true if any of its dependencies is unattainable
	// and is false if all of its dependencies are attainable.
	UnattainableDependency bool `bson:"unattainable_dependency" json:"unattainable_dependency"`
	NumDependents          int  `bson:"num_dependents,omitempty" json:"num_dependents,omitempty"`
	// OverrideDependencies indicates whether a task should override its dependencies. If set, it will not
	// wait for its dependencies to finish before running.
	OverrideDependencies bool `bson:"override_dependencies,omitempty" json:"override_dependencies,omitempty"`

	// SecondaryDistros refer to the optional secondary distros that can be
	// associated with a task. This is used for running tasks in case there are
	// idle hosts in a distro with an empty primary queue. This is a distinct concept
	// from distro aliases (i.e. alternative distro names).
	// Tags refer to outdated naming; maintained for compatibility.
	SecondaryDistros []string `bson:"distro_aliases,omitempty" json:"distro_aliases,omitempty"`

	// Human-readable name
	DisplayName string `bson:"display_name" json:"display_name"`

	// Tags that describe the task
	Tags []string `bson:"tags,omitempty" json:"tags,omitempty"`

	// The host the task was run on. This value is only set for host tasks.
	HostId string `bson:"host_id,omitempty" json:"host_id"`

	// ExecutionPlatform determines the execution environment that the task runs
	// in.
	ExecutionPlatform ExecutionPlatform `bson:"execution_platform,omitempty" json:"execution_platform,omitempty"`

	// The version of the agent this task was run on.
	AgentVersion string `bson:"agent_version,omitempty" json:"agent_version,omitempty"`
	// TaskOutputInfo holds the information for the interface that
	// coordinates persistent storage of a task's output data.
	// There are four possible scenarios:
	//     1. The task will never have output data (e.g., display tasks)
	//        and, therefore, the value is and always will be nil.
	//     2. The task does not have output data yet, but can in the future
	//        if/when dispatched, and, therefore, the value is currently
	//        nil.
	//     3. The task has been dispatched with the task output information
	//        initialized and the application can safely use this field to
	//        fetch any output data. If the task has not finished running,
	//        the output data is accessible but may not be complete yet.
	//     4. The task has data but was run before the introduction of the
	//        field and should be initialized before the application can
	//        safely fetch any output data.
	// This field should *never* be accessed directly, instead call
	// `Task.GetTaskOutputSafe()`.
	TaskOutputInfo *TaskOutput `bson:"task_output_info,omitempty" json:"task_output_info,omitempty"`

	// Set to true if the task should be considered for mainline github checks
	IsGithubCheck bool `bson:"is_github_check,omitempty" json:"is_github_check,omitempty"`

	// CheckRunPath is a local file path to an output json file for the checkrun.
	CheckRunPath *string `bson:"check_run_path,omitempty" json:"check_run_path,omitempty"`

	// CheckRunId is the id for the checkrun that was created in github.
	// This is used to update the checkrun for future executions of the task.
	CheckRunId *int64 `bson:"check_run_id,omitempty" json:"check_run_id,omitempty"`

	// CanReset indicates that the task has successfully archived and is in a valid state to be reset.
	CanReset bool `bson:"can_reset,omitempty" json:"can_reset,omitempty"`

	Execution int    `bson:"execution" json:"execution"`
	OldTaskId string `bson:"old_task_id,omitempty" json:"old_task_id,omitempty"`
	Archived  bool   `bson:"archived,omitempty" json:"archived,omitempty"`

	// RevisionOrderNumber for user-submitted patches is the user's current patch submission count.
	// For mainline commits for a project, it is the amount of versions for that repositry so far.
	RevisionOrderNumber int `bson:"order,omitempty" json:"order,omitempty"`

	// task requester - this is used to help tell the
	// reason this task was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"r"`

	// tasks that are part of a child patch will store the id and patch number of the parent patch
	ParentPatchID     string `bson:"parent_patch_id,omitempty" json:"parent_patch_id,omitempty"`
	ParentPatchNumber int    `bson:"parent_patch_number,omitempty" json:"parent_patch_number,omitempty"`

	// Status represents the various stages the task could be in. Note that this
	// task status is distinct from the way a task status is displayed in the
	// UI. For example, a task that has failed will have a status of
	// evergreen.TaskFailed regardless of the specific cause of failure.
	// However, in the UI, the displayed status supports more granular failure
	// type such as system failed and setup failed by checking this status and
	// the task status details.
	Status    string                  `bson:"status" json:"status"`
	Details   apimodels.TaskEndDetail `bson:"details" json:"task_end_details"`
	Aborted   bool                    `bson:"abort,omitempty" json:"abort"`
	AbortInfo AbortInfo               `bson:"abort_info,omitempty" json:"abort_info,omitempty"`

	// HostCreateDetails stores information about why host.create failed for this task
	HostCreateDetails []HostCreateDetail `bson:"host_create_details,omitempty" json:"host_create_details,omitempty"`
	// DisplayStatus is not persisted to the db. It is the status to display in the UI.
	// It may be added via aggregation
	DisplayStatus string `bson:"display_status,omitempty" json:"display_status,omitempty"`
	// DisplayStatusCache is semantically the same as DisplayStatus, but is persisted to the DB, unlike DisplayStatus.
	DisplayStatusCache string `bson:"display_status_cache,omitempty" json:"display_status_cache,omitempty"`
	// BaseTask is not persisted to the db. It is the data of the task on the base commit
	// It may be added via aggregation
	BaseTask BaseTaskInfo `bson:"base_task" json:"base_task"`

	// TimeTaken is how long the task took to execute (if it has finished) or how long the task has been running (if it has started)
	TimeTaken time.Duration `bson:"time_taken" json:"time_taken"`
	// PredictedTaskCost is the expected cost of running the task based on historical data
	PredictedTaskCost cost.Cost `bson:"predicted_cost,omitempty" json:"predicted_cost,omitempty"`
	// TaskCost is the actual cost of the task based on runtime and distro cost rates
	TaskCost cost.Cost `bson:"cost,omitempty" json:"cost,omitempty"`
	// S3Usage tracks S3 API usage for cost calculation
	S3Usage S3Usage `bson:"s3_usage,omitempty" json:"s3_usage,omitempty"`
	// WaitSinceDependenciesMet is populated in GetDistroQueueInfo, used for host allocation
	WaitSinceDependenciesMet time.Duration `bson:"wait_since_dependencies_met,omitempty" json:"wait_since_dependencies_met,omitempty"`

	// how long we expect the task to take from start to
	// finish. expected duration is the legacy value, but the UI
	// probably depends on it, so we maintain both values.
	ExpectedDuration       time.Duration            `bson:"expected_duration,omitempty" json:"expected_duration,omitempty"`
	ExpectedDurationStdDev time.Duration            `bson:"expected_duration_std_dev,omitempty" json:"expected_duration_std_dev,omitempty"`
	DurationPrediction     util.CachedDurationValue `bson:"duration_prediction,omitempty" json:"-"`

	// test results embedded from the testresults collection
	LocalTestResults []testresult.TestResult `bson:"-" json:"test_results"`

	// display task fields
	DisplayOnly           bool     `bson:"display_only,omitempty" json:"display_only,omitempty"`
	ExecutionTasks        []string `bson:"execution_tasks,omitempty" json:"execution_tasks,omitempty"`
	LatestParentExecution int      `bson:"latest_parent_execution" json:"latest_parent_execution"`

	StepbackInfo *StepbackInfo `bson:"stepback_info,omitempty" json:"stepback_info,omitempty"`

	// ResetWhenFinished indicates that a task should be reset once it is
	// finished running. This is typically to deal with tasks that should be
	// reset but cannot do so yet because they're currently running. This and
	// ResetFailedWhenFinished are mutually exclusive settings.
	ResetWhenFinished bool `bson:"reset_when_finished,omitempty" json:"reset_when_finished,omitempty"`
	// ResetWhenFinished indicates that a task should be reset once it is
	// finished running and only reset if it fails. This is typically to deal
	// with tasks that should be reset on failure but cannot do so yet because
	// they're currently running. This and ResetWhenFinished are mutually
	// exclusive settings.
	ResetFailedWhenFinished bool `bson:"reset_failed_when_finished,omitempty" json:"reset_failed_when_finished,omitempty"`
	// NumAutomaticRestarts is the number of times the task has been programmatically restarted via a failed agent command.
	NumAutomaticRestarts int `bson:"num_automatic_restarts,omitempty" json:"num_automatic_restarts,omitempty"`
	// IsAutomaticRestart indicates that the task was restarted via a failing agent command that was set to retry on failure.
	IsAutomaticRestart bool  `bson:"is_automatic_restart,omitempty" json:"is_automatic_restart,omitempty"`
	DisplayTask        *Task `bson:"-" json:"-"` // this is a local pointer from an exec to display task

	// DisplayTaskId is set to the display task ID if the task is an execution task, the empty string if it's not an execution task,
	// and is nil if we haven't yet checked whether or not this task has a display task.
	DisplayTaskId *string `bson:"display_task_id,omitempty" json:"display_task_id,omitempty"`

	// GenerateTask indicates that the task generates other tasks, which the
	// scheduler will use to prioritize this task. This will not be set for
	// tasks where the generate.tasks command runs outside of the main task
	// block (e.g. pre, timeout).
	GenerateTask bool `bson:"generate_task,omitempty" json:"generate_task,omitempty"`
	// GeneratedTasks indicates that the task has already generated other tasks. This fields
	// allows us to noop future requests, since a task should only generate others once.
	GeneratedTasks bool `bson:"generated_tasks,omitempty" json:"generated_tasks,omitempty"`
	// GeneratedBy, if present, is the ID of the task that generated this task.
	GeneratedBy string `bson:"generated_by,omitempty" json:"generated_by,omitempty"`
	// GeneratedJSONAsString is the configuration information to update the
	// project YAML for generate.tasks. This is only used to store the
	// configuration if GeneratedJSONStorageMethod is unset or is explicitly set
	// to "db".
	GeneratedJSONAsString GeneratedJSONFiles `bson:"generated_json,omitempty" json:"generated_json,omitempty"`
	// GeneratedJSONStorageMethod describes how the generated JSON for
	// generate.tasks is stored for this task before it's merged with the
	// existing project YAML.
	GeneratedJSONStorageMethod evergreen.ParserProjectStorageMethod `bson:"generated_json_storage_method,omitempty" json:"generated_json_storage_method,omitempty"`
	// GenerateTasksError any encountered while generating tasks.
	GenerateTasksError string `bson:"generate_error,omitempty" json:"generate_error,omitempty"`
	// GeneratedTasksToActivate is only populated if we want to override activation for these generated tasks, because of stepback.
	// Maps the build variant to a list of task names.
	GeneratedTasksToActivate map[string][]string `bson:"generated_tasks_to_stepback,omitempty" json:"generated_tasks_to_stepback,omitempty"`
	// NumGeneratedTasks is the number of tasks that this task has generated.
	NumGeneratedTasks int `bson:"num_generated_tasks,omitempty" json:"num_generated_tasks,omitempty"`
	// EstimatedNumGeneratedTasks is the estimated number of tasks that this task will generate.
	EstimatedNumGeneratedTasks *int `bson:"estimated_num_generated_tasks,omitempty" json:"estimated_num_generated_tasks,omitempty"`
	// NumActivatedGeneratedTasks is the number of tasks that this task has generated and activated.
	NumActivatedGeneratedTasks int `bson:"num_activated_generated_tasks,omitempty" json:"num_activated_generated_tasks,omitempty"`
	// EstimatedNumActivatedGeneratedTasks is the estimated number of tasks that this task will generate and activate.
	EstimatedNumActivatedGeneratedTasks *int `bson:"estimated_num_activated_generated_tasks,omitempty" json:"estimated_num_activated_generated_tasks,omitempty"`

	// Fields set if triggered by an upstream build.
	TriggerID    string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	TriggerType  string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`

	// IsEssentialToSucceed indicates that this task must finish in order for
	// its build and version to be considered successful. For example, tasks
	// selected by the GitHub PR alias must succeed for the GitHub PR requester
	// before its build or version can be reported as successful, but tasks
	// manually scheduled by the user afterwards are not required.
	IsEssentialToSucceed bool `bson:"is_essential_to_succeed" json:"is_essential_to_succeed"`
	// HasAnnotations indicates whether there exist task annotations with this task's
	// execution and id that have a populated Issues key
	HasAnnotations bool `bson:"has_annotations" json:"has_annotations"`

	// NumNextTaskDispatches is the number of times the task has been dispatched to run on a
	// host or in a container. This is used to determine if the task seems to be stuck.
	NumNextTaskDispatches int `bson:"num_next_task_dispatches" json:"num_next_task_dispatches"`

	// CachedProjectStorageMethod is a cached value how the parser project for this task's version was
	// stored at the time this task was created. If this is empty, the default storage method is StorageMethodDB.
	CachedProjectStorageMethod evergreen.ParserProjectStorageMethod `bson:"cached_project_storage_method" json:"cached_project_storage_method,omitempty"`

	// TestSelectionEnabled indicates whether test selection is enabled for this
	// task.
	TestSelectionEnabled bool `bson:"test_selection_enabled" json:"test_selection_enabled"`
}

// GeneratedJSONFiles represent files used by a task for generate.tasks to update the project YAML.
type GeneratedJSONFiles []string

// StepbackInfo helps determine which task to bisect to when performing stepback.
type StepbackInfo struct {
	// LastFailingStepbackTaskId stores the last failing task while doing stepback.
	LastFailingStepbackTaskId string `bson:"last_failing_stepback_task_id,omitempty" json:"last_failing_stepback_task_id"`
	// LastPassingStepbackTaskId stores the last passing task while doing stepback.
	LastPassingStepbackTaskId string `bson:"last_passing_stepback_task_id,omitempty" json:"last_passing_stepback_task_id"`
	// NextStepbackTaskId stores the next task id to stepback to when doing bisect stepback. This
	// is the middle of LastFailingStepbackTaskId and LastPassingStepbackTaskId of the last iteration.
	NextStepbackTaskId string `bson:"next_stepback_task_id,omitempty" json:"next_stepback_task_id"`
	// PreviousStepbackTaskId stores the last stepback iteration id.
	PreviousStepbackTaskId string `bson:"previous_stepback_task_id,omitempty" json:"previous_stepback_task_id"`
	// GeneratedStepbackInfo stores information on a generator for it's generated tasks.
	GeneratedStepbackInfo []StepbackInfo `bson:"generated_stepback_info,omitempty" json:"generated_stepback_info,omitempty"`

	// Generator fields only (responsible for propogating stepback in its generated tasks).
	// DisplayName is the display name of the generated task.
	DisplayName string `bson:"display_name,omitempty" json:"display_name,omitempty"`
	// BuildVariant is the build variant of the generated task.
	BuildVariant string `bson:"build_variant,omitempty" json:"build_variant,omitempty"`
}

// IsZero returns true if the StepbackInfo is empty or nil.
// It does not include GeneratedStepbackInfo in the check because
// those do not cause a generator to stepback.
func (s *StepbackInfo) IsZero() bool {
	if s == nil {
		return true
	}
	if s.LastFailingStepbackTaskId != "" && s.LastPassingStepbackTaskId != "" {
		return false
	}
	// If the other fields are set but not the ones above, the struct should be considered empty.
	return true
}

// GetStepbackInfoForGeneratedTask returns the StepbackInfo for a generated task that's
// on a generator task.
func (s *StepbackInfo) GetStepbackInfoForGeneratedTask(displayName string, buildVariant string) *StepbackInfo {
	if s == nil {
		return nil
	}
	for _, info := range s.GeneratedStepbackInfo {
		if info.DisplayName == displayName && info.BuildVariant == buildVariant {
			return &info
		}
	}
	return nil
}

// ExecutionPlatform indicates the type of environment that the task runs in.
type ExecutionPlatform string

const (
	// ExecutionPlatformHost indicates that the task runs in a host.
	ExecutionPlatformHost ExecutionPlatform = "host"
	// ExecutionPlatformContainer indicates that the task runs in a container.
	ExecutionPlatformContainer ExecutionPlatform = "container"
)

func (t *Task) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(t) }
func (t *Task) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, t) }

func (t *Task) GetTaskGroupString() string {
	return fmt.Sprintf("%s_%s_%s_%s", t.TaskGroup, t.BuildVariant, t.Project, t.Version)
}

// Dependency represents a task that must be completed before the owning
// task can be scheduled.
type Dependency struct {
	TaskId       string `bson:"_id" json:"id"`
	Status       string `bson:"status" json:"status"`
	Unattainable bool   `bson:"unattainable" json:"unattainable"`
	// Finished indicates if the task's dependency has finished running or not.
	Finished bool `bson:"finished" json:"finished"`
	// FinishedAt indicates the time the task's dependency was finished at.
	FinishedAt time.Time `bson:"finished_at,omitempty" json:"finished_at,omitempty"`
	// OmitGeneratedTasks causes tasks that depend on a generator task to not depend on
	// the generated tasks if this is set
	OmitGeneratedTasks bool `bson:"omit_generated_tasks,omitempty" json:"omit_generated_tasks,omitempty"`
}

// BaseTaskInfo is a subset of task fields that should be returned for patch tasks.
// The bson keys must match those of the actual task document
type BaseTaskInfo struct {
	Id     string `bson:"_id" json:"id"`
	Status string `bson:"status" json:"status"`
}

type HostCreateDetail struct {
	HostId string `bson:"host_id" json:"host_id"`
	Error  string `bson:"error" json:"error"`
}

func (d *Dependency) UnmarshalBSON(in []byte) error {
	return mgobson.Unmarshal(in, d)
}

// SetBSON allows us to use dependency representation of both
// just task Ids and of true Dependency structs.
//
//	TODO eventually drop all of this switching
func (d *Dependency) SetBSON(raw mgobson.Raw) error {
	// copy the Dependency type to remove this SetBSON method but preserve bson struct tags
	type nakedDep Dependency
	var depCopy nakedDep
	if err := raw.Unmarshal(&depCopy); err == nil {
		if depCopy.TaskId != "" {
			*d = Dependency(depCopy)
			return nil
		}
	}

	// hack to support the legacy depends_on, since we can't just unmarshal a string
	strBytes, _ := mgobson.Marshal(mgobson.RawD{{Name: "str", Value: raw}})
	var strStruct struct {
		String string `bson:"str"`
	}
	if err := mgobson.Unmarshal(strBytes, &strStruct); err == nil {
		if strStruct.String != "" {
			d.TaskId = strStruct.String
			d.Status = evergreen.TaskSucceeded
			return nil
		}
	}

	return mgobson.SetZero
}

type AbortInfo struct {
	User       string `bson:"user,omitempty" json:"user,omitempty"`
	TaskID     string `bson:"task_id,omitempty" json:"task_id,omitempty"`
	NewVersion string `bson:"new_version,omitempty" json:"new_version,omitempty"`
	PRClosed   bool   `bson:"pr_closed,omitempty" json:"pr_closed,omitempty"`
}

var (
	AllStatuses = "*"
)

// IsAbortable returns true if the task can be aborted.
func (t *Task) IsAbortable() bool {
	return t.Status == evergreen.TaskStarted ||
		t.Status == evergreen.TaskDispatched
}

// IsFinished returns true if the task is no longer running
func (t *Task) IsFinished() bool {
	return evergreen.IsFinishedTaskStatus(t.Status)
}

// IsHostDispatchable returns true if the task should run on a host and can be
// dispatched.
func (t *Task) IsHostDispatchable() bool {
	return t.IsHostTask() && t.WillRun()
}

// IsHostTask returns true if it's a task that runs on hosts.
func (t *Task) IsHostTask() bool {
	return (t.ExecutionPlatform == "" || t.ExecutionPlatform == ExecutionPlatformHost) && !t.DisplayOnly
}

// IsStuckTask returns true if the task has been dispatched over the system limit
func (t *Task) IsStuckTask() bool {
	return t.NumNextTaskDispatches >= evergreen.MaxTaskDispatchAttempts
}

// IsRestartFailedOnly returns true if the task should only restart failed tests.
func (t *Task) IsRestartFailedOnly() bool {
	return t.ResetFailedWhenFinished && !t.ResetWhenFinished
}

// SatisfiesDependency checks a task the receiver task depends on
// to see if its status satisfies a dependency. If the "Status" field is
// unset, default to checking that is succeeded.
func (t *Task) SatisfiesDependency(depTask *Task) bool {
	for _, dep := range t.DependsOn {
		if dep.TaskId == depTask.Id {
			switch dep.Status {
			case evergreen.TaskSucceeded, "":
				return depTask.Status == evergreen.TaskSucceeded
			case evergreen.TaskFailed:
				return depTask.Status == evergreen.TaskFailed
			case AllStatuses:
				return depTask.Status == evergreen.TaskFailed || depTask.Status == evergreen.TaskSucceeded || depTask.Blocked()
			}
		}
	}
	return false
}

func (t *Task) IsPatchRequest() bool {
	return utility.StringSliceContains(evergreen.PatchRequesters, t.Requester)
}

// IsUnfinishedSystemUnresponsive returns true only if this is an unfinished system unresponsive task (i.e. not on max execution)
func (t *Task) IsUnfinishedSystemUnresponsive() bool {
	return t.isSystemUnresponsive() && t.Execution < evergreen.GetEnvironment().Settings().TaskLimits.MaxTaskExecution
}

func (t *Task) isSystemUnresponsive() bool {
	// this is a legacy case
	if t.Status == evergreen.TaskSystemUnresponse {
		return true
	}

	if t.Details.Type == evergreen.CommandTypeSystem && t.Details.TimedOut && t.Details.Description == evergreen.TaskDescriptionHeartbeat {
		return true
	}
	return false
}

func (t *Task) SetOverrideDependencies(ctx context.Context, userID string) error {
	dependenciesMetTime := time.Now()
	t.OverrideDependencies = true
	t.DependenciesMetTime = dependenciesMetTime
	t.DisplayStatusCache = t.DetermineDisplayStatus()
	event.LogTaskDependenciesOverridden(ctx, t.Id, t.Execution, userID)
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				OverrideDependenciesKey: true,
				DependenciesMetTimeKey:  dependenciesMetTime,
				DisplayStatusCacheKey:   t.DisplayStatusCache,
			},
		},
	)
}

func (t *Task) AddDependency(ctx context.Context, d Dependency) error {
	// ensure the dependency doesn't already exist
	for _, existingDependency := range t.DependsOn {
		if d.TaskId == t.Id {
			grip.Error(message.Fields{
				"message": "task is attempting to add a dependency on itself, skipping this dependency",
				"task_id": t.Id,
				"stack":   string(debug.Stack()),
			})
			return nil
		}
		if existingDependency.TaskId == d.TaskId && existingDependency.Status == d.Status {
			if existingDependency.Unattainable == d.Unattainable {
				return nil // nothing to be done
			}
			updatedTasks, err := MarkAllForUnattainableDependencies(ctx, []Task{*t}, []string{existingDependency.TaskId}, d.Unattainable)
			if err != nil {
				return errors.Wrapf(err, "updating matching dependency '%s' for task '%s'", existingDependency.TaskId, t.Id)
			}
			*t = updatedTasks[0]
			return nil
		}
	}
	t.DependsOn = append(t.DependsOn, d)
	t.DisplayStatusCache = t.DetermineDisplayStatus()
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$push": bson.M{
				DependsOnKey: d,
			},
			"$set": bson.M{
				DisplayStatusCacheKey: t.DisplayStatusCache,
			},
		},
	)
}

// DependenciesMet checks whether the dependencies for the task have all completed successfully.
// If any of the dependencies exist in the map that is passed in, they are
// used to check rather than fetching from the database. All queries
// are cached back into the map for later use.
func (t *Task) DependenciesMet(ctx context.Context, depCaches map[string]Task) (bool, error) {
	if t.HasDependenciesMet() {
		return true, nil
	}

	_, err := t.populateDependencyTaskCache(ctx, depCaches)
	if err != nil {
		return false, errors.WithStack(err)
	}

	for _, dependency := range t.DependsOn {
		depTask, err := populateDependencyTaskCacheSingular(ctx, depCaches, dependency.TaskId)
		if err != nil {
			return false, err
		}
		if !t.SatisfiesDependency(depTask) {
			return false, nil
		}
	}

	t.setDependenciesMetTime()
	err = UpdateOne(
		ctx,
		bson.M{IdKey: t.Id},
		bson.M{
			"$set": bson.M{
				DependenciesMetTimeKey: t.DependenciesMetTime,
			},
		})
	grip.Error(message.WrapError(err, message.Fields{
		"message": "task.DependenciesMet() failed to update task",
		"task_id": t.Id}))

	return true, nil
}

func (t *Task) setDependenciesMetTime() {
	dependenciesMetTime := utility.ZeroTime
	for _, dependency := range t.DependsOn {
		if !utility.IsZeroTime(dependency.FinishedAt) && dependency.FinishedAt.After(dependenciesMetTime) {
			dependenciesMetTime = dependency.FinishedAt
		}
	}
	if utility.IsZeroTime(dependenciesMetTime) {
		dependenciesMetTime = time.Now()
	}
	t.DependenciesMetTime = dependenciesMetTime
}

// populateDependencyTaskCache ensures that all the dependencies for the task are in the cache.
func (t *Task) populateDependencyTaskCache(ctx context.Context, depCache map[string]Task) ([]Task, error) {
	var deps []Task
	depIdsToQueryFor := make([]string, 0, len(t.DependsOn))
	for _, dep := range t.DependsOn {
		if cachedDep, ok := depCache[dep.TaskId]; !ok {
			depIdsToQueryFor = append(depIdsToQueryFor, dep.TaskId)
		} else {
			deps = append(deps, cachedDep)
		}
	}

	if len(depIdsToQueryFor) > 0 {
		newDeps, err := FindWithFields(ctx, ByIds(depIdsToQueryFor), StatusKey, DependsOnKey, ActivatedKey)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// add queried dependencies to the cache
		for _, newDep := range newDeps {
			deps = append(deps, newDep)
			depCache[newDep.Id] = newDep
		}
	}

	return deps, nil
}

// GetFinishedBlockingDependencies gets all blocking tasks that are finished or blocked.
func (t *Task) GetFinishedBlockingDependencies(ctx context.Context, depCache map[string]Task) ([]Task, error) {
	if len(t.DependsOn) == 0 || t.OverrideDependencies {
		return nil, nil
	}

	// do this early to avoid caching tasks we won't need.
	for _, dep := range t.DependsOn {
		if dep.Unattainable {
			return nil, nil
		}
	}

	_, err := t.populateDependencyTaskCache(ctx, depCache)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	blockedDeps := []Task{}
	for _, dep := range t.DependsOn {
		depTask, ok := depCache[dep.TaskId]
		if !ok {
			return nil, errors.Errorf("task '%s' is not in the cache", dep.TaskId)
		}
		if t.SatisfiesDependency(&depTask) {
			continue
		}
		// If it is finished and did not statisfy the dependency, it is blocked.
		if depTask.IsFinished() || depTask.Blocked() {
			blockedDeps = append(blockedDeps, depTask)
		}
	}

	return blockedDeps, nil
}

// GetDeactivatedBlockingDependencies gets all blocking tasks that are not finished and are not activated.
// These tasks are not going to run unless they are manually activated.
func (t *Task) GetDeactivatedBlockingDependencies(ctx context.Context, depCache map[string]Task) ([]string, error) {
	_, err := t.populateDependencyTaskCache(ctx, depCache)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	blockingDeps := []string{}
	for _, dep := range t.DependsOn {
		depTask, err := populateDependencyTaskCacheSingular(ctx, depCache, dep.TaskId)
		if err != nil {
			return nil, err
		}
		if !depTask.IsFinished() && !depTask.Activated {
			blockingDeps = append(blockingDeps, depTask.Id)
		}
	}

	return blockingDeps, nil
}

// populateDependencyTaskCacheSingular ensures that a single dependency for the task is in the cache.
// And if it is not, it queries the database for it.
func populateDependencyTaskCacheSingular(ctx context.Context, depCache map[string]Task, depId string) (*Task, error) {
	if depTask, ok := depCache[depId]; ok {
		return &depTask, nil
	}
	foundTask, err := FindOneId(ctx, depId)
	if err != nil {
		return nil, errors.Wrap(err, "finding dependency")
	}
	if foundTask == nil {
		return nil, errors.Errorf("dependency '%s' not found", depId)
	}
	depCache[foundTask.Id] = *foundTask
	return foundTask, nil
}

// AllDependenciesSatisfied inspects the tasks first-order
// dependencies with regards to the cached tasks, and reports if all
// of the dependencies have been satisfied.
//
// If the cached tasks do not include a dependency specified by one of
// the tasks, the function returns an error.
func (t *Task) AllDependenciesSatisfied(ctx context.Context, cache map[string]Task) (bool, error) {
	if len(t.DependsOn) == 0 {
		return true, nil
	}

	catcher := grip.NewBasicCatcher()
	deps := []Task{}
	for _, dep := range t.DependsOn {
		cachedDep, err := populateDependencyTaskCacheSingular(ctx, cache, dep.TaskId)
		if err != nil {
			return false, err
		}
		deps = append(deps, *cachedDep)
	}

	if catcher.HasErrors() {
		return false, catcher.Resolve()
	}

	for _, depTask := range deps {
		if !t.SatisfiesDependency(&depTask) {
			return false, nil
		}
	}

	return true, nil
}

// MarkDependenciesFinished updates all direct dependencies on this task to
// cache whether this task has finished running, and at what time it finished (if applicable).
func (t *Task) MarkDependenciesFinished(ctx context.Context, finished bool) error {
	if t.DisplayOnly {
		// This update can be skipped for display tasks since tasks are not
		// allowed to have dependencies on display tasks.
		return nil
	}

	finishedAt := t.FinishTime
	if !finished {
		finishedAt = utility.ZeroTime
	}

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx,
		bson.M{
			DependsOnKey: bson.M{"$elemMatch": bson.M{
				DependencyTaskIdKey: t.Id,
			}},
		},
		bson.M{
			"$set": bson.M{
				bsonutil.GetDottedKeyName(DependsOnKey, "$[elem]", DependencyFinishedKey):   finished,
				bsonutil.GetDottedKeyName(DependsOnKey, "$[elem]", DependencyFinishedAtKey): finishedAt,
			},
		},
		options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
			bson.M{bsonutil.GetDottedKeyName("elem", DependencyTaskIdKey): t.Id},
		}}),
	)
	if err != nil {
		return errors.Wrap(err, "marking finished dependencies")
	}

	return nil
}

// FindTaskOnBaseCommit returns the task that is on the base commit.
func (t *Task) FindTaskOnBaseCommit(ctx context.Context) (*Task, error) {
	return FindOne(ctx, db.Query(ByCommit(t.Revision, t.BuildVariant, t.DisplayName, t.Project, evergreen.RepotrackerVersionRequester)))
}

func (t *Task) FindTaskOnPreviousCommit(ctx context.Context) (*Task, error) {
	return FindOne(ctx, db.Query(ByPreviousCommit(t.BuildVariant, t.DisplayName, t.Project, evergreen.RepotrackerVersionRequester, t.RevisionOrderNumber)).Sort([]string{"-" + RevisionOrderNumberKey}))
}

// Find the previously completed task for the same project +
// build variant + display name combination as the specified task
func (t *Task) PreviousCompletedTask(ctx context.Context, project string, statuses []string) (*Task, error) {
	if len(statuses) == 0 {
		statuses = evergreen.TaskCompletedStatuses
	}
	query := db.Query(ByBeforeRevisionWithStatusesAndRequesters(t.RevisionOrderNumber, statuses, t.BuildVariant,
		t.DisplayName, project, evergreen.SystemVersionRequesterTypes)).Sort([]string{"-" + RevisionOrderNumberKey})
	return FindOne(ctx, query)
}

func (t *Task) cacheExpectedDuration(ctx context.Context) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				DurationPredictionKey:     t.DurationPrediction,
				ExpectedDurationKey:       t.DurationPrediction.Value,
				ExpectedDurationStddevKey: t.DurationPrediction.StdDev,
			},
		},
	)
}

// MarkAsHostDispatched marks that the task has been dispatched onto a
// particular host. If the task is part of a display task, the display task is
// also marked as dispatched to a host. Returns an error if any of the database
// updates fail.
func (t *Task) MarkAsHostDispatched(ctx context.Context, hostID, distroID, agentRevision string, dispatchTime time.Time) error {
	doUpdate := func(update []bson.M) error {
		return UpdateOne(ctx, bson.M{IdKey: t.Id}, update)
	}
	if err := t.markAsHostDispatchedWithFunc(doUpdate, hostID, distroID, agentRevision, dispatchTime); err != nil {
		return err
	}

	// When dispatching an execution task, mark its parent as dispatched.
	if dt, _ := t.GetDisplayTask(ctx); dt != nil && dt.DispatchTime == utility.ZeroTime {
		return dt.MarkAsHostDispatched(ctx, "", "", "", dispatchTime)
	}
	return nil
}

// MarkAsHostDispatchedWithEnv marks that the task has been dispatched onto
// a particular host. Unlike MarkAsHostDispatched, this does not update the
// parent display task.
func (t *Task) MarkAsHostDispatchedWithEnv(ctx context.Context, env evergreen.Environment, hostID, distroID, agentRevision string, dispatchTime time.Time) error {
	doUpdate := func(update []bson.M) error {
		_, err := env.DB().Collection(Collection).UpdateByID(ctx, t.Id, update)
		return err
	}
	return t.markAsHostDispatchedWithFunc(doUpdate, hostID, distroID, agentRevision, dispatchTime)
}

func (t *Task) markAsHostDispatchedWithFunc(doUpdate func(update []bson.M) error, hostID, distroID, agentRevision string, dispatchTime time.Time) error {

	set := bson.M{
		DispatchTimeKey:  dispatchTime,
		StatusKey:        evergreen.TaskDispatched,
		HostIdKey:        hostID,
		LastHeartbeatKey: dispatchTime,
		DistroIdKey:      distroID,
		AgentVersionKey:  agentRevision,
	}
	output, ok := t.initializeTaskOutputInfo(evergreen.GetEnvironment())
	if ok {
		set[TaskOutputInfoKey] = output
	}
	if err := doUpdate([]bson.M{
		{
			"$set": set,
		},
		{
			"$unset": []string{
				AbortedKey,
				AbortInfoKey,
				DetailsKey,
			},
		},
		addDisplayStatusCache,
	}); err != nil {
		return err
	}

	t.DispatchTime = dispatchTime
	t.Status = evergreen.TaskDispatched
	t.HostId = hostID
	t.AgentVersion = agentRevision
	t.TaskOutputInfo = output
	t.LastHeartbeat = dispatchTime
	t.DistroId = distroID
	t.Aborted = false
	t.AbortInfo = AbortInfo{}
	t.Details = apimodels.TaskEndDetail{}
	t.DisplayStatusCache = t.DetermineDisplayStatus()

	return nil
}

// MarkAsHostUndispatched marks that the host task is undispatched.
// If the task is already dispatched to a host, it aborts the dispatch by
// undoing the dispatch updates. This is the inverse operation of
// MarkAsHostDispatchedWithEnv.
func (t *Task) MarkAsHostUndispatched(ctx context.Context, env evergreen.Environment) error {
	doUpdate := func(update []bson.M) error {
		_, err := env.DB().Collection(Collection).UpdateByID(ctx, t.Id, update)
		return err
	}
	return t.markAsHostUndispatchedWithFunc(doUpdate)
}

func (t *Task) markAsHostUndispatchedWithFunc(doUpdate func(update []bson.M) error) error {
	update := []bson.M{
		{
			"$set": bson.M{
				StatusKey:        evergreen.TaskUndispatched,
				DispatchTimeKey:  utility.ZeroTime,
				LastHeartbeatKey: utility.ZeroTime,
			},
		},
		{
			"$unset": bson.A{
				HostIdKey,
				AgentVersionKey,
				TaskOutputInfoKey,
				AbortedKey,
				AbortInfoKey,
				DetailsKey,
			},
		},
		addDisplayStatusCache,
	}

	if err := doUpdate(update); err != nil {
		return err
	}

	t.Status = evergreen.TaskUndispatched
	t.DispatchTime = utility.ZeroTime
	t.LastHeartbeat = utility.ZeroTime
	t.HostId = ""
	t.AgentVersion = ""
	t.TaskOutputInfo = nil
	t.Aborted = false
	t.AbortInfo = AbortInfo{}
	t.Details = apimodels.TaskEndDetail{}
	t.DisplayStatusCache = t.DetermineDisplayStatus()

	return nil
}

// MarkGeneratedTasks marks that the task has generated tasks.
func MarkGeneratedTasks(ctx context.Context, taskID string) error {
	query := bson.M{
		IdKey:             taskID,
		GeneratedTasksKey: bson.M{"$exists": false},
	}
	update := bson.M{
		"$set": bson.M{
			GeneratedTasksKey: true,
		},
		"$unset": bson.M{
			GenerateTasksErrorKey: 1,
		},
	}
	err := UpdateOne(ctx, query, update)
	if adb.ResultsNotFound(err) {
		return nil
	}
	return errors.Wrap(err, "marking generate.tasks complete")
}

// MarkGeneratedTasksErr marks that the task hit errors generating tasks.
func MarkGeneratedTasksErr(ctx context.Context, taskID string, errorToSet error) error {
	if errorToSet == nil || adb.ResultsNotFound(errorToSet) || db.IsDuplicateKey(errorToSet) {
		return nil
	}
	query := bson.M{
		IdKey:             taskID,
		GeneratedTasksKey: bson.M{"$exists": false},
	}
	update := bson.M{
		"$set": bson.M{
			GenerateTasksErrorKey: errorToSet.Error(),
		},
	}
	err := UpdateOne(ctx, query, update)
	if adb.ResultsNotFound(err) {
		return nil
	}
	return errors.Wrap(err, "setting generate.tasks error")
}

// GenerateNotRun returns tasks that have requested to generate tasks.
func GenerateNotRun(ctx context.Context) ([]Task, error) {
	const maxGenerateTimeAgo = 24 * time.Hour
	return FindAll(ctx, db.Query(bson.M{
		StatusKey:                evergreen.TaskStarted,                              // task is running
		StartTimeKey:             bson.M{"$gt": time.Now().Add(-maxGenerateTimeAgo)}, // ignore older tasks, just in case
		GeneratedTasksKey:        bson.M{"$ne": true},                                // generate.tasks has not yet run
		GeneratedJSONAsStringKey: bson.M{"$exists": true},                            // config has been posted by generate.tasks command
	}))
}

// SetGeneratedJSON sets JSON data to generate tasks from. If the generated JSON
// files have already been stored, this is a no-op.
func (t *Task) SetGeneratedJSON(ctx context.Context, files GeneratedJSONFiles) error {
	if len(t.GeneratedJSONAsString) > 0 || t.GeneratedJSONStorageMethod != "" {
		return nil
	}

	if err := UpdateOne(
		ctx,
		bson.M{
			IdKey:                         t.Id,
			GeneratedJSONAsStringKey:      bson.M{"$exists": false},
			GeneratedJSONStorageMethodKey: nil,
		},
		bson.M{
			"$set": bson.M{
				GeneratedJSONAsStringKey:      files,
				GeneratedJSONStorageMethodKey: evergreen.ProjectStorageMethodDB,
			},
		},
	); err != nil {
		return err
	}

	t.GeneratedJSONAsString = files
	t.GeneratedJSONStorageMethod = evergreen.ProjectStorageMethodDB

	return nil
}

// SetGeneratedJSONStorageMethod sets the task's generated JSON file storage
// method. If it's already been set, this is a no-op.
func (t *Task) SetGeneratedJSONStorageMethod(ctx context.Context, method evergreen.ParserProjectStorageMethod) error {
	if t.GeneratedJSONStorageMethod != "" {
		return nil
	}

	if err := UpdateOne(
		ctx,
		bson.M{
			IdKey:                         t.Id,
			GeneratedJSONStorageMethodKey: nil,
		},
		bson.M{
			"$set": bson.M{
				GeneratedJSONStorageMethodKey: method,
			},
		},
	); err != nil {
		return err
	}

	t.GeneratedJSONStorageMethod = method

	return nil
}

// SetGeneratedTasksToActivate adds a task to stepback after activation
func (t *Task) SetGeneratedTasksToActivate(ctx context.Context, buildVariantName, taskName string) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$addToSet": bson.M{
				bsonutil.GetDottedKeyName(GeneratedTasksToActivateKey, buildVariantName): taskName,
			},
		},
	)
}

// SetTasksScheduledAndDepsMetTime takes a list of tasks and a time, and then sets
// the scheduled time in the database for the tasks if ScheduledTime is currently unset, and
// does the same for the dependencies met time for the tasks if the task has its
// dependencies met and DependenciesMetTime is unset.
func SetTasksScheduledAndDepsMetTime(ctx context.Context, tasks []Task, scheduledTime time.Time) error {
	idsToSchedule := []string{}
	idsToSetDependenciesMet := []string{}
	for i := range tasks {
		// Skip tasks with scheduled time to prevent large updates
		if utility.IsZeroTime(tasks[i].ScheduledTime) {
			tasks[i].ScheduledTime = scheduledTime
			idsToSchedule = append(idsToSchedule, tasks[i].Id)
		}
		if utility.IsZeroTime(tasks[i].DependenciesMetTime) && tasks[i].HasDependenciesMet() {
			tasks[i].DependenciesMetTime = scheduledTime
			idsToSetDependenciesMet = append(idsToSetDependenciesMet, tasks[i].Id)
		}

		// Display tasks are considered scheduled when their first exec task is scheduled
		if tasks[i].IsPartOfDisplay(ctx) {
			idsToSchedule = append(idsToSchedule, utility.FromStringPtr(tasks[i].DisplayTaskId))
			idsToSetDependenciesMet = append(idsToSetDependenciesMet, utility.FromStringPtr(tasks[i].DisplayTaskId))
		}
	}
	// Remove duplicates to prevent large updates
	uniqueIDsToSchedule := utility.UniqueStrings(idsToSchedule)
	uniqueIDsToSetDependenciesMet := utility.UniqueStrings(idsToSetDependenciesMet)

	if err := setScheduledTimeForTasks(ctx, uniqueIDsToSchedule, scheduledTime); err != nil {
		return errors.Wrap(err, "setting scheduled time for tasks")
	}
	if err := setDependenciesMetTimeForTasks(ctx, uniqueIDsToSetDependenciesMet, scheduledTime); err != nil {
		return errors.Wrap(err, "setting dependencies met time for tasks")
	}
	return nil
}

func setScheduledTimeForTasks(ctx context.Context, uniqueIDsToSchedule []string, scheduledTime time.Time) error {
	if len(uniqueIDsToSchedule) == 0 {
		return nil
	}
	_, err := UpdateAll(
		ctx,
		bson.M{
			IdKey: bson.M{
				"$in": uniqueIDsToSchedule,
			},
			ScheduledTimeKey: bson.M{
				"$lte": utility.ZeroTime,
			},
		},
		bson.M{
			"$set": bson.M{
				ScheduledTimeKey: scheduledTime,
			},
		},
	)
	if err != nil {
		return err
	}
	return nil
}

func setDependenciesMetTimeForTasks(ctx context.Context, uniqueIDsToSetDependenciesMet []string, dependenciesMetTime time.Time) error {
	if len(uniqueIDsToSetDependenciesMet) == 0 {
		return nil
	}
	_, err := UpdateAll(
		ctx,
		bson.M{
			IdKey: bson.M{
				"$in": uniqueIDsToSetDependenciesMet,
			},
			DependenciesMetTimeKey: bson.M{
				"$lte": utility.ZeroTime,
			},
		},
		bson.M{
			"$set": bson.M{
				DependenciesMetTimeKey: dependenciesMetTime,
			},
		},
	)
	if err != nil {
		return err
	}
	return nil
}

// ByBeforeMidwayTaskFromIds tries to get the midway task between two tasks
// but if it does not find it (i.e. periodic builds), it gets the closest task
// (with lower order number). If there are no matching tasks, or the task it
// gets is out of bounds, it returns the given lower order revision task.
//
// It verifies that the tasks are from the same project, requester,
// build variant, and display name.
func ByBeforeMidwayTaskFromIds(ctx context.Context, t1Id, t2Id string) (*Task, error) {
	t1, err := FindOneId(ctx, t1Id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task id '%s'", t1Id)
	}
	if t1 == nil {
		return nil, errors.Errorf("could not find task id '%s'", t1Id)
	}
	t2, err := FindOneId(ctx, t2Id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task id '%s'", t2Id)
	}
	if t2 == nil {
		return nil, errors.Errorf("could not find task id '%s'", t2Id)
	}

	// The tasks should be the same build variant, display name, project, and requester.
	catcher := grip.NewBasicCatcher() // Makes an error accumulator
	catcher.ErrorfWhen(t1.BuildVariant != t2.BuildVariant, "given tasks have differing build variants '%s' and '%s'", t1.BuildVariant, t2.BuildVariant)
	catcher.ErrorfWhen(t1.DisplayName != t2.DisplayName, "given tasks have differing display name '%s' and '%s'", t1.DisplayName, t2.DisplayName)
	catcher.ErrorfWhen(t1.Project != t2.Project, "given tasks have differing project '%s' and '%s'", t1.Project, t2.Project)
	catcher.ErrorfWhen(t1.Requester != t2.Requester, "given tasks have differing requester '%s' and '%s'", t1.Requester, t2.Requester)
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	middleOrderNumber := (t1.RevisionOrderNumber + t2.RevisionOrderNumber) / 2
	filter, sort := ByBeforeRevision(middleOrderNumber+1, t1.BuildVariant, t1.DisplayName, t1.Project, t1.Requester)
	query := db.Query(filter).Sort(sort)

	task, err := FindOne(ctx, query)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task between '%s' and '%s'", t1Id, t2Id)
	}
	if task == nil {
		return nil, errors.Errorf("could not find task between '%s' and '%s'", t1Id, t2Id)
	}

	lowerBoundTask := t1
	upperBoundTask := t2
	// If t1 is after t2, t1 is our upper bound and t2 is our lower bound.
	if t1.RevisionOrderNumber > t2.RevisionOrderNumber {
		upperBoundTask = t1
		lowerBoundTask = t2
	}
	if task.RevisionOrderNumber >= upperBoundTask.RevisionOrderNumber ||
		task.RevisionOrderNumber <= lowerBoundTask.RevisionOrderNumber {
		grip.Info(message.Fields{
			"message":                 "found midway task is out of bounds",
			"t1_id":                   t1Id,
			"t1_order_number":         t1.RevisionOrderNumber,
			"t2_id":                   t2Id,
			"t2_order_number":         t2.RevisionOrderNumber,
			"found_task":              task.Id,
			"found_task_order_number": task.RevisionOrderNumber,
		})
		// We return the lower bound task if the found task is out of bounds.
		return lowerBoundTask, nil
	}

	return task, nil
}

// UnscheduleStaleUnderwaterHostTasks Removes host tasks older than the unschedulable threshold (e.g. one week) from
// the scheduler queue.
// If you pass an empty string as an argument to this function, this operation
// will select tasks from all distros.
func UnscheduleStaleUnderwaterHostTasks(ctx context.Context, distroID string) ([]Task, error) {
	query := schedulableHostTasksQuery()

	if err := addApplicableDistroFilter(ctx, distroID, DistroIdKey, query); err != nil {
		return nil, errors.WithStack(err)
	}

	query[ActivatedTimeKey] = bson.M{"$lte": time.Now().Add(-UnschedulableThreshold)}

	tasks, err := FindAll(ctx, db.Query(query))
	if err != nil {
		return nil, errors.Wrap(err, "finding matching tasks")
	}
	update := []bson.M{
		{
			"$set": bson.M{
				PriorityKey:  evergreen.DisabledTaskPriority,
				ActivatedKey: false,
			},
		},
		addDisplayStatusCache,
	}

	// Force the query to use 'distro_1_status_1_activated_1_priority_1_override_dependencies_1_unattainable_dependency_1'
	// instead of defaulting to 'status_1_depends_on.status_1_depends_on.unattainable_1'.
	_, err = UpdateAllWithHint(ctx, query, update, ActivatedTasksByDistroIndex)
	if err != nil {
		return nil, errors.Wrap(err, "unscheduling stale underwater tasks")
	}
	for _, modifiedTask := range tasks {
		event.LogTaskPriority(ctx, modifiedTask.Id, modifiedTask.Execution, evergreen.UnderwaterTaskUnscheduler, evergreen.DisabledTaskPriority)
	}
	return tasks, nil
}

// DeactivateStepbackTask deactivates and aborts the matching stepback task.
func DeactivateStepbackTask(ctx context.Context, projectId, buildVariantName, taskName, caller string) error {
	t, err := FindActivatedStepbackTaskByName(ctx, projectId, buildVariantName, taskName)
	if err != nil {
		return err
	}
	if t == nil {
		return errors.Errorf("no stepback task '%s' for variant '%s' found", taskName, buildVariantName)
	}

	if err = DeactivateTasks(ctx, []Task{*t}, true, caller); err != nil {
		return errors.Wrap(err, "deactivating stepback task")
	}
	if t.IsAbortable() {
		event.LogTaskAbortRequest(ctx, t.Id, t.Execution, caller)
		if err = t.SetAborted(ctx, AbortInfo{User: caller}); err != nil {
			return errors.Wrap(err, "setting task aborted")
		}
	}
	return nil
}

// MarkFailed changes the state of the task to failed.
func (t *Task) MarkFailed(ctx context.Context) error {
	t.Status = evergreen.TaskFailed
	t.DisplayStatusCache = t.DetermineDisplayStatus()
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:             evergreen.TaskFailed,
				DisplayStatusCacheKey: t.DisplayStatusCache,
			},
		},
	)
}

func (t *Task) MarkSystemFailed(ctx context.Context, description string) error {
	t.FinishTime = time.Now()
	t.Details = GetSystemFailureDetails(description)

	switch t.ExecutionPlatform {
	case ExecutionPlatformHost:
		event.LogHostTaskFinished(ctx, t.Id, t.Execution, t.HostId, evergreen.TaskSystemFailed)
	default:
		event.LogTaskFinished(ctx, t.Id, t.Execution, evergreen.TaskSystemFailed)
	}
	grip.Info(message.Fields{
		"message":            "marking task system failed",
		"included_on":        evergreen.ContainerHealthDashboard,
		"task_id":            t.Id,
		"execution":          t.Execution,
		"status":             t.Status,
		"host_id":            t.HostId,
		"description":        description,
		"execution_platform": t.ExecutionPlatform,
	})

	return t.MarkEnd(ctx, t.FinishTime, &t.Details)
}

// GetSystemFailureDetails returns a task's end details based on an input description.
func GetSystemFailureDetails(description string) apimodels.TaskEndDetail {
	details := apimodels.TaskEndDetail{
		Status:      evergreen.TaskFailed,
		Type:        evergreen.CommandTypeSystem,
		Description: description,
	}
	if description == evergreen.TaskDescriptionHeartbeat {
		details.TimedOut = true
	}
	return details
}

// SetAborted sets the abort field and abort info of task to aborted
// and prevents the task from being reset when finished.
func (t *Task) SetAborted(ctx context.Context, reason AbortInfo) error {
	t.Aborted = true
	t.DisplayStatus = t.DetermineDisplayStatus()
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		[]bson.M{
			{"$set": taskAbortUpdate(reason)},
			addDisplayStatusCache,
		},
	)
}

func taskAbortUpdate(reason AbortInfo) bson.M {
	return bson.M{
		AbortedKey:                 true,
		AbortInfoKey:               reason,
		ResetWhenFinishedKey:       false,
		ResetFailedWhenFinishedKey: false,
		IsAutomaticRestartKey:      false,
	}
}

// SetLastAndPreviousStepbackIds sets the LastFailingStepbackTaskId,
// LastPassingStepbackTaskId, and PreviousStepbackTaskId for a given task id.
func SetLastAndPreviousStepbackIds(ctx context.Context, taskId string, s StepbackInfo) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: taskId,
		},
		bson.M{
			"$set": bson.M{
				StepbackInfoKey: bson.M{
					LastFailingStepbackTaskIdKey: s.LastFailingStepbackTaskId,
					LastPassingStepbackTaskIdKey: s.LastPassingStepbackTaskId,
					PreviousStepbackTaskIdKey:    s.PreviousStepbackTaskId,
				},
			},
		},
	)
}

// AddGeneratedStepbackInfoForGenerator appends a new StepbackInfo to the
// task's GeneratedStepbackInfo.
func AddGeneratedStepbackInfoForGenerator(ctx context.Context, taskId string, s StepbackInfo) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: taskId,
		},
		bson.M{
			"$push": bson.M{
				bsonutil.GetDottedKeyName(StepbackInfoKey, GeneratedStepbackInfoKey): s,
			},
		},
	)
}

// SetGeneratedStepbackInfoForGenerator sets the StepbackInfo's GeneratedStepbackInfo
// element with the same DisplayName and BuildVariant as the input StepbackInfo.
func SetGeneratedStepbackInfoForGenerator(ctx context.Context, taskId string, s StepbackInfo) error {
	r, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateOne(ctx,
		bson.M{
			IdKey: taskId,
			StepbackInfoKey: bson.M{
				GeneratedStepbackInfoKey: bson.M{
					"$elemMatch": bson.M{
						StepbackInfoDisplayNameKey:  s.DisplayName,
						StepbackInfoBuildVariantKey: s.BuildVariant,
					},
				},
			},
		},
		bson.M{
			"$set": bson.M{
				bsonutil.GetDottedKeyName(StepbackInfoKey, GeneratedStepbackInfoKey, "$[elem]", LastFailingStepbackTaskIdKey): s.LastFailingStepbackTaskId,
				bsonutil.GetDottedKeyName(StepbackInfoKey, GeneratedStepbackInfoKey, "$[elem]", LastPassingStepbackTaskIdKey): s.LastPassingStepbackTaskId,
				bsonutil.GetDottedKeyName(StepbackInfoKey, GeneratedStepbackInfoKey, "$[elem]", NextStepbackTaskIdKey):        s.NextStepbackTaskId,
				bsonutil.GetDottedKeyName(StepbackInfoKey, GeneratedStepbackInfoKey, "$[elem]", PreviousStepbackTaskIdKey):    s.PreviousStepbackTaskId,
			},
		},
		options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
			bson.M{
				bsonutil.GetDottedKeyName("elem", DisplayNameKey):  s.DisplayName,
				bsonutil.GetDottedKeyName("elem", BuildVariantKey): s.BuildVariant,
			},
		}}),
	)
	// If no documents were modified, fallback to adding the new StepbackInfo.
	if err == nil && r.ModifiedCount == 0 {
		return AddGeneratedStepbackInfoForGenerator(ctx, taskId, s)
	}
	return err
}

// SetNextStepbackId sets the NextStepbackTaskId for a given task id.
func SetNextStepbackId(ctx context.Context, taskId string, s StepbackInfo) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: taskId,
		},
		bson.M{
			"$set": bson.M{
				bsonutil.GetDottedKeyName(StepbackInfoKey, NextStepbackTaskIdKey): s.NextStepbackTaskId,
			},
		},
	)
}

// initializeTaskOutputInfo returns the task output information with the most
// up-to-date configuration for the task run. Returns false if the task will
// never have output. This function should only be used to set the task output
// field upon task dispatch.
func (t *Task) initializeTaskOutputInfo(env evergreen.Environment) (*TaskOutput, bool) {
	if t.DisplayOnly || t.Archived {
		return nil, false
	}

	return InitializeTaskOutput(env, t.Project), true
}

// GetTaskOutputSafe returns an instantiation of the task output interface and
// whether it is safe to fetch task output data. This function should always
// be called to access task output data.
func (t *Task) GetTaskOutputSafe() (*TaskOutput, bool) {
	if t.DisplayOnly || t.Status == evergreen.TaskUndispatched {
		return nil, false
	}

	if t.TaskOutputInfo == nil {
		// Return the zero value for tasks that do not have the task
		// output metadata saved in the database. This is for backwards
		// compatibility. We can safely assume version zero for each
		// task output type.
		return &TaskOutput{}, true
	}

	return t.TaskOutputInfo, true
}

// GetTaskLogs returns the task's task logs with the given options.
func (t *Task) GetTaskLogs(ctx context.Context, getOpts TaskLogGetOptions) (log.LogIterator, error) {
	if t.DisplayOnly {
		return nil, errors.New("cannot get task logs for a display task")
	}

	tsk := t
	if t.Archived {
		tsk.Id = t.OldTaskId
	}
	return getTaskLogs(ctx, *tsk, getOpts)
}

// GetTestLogs returns the task's test logs with the specified options.
func (t *Task) GetTestLogs(ctx context.Context, getOpts TestLogGetOptions) (log.LogIterator, error) {
	if t.DisplayOnly {
		return nil, errors.New("cannot get test logs for a display task")
	}

	task := t
	if t.Archived {
		task.Id = t.OldTaskId
	}
	return getTestLogs(ctx, *task, getOpts)
}

// SetResultsInfo sets the task's test results info.
//
// Note that if failedResults is false, ResultsFailed is not set. This is
// because in cases where multiple calls to attach test results are made for a
// task, only one call needs to have a test failure for the ResultsFailed field
// to be set to true.
func (t *Task) SetResultsInfo(ctx context.Context, failedResults bool) error {
	if t.DisplayOnly {
		return errors.New("cannot set results info on a display task")
	}
	if t.HasTestResults && !failedResults {
		return nil
	}
	t.HasTestResults = true
	set := bson.M{HasTestResultsKey: true}
	if failedResults {
		t.ResultsFailed = true
		set[ResultsFailedKey] = true
	}

	return errors.WithStack(UpdateOne(ctx, ById(t.Id), bson.M{"$set": set}))
}

// HasResults returns whether the task has test results or not.
func (t *Task) HasResults(ctx context.Context) bool {
	if t.DisplayOnly && len(t.ExecutionTasks) > 0 {
		hasResults := []bson.M{{ResultsServiceKey: bson.M{"$exists": true}}, {HasTestResultsKey: true}}
		if t.Archived {
			execTasks, err := FindByExecutionTasksAndMaxExecution(ctx, t.ExecutionTasks, t.Execution, bson.E{Key: "$or", Value: hasResults})
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "getting execution tasks for archived display task",
				}))
			}

			return len(execTasks) > 0
		} else {
			query := ByIds(t.ExecutionTasks)
			query["$or"] = hasResults
			execTasksWithResults, err := Count(ctx, db.Query(query))
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "getting count of execution tasks with results for display task",
				}))
			}

			return execTasksWithResults > 0
		}
	}

	return t.ResultsService != "" || t.HasTestResults
}

// ActivateTasks sets all given tasks to active, logs them as activated, and
// proceeds to activate any dependencies that were deactivated. This returns the
// task IDs that were activated.
func ActivateTasks(ctx context.Context, tasks []Task, activationTime time.Time, updateDependencies bool, caller string) ([]string, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	tasksToActivate := make([]Task, 0, len(tasks))
	taskIDs := make([]string, 0, len(tasks))
	numEstimatedActivatedGeneratedTasks := 0
	for _, t := range tasks {
		// Activating an activated task is a noop.
		if t.Activated {
			continue
		}
		tasksToActivate = append(tasksToActivate, t)
		taskIDs = append(taskIDs, t.Id)
		numEstimatedActivatedGeneratedTasks += utility.FromIntPtr(t.EstimatedNumActivatedGeneratedTasks)
	}
	depTasksToUpdate, depTaskIDsToUpdate, err := getDependencyTaskIdsToActivate(ctx, taskIDs, updateDependencies)
	if err != nil {
		return nil, errors.Wrap(err, "getting dependency tasks to activate")
	}
	for _, depTask := range depTasksToUpdate {
		numEstimatedActivatedGeneratedTasks += utility.FromIntPtr(depTask.EstimatedNumActivatedGeneratedTasks)
	}
	// Tasks passed into this function will all be from the same version or build, so we can assume
	// all tasks also share the same requester field.
	numTasksModified := len(taskIDs) + len(depTaskIDsToUpdate) + numEstimatedActivatedGeneratedTasks
	if err = UpdateSchedulingLimit(ctx, caller, tasks[0].Requester, numTasksModified, true); err != nil {
		return nil, err
	}
	err = activateTasks(ctx, taskIDs, caller, activationTime)
	if err != nil {
		return nil, errors.Wrap(err, "activating tasks")
	}
	logs := []event.EventLogEntry{}
	for _, t := range tasksToActivate {
		logs = append(logs, event.GetTaskActivatedEvent(t.Id, t.Execution, caller))
	}
	grip.Error(message.WrapError(event.LogManyEvents(ctx, logs), message.Fields{
		"message":  "problem logging task activated events",
		"task_ids": taskIDs,
		"caller":   caller,
	}))

	activatedTaskIDs := make([]string, 0, len(taskIDs)+len(depTaskIDsToUpdate))
	activatedTaskIDs = append(activatedTaskIDs, taskIDs...)
	activatedTaskIDs = append(activatedTaskIDs, depTaskIDsToUpdate...)

	if len(depTaskIDsToUpdate) > 0 {
		return activatedTaskIDs, activateDeactivatedDependencies(ctx, depTasksToUpdate, depTaskIDsToUpdate, caller)
	}

	return activatedTaskIDs, nil
}

// UpdateSchedulingLimit retrieves a user from the DB and updates their hourly scheduling limit info
// if they are not a service user.
func UpdateSchedulingLimit(ctx context.Context, username, requester string, numTasksModified int, activated bool) error {
	if evergreen.IsSystemActivator(username) || !evergreen.IsPatchRequester(requester) || numTasksModified == 0 {
		return nil
	}
	s := evergreen.GetEnvironment().Settings()
	maxScheduledTasks := s.TaskLimits.MaxHourlyPatchTasks
	if maxScheduledTasks == 0 {
		return nil
	}
	u, err := user.FindOneById(ctx, username)
	if err != nil {
		return errors.Wrap(err, "getting user")
	}
	if u != nil && !u.OnlyAPI {
		return errors.Wrapf(u.CheckAndUpdateSchedulingLimit(ctx, maxScheduledTasks, numTasksModified, activated), "checking task scheduling limit for user '%s'", u.Id)
	}
	return nil
}

// ActivateTasksByIdsWithDependencies activates the given tasks and their dependencies.
func ActivateTasksByIdsWithDependencies(ctx context.Context, ids []string, caller string) error {
	q := db.Query(bson.M{
		IdKey:     bson.M{"$in": ids},
		StatusKey: evergreen.TaskUndispatched,
	})

	tasks, err := FindAll(ctx, q.WithFields(IdKey, DependsOnKey, ExecutionKey, ActivatedKey))
	if err != nil {
		return errors.Wrap(err, "getting tasks for activation")
	}
	dependOn, err := GetRecursiveDependenciesUp(ctx, tasks, nil)
	if err != nil {
		return errors.Wrap(err, "getting recursive dependencies")
	}

	if _, err = ActivateTasks(ctx, append(tasks, dependOn...), time.Now(), true, caller); err != nil {
		return errors.Wrap(err, "updating tasks for activation")
	}
	return nil
}

func getDependencyTaskIdsToActivate(ctx context.Context, tasks []string, updateDependencies bool) (map[string]Task, []string, error) {
	if !updateDependencies {
		return nil, nil, nil
	}
	taskMap := make(map[string]bool)
	for _, t := range tasks {
		taskMap[t] = true
	}

	tasksDependingOnTheseTasks, err := getRecursiveDependenciesDown(ctx, tasks, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting recursive dependencies down")
	}

	// do a topological sort so we've dealt with
	// all a task's dependencies by the time we get up to it
	sortedDependencies, err := topologicalSort(tasksDependingOnTheseTasks)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// Get dependencies we don't have yet and add them to a map
	tasksToGet := []string{}
	depTaskMap := make(map[string]bool)
	for _, t := range sortedDependencies {
		depTaskMap[t.Id] = true

		if t.Activated || !t.DeactivatedForDependency {
			continue
		}

		for _, dep := range t.DependsOn {
			if !taskMap[dep.TaskId] && !depTaskMap[dep.TaskId] {
				tasksToGet = append(tasksToGet, dep.TaskId)
			}
		}
	}

	missingTaskMap := make(map[string]Task)
	if len(tasksToGet) > 0 {
		var missingTasks []Task
		missingTasks, err = FindAll(ctx, db.Query(bson.M{IdKey: bson.M{"$in": tasksToGet}}).WithFields(ActivatedKey))
		if err != nil {
			return nil, nil, errors.Wrap(err, "getting missing tasks")
		}
		for _, t := range missingTasks {
			missingTaskMap[t.Id] = t
		}
	}

	tasksToActivate := make(map[string]Task)
	for _, t := range sortedDependencies {
		if t.Activated || !t.DeactivatedForDependency {
			continue
		}

		depsSatisfied := true
		for _, dep := range t.DependsOn {
			// not being activated now
			if _, ok := tasksToActivate[dep.TaskId]; !ok && !taskMap[dep.TaskId] {
				// and not already activated
				if depTask := missingTaskMap[dep.TaskId]; !depTask.Activated {
					depsSatisfied = false
					break
				}
			}
		}
		if depsSatisfied {
			tasksToActivate[t.Id] = t
		}
	}
	if len(tasksToActivate) == 0 {
		return nil, nil, nil
	}

	taskIDsToActivate := make([]string, 0, len(tasksToActivate))
	for _, t := range tasksToActivate {
		taskIDsToActivate = append(taskIDsToActivate, t.Id)
	}
	return tasksToActivate, taskIDsToActivate, nil
}

// activateDeactivatedDependencies activates tasks that depend on these tasks which were deactivated because a task
// they depended on was deactivated. Only activate when all their dependencies are activated or are being activated
func activateDeactivatedDependencies(ctx context.Context, tasksToActivate map[string]Task, taskIDsToActivate []string, caller string) error {
	// Separate tasks by whether they need predictions
	var tasksNeedingPredictions []Task
	var taskIDsWithPredictions []string
	for _, t := range tasksToActivate {
		if t.PredictedTaskCost.IsZero() {
			tasksNeedingPredictions = append(tasksNeedingPredictions, t)
		} else {
			taskIDsWithPredictions = append(taskIDsWithPredictions, t.Id)
		}
	}

	now := time.Now()

	// Activate tasks that already have predictions (no need to compute or set predictions)
	if len(taskIDsWithPredictions) > 0 {
		_, err := UpdateAll(
			ctx,
			bson.M{
				IdKey: bson.M{"$in": taskIDsWithPredictions},
			},
			[]bson.M{
				{
					"$set": bson.M{
						ActivatedKey:                true,
						DeactivatedForDependencyKey: false,
						ActivatedByKey:              caller,
						ActivatedTimeKey:            now,
					},
				},
				addDisplayStatusCache,
			})
		if err != nil {
			return errors.Wrap(err, "activating dependent tasks with existing predictions")
		}
	}

	// Compute and update predictions for tasks that need them
	if len(tasksNeedingPredictions) > 0 {
		predictions, err := computeCostPredictionsInParallel(ctx, tasksNeedingPredictions)
		if err != nil {
			return errors.Wrap(err, "computing cost predictions for dependencies")
		}

		env := evergreen.GetEnvironment()
		coll := env.DB().Collection(Collection)
		var writes []mongo.WriteModel

		for _, t := range tasksNeedingPredictions {
			prediction := predictions[t.Id]
			setFields := bson.M{
				ActivatedKey:                true,
				DeactivatedForDependencyKey: false,
				ActivatedByKey:              caller,
				ActivatedTimeKey:            now,
			}

			addPredictedCostToUpdate(setFields, prediction.PredictedCost)

			writes = append(writes, mongo.NewUpdateOneModel().
				SetFilter(bson.M{IdKey: t.Id}).
				SetUpdate([]bson.M{
					{"$set": setFields},
					addDisplayStatusCache,
				}))
		}

		if len(writes) > 0 {
			_, err := coll.BulkWrite(ctx, writes)
			if err != nil {
				return errors.Wrap(err, "bulk updating dependent tasks with new predictions")
			}
		}
	}

	logs := []event.EventLogEntry{}
	for _, t := range tasksToActivate {
		logs = append(logs, event.GetTaskActivatedEvent(t.Id, t.Execution, caller))
	}
	grip.Error(message.WrapError(event.LogManyEvents(ctx, logs), message.Fields{
		"message":  "problem logging task activated events",
		"task_ids": taskIDsToActivate,
		"caller":   caller,
	}))

	return nil
}

func topologicalSort(tasks []Task) ([]Task, error) {
	var fromTask, toTask string
	defer func() {
		taskIds := []string{}
		for _, t := range tasks {
			taskIds = append(taskIds, t.Id)
		}
		panicErr := recovery.HandlePanicWithError(recover(), nil, "problem adding edge")
		grip.Error(message.WrapError(panicErr, message.Fields{
			"function":       "topologicalSort",
			"from_task":      fromTask,
			"to_task":        toTask,
			"original_tasks": taskIds,
		}))
	}()
	depGraph := simple.NewDirectedGraph()
	taskNodeMap := make(map[string]graph.Node)
	nodeTaskMap := make(map[int64]Task)

	for _, task := range tasks {
		node := depGraph.NewNode()
		depGraph.AddNode(node)
		nodeTaskMap[node.ID()] = task
		taskNodeMap[task.Id] = node
	}

	for _, task := range tasks {
		for _, dep := range task.DependsOn {
			fromTask = dep.TaskId
			if toNode, ok := taskNodeMap[fromTask]; ok {
				toTask = task.Id
				edge := simple.Edge{
					F: simple.Node(toNode.ID()),
					T: simple.Node(taskNodeMap[toTask].ID()),
				}
				depGraph.SetEdge(edge)
			}
		}
	}

	sorted, err := topo.Sort(depGraph)
	if err != nil {
		return nil, errors.Wrap(err, "topologically sorting dependency graph")
	}
	sortedTasks := make([]Task, 0, len(tasks))
	for _, node := range sorted {
		sortedTasks = append(sortedTasks, nodeTaskMap[node.ID()])
	}

	return sortedTasks, nil
}

func DeactivateTasks(ctx context.Context, tasks []Task, updateDependencies bool, caller string) error {
	if len(tasks) == 0 {
		return nil
	}
	taskIDs := make([]string, 0, len(tasks))
	numEstimatedActivatedGeneratedTasks := 0
	for _, t := range tasks {
		// Deactivating a deactivated task is a noop.
		if !t.Activated {
			continue
		}
		if t.DisplayOnly {
			taskIDs = append(taskIDs, t.ExecutionTasks...)
		}
		taskIDs = append(taskIDs, t.Id)
		numEstimatedActivatedGeneratedTasks += utility.FromIntPtr(t.EstimatedNumActivatedGeneratedTasks)
	}

	depTasksToUpdate, depTaskIDsToUpdate, err := getDependencyTasksToUpdate(ctx, taskIDs, updateDependencies)
	if err != nil {
		return errors.Wrap(err, "retrieving dependency tasks to deactivate")
	}

	for _, depTask := range depTasksToUpdate {
		numEstimatedActivatedGeneratedTasks += utility.FromIntPtr(depTask.EstimatedNumActivatedGeneratedTasks)
	}
	// Tasks passed into this function will all be from the same version or build, so we can assume
	// all tasks also share the same requester field.
	numTasksModified := len(taskIDs) + len(depTaskIDsToUpdate) + numEstimatedActivatedGeneratedTasks
	if err = UpdateSchedulingLimit(ctx, caller, tasks[0].Requester, numTasksModified, false); err != nil {
		return err
	}

	_, err = UpdateAll(
		ctx,
		bson.M{
			IdKey: bson.M{"$in": taskIDs},
		},
		[]bson.M{
			{
				"$set": bson.M{
					ActivatedKey:     false,
					ActivatedByKey:   caller,
					ScheduledTimeKey: utility.ZeroTime,
				},
			},
			addDisplayStatusCache,
		},
	)
	if err != nil {
		return errors.Wrap(err, "deactivating tasks")
	}

	logs := []event.EventLogEntry{}
	for _, t := range tasks {
		logs = append(logs, event.GetTaskDeactivatedEvent(t.Id, t.Execution, caller))
	}
	grip.Error(message.WrapError(event.LogManyEvents(ctx, logs), message.Fields{
		"message":  "problem logging task deactivated events",
		"task_ids": taskIDs,
		"caller":   caller,
	}))

	if len(depTaskIDsToUpdate) > 0 {
		return deactivateDependencies(ctx, depTasksToUpdate, depTaskIDsToUpdate, caller)
	}
	return nil
}

func getDependencyTasksToUpdate(ctx context.Context, tasks []string, updateDependencies bool) ([]Task, []string, error) {
	if !updateDependencies {
		return nil, nil, nil
	}
	tasksDependingOnTheseTasks, err := getRecursiveDependenciesDown(ctx, tasks, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting recursive dependencies down")
	}

	tasksToUpdate := make([]Task, 0, len(tasksDependingOnTheseTasks))
	taskIDsToUpdate := make([]string, 0, len(tasksDependingOnTheseTasks))
	for _, t := range tasksDependingOnTheseTasks {
		if t.Activated {
			tasksToUpdate = append(tasksToUpdate, t)
			taskIDsToUpdate = append(taskIDsToUpdate, t.Id)
		}
	}
	return tasksToUpdate, taskIDsToUpdate, nil
}

func deactivateDependencies(ctx context.Context, tasksToUpdate []Task, taskIDsToUpdate []string, caller string) error {
	if len(tasksToUpdate) == 0 {
		return nil
	}
	_, err := UpdateAll(
		ctx,
		bson.M{
			IdKey: bson.M{"$in": taskIDsToUpdate},
		},
		[]bson.M{
			{
				"$set": bson.M{
					ActivatedKey:                false,
					DeactivatedForDependencyKey: true,
					ScheduledTimeKey:            utility.ZeroTime,
				},
			},
			addDisplayStatusCache,
		},
	)
	if err != nil {
		return errors.Wrap(err, "deactivating dependencies")
	}

	logs := []event.EventLogEntry{}
	for _, t := range tasksToUpdate {
		logs = append(logs, event.GetTaskDeactivatedEvent(t.Id, t.Execution, caller))
	}
	grip.Error(message.WrapError(event.LogManyEvents(ctx, logs), message.Fields{
		"message":  "problem logging task deactivated events",
		"task_ids": taskIDsToUpdate,
		"caller":   caller,
	}))

	return nil
}

// DeactivateDependencies gets all tasks that are blocked by the given tasks (this could be 1st level
// or recursive) and deactivates them. Then it sends out the event logs for the deactivation.
func DeactivateDependencies(ctx context.Context, tasks []string, caller string) error {
	tasksToUpdate, taskIDsToUpdate, err := getDependencyTasksToUpdate(ctx, tasks, true)
	if err != nil {
		return errors.Wrap(err, "retrieving dependency tasks to deactivate")
	}
	return errors.Wrap(deactivateDependencies(ctx, tasksToUpdate, taskIDsToUpdate, caller), "marking dependencies deactivated")
}

// MarkEnd handles the Task updates associated with ending a task. If the task's start time is zero
// at this time, it will set it to the finish time minus the timeout time.
func (t *Task) MarkEnd(ctx context.Context, finishTime time.Time, detail *apimodels.TaskEndDetail) error {
	// if there is no start time set, either set it to the create time
	// or set 2 hours previous to the finish time.
	if utility.IsZeroTime(t.StartTime) {
		timedOutStart := finishTime.Add(-2 * time.Hour)
		t.StartTime = timedOutStart
		if timedOutStart.Before(t.IngestTime) {
			t.StartTime = t.IngestTime
		}
	}

	t.TimeTaken = finishTime.Sub(t.StartTime)

	// Calculate task cost now that we have the actual runtime
	if err := t.UpdateTaskCost(ctx); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message":   "failed to calculate task cost",
			"task_id":   t.Id,
			"execution": t.Execution,
		}))
		// Don't fail the task finishing if cost calculation fails
	}

	grip.Debug(message.Fields{
		"message":   "marking task finished",
		"task_id":   t.Id,
		"execution": t.Execution,
		"project":   t.Project,
		"details":   t.Details,
	})
	if detail.IsEmpty() {
		grip.Debug(message.Fields{
			"message":   "detail status was empty, setting to failed",
			"task_id":   t.Id,
			"execution": t.Execution,
			"project":   t.Project,
			"details":   t.Details,
		})
		detail = &apimodels.TaskEndDetail{
			Status: evergreen.TaskFailed,
		}
	}

	// record that the task has finished, in memory and in the db
	t.Status = detail.Status
	t.FinishTime = finishTime
	t.Details = *detail
	t.DisplayStatusCache = t.DetermineDisplayStatus()
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				FinishTimeKey:         finishTime,
				StatusKey:             detail.Status,
				TimeTakenKey:          t.TimeTaken,
				TaskCostKey:           t.TaskCost,
				S3UsageKey:            t.S3Usage,
				DetailsKey:            detail,
				StartTimeKey:          t.StartTime,
				DisplayStatusCacheKey: t.DisplayStatusCache,
				TaskOutputInfoKey:     t.TaskOutputInfo,
			},
		})
}

// GetDisplayStatus finds and sets DisplayStatus to the task. It should reflect
// the statuses assigned during the addDisplayStatus aggregation step.
func (t *Task) GetDisplayStatus() string {
	if t.DisplayStatus != "" {
		return t.DisplayStatus
	}
	t.DisplayStatus = t.DetermineDisplayStatus()
	return t.DisplayStatus
}

// DetermineDisplayStatus publicly exports findDisplayStatus
func (t *Task) DetermineDisplayStatus() string {
	return t.determineDisplayStatus()
}

// determineDisplayStatus calculates the display status for a task based on its current state.
func (t *Task) determineDisplayStatus() string {
	if t.HasAnnotations {
		return evergreen.TaskKnownIssue
	}
	if t.Aborted {
		return evergreen.TaskAborted
	}
	if t.Status == evergreen.TaskSucceeded {
		return evergreen.TaskSucceeded
	}
	if t.Details.Type == evergreen.CommandTypeSetup {
		return evergreen.TaskSetupFailed
	}
	if t.Details.Type == evergreen.CommandTypeSystem {
		if t.Details.TimedOut && t.Details.Description == evergreen.TaskDescriptionHeartbeat {
			return evergreen.TaskSystemUnresponse
		}
		if t.Details.TimedOut {
			return evergreen.TaskSystemTimedOut
		}
		return evergreen.TaskSystemFailed
	}
	if t.Details.TimedOut {
		return evergreen.TaskTimedOut
	}
	if t.Status == evergreen.TaskUndispatched {
		if !t.Activated {
			return evergreen.TaskUnscheduled
		}
		if t.Blocked() {
			return evergreen.TaskStatusBlocked
		}
		return evergreen.TaskWillRun
	}
	return t.Status
}

// displayTaskPriority answers the question "if there is a display task whose executions are
// in these statuses, which overall status would a user expect to see?"
// for example, if there are both successful and failed tasks, one would expect to see "failed"
func (t *Task) displayTaskPriority() int {
	switch t.GetDisplayStatus() {
	case evergreen.TaskStarted:
		return 10
	case evergreen.TaskFailed:
		return 20
	case evergreen.TaskTestTimedOut:
		return 30
	case evergreen.TaskTimedOut:
		return 40
	case evergreen.TaskSystemFailed:
		return 50
	case evergreen.TaskSystemTimedOut:
		return 60
	case evergreen.TaskSystemUnresponse:
		return 70
	case evergreen.TaskSetupFailed:
		return 80
	case evergreen.TaskUndispatched:
		return 90
	case evergreen.TaskInactive:
		return 100
	case evergreen.TaskSucceeded:
		return 110
	}
	// Note that this includes evergreen.TaskDispatched.
	return 1000
}

// Reset sets the task state to a state in which it is scheduled to re-run.
func (t *Task) Reset(ctx context.Context, caller string) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey:       t.Id,
			StatusKey:   bson.M{"$in": evergreen.TaskCompletedStatuses},
			CanResetKey: true,
		},
		resetTaskUpdate(t, caller, nil),
	)
}

// ResetTasks performs the same DB updates as (*Task).Reset, but resets many
// tasks instead of a single one.
func ResetTasks(ctx context.Context, tasks []Task, caller string) error {
	if len(tasks) == 0 {
		return nil
	}

	// Separate tasks by whether they need predictions
	var tasksNeedingPredictions []Task
	var tasksWithPredictions []Task
	for _, t := range tasks {
		if t.PredictedTaskCost.IsZero() {
			tasksNeedingPredictions = append(tasksNeedingPredictions, t)
		} else {
			tasksWithPredictions = append(tasksWithPredictions, t)
		}
	}

	// Compute cost predictions only for tasks that need them
	predictions, err := computeCostPredictionsInParallel(ctx, tasksNeedingPredictions)
	if err != nil {
		return errors.Wrap(err, "computing cost predictions for reset tasks")
	}

	env := evergreen.GetEnvironment()
	coll := env.DB().Collection(Collection)
	var writes []mongo.WriteModel

	// Add tasks with new predictions
	for _, t := range tasksNeedingPredictions {
		prediction := predictions[t.Id]
		update := resetTaskUpdate(nil, caller, &prediction)

		writes = append(writes, mongo.NewUpdateOneModel().
			SetFilter(bson.M{
				IdKey:       t.Id,
				StatusKey:   bson.M{"$in": evergreen.TaskCompletedStatuses},
				CanResetKey: true,
			}).
			SetUpdate(update))
	}

	// Add tasks with existing predictions
	for _, t := range tasksWithPredictions {
		update := resetTaskUpdate(nil, caller, nil)

		writes = append(writes, mongo.NewUpdateOneModel().
			SetFilter(bson.M{
				IdKey:       t.Id,
				StatusKey:   bson.M{"$in": evergreen.TaskCompletedStatuses},
				CanResetKey: true,
			}).
			SetUpdate(update))
	}

	if len(writes) > 0 {
		_, err := coll.BulkWrite(ctx, writes)
		if err != nil {
			return errors.Wrap(err, "bulk resetting tasks")
		}
	}

	return nil
}

func resetTaskUpdate(t *Task, caller string, prediction *CostPredictionResult) []bson.M {
	newSecret := utility.RandomString()
	now := time.Now()
	if t != nil {
		t.Activated = true
		t.ActivatedTime = now
		t.ActivatedBy = caller
		t.Secret = newSecret
		t.HostId = ""
		t.Status = evergreen.TaskUndispatched
		t.DispatchTime = utility.ZeroTime
		t.StartTime = utility.ZeroTime
		t.ScheduledTime = utility.ZeroTime
		t.FinishTime = utility.ZeroTime
		t.DependenciesMetTime = utility.ZeroTime
		t.TimeTaken = 0
		t.LastHeartbeat = utility.ZeroTime
		t.Details = apimodels.TaskEndDetail{}
		t.TaskOutputInfo = nil
		t.ResultsService = ""
		t.ResultsFailed = false
		t.HasTestResults = false
		t.ResetWhenFinished = false
		t.ResetFailedWhenFinished = false
		t.AgentVersion = ""
		t.HostCreateDetails = []HostCreateDetail{}
		t.OverrideDependencies = false
		t.NumNextTaskDispatches = 0
		t.CanReset = false
		t.IsAutomaticRestart = false
		t.HasAnnotations = false
		if prediction != nil {
			t.SetPredictedCost(prediction.PredictedCost)
		}
		t.DisplayStatusCache = t.DetermineDisplayStatus()
	}

	setFields := bson.M{
		ActivatedKey:             true,
		ActivatedTimeKey:         now,
		ActivatedByKey:           caller,
		SecretKey:                newSecret,
		StatusKey:                evergreen.TaskUndispatched,
		DispatchTimeKey:          utility.ZeroTime,
		StartTimeKey:             utility.ZeroTime,
		ScheduledTimeKey:         utility.ZeroTime,
		FinishTimeKey:            utility.ZeroTime,
		DependenciesMetTimeKey:   utility.ZeroTime,
		TimeTakenKey:             0,
		LastHeartbeatKey:         utility.ZeroTime,
		NumNextTaskDispatchesKey: 0,
	}

	if prediction != nil {
		addPredictedCostToUpdate(setFields, prediction.PredictedCost)
	}

	update := []bson.M{
		{"$set": setFields},
		{
			"$unset": []string{
				DetailsKey,
				TaskOutputInfoKey,
				ResultsFailedKey,
				HasTestResultsKey,
				ResetWhenFinishedKey,
				IsAutomaticRestartKey,
				ResetFailedWhenFinishedKey,
				AgentVersionKey,
				HostIdKey,
				HostCreateDetailsKey,
				OverrideDependenciesKey,
				CanResetKey,
				HasAnnotationsKey,
			},
		},
		addDisplayStatusCache,
	}
	return update
}

// UpdateHeartbeat updates the heartbeat to be the current time
func (t *Task) UpdateHeartbeat(ctx context.Context) error {
	t.LastHeartbeat = time.Now()
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				LastHeartbeatKey: t.LastHeartbeat,
			},
		},
	)
}

// SetNumGeneratedTasks sets the number of generated tasks to the given value.
func (t *Task) SetNumGeneratedTasks(ctx context.Context, numGeneratedTasks int) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				NumGeneratedTasksKey: numGeneratedTasks,
			},
		},
	)
}

// SetNumActivatedGeneratedTasks sets the number of activated generated tasks to the given value.
func (t *Task) SetNumActivatedGeneratedTasks(ctx context.Context, numActivatedGeneratedTasks int) error {
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				NumActivatedGeneratedTasksKey: numActivatedGeneratedTasks,
			},
		},
	)
}

// GetRecursiveDependenciesUp returns all tasks recursively depended upon
// that are not in the original task slice (this includes earlier tasks in task groups, if applicable).
// depCache should originally be nil. We assume there are no dependency cycles.
func GetRecursiveDependenciesUp(ctx context.Context, tasks []Task, depCache map[string]Task) ([]Task, error) {
	if depCache == nil {
		depCache = make(map[string]Task)
	}
	for _, t := range tasks {
		depCache[t.Id] = t
	}

	tasksToFind := []string{}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if _, ok := depCache[dep.TaskId]; !ok {
				tasksToFind = append(tasksToFind, dep.TaskId)
			}
		}
		if t.IsPartOfSingleHostTaskGroup() {
			tasksInGroup, err := FindTaskGroupFromBuild(ctx, t.BuildId, t.TaskGroup)
			if err != nil {
				return nil, errors.Wrapf(err, "finding task group '%s'", t.TaskGroup)
			}
			for _, taskInGroup := range tasksInGroup {
				if taskInGroup.TaskGroupOrder < t.TaskGroupOrder {
					if _, ok := depCache[taskInGroup.Id]; !ok {
						tasksToFind = append(tasksToFind, taskInGroup.Id)
					}
				}
			}
		}
	}

	// leaf node
	if len(tasksToFind) == 0 {
		return nil, nil
	}

	deps, err := FindWithFields(ctx, ByIds(tasksToFind), IdKey, DependsOnKey, ExecutionKey, BuildIdKey, StatusKey, TaskGroupKey, ActivatedKey, DisplayNameKey, PriorityKey)
	if err != nil {
		return nil, errors.Wrap(err, "getting dependencies")
	}

	recursiveDeps, err := GetRecursiveDependenciesUp(ctx, deps, depCache)
	if err != nil {
		return nil, errors.Wrap(err, "getting recursive dependencies")
	}

	return append(deps, recursiveDeps...), nil
}

// getRecursiveDependenciesDown returns a slice containing all tasks recursively depending on tasks.
// taskMap should originally be nil.
// We assume there are no dependency cycles.
func getRecursiveDependenciesDown(ctx context.Context, tasks []string, taskMap map[string]bool) ([]Task, error) {
	if taskMap == nil {
		taskMap = make(map[string]bool)
	}
	for _, t := range tasks {
		taskMap[t] = true
	}

	// find the tasks that depend on these tasks
	query := db.Query(bson.M{
		bsonutil.GetDottedKeyName(DependsOnKey, DependencyTaskIdKey): bson.M{"$in": tasks},
	}).WithFields(IdKey, ActivatedKey, DeactivatedForDependencyKey, ExecutionKey, DependsOnKey, BuildIdKey)
	dependOnUsTasks, err := FindAll(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "can't get dependencies")
	}

	// if the task hasn't yet been visited we need to recurse on it
	newDeps := []Task{}
	for _, t := range dependOnUsTasks {
		if !taskMap[t.Id] {
			newDeps = append(newDeps, t)
		}
	}

	// everything is aleady in the map or nothing depends on tasks
	if len(newDeps) == 0 {
		return nil, nil
	}

	newDepIDs := make([]string, 0, len(newDeps))
	for _, t := range newDeps {
		newDepIDs = append(newDepIDs, t.Id)
	}
	recurseTasks, err := getRecursiveDependenciesDown(ctx, newDepIDs, taskMap)
	if err != nil {
		return nil, errors.Wrap(err, "getting recursive dependencies")
	}

	return append(newDeps, recurseTasks...), nil
}

// MarkStart updates the task's start time and sets the status to started
func (t *Task) MarkStart(ctx context.Context, startTime time.Time) error {
	// record the start time in the in-memory task
	t.StartTime = startTime
	t.Status = evergreen.TaskStarted
	t.DisplayStatusCache = t.DetermineDisplayStatus()

	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:             evergreen.TaskStarted,
				LastHeartbeatKey:      startTime,
				StartTimeKey:          startTime,
				DisplayStatusCacheKey: t.DisplayStatusCache,
			},
		},
	)
}

// MarkUnscheduled marks the task as undispatched and updates it in the database
func (t *Task) MarkUnscheduled(ctx context.Context) error {
	t.Status = evergreen.TaskUndispatched
	t.DisplayStatusCache = t.DetermineDisplayStatus()
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:             evergreen.TaskUndispatched,
				DisplayStatusCacheKey: t.DisplayStatusCache,
			},
		},
	)

}

// MarkAllForUnattainableDependencies updates many tasks (taskIDs) to mark a
// subset of all their dependencies (dependencyIDs) as attainable or not. If
// marking the dependencies unattainable, it creates an event log for each newly
// blocked task. This returns all the tasks after the update.
func MarkAllForUnattainableDependencies(ctx context.Context, tasks []Task, dependencyIDs []string, unattainable bool) ([]Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}

	dependencyIDSet := map[string]struct{}{}
	for _, depID := range dependencyIDs {
		dependencyIDSet[depID] = struct{}{}
	}

	toUpdate := getTaskDependenciesToUpdate(tasks, dependencyIDSet, unattainable)

	if err := updateTaskDependenciesChunked(ctx, toUpdate, unattainable); err != nil {
		return nil, errors.Wrap(err, "updating task dependencies in chunks")
	}

	event.LogManyTasksBlocked(ctx, toUpdate.newlyBlockedTaskData)

	updatedTasks, err := findAllTasksChunked(ctx, toUpdate.taskIDs)
	if err != nil {
		return nil, errors.Wrap(err, "finding updated tasks")
	}
	grip.ErrorWhen(len(updatedTasks) != len(tasks), message.Fields{
		"message":            "successfully updated dependencies for tasks but the subsequent query for updated tasks returned a different number of tasks than expected (which may cause bugs blocking other tasks)",
		"expected_num_tasks": len(tasks),
		"actual_num_tasks":   len(updatedTasks),
	})

	return updatedTasks, nil
}

type taskDependencyUpdate struct {
	// taskIDs is the list of task IDs whose dependencies should be updated.
	taskIDs []string
	// taskDependenciesToUpdate is a mapping of task IDs that need updating to
	// the dependency IDs that should be updated for that task.
	taskDependenciesToUpdate map[string][]string
	// newlyBlockedTaskData contains info on tasks that were not blocked but are
	// about to be blocked.
	newlyBlockedTaskData []event.TaskBlockedData
}

func getTaskDependenciesToUpdate(tasks []Task, dependencyIDSet map[string]struct{}, unattainable bool) taskDependencyUpdate {
	taskIDs := make([]string, 0, len(tasks))
	taskDependenciesToUpdate := make(map[string][]string)
	newlyBlockedTaskData := make([]event.TaskBlockedData, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)

		taskCanBeNewlyBlocked := unattainable && !t.Blocked() && !t.OverrideDependencies
		var taskBlockedOn string
		for _, dependsOn := range t.DependsOn {
			depID := dependsOn.TaskId
			if _, ok := dependencyIDSet[depID]; !ok {
				continue
			}

			taskDependenciesToUpdate[t.Id] = append(taskDependenciesToUpdate[t.Id], depID)

			if taskCanBeNewlyBlocked && taskBlockedOn == "" {
				// If a task is going to be newly blocked by multiple
				// dependencies at once, just block it arbitrarily on
				// the first one.
				taskBlockedOn = depID
			}
		}

		if taskCanBeNewlyBlocked && taskBlockedOn != "" {
			data := event.TaskBlockedData{
				ID:        t.Id,
				Execution: t.Execution,
				BlockedOn: taskBlockedOn,
			}
			newlyBlockedTaskData = append(newlyBlockedTaskData, data)
		}
	}

	return taskDependencyUpdate{
		taskIDs:                  taskIDs,
		taskDependenciesToUpdate: taskDependenciesToUpdate,
		newlyBlockedTaskData:     newlyBlockedTaskData,
	}
}

// updateTaskDependenciesChunked updates all task dependencies in toUpdate in
// chunks of updates. The updates must be updated in smaller chunks to avoid the
// 16 MB update query size limit - if too many task IDs and dependency IDs are
// passed into a single query, the query size will be too large and the DB will
// reject it.
func updateTaskDependenciesChunked(ctx context.Context, toUpdate taskDependencyUpdate, unattainable bool) error {
	const taskDependencyUpdateChunkSize = 2000
	var chunkTaskIDs []string
	chunkDependencyIDSet := make(map[string]struct{})

	numTasksToUpdate := len(toUpdate.taskDependenciesToUpdate)
	numTasksSeen := 0
	for taskID, dependencyIDs := range toUpdate.taskDependenciesToUpdate {
		chunkTaskIDs = append(chunkTaskIDs, taskID)
		for _, depID := range dependencyIDs {
			chunkDependencyIDSet[depID] = struct{}{}
		}

		numTasksSeen++

		if len(chunkDependencyIDSet) >= taskDependencyUpdateChunkSize || numTasksSeen == numTasksToUpdate {
			// Update the tasks now - either there's no remaining task
			// dependencies to update, or the max number of task dependencies
			// that can be checked per query has been reached.
			if err := updateTaskDependenciesForChunk(ctx, chunkTaskIDs, chunkDependencyIDSet, unattainable); err != nil {
				return err
			}

			chunkTaskIDs = nil
			chunkDependencyIDSet = make(map[string]struct{})
		}
	}

	return nil
}

func updateTaskDependenciesForChunk(ctx context.Context, taskIDs []string, dependencyIDSet map[string]struct{}, unattainable bool) error {
	// Update this chunk of task dependencies, which has hit/exceeded
	// the max number of dependencies that can be updated at once, then
	// continue on with a new chunk of task dependencies.

	dependencyIDs := make([]string, 0, len(dependencyIDSet))
	for depID := range dependencyIDSet {
		dependencyIDs = append(dependencyIDs, depID)
	}

	if err := updateAllTasksForAllMatchingDependencies(ctx, taskIDs, dependencyIDs, unattainable); err != nil {
		return errors.Wrapf(err, "updating %d dependencies across %d tasks for dependency attainability", len(dependencyIDs), len(taskIDs))
	}

	return nil
}

// findAllTasksChunked finds all tasks by ID. This finds tasks in smaller chunks
// to avoid the 16  MB query size limit - if the number of tasks is large, a
// single query could be too large and the DB will reject it.
func findAllTasksChunked(ctx context.Context, taskIDs []string) ([]Task, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}

	const maxTasksPerQuery = 2000
	var chunkedTasks []string
	allTasks := make([]Task, 0, len(taskIDs))

	for i, taskID := range taskIDs {
		chunkedTasks = append(chunkedTasks, taskID)

		if i == len(taskIDs)-1 || len(chunkedTasks) > maxTasksPerQuery {
			// Query the tasks now - either there's no remaining task IDs, or
			// the max task IDs that can be checked per query has been reached.
			tasks, err := FindAll(ctx, db.Query(ByIds(chunkedTasks)))
			if err != nil {
				return nil, errors.Wrap(err, "finding tasks")
			}
			allTasks = append(allTasks, tasks...)
			chunkedTasks = nil
		}
	}

	return allTasks, nil
}

// AbortBuildTasks sets the abort flag on all tasks associated with the build which are in an abortable
func AbortBuildTasks(ctx context.Context, buildId string, reason AbortInfo) error {
	q := bson.M{
		BuildIdKey: buildId,
		StatusKey:  bson.M{"$in": evergreen.TaskInProgressStatuses},
	}
	if reason.TaskID != "" {
		q[IdKey] = bson.M{"$ne": reason.TaskID}
	}
	return errors.Wrapf(abortTasksByQuery(ctx, q, reason), "aborting tasks for build '%s'", buildId)
}

// AbortVersionTasks sets the abort flag on all tasks associated with the version which are in an
// abortable state
func AbortVersionTasks(ctx context.Context, versionId string, reason AbortInfo) error {
	q := ByVersionWithChildTasks(versionId)
	q[StatusKey] = bson.M{"$in": evergreen.TaskInProgressStatuses}
	if reason.TaskID != "" {
		q[IdKey] = bson.M{"$ne": reason.TaskID}
		// if the aborting task is part of a display task, we also don't want to mark it as aborted
		q[ExecutionTasksKey] = bson.M{"$ne": reason.TaskID}
	}
	return errors.Wrapf(abortTasksByQuery(ctx, q, reason), "aborting tasks for version '%s'", versionId)
}

func abortTasksByQuery(ctx context.Context, q bson.M, reason AbortInfo) error {
	ids, err := findAllTaskIDs(ctx, db.Query(q))
	if err != nil {
		return errors.Wrap(err, "finding updated tasks")
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = UpdateAll(
		ctx,
		ByIds(ids),
		[]bson.M{
			{"$set": taskAbortUpdate(reason)},
			addDisplayStatusCache,
		},
	)
	if err != nil {
		return errors.Wrap(err, "setting aborted statuses")
	}
	event.LogManyTaskAbortRequests(ctx, ids, reason.User)
	return nil
}

// String represents the stringified version of a task
func (t *Task) String() (taskStruct string) {
	taskStruct += fmt.Sprintf("Id: %v\n", t.Id)
	taskStruct += fmt.Sprintf("Status: %v\n", t.Status)
	taskStruct += fmt.Sprintf("Display Status: %v\n", t.DisplayStatusCache)
	taskStruct += fmt.Sprintf("Host: %v\n", t.HostId)
	taskStruct += fmt.Sprintf("ScheduledTime: %v\n", t.ScheduledTime)
	taskStruct += fmt.Sprintf("DispatchTime: %v\n", t.DispatchTime)
	taskStruct += fmt.Sprintf("StartTime: %v\n", t.StartTime)
	taskStruct += fmt.Sprintf("FinishTime: %v\n", t.FinishTime)
	taskStruct += fmt.Sprintf("TimeTaken: %v\n", t.TimeTaken)
	taskStruct += fmt.Sprintf("Activated: %v\n", t.Activated)
	taskStruct += fmt.Sprintf("Requester: %v\n", t.Requester)
	taskStruct += fmt.Sprintf("PredictedDuration: %v\n", t.DurationPrediction)

	return
}

// Insert writes the task to the db.
func (t *Task) Insert(ctx context.Context) error {
	return db.Insert(ctx, Collection, t)
}

// Archive modifies the current execution of the task so that it is no longer
// considered the latest execution. This task execution is inserted
// into the old_tasks collection. If this is a display task, its execution tasks
// are also archived.
func (t *Task) Archive(ctx context.Context) error {
	if !utility.StringSliceContains(evergreen.TaskCompletedStatuses, t.Status) {
		return nil
	}
	if t.CanReset {
		// For idempotency reasons, skip tasks that are currently waiting to
		// reset. It prevents a race where the same task data can be
		// archived for two consecutive task executions.
		return nil
	}

	if t.DisplayOnly && len(t.ExecutionTasks) > 0 {
		return errors.Wrapf(ArchiveMany(ctx, []Task{*t}), "archiving display task '%s'", t.Id)
	}

	archiveTask := t.makeArchivedTask()
	err := db.Insert(ctx, OldCollection, archiveTask)
	if err != nil && !db.IsDuplicateKey(err) {
		return errors.Wrap(err, "inserting archived task into old tasks")
	}
	t.Aborted = false
	err = UpdateOne(
		ctx,
		bson.M{
			IdKey:     t.Id,
			StatusKey: bson.M{"$in": evergreen.TaskCompletedStatuses},
			"$or": []bson.M{
				{
					CanResetKey: bson.M{"$exists": false},
				},
				{
					CanResetKey: false,
				},
			},
		},
		[]bson.M{
			updateDisplayTasksAndTasksSet,
			updateDisplayTasksAndTasksUnset,
			addDisplayStatusCache,
		},
	)
	// Return nil if the task has already been archived
	if adb.ResultsNotFound(err) {
		return nil
	}
	return errors.Wrap(err, "updating task")
}

// ArchiveMany accepts tasks and display tasks (no execution tasks). The function
// expects that each one is going to be archived and progressed to the next execution.
// For execution tasks in display tasks, it will properly account for archiving
// only tasks that should be if failed.
func ArchiveMany(ctx context.Context, tasks []Task) error {
	allTaskIds := []string{}          // Contains all tasks and display tasks IDs
	execTaskIds := []string{}         // Contains all exec tasks IDs
	toUpdateExecTaskIds := []string{} // Contains all exec tasks IDs that should update and have new execution
	archivedTasks := []any{}          // Contains all archived tasks (task, display, and execution). Created by Task.makeArchivedTask()

	for _, t := range tasks {
		if !utility.StringSliceContains(evergreen.TaskCompletedStatuses, t.Status) {
			continue
		}
		if t.CanReset {
			// For idempotency reasons, skip tasks that are currently waiting to
			// reset. It prevents a race where the same task data can be
			// archived for two consecutive task executions.
			continue
		}

		allTaskIds = append(allTaskIds, t.Id)
		archivedTasks = append(archivedTasks, t.makeArchivedTask())
		if t.DisplayOnly && len(t.ExecutionTasks) > 0 {
			var execTasks []Task
			var err error

			if t.IsRestartFailedOnly() {
				execTasks, err = Find(ctx, FailedTasksByIds(t.ExecutionTasks))
			} else {
				execTasks, err = FindAll(ctx, db.Query(ByIdsAndStatus(t.ExecutionTasks, evergreen.TaskCompletedStatuses)))
			}

			if err != nil {
				return errors.Wrapf(err, "finding execution tasks for display task '%s'", t.Id)
			}
			execTaskIds = append(execTaskIds, t.ExecutionTasks...)
			for _, et := range execTasks {
				if !utility.StringSliceContains(evergreen.TaskCompletedStatuses, et.Status) {
					grip.Debug(message.Fields{
						"message":   "execution task is in incomplete state, skipping archiving",
						"task_id":   et.Id,
						"execution": et.Execution,
						"func":      "ArchiveMany",
					})
					continue
				}
				archivedTasks = append(archivedTasks, et.makeArchivedTask())
				toUpdateExecTaskIds = append(toUpdateExecTaskIds, et.Id)
			}
		}
	}

	return archiveAll(ctx, allTaskIds, execTaskIds, toUpdateExecTaskIds, archivedTasks)
}

// archiveAll takes in:
// - taskIds                : All tasks and display tasks IDs
// - execTaskIds            : All execution task IDs
// - toRestartExecTaskIds   : All execution task IDs for execution tasks that will be archived/restarted
// - archivedTasks          : All archived tasks created by Task.makeArchivedTask()
func archiveAll(ctx context.Context, taskIds, execTaskIds, toRestartExecTaskIds []string, archivedTasks []any) error {
	mongoClient := evergreen.GetEnvironment().Client()
	session, err := mongoClient.StartSession()
	if err != nil {
		return errors.Wrap(err, "starting DB session")
	}
	defer session.EndSession(ctx)

	txFunc := func(ctx mongo.SessionContext) (any, error) {
		var err error
		if len(archivedTasks) > 0 {
			oldTaskColl := evergreen.GetEnvironment().DB().Collection(OldCollection)
			_, err = oldTaskColl.InsertMany(ctx, archivedTasks)
			if err != nil {
				return nil, errors.Wrap(err, "archiving tasks")
			}
		}
		if len(taskIds) > 0 {
			_, err = evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx,
				bson.M{
					IdKey:     bson.M{"$in": taskIds},
					StatusKey: bson.M{"$in": evergreen.TaskCompletedStatuses},
					"$or": []bson.M{
						{
							CanResetKey: bson.M{"$exists": false},
						},
						{
							CanResetKey: false,
						},
					},
				},
				[]bson.M{
					updateDisplayTasksAndTasksSet,
					updateDisplayTasksAndTasksUnset,
					addDisplayStatusCache,
				},
			)
			if err != nil {
				return nil, errors.Wrap(err, "archiving tasks")
			}
		}
		if len(execTaskIds) > 0 {
			_, err = evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx,
				bson.M{IdKey: bson.M{"$in": execTaskIds}}, // Query all execution tasks
				bson.A{ // Pipeline
					bson.M{"$set": bson.M{ // Sets LatestParentExecution (LPE) = LPE + 1
						LatestParentExecutionKey: bson.M{"$add": bson.A{
							"$" + LatestParentExecutionKey, 1,
						}},
					}},
					addDisplayStatusCache,
				})

			if err != nil {
				return nil, errors.Wrap(err, "updating latest parent executions")
			}

			// Call to update all tasks that are actually restarting
			_, err = evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx,
				bson.M{IdKey: bson.M{"$in": toRestartExecTaskIds}}, // Query all archiving/restarting execution tasks
				bson.A{ // Pipeline
					bson.M{"$set": bson.M{ // Execution = LPE
						ExecutionKey: "$" + LatestParentExecutionKey,
						CanResetKey:  true,
					}},
					bson.M{"$unset": bson.A{
						AbortedKey,
						AbortInfoKey,
						OverrideDependenciesKey,
					}},
					addDisplayStatusCache,
				})

			return nil, errors.Wrap(err, "updating restarting exec tasks")
		}
		return nil, errors.Wrap(err, "updating tasks")
	}

	_, err = session.WithTransaction(ctx, txFunc)

	// Return nil if the tasks have already been archived. Because we use a transaction here,
	// we can trust that either all tasks have already been archived, or none of them.
	if err == nil || db.IsDuplicateKey(err) || adb.ResultsNotFound(err) {
		return nil
	}
	return errors.Wrap(err, "archiving execution tasks and updating execution tasks")
}

func (t *Task) makeArchivedTask() *Task {
	archiveTask := *t
	archiveTask.Id = MakeOldID(t.Id, t.Execution)
	archiveTask.OldTaskId = t.Id
	archiveTask.Archived = true

	return &archiveTask
}

// Aggregation

// PopulateTestResults populates the task's LocalTestResults field with any
// test results the task may have. If the results are already populated, this
// function no-ops.
func (t *Task) PopulateTestResults(ctx context.Context) error {
	if len(t.LocalTestResults) > 0 {
		return nil
	}

	taskTestResults, err := t.GetTestResults(ctx, evergreen.GetEnvironment(), nil)
	if err != nil {
		return errors.Wrap(err, "populating test results")
	}
	t.LocalTestResults = taskTestResults.Results

	return nil
}

// GetTestResults returns the merged test results filtered, sorted,
// and paginated as specified by the optional filter options for the given
// tasks.
func (t *Task) GetTestResults(ctx context.Context, env evergreen.Environment, filterOpts *FilterOptions) (testresult.TaskTestResults, error) {
	taskOpts, err := t.GetTestResultsTasks(ctx)
	if err != nil {
		return testresult.TaskTestResults{}, errors.Wrap(err, "creating test results task options")
	}
	if len(taskOpts) == 0 {
		return testresult.TaskTestResults{}, nil
	}
	return getMergedTaskTestResults(ctx, env, taskOpts, filterOpts)
}

// GetTestResultsStats returns basic statistics of the task's test results.
func (t *Task) GetTestResultsStats(ctx context.Context, env evergreen.Environment) (testresult.TaskTestResultsStats, error) {
	taskOpts, err := t.GetTestResultsTasks(ctx)
	if err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "creating test results task options")
	}
	if len(taskOpts) == 0 {
		return testresult.TaskTestResultsStats{}, nil
	}
	return getTaskTestResultsStats(ctx, env, taskOpts)
}

// GetTestResultsTasks returns the options required for fetching test
// results for the task.
//
// Calling this function explicitly is typically not necessary. In cases where
// additional tasks are required for fetching test results, such as when
// sorting results by some base status, using this function to populate those
// task options is useful.
func (t *Task) GetTestResultsTasks(ctx context.Context) ([]Task, error) {
	var tasks []Task
	if t.DisplayOnly && len(t.ExecutionTasks) > 0 {
		var (
			execTasksWithResults []Task
			err                  error
		)
		hasResults := []bson.M{{ResultsServiceKey: bson.M{"$exists": true}}, {HasTestResultsKey: true}}
		if t.Archived {
			execTasksWithResults, err = FindByExecutionTasksAndMaxExecution(ctx, t.ExecutionTasks, t.Execution, bson.E{Key: "$or", Value: hasResults})
		} else {
			query := ByIds(t.ExecutionTasks)
			query["$or"] = hasResults
			execTasksWithResults, err = FindWithFields(ctx, query, ExecutionKey, ResultsServiceKey, HasTestResultsKey, TaskOutputInfoKey)
		}
		if err != nil {
			return nil, errors.Wrap(err, "getting execution tasks for display task")
		}

		for _, execTask := range execTasksWithResults {
			et := execTask
			if execTask.Archived {
				et.Id = execTask.OldTaskId
			}
			tasks = append(tasks, et)
		}
	} else if t.HasResults(ctx) {
		task := t
		if t.Archived {
			task.Id = t.OldTaskId
		}
		tasks = append(tasks, *task)
	}

	return tasks, nil
}

// SetResetWhenFinished requests that a display task or single-host task group
// reset itself when finished. Will mark itself as system failed.
func (t *Task) SetResetWhenFinished(ctx context.Context, caller string) error {
	if t.ResetWhenFinished {
		return nil
	}
	if err := updateSchedulingLimitForResetWhenFinished(ctx, t, caller); err != nil {
		return errors.Wrapf(err, "updating user '%s' patch task scheduling limit", caller)
	}
	t.ResetFailedWhenFinished = false
	t.ResetWhenFinished = true
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$unset": bson.M{
				ResetFailedWhenFinishedKey: 1,
			},
			"$set": bson.M{
				ResetWhenFinishedKey: true,
			},
		},
	)
}

// SetResetWhenFinishedWithInc requests that a task (that was marked to
// automatically reset when finished via the agent status server) reset itself
// when finished. It will also increment the number of automatic resets the task
// has performed.
func (t *Task) SetResetWhenFinishedWithInc(ctx context.Context) error {
	if t.ResetWhenFinished {
		return nil
	}
	if t.Aborted {
		return errors.New("cannot set reset when finished for aborted task")
	}
	err := UpdateOne(
		ctx,
		bson.M{
			IdKey:                 t.Id,
			AbortedKey:            bson.M{"$ne": true},
			IsAutomaticRestartKey: bson.M{"$ne": true},
		},
		bson.M{
			"$unset": bson.M{
				ResetFailedWhenFinishedKey: 1,
			},
			"$set": bson.M{
				ResetWhenFinishedKey:  true,
				IsAutomaticRestartKey: true,
			},
			"$inc": bson.M{
				NumAutomaticRestartsKey: 1,
			},
		},
	)
	return err
}

// SetResetFailedWhenFinished requests that a display task
// only restarts failed tasks.
func (t *Task) SetResetFailedWhenFinished(ctx context.Context, caller string) error {
	if t.ResetFailedWhenFinished {
		return nil
	}
	if err := updateSchedulingLimitForResetWhenFinished(ctx, t, caller); err != nil {
		return errors.Wrapf(err, "updating user '%s' patch task scheduling limit", caller)
	}
	t.ResetWhenFinished = false
	t.ResetFailedWhenFinished = true
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$unset": bson.M{
				ResetWhenFinishedKey: 1,
			},
			"$set": bson.M{
				ResetFailedWhenFinishedKey: true,
			},
		},
	)
}

func updateSchedulingLimitForResetWhenFinished(ctx context.Context, t *Task, caller string) error {
	if !(t.Requester == evergreen.PatchVersionRequester || t.Requester == evergreen.GithubPRRequester) || evergreen.IsSystemActivator(caller) {
		return nil
	}
	var tasks []Task
	var err error
	if t.DisplayOnly {
		tasks, err = Find(ctx, ByIds(t.ExecutionTasks))
		if err != nil {
			return errors.Wrapf(err, "finding execution tasks for '%s'", t.Id)
		}
	} else if t.IsPartOfSingleHostTaskGroup() {
		tasks, err = FindTaskGroupFromBuild(ctx, t.BuildId, t.TaskGroup)
		if err != nil {
			return errors.Wrapf(err, "finding task group '%s'", t.TaskGroup)
		}
	}
	if len(tasks) == 0 {
		return nil
	}
	return errors.Wrap(CheckUsersPatchTaskLimit(ctx, t.Requester, caller, true, tasks...), "updating patch task limit for user")
}

// CheckUsersPatchTaskLimit takes in an input list of tasks that is set to get activated, and checks if they're
// non commit-queue patch tasks, and that the request has been submitted by a user. If so, the maximum hourly patch tasks counter
// will be incremented accordingly. The includeDisplayAndTaskGroups parameter indicates that execution tasks and single host task
// group tasks are to be counted as part of the limit update, otherwise they will be ignored.
func CheckUsersPatchTaskLimit(ctx context.Context, requester, username string, includeDisplayAndTaskGroups bool, tasks ...Task) error {
	// we only care about patch tasks that are to be activated by an actual user
	if !(requester == evergreen.PatchVersionRequester || requester == evergreen.GithubPRRequester) || evergreen.IsSystemActivator(username) {
		return nil
	}
	s := evergreen.GetEnvironment().Settings()
	if s.TaskLimits.MaxHourlyPatchTasks == 0 {
		return nil
	}
	numTasksToActivate := 0
	for _, t := range tasks {
		if !includeDisplayAndTaskGroups && (t.DisplayOnly || t.IsPartOfDisplay(ctx) || t.IsPartOfSingleHostTaskGroup()) {
			continue
		}
		if t.Activated {
			numTasksToActivate++
			numTasksToActivate += utility.FromIntPtr(t.EstimatedNumActivatedGeneratedTasks)
		}
	}
	return UpdateSchedulingLimit(ctx, username, requester, numTasksToActivate, true)
}

func FindExecTasksToReset(ctx context.Context, t *Task) ([]string, error) {
	if !t.IsRestartFailedOnly() {
		return t.ExecutionTasks, nil
	}

	failedExecTasks, err := FindWithFields(ctx, FailedTasksByIds(t.ExecutionTasks), IdKey)
	if err != nil {
		return nil, errors.Wrap(err, "retrieving failed execution tasks")
	}
	failedExecTaskIds := []string{}
	for _, et := range failedExecTasks {
		failedExecTaskIds = append(failedExecTaskIds, et.Id)
	}
	return failedExecTaskIds, nil
}

// FindHostSchedulable finds all tasks that can be scheduled for a distro
// primary queue.
func FindHostSchedulable(ctx context.Context, distroID string) ([]Task, error) {
	query := schedulableHostTasksQuery()

	if err := addApplicableDistroFilter(ctx, distroID, DistroIdKey, query); err != nil {
		return nil, errors.WithStack(err)
	}

	return Find(ctx, query)
}

func addApplicableDistroFilter(ctx context.Context, id string, fieldName string, query bson.M) error {
	if id == "" {
		return nil
	}

	aliases, err := distro.FindApplicableDistroIDs(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(aliases) == 1 {
		query[fieldName] = aliases[0]
	} else {
		query[fieldName] = bson.M{"$in": aliases}
	}

	return nil
}

// FindHostSchedulableForAlias finds all tasks that can be scheduled for a
// distro secondary queue.
func FindHostSchedulableForAlias(ctx context.Context, id string) ([]Task, error) {
	q := schedulableHostTasksQuery()

	if err := addApplicableDistroFilter(ctx, id, SecondaryDistrosKey, q); err != nil {
		return nil, errors.WithStack(err)
	}

	// Single-host task groups can't be put in an alias queue, because it can
	// cause a race when assigning tasks to hosts where the tasks in the task
	// group might be assigned to different hosts.
	q[TaskGroupMaxHostsKey] = bson.M{"$ne": 1}

	return FindAll(ctx, db.Query(q))
}

func (t *Task) IsPartOfSingleHostTaskGroup() bool {
	return t.TaskGroup != "" && t.TaskGroupMaxHosts == 1
}

// HasDependenciesMet indicates whether the task has had its dependencies met.
func (t *Task) HasDependenciesMet() bool {
	return len(t.DependsOn) == 0 || t.OverrideDependencies || !utility.IsZeroTime(t.DependenciesMetTime)
}

// HasCheckRun retruns true if the task specifies a check run path.
func (t *Task) HasCheckRun() bool {
	return t.CheckRunPath != nil
}

func (t *Task) IsPartOfDisplay(ctx context.Context) bool {
	// if display task ID is nil, we need to check manually if we have an execution task
	if t.DisplayTaskId == nil {
		dt, err := t.GetDisplayTask(ctx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":        "unable to get display task",
				"execution_task": t.Id,
			}))
			return false
		}
		return dt != nil
	}

	return utility.FromStringPtr(t.DisplayTaskId) != ""
}

func (t *Task) GetDisplayTask(ctx context.Context) (*Task, error) {
	if t.DisplayTask != nil {
		return t.DisplayTask, nil
	}
	dtId := utility.FromStringPtr(t.DisplayTaskId)
	if t.DisplayTaskId != nil && dtId == "" {
		// display task ID is explicitly set to empty if it's not a display task
		return nil, nil
	}
	var dt *Task
	var err error
	if t.Archived {
		if dtId != "" {
			dt, err = FindOneOldByIdAndExecution(ctx, dtId, t.Execution)
		} else {
			dt, err = FindOneOld(ctx, ByExecutionTask(t.OldTaskId))
			if dt != nil {
				dtId = dt.OldTaskId // save the original task ID to cache
			}
		}
	} else {
		if dtId != "" {
			dt, err = FindOneId(ctx, dtId)
		} else {
			dt, err = FindOne(ctx, db.Query(ByExecutionTask(t.Id)))
			if dt != nil {
				dtId = dt.Id
			}
		}
	}
	if err != nil {
		return nil, err
	}

	if t.DisplayTaskId == nil {
		grip.Info(message.Fields{
			"message": "missing display task ID",
			"task_id": t.Id,
			"dt_id":   dtId,
			"ticket":  "DEVPROD-13634",
		})
		// Cache display task ID for future use. If we couldn't find the display task,
		// we cache the empty string to show that it doesn't exist.
		grip.Error(message.WrapError(t.SetDisplayTaskID(ctx, dtId), message.Fields{
			"message":         "failed to cache display task ID for task",
			"task_id":         t.Id,
			"display_task_id": dtId,
		}))
	}

	t.DisplayTask = dt
	return dt, nil
}

// GetAllDependencies returns all the dependencies the tasks in taskIDs rely on
func GetAllDependencies(ctx context.Context, taskIDs []string, taskMap map[string]*Task) ([]Dependency, error) {
	// fill in the gaps in taskMap
	tasksToFetch := []string{}
	for _, tID := range taskIDs {
		if _, ok := taskMap[tID]; !ok {
			tasksToFetch = append(tasksToFetch, tID)
		}
	}
	missingTaskMap := make(map[string]*Task)
	if len(tasksToFetch) > 0 {
		missingTasks, err := FindAll(ctx, db.Query(ByIds(tasksToFetch)).WithFields(DependsOnKey))
		if err != nil {
			return nil, errors.Wrap(err, "getting tasks missing from map")
		}
		if missingTasks == nil {
			return nil, errors.New("no missing tasks found")
		}
		for i, t := range missingTasks {
			missingTaskMap[t.Id] = &missingTasks[i]
		}
	}

	// extract the set of dependencies
	depSet := make(map[Dependency]bool)
	for _, tID := range taskIDs {
		t, ok := taskMap[tID]
		if !ok {
			t, ok = missingTaskMap[tID]
		}
		if !ok {
			return nil, errors.Errorf("task '%s' does not exist", tID)
		}
		for _, dep := range t.DependsOn {
			depSet[dep] = true
		}
	}

	deps := make([]Dependency, 0, len(depSet))
	for dep := range depSet {
		deps = append(deps, dep)
	}

	return deps, nil
}

func (t *Task) FetchExpectedDuration(ctx context.Context) util.DurationStats {
	if t.DurationPrediction.TTL == 0 {
		t.DurationPrediction.TTL = utility.JitterInterval(predictionTTL)
	}

	if t.DurationPrediction.Value == 0 && t.ExpectedDuration != 0 {
		// this is probably just backfill, if we have an
		// expected duration, let's assume it was collected
		// before now slightly.
		t.DurationPrediction.Value = t.ExpectedDuration
		t.DurationPrediction.CollectedAt = time.Now().Add(-time.Minute)

		if err := t.cacheExpectedDuration(ctx); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"task":    t.Id,
				"message": "caching expected duration",
			}))
		}

		return util.DurationStats{Average: t.ExpectedDuration, StdDev: t.ExpectedDurationStdDev}
	}

	refresher := func(previous util.DurationStats) (util.DurationStats, bool) {
		defaultVal := util.DurationStats{Average: defaultTaskDuration, StdDev: 0}
		vals, err := getExpectedDurationsForWindow(t.DisplayName, t.Project, t.BuildVariant,
			time.Now().Add(-taskCompletionEstimateWindow), time.Now())
		grip.Notice(message.WrapError(err, message.Fields{
			"name":      t.DisplayName,
			"id":        t.Id,
			"project":   t.Project,
			"variant":   t.BuildVariant,
			"operation": "fetching expected duration, expect stale scheduling data",
		}))
		if err != nil {
			return defaultVal, false
		}

		if len(vals) != 1 {
			if previous.Average == 0 {
				return defaultVal, true
			}

			return previous, true
		}

		avg := time.Duration(vals[0].ExpectedDuration)
		if avg == 0 {
			return defaultVal, true
		}
		stdDev := time.Duration(vals[0].StdDev)
		return util.DurationStats{Average: avg, StdDev: stdDev}, true
	}

	grip.Error(message.WrapError(t.DurationPrediction.SetRefresher(refresher), message.Fields{
		"message": "problem setting cached value refresher",
		"cause":   "programmer error",
	}))

	stats, ok := t.DurationPrediction.Get()
	if ok {
		if err := t.cacheExpectedDuration(ctx); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"task":    t.Id,
				"message": "caching expected duration",
			}))
		}
	}
	t.ExpectedDuration = stats.Average
	t.ExpectedDurationStdDev = stats.StdDev

	return stats
}

// TaskStatusCount holds counts for task statuses
type TaskStatusCount struct {
	Succeeded    int `json:"succeeded"`
	Failed       int `json:"failed"`
	Started      int `json:"started"`
	Undispatched int `json:"undispatched"`
	Inactive     int `json:"inactive"`
	Dispatched   int `json:"dispatched"`
	TimedOut     int `json:"timed_out"`
}

func (tsc *TaskStatusCount) IncrementStatus(status string, statusDetails apimodels.TaskEndDetail) {
	switch status {
	case evergreen.TaskSucceeded:
		tsc.Succeeded++
	case evergreen.TaskFailed, evergreen.TaskSetupFailed:
		if statusDetails.TimedOut && statusDetails.Description == evergreen.TaskDescriptionHeartbeat {
			tsc.TimedOut++
		} else {
			tsc.Failed++
		}
	case evergreen.TaskStarted, evergreen.TaskDispatched:
		tsc.Started++
	case evergreen.TaskUndispatched:
		tsc.Undispatched++
	case evergreen.TaskInactive:
		tsc.Inactive++
	}
}

const jqlBFQuery = "(project in (%v)) and ( %v ) order by updatedDate desc"

// Generates a jira JQL string from the task
// When we search in jira for a task we search in the specified JIRA project
// If there are any test results, then we only search by test file
// name of all of the failed tests.
// Otherwise we search by the task name.
func (t *Task) GetJQL(searchProjects []string) string {
	var jqlParts []string
	var jqlClause string
	for _, testResult := range t.LocalTestResults {
		if testResult.Status == evergreen.TestFailedStatus {
			testName := testResult.GetDisplayTestName()
			fileParts := eitherSlash.Split(testName, -1)
			jqlParts = append(jqlParts, fmt.Sprintf("text~\"%v\"", util.EscapeJQLReservedChars(fileParts[len(fileParts)-1])))
		}
	}
	if jqlParts != nil {
		jqlClause = strings.Join(jqlParts, " or ")
	} else {
		jqlClause = fmt.Sprintf("text~\"%v\"", util.EscapeJQLReservedChars(t.DisplayName))
	}

	return fmt.Sprintf(jqlBFQuery, strings.Join(searchProjects, ", "), jqlClause)
}

// Blocked returns if a task cannot run given the state of the task
func (t *Task) Blocked() bool {
	if t.OverrideDependencies {
		return false
	}

	for _, dependency := range t.DependsOn {
		if dependency.Unattainable {
			return true
		}
	}
	return false
}

// WillRun returns true if the task will run eventually, but has not started
// running yet. This is logically equivalent to evergreen.TaskWillRun from
// (Task).GetDisplayStatus.
func (t *Task) WillRun() bool {
	return t.Status == evergreen.TaskUndispatched && t.Activated && !t.Blocked()
}

// IsUnscheduled returns true if a task is unscheduled and will not run. This is
// logically equivalent to evergreen.TaskUnscheduled from
// (Task).GetDisplayStatus.
func (t *Task) IsUnscheduled() bool {
	return t.Status == evergreen.TaskUndispatched && !t.Activated
}

// IsInProgress returns true if the task has been dispatched and is about to
// run, or is already running.
func (t *Task) IsInProgress() bool {
	return utility.StringSliceContains(evergreen.TaskInProgressStatuses, t.Status)
}

func (t *Task) ToTaskNode() TaskNode {
	return TaskNode{
		Name:    t.DisplayName,
		Variant: t.BuildVariant,
		ID:      t.Id,
	}
}

func AnyActiveTasks(tasks []Task) bool {
	for _, t := range tasks {
		if t.Activated {
			return true
		}
	}
	return false
}

func TaskSliceToMap(tasks []Task) map[string]Task {
	taskMap := make(map[string]Task, len(tasks))
	for _, t := range tasks {
		taskMap[t.Id] = t
	}

	return taskMap
}

func GetLatestExecution(ctx context.Context, taskId string) (int, error) {
	var t *Task
	var err error
	t, err = FindOneId(ctx, taskId)
	if err != nil {
		return -1, err
	}
	if t == nil {
		pieces := strings.Split(taskId, "_")
		pieces = pieces[:len(pieces)-1]
		taskId = strings.Join(pieces, "_")
		t, err = FindOneId(ctx, taskId)
		if err != nil {
			return -1, errors.Wrap(err, "getting task")
		}
	}
	if t == nil {
		return -1, errors.Errorf("task '%s' not found", taskId)
	}
	return t.Execution, nil
}

// GetTimeSpent returns the total time_taken and makespan of tasks
func GetTimeSpent(tasks []Task) (time.Duration, time.Duration) {
	var timeTaken time.Duration
	earliestStartTime := utility.MaxTime
	latestFinishTime := utility.ZeroTime
	for _, t := range tasks {
		if t.DisplayOnly {
			continue
		}
		timeTaken += t.TimeTaken
		if !utility.IsZeroTime(t.StartTime) && t.StartTime.Before(earliestStartTime) {
			earliestStartTime = t.StartTime
		}
		if t.FinishTime.After(latestFinishTime) {
			latestFinishTime = t.FinishTime
		}
	}

	if earliestStartTime == utility.MaxTime || latestFinishTime == utility.ZeroTime {
		return 0, 0
	}

	return timeTaken, latestFinishTime.Sub(earliestStartTime)
}

// GetFormattedTimeSpent returns the total time_taken and makespan of tasks as a formatted string
func GetFormattedTimeSpent(tasks []Task) (string, string) {
	timeTaken, makespan := GetTimeSpent(tasks)

	t := timeTaken.Round(time.Second).String()
	m := makespan.Round(time.Second).String()

	return formatDuration(t), formatDuration(m)
}

func formatDuration(duration string) string {
	regex := regexp.MustCompile(`\d*[dhms]`)
	return strings.TrimSpace(regex.ReplaceAllStringFunc(duration, func(m string) string {
		return m + " "
	}))
}

type TasksSortOrder struct {
	Key   string
	Order int
}

type GetTasksByProjectAndCommitOptions struct {
	Project        string
	CommitHash     string
	StartingTaskId string
	Status         string
	VariantName    string
	VariantRegex   string
	TaskName       string
	Limit          int
}

func AddParentDisplayTasks(ctx context.Context, tasks []Task) ([]Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}
	taskIDs := []string{}
	tasksCopy := tasks
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	parents, err := FindAll(ctx, db.Query(ByExecutionTasks(taskIDs)))
	if err != nil {
		return nil, errors.Wrap(err, "finding parent display tasks")
	}
	childrenToParents := map[string]*Task{}
	for i, dt := range parents {
		for _, et := range dt.ExecutionTasks {
			childrenToParents[et] = &parents[i]
		}
	}
	for i, t := range tasksCopy {
		if childrenToParents[t.Id] != nil {
			t.DisplayTask = childrenToParents[t.Id]
			tasksCopy[i] = t
		}
	}
	return tasksCopy, nil
}

// UpdateDependsOn appends new dependencies to tasks that already depend on this task
// if the task does not explicitly omit having generated tasks as dependencies
func (t *Task) UpdateDependsOn(ctx context.Context, status string, newDependencyIDs []string) error {
	newDependencies := make([]Dependency, 0, len(newDependencyIDs))
	for _, depID := range newDependencyIDs {
		if depID == t.Id {
			grip.Error(message.Fields{
				"message": "task is attempting to add a dependency on itself, skipping this dependency",
				"task_id": t.Id,
				"stack":   string(debug.Stack()),
			})
			continue
		}

		newDependencies = append(newDependencies, Dependency{
			TaskId: depID,
			Status: status,
		})
	}

	_, err := UpdateAll(
		ctx,
		bson.M{
			DependsOnKey: bson.M{"$elemMatch": bson.M{
				DependencyTaskIdKey:             t.Id,
				DependencyStatusKey:             status,
				DependencyOmitGeneratedTasksKey: bson.M{"$ne": true},
			}},
		},
		[]bson.M{
			{"$set": bson.M{
				DependsOnKey: bson.M{
					"$concatArrays": []any{
						"$" + DependsOnKey,
						// Add dependencies to this task, but avoid adding a
						// dependency if it's the task's own ID since that would
						// create a self-dependency cycle.
						bson.M{
							"$filter": bson.M{
								"input": newDependencies,
								"as":    "dep",
								"cond": bson.M{
									"$ne": []any{"$$dep." + DependencyTaskIdKey, "$" + IdKey},
								},
							},
						},
					},
				},
			}},
			addDisplayStatusCache,
		},
	)

	return errors.Wrap(err, "updating dependencies")
}

func (t *Task) SetDisplayTaskID(ctx context.Context, id string) error {
	t.DisplayTaskId = utility.ToStringPtr(id)
	return errors.WithStack(UpdateOne(ctx, bson.M{IdKey: t.Id},
		bson.M{"$set": bson.M{
			DisplayTaskIdKey: id,
		}}))
}

// SetCheckRunId sets the checkRunId for the task
func (t *Task) SetCheckRunId(ctx context.Context, checkRunId int64) error {
	if err := UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				CheckRunIdKey: checkRunId,
			},
		},
	); err != nil {
		return err
	}
	t.CheckRunId = utility.ToInt64Ptr(checkRunId)
	return nil
}

func (t *Task) SetNumDependents(ctx context.Context) error {
	update := bson.M{
		"$set": bson.M{
			NumDependentsKey: t.NumDependents,
		},
	}
	if t.NumDependents == 0 {
		update = bson.M{"$unset": bson.M{
			NumDependentsKey: "",
		}}
	}
	return UpdateOne(ctx, bson.M{
		IdKey: t.Id,
	}, update)
}

func AddDisplayTaskIdToExecTasks(ctx context.Context, displayTaskId string, execTasksToUpdate []string) error {
	if len(execTasksToUpdate) == 0 {
		return nil
	}
	_, err := UpdateAll(ctx, bson.M{
		IdKey: bson.M{"$in": execTasksToUpdate},
	},
		bson.M{"$set": bson.M{
			DisplayTaskIdKey: displayTaskId,
		}},
	)
	return err
}

func AddExecTasksToDisplayTask(ctx context.Context, displayTaskId string, execTasks []string, displayTaskActivated bool) error {
	if len(execTasks) == 0 {
		return nil
	}
	update := bson.M{"$addToSet": bson.M{
		ExecutionTasksKey: bson.M{"$each": execTasks},
	}}

	if displayTaskActivated {
		// verify that the display task isn't already activated
		dt, err := FindOneId(ctx, displayTaskId)
		if err != nil {
			return errors.Wrap(err, "getting display task")
		}
		if dt == nil {
			return errors.Errorf("display task not found")
		}
		if !dt.Activated {
			update["$set"] = bson.M{
				ActivatedKey:     true,
				ActivatedTimeKey: time.Now(),
			}
		}
	}

	return UpdateOne(
		ctx,
		bson.M{IdKey: displayTaskId},
		update,
	)
}

// in the process of aborting and will eventually reset themselves.
func (t *Task) FindAbortingAndResettingDependencies(ctx context.Context) ([]Task, error) {
	recursiveDeps, err := GetRecursiveDependenciesUp(ctx, []Task{*t}, map[string]Task{})
	if err != nil {
		return nil, errors.Wrap(err, "getting recursive parent dependencies")
	}
	var taskIDs []string
	for _, dep := range recursiveDeps {
		taskIDs = append(taskIDs, dep.Id)
	}
	if len(taskIDs) == 0 {
		return nil, nil
	}

	// GetRecursiveDependenciesUp only populates a subset of the task's
	// in-memory fields, so query for them again with the necessary keys.
	q := db.Query(bson.M{
		IdKey:      bson.M{"$in": taskIDs},
		AbortedKey: true,
		"$or": []bson.M{
			{ResetWhenFinishedKey: true},
			{ResetFailedWhenFinishedKey: true},
		},
	})
	return FindAll(ctx, q)
}

// SortingValueBreakdown is the full breakdown of the final value used to sort on in the queue,
// with accompanying breakdowns of priority and rank value.
type SortingValueBreakdown struct {
	TaskGroupLength    int64
	TotalValue         int64
	PriorityBreakdown  PriorityBreakdown
	RankValueBreakdown RankValueBreakdown
}

// PriorityBreakdown contains information on how much various factors impacted the custom
// priority value that is used in the queue sorting value equation.
type PriorityBreakdown struct {
	// InitialPriorityImpact represents how much of the total priority can be attributed to the
	// original priority set on a task.
	InitialPriorityImpact int64
	// TaskGroupImpact represents how much of the total priority can be attributed to a
	// task being in a task group.
	TaskGroupImpact int64
	// GeneratorTaskImpact represents how much of the total priority can be attributed to a
	// task having a generate.tasks command in it.
	GeneratorTaskImpact int64
	// CommitQueueImpact represents how much of the total priority can be attributed to a task
	// being in the commit queue.
	CommitQueueImpact int64
}

// RankValueBreakdown contains information on how much various factors impacted the custom
// rank value that is used in the queue sorting value equation.
type RankValueBreakdown struct {
	// CommitQueueImpact represents how much of the total rank value can be attributed to a task
	// being in the commit queue.
	CommitQueueImpact int64
	// NumDependentsImpact represents how much of the total rank value can be attributed to the number
	// of tasks that are dependents of a task.
	NumDependentsImpact int64
	// EstimatedRuntimeImpact represents how much of the total rank value can be attributed to the
	// estimated runtime of a task.
	EstimatedRuntimeImpact int64
	// MainlineWaitTimeImpact represents how much of the total rank value can be attributed to
	// how long a mainline task has been waiting for dispatch.
	MainlineWaitTimeImpact int64
	// StepbackImpact represents how much of the total rank value can be attributed to
	// a task being activated by the stepback process.
	StepbackImpact int64
	// PatchImpact represents how much of the total rank value can be attributed to a task
	// being part of a patch.
	PatchImpact int64
	// PatchWaitTimeImpact represents how much of the total rank value can be attributed to
	// how long a patch task has been waiting for dispatch.
	PatchWaitTimeImpact int64
}

const (
	priorityBreakdownAttributePrefix = "evergreen.priority_breakdown"
	rankBreakdownAttributePrefix     = "evergreen.rank_breakdown"
	priorityScaledRankAttribute      = "evergreen.priority_scaled_rank"
)

// SetSortingValueBreakdownAttributes saves a full breakdown which compartmentalizes each factor that played a role in computing the
// overall value used to sort it in the queue, and creates a honeycomb trace with this data to enable dashboards/analysis.
func (t *Task) SetSortingValueBreakdownAttributes(ctx context.Context, breakdown SortingValueBreakdown) {
	_, span := tracer.Start(ctx, "queue-factor-breakdown")
	defer span.End()
	span.SetAttributes(
		attribute.String(evergreen.DistroIDOtelAttribute, t.DistroId),
		attribute.String(evergreen.TaskIDOtelAttribute, t.Id),
		attribute.Int64(priorityScaledRankAttribute, breakdown.TotalValue),
		// Priority values
		attribute.Int64(fmt.Sprintf("%s.base_priority", priorityBreakdownAttributePrefix), breakdown.PriorityBreakdown.InitialPriorityImpact),
		attribute.Int64(fmt.Sprintf("%s.task_group", priorityBreakdownAttributePrefix), breakdown.PriorityBreakdown.TaskGroupImpact),
		attribute.Int64(fmt.Sprintf("%s.generate_task", priorityBreakdownAttributePrefix), breakdown.PriorityBreakdown.GeneratorTaskImpact),
		attribute.Int64(fmt.Sprintf("%s.commit_queue", priorityBreakdownAttributePrefix), breakdown.PriorityBreakdown.CommitQueueImpact),
		// Rank values
		attribute.Int64(fmt.Sprintf("%s.commit_queue", rankBreakdownAttributePrefix), breakdown.RankValueBreakdown.CommitQueueImpact),
		attribute.Int64(fmt.Sprintf("%s.patch", rankBreakdownAttributePrefix), breakdown.RankValueBreakdown.PatchImpact),
		attribute.Int64(fmt.Sprintf("%s.patch_wait_time", rankBreakdownAttributePrefix), breakdown.RankValueBreakdown.PatchWaitTimeImpact),
		attribute.Int64(fmt.Sprintf("%s.mainline_wait_time", rankBreakdownAttributePrefix), breakdown.RankValueBreakdown.MainlineWaitTimeImpact),
		attribute.Int64(fmt.Sprintf("%s.stepback", rankBreakdownAttributePrefix), breakdown.RankValueBreakdown.StepbackImpact),
		attribute.Int64(fmt.Sprintf("%s.num_dependents", rankBreakdownAttributePrefix), breakdown.RankValueBreakdown.NumDependentsImpact),
		attribute.Int64(fmt.Sprintf("%s.estimated_runtime", rankBreakdownAttributePrefix), breakdown.RankValueBreakdown.EstimatedRuntimeImpact),
		// Priority percentage values
		attribute.Float64(fmt.Sprintf("%s.base_priority_pct", priorityBreakdownAttributePrefix), float64(breakdown.PriorityBreakdown.InitialPriorityImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.task_group_pct", priorityBreakdownAttributePrefix), float64(breakdown.PriorityBreakdown.TaskGroupImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.generate_task_pct", priorityBreakdownAttributePrefix), float64(breakdown.PriorityBreakdown.GeneratorTaskImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.commit_queue_pct", priorityBreakdownAttributePrefix), float64(breakdown.PriorityBreakdown.CommitQueueImpact/breakdown.TotalValue*100)),
		// Rank value percentage values
		attribute.Float64(fmt.Sprintf("%s.patch_pct", rankBreakdownAttributePrefix), float64(breakdown.RankValueBreakdown.PatchImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.patch_wait_time_pct", rankBreakdownAttributePrefix), float64(breakdown.RankValueBreakdown.PatchWaitTimeImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.commit_queue_pct", rankBreakdownAttributePrefix), float64(breakdown.RankValueBreakdown.CommitQueueImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.mainline_wait_time_pct", rankBreakdownAttributePrefix), float64(breakdown.RankValueBreakdown.MainlineWaitTimeImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.stepback_pct", rankBreakdownAttributePrefix), float64(breakdown.RankValueBreakdown.StepbackImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.num_dependents_pct", rankBreakdownAttributePrefix), float64(breakdown.RankValueBreakdown.NumDependentsImpact/breakdown.TotalValue*100)),
		attribute.Float64(fmt.Sprintf("%s.estimated_runtime_pct", rankBreakdownAttributePrefix), float64(breakdown.RankValueBreakdown.EstimatedRuntimeImpact/breakdown.TotalValue*100)),
	)
	t.SortingValueBreakdown = breakdown
}

// CalculateOnDemandCost calculates the on-demand cost of running a task.
func CalculateOnDemandCost(runtimeSeconds float64, distroCostData distro.CostData, financeConfig evergreen.CostConfig) float64 {
	if runtimeSeconds <= 0 {
		return 0
	}

	// Convert the on-demand rate from per hour to per second
	onDemandRatePerSecond := distroCostData.OnDemandRate / 3600.0

	// Apply the discount
	discountedRate := onDemandRatePerSecond * (1 - financeConfig.OnDemandDiscount)

	// Calculate the total cost for the given runtime
	return runtimeSeconds * discountedRate
}

// CalculateAdjustedTaskCost calculates the adjusted cost of running a task based on the provided runtime,
// distro cost data, and admin finance settings using the finance formula.
func CalculateAdjustedTaskCost(runtimeSeconds float64, distroCostData distro.CostData, financeConfig evergreen.CostConfig) float64 {
	if runtimeSeconds <= 0 {
		return 0
	}

	savingsPlanComponent := financeConfig.FinanceFormula * (distroCostData.SavingsPlanRate / 3600.0) * (1 - financeConfig.SavingsPlanDiscount)
	onDemandComponent := (1 - financeConfig.FinanceFormula) * (distroCostData.OnDemandRate / 3600.0) * (1 - financeConfig.OnDemandDiscount)
	costPerSecond := savingsPlanComponent + onDemandComponent

	return runtimeSeconds * costPerSecond
}

// CalculateTaskCost calculates both on-demand and adjusted costs for a task.
func CalculateTaskCost(runtimeSeconds float64, distroCostData distro.CostData, financeConfig evergreen.CostConfig) cost.Cost {
	return cost.Cost{
		OnDemandEC2Cost: CalculateOnDemandCost(runtimeSeconds, distroCostData, financeConfig),
		AdjustedEC2Cost: CalculateAdjustedTaskCost(runtimeSeconds, distroCostData, financeConfig),
	}
}

func (t *Task) getFinanceConfigAndDistro(ctx context.Context) (evergreen.CostConfig, distro.CostData, error) {
	financeConfig := evergreen.CostConfig{}
	if err := financeConfig.Get(ctx); err != nil {
		return financeConfig, distro.CostData{}, errors.Wrap(err, "getting finance configuration")
	}

	if !financeConfig.IsConfigured() {
		return financeConfig, distro.CostData{}, errors.New("finance configuration not set up")
	}

	d, err := distro.FindOneId(ctx, t.DistroId)
	if err != nil {
		return financeConfig, distro.CostData{}, errors.Wrapf(err, "finding distro '%s'", t.DistroId)
	}
	if d == nil {
		return financeConfig, distro.CostData{}, errors.Errorf("distro '%s' not found", t.DistroId)
	}

	if !d.CostData.IsConfigured() {
		return financeConfig, distro.CostData{}, errors.New("distro cost data not configured")
	}

	return financeConfig, d.CostData, nil
}

func (t *Task) UpdateTaskCost(ctx context.Context) error {
	if t.TimeTaken <= 0 {
		return nil
	}

	financeConfig, costData, err := t.getFinanceConfigAndDistro(ctx)
	if err != nil {
		return nil
	}

	runtimeSeconds := t.TimeTaken.Seconds()
	t.TaskCost = CalculateTaskCost(runtimeSeconds, costData, financeConfig)

	return UpdateOne(ctx, bson.M{"_id": t.Id}, bson.M{
		"$set": bson.M{
			TaskCostKey: t.TaskCost,
		},
	})
}

type CostPredictionResult struct {
	PredictedCost cost.Cost
}

func (t *Task) ComputePredictedCostForWeek(ctx context.Context) (CostPredictionResult, error) {
	end := time.Now()
	start := end.Add(-taskCompletionEstimateWindow)

	results, err := getPredictedCostsForWindow(ctx, t.DisplayName, t.Project, t.BuildVariant, start, end)
	if err != nil {
		return CostPredictionResult{}, errors.Wrap(err, "querying expected costs")
	}

	if len(results) == 0 {
		return CostPredictionResult{}, nil
	}

	result := results[0]
	return CostPredictionResult{
		PredictedCost: cost.Cost{
			OnDemandEC2Cost: result.AvgOnDemandCost,
			AdjustedEC2Cost: result.AvgAdjustedCost,
		},
	}, nil
}

func (t *Task) HasCostPrediction() bool {
	return !t.PredictedTaskCost.IsZero()
}

// GetDisplayCost returns the task's cost for display purposes.
func (t *Task) GetDisplayCost() cost.Cost {
	if !t.TaskCost.IsZero() {
		return t.TaskCost
	}
	if !t.PredictedTaskCost.IsZero() {
		return t.PredictedTaskCost
	}
	return cost.Cost{}
}

// SetPredictedCost sets the task's predicted cost.
func (t *Task) SetPredictedCost(c cost.Cost) {
	t.PredictedTaskCost = c
}

func addPredictedCostToUpdate(setFields bson.M, predictedCost cost.Cost) {
	if !predictedCost.IsZero() {
		setFields[PredictedTaskCostKey] = predictedCost
	}
}

// moveLogsByNamesToBucket moves task + test logs to the specified bucket.
func (t *Task) moveLogsByNamesToBucket(ctx context.Context, settings *evergreen.Settings, output *TaskOutput, sourceBucketCfg *evergreen.BucketConfig) error {
	if output.TestLogs.BucketConfig != output.TaskLogs.BucketConfig {
		// test logs and task logs will always be in the same bucket
		return errors.New("test log and task log buckets do not match")
	}
	failedCfg := settings.Buckets.LogBucketFailedTasks
	if failedCfg.Name == "" {
		return errors.New("failed bucket is not configured")
	}
	// Use the provided source bucket config if available, otherwise use the task's current bucket config
	srcCfg := output.TestLogs.BucketConfig
	if sourceBucketCfg != nil && sourceBucketCfg.Name != "" {
		srcCfg = *sourceBucketCfg
	}
	srcBucket, err := newBucket(ctx, srcCfg, output.TestLogs.AWSCredentials)
	if err != nil {
		return errors.Wrap(err, "getting regular test log bucket")
	}

	logService := log.NewLogServiceV0(srcBucket)

	logNames := make([]string, 0, 4)
	// add task logs
	for _, logType := range []TaskLogType{TaskLogTypeAgent, TaskLogTypeSystem, TaskLogTypeTask} {
		logNames = append(logNames, getLogName(*t, logType, output.TaskLogs.ID()))
	}
	// add test logs
	logNames = append(logNames, fmt.Sprintf("%s/%s/%d/%s", t.Project, t.Id, t.Execution, output.TestLogs.ID()))

	keys, err := logService.GetChunkKeys(ctx, logNames)
	if err != nil {
		return errors.Wrap(err, "getting chunk keys for log names")
	}
	allKeys := keys
	failedBucket, err := newBucket(ctx, failedCfg, output.TestLogs.AWSCredentials)
	if err != nil {
		return errors.Wrap(err, "getting failed bucket")
	}

	if err = logService.MoveObjectsToBucket(ctx, allKeys, failedBucket); err != nil {
		return errors.Wrap(err, "moving logs to failed bucket")
	}

	// Update the task output info with the new bucket config.
	t.TaskOutputInfo.TaskLogs.BucketConfig = failedCfg
	t.TaskOutputInfo.TestLogs.BucketConfig = failedCfg
	err = UpdateOne(
		ctx,
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskOutputInfoKey: t.TaskOutputInfo,
			},
		})
	return errors.Wrapf(err, "updating task output for task %s", t.Id)
}

// MoveTestAndTaskLogsToFailedBucket moves task + test logs to the failed-task bucket
// using the provided source bucket config
func (t *Task) MoveTestAndTaskLogsToFailedBucket(ctx context.Context, settings *evergreen.Settings, sourceBucketCfg evergreen.BucketConfig) error {
	if t.UsesLongRetentionBucket(settings) {
		return nil
	}
	output, ok := t.GetTaskOutputSafe()
	if !ok {
		return nil
	}

	return t.moveLogsByNamesToBucket(ctx, settings, output, &sourceBucketCfg)
}

// UsesLongRetentionBucket returns true if the task failed and is not in LongRetentionProjects.
func (t *Task) UsesLongRetentionBucket(settings *evergreen.Settings) bool {
	if settings != nil && slices.Contains(settings.Buckets.LongRetentionProjects, t.Project) {
		return true
	}
	return false
}

// HasValidDistro determines if the task has a valid distro.
func (t *Task) HasValidDistro(ctx context.Context) bool {
	_, err := distro.FindApplicableDistroIDs(ctx, t.DistroId)
	if err == nil {
		return true
	}
	for _, secondaryDistro := range t.SecondaryDistros {
		_, err = distro.FindApplicableDistroIDs(ctx, secondaryDistro)
		if err == nil {
			return true
		}
	}
	return false
}
