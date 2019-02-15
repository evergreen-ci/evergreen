package task

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/tychoish/tarjan"
	"gopkg.in/mgo.v2/bson"
)

const (
	edgesKey = "edges"
	taskKey  = "task"

	// tasks should be unscheduled after ~a week
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

	taskBlocked = "blocked"
	taskPending = "pending"
)

var (
	AgentHeartbeat = "heartbeat"

	// A regex that matches either / or \ for splitting directory paths
	// on either windows or linux paths.
	eitherSlash *regexp.Regexp = regexp.MustCompile(`[/\\]`)
)

type Task struct {
	Id     string `bson:"_id" json:"id"`
	Secret string `bson:"secret" json:"secret"`

	// time information for task
	// create - the creation time for the task, derived from the commit time or the patch creation time.
	// dispatch - the time the task runner starts up the agent on the host
	// scheduled - the time the commit is scheduled
	// start - the time the agent starts the task on the host after spinning it up
	// finish - the time the task was completed on the remote host
	CreateTime    time.Time `bson:"create_time" json:"create_time"`
	IngestTime    time.Time `bson:"injest_time" json:"ingest_time"`
	DispatchTime  time.Time `bson:"dispatch_time" json:"dispatch_time"`
	ScheduledTime time.Time `bson:"scheduled_time" json:"scheduled_time"`
	StartTime     time.Time `bson:"start_time" json:"start_time"`
	FinishTime    time.Time `bson:"finish_time" json:"finish_time"`
	ActivatedTime time.Time `bson:"activated_time" json:"activated_time"`

	Version           string              `bson:"version" json:"version,omitempty"`
	Project           string              `bson:"branch" json:"branch,omitempty"`
	Revision          string              `bson:"gitspec" json:"gitspec"`
	Priority          int64               `bson:"priority" json:"priority"`
	TaskGroup         string              `bson:"task_group" json:"task_group"`
	TaskGroupMaxHosts int                 `bson:"task_group_max_hosts,omitempty" json:"task_group_max_hosts,omitempty"`
	Logs              *apimodels.TaskLogs `bson:"logs,omitempty" json:"logs,omitempty"`

	// only relevant if the task is runnin.  the time of the last heartbeat
	// sent back by the agent
	LastHeartbeat time.Time `bson:"last_heartbeat"`

	// used to indicate whether task should be scheduled to run
	Activated            bool         `bson:"activated" json:"activated"`
	ActivatedBy          string       `bson:"activated_by" json:"activated_by"`
	BuildId              string       `bson:"build_id" json:"build_id"`
	DistroId             string       `bson:"distro" json:"distro"`
	BuildVariant         string       `bson:"build_variant" json:"build_variant"`
	DependsOn            []Dependency `bson:"depends_on" json:"depends_on"`
	NumDependents        int          `bson:"num_dependents,omitempty" json:"num_dependents,omitempty"`
	OverrideDependencies bool         `bson:"override_dependencies,omitempty" json:"override_dependencies,omitempty"`

	// Human-readable name
	DisplayName string `bson:"display_name" json:"display_name"`

	// Tags that describe the task
	Tags []string `bson:"tags,omitempty" json:"tags,omitempty"`

	// The host the task was run on. This value is empty for display
	// tasks
	HostId string `bson:"host_id" json:"host_id"`

	// the number of times this task has been restarted
	Restarts            int    `bson:"restarts" json:"restarts,omitempty"`
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

	// Status represents the various stages the task could be in
	Status  string                  `bson:"status" json:"status"`
	Details apimodels.TaskEndDetail `bson:"details" json:"task_end_details"`
	Aborted bool                    `bson:"abort,omitempty" json:"abort"`

	// TimeTaken is how long the task took to execute.  meaningless if the task is not finished
	TimeTaken time.Duration `bson:"time_taken" json:"time_taken"`

	// how long we expect the task to take from start to
	// finish. expected duration is the legacy value, but the UI
	// probably depends on it, so we maintain both values.
	ExpectedDuration   time.Duration            `bson:"expected_duration,omitempty" json:"expected_duration,omitempty"`
	DurationPrediction util.CachedDurationValue `bson:"duration_prediction,omitempty" json:"-"`

	// an estimate of what the task cost to run, hidden from JSON views for now
	Cost float64 `bson:"cost,omitempty" json:"-"`
	// total estimated cost of hosts this task spawned
	SpawnedHostCost float64 `bson:"spawned_host_cost,omitempty" json:"spawned_host_cost,omitempty"`

	// test results embedded from the testresults collection
	LocalTestResults []TestResult `bson:"-" json:"test_results"`

	// display task fields
	DisplayOnly       bool     `bson:"display_only,omitempty" json:"display_only,omitempty"`
	ExecutionTasks    []string `bson:"execution_tasks,omitempty" json:"execution_tasks,omitempty"`
	ResetWhenFinished bool     `bson:"reset_when_finished,omitempty" json:"reset_when_finished,omitempty"`
	DisplayTask       *Task    `bson:"-" json:"-"` // this is a local pointer from an exec to display task

	// GenerateTask indicates that the task generates other tasks, which the
	// scheduler will use to prioritize this task.
	GenerateTask bool `bson:"generate_task,omitempty" json:"generate_task,omitempty"`
	// GeneratedTasks indicates that the task has already generated other tasks. This fields
	// allows us to noop future requests, since a task should only generate others once.
	GeneratedTasks bool `bson:"generated_tasks,omitempty" json:"generated_tasks,omitempty"`
	// GeneratedBy, if present, is the ID of the task that generated this task.
	GeneratedBy string `bson:"generated_by,omitempty" json:"generated_by,omitempty"`

	// Fields set if triggered by an upstream build
	TriggerID    string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	TriggerType  string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`
}

// Dependency represents a task that must be completed before the owning
// task can be scheduled.
type Dependency struct {
	TaskId string `bson:"_id" json:"id"`
	Status string `bson:"status" json:"status"`
}

// VersionCost is service level model for representing cost data related to a version.
// SumTimeTaken is the aggregation of time taken by all tasks associated with a version.
type VersionCost struct {
	VersionId        string        `bson:"version_id"`
	SumTimeTaken     time.Duration `bson:"sum_time_taken"`
	SumEstimatedCost float64       `bson:"sum_estimated_cost"`
}

// DistroCost is service level model for representing cost data related to a distro.
// SumTimeTaken is the aggregation of time taken by all tasks associated with a distro.
type DistroCost struct {
	DistroId         string                 `bson:"distro_id"`
	SumTimeTaken     time.Duration          `bson:"sum_time_taken"`
	SumEstimatedCost float64                `bson:"sum_estimated_cost"`
	Provider         string                 `json:"provider"`
	ProviderSettings map[string]interface{} `json:"provider_settings"`
	NumTasks         int                    `bson:"num_tasks"`
}

// SetBSON allows us to use dependency representation of both
// just task Ids and of true Dependency structs.
//  TODO eventually drop all of this switching
func (d *Dependency) SetBSON(raw bson.Raw) error {
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
	strBytes, _ := bson.Marshal(bson.RawD{{Name: "str", Value: raw}})
	var strStruct struct {
		String string `bson:"str"`
	}
	if err := bson.Unmarshal(strBytes, &strStruct); err == nil {
		if strStruct.String != "" {
			d.TaskId = strStruct.String
			d.Status = evergreen.TaskSucceeded
			return nil
		}
	}

	return bson.SetZero
}

// LocalTestResults is only used when transferring data from agent to api.
type LocalTestResults struct {
	Results []TestResult `json:"results"`
}

type TestResult struct {
	Status    string  `json:"status" bson:"status"`
	TestFile  string  `json:"test_file" bson:"test_file"`
	URL       string  `json:"url" bson:"url,omitempty"`
	URLRaw    string  `json:"url_raw" bson:"url_raw,omitempty"`
	LogId     string  `json:"log_id,omitempty" bson:"log_id,omitempty"`
	LineNum   int     `json:"line_num,omitempty" bson:"line_num,omitempty"`
	ExitCode  int     `json:"exit_code" bson:"exit_code"`
	StartTime float64 `json:"start" bson:"start"`
	EndTime   float64 `json:"end" bson:"end"`

	// LogRaw is not saved in the task
	LogRaw string `json:"log_raw" bson:"log_raw,omitempty"`
}

var (
	AllStatuses = "*"
)

// Abortable returns true if the task can be aborted.
func IsAbortable(t Task) bool {
	return t.Status == evergreen.TaskStarted ||
		t.Status == evergreen.TaskDispatched
}

// IsFinished returns true if the project is no longer running
func (t *Task) IsFinished() bool {
	return evergreen.IsFinishedTaskStatus(t.Status)
}

// IsDispatchable return true if the task should be dispatched
func (t *Task) IsDispatchable() bool {
	return t.Status == evergreen.TaskUndispatched && t.Activated
}

func (t *Task) GetTaskCreatedTime() time.Time {
	if t.IngestTime.IsZero() {
		return t.CreateTime
	}

	return t.IngestTime
}

// satisfiesDependency checks a task the receiver task depends on
// to see if its status satisfies a dependency. If the "Status" field is
// unset, default to checking that is succeeded.
func (t *Task) satisfiesDependency(depTask *Task) bool {
	for _, dep := range t.DependsOn {
		if dep.TaskId == depTask.Id {
			switch dep.Status {
			case evergreen.TaskSucceeded, "":
				return depTask.Status == evergreen.TaskSucceeded
			case evergreen.TaskFailed:
				return depTask.Status == evergreen.TaskFailed
			case AllStatuses:
				return depTask.Status == evergreen.TaskFailed || depTask.Status == evergreen.TaskSucceeded
			}
		}
	}
	return false
}

func (t *Task) IsPatchRequest() bool {
	return util.StringSliceContains(evergreen.PatchRequesters, t.Requester)
}

func (t *Task) IsMergeRequest() bool {
	return t.Requester == evergreen.MergeTestRequester
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

// Checks whether the dependencies for the task have all completed successfully.
// If any of the dependencies exist in the map that is passed in, they are
// used to check rather than fetching from the database. All queries
// are cached back into the map for later use.
func (t *Task) DependenciesMet(depCaches map[string]Task) (bool, error) {

	if len(t.DependsOn) == 0 || t.OverrideDependencies {
		return true, nil
	}

	deps := make([]Task, 0, len(t.DependsOn))

	depIdsToQueryFor := make([]string, 0, len(t.DependsOn))
	for _, dep := range t.DependsOn {
		if cachedDep, ok := depCaches[dep.TaskId]; !ok {
			depIdsToQueryFor = append(depIdsToQueryFor, dep.TaskId)
		} else {
			deps = append(deps, cachedDep)
		}
	}

	if len(depIdsToQueryFor) > 0 {
		newDeps, err := Find(ByIds(depIdsToQueryFor).WithFields(StatusKey))
		if err != nil {
			return false, err
		}

		// add queried dependencies to the cache
		for _, newDep := range newDeps {
			deps = append(deps, newDep)
			depCaches[newDep.Id] = newDep
		}
	}

	for _, depTask := range deps {
		if !t.satisfiesDependency(&depTask) {
			return false, nil
		}
	}

	return true, nil
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
		if cachedDep, ok := cache[dep.TaskId]; !ok {
			catcher.Add(errors.Errorf("cannot resolve task %s", dep.TaskId))
			continue
		} else {
			deps = append(deps, cachedDep)
		}
	}

	if catcher.HasErrors() {
		return false, catcher.Resolve()
	}

	for _, depTask := range deps {
		if !t.satisfiesDependency(&depTask) {
			return false, nil
		}
	}

	return true, nil
}

// HasFailedTests iterates through a tasks' tests and returns true if
// that task had any failed tests.
func (t *Task) HasFailedTests() bool {
	for _, test := range t.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			return true
		}
	}
	return false
}

// FindTaskOnBaseCommit returns the task that is on the base commit.
func (t *Task) FindTaskOnBaseCommit() (*Task, error) {
	return FindOne(ByCommit(t.Revision, t.BuildVariant, t.DisplayName, t.Project, evergreen.RepotrackerVersionRequester))
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
	return Count(ByDifferentFailedBuildVariants(t.Revision, t.BuildVariant, t.DisplayName,
		t.Project, t.Requester))
}

// Find the previously completed task for the same project +
// build variant + display name combination as the specified task
func (t *Task) PreviousCompletedTask(project string, statuses []string) (*Task, error) {
	if len(statuses) == 0 {
		statuses = CompletedStatuses
	}
	return FindOneNoMerge(ByBeforeRevisionWithStatusesAndRequesters(t.RevisionOrderNumber, statuses, t.BuildVariant,
		t.DisplayName, project, evergreen.SystemVersionRequesterTypes))
}

// SetExpectedDuration updates the expected duration field for the task
func (t *Task) SetExpectedDuration(duration time.Duration) error {
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				ExpectedDurationKey:   duration,
				DurationPredictionKey: t.DurationPrediction,
			},
		},
	)
}

func (t *Task) cacheExpectedDuration() error {
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				DurationPredictionKey: t.DurationPrediction,
				ExpectedDurationKey:   t.DurationPrediction.Value,
			},
		},
	)
}

// Mark that the task has been dispatched onto a particular host. Sets the
// running task field on the host and the host id field on the task.
// Returns an error if any of the database updates fail.
func (t *Task) MarkAsDispatched(hostId string, distroId string, dispatchTime time.Time) error {
	t.DispatchTime = dispatchTime
	t.Status = evergreen.TaskDispatched
	t.HostId = hostId
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
			},
			"$unset": bson.M{
				AbortedKey: "",
				DetailsKey: "",
			},
		},
	)
	if err != nil {
		return errors.Wrapf(err, "error marking task %s as dispatched", t.Id)
	}
	if t.IsPartOfDisplay() {
		//when dispatching an execution task, mark its parent as dispatched
		if t.DisplayTask != nil && t.DisplayTask.DispatchTime == util.ZeroTime {
			return t.DisplayTask.MarkAsDispatched("", "", dispatchTime)
		}
	}
	return nil
}

// MarkAsUndispatched marks that the task has been undispatched from a
// particular host. Unsets the running task field on the host and the
// host id field on the task
// Returns an error if any of the database updates fail.
func (t *Task) MarkAsUndispatched() error {
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
				DispatchTimeKey:  util.ZeroTime,
				LastHeartbeatKey: util.ZeroTime,
				DistroIdKey:      "",
				HostIdKey:        "",
				AbortedKey:       "",
				DetailsKey:       "",
			},
		},
	)
}

// MarkGeneratedTasks marks that the task has generated tasks.
func (t *Task) MarkGeneratedTasks() error {
	t.GeneratedTasks = true
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				GeneratedTasksKey: true,
			},
		},
	)
}

// SetTasksScheduledTime takes a list of tasks and a time, and then sets
// the scheduled time in the database for the tasks if it is currently unset
func SetTasksScheduledTime(tasks []Task, scheduledTime time.Time) error {
	var ids []string
	for i := range tasks {
		tasks[i].ScheduledTime = scheduledTime
		ids = append(ids, tasks[i].Id)
	}
	info, err := UpdateAll(
		bson.M{
			IdKey: bson.M{
				"$in": ids,
			},
			ScheduledTimeKey: bson.M{
				"$lte": util.ZeroTime,
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

	if info.Updated > 0 {
		for _, t := range tasks {
			event.LogTaskScheduled(t.Id, t.Execution, scheduledTime)
		}
	}
	return nil
}

// Removes tasks older than the unscheduable threshold (e.g. two
// weeks) from the scheduler queue.
//
// If you pass an empty string as an argument to this function, this
// operation will select tasks from all distros.
func UnscheduleStaleUnderwaterTasks(distroID string) (int, error) {
	query := scheduleableTasksQuery()
	query[PriorityKey] = 0

	if distroID != "" {
		query[DistroIdKey] = distroID
	}

	query["$and"] = []bson.M{
		{ActivatedTimeKey: bson.M{"$lte": time.Now().Add(-UnschedulableThreshold)}},
		{ActivatedTimeKey: bson.M{"$gt": util.ZeroTime}},
	}

	update := bson.M{
		"$set": bson.M{
			PriorityKey:  -1,
			ActivatedKey: false,
		},
	}

	info, err := UpdateAll(query, update)
	if err != nil {
		return 0, errors.Wrap(err, "problem unscheduling stale underwater tasks")
	}

	return info.Updated, nil
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

func (t *Task) MarkSystemFailed() error {
	t.Status = evergreen.TaskFailed
	t.FinishTime = time.Now()

	t.Details = apimodels.TaskEndDetail{
		Status: evergreen.TaskFailed,
		Type:   evergreen.CommandTypeSystem,
	}

	event.LogTaskFinished(t.Id, t.Execution, t.HostId, evergreen.TaskSystemFailed)

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

// SetAborted sets the abort field of task to aborted
func (t *Task) SetAborted() error {
	t.Aborted = true
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				AbortedKey: true,
			},
		},
	)
}

// ActivateTask will set the ActivatedBy field to the caller and set the active state to be true
func (t *Task) ActivateTask(caller string) error {
	t.ActivatedBy = caller
	t.Activated = true
	t.ActivatedTime = time.Now()
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				ActivatedKey:     true,
				ActivatedByKey:   caller,
				ActivatedTimeKey: t.ActivatedTime,
			},
		})
}

// DeactivateTask will set the ActivatedBy field to the caller and set the active state to be false and deschedule the task
func (t *Task) DeactivateTask(caller string) error {
	t.ActivatedBy = caller
	t.Activated = false
	t.ScheduledTime = util.ZeroTime
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				ActivatedKey:     false,
				ScheduledTimeKey: util.ZeroTime,
			},
		},
	)
}

// MarkEnd handles the Task updates associated with ending a task. If the task's start time is zero
// at this time, it will set it to the finish time minus the timeout time.
func (t *Task) MarkEnd(finishTime time.Time, detail *apimodels.TaskEndDetail) error {
	// record that the task has finished, in memory and in the db
	t.Status = detail.Status
	t.FinishTime = finishTime

	// if there is no start time set, either set it to the create time
	// or set 2 hours previous to the finish time.
	if util.IsZeroTime(t.StartTime) {
		timedOutStart := finishTime.Add(-2 * time.Hour)
		t.StartTime = timedOutStart
		if timedOutStart.Before(t.IngestTime) {
			t.StartTime = t.IngestTime
		}
		grip.Warning(message.Fields{
			"message":      "Task is missing start time",
			"task_id":      t.Id,
			"execution":    t.Execution,
			"requester":    t.Requester,
			"activated_by": t.ActivatedBy,
		})
	}

	t.TimeTaken = finishTime.Sub(t.StartTime)
	t.Details = *detail

	grip.Debug(message.Fields{
		"message":   "marking task finished",
		"task_id":   t.Id,
		"execution": t.Execution,
		"project":   t.Project,
		"details":   t.Details,
	})
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				FinishTimeKey: finishTime,
				StatusKey:     detail.Status,
				TimeTakenKey:  t.TimeTaken,
				DetailsKey:    t.Details,
				StartTimeKey:  t.StartTime,
				LogsKey:       detail.Logs,
			},
			"$unset": bson.M{
				AbortedKey: "",
			},
		})

}

func displayTaskPriority(status string) int {
	switch status {
	case evergreen.TaskStarted:
		return 10
	case evergreen.TaskUndispatched:
		return 40
	case evergreen.TaskFailed:
		return 50
	case evergreen.TaskTestTimedOut:
		return 60
	case evergreen.TaskSystemFailed:
		return 70
	case evergreen.TaskSystemTimedOut:
		return 80
	case evergreen.TaskSystemUnresponse:
		return 90
	case evergreen.TaskSetupFailed:
		return 95
	case evergreen.TaskSucceeded:
		return 100
	case evergreen.TaskInactive:
		return 110
	}
	return 1000
}

// Reset sets the task state to be activated, with a new secret,
// undispatched status and zero time on Start, Scheduled, Dispatch and FinishTime
func (t *Task) Reset() error {

	if t.DisplayOnly {
		for _, et := range t.ExecutionTasks {
			execTask, err := FindOne(ById(et))
			if err != nil {
				return errors.Wrap(err, "error retrieving execution task")
			}
			if err = execTask.Reset(); err != nil {
				return errors.Wrap(err, "error resetting execution task")
			}
		}
	}

	t.Activated = true
	t.Secret = util.RandomString()
	t.DispatchTime = util.ZeroTime
	t.StartTime = util.ZeroTime
	t.ScheduledTime = util.ZeroTime
	t.FinishTime = util.ZeroTime
	t.ResetWhenFinished = false
	reset := bson.M{
		"$set": bson.M{
			ActivatedKey:     true,
			SecretKey:        t.Secret,
			StatusKey:        evergreen.TaskUndispatched,
			DispatchTimeKey:  util.ZeroTime,
			StartTimeKey:     util.ZeroTime,
			ScheduledTimeKey: util.ZeroTime,
			FinishTimeKey:    util.ZeroTime,
		},
		"$unset": bson.M{
			DetailsKey:           "",
			ResetWhenFinishedKey: "",
		},
	}

	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		reset,
	)
}

// Reset sets the task state to be activated, with a new secret,
// undispatched status and zero time on Start, Scheduled, Dispatch and FinishTime
func ResetTasks(taskIds []string) error {
	tasks, err := FindWithDisplayTasks(ByIds(taskIds))
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.DisplayOnly {
			taskIds = append(taskIds, t.Id)
		}
	}

	reset := bson.M{
		"$set": bson.M{
			ActivatedKey:     true,
			SecretKey:        util.RandomString(),
			StatusKey:        evergreen.TaskUndispatched,
			DispatchTimeKey:  util.ZeroTime,
			StartTimeKey:     util.ZeroTime,
			ScheduledTimeKey: util.ZeroTime,
			FinishTimeKey:    util.ZeroTime,
		},
		"$unset": bson.M{
			DetailsKey: "",
		},
	}

	_, err = UpdateAll(
		bson.M{
			IdKey: bson.M{"$in": taskIds},
		},
		reset,
	)

	return err
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

// SetPriority sets the priority of the tasks and the tasks that they depend on
func (t *Task) SetPriority(priority int64, user string) error {
	t.Priority = priority
	modifier := bson.M{PriorityKey: priority}

	//blacklisted - this task should never run, so unschedule it now
	if priority < 0 {
		modifier[ActivatedKey] = false
	}

	ids, err := t.getRecursiveDependencies()
	if err != nil {
		return errors.Wrap(err, "error getting task dependencies")
	}
	ids = append(ids, t.ExecutionTasks...)

	_, err = UpdateAll(
		bson.M{"$or": []bson.M{
			{IdKey: t.Id},
			{IdKey: bson.M{"$in": ids},
				PriorityKey: bson.M{"$lt": priority}},
		}},
		bson.M{"$set": modifier},
	)

	event.LogTaskPriority(t.Id, t.Execution, user, priority)

	return errors.WithStack(err)

}

// getRecursiveDependencies creates a slice containing t.Id and the Ids of all recursive dependencies.
// We assume there are no dependency cycles.
func (t *Task) getRecursiveDependencies() ([]string, error) {
	recurIds := make([]string, 0, len(t.DependsOn))
	for _, dependency := range t.DependsOn {
		recurIds = append(recurIds, dependency.TaskId)
	}

	recurTasks, err := Find(ByIds(recurIds))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ids := make([]string, 0)
	for _, recurTask := range recurTasks {
		appendIds, err := recurTask.getRecursiveDependencies()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ids = append(ids, appendIds...)
	}

	ids = append(ids, t.Id)
	return ids, nil
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
		docs[idx] = result.convertToNewStyleTestResult(t.Id, t.Execution)
	}

	return errors.Wrap(testresult.InsertMany(docs), "error inserting into testresults collection")
}

func (t TestResult) convertToNewStyleTestResult(id string, execution int) testresult.TestResult {
	return testresult.TestResult{
		TaskID:    id,
		Execution: execution,
		Status:    t.Status,
		TestFile:  t.TestFile,
		URL:       t.URL,
		URLRaw:    t.URLRaw,
		LogID:     t.LogId,
		LineNum:   t.LineNum,
		ExitCode:  t.ExitCode,
		StartTime: t.StartTime,
		EndTime:   t.EndTime,
	}
}

func ConvertToOld(in *testresult.TestResult) TestResult {
	return TestResult{
		Status:    in.Status,
		TestFile:  in.TestFile,
		URL:       in.URL,
		URLRaw:    in.URLRaw,
		LogId:     in.LogID,
		LineNum:   in.LineNum,
		ExitCode:  in.ExitCode,
		StartTime: in.StartTime,
		EndTime:   in.EndTime,
		LogRaw:    in.LogRaw,
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

// SetCost updates the task's Cost field
func (t *Task) SetCost(cost float64) error {
	t.Cost = cost
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				CostKey: cost,
			},
		},
	)
}

func IncSpawnedHostCost(taskID string, cost float64) error {
	return UpdateOne(
		bson.M{
			IdKey: taskID,
		},
		bson.M{
			"$inc": bson.M{
				SpawnedHostCostKey: cost,
			},
		},
	)
}

// AbortBuild sets the abort flag on all tasks associated with the build which are in an abortable
// state
func AbortBuild(buildId, caller string) error {
	_, err := UpdateAll(
		bson.M{
			BuildIdKey: buildId,
			StatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
		},
		bson.M{"$set": bson.M{AbortedKey: true}},
	)
	if err != nil {
		return errors.Wrap(err, "error setting aborted statuses")
	}
	ids, err := FindAllTaskIDsFromBuild(buildId)
	if err != nil {
		return errors.Wrap(err, "error finding tasks by build id")
	}
	if len(ids) > 0 {
		event.LogManyTaskAbortRequests(ids, caller)
	}
	return nil
}

//String represents the stringified version of a task
func (t *Task) String() (taskStruct string) {
	taskStruct += fmt.Sprintf("Id: %v\n", t.Id)
	taskStruct += fmt.Sprintf("Status: %v\n", t.Status)
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

// Insert writes the b to the db.
func (t *Task) Insert() error {
	return db.Insert(Collection, t)
}

// Inserts the task into the old_tasks collection
func (t *Task) Archive() error {
	var update bson.M
	if t.DisplayOnly {
		for _, et := range t.ExecutionTasks {
			execTask, err := FindOne(ById(et))
			if err != nil {
				return errors.Wrap(err, "error retrieving execution task")
			}
			if execTask == nil {
				return errors.Errorf("unable to find execution task %s from display task %s", et, t.Id)
			}
			if err = execTask.Archive(); err != nil {
				return errors.Wrap(err, "error archiving execution task")
			}
		}
	}
	t0 := time.Now()

	// only increment restarts if have a current restarts
	// this way restarts will never be set for new tasks but will be
	// maintained for old ones
	if t.Restarts > 0 {
		update = bson.M{"$inc": bson.M{
			ExecutionKey: 1,
			RestartsKey:  1,
		}}
	} else {
		update = bson.M{
			"$inc": bson.M{ExecutionKey: 1},
		}
	}
	err := UpdateOne(
		bson.M{IdKey: t.Id},
		update)
	if err != nil {
		return errors.Wrap(err, "task.Archive() failed")
	}

	grip.Info(message.Fields{
		"message":   "Time to update executions/restarts",
		"modify_by": "restart",
		"versionId": t.Id,
		"duration":  time.Since(t0).String(),
	})
	t0 = time.Now()

	archiveTask := *t
	archiveTask.Id = fmt.Sprintf("%v_%v", t.Id, t.Execution)
	archiveTask.OldTaskId = t.Id
	archiveTask.Archived = true
	err = db.Insert(OldCollection, &archiveTask)
	if err != nil {
		return errors.Wrap(err, "task.Archive() failed")
	}

	grip.Info(message.Fields{
		"message":   "Time to insert task",
		"modify_by": "restart",
		"versionId": t.Id,
		"duration":  time.Since(t0).String(),
	})
	t0 = time.Now()

	err = event.UpdateExecutions(t.HostId, t.Id, t.Execution)
	if err != nil {
		return errors.Wrap(err, "unable to update host event logs")
	}
	grip.Info(message.Fields{
		"message":   "Time to update archived task",
		"modify_by": "restart",
		"versionId": t.Id,
		"duration":  time.Since(t0).String(),
	})
	return nil
}

// Aggregation

// AverageTaskTimeDifference takes two field names (such that field2 happened
// after field1), a field to group on, and a cutoff time.
// It returns the average duration between fields 1 and 2, grouped by
// the groupBy field, including only task documents where both time
// fields happened after the given cutoff time. This information is returned
// as a map from groupBy_field -> avg_time_difference
//
// NOTE: THIS FUNCTION DOES NOT SANITIZE INPUT!
// BAD THINGS CAN HAPPEN IF NON-TIME FIELDNAMES ARE PASSED IN
// OR IF A FIELD OF NON-STRING TYPE IS SUPPLIED FOR groupBy!
func AverageTaskTimeDifference(field1 string, field2 string,
	groupByField string, cutoff time.Time) (map[string]time.Duration, error) {

	// This pipeline returns the average time difference between
	// two time fields, grouped by a given field of "string" type.
	// It assumes field2 happened later than field1.
	// Time difference returned in milliseconds.
	pipeline := []bson.M{
		{"$match": bson.M{
			field1: bson.M{"$gt": cutoff},
			field2: bson.M{"$gt": cutoff}}},
		{"$group": bson.M{
			"_id": "$" + groupByField,
			"avg_time": bson.M{
				"$avg": bson.M{
					"$subtract": []string{"$" + field2, "$" + field1},
				},
			},
		}},
	}

	// anonymous struct for unmarshalling result bson
	// NOTE: This means we can only group by string fields currently
	var results []struct {
		GroupId     string `bson:"_id"`
		AverageTime int64  `bson:"avg_time"`
	}

	err := db.Aggregate(Collection, pipeline, &results)
	if err != nil {
		return nil, errors.Wrapf(err, "Error aggregating task times by [%v, %v]", field1, field2)
	}

	avgTimes := make(map[string]time.Duration)
	for _, res := range results {
		avgTimes[res.GroupId] = time.Duration(res.AverageTime) * time.Millisecond
	}

	return avgTimes, nil
}

// MergeNewTestResults returns the task with both old (embedded in
// the tasks collection) and new (from the testresults collection) test results
// merged in the Task's LocalTestResults field.
func (t *Task) MergeNewTestResults() error {
	id := t.Id
	if t.Archived {
		id = t.OldTaskId
	}
	newTestResults, err := testresult.FindByTaskIDAndExecution(id, t.Execution)
	if err != nil {
		return errors.Wrap(err, "problem finding test results")
	}
	for _, result := range newTestResults {
		t.LocalTestResults = append(t.LocalTestResults, TestResult{
			Status:    result.Status,
			TestFile:  result.TestFile,
			URL:       result.URL,
			URLRaw:    result.URLRaw,
			LogId:     result.LogID,
			LineNum:   result.LineNum,
			ExitCode:  result.ExitCode,
			StartTime: result.StartTime,
			EndTime:   result.EndTime,
		})
	}
	return nil
}

// GetTestResultsForDisplayTask returns the test results for the execution tasks
// for a display task.
func (t *Task) GetTestResultsForDisplayTask() ([]TestResult, error) {
	if !t.DisplayOnly {
		return nil, errors.Errorf("%s is not a display task", t.Id)
	}
	tasks, err := MergeTestResultsBulk([]Task{*t}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error merging test results for display task")
	}
	return tasks[0].LocalTestResults, nil
}

// SetResetWhenFinished requests that a display task reset itself when finished. Will mark itself as system failed
func (t *Task) SetResetWhenFinished() error {
	if !t.DisplayOnly {
		return errors.Errorf("%s is not a display task", t.Id)
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

// MergeTestResultsBulk takes a slice of task structs and returns the slice with
// test results populated. Note that the order may change. The second parameter
// can be used to use a specific test result filtering query, otherwise all test
// results for the passed in tasks will be merged. Display tasks will have
// the execution task results merged
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
			if (result.TaskID == t.Id || util.StringSliceContains(t.ExecutionTasks, result.TaskID)) && result.Execution == t.Execution {
				t.LocalTestResults = append(t.LocalTestResults, ConvertToOld(&result))
			}
		}
		out = append(out, t)
	}

	return out, nil
}

func FindSchedulable(distroID string) ([]Task, error) {
	query := scheduleableTasksQuery()

	if distroID == "" {
		return Find(db.Query(query))
	}

	query[DistroIdKey] = distroID
	return Find(db.Query(query))
}

func FindRunnable(distroID string) ([]Task, error) {
	expectedStatuses := []string{evergreen.TaskSucceeded, evergreen.TaskFailed, ""}

	match := scheduleableTasksQuery()
	if distroID != "" {
		match[DistroIdKey] = distroID

	}

	matchActivatedUndispatchedTasks := bson.M{
		"$match": match,
	}

	graphLookupTaskDeps := bson.M{
		"$graphLookup": bson.M{
			"from":             Collection,
			"startWith":        "$" + DependsOnKey + "." + IdKey,
			"connectFromField": DependsOnKey + "." + IdKey,
			"connectToField":   IdKey,
			"as":               edgesKey,
			// restrict graphLookup to only direct dependencies
			"maxDepth": 0,
			"restrictSearchWithMatch": bson.M{
				StatusKey: bson.M{
					"$in": expectedStatuses,
				},
			},
		},
	}

	reshapeTasksAndEdges := bson.M{
		"$project": bson.M{
			edgesKey + "." + IdKey:     1,
			edgesKey + "." + StatusKey: 1,
			taskKey:                    "$$ROOT",
		},
	}

	removeEdgesFromTask := bson.M{
		"$project": bson.M{
			taskKey + "." + edgesKey: 0,
		},
	}

	redactUnrunnableTasks := bson.M{
		"$redact": bson.M{
			"$cond": bson.M{
				"if": bson.M{
					"$setEquals": []string{"$" + taskKey + "." + DependsOnKey, "$" + edgesKey},
				},
				"then": "$$KEEP",
				"else": "$$PRUNE",
			},
		},
	}

	replaceRoot := bson.M{
		"$replaceRoot": bson.M{
			"newRoot": "$" + taskKey,
		},
	}

	joinProjectRef := bson.M{
		"$lookup": bson.M{
			"from":         "project_ref",
			"localField":   ProjectKey,
			"foreignField": "identifier",
			"as":           "project_ref",
		},
	}

	filterDisabledProejcts := bson.M{
		"$match": bson.M{
			"project_ref.0." + "enabled": true,
		},
	}

	filterPatchingDisabledProjects := bson.M{
		"$match": bson.M{"$or": []bson.M{
			{
				RequesterKey: bson.M{"$nin": evergreen.PatchRequesters},
			},
			{
				"project_ref.0." + "patching_disabled": false,
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
		graphLookupTaskDeps,
		reshapeTasksAndEdges,
		removeEdgesFromTask,
		redactUnrunnableTasks,
		replaceRoot,
		joinProjectRef,
		filterDisabledProejcts,
		filterPatchingDisabledProjects,
		removeProjectRef,
	}

	runnableTasks := []Task{}
	if err := Aggregate(pipeline, &runnableTasks); err != nil {
		return nil, errors.Wrap(err, "failed to fetch runnable tasks")
	}

	return runnableTasks, nil
}

func (t *Task) IsPartOfDisplay() bool {
	dt, err := t.GetDisplayTask()
	if err != nil {
		grip.Error(err)
		return false
	}
	return dt != nil
}

func (t *Task) GetDisplayTask() (*Task, error) {
	if t.DisplayTask != nil {
		return t, nil
	}
	dt, err := FindOne(ByExecutionTask(t.Id))
	if err != nil {
		return nil, err
	}
	t.DisplayTask = dt
	return dt, nil
}

func (t *Task) GetHistoricRuntime() (time.Duration, error) {
	runtimes, err := getExpectedDurationsForWindow(t.DisplayName, t.Project, t.BuildVariant, t.FinishTime.Add(-oneMonthIsh), t.FinishTime.Add(-time.Second))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	if len(runtimes) != 1 {
		return 0, errors.Errorf("got unexpected task runtimes data points (%d)", len(runtimes))
	}

	return time.Duration(runtimes[0].ExpectedDuration), nil
}

func (t *Task) FetchExpectedDuration() time.Duration {
	if t.DurationPrediction.TTL == 0 {
		t.DurationPrediction.TTL = util.JitterInterval(predictionTTL)
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

		return t.ExpectedDuration
	}

	grip.Debug(message.WrapError(t.DurationPrediction.SetRefresher(func(previous time.Duration) (time.Duration, bool) {
		vals, err := getExpectedDurationsForWindow(t.DisplayName, t.Project, t.BuildVariant, time.Now().Add(-taskCompletionEstimateWindow), time.Now())
		grip.Notice(message.WrapError(err, message.Fields{
			"name":      t.DisplayName,
			"id":        t.Id,
			"project":   t.Project,
			"variant":   t.BuildVariant,
			"operation": "fetching expected duration, expect stale scheduling data",
		}))
		if err != nil {
			return defaultTaskDuration, false
		}

		if len(vals) != 1 {
			if previous == 0 {
				return defaultTaskDuration, true
			}

			return previous, true
		}

		ret := time.Duration(vals[0].ExpectedDuration)
		if ret == 0 {
			return defaultTaskDuration, true
		}
		return ret, true
	}), message.Fields{
		"message": "problem setting cached value refresher",
		"cause":   "programmer error",
	}))

	expectedDuration, ok := t.DurationPrediction.Get()
	if ok {
		if err := t.cacheExpectedDuration(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"task":    t.Id,
				"message": "caching expected duration",
			}))
		}
	}

	return expectedDuration
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
		if statusDetails.TimedOut && statusDetails.Description == "heartbeat" {
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

// BlockedState returns "blocked," "pending" (unsatisfied dependencies,
// but unblocked), or "" (runnable) to represent the state of the task
// with respect to its dependencies
func (t *Task) BlockedState(tasksWithDeps []Task) (string, error) {
	if t.DisplayOnly {
		return t.blockedStateForDisplayTask(tasksWithDeps)
	}
	if len(t.DependsOn) == 0 {
		return "", nil
	}
	return t.blockedStatePrivate()
}

func (t *Task) blockedStatePrivate() (string, error) {
	if len(t.DependsOn) == 0 {
		return "", nil
	}
	dependencyIDs := []string{}
	for _, d := range t.DependsOn {
		dependencyIDs = append(dependencyIDs, d.TaskId)
	}
	dependentTasks, err := Find(ByIds(dependencyIDs).WithFields(DisplayNameKey, StatusKey,
		ActivatedKey, BuildVariantKey, DetailsKey, DependsOnKey))
	if err != nil {
		return "", errors.Wrap(err, "error finding dependencies")
	}
	taskMap := map[string]*Task{}
	for i := range dependentTasks {
		taskMap[dependentTasks[i].Id] = &dependentTasks[i]
	}
	for _, dependency := range t.DependsOn {
		depTask := taskMap[dependency.TaskId]
		state, err := depTask.blockedStatePrivate()
		if err != nil {
			return "", err
		}
		if state == taskBlocked {
			return taskBlocked, nil
		} else if depTask.Status == evergreen.TaskSucceeded || depTask.Status == evergreen.TaskFailed {
			if depTask.Status != dependency.Status && dependency.Status != AllStatuses {
				return taskBlocked, nil
			}
		} else {
			return taskPending, nil
		}
	}
	return "", nil
}

func (t *Task) blockedStateForDisplayTask(tasksWithDeps []Task) (string, error) {
	execTasks, err := Find(ByIds(t.ExecutionTasks))
	if err != nil {
		return "", errors.Wrap(err, "error finding execution tasks")
	}
	if len(execTasks) == 0 {
		return "", nil
	}
	state := ""
	for _, execTask := range execTasks {
		etState, err := execTask.BlockedState(tasksWithDeps)
		if err != nil {
			return "", errors.Wrap(err, "error finding blocked state")
		}
		if etState == taskBlocked {
			return taskBlocked, nil
		} else if etState == taskPending {
			state = taskPending
		}
	}
	return state, nil
}

func (t *Task) CircularDependencies() error {
	var err error
	tasksWithDeps, err := FindAllTasksFromVersionWithDependencies(t.Version)
	if err != nil {
		return errors.Wrap(err, "error finding tasks with dependencies")
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
			catcher.Add(errors.Errorf("Dependency cycle detected: %s", strings.Join(cycle, ",")))
		}
	}
	return catcher.Resolve()
}

func (t *Task) IsBlockedDisplayTask() bool {
	if !t.DisplayOnly {
		return false
	}

	tasksWithDeps, err := FindAllTasksFromVersionWithDependencies(t.Version)
	if err != nil {
		grip.Error(message.WrapError(err, "error finding tasks with dependencies"))
		return false
	}
	blockedState, err := t.BlockedState(tasksWithDeps)
	if err != nil {
		grip.Error(message.WrapError(err, "error determining blocked state"))
		return false
	}
	return blockedState == taskBlocked
}
