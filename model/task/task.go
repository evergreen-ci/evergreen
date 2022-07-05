package task

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/tarjan"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

	// UnschedulableThreshold is the threshold after which a task waiting to
	// dispatch should be unscheduled due to staleness.
	UnschedulableThreshold = 7 * 24 * time.Hour

	// indicates the window of completed tasks we want to use in computing
	// average task duration. By default we use tasks that have
	// completed within the last 7 days
	taskCompletionEstimateWindow = 24 * 7 * time.Hour

	// if we have no data on a given task, default to 10 minutes so we
	// have some new hosts spawned
	defaultTaskDuration = 10 * time.Minute

	// length of time to cache the expected duration in the task document
	predictionTTL = 8 * time.Hour
)

var (
	// A regex that matches either / or \ for splitting directory paths
	// on either windows or linux paths.
	eitherSlash *regexp.Regexp = regexp.MustCompile(`[/\\]`)
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
	// ContainerAllocated - for tasks that run on containers, the time the container was allocated.
	CreateTime             time.Time `bson:"create_time" json:"create_time"`
	IngestTime             time.Time `bson:"injest_time" json:"ingest_time"`
	DispatchTime           time.Time `bson:"dispatch_time" json:"dispatch_time"`
	ScheduledTime          time.Time `bson:"scheduled_time" json:"scheduled_time"`
	StartTime              time.Time `bson:"start_time" json:"start_time"`
	FinishTime             time.Time `bson:"finish_time" json:"finish_time"`
	ActivatedTime          time.Time `bson:"activated_time" json:"activated_time"`
	DependenciesMetTime    time.Time `bson:"dependencies_met_time,omitempty" json:"dependencies_met_time,omitempty"`
	ContainerAllocatedTime time.Time `bson:"container_allocated_time,omitempty" json:"container_allocated_time,omitempty"`

	Version            string              `bson:"version" json:"version,omitempty"`
	Project            string              `bson:"branch" json:"branch,omitempty"`
	Revision           string              `bson:"gitspec" json:"gitspec"`
	Priority           int64               `bson:"priority" json:"priority"`
	TaskGroup          string              `bson:"task_group" json:"task_group"`
	TaskGroupMaxHosts  int                 `bson:"task_group_max_hosts,omitempty" json:"task_group_max_hosts,omitempty"`
	TaskGroupOrder     int                 `bson:"task_group_order,omitempty" json:"task_group_order,omitempty"`
	Logs               *apimodels.TaskLogs `bson:"logs,omitempty" json:"logs,omitempty"`
	MustHaveResults    bool                `bson:"must_have_results,omitempty" json:"must_have_results,omitempty"`
	HasCedarResults    bool                `bson:"has_cedar_results,omitempty" json:"has_cedar_results,omitempty"`
	CedarResultsFailed bool                `bson:"cedar_results_failed,omitempty" json:"cedar_results_failed,omitempty"`
	// we use a pointer for HasLegacyResults to distinguish the default from an intentional "false"
	HasLegacyResults *bool `bson:"has_legacy_results,omitempty" json:"has_legacy_results,omitempty"`
	// only relevant if the task is running.  the time of the last heartbeat
	// sent back by the agent
	LastHeartbeat time.Time `bson:"last_heartbeat" json:"last_heartbeat"`

	// Activated indicates whether the task should be scheduled to run or not.
	Activated                bool   `bson:"activated" json:"activated"`
	ActivatedBy              string `bson:"activated_by" json:"activated_by"`
	DeactivatedForDependency bool   `bson:"deactivated_for_dependency" json:"deactivated_for_dependency"`
	// ContainerAllocated indicates whether this task has been allocated a
	// container to run it. It only applies to tasks running in containers.
	ContainerAllocated bool `bson:"container_allocated" json:"container_allocated"`
	// ContainerAllocationAttempts is the number of times this task has
	// been allocated a container to run it (for a single execution).
	ContainerAllocationAttempts int `bson:"container_allocation_attempts" json:"container_allocation_attempts"`

	BuildId  string `bson:"build_id" json:"build_id"`
	DistroId string `bson:"distro" json:"distro"`
	// Container is the name of the container configuration for running a
	// container task.
	Container string `bson:"container,omitempty" json:"container,omitempty"`
	// ContainerOpts contains the options to configure the container that will
	// run the task.
	ContainerOpts           ContainerOptions `bson:"container_options,omitempty" json:"container_options,omitempty"`
	BuildVariant            string           `bson:"build_variant" json:"build_variant"`
	BuildVariantDisplayName string           `bson:"build_variant_display_name" json:"-"`
	DependsOn               []Dependency     `bson:"depends_on" json:"depends_on"`
	NumDependents           int              `bson:"num_dependents,omitempty" json:"num_dependents,omitempty"`
	OverrideDependencies    bool             `bson:"override_dependencies,omitempty" json:"override_dependencies,omitempty"`

	// DistroAliases refer to the optional secondary distros that can be
	// associated with a task. This is used for running tasks in case there are
	// idle hosts in a distro with an empty primary queue. Despite the variable
	// name, this is a distinct concept from actual distro aliases (i.e.
	// alternative distro names).
	// TODO (EVG-15148): rename this to represent secondary distros.
	DistroAliases []string `bson:"distro_aliases,omitempty" json:"distro_aliases,omitempty"`

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

	// Set to true if the task should be considered for mainline github checks
	IsGithubCheck bool `bson:"is_github_check,omitempty" json:"is_github_check,omitempty"`

	Execution           int    `bson:"execution" json:"execution"`
	OldTaskId           string `bson:"old_task_id,omitempty" json:"old_task_id,omitempty"`
	Archived            bool   `bson:"archived,omitempty" json:"archived,omitempty"`
	RevisionOrderNumber int    `bson:"order,omitempty" json:"order,omitempty"`

	// task requester - this is used to help tell the
	// reason this task was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"r"`

	// tasks that are part of a child patch will store the id and patch number of the parent patch
	ParentPatchID     string `bson:"parent_patch_id,omitempty" json:"parent_patch_id,omitempty"`
	ParentPatchNumber int    `bson:"parent_patch_number,omitempty" json:"parent_patch_number,omitempty"`

	// Status represents the various stages the task could be in
	Status    string                  `bson:"status" json:"status"`
	Details   apimodels.TaskEndDetail `bson:"details" json:"task_end_details"`
	Aborted   bool                    `bson:"abort,omitempty" json:"abort"`
	AbortInfo AbortInfo               `bson:"abort_info,omitempty" json:"abort_info,omitempty"`

	// HostCreateDetails stores information about why host.create failed for this task
	HostCreateDetails []HostCreateDetail `bson:"host_create_details,omitempty" json:"host_create_details,omitempty"`
	// DisplayStatus is not persisted to the db. It is the status to display in the UI.
	// It may be added via aggregation
	DisplayStatus string `bson:"display_status,omitempty" json:"display_status,omitempty"`
	// BaseTask is not persisted to the db. It is the data of the task on the base commit
	// It may be added via aggregation
	BaseTask BaseTaskInfo `bson:"base_task" json:"base_task"`

	// TimeTaken is how long the task took to execute (if it has finished) or how long the task has been running (if it has started)
	TimeTaken time.Duration `bson:"time_taken" json:"time_taken"`
	// WaitSinceDependenciesMet is populated in GetDistroQueueInfo, used for host allocation
	WaitSinceDependenciesMet time.Duration `bson:"wait_since_dependencies_met,omitempty" json:"wait_since_dependencies_met,omitempty"`

	// how long we expect the task to take from start to
	// finish. expected duration is the legacy value, but the UI
	// probably depends on it, so we maintain both values.
	ExpectedDuration       time.Duration            `bson:"expected_duration,omitempty" json:"expected_duration,omitempty"`
	ExpectedDurationStdDev time.Duration            `bson:"expected_duration_std_dev,omitempty" json:"expected_duration_std_dev,omitempty"`
	DurationPrediction     util.CachedDurationValue `bson:"duration_prediction,omitempty" json:"-"`

	// test results embedded from the testresults collection
	LocalTestResults []TestResult `bson:"-" json:"test_results"`

	// display task fields
	DisplayOnly    bool     `bson:"display_only,omitempty" json:"display_only,omitempty"`
	ExecutionTasks []string `bson:"execution_tasks,omitempty" json:"execution_tasks,omitempty"`
	// ResetWhenFinished indicates that a task should be reset once it is
	// finished running. This is typically to deal with tasks that should be
	// reset but cannot do so yet because they're currently running.
	ResetWhenFinished       bool  `bson:"reset_when_finished,omitempty" json:"reset_when_finished,omitempty"`
	ResetFailedWhenFinished bool  `bson:"reset_failed_when_finished,omitempty" json:"reset_failed_when_finished,omitempty"`
	DisplayTask             *Task `bson:"-" json:"-"` // this is a local pointer from an exec to display task

	// DisplayTaskId is set to the display task ID if the task is an execution task, the empty string if it's not an execution task,
	// and is nil if we haven't yet checked whether or not this task has a display task.
	DisplayTaskId *string `bson:"display_task_id,omitempty" json:"display_task_id,omitempty"`

	// GenerateTask indicates that the task generates other tasks, which the
	// scheduler will use to prioritize this task.
	GenerateTask bool `bson:"generate_task,omitempty" json:"generate_task,omitempty"`
	// GeneratedTasks indicates that the task has already generated other tasks. This fields
	// allows us to noop future requests, since a task should only generate others once.
	GeneratedTasks bool `bson:"generated_tasks,omitempty" json:"generated_tasks,omitempty"`
	// GeneratedBy, if present, is the ID of the task that generated this task.
	GeneratedBy string `bson:"generated_by,omitempty" json:"generated_by,omitempty"`
	// GeneratedJSON is no longer used but must be kept for old tasks.
	GeneratedJSON []json.RawMessage `bson:"generate_json,omitempty" json:"generate_json,omitempty"`
	// GeneratedJSONAsString is the configuration information to create new tasks from.
	GeneratedJSONAsString []string `bson:"generated_json,omitempty" json:"generated_json,omitempty"`
	// GenerateTasksError any encountered while generating tasks.
	GenerateTasksError string `bson:"generate_error,omitempty" json:"generate_error,omitempty"`
	// GeneratedTasksToActivate is only populated if we want to override activation for these generated tasks, because of stepback.
	// Maps the build variant to a list of task names.
	GeneratedTasksToActivate map[string][]string `bson:"generated_tasks_to_stepback,omitempty" json:"generated_tasks_to_stepback,omitempty"`

	// Fields set if triggered by an upstream build
	TriggerID    string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	TriggerType  string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`

	CommitQueueMerge bool `bson:"commit_queue_merge,omitempty" json:"commit_queue_merge,omitempty"`

	CanSync       bool             `bson:"can_sync" json:"can_sync"`
	SyncAtEndOpts SyncAtEndOptions `bson:"sync_at_end_opts,omitempty" json:"sync_at_end_opts,omitempty"`

	// testResultsPopulated is a local field that indicates whether the
	// task's test results are successfully cached in LocalTestResults.
	testResultsPopulated bool
}

// ExecutionPlatform indicates the type of environment that the task runs in.
type ExecutionPlatform string

const (
	// ExecutionPlatformHost indicates that the task runs in a host.
	ExecutionPlatformHost ExecutionPlatform = "host"
	// ExecutionPlatformContainer indicates that the task runs in a container.
	ExecutionPlatformContainer ExecutionPlatform = "container"
)

// ContainerOptions represent options to create the container to run a task.
type ContainerOptions struct {
	CPU            int
	MemoryMB       int
	WorkingDir     string
	Image          string
	OS             evergreen.ContainerOS
	Arch           evergreen.ContainerArch
	WindowsVersion evergreen.WindowsVersion
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (o ContainerOptions) IsZero() bool {
	return o == ContainerOptions{}
}

func (t *Task) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(t) }
func (t *Task) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, t) }

func (t *Task) GetTaskGroupString() string {
	return fmt.Sprintf("%s_%s_%s_%s", t.TaskGroup, t.BuildVariant, t.Project, t.Version)
}

// S3Path returns the path to a task's directory dump in S3.
func (t *Task) S3Path(bv, name string) string {
	return strings.Join([]string{t.Project, t.Version, bv, name, "latest"}, "/")
}

type SyncAtEndOptions struct {
	Enabled  bool          `bson:"enabled,omitempty" json:"enabled,omitempty"`
	Statuses []string      `bson:"statuses,omitempty" json:"statuses,omitempty"`
	Timeout  time.Duration `bson:"timeout,omitempty" json:"timeout,omitempty"`
}

// Dependency represents a task that must be completed before the owning
// task can be scheduled.
type Dependency struct {
	TaskId       string `bson:"_id" json:"id"`
	Status       string `bson:"status" json:"status"`
	Unattainable bool   `bson:"unattainable" json:"unattainable"`
	// Finished indicates if the task's dependency has finished running or not.
	Finished bool `bson:"finished" json:"finished"`
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
//  TODO eventually drop all of this switching
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

// LocalTestResults is only used when transferring data from agent to api.
type LocalTestResults struct {
	Results []TestResult `json:"results"`
}

type TestResult struct {
	Status          string  `json:"status" bson:"status"`
	TestFile        string  `json:"test_file" bson:"test_file"`
	DisplayTestName string  `json:"display_test_name" bson:"display_test_name"`
	GroupID         string  `json:"group_id,omitempty" bson:"group_id,omitempty"`
	URL             string  `json:"url" bson:"url,omitempty"`
	URLRaw          string  `json:"url_raw" bson:"url_raw,omitempty"`
	LogId           string  `json:"log_id,omitempty" bson:"log_id,omitempty"`
	LineNum         int     `json:"line_num,omitempty" bson:"line_num,omitempty"`
	ExitCode        int     `json:"exit_code" bson:"exit_code"`
	StartTime       float64 `json:"start" bson:"start"`
	EndTime         float64 `json:"end" bson:"end"`
	TaskID          string  `json:"task_id" bson:"task_id"`
	Execution       int     `json:"execution" bson:"execution"`

	// LogRaw and LogTestName are not saved in the task
	LogRaw      string `json:"log_raw" bson:"log_raw,omitempty"`
	LogTestName string `json:"log_test_name" bson:"log_test_name"`
}

// GetLogTestName returns the name of the test in the logging backend. This is
// used for test logs in Cedar where the name of the test in the logging
// service may differ from that in the test results service.
func (tr TestResult) GetLogTestName() string {
	if tr.LogTestName != "" {
		return tr.LogTestName
	}

	return tr.TestFile
}

// GetDisplayTestName returns the name of the test that should be displayed in
// the UI. In most cases, this will just be TestFile.
func (tr TestResult) GetDisplayTestName() string {
	if tr.DisplayTestName != "" {
		return tr.DisplayTestName
	}

	return tr.TestFile
}

// GetLogURL returns the external or internal log URL for this test result.
//
// It is not advisable to set URL or URLRaw with the output of this function as
// those fields are reserved for external logs and used to determine URL
// generation for other log viewers.
func (tr TestResult) GetLogURL(viewer evergreen.LogViewer) string {
	root := evergreen.GetEnvironment().Settings().ApiUrl
	deprecatedLobsterURL := "https://logkeeper.mongodb.org"

	switch viewer {
	case evergreen.LogViewerHTML:
		if tr.URL != "" {
			if strings.Contains(tr.URL, deprecatedLobsterURL) {
				return strings.Replace(tr.URL, deprecatedLobsterURL, root+"/lobster", 1)
			}

			// Some test results may have internal URLs that are
			// missing the root.
			if err := util.CheckURL(tr.URL); err != nil {
				return root + tr.URL
			}

			return tr.URL
		}

		if tr.LogId != "" {
			return fmt.Sprintf("%s/test_log/%s#L%d",
				root,
				url.PathEscape(tr.LogId),
				tr.LineNum,
			)
		}

		return fmt.Sprintf("%s/test_log/%s/%d?test_name=%s&group_id=%s#L%d",
			root,
			url.PathEscape(tr.TaskID),
			tr.Execution,
			url.QueryEscape(tr.GetLogTestName()),
			url.QueryEscape(tr.GroupID),
			tr.LineNum,
		)
	case evergreen.LogViewerLobster:
		// Evergreen-hosted lobster does not support external logs nor
		// logs stored in the database.
		if tr.URL != "" || tr.URLRaw != "" || tr.LogId != "" {
			return ""
		}

		return fmt.Sprintf("%s/lobster/evergreen/test/%s/%d/%s/%s#shareLine=%d",
			root,
			url.PathEscape(tr.TaskID),
			tr.Execution,
			url.QueryEscape(tr.GetLogTestName()),
			url.QueryEscape(tr.GroupID),
			tr.LineNum,
		)
	default:
		if tr.URLRaw != "" {
			// Some test results may have internal URLs that are
			// missing the root.
			if err := util.CheckURL(tr.URL); err != nil {
				return root + tr.URL
			}

			return tr.URLRaw
		}

		if tr.LogId != "" {
			return fmt.Sprintf("%s/test_log/%s?text=true",
				root,
				url.PathEscape(tr.LogId),
			)
		}

		return fmt.Sprintf("%s/test_log/%s/%d?test_name=%s&group_id=%s&text=true",
			root,
			url.PathEscape(tr.TaskID),
			tr.Execution,
			url.QueryEscape(tr.GetLogTestName()),
			url.QueryEscape(tr.GroupID),
		)
	}
}

type DisplayTaskCache struct {
	execToDisplay map[string]*Task
	displayTasks  []*Task
}

func (c *DisplayTaskCache) Get(t *Task) (*Task, error) {
	if parent, exists := c.execToDisplay[t.Id]; exists {
		return parent, nil
	}
	displayTask, err := t.GetDisplayTask()
	if err != nil {
		return nil, err
	}
	if displayTask == nil {
		return nil, nil
	}
	for _, execTask := range displayTask.ExecutionTasks {
		c.execToDisplay[execTask] = displayTask
	}
	c.displayTasks = append(c.displayTasks, displayTask)
	return displayTask, nil
}
func (c *DisplayTaskCache) List() []*Task { return c.displayTasks }

func NewDisplayTaskCache() DisplayTaskCache {
	return DisplayTaskCache{execToDisplay: map[string]*Task{}, displayTasks: []*Task{}}
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

// IsDispatchable returns true if the task should make progress towards
// dispatching to run.
func (t *Task) IsDispatchable() bool {
	return t.IsHostDispatchable() || t.ShouldAllocateContainer() || t.IsContainerDispatchable()
}

// IsHostDispatchable returns true if the task should run on a host and can be
// dispatched.
func (t *Task) IsHostDispatchable() bool {
	return t.IsHostTask() && t.Status == evergreen.TaskUndispatched && t.Activated
}

// IsHostTask returns true if it's a task that runs on hosts.
func (t *Task) IsHostTask() bool {
	return (t.ExecutionPlatform == "" || t.ExecutionPlatform == ExecutionPlatformHost) && !t.DisplayOnly
}

// IsContainerTask returns true if it's a task that runs on containers.
func (t *Task) IsContainerTask() bool {
	return t.ExecutionPlatform == ExecutionPlatformContainer
}

// ShouldAllocateContainer indicates whether a task should be allocated a
// container or not.
func (t *Task) ShouldAllocateContainer() bool {
	if t.ContainerAllocated {
		return false
	}
	if t.RemainingContainerAllocationAttempts() == 0 {
		return false
	}

	return t.isContainerScheduled()
}

// RemainingContainerAllocationAttempts returns the number of times this task
// execution is allowed to try allocating a container.
func (t *Task) RemainingContainerAllocationAttempts() int {
	return maxContainerAllocationAttempts - t.ContainerAllocationAttempts
}

// IsContainerDispatchable returns true if the task should run in a container
// and can be dispatched.
func (t *Task) IsContainerDispatchable() bool {
	if !t.ContainerAllocated {
		return false
	}
	return t.isContainerScheduled()
}

// isContainerTaskScheduled returns whether or not the task is in a state
// where it should eventually dispatch to run on a container.
func (t *Task) isContainerScheduled() bool {
	if !t.IsContainerTask() {
		return false
	}
	if t.Status != evergreen.TaskUndispatched {
		return false
	}
	if !t.Activated {
		return false
	}
	if t.Priority <= evergreen.DisabledTaskPriority {
		return false
	}

	for _, dep := range t.DependsOn {
		if dep.Unattainable {
			return false
		}
		if !dep.Finished {
			return false
		}
	}

	return true
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

func (t *Task) IsSystemUnresponsive() bool {
	// this is a legacy case
	if t.Status == evergreen.TaskSystemUnresponse {
		return true
	}

	if t.Details.Type == evergreen.CommandTypeSystem && t.Details.TimedOut && t.Details.Description == evergreen.TaskDescriptionHeartbeat {
		return true
	}

	return false
}

func (t *Task) SetOverrideDependencies(userID string) error {
	t.OverrideDependencies = true
	event.LogTaskDependenciesOverridden(t.Id, t.Execution, userID)
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				OverrideDependenciesKey: true,
			},
		},
	)
}

func (t *Task) AddDependency(d Dependency) error {
	// ensure the dependency doesn't already exist
	for _, existingDependency := range t.DependsOn {
		if existingDependency.TaskId == d.TaskId && existingDependency.Status == d.Status {
			if existingDependency.Unattainable == d.Unattainable {
				return nil // nothing to be done
			}
			return errors.Wrapf(t.MarkUnattainableDependency(existingDependency.TaskId, d.Unattainable),
				"updating matching dependency '%s' for task '%s'", existingDependency.TaskId, t.Id)
		}
	}
	t.DependsOn = append(t.DependsOn, d)
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$push": bson.M{
				DependsOnKey: d,
			},
		},
	)
}

func (t *Task) RemoveDependency(dependencyId string) error {
	found := false
	for i := len(t.DependsOn) - 1; i >= 0; i-- {
		d := t.DependsOn[i]
		if d.TaskId == dependencyId {
			t.DependsOn = append(t.DependsOn[:i], t.DependsOn[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return errors.Errorf("dependency '%s' not found", dependencyId)
	}

	query := bson.M{IdKey: t.Id}
	update := bson.M{
		"$pull": bson.M{
			DependsOnKey: bson.M{
				DependencyTaskIdKey: dependencyId,
			},
		},
	}
	return db.Update(Collection, query, update)
}

// Checks whether the dependencies for the task have all completed successfully.
// If any of the dependencies exist in the map that is passed in, they are
// used to check rather than fetching from the database. All queries
// are cached back into the map for later use.
func (t *Task) DependenciesMet(depCaches map[string]Task) (bool, error) {
	if len(t.DependsOn) == 0 || t.OverrideDependencies || !utility.IsZeroTime(t.DependenciesMetTime) {
		return true, nil
	}

	_, err := t.populateDependencyTaskCache(depCaches)
	if err != nil {
		return false, errors.WithStack(err)
	}

	for _, dependency := range t.DependsOn {
		depTask, exists := depCaches[dependency.TaskId]
		if !exists {
			foundTask, err := FindOneId(dependency.TaskId)
			if err != nil {
				return false, errors.Wrap(err, "finding dependency")
			}
			if foundTask == nil {
				return false, errors.Errorf("dependency '%s' not found", dependency.TaskId)
			}
			depTask = *foundTask
			depCaches[depTask.Id] = depTask
		}
		if !t.SatisfiesDependency(&depTask) {
			return false, nil
		}
	}
	// this is not exact, but depTask.FinishTime is not always set in time to use that
	t.DependenciesMetTime = time.Now()
	err = UpdateOne(
		bson.M{IdKey: t.Id},
		bson.M{
			"$set": bson.M{DependenciesMetTimeKey: t.DependenciesMetTime},
		})
	grip.Error(message.WrapError(err, message.Fields{
		"message": "task.DependenciesMet() failed to update task",
		"task_id": t.Id}))

	return true, nil
}

func (t *Task) populateDependencyTaskCache(depCache map[string]Task) ([]Task, error) {
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
		newDeps, err := FindWithFields(ByIds(depIdsToQueryFor), StatusKey, DependsOnKey, ActivatedKey)
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

// RefreshBlockedDependencies manually rechecks first degree dependencies
// when a task isn't marked as blocked. It returns a slice of this task's dependencies that
// need to recursively update their dependencies
func (t *Task) RefreshBlockedDependencies(depCache map[string]Task) ([]Task, error) {
	if len(t.DependsOn) == 0 || t.OverrideDependencies {
		return nil, nil
	}

	// do this early to avoid caching tasks we won't need.
	for _, dep := range t.DependsOn {
		if dep.Unattainable {
			return nil, nil
		}
	}

	_, err := t.populateDependencyTaskCache(depCache)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	blockedDeps := []Task{}
	for _, dep := range t.DependsOn {
		depTask, ok := depCache[dep.TaskId]
		if !ok {
			return nil, errors.Errorf("task '%s' is not in the cache", dep.TaskId)
		}
		if !t.SatisfiesDependency(&depTask) && (depTask.IsFinished() || depTask.Blocked()) {
			blockedDeps = append(blockedDeps, depTask)
		}
	}

	return blockedDeps, nil
}

func (t *Task) BlockedOnDeactivatedDependency(depCache map[string]Task) ([]string, error) {
	_, err := t.populateDependencyTaskCache(depCache)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	blockingDeps := []string{}
	for _, dep := range t.DependsOn {
		depTask, exists := depCache[dep.TaskId]
		if !exists {
			foundTask, err := FindOneId(dep.TaskId)
			if err != nil {
				return nil, errors.Wrap(err, "finding dependency")
			}
			if foundTask == nil {
				return nil, errors.Errorf("dependency '%s' not found", depTask.Id)
			}
			depTask = *foundTask
			depCache[depTask.Id] = depTask
		}
		if !depTask.IsFinished() && !depTask.Activated {
			blockingDeps = append(blockingDeps, depTask.Id)
		}
	}

	return blockingDeps, nil
}

// AllDependenciesSatisfied inspects the tasks first-order
// dependencies with regards to the cached tasks, and reports if all
// of the dependencies have been satisfied.
//
// If the cached tasks do not include a dependency specified by one of
// the tasks, the function returns an error.
func (t *Task) AllDependenciesSatisfied(cache map[string]Task) (bool, error) {
	if len(t.DependsOn) == 0 {
		return true, nil
	}

	catcher := grip.NewBasicCatcher()
	deps := []Task{}
	for _, dep := range t.DependsOn {
		cachedDep, ok := cache[dep.TaskId]
		if !ok {
			foundTask, err := FindOneId(dep.TaskId)
			if err != nil {
				return false, errors.Wrap(err, "finding dependency")
			}
			if foundTask == nil {
				return false, errors.Errorf("dependency '%s' not found", dep.TaskId)
			}
			cachedDep = *foundTask
			cache[dep.TaskId] = cachedDep
		}
		deps = append(deps, cachedDep)
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
// cache whether or not this task has finished running.
func (t *Task) MarkDependenciesFinished(finished bool) error {
	if t.DisplayOnly {
		// This update can be skipped for display tasks since tasks are not
		// allowed to have dependencies on display tasks.
		return nil
	}

	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	_, err := env.DB().Collection(Collection).UpdateMany(ctx,
		bson.M{
			DependsOnKey: bson.M{"$elemMatch": bson.M{
				DependencyTaskIdKey: t.Id,
			}},
		},
		bson.M{
			"$set": bson.M{bsonutil.GetDottedKeyName(DependsOnKey, "$[elem]", DependencyFinishedKey): finished},
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

// HasFailedTests returns true if the task had any failed tests.
func (t *Task) HasFailedTests() (bool, error) {
	// Check Cedar flags before populating test results to avoid
	// unnecessarily fetching test results.
	if t.HasCedarResults {
		if t.CedarResultsFailed {
			return true, nil
		}

		return false, nil
	}

	if err := t.PopulateTestResults(); err != nil {
		return false, errors.WithStack(err)
	}
	for _, test := range t.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			return true, nil
		}
	}

	return false, nil
}

// FindTaskOnBaseCommit returns the task that is on the base commit.
func (t *Task) FindTaskOnBaseCommit() (*Task, error) {
	return FindOne(db.Query(ByCommit(t.Revision, t.BuildVariant, t.DisplayName, t.Project, evergreen.RepotrackerVersionRequester)))
}

func (t *Task) FindTaskOnPreviousCommit() (*Task, error) {
	return FindOne(db.Query(ByPreviousCommit(t.BuildVariant, t.DisplayName, t.Project, evergreen.RepotrackerVersionRequester, t.RevisionOrderNumber)))
}

// FindIntermediateTasks returns the tasks from most recent to least recent between two tasks.
func (current *Task) FindIntermediateTasks(previous *Task) ([]Task, error) {
	intermediateTasks, err := Find(ByIntermediateRevisions(previous.RevisionOrderNumber, current.RevisionOrderNumber, current.BuildVariant,
		current.DisplayName, current.Project, current.Requester))
	if err != nil {
		return nil, err
	}

	// reverse the slice of tasks
	intermediateTasksReversed := make([]Task, len(intermediateTasks))
	for idx, t := range intermediateTasks {
		intermediateTasksReversed[len(intermediateTasks)-idx-1] = t
	}
	return intermediateTasksReversed, nil
}

// CountSimilarFailingTasks returns a count of all tasks with the same project,
// same display name, and in other buildvariants, that have failed in the same
// revision
func (t *Task) CountSimilarFailingTasks() (int, error) {
	return Count(db.Query(ByDifferentFailedBuildVariants(t.Revision, t.BuildVariant, t.DisplayName,
		t.Project, t.Requester)))
}

// Find the previously completed task for the same project +
// build variant + display name combination as the specified task
func (t *Task) PreviousCompletedTask(project string, statuses []string) (*Task, error) {
	if len(statuses) == 0 {
		statuses = evergreen.TaskCompletedStatuses
	}
	query := db.Query(ByBeforeRevisionWithStatusesAndRequesters(t.RevisionOrderNumber, statuses, t.BuildVariant,
		t.DisplayName, project, evergreen.SystemVersionRequesterTypes)).Sort([]string{"-" + RevisionOrderNumberKey})
	return FindOne(query)
}

func (t *Task) cacheExpectedDuration() error {
	return UpdateOne(
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

// MarkAsContainerDispatched marks that the container task has been dispatched
// to a pod.
func (t *Task) MarkAsContainerDispatched(ctx context.Context, env evergreen.Environment, agentVersion string) error {
	dispatchedAt := time.Now()
	query := isContainerTaskScheduledQuery()
	query[StatusKey] = evergreen.TaskUndispatched
	query[ContainerAllocatedKey] = true
	update := bson.M{
		"$set": bson.M{
			StatusKey:        evergreen.TaskDispatched,
			DispatchTimeKey:  dispatchedAt,
			LastHeartbeatKey: dispatchedAt,
			AgentVersionKey:  agentVersion,
		},
	}
	res, err := env.DB().Collection(Collection).UpdateOne(ctx, query, update)
	if err != nil {
		return errors.Wrap(err, "updating task")
	}
	if res.ModifiedCount == 0 {
		return errors.New("task was not updated")
	}

	t.Status = evergreen.TaskDispatched
	t.DispatchTime = dispatchedAt
	t.LastHeartbeat = dispatchedAt
	t.AgentVersion = agentVersion

	return nil
}

// MarkAsHostDispatched marks that the task has been dispatched onto a
// particular host. If the task is part of a display task, the display task is
// also marked as dispatched to a host. Returns an error if any of the database
// updates fail.
func (t *Task) MarkAsHostDispatched(hostId, distroId, agentRevision string,
	dispatchTime time.Time) error {
	t.DispatchTime = dispatchTime
	t.Status = evergreen.TaskDispatched
	t.HostId = hostId
	t.AgentVersion = agentRevision
	t.LastHeartbeat = dispatchTime
	t.DistroId = distroId
	err := UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				DispatchTimeKey:  dispatchTime,
				StatusKey:        evergreen.TaskDispatched,
				HostIdKey:        hostId,
				LastHeartbeatKey: dispatchTime,
				DistroIdKey:      distroId,
				AgentVersionKey:  agentRevision,
			},
			"$unset": bson.M{
				AbortedKey:   "",
				AbortInfoKey: "",
				DetailsKey:   "",
			},
		},
	)
	if err != nil {
		return errors.Wrapf(err, "marking task '%s' as dispatched", t.Id)
	}

	//when dispatching an execution task, mark its parent as dispatched
	if dt, _ := t.GetDisplayTask(); dt != nil && dt.DispatchTime == utility.ZeroTime {
		return dt.MarkAsHostDispatched("", "", "", dispatchTime)
	}
	return nil
}

// MarkAsHostUndispatched marks that the host task is undispatched. If the task
// is already dispatched to a host, it unsets the host ID field on the task. It
// returns an error if any of the database updates fail.
func (t *Task) MarkAsHostUndispatched() error {
	// then, update the task document
	t.Status = evergreen.TaskUndispatched

	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.TaskUndispatched,
			},
			"$unset": bson.M{
				DispatchTimeKey:  utility.ZeroTime,
				LastHeartbeatKey: utility.ZeroTime,
				DistroIdKey:      "",
				HostIdKey:        "",
				AbortedKey:       "",
				AbortInfoKey:     "",
				DetailsKey:       "",
			},
		},
	)
}

// maxContainerAllocationAttempts is the maximum number of times a container
// task is allowed to try to allocate a container for a single execution.
const maxContainerAllocationAttempts = 5

// MarkAsContainerAllocated marks a container task as allocated a container.
// This will fail if the task is not in a state where it needs a container to be
// allocated to it.
func (t *Task) MarkAsContainerAllocated(ctx context.Context, env evergreen.Environment) error {
	if t.ContainerAllocated {
		return errors.New("cannot allocate a container task if it's currently allocated")
	}
	if t.RemainingContainerAllocationAttempts() == 0 {
		return errors.Errorf("task execution has hit the max allowed allocation attempts (%d)", maxContainerAllocationAttempts)
	}
	q := needsContainerAllocation()
	q[IdKey] = t.Id
	q[ContainerAllocationAttemptsKey] = bson.M{"$lt": maxContainerAllocationAttempts}

	allocatedAt := time.Now()
	update, err := env.DB().Collection(Collection).UpdateOne(ctx, q, bson.M{
		"$set": bson.M{
			ContainerAllocatedKey:     true,
			ContainerAllocatedTimeKey: allocatedAt,
		},
		"$inc": bson.M{
			ContainerAllocationAttemptsKey: 1,
		},
	})
	if err != nil {
		return err
	}
	if update.ModifiedCount == 0 {
		return errors.New("task was not updated")
	}

	t.ContainerAllocated = true
	t.ContainerAllocatedTime = allocatedAt

	return nil
}

func containerDeallocatedUpdate() bson.M {
	return bson.M{
		"$set": bson.M{
			ContainerAllocatedKey: false,
		},
		"$unset": bson.M{
			ContainerAllocatedTimeKey: 1,
		},
	}
}

// MarkAsContainerDeallocated marks a container task that was allocated as no
// longer allocated a container.
func (t *Task) MarkAsContainerDeallocated(ctx context.Context, env evergreen.Environment) error {
	if !t.ContainerAllocated {
		return errors.New("cannot deallocate a container task if it's not currently allocated")
	}

	res, err := env.DB().Collection(Collection).UpdateOne(ctx, bson.M{
		IdKey:                 t.Id,
		ExecutionPlatformKey:  ExecutionPlatformContainer,
		ContainerAllocatedKey: true,
	}, containerDeallocatedUpdate())
	if err != nil {
		return errors.Wrap(err, "updating task")
	}
	if res.ModifiedCount == 0 {
		return errors.New("task was not updated")
	}

	t.ContainerAllocated = false
	t.ContainerAllocatedTime = time.Time{}

	return nil
}

// MarkTasksAsContainerDeallocated marks multiple container tasks as no longer
// allocated containers.
func MarkTasksAsContainerDeallocated(taskIDs []string) error {
	if len(taskIDs) == 0 {
		return nil
	}

	if _, err := UpdateAll(bson.M{
		IdKey:                bson.M{"$in": taskIDs},
		ExecutionPlatformKey: ExecutionPlatformContainer,
	}, containerDeallocatedUpdate()); err != nil {
		return errors.Wrap(err, "updating tasks")
	}

	return nil
}

// MarkGeneratedTasks marks that the task has generated tasks.
func MarkGeneratedTasks(taskID string) error {
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
	err := UpdateOne(query, update)
	if adb.ResultsNotFound(err) {
		return nil
	}
	return errors.Wrap(err, "marking generate.tasks complete")
}

// MarkGeneratedTasksErr marks that the task hit errors generating tasks.
func MarkGeneratedTasksErr(taskID string, errorToSet error) error {
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
	err := UpdateOne(query, update)
	if adb.ResultsNotFound(err) {
		return nil
	}
	return errors.Wrap(err, "setting generate.tasks error")
}

func GenerateNotRun() ([]Task, error) {
	const maxGenerateTimeAgo = 24 * time.Hour
	return FindAll(db.Query(bson.M{
		StatusKey:                evergreen.TaskStarted,                              // task is running
		StartTimeKey:             bson.M{"$gt": time.Now().Add(-maxGenerateTimeAgo)}, // ignore older tasks, just in case
		GenerateTaskKey:          true,                                               // task contains generate.tasks command
		GeneratedTasksKey:        bson.M{"$exists": false},                           // generate.tasks has not yet run
		GeneratedJSONAsStringKey: bson.M{"$exists": true},                            // config has been posted by generate.tasks command
	}))
}

// SetGeneratedJSON sets JSON data to generate tasks from.
func (t *Task) SetGeneratedJSON(json []json.RawMessage) error {
	if len(t.GeneratedJSONAsString) > 0 || len(t.GeneratedJSON) > 0 {
		return nil
	}
	s := []string{}
	for _, j := range json {
		s = append(s, string(j))
	}
	t.GeneratedJSONAsString = s
	return UpdateOne(
		bson.M{
			IdKey:                    t.Id,
			GeneratedJSONAsStringKey: bson.M{"$exists": false},
		},
		bson.M{
			"$set": bson.M{
				GeneratedJSONAsStringKey: s,
			},
		},
	)
}

// SetGeneratedTasksToActivate adds a task to stepback after activation
func (t *Task) SetGeneratedTasksToActivate(buildVariantName, taskName string) error {
	return UpdateOne(
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

// SetTasksScheduledTime takes a list of tasks and a time, and then sets
// the scheduled time in the database for the tasks if it is currently unset
func SetTasksScheduledTime(tasks []Task, scheduledTime time.Time) error {
	ids := []string{}
	for i := range tasks {
		tasks[i].ScheduledTime = scheduledTime
		ids = append(ids, tasks[i].Id)

		// Display tasks are considered scheduled when their first exec task is scheduled
		if tasks[i].IsPartOfDisplay() {
			ids = append(ids, utility.FromStringPtr(tasks[i].DisplayTaskId))
		}
	}
	_, err := UpdateAll(
		bson.M{
			IdKey: bson.M{
				"$in": ids,
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

// Removes host tasks older than the unscheduable threshold (e.g. one week) from
// the scheduler queue.
//
// If you pass an empty string as an argument to this function, this operation
// will select tasks from all distros.
func UnscheduleStaleUnderwaterHostTasks(distroID string) (int, error) {
	query := schedulableHostTasksQuery()

	if err := addApplicableDistroFilter(distroID, DistroIdKey, query); err != nil {
		return 0, errors.WithStack(err)
	}

	query[ActivatedTimeKey] = bson.M{"$lte": time.Now().Add(-UnschedulableThreshold)}

	update := bson.M{
		"$set": bson.M{
			PriorityKey:  evergreen.DisabledTaskPriority,
			ActivatedKey: false,
		},
	}

	// Force the query to use 'distro_1_status_1_activated_1_priority_1'
	// instead of defaulting to 'status_1_depends_on.status_1_depends_on.unattainable_1'.
	info, err := UpdateAllWithHint(query, update, ActivatedTasksByDistroIndex)
	if err != nil {
		return 0, errors.Wrap(err, "unscheduling stale underwater tasks")
	}

	return info.Updated, nil
}

// DisableStaleContainerTasks disables all container tasks that have been
// scheduled to run for a long time without actually dispatching the task.
func DisableStaleContainerTasks(caller string) error {
	query := isContainerTaskScheduledQuery()
	query[ActivatedTimeKey] = bson.M{"$lte": time.Now().Add(-UnschedulableThreshold)}

	tasks, err := FindAll(db.Query(query))
	if err != nil {
		return errors.Wrap(err, "finding tasks that need to be disabled")
	}

	grip.Info(message.Fields{
		"message":   "disabling container tasks that are still scheduled to run but are stale",
		"num_tasks": len(tasks),
		"caller":    caller,
	})

	if err := DisableTasks(tasks, caller); err != nil {
		return errors.Wrap(err, "disabled stale container tasks")
	}

	return nil
}

// DeactivateStepbackTasksForProjects deactivates and aborts any scheduled/running tasks
// for this project that were activated by stepback.
func DeactivateStepbackTasksForProject(projectId, caller string) error {
	tasks, err := FindActivatedStepbackTasks(projectId)
	if err != nil {
		return errors.Wrap(err, "finding activated stepback tasks")
	}

	if err = DeactivateTasks(tasks, true, caller); err != nil {
		return errors.Wrap(err, "deactivating active stepback tasks")
	}

	grip.InfoWhen(len(tasks) > 0, message.Fields{
		"message":    "deactivated active stepback tasks",
		"project_id": projectId,
		"user":       caller,
		"num_tasks":  len(tasks),
	})

	abortTaskIds := []string{}
	for _, t := range tasks {
		if t.IsAbortable() {
			abortTaskIds = append(abortTaskIds, t.Id)
			event.LogTaskAbortRequest(t.Id, t.Execution, caller)
		}
	}
	if err = SetManyAborted(abortTaskIds, AbortInfo{User: caller}); err != nil {
		return errors.Wrap(err, "aborting in progress tasks")
	}

	return nil
}

// MarkFailed changes the state of the task to failed.
func (t *Task) MarkFailed() error {
	t.Status = evergreen.TaskFailed
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.TaskFailed,
			},
		},
	)
}

func (t *Task) MarkSystemFailed(description string) error {
	t.Status = evergreen.TaskFailed
	t.FinishTime = time.Now()

	t.Details = apimodels.TaskEndDetail{
		Status:      evergreen.TaskFailed,
		Type:        evergreen.CommandTypeSystem,
		Description: description,
	}

	event.LogTaskFinished(t.Id, t.Execution, t.HostId, evergreen.TaskSystemFailed)
	grip.Info(message.Fields{
		"message":     "marking task system failed",
		"task_id":     t.Id,
		"execution":   t.Execution,
		"status":      t.Status,
		"host_id":     t.HostId,
		"description": description,
	})

	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:     evergreen.TaskFailed,
				FinishTimeKey: t.FinishTime,
				DetailsKey:    t.Details,
			},
		},
	)
}

func SetManyAborted(taskIds []string, reason AbortInfo) error {
	return UpdateOne(
		ByIds(taskIds),
		bson.M{
			"$set": bson.M{
				AbortedKey:   true,
				AbortInfoKey: reason,
			},
		},
	)
}

// SetAborted sets the abort field of task to aborted
func (t *Task) SetAborted(reason AbortInfo) error {
	t.Aborted = true
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				AbortedKey:   true,
				AbortInfoKey: reason,
			},
		},
	)
}

// SetHasCedarResults sets the HasCedarResults field of the task to
// hasCedarResults and, if failedResults is true, sets CedarResultsFailed to
// true. If the task is part of a display task, the display tasks's fields are
// also set. An error is returned if hasCedarResults is false and failedResults
// is true as this is an invalid state. Note that if failedResults is false,
// CedarResultsFailed is not set. This is because in cases where separate calls
// to attach test results are made, only one call needs to have a test failure
// for the CedarResultsFailed to be set to true.
func (t *Task) SetHasCedarResults(hasCedarResults, failedResults bool) error {
	if !hasCedarResults && failedResults {
		return errors.New("cannot set Cedar results as failed when task does not have Cedar results")
	}

	t.HasCedarResults = hasCedarResults
	set := bson.M{
		HasCedarResultsKey: hasCedarResults,
	}
	if failedResults {
		t.CedarResultsFailed = true
		set[CedarResultsFailedKey] = true
	}

	if err := UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": set,
		},
	); err != nil {
		return err
	}

	if !t.DisplayOnly && t.IsPartOfDisplay() {
		displayTask, err := t.GetDisplayTask()
		if err != nil {
			return errors.Wrap(err, "getting display task")
		}
		return displayTask.SetHasCedarResults(hasCedarResults, failedResults)
	}

	return nil
}

func (t *Task) SetHasLegacyResults(hasLegacyResults bool) error {
	t.HasLegacyResults = utility.ToBoolPtr(hasLegacyResults)
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				HasLegacyResultsKey: t.HasLegacyResults,
			},
		},
	)
}

// ActivateTask will set the ActivatedBy field to the caller and set the active state to be true.
// Also activates dependencies of the task.
func (t *Task) ActivateTask(caller string) error {
	t.ActivatedBy = caller
	t.Activated = true
	t.ActivatedTime = time.Now()

	return ActivateTasks([]Task{*t}, t.ActivatedTime, true, caller)
}

// ActivateTasks sets all given tasks to active, logs them as activated, and proceeds to activate any dependencies that were deactivated.
func ActivateTasks(tasks []Task, activationTime time.Time, updateDependencies bool, caller string) error {
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}

	_, err := UpdateAll(
		bson.M{
			IdKey: bson.M{"$in": taskIDs},
		},
		bson.M{
			"$set": bson.M{
				ActivatedKey:     true,
				ActivatedByKey:   caller,
				ActivatedTimeKey: activationTime,
			},
		})
	if err != nil {
		return errors.Wrap(err, "activating tasks")
	}
	for _, t := range tasks {
		event.LogTaskActivated(t.Id, t.Execution, caller)
	}

	if updateDependencies {
		return ActivateDeactivatedDependencies(taskIDs, caller)
	}
	return nil
}

// ActivateTasksByIdsWithDependencies activates the given tasks and their dependencies.
func ActivateTasksByIdsWithDependencies(ids []string, caller string) error {
	q := db.Query(bson.M{
		IdKey:     bson.M{"$in": ids},
		StatusKey: evergreen.TaskUndispatched,
	})

	tasks, err := FindAll(q.WithFields(IdKey, DependsOnKey, ExecutionKey))
	if err != nil {
		return errors.Wrap(err, "getting tasks for activation")
	}
	dependOn, err := GetRecursiveDependenciesUp(tasks, nil)
	if err != nil {
		return errors.Wrap(err, "getting recursive dependencies")
	}

	if err = ActivateTasks(append(tasks, dependOn...), time.Now(), true, caller); err != nil {
		return errors.Wrap(err, "updating tasks for activation")
	}
	return nil
}

// ActivateDeactivatedDependencies activates tasks that depend on these tasks which were deactivated because a task
// they depended on was deactivated. Only activate when all their dependencies are activated or are being activated
func ActivateDeactivatedDependencies(tasks []string, caller string) error {
	taskMap := make(map[string]bool)
	for _, t := range tasks {
		taskMap[t] = true
	}

	tasksDependingOnTheseTasks, err := getRecursiveDependenciesDown(tasks, nil)
	if err != nil {
		return errors.Wrap(err, "getting recursive dependencies down")
	}

	// do a topological sort so we've dealt with
	// all a task's dependencies by the time we get up to it
	sortedDependencies, err := topologicalSort(tasksDependingOnTheseTasks)
	if err != nil {
		return errors.WithStack(err)
	}

	// get dependencies we don't have yet and add them to a map
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
		missingTasks, err = FindAll(db.Query(bson.M{IdKey: bson.M{"$in": tasksToGet}}).WithFields(ActivatedKey))
		if err != nil {
			return errors.Wrap(err, "getting missing tasks")
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
		return nil
	}

	taskIDsToActivate := make([]string, 0, len(tasksToActivate))
	for _, t := range tasksToActivate {
		taskIDsToActivate = append(taskIDsToActivate, t.Id)
	}
	_, err = UpdateAll(
		bson.M{IdKey: bson.M{"$in": taskIDsToActivate}},
		bson.M{"$set": bson.M{
			ActivatedKey:                true,
			DeactivatedForDependencyKey: false,
			ActivatedByKey:              caller,
			ActivatedTimeKey:            time.Now(),
		}},
	)
	if err != nil {
		return errors.Wrap(err, "updating activation for dependencies")
	}

	for _, t := range tasksToActivate {
		event.LogTaskActivated(t.Id, t.Execution, caller)
	}

	return nil
}

func topologicalSort(tasks []Task) ([]Task, error) {
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
			if toNode, ok := taskNodeMap[dep.TaskId]; ok {
				edge := simple.Edge{
					F: simple.Node(toNode.ID()),
					T: simple.Node(taskNodeMap[task.Id].ID()),
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

// DeactivateTask will set the ActivatedBy field to the caller and set the active state to be false and deschedule the task
func (t *Task) DeactivateTask(caller string) error {
	t.ActivatedBy = caller
	t.Activated = false
	t.ScheduledTime = utility.ZeroTime

	return DeactivateTasks([]Task{*t}, true, caller)
}

func DeactivateTasks(tasks []Task, updateDependencies bool, caller string) error {
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}

	_, err := UpdateAll(
		bson.M{
			IdKey: bson.M{"$in": taskIDs},
		},
		bson.M{
			"$set": bson.M{
				ActivatedKey:     false,
				ActivatedByKey:   caller,
				ScheduledTimeKey: utility.ZeroTime,
			},
		},
	)
	if err != nil {
		return errors.Wrap(err, "deactivating tasks")
	}
	for _, t := range tasks {
		event.LogTaskDeactivated(t.Id, t.Execution, caller)
	}

	if updateDependencies {
		return DeactivateDependencies(taskIDs, caller)
	}
	return nil
}

func DeactivateDependencies(tasks []string, caller string) error {
	tasksDependingOnTheseTasks, err := getRecursiveDependenciesDown(tasks, nil)
	if err != nil {
		return errors.Wrap(err, "getting recursive dependencies down")
	}

	tasksToUpdate := make([]Task, 0, len(tasksDependingOnTheseTasks))
	taskIDsToUpdate := make([]string, 0, len(tasksDependingOnTheseTasks))
	for _, t := range tasksDependingOnTheseTasks {
		if t.Activated {
			tasksToUpdate = append(tasksToUpdate, t)
			taskIDsToUpdate = append(taskIDsToUpdate, t.Id)
		}
	}

	if len(tasksToUpdate) == 0 {
		return nil
	}

	_, err = UpdateAll(
		bson.M{
			IdKey: bson.M{"$in": taskIDsToUpdate},
		},
		bson.M{"$set": bson.M{
			ActivatedKey:                false,
			DeactivatedForDependencyKey: true,
			ScheduledTimeKey:            utility.ZeroTime,
		}},
	)
	if err != nil {
		return errors.Wrap(err, "deactivating dependencies")
	}
	for _, t := range tasksToUpdate {
		event.LogTaskDeactivated(t.Id, t.Execution, caller)
	}

	return nil
}

// MarkEnd handles the Task updates associated with ending a task. If the task's start time is zero
// at this time, it will set it to the finish time minus the timeout time.
func (t *Task) MarkEnd(finishTime time.Time, detail *apimodels.TaskEndDetail) error {
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

	grip.Debug(message.Fields{
		"message":   "marking task finished",
		"task_id":   t.Id,
		"execution": t.Execution,
		"project":   t.Project,
		"details":   t.Details,
	})
	if detail.Status == "" {
		grip.Debug(message.Fields{
			"message":   "detail status was empty, setting to failed",
			"task_id":   t.Id,
			"execution": t.Execution,
			"project":   t.Project,
			"details":   t.Details,
		})
		detail.Status = evergreen.TaskFailed
	}
	// record that the task has finished, in memory and in the db
	t.Status = detail.Status
	t.FinishTime = finishTime
	t.Details = *detail
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				FinishTimeKey:       finishTime,
				StatusKey:           detail.Status,
				TimeTakenKey:        t.TimeTaken,
				DetailsKey:          detail,
				StartTimeKey:        t.StartTime,
				LogsKey:             detail.Logs,
				HasLegacyResultsKey: t.HasLegacyResults,
			},
		})

}

// GetDisplayStatus should reflect the statuses assigned during the addDisplayStatus aggregation step
func (t *Task) GetDisplayStatus() string {
	if t.DisplayStatus != "" {
		return t.DisplayStatus
	}
	if t.Aborted && t.IsFinished() {
		return evergreen.TaskAborted
	}
	if t.Status == evergreen.TaskUndispatched {
		if !t.Activated {
			return evergreen.TaskUnscheduled
		}
		if t.Blocked() && !t.OverrideDependencies {
			return evergreen.TaskStatusBlocked
		}
		return evergreen.TaskWillRun
	}
	if !t.IsFinished() {
		return t.Status
	}
	if t.Status == evergreen.TaskSucceeded {
		return evergreen.TaskSucceeded
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
	if t.Details.Type == evergreen.CommandTypeSetup {
		return evergreen.TaskSetupFailed
	}
	if t.Details.TimedOut {
		return evergreen.TaskTimedOut
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
	return 1000
}

// Reset sets the task state to a state in which it is scheduled to re-run.
func (t *Task) Reset() error {
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		resetTaskUpdate(t),
	)
}

// ResetTasks performs the same DB updates as (*Task).Reset, but resets many
// tasks instead of a single one.
func ResetTasks(tasks []Task) error {
	if len(tasks) == 0 {
		return nil
	}
	var taskIDs []string
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}

	if _, err := UpdateAll(
		bson.M{IdKey: bson.M{"$in": taskIDs}},
		resetTaskUpdate(nil),
	); err != nil {
		return err
	}

	return nil
}

func resetTaskUpdate(t *Task) bson.M {
	newSecret := utility.RandomString()
	now := time.Now()
	if t != nil {
		t.Activated = true
		t.ActivatedTime = now
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
		t.HasCedarResults = false
		t.ResetWhenFinished = false
		t.ResetFailedWhenFinished = false
		t.AgentVersion = ""
		t.HostCreateDetails = []HostCreateDetail{}
		t.OverrideDependencies = false
		t.ContainerAllocationAttempts = 0
	}
	update := bson.M{
		"$set": bson.M{
			ActivatedKey:                   true,
			ActivatedTimeKey:               now,
			SecretKey:                      newSecret,
			StatusKey:                      evergreen.TaskUndispatched,
			DispatchTimeKey:                utility.ZeroTime,
			StartTimeKey:                   utility.ZeroTime,
			ScheduledTimeKey:               utility.ZeroTime,
			FinishTimeKey:                  utility.ZeroTime,
			DependenciesMetTimeKey:         utility.ZeroTime,
			TimeTakenKey:                   0,
			LastHeartbeatKey:               utility.ZeroTime,
			ContainerAllocationAttemptsKey: 0,
		},
		"$unset": bson.M{
			DetailsKey:                 "",
			HasCedarResultsKey:         "",
			CedarResultsFailedKey:      "",
			ResetWhenFinishedKey:       "",
			ResetFailedWhenFinishedKey: "",
			AgentVersionKey:            "",
			HostIdKey:                  "",
			HostCreateDetailsKey:       "",
			OverrideDependenciesKey:    "",
		},
	}
	return update
}

// UpdateHeartbeat updates the heartbeat to be the current time
func (t *Task) UpdateHeartbeat() error {
	t.LastHeartbeat = time.Now()
	return UpdateOne(
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

// SetDisabledPriority sets the priority of a task so it will never run. If it's
// a display task, it will disable the display task and all of its child
// execution tasks. If it's an execution task, its parent display task will not
// be updated. It also deactivates the task and any tasks that depend on it.
func (t *Task) SetDisabledPriority(caller string) error {
	t.Priority = evergreen.DisabledTaskPriority

	ids := append([]string{t.Id}, t.ExecutionTasks...)
	_, err := UpdateAll(
		bson.M{IdKey: bson.M{"$in": ids}},
		bson.M{"$set": bson.M{PriorityKey: evergreen.DisabledTaskPriority}},
	)
	if err != nil {
		return errors.Wrap(err, "updating priority")
	}

	query := db.Query(bson.M{
		IdKey: bson.M{"$in": ids},
	}).WithFields(ExecutionKey)
	tasks, err := FindAll(query)
	if err != nil {
		return errors.Wrap(err, "finding matching tasks")
	}
	for _, task := range tasks {
		event.LogTaskPriority(task.Id, task.Execution, caller, evergreen.DisabledTaskPriority)
	}

	return t.DeactivateTask(caller)
}

// DisableTasks is the same as (*Task).SetDisabledPriority but for many tasks.
func DisableTasks(tasks []Task, caller string) error {
	if len(tasks) == 0 {
		return nil
	}

	tasksPresent := map[string]struct{}{}
	var taskIDs []string
	var execTaskIDs []string
	for _, t := range tasks {
		tasksPresent[t.Id] = struct{}{}
		taskIDs = append(taskIDs, t.Id)
		execTaskIDs = append(execTaskIDs, t.ExecutionTasks...)
	}

	_, err := UpdateAll(
		ByIds(append(taskIDs, execTaskIDs...)),
		bson.M{"$set": bson.M{PriorityKey: evergreen.DisabledTaskPriority}},
	)
	if err != nil {
		return errors.Wrap(err, "updating task priorities")
	}

	execTasks, err := findMissingTasks(execTaskIDs, tasksPresent)
	if err != nil {
		return errors.Wrap(err, "finding additional execution tasks")
	}
	tasks = append(tasks, execTasks...)

	for _, t := range tasks {
		t.Priority = evergreen.DisabledTaskPriority
		event.LogTaskPriority(t.Id, t.Execution, caller, evergreen.DisabledTaskPriority)
	}

	if err := DeactivateTasks(tasks, true, caller); err != nil {
		return errors.Wrap(err, "deactivating dependencies")
	}

	return nil
}

// findMissingTasks finds all tasks whose IDs are missing from tasksPresent.
func findMissingTasks(taskIDs []string, tasksPresent map[string]struct{}) ([]Task, error) {
	var missingTaskIDs []string
	for _, id := range taskIDs {
		if _, ok := tasksPresent[id]; ok {
			continue
		}
		missingTaskIDs = append(missingTaskIDs, id)
	}
	if len(missingTaskIDs) == 0 {
		return nil, nil
	}

	missingTasks, err := FindAll(db.Query(ByIds(missingTaskIDs)))
	if err != nil {
		return nil, err
	}

	return missingTasks, nil
}

// GetRecursiveDependenciesUp returns all tasks recursively depended upon
// that are not in the original task slice (this includes earlier tasks in task groups, if applicable).
// depCache should originally be nil. We assume there are no dependency cycles.
func GetRecursiveDependenciesUp(tasks []Task, depCache map[string]Task) ([]Task, error) {
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
			tasksInGroup, err := FindTaskGroupFromBuild(t.BuildId, t.TaskGroup)
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

	deps, err := FindWithFields(ByIds(tasksToFind), IdKey, DependsOnKey, ExecutionKey, BuildIdKey, StatusKey, TaskGroupKey)
	if err != nil {
		return nil, errors.Wrap(err, "getting dependencies")
	}

	recursiveDeps, err := GetRecursiveDependenciesUp(deps, depCache)
	if err != nil {
		return nil, errors.Wrap(err, "getting recursive dependencies")
	}

	return append(deps, recursiveDeps...), nil
}

// getRecursiveDependenciesDown returns a slice containing all tasks recursively depending on tasks.
// taskMap should originally be nil.
// We assume there are no dependency cycles.
func getRecursiveDependenciesDown(tasks []string, taskMap map[string]bool) ([]Task, error) {
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
	dependOnUsTasks, err := FindAll(query)
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
	recurseTasks, err := getRecursiveDependenciesDown(newDepIDs, taskMap)
	if err != nil {
		return nil, errors.Wrap(err, "getting recursive dependencies")
	}

	return append(newDeps, recurseTasks...), nil
}

// MarkStart updates the task's start time and sets the status to started
func (t *Task) MarkStart(startTime time.Time) error {
	// record the start time in the in-memory task
	t.StartTime = startTime
	t.Status = evergreen.TaskStarted
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:        evergreen.TaskStarted,
				LastHeartbeatKey: startTime,
				StartTimeKey:     startTime,
			},
		},
	)
}

// SetResults sets the results of the task in LocalTestResults
func (t *Task) SetResults(results []TestResult) error {
	docs := make([]testresult.TestResult, len(results))

	for idx, result := range results {
		docs[idx] = result.convertToNewStyleTestResult(t)
	}

	grip.Debug(message.Fields{
		"message":        "writing test results",
		"task":           t.Id,
		"project":        t.Project,
		"requester":      t.Requester,
		"version":        t.Version,
		"display_name":   t.DisplayName,
		"results_length": len(results),
	})

	return errors.Wrap(testresult.InsertMany(docs), "inserting test results")
}

func (t TestResult) convertToNewStyleTestResult(task *Task) testresult.TestResult {
	ExecutionDisplayName := ""
	if displayTask, _ := task.GetDisplayTask(); displayTask != nil {
		ExecutionDisplayName = displayTask.DisplayName
	}
	return testresult.TestResult{
		// copy fields from local test result.
		Status:          t.Status,
		TestFile:        t.TestFile,
		DisplayTestName: t.DisplayTestName,
		GroupID:         t.GroupID,
		URL:             t.URL,
		URLRaw:          t.URLRaw,
		LogID:           t.LogId,
		LineNum:         t.LineNum,
		ExitCode:        t.ExitCode,
		StartTime:       t.StartTime,
		EndTime:         t.EndTime,

		// copy field values from enclosing tasks.
		TaskID:               task.Id,
		Execution:            task.Execution,
		Project:              task.Project,
		BuildVariant:         task.BuildVariant,
		DistroId:             task.DistroId,
		Container:            task.Container,
		Requester:            task.Requester,
		DisplayName:          task.DisplayName,
		TaskCreateTime:       task.CreateTime,
		ExecutionDisplayName: ExecutionDisplayName,

		TestStartTime: utility.FromPythonTime(t.StartTime).In(time.UTC),
		TestEndTime:   utility.FromPythonTime(t.EndTime).In(time.UTC),
	}
}

func ConvertToOld(in *testresult.TestResult) TestResult {
	return TestResult{
		Status:          in.Status,
		TestFile:        in.TestFile,
		DisplayTestName: in.DisplayTestName,
		GroupID:         in.GroupID,
		URL:             in.URL,
		URLRaw:          in.URLRaw,
		LogId:           in.LogID,
		LineNum:         in.LineNum,
		ExitCode:        in.ExitCode,
		StartTime:       in.StartTime,
		EndTime:         in.EndTime,
		LogRaw:          in.LogRaw,
		TaskID:          in.TaskID,
		Execution:       in.Execution,
	}
}

// MarkUnscheduled marks the task as undispatched and updates it in the database
func (t *Task) MarkUnscheduled() error {
	t.Status = evergreen.TaskUndispatched
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.TaskUndispatched,
			},
		},
	)

}

// MarkUnattainableDependency updates the unattainable field for the dependency in the task's dependency list,
// and logs if the task is newly blocked.
func (t *Task) MarkUnattainableDependency(dependencyId string, unattainable bool) error {
	wasBlocked := t.Blocked()
	// check all dependencies in case of erroneous duplicate
	for i := range t.DependsOn {
		if t.DependsOn[i].TaskId == dependencyId {
			t.DependsOn[i].Unattainable = unattainable
		}
	}

	if err := updateAllMatchingDependenciesForTask(t.Id, dependencyId, unattainable); err != nil {
		return err
	}

	// only want to log the task as blocked if it wasn't already blocked
	if !wasBlocked && unattainable {
		event.LogTaskBlocked(t.Id, t.Execution)
	}
	return nil
}

// AbortBuild sets the abort flag on all tasks associated with the build which are in an abortable
// state
func AbortBuild(buildId string, reason AbortInfo) error {
	q := bson.M{
		BuildIdKey: buildId,
		StatusKey:  bson.M{"$in": evergreen.TaskAbortableStatuses},
	}
	if reason.TaskID != "" {
		q[IdKey] = bson.M{"$ne": reason.TaskID}
	}
	ids, err := findAllTaskIDs(db.Query(q))
	if err != nil {
		return errors.Wrapf(err, "finding tasks to abort from build '%s'", buildId)
	}
	if len(ids) == 0 {
		grip.Info(message.Fields{
			"message": "no tasks aborted for build",
			"buildId": buildId,
		})
		return nil
	}

	_, err = UpdateAll(
		bson.M{IdKey: bson.M{"$in": ids}},
		bson.M{"$set": bson.M{
			AbortedKey:   true,
			AbortInfoKey: reason,
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "setting aborted statuses for tasks in build '%s'", buildId)
	}

	event.LogManyTaskAbortRequests(ids, reason.User)

	return nil
}

// AbortVersion sets the abort flag on all tasks associated with the version which are in an
// abortable state
func AbortVersion(versionId string, reason AbortInfo) error {
	q := bson.M{
		VersionKey: versionId,
		StatusKey:  bson.M{"$in": evergreen.TaskAbortableStatuses},
	}
	if reason.TaskID != "" {
		q[IdKey] = bson.M{"$ne": reason.TaskID}
		// if the aborting task is part of a display task, we also don't want to mark it as aborted
		q[ExecutionTasksKey] = bson.M{"$ne": reason.TaskID}
	}
	ids, err := findAllTaskIDs(db.Query(q))
	if err != nil {
		return errors.Wrap(err, "finding updated tasks")
	}

	if len(ids) == 0 {
		grip.Info(message.Fields{
			"message": "no tasks aborted for version",
			"buildId": versionId,
		})
		return nil
	}

	_, err = UpdateAll(
		bson.M{IdKey: bson.M{"$in": ids}},
		bson.M{"$set": bson.M{
			AbortedKey:   true,
			AbortInfoKey: reason,
		}},
	)
	if err != nil {
		return errors.Wrap(err, "setting aborted statuses")
	}

	event.LogManyTaskAbortRequests(ids, reason.User)

	return nil
}

//String represents the stringified version of a task
func (t *Task) String() (taskStruct string) {
	taskStruct += fmt.Sprintf("Id: %v\n", t.Id)
	taskStruct += fmt.Sprintf("Status: %v\n", t.Status)
	taskStruct += fmt.Sprintf("Host: %v\n", t.HostId)
	taskStruct += fmt.Sprintf("ScheduledTime: %v\n", t.ScheduledTime)
	taskStruct += fmt.Sprintf("ContainerAllocatedTime: %v\n", t.ContainerAllocatedTime)
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
func (t *Task) Insert() error {
	return db.Insert(Collection, t)
}

// Archive modifies the current execution of the task so that it is no longer
// considered the latest execution. This task execution is inserted
// into the old_tasks collection. If this is a display task, its execution tasks
// are also archived.
func (t *Task) Archive() error {
	if t.DisplayOnly && len(t.ExecutionTasks) > 0 {
		execTasks, err := FindAll(db.Query(ByIds(t.ExecutionTasks)))
		if err != nil {
			return errors.Wrap(err, "retrieving execution tasks")
		}
		if err = ArchiveMany(execTasks); err != nil {
			return errors.Wrap(err, "archiving execution tasks")
		}
	}

	archiveTask := t.makeArchivedTask()
	err := db.Insert(OldCollection, archiveTask)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"archive_task_id": archiveTask.Id,
			"old_task_id":     archiveTask.OldTaskId,
			"execution":       t.Execution,
			"display_only":    t.DisplayOnly,
		}))
		return errors.Wrap(err, "inserting archived task into old tasks")
	}
	err = UpdateOne(
		bson.M{IdKey: t.Id},
		bson.M{
			"$unset": bson.M{
				AbortedKey:              "",
				AbortInfoKey:            "",
				OverrideDependenciesKey: "",
			},
			"$inc": bson.M{ExecutionKey: 1},
		})
	if err != nil {
		return errors.Wrap(err, "updating task")
	}
	t.Aborted = false

	if t.IsHostTask() {
		// Host event logs involving running a host task don't include the
		// execution number but need it to distinguish which task execution it
		// ran once the task is no longer the latest execution. This
		// retroactively sets the task execution for event logs involving the
		// host running this task so that it correctly identifies this archived
		// execution.
		err = event.UpdateHostTaskExecutions(t.HostId, t.Id, t.Execution)
		if err != nil {
			return errors.Wrap(err, "updating host event logs")
		}
	}
	return nil
}

func ArchiveMany(tasks []Task) error {
	if len(tasks) == 0 {
		return nil
	}
	// add execution tasks of display tasks passed in, if they are not there
	execTaskMap := map[string]bool{}
	for _, t := range tasks {
		execTaskMap[t.Id] = true
	}
	additionalTasks := []string{}
	for _, t := range tasks {
		// for any display tasks here, make sure that we archive all their execution tasks
		for _, et := range t.ExecutionTasks {
			if !execTaskMap[et] {
				additionalTasks = append(additionalTasks, t.ExecutionTasks...)
				continue
			}
		}
	}
	if len(additionalTasks) > 0 {
		toAdd, err := FindAll(db.Query(ByIds((additionalTasks))))
		if err != nil {
			return errors.Wrap(err, "finding execution tasks")
		}
		tasks = append(tasks, toAdd...)
	}

	archived := []interface{}{}
	taskIds := []string{}
	for _, t := range tasks {
		archived = append(archived, *t.makeArchivedTask())
		taskIds = append(taskIds, t.Id)
	}

	mongoClient := evergreen.GetEnvironment().Client()
	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()
	session, err := mongoClient.StartSession()
	if err != nil {
		return errors.Wrap(err, "starting DB session")
	}
	defer session.EndSession(ctx)

	txFunc := func(sessCtx mongo.SessionContext) (interface{}, error) {
		oldTaskColl := evergreen.GetEnvironment().DB().Collection(OldCollection)
		_, err = oldTaskColl.InsertMany(ctx, archived)
		if err != nil {
			return nil, err
		}

		taskColl := evergreen.GetEnvironment().DB().Collection(Collection)
		_, err = taskColl.UpdateMany(ctx, bson.M{
			IdKey: bson.M{
				"$in": taskIds,
			},
		},
			bson.M{
				"$unset": bson.M{
					AbortedKey:   "",
					AbortInfoKey: "",
				},
				"$inc": bson.M{
					ExecutionKey: 1,
				},
			})
		if err != nil {
			return nil, err
		}
		return nil, err
	}

	_, err = session.WithTransaction(ctx, txFunc)
	if err != nil {
		return errors.Wrap(err, "archiving tasks")
	}

	eventLogErrs := grip.NewBasicCatcher()
	for _, t := range tasks {
		if t.IsHostTask() {
			// Host event logs involving running a host task don't include the
			// execution number but need it to distinguish which task execution
			// it ran once the task is no longer the latest execution. This
			// retroactively sets the task execution for event logs involving
			// the host running this task so that it correctly identifies this
			// archived execution.
			eventLogErrs.Wrapf(event.UpdateHostTaskExecutions(t.HostId, t.Id, t.Execution), "updating execution %d of task '%s' for host '%s'", t.Execution, t.Id, t.HostId)
		}
	}

	return eventLogErrs.Resolve()
}

func (t *Task) makeArchivedTask() *Task {
	archiveTask := *t
	archiveTask.Id = MakeOldID(t.Id, t.Execution)
	archiveTask.OldTaskId = t.Id
	archiveTask.Archived = true

	return &archiveTask
}

// Aggregation

// PopulateTestResults returns the task with both old (embedded in the tasks
// collection) and new (from the testresults collection) OR Cedar test results
// merged into the Task's LocalTestResults field.
func (t *Task) PopulateTestResults() error {
	if t.testResultsPopulated {
		return nil
	}
	if !evergreen.IsFinishedTaskStatus(t.Status) && t.Status != evergreen.TaskStarted {
		// Task won't have test results.
		return nil
	}

	if t.DisplayOnly && !t.hasCedarResults() {
		return t.populateTestResultsForDisplayTask()
	}
	if !t.hasCedarResults() {
		return t.populateNewTestResults()
	}

	results, err := t.getCedarTestResults()
	if err != nil {
		return errors.Wrap(err, "getting test results from cedar")
	}
	t.LocalTestResults = append(t.LocalTestResults, results...)
	t.testResultsPopulated = true

	return nil
}

// populateNewTestResults returns the task with both old (embedded in the tasks
// collection) and new (from the testresults collection) test results merged in
// the Task's LocalTestResults field.
func (t *Task) populateNewTestResults() error {
	id := t.Id
	if t.Archived {
		id = t.OldTaskId
	}

	newTestResults, err := testresult.FindByTaskIDAndExecution(id, t.Execution)
	if err != nil {
		return errors.Wrap(err, "finding test results")
	}
	for i := range newTestResults {
		t.LocalTestResults = append(t.LocalTestResults, ConvertToOld(&newTestResults[i]))
	}
	t.testResultsPopulated = true

	// Store whether or not results exist so we know if we should look them
	// up in the future.
	if t.HasLegacyResults == nil && !t.Archived {
		return t.SetHasLegacyResults(len(newTestResults) > 0)
	}
	return nil
}

// populateTestResultsForDisplayTask returns the test results for the execution
// tasks of a display task.
func (t *Task) populateTestResultsForDisplayTask() error {
	if !t.DisplayOnly {
		return errors.Errorf("'%s' is not a display task", t.Id)
	}

	out, err := MergeTestResultsBulk([]Task{*t}, nil)
	if err != nil {
		return errors.Wrap(err, "merging test results for display task")
	}
	t.LocalTestResults = out[0].LocalTestResults
	t.testResultsPopulated = true

	return nil
}

// SetResetWhenFinished requests that a display task or single-host task group
// reset itself when finished. Will mark itself as system failed.
func (t *Task) SetResetWhenFinished() error {
	if t.ResetWhenFinished {
		return nil
	}
	t.ResetWhenFinished = true
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				ResetWhenFinishedKey: true,
			},
		},
	)
}

// SetResetFailedWhenFinished requests that a display task
// only restarts failed tasks.
func (t *Task) SetResetFailedWhenFinished() error {
	if t.ResetFailedWhenFinished {
		return nil
	}
	t.ResetFailedWhenFinished = true
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				ResetFailedWhenFinishedKey: true,
			},
		},
	)
}

// SetAbortedTasksResetWhenFinished sets all matching aborted tasks as ResetWhenFinished.
func SetAbortedTasksResetWhenFinished(taskIds []string) error {
	_, err := UpdateAll(
		bySubsetAborted(taskIds),
		bson.M{
			"$set": bson.M{
				ResetWhenFinishedKey: true,
			},
		},
	)
	return err
}

// MergeTestResultsBulk takes a slice of task structs and returns the slice with
// test results populated. Note that the order may change. The second parameter
// can be used to use a specific test result filtering query, otherwise all test
// results for the passed in tasks will be merged. Display tasks will have
// the execution task results merged.
//
// Keeping this function public for backwards compatibility (legacy test
// results uses this for test history).
func MergeTestResultsBulk(tasks []Task, query *db.Q) ([]Task, error) {
	out := []Task{}
	if query == nil {
		taskIds := []string{}
		for _, t := range tasks {
			taskIds = append(taskIds, t.Id)
			taskIds = append(taskIds, t.ExecutionTasks...)
		}
		q := testresult.ByTaskIDs(taskIds)
		query = &q
	}
	results, err := testresult.Find(*query)
	if err != nil {
		return nil, err
	}

	for _, t := range tasks {
		for _, result := range results {
			if (result.TaskID == t.Id || utility.StringSliceContains(t.ExecutionTasks, result.TaskID)) && result.Execution == t.Execution {
				t.LocalTestResults = append(t.LocalTestResults, ConvertToOld(&result))
			}
		}
		out = append(out, t)
	}

	return out, nil
}

// FindHostSchedulable finds all tasks that can be scheduled for a distro
// primary queue.
func FindHostSchedulable(distroID string) ([]Task, error) {
	query := schedulableHostTasksQuery()

	if err := addApplicableDistroFilter(distroID, DistroIdKey, query); err != nil {
		return nil, errors.WithStack(err)
	}

	return Find(query)
}

func addApplicableDistroFilter(id string, fieldName string, query bson.M) error {
	if id == "" {
		return nil
	}

	aliases, err := distro.FindApplicableDistroIDs(id)
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
func FindHostSchedulableForAlias(id string) ([]Task, error) {
	q := schedulableHostTasksQuery()

	if err := addApplicableDistroFilter(id, DistroAliasesKey, q); err != nil {
		return nil, errors.WithStack(err)
	}

	// Single-host task groups can't be put in an alias queue, because it can
	// cause a race when assigning tasks to hosts where the tasks in the task
	// group might be assigned to different hosts.
	q[TaskGroupMaxHostsKey] = bson.M{"$ne": 1}

	return FindAll(db.Query(q))
}

// FindHostRunnable finds all host tasks that can be scheduled for a distro with
// an additional consideration for whether the task's dependencies are met. If
// removeDeps is true, tasks with unmet dependencies are excluded.
func FindHostRunnable(distroID string, removeDeps bool) ([]Task, error) {
	match := schedulableHostTasksQuery()
	var d distro.Distro
	var err error
	if distroID != "" {
		foundDistro, err := distro.FindOne(distro.ById(distroID).WithFields(distro.ValidProjectsKey))
		if err != nil {
			return nil, errors.Wrapf(err, "finding distro '%s'", distroID)
		}
		if foundDistro != nil {
			d = *foundDistro
		}
	}

	if err = addApplicableDistroFilter(distroID, DistroIdKey, match); err != nil {
		return nil, errors.WithStack(err)
	}

	matchActivatedUndispatchedTasks := bson.M{
		"$match": match,
	}

	filterInvalidDistros := bson.M{
		"$match": bson.M{ProjectKey: bson.M{"$in": d.ValidProjects}},
	}

	removeFields := bson.M{
		"$project": bson.M{
			LogsKey:      0,
			OldTaskIdKey: 0,
			DependsOnKey + "." + DependencyUnattainableKey: 0,
		},
	}

	graphLookupTaskDeps := bson.M{
		"$graphLookup": bson.M{
			"from":             Collection,
			"startWith":        "$" + DependsOnKey + "." + IdKey,
			"connectFromField": DependsOnKey + "." + IdKey,
			"connectToField":   IdKey,
			"as":               dependencyKey,
			// restrict graphLookup to only direct dependencies
			"maxDepth": 0,
		},
	}

	unwindDependencies := bson.M{
		"$unwind": bson.M{
			"path":                       "$" + dependencyKey,
			"preserveNullAndEmptyArrays": true,
		},
	}

	unwindDependsOn := bson.M{
		"$unwind": bson.M{
			"path":                       "$" + DependsOnKey,
			"preserveNullAndEmptyArrays": true,
		},
	}

	matchIds := bson.M{
		"$match": bson.M{
			"$expr": bson.M{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyTaskIdKey), "$" + bsonutil.GetDottedKeyName(dependencyKey, IdKey)}},
		},
	}

	projectSatisfied := bson.M{
		"$addFields": bson.M{
			"satisfied_dependencies": bson.M{
				"$cond": bson.A{
					bson.M{
						"$or": []bson.M{
							{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyStatusKey), "$" + bsonutil.GetDottedKeyName(dependencyKey, StatusKey)}},
							{"$and": []bson.M{
								{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyStatusKey), "*"}},
								{"$or": []bson.M{
									{"$in": bson.A{"$" + bsonutil.GetDottedKeyName(dependencyKey, StatusKey), evergreen.TaskCompletedStatuses}},
									{"$anyElementTrue": "$" + bsonutil.GetDottedKeyName(dependencyKey, DependsOnKey, DependencyUnattainableKey)},
								}},
							}},
						},
					},
					true,
					false,
				},
			},
		},
	}

	regroupTasks := bson.M{
		"$group": bson.M{
			"_id":           "$_id",
			"satisfied_set": bson.M{"$addToSet": "$satisfied_dependencies"},
			"root":          bson.M{"$first": "$$ROOT"},
		},
	}

	redactUnsatisfiedDependencies := bson.M{
		"$redact": bson.M{
			"$cond": bson.A{
				bson.M{"$allElementsTrue": "$satisfied_set"},
				"$$KEEP",
				"$$PRUNE",
			},
		},
	}

	replaceRoot := bson.M{"$replaceRoot": bson.M{"newRoot": "$root"}}

	joinProjectRef := bson.M{
		"$lookup": bson.M{
			"from":         "project_ref",
			"localField":   ProjectKey,
			"foreignField": "_id",
			"as":           "project_ref",
		},
	}

	filterDisabledProjects := bson.M{
		"$match": bson.M{
			bsonutil.GetDottedKeyName("project_ref", "0", "enabled"):              true,
			bsonutil.GetDottedKeyName("project_ref", "0", "dispatching_disabled"): bson.M{"$ne": true},
		},
	}

	filterPatchingDisabledProjects := bson.M{
		"$match": bson.M{"$or": []bson.M{
			{
				RequesterKey: bson.M{"$nin": evergreen.PatchRequesters},
			},
			{
				bsonutil.GetDottedKeyName("project_ref", "0", "patching_disabled"): false,
			},
		}},
	}

	removeProjectRef := bson.M{
		"$project": bson.M{
			"project_ref": 0,
		},
	}

	pipeline := []bson.M{
		matchActivatedUndispatchedTasks,
		removeFields,
		graphLookupTaskDeps,
	}

	if distroID != "" && len(d.ValidProjects) > 0 {
		pipeline = append(pipeline, filterInvalidDistros)
	}

	if removeDeps {
		pipeline = append(pipeline,
			unwindDependencies,
			unwindDependsOn,
			matchIds,
			projectSatisfied,
			regroupTasks,
			redactUnsatisfiedDependencies,
			replaceRoot,
		)
	}

	pipeline = append(pipeline,
		joinProjectRef,
		filterDisabledProjects,
		filterPatchingDisabledProjects,
		removeProjectRef,
	)

	runnableTasks := []Task{}
	if err := Aggregate(pipeline, &runnableTasks); err != nil {
		return nil, errors.Wrap(err, "fetching runnable host tasks")
	}

	return runnableTasks, nil
}

func GetTestCountByTaskIdAndFilters(taskId, testName string, statuses []string, execution int) (int, error) {
	t, err := FindOneIdNewOrOld(taskId)
	if err != nil {
		return 0, errors.Wrapf(err, "finding task '%s'", taskId)
	}
	if t == nil {
		return 0, errors.Errorf("task '%s' not found", taskId)
	}
	var taskIds []string
	if t.DisplayOnly {
		taskIds = t.ExecutionTasks
	} else {
		taskIds = []string{taskId}
	}
	count, err := testresult.TestResultCount(taskIds, testName, statuses, execution)
	if err != nil {
		return 0, errors.Wrapf(err, "counting test results for task '%s'", taskId)
	}
	return count, nil
}

// FindVariantsWithTask returns a list of build variants between specified commits that contain a specific task name
func FindVariantsWithTask(taskName, project string, orderMin, orderMax int) ([]string, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				ProjectKey:     project,
				RequesterKey:   evergreen.RepotrackerVersionRequester,
				DisplayNameKey: taskName,
				"$and": []bson.M{
					{RevisionOrderNumberKey: bson.M{"$gte": orderMin}},
					{RevisionOrderNumberKey: bson.M{"$lte": orderMax}},
				},
			},
		},
		{
			"$group": bson.M{
				"_id": "$" + BuildVariantKey,
			},
		},
	}
	docs := []map[string]string{}
	err := Aggregate(pipeline, &docs)
	if err != nil {
		return nil, errors.Wrapf(err, "finding variants with task named '%s'", taskName)
	}
	variants := []string{}
	for _, doc := range docs {
		variants = append(variants, doc["_id"])
	}
	return variants, nil
}

func (t *Task) IsPartOfSingleHostTaskGroup() bool {
	return t.TaskGroup != "" && t.TaskGroupMaxHosts == 1
}

func (t *Task) IsPartOfDisplay() bool {
	// if display task ID is nil, we need to check manually if we have an execution task
	if t.DisplayTaskId == nil {
		dt, err := t.GetDisplayTask()
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

func (t *Task) GetDisplayTask() (*Task, error) {
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
			dt, err = FindOneOldByIdAndExecution(dtId, t.Execution)
		} else {
			dt, err = FindOneOld(ByExecutionTask(t.OldTaskId))
			if dt != nil {
				dtId = dt.OldTaskId // save the original task ID to cache
			}
		}
	} else {
		if dtId != "" {
			dt, err = FindOneId(dtId)
		} else {
			dt, err = FindOne(db.Query(ByExecutionTask(t.Id)))
			if dt != nil {
				dtId = dt.Id
			}
		}
	}
	if err != nil {
		return nil, err
	}

	if t.DisplayTaskId == nil {
		// Cache display task ID for future use. If we couldn't find the display task,
		// we cache the empty string to show that it doesn't exist.
		grip.Error(message.WrapError(t.SetDisplayTaskID(dtId), message.Fields{
			"message":         "failed to cache display task ID for task",
			"task_id":         t.Id,
			"display_task_id": dtId,
		}))
	}

	t.DisplayTask = dt
	return dt, nil
}

// GetAllDependencies returns all the dependencies the tasks in taskIDs rely on
func GetAllDependencies(taskIDs []string, taskMap map[string]*Task) ([]Dependency, error) {
	// fill in the gaps in taskMap
	tasksToFetch := []string{}
	for _, tID := range taskIDs {
		if _, ok := taskMap[tID]; !ok {
			tasksToFetch = append(tasksToFetch, tID)
		}
	}
	missingTaskMap := make(map[string]*Task)
	if len(tasksToFetch) > 0 {
		missingTasks, err := FindAll(db.Query(ByIds(tasksToFetch)).WithFields(DependsOnKey))
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

func (t *Task) GetHistoricRuntime() (time.Duration, error) {
	runtimes, err := getExpectedDurationsForWindow(t.DisplayName, t.Project, t.BuildVariant, t.FinishTime.Add(-oneMonthIsh), t.FinishTime.Add(-time.Second))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	if len(runtimes) != 1 {
		return 0, errors.Errorf("expected exactly one task runtime data point, but actually got %d", len(runtimes))
	}

	return time.Duration(runtimes[0].ExpectedDuration), nil
}

func (t *Task) FetchExpectedDuration() util.DurationStats {
	if t.DurationPrediction.TTL == 0 {
		t.DurationPrediction.TTL = utility.JitterInterval(predictionTTL)
	}

	if t.DurationPrediction.Value == 0 && t.ExpectedDuration != 0 {
		// this is probably just backfill, if we have an
		// expected duration, let's assume it was collected
		// before now slightly.
		t.DurationPrediction.Value = t.ExpectedDuration
		t.DurationPrediction.CollectedAt = time.Now().Add(-time.Minute)

		if err := t.cacheExpectedDuration(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"task":    t.Id,
				"message": "caching expected duration",
			}))
		}

		return util.DurationStats{Average: t.ExpectedDuration, StdDev: t.ExpectedDurationStdDev}
	}

	refresher := func(previous util.DurationStats) (util.DurationStats, bool) {
		defaultVal := util.DurationStats{Average: defaultTaskDuration, StdDev: 0}
		vals, err := getExpectedDurationsForWindow(t.DisplayName, t.Project, t.BuildVariant, time.Now().Add(-taskCompletionEstimateWindow), time.Now())
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
		if err := t.cacheExpectedDuration(); err != nil {
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
			fileParts := eitherSlash.Split(testResult.TestFile, -1)
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
	for _, dependency := range t.DependsOn {
		if dependency.Unattainable {
			return true
		}
	}

	return false
}

func (t *Task) BlockedState(dependencies map[string]*Task) (string, error) {
	if t.Blocked() {
		return evergreen.TaskStatusBlocked, nil
	}

	for _, dep := range t.DependsOn {
		depTask, ok := dependencies[dep.TaskId]
		if !ok {
			continue
		}
		if !t.SatisfiesDependency(depTask) {
			return evergreen.TaskStatusPending, nil
		}
	}

	return "", nil
}

// CircularDependencies detects if any tasks in this version are part of a dependency cycle
// Note that it does not check inter-version dependencies, because only evergreen can add those
func (t *Task) CircularDependencies() error {
	var err error
	tasksWithDeps, err := FindAllTasksFromVersionWithDependencies(t.Version)
	if err != nil {
		return errors.Wrap(err, "finding tasks with dependencies")
	}
	if len(tasksWithDeps) == 0 {
		return nil
	}
	dependencyMap := map[string][]string{}
	for _, versionTask := range tasksWithDeps {
		for _, dependency := range versionTask.DependsOn {
			dependencyMap[versionTask.Id] = append(dependencyMap[versionTask.Id], dependency.TaskId)
		}
	}
	catcher := grip.NewBasicCatcher()
	cycles := tarjan.Connections(dependencyMap)
	for _, cycle := range cycles {
		if len(cycle) > 1 {
			catcher.Errorf("dependency cycle detected: %s", strings.Join(cycle, ","))
		}
	}
	return catcher.Resolve()
}

func (t *Task) FindAllUnmarkedBlockedDependencies() ([]Task, error) {
	okStatusSet := []string{AllStatuses, t.Status}
	query := db.Query(bson.M{
		DependsOnKey: bson.M{"$elemMatch": bson.M{
			DependencyTaskIdKey:       t.Id,
			DependencyStatusKey:       bson.M{"$nin": okStatusSet},
			DependencyUnattainableKey: false,
		},
		}},
	)
	return FindAll(query)
}

func (t *Task) FindAllMarkedUnattainableDependencies() ([]Task, error) {
	query := db.Query(bson.M{
		DependsOnKey: bson.M{"$elemMatch": bson.M{
			DependencyTaskIdKey:       t.Id,
			DependencyUnattainableKey: true,
		},
		}},
	)
	return FindAll(query)
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

func GetLatestExecution(taskId string) (int, error) {
	var t *Task
	var err error
	t, err = FindOneId(taskId)
	if err != nil {
		return -1, err
	}
	if t == nil {
		pieces := strings.Split(taskId, "_")
		pieces = pieces[:len(pieces)-1]
		taskId = strings.Join(pieces, "_")
		t, err = FindOneId(taskId)
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
	TaskName       string
	Limit          int
}

type GetTasksByVersionOptions struct {
	Statuses                       []string
	BaseStatuses                   []string
	Variants                       []string
	TaskNames                      []string
	Page                           int
	Limit                          int
	FieldsToProject                []string
	Sorts                          []TasksSortOrder
	IncludeExecutionTasks          bool
	IncludeBaseTasks               bool
	IncludeEmptyActivation         bool
	IncludeBuildVariantDisplayName bool
	IsMainlineCommit               bool
}

// GetTasksByVersion gets all tasks for a specific version
// Query results can be filtered by task name, variant name and status in addition to being paginated and limited
func GetTasksByVersion(versionID string, opts GetTasksByVersionOptions) ([]Task, int, error) {

	pipeline := getTasksByVersionPipeline(versionID, opts)

	if len(opts.Sorts) > 0 {
		sortPipeline := []bson.M{}

		sortFields := bson.D{}
		for _, singleSort := range opts.Sorts {
			if singleSort.Key == DisplayStatusKey || singleSort.Key == BaseTaskStatusKey {
				sortPipeline = append(sortPipeline, addStatusColorSort((singleSort.Key)))
				sortFields = append(sortFields, bson.E{Key: "__" + singleSort.Key, Value: singleSort.Order})
			} else if singleSort.Key == TimeTakenKey {
				sortPipeline = append(sortPipeline, recalculateTimeTaken())
				sortFields = append(sortFields, bson.E{Key: singleSort.Key, Value: singleSort.Order})
			} else {
				sortFields = append(sortFields, bson.E{Key: singleSort.Key, Value: singleSort.Order})
			}
		}
		sortFields = append(sortFields, bson.E{Key: IdKey, Value: 1})

		sortPipeline = append(sortPipeline, bson.M{
			"$sort": sortFields,
		})

		pipeline = append(pipeline, sortPipeline...)
	}

	if len(opts.FieldsToProject) > 0 {
		fieldKeys := bson.M{}
		for _, field := range opts.FieldsToProject {
			fieldKeys[field] = 1
		}
		pipeline = append(pipeline, bson.M{
			"$project": fieldKeys,
		})
	}

	// If there is a limit we should calculate the total count before we apply the limit and pagination
	if opts.Limit > 0 {
		paginatePipeline := []bson.M{}
		paginatePipeline = append(paginatePipeline, bson.M{
			"$skip": opts.Page * opts.Limit,
		})
		paginatePipeline = append(paginatePipeline, bson.M{
			"$limit": opts.Limit,
		})
		// Use a $facet to perform separate aggregations for $count and to sort and paginate the results in the same query
		tasksAndCountPipeline := bson.M{
			"$facet": bson.M{
				"count": []bson.M{
					{"$count": "count"},
				},
				"tasks": paginatePipeline,
			},
		}
		pipeline = append(pipeline, tasksAndCountPipeline)
	}

	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}

	var results []Task
	var count int

	// If there is no limit applied we should just return the tasks and compute the total count in go.
	// This avoids hitting the 16 MB limit on the aggregation pipeline in the $facet stage https://jira.mongodb.org/browse/EVG-15334
	if opts.Limit > 0 {
		type TasksAndCount struct {
			Tasks []Task           `bson:"tasks"`
			Count []map[string]int `bson:"count"`
		}
		taskAndCountResults := []TasksAndCount{}
		err = cursor.All(ctx, &taskAndCountResults)
		if err != nil {
			return nil, 0, err
		}
		if len(taskAndCountResults) > 0 && len(taskAndCountResults[0].Count) > 0 {
			count = taskAndCountResults[0].Count[0]["count"]
			results = taskAndCountResults[0].Tasks
		}
	} else {
		taskResults := []Task{}
		err = cursor.All(ctx, &taskResults)
		if err != nil {
			return nil, 0, err
		}
		results = taskResults
		count = len(results)
	}

	if len(results) == 0 {
		return nil, 0, nil
	}

	return results, count, nil
}

type StatusCount struct {
	Status string `bson:"status"`
	Count  int    `bson:"count"`
}

type TaskStats struct {
	Counts []StatusCount `bson:"counts"`
	ETA    *time.Time    `bson:"eta"`
}

func GetTaskStatsByVersion(versionID string, opts GetTasksByVersionOptions) (*TaskStats, error) {
	pipeline := getTasksByVersionPipeline(versionID, opts)
	maxEtaPipeline := []bson.M{
		{
			"$match": bson.M{
				ExpectedDurationKey: bson.M{"$exists": true},
				StartTimeKey:        bson.M{"$exists": true},
				DisplayStatusKey:    bson.M{"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
			},
		},
		{
			"$project": bson.M{
				"eta": bson.M{
					"$add": []interface{}{
						bson.M{"$divide": []interface{}{"$" + ExpectedDurationKey, time.Millisecond}},
						"$" + StartTimeKey,
					},
				},
			},
		},
		{
			"$group": bson.M{
				"_id":     nil,
				"max_eta": bson.M{"$max": "$eta"},
			},
		},
		{
			"$project": bson.M{
				"_id":     0,
				"max_eta": 1,
			},
		},
	}
	groupPipeline := []bson.M{
		{"$group": bson.M{
			"_id":   "$" + DisplayStatusKey,
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{"_id": 1}},
		{"$project": bson.M{
			"status": "$_id",
			"count":  1,
		}},
	}
	facet := bson.M{"$facet": bson.M{
		"counts": groupPipeline,
		"eta":    maxEtaPipeline,
	}}
	pipeline = append(pipeline, facet)

	type maxETAForQuery struct {
		MaxETA time.Time `bson:"max_eta"`
	}

	type taskStatsForQueryResult struct {
		Counts []StatusCount    `bson:"counts"`
		ETA    []maxETAForQuery `bson:"eta"`
	}

	taskStats := []taskStatsForQueryResult{}
	if err := Aggregate(pipeline, &taskStats); err != nil {
		return nil, errors.Wrap(err, "aggregating task stats for version")
	}
	result := TaskStats{}
	result.Counts = taskStats[0].Counts
	if len(taskStats[0].ETA) > 0 {
		result.ETA = &taskStats[0].ETA[0].MaxETA
	}

	return &result, nil
}

type GroupedTaskStatusCount struct {
	Variant      string         `bson:"variant"`
	DisplayName  string         `bson:"display_name"`
	StatusCounts []*StatusCount `bson:"status_counts"`
}

func GetGroupedTaskStatsByVersion(versionID string, opts GetTasksByVersionOptions) ([]*GroupedTaskStatusCount, error) {
	pipeline := getTasksByVersionPipeline(versionID, opts)
	project := bson.M{"$project": bson.M{
		BuildVariantKey:            "$" + BuildVariantKey,
		BuildVariantDisplayNameKey: "$" + BuildVariantDisplayNameKey,
		DisplayStatusKey:           "$" + DisplayStatusKey,
	}}
	pipeline = append(pipeline, project)
	variantStatusesKey := "variant_statuses"
	statusCountsKey := "status_counts"
	groupByStatusPipeline := []bson.M{
		// Group tasks by variant
		{
			"$group": bson.M{
				"_id": "$" + BuildVariantKey,
				variantStatusesKey: bson.M{
					"$push": bson.M{
						DisplayStatusKey:           "$" + DisplayStatusKey,
						BuildVariantKey:            "$" + BuildVariantKey,
						BuildVariantDisplayNameKey: "$" + BuildVariantDisplayNameKey,
					},
				},
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$" + variantStatusesKey,
				"preserveNullAndEmptyArrays": false,
			},
		},
		{
			"$project": bson.M{
				variantStatusesKey: 1,
				"_id":              0,
			},
		},
		// Group tasks by variant and status and calculate count for each status
		{
			"$group": bson.M{
				"_id": bson.M{
					DisplayStatusKey:           "$" + bsonutil.GetDottedKeyName(variantStatusesKey, DisplayStatusKey),
					BuildVariantKey:            "$" + bsonutil.GetDottedKeyName(variantStatusesKey, BuildVariantKey),
					BuildVariantDisplayNameKey: "$" + bsonutil.GetDottedKeyName(variantStatusesKey, BuildVariantDisplayNameKey),
				},
				"count": bson.M{"$sum": 1},
			},
		},
		// Sort the values by status so they are sorted before being grouped. This will ensure that they are sorted in the array when they are grouped.
		{
			"$sort": bson.M{
				bsonutil.GetDottedKeyName("_id", DisplayStatusKey): 1,
			},
		},
		// Group the elements by build variant and status_counts
		{
			"$group": bson.M{
				"_id": bson.M{BuildVariantKey: "$" + bsonutil.GetDottedKeyName("_id", BuildVariantKey), BuildVariantDisplayNameKey: "$" + bsonutil.GetDottedKeyName("_id", BuildVariantDisplayNameKey)},
				statusCountsKey: bson.M{
					"$push": bson.M{
						"status": "$" + bsonutil.GetDottedKeyName("_id", DisplayStatusKey),
						"count":  "$count",
					},
				},
			},
		},
		{
			"$project": bson.M{
				"variant":       "$" + bsonutil.GetDottedKeyName("_id", BuildVariantKey),
				"display_name":  "$" + bsonutil.GetDottedKeyName("_id", BuildVariantDisplayNameKey),
				statusCountsKey: 1,
			},
		},
		// Sort build variants in alphanumeric order for final return
		{
			"$sort": bson.M{
				"display_name": 1,
			},
		},
	}
	pipeline = append(pipeline, groupByStatusPipeline...)
	result := []*GroupedTaskStatusCount{}

	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "aggregating task stats")
	}
	return result, nil

}

// GetBaseStatusesForActivatedTasks returns the base statuses for activated tasks on a version.
func GetBaseStatusesForActivatedTasks(versionID string, baseVersionID string) ([]string, error) {
	pipeline := []bson.M{}
	taskField := "tasks"

	// Fetch all activated tasks from version, and all tasks from base version
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{
			"$or": []bson.M{
				{VersionKey: baseVersionID},
				{VersionKey: versionID, ActivatedTimeKey: bson.M{"$ne": utility.ZeroTime}},
			},
		}})
	// Add display status
	pipeline = append(pipeline, addDisplayStatus)
	// Group by display name and build variant, and keep track of DisplayStatus and Version fields
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": bson.M{DisplayNameKey: "$" + DisplayNameKey, BuildVariantKey: "$" + BuildVariantKey},
			taskField: bson.M{"$push": bson.M{
				DisplayStatusKey: "$" + DisplayStatusKey,
				VersionKey:       "$" + VersionKey,
			}},
		},
	})
	// Only keep records that exist both on the version & base version (i.e. there are 2 copies)
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{taskField: bson.M{"$size": 2}},
	})
	// Unwind to put tasks into a state where it's easier to filter
	pipeline = append(pipeline, bson.M{
		"$unwind": bson.M{
			"path": "$" + taskField,
		},
	})
	// Filter out tasks that aren't from base version
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{bsonutil.GetDottedKeyName(taskField, VersionKey): baseVersionID},
	})
	// Group to get rid of duplicate statuses
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": "$" + bsonutil.GetDottedKeyName(taskField, DisplayStatusKey),
		},
	})
	// Sort to guarantee order
	pipeline = append(pipeline, bson.M{
		"$sort": bson.D{
			bson.E{Key: "_id", Value: 1},
		},
	})

	res := []map[string]string{}
	err := Aggregate(pipeline, &res)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating base task statuses")
	}
	statuses := []string{}
	for _, r := range res {
		statuses = append(statuses, r["_id"])
	}
	return statuses, nil
}

type HasMatchingTasksOptions struct {
	TaskNames []string
	Variants  []string
	Statuses  []string
}

// HasMatchingTasks returns true if the version has tasks with the given statuses
func HasMatchingTasks(versionID string, opts HasMatchingTasksOptions) (bool, error) {
	options := GetTasksByVersionOptions{
		TaskNames:                      opts.TaskNames,
		Variants:                       opts.Variants,
		Statuses:                       opts.Statuses,
		IncludeBuildVariantDisplayName: true,
	}
	pipeline := getTasksByVersionPipeline(versionID, options)
	pipeline = append(pipeline, bson.M{"$count": "count"})
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return false, err
	}
	type Count struct {
		Count int `bson:"count"`
	}
	count := []*Count{}
	err = cursor.All(ctx, &count)
	if err != nil {
		return false, err
	}
	if len(count) == 0 {
		return false, nil
	}
	return count[0].Count > 0, nil
}

func getTasksByVersionPipeline(versionID string, opts GetTasksByVersionOptions) []bson.M {
	var match bson.M = bson.M{}

	// Allow searching by either variant name or variant display
	if len(opts.Variants) > 0 {
		variantsAsRegex := strings.Join(opts.Variants, "|")

		match = bson.M{
			"$or": []bson.M{
				{BuildVariantDisplayNameKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
				{BuildVariantKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
			},
		}
	}
	if len(opts.TaskNames) > 0 {
		taskNamesAsRegex := strings.Join(opts.TaskNames, "|")
		match[DisplayNameKey] = bson.M{"$regex": taskNamesAsRegex, "$options": "i"}
	}
	// Activated Time is needed to filter out generated tasks that have been generated but not yet activated
	if !opts.IncludeEmptyActivation {
		match[ActivatedTimeKey] = bson.M{"$ne": utility.ZeroTime}
	}
	match[VersionKey] = versionID
	pipeline := []bson.M{}
	// Add BuildVariantDisplayName to all the results if it we need to match on the entire set of results
	// This is an expensive operation so we only want to do it if we have to
	if len(opts.Variants) > 0 && opts.IncludeBuildVariantDisplayName {
		pipeline = append(pipeline, AddBuildVariantDisplayName...)
	}
	pipeline = append(pipeline,
		bson.M{"$match": match},
	)

	if !opts.IncludeExecutionTasks {
		const tempParentKey = "_parent"
		// Split tasks so that we only look up if the task is an execution task if display task ID is unset and
		// display only is false (i.e. we don't know if it's a display task or not).
		facet := bson.M{
			"$facet": bson.M{
				// We skip lookup for anything we already know is not part of a display task
				"id_empty": []bson.M{
					{
						"$match": bson.M{
							"$or": []bson.M{
								{DisplayTaskIdKey: ""},
								{DisplayOnlyKey: true},
							},
						},
					},
				},
				// No ID and not display task: lookup if it's an execution task for some task, and then filter it out if it is
				"no_id": []bson.M{
					{
						"$match": bson.M{
							DisplayTaskIdKey: nil,
							DisplayOnlyKey:   bson.M{"$ne": true},
						},
					},
					{"$lookup": bson.M{
						"from":         Collection,
						"localField":   IdKey,
						"foreignField": ExecutionTasksKey,
						"as":           tempParentKey,
					}},
					{
						"$match": bson.M{
							tempParentKey: []interface{}{},
						},
					},
				},
			},
		}
		pipeline = append(pipeline, facet)

		// Recombine the tasks so that we can continue the pipeline on the joined tasks
		recombineTasks := []bson.M{
			{"$project": bson.M{
				"tasks": bson.M{
					"$setUnion": []string{"$no_id", "$id_empty"},
				}},
			},
			{"$unwind": "$tasks"},
			{"$replaceRoot": bson.M{"newRoot": "$tasks"}},
		}

		pipeline = append(pipeline, recombineTasks...)
	}

	annotationFacet := bson.M{
		"$facet": bson.M{
			// We skip annotation lookup for non-failed tasks, because these can't have annotations
			"not_failed": []bson.M{
				{
					"$match": bson.M{
						StatusKey: bson.M{"$nin": evergreen.TaskFailureStatuses},
					},
				},
			},
			// for failed tasks, get any annotation that has at least one issue
			"failed": []bson.M{
				{
					"$match": bson.M{
						StatusKey: bson.M{"$in": evergreen.TaskFailureStatuses},
					},
				},
				{
					"$lookup": bson.M{
						"from": annotations.Collection,
						"let":  bson.M{"task_annotation_id": "$" + IdKey, "task_annotation_execution": "$" + ExecutionKey},
						"pipeline": []bson.M{
							{
								"$match": bson.M{
									"$expr": bson.M{
										"$and": []bson.M{
											{
												"$eq": []string{"$" + annotations.TaskIdKey, "$$task_annotation_id"},
											},
											{
												"$eq": []string{"$" + annotations.TaskExecutionKey, "$$task_annotation_execution"},
											},
											{
												"$ne": []interface{}{
													bson.M{
														"$size": bson.M{"$ifNull": []interface{}{"$" + annotations.IssuesKey, []bson.M{}}},
													}, 0,
												},
											},
										},
									},
								}}},
						"as": "annotation_docs",
					},
				},
			},
		},
	}
	pipeline = append(pipeline, annotationFacet)
	recombineAnnotationFacet := []bson.M{
		{"$project": bson.M{
			"tasks": bson.M{
				"$setUnion": []string{"$not_failed", "$failed"},
			}},
		},
		{"$unwind": "$tasks"},
		{"$replaceRoot": bson.M{"newRoot": "$tasks"}},
	}
	pipeline = append(pipeline, recombineAnnotationFacet...)
	pipeline = append(pipeline,
		// Add a field for the display status of each task
		addDisplayStatus,
	)
	// Filter on the computed display status before continuing to add additional fields.
	if len(opts.Statuses) > 0 {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				DisplayStatusKey: bson.M{"$in": opts.Statuses},
			},
		})
	}
	if opts.IncludeBaseTasks {
		baseCommitMatch := []bson.M{
			{"$eq": []string{"$" + BuildVariantKey, "$$" + BuildVariantKey}},
			{"$eq": []string{"$" + DisplayNameKey, "$$" + DisplayNameKey}},
		}

		// If we are requesting a mainline commit's base task we want to use the previous commit instead.
		if opts.IsMainlineCommit {
			baseCommitMatch = append(baseCommitMatch, bson.M{
				"$eq": []interface{}{"$" + RevisionOrderNumberKey, bson.M{
					"$subtract": []interface{}{"$$" + RevisionOrderNumberKey, 1},
				}},
			})
		} else {
			baseCommitMatch = append(baseCommitMatch, bson.M{
				"$eq": []string{"$" + RevisionKey, "$$" + RevisionKey},
			})
		}
		pipeline = append(pipeline, []bson.M{
			// Add data about the base task
			{"$lookup": bson.M{
				"from": Collection,
				"let": bson.M{
					RevisionKey:            "$" + RevisionKey,
					BuildVariantKey:        "$" + BuildVariantKey,
					DisplayNameKey:         "$" + DisplayNameKey,
					RevisionOrderNumberKey: "$" + RevisionOrderNumberKey,
				},
				"as": BaseTaskKey,
				"pipeline": []bson.M{
					{"$match": bson.M{
						RequesterKey: evergreen.RepotrackerVersionRequester,
						"$expr": bson.M{
							"$and": baseCommitMatch,
						},
					}},
					{"$project": bson.M{
						IdKey:     1,
						StatusKey: displayStatusExpression,
					}},
					{"$limit": 1},
				},
			}},
			{
				"$unwind": bson.M{
					"path":                       "$" + BaseTaskKey,
					"preserveNullAndEmptyArrays": true,
				},
			},
		}...,
		)
	}
	// Add the build variant display name to the returned subset of results if it wasn't added earlier
	if len(opts.Variants) == 0 && opts.IncludeBuildVariantDisplayName {
		pipeline = append(pipeline, AddBuildVariantDisplayName...)
	}

	if opts.IncludeBaseTasks && len(opts.BaseStatuses) > 0 {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				BaseTaskStatusKey: bson.M{"$in": opts.BaseStatuses},
			},
		})
	}

	return pipeline
}

func recalculateTimeTaken() bson.M {
	return bson.M{
		"$set": bson.M{
			TimeTakenKey: bson.M{
				"$cond": bson.M{
					"if": bson.M{
						"$eq": []string{"$" + StatusKey, evergreen.TaskStarted},
					},
					// Time taken for a task is in nanoseconds. Since subtracting two dates in MongoDB yields milliseconds, we have
					// to multiply by time.Millisecond (1000000) to keep time taken consistently in nanoseconds.
					"then": bson.M{"$multiply": []interface{}{time.Millisecond, bson.M{"$subtract": []interface{}{"$$NOW", "$" + StartTimeKey}}}},
					"else": "$" + TimeTakenKey,
				},
			},
		},
	}
}

// addStatusColorSort adds a stage which takes a task display status and returns an integer
// for the rank at which it should be sorted. the return value groups all statuses with the
// same color together. this should be kept consistent with the badge status colors in spruce
func addStatusColorSort(key string) bson.M {
	return bson.M{
		"$addFields": bson.M{
			"__" + key: bson.M{
				"$switch": bson.M{
					"branches": []bson.M{
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskFailed, evergreen.TaskTestTimedOut, evergreen.TaskTimedOut}},
							},
							"then": 1, // red
						},
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskKnownIssue}},
							},
							"then": 2,
						},
						{
							"case": bson.M{
								"$eq": []string{"$" + key, evergreen.TaskSetupFailed},
							},
							"then": 3, // lavender
						},
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskSystemFailed, evergreen.TaskSystemUnresponse, evergreen.TaskSystemTimedOut}},
							},
							"then": 4, // purple
						},
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
							},
							"then": 5, // yellow
						},
						{
							"case": bson.M{
								"$eq": []string{"$" + key, evergreen.TaskSucceeded},
							},
							"then": 10, // green
						},
					},
					"default": 6, // all shades of grey
				},
			},
		},
	}
}

func AddParentDisplayTasks(tasks []Task) ([]Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}
	taskIDs := []string{}
	tasksCopy := tasks
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	parents, err := FindAll(db.Query(ByExecutionTasks(taskIDs)))
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
func (t *Task) UpdateDependsOn(status string, newDependencyIDs []string) error {
	newDependencies := make([]Dependency, 0, len(newDependencyIDs))
	for _, depID := range newDependencyIDs {
		newDependencies = append(newDependencies, Dependency{
			TaskId: depID,
			Status: status,
		})
	}

	_, err := UpdateAll(
		bson.M{
			DependsOnKey: bson.M{"$elemMatch": bson.M{
				DependencyTaskIdKey: t.Id,
				DependencyStatusKey: status,
			}},
		},
		bson.M{"$push": bson.M{DependsOnKey: bson.M{"$each": newDependencies}}},
	)

	return errors.Wrap(err, "updating dependencies")
}

func (t *Task) SetTaskGroupInfo() error {
	return errors.WithStack(UpdateOne(bson.M{IdKey: t.Id},
		bson.M{"$set": bson.M{
			TaskGroupOrderKey:    t.TaskGroupOrder,
			TaskGroupMaxHostsKey: t.TaskGroupMaxHosts,
		}}))
}

func (t *Task) SetDisplayTaskID(id string) error {
	t.DisplayTaskId = utility.ToStringPtr(id)
	return errors.WithStack(UpdateOne(bson.M{IdKey: t.Id},
		bson.M{"$set": bson.M{
			DisplayTaskIdKey: id,
		}}))
}

func (t *Task) SetNumDependents() error {
	update := bson.M{
		"$set": bson.M{
			NumDepsKey: t.NumDependents,
		},
	}
	if t.NumDependents == 0 {
		update = bson.M{"$unset": bson.M{
			NumDepsKey: "",
		}}
	}
	return UpdateOne(bson.M{
		IdKey: t.Id,
	}, update)
}

func AddDisplayTaskIdToExecTasks(displayTaskId string, execTasksToUpdate []string) error {
	if len(execTasksToUpdate) == 0 {
		return nil
	}
	_, err := UpdateAll(bson.M{
		IdKey: bson.M{"$in": execTasksToUpdate},
	},
		bson.M{"$set": bson.M{
			DisplayTaskIdKey: displayTaskId,
		}},
	)
	return err
}

func AddExecTasksToDisplayTask(displayTaskId string, execTasks []string, displayTaskActivated bool) error {
	if len(execTasks) == 0 {
		return nil
	}
	update := bson.M{"$addToSet": bson.M{
		ExecutionTasksKey: bson.M{"$each": execTasks},
	}}

	if displayTaskActivated {
		// verify that the display task isn't already activated
		dt, err := FindOneId(displayTaskId)
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
		bson.M{IdKey: displayTaskId},
		update,
	)
}

////////////////
// Cedar Helpers
////////////////

// getCedarTestResults fetches the task's test results from the Cedar service.
// If the task does not have test results in Cedar, (nil, nil) is returned. If
// the task is a display task, all of its execution tasks' test results are
// returned.
func (t *Task) getCedarTestResults() ([]TestResult, error) {
	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()

	if !t.hasCedarResults() {
		return nil, nil
	}

	taskID := t.Id
	if t.Archived {
		taskID = t.OldTaskId
	}

	opts := apimodels.GetCedarTestResultsOptions{
		BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		TaskID:      taskID,
		Execution:   utility.ToIntPtr(t.Execution),
		DisplayTask: t.DisplayOnly,
	}

	cedarResults, err := apimodels.GetCedarTestResultsWithStatusError(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting test results from cedar")
	}

	results := make([]TestResult, len(cedarResults.Results))
	for i, result := range cedarResults.Results {
		results[i] = ConvertCedarTestResult(result)
	}

	return results, nil
}

func (t *Task) hasCedarResults() bool {
	if !t.DisplayOnly || t.HasCedarResults {
		return t.HasCedarResults
	}

	// Older display tasks may incorrectly indicate that they do not have
	// test results in Cedar. In the case that the execution tasks have
	// results in Cedar, this will attempt to update the display task
	// accordingly.
	if len(t.ExecutionTasks) > 0 {
		var (
			execTasks []Task
			err       error
		)
		if t.Archived {
			// This is a display task from the old task collection,
			// we need to look there for its execution tasks.
			execTasks, err = FindAllOld(db.Query(bson.M{
				OldTaskIdKey: bson.M{"$in": t.ExecutionTasks},
				ExecutionKey: t.Execution,
			}))
		} else {
			execTasks, err = FindAll(db.Query(ByIds(t.ExecutionTasks)))
		}
		if err != nil {
			return false
		}

		for _, execTask := range execTasks {
			if execTask.HasCedarResults {
				// Attempt to update the display task's
				// HasCedarResults field. We will not update
				// the CedarResultsFailed field since we do
				// want to iterate through all of the execution
				// tasks and it isn't really needed for display
				// tasks. Since we do not want to fail here, we
				// can ignore the error.
				_ = t.SetHasCedarResults(true, false)

				return true
			}
		}
	}

	return false
}

// ConvertCedarTestResult converts a CedarTestResult struct into a TestResult
// struct.
func ConvertCedarTestResult(result apimodels.CedarTestResult) TestResult {
	return TestResult{
		TaskID:          result.TaskID,
		Execution:       result.Execution,
		TestFile:        result.TestName,
		DisplayTestName: result.DisplayTestName,
		GroupID:         result.GroupID,
		LogTestName:     result.LogTestName,
		URL:             result.LogURL,
		URLRaw:          result.RawLogURL,
		LineNum:         result.LineNum,
		StartTime:       float64(result.Start.Unix()),
		EndTime:         float64(result.End.Unix()),
		Status:          result.Status,
	}
}
