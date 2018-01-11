package task

import (
	"fmt"
	"sort"
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
	"gopkg.in/mgo.v2/bson"
)

const (
	edgesKey = "edges"
	taskKey  = "task"

	// tasks should be unscheduled after ~2 weeks
	unschedulableThreshold = 2 * 7 * 24 * time.Hour
)

var (
	AgentHeartbeat = "heartbeat"
)

type Task struct {
	Id     string `bson:"_id" json:"id"`
	Secret string `bson:"secret" json:"secret"`

	// time information for task
	// create - the time we created this task in our database
	// dispatch - the time the task runner starts up the agent on the host
	// push - the time the commit generating this build was pushed to the remote
	// scheduled - the time the commit is scheduled
	// start - the time the agent starts the task on the host after spinning it up
	// finish - the time the task was completed on the remote host
	CreateTime    time.Time `bson:"create_time" json:"create_time"`
	DispatchTime  time.Time `bson:"dispatch_time" json:"dispatch_time"`
	PushTime      time.Time `bson:"push_time" json:"push_time"`
	ScheduledTime time.Time `bson:"scheduled_time" json:"scheduled_time"`
	StartTime     time.Time `bson:"start_time" json:"start_time"`
	FinishTime    time.Time `bson:"finish_time" json:"finish_time"`

	Version  string `bson:"version" json:"version,omitempty"`
	Project  string `bson:"branch" json:"branch,omitempty"`
	Revision string `bson:"gitspec" json:"gitspec"`
	Priority int64  `bson:"priority" json:"priority"`

	// only relevant if the task is running.  the time of the last heartbeat
	// sent back by the agent
	LastHeartbeat time.Time `bson:"last_heartbeat"`

	// used to indicate whether task should be scheduled to run
	Activated     bool         `bson:"activated" json:"activated"`
	ActivatedBy   string       `bson:"activated_by" json:"activated_by"`
	BuildId       string       `bson:"build_id" json:"build_id"`
	DistroId      string       `bson:"distro" json:"distro"`
	BuildVariant  string       `bson:"build_variant" json:"build_variant"`
	DependsOn     []Dependency `bson:"depends_on" json:"depends_on"`
	NumDependents int          `bson:"num_dependents,omitempty" json:"num_dependents,omitempty"`

	// Human-readable name
	DisplayName string `bson:"display_name" json:"display_name"`

	// Tags that describe the task
	Tags []string `bson:"tags,omitempty" json:"tags,omitempty"`

	// The host the task was run on
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

	// how long we expect the task to take from start to finish
	ExpectedDuration time.Duration `bson:"expected_duration,omitempty" json:"expected_duration,omitempty"`

	// an estimate of what the task cost to run, hidden from JSON views for now
	Cost float64 `bson:"cost,omitempty" json:"-"`

	// test results embedded from the testresults collection
	LocalTestResults []TestResult `bson:"-" json:"test_results"`

	// display task fields
	DisplayOnly    bool     `bson:"display_only,omitempty" json:"display_only,omitempty"`
	ExecutionTasks []string `bson:"execution_tasks,omitempty" json:"execution_tasks,omitempty"`
	DisplayTask    *Task    `bson:"-" json:"-"` // this is a local pointer from an exec to display task
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
func IsFinished(t Task) bool {
	return t.Status == evergreen.TaskFailed ||
		t.Status == evergreen.TaskSucceeded ||
		(t.Status == evergreen.TaskUndispatched && !util.IsZeroTime(t.DispatchTime)) ||
		t.Status == evergreen.TaskSystemFailed ||
		t.Status == evergreen.TaskSystemTimedOut ||
		t.Status == evergreen.TaskSystemUnresponse ||
		t.Status == evergreen.TaskTestTimedOut
}

// IsDispatchable return true if the task should be dispatched
func (t *Task) IsDispatchable() bool {
	return t.Status == evergreen.TaskUndispatched && t.Activated
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

// Checks whether the dependencies for the task have all completed successfully.
// If any of the dependencies exist in the map that is passed in, they are
// used to check rather than fetching from the database. All queries
// are cached back into the map for later use.
func (t *Task) DependenciesMet(depCaches map[string]Task) (bool, error) {

	if len(t.DependsOn) == 0 {
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

// Find the previously completed task for the same requester + project +
// build variant + display name combination as the specified task
func (t *Task) PreviousCompletedTask(project string,
	statuses []string) (*Task, error) {
	if len(statuses) == 0 {
		statuses = CompletedStatuses
	}
	return FindOneNoMerge(ByBeforeRevisionWithStatuses(t.RevisionOrderNumber, statuses, t.BuildVariant,
		t.DisplayName, project))
}

// SetExpectedDuration updates the expected duration field for the task
func (t *Task) SetExpectedDuration(duration time.Duration) error {
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				ExpectedDurationKey: duration,
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
	return UpdateOne(
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
			event.LogTaskScheduled(t.Id, scheduledTime)
		}
	}
	return nil

}

// Removes tasks older than the unscheduable threshold (e.g. two
// weeks) from the scheduler queue.
func UnscheduleStaleUnderwaterTasks() (int, error) {
	query := scheduleableTasksQuery()
	query[PriorityKey] = 0
	query["$and"] = []bson.M{
		{CreateTimeKey: bson.M{"$lte": time.Now().Add(-unschedulableThreshold)}},
		{CreateTimeKey: bson.M{"$gt": util.ZeroTime}},
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
	return UpdateOne(bson.M{
		IdKey: t.Id,
	},
		bson.M{
			"$set": bson.M{
				ActivatedKey:   true,
				ActivatedByKey: caller,
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
		if timedOutStart.Before(t.CreateTime) {
			t.StartTime = t.CreateTime
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
			},
			"$unset": bson.M{
				AbortedKey: "",
			},
		})

}

func (t *Task) UpdateDisplayTask() error {
	if !t.DisplayOnly {
		return fmt.Errorf("%s is not a display task", t.Id)
	}

	statuses := []string{}
	var timeTaken time.Duration
	var status string
	execTasks, err := Find(ByIds(t.ExecutionTasks))
	if err != nil {
		return errors.Wrap(err, "error retrieving execution tasks")
	}
	hasFinishedTasks := false
	hasUnfinishedTasks := false
	startTime := time.Unix(1<<62, 0)
	endTime := util.ZeroTime
	for _, execTask := range execTasks {
		// if any of the execution tasks are scheduled, the display task is too
		if execTask.Activated {
			t.Activated = true
		}

		if IsFinished(execTask) {
			hasFinishedTasks = true
		} else if execTask.IsDispatchable() {
			hasUnfinishedTasks = true
		}

		// the display task's status will be the highest priority of its exec tasks
		statuses = append(statuses, execTask.ResultStatus())

		// add up the duration of the execution tasks as the cumulative time taken
		timeTaken += execTask.TimeTaken

		// set the start/end time of the display task as the earliest/latest task
		if execTask.StartTime.Before(startTime) {
			startTime = execTask.StartTime
		}
		if execTask.FinishTime.After(endTime) {
			endTime = execTask.FinishTime
		}
	}

	if hasFinishedTasks && hasUnfinishedTasks {
		// if the display task has a mix of finished and unfinished tasks, the status
		// will be "started"
		status = evergreen.TaskStarted
	} else if len(statuses) > 0 {
		// the status of the display task will be the status of its constituent task
		// that is logically the most exclusive
		sort.Sort(byPriority(statuses))
		status = statuses[0]
	}

	update := bson.M{
		StatusKey:    status,
		ActivatedKey: t.Activated,
		TimeTakenKey: timeTaken,
	}
	if startTime != time.Unix(1<<62, 0) {
		update[StartTimeKey] = startTime
	}
	if endTime != util.ZeroTime {
		update[FinishTimeKey] = endTime
	}

	err = UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": update,
		})
	if err != nil {
		return errors.Wrap(err, "error updating display task")
	}

	t.Status = status
	t.TimeTaken = timeTaken
	return nil
}

type byPriority []string

func (p byPriority) Len() int {
	return len(p)
}

func (p byPriority) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p byPriority) Less(i, j int) bool {
	return displayTaskPriority(p[i]) < displayTaskPriority(p[j])
}

func displayTaskPriority(status string) int {
	switch status {
	case evergreen.TaskStarted:
		return 10
	case evergreen.TaskInactive:
		return 20
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
	case evergreen.TaskSucceeded:
		return 100
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
			DetailsKey: "",
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
	tasks, err := Find(ByIds(taskIds))
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

	_, err = UpdateAll(
		bson.M{"$or": []bson.M{
			{IdKey: t.Id},
			{IdKey: bson.M{"$in": ids},
				PriorityKey: bson.M{"$lt": priority}},
		}},
		bson.M{"$set": modifier},
	)

	event.LogTaskPriority(t.Id, user, priority)

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
				StatusKey:    evergreen.TaskStarted,
				StartTimeKey: startTime,
			},
		},
	)
}

// SetResults sets the results of the task in LocalTestResults
func (t *Task) SetResults(results []TestResult) error {
	catcher := grip.NewSimpleCatcher()
	var testResult testresult.TestResult
	for _, result := range results {
		testResult = result.convertToNewStyleTestResult()
		catcher.Add(testResult.InsertByTaskIDAndExecution(t.Id, t.Execution))
	}
	return errors.Wrap(catcher.Resolve(), "error inserting into testresults collection")
}

func (t TestResult) convertToNewStyleTestResult() testresult.TestResult {
	return testresult.TestResult{
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

// AbortBuild sets the abort flag on all tasks associated with the build which are in an abortable
// state
func AbortBuild(buildId string) error {
	_, err := UpdateAll(
		bson.M{
			BuildIdKey: buildId,
			StatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
		},
		bson.M{"$set": bson.M{AbortedKey: true}},
	)
	return errors.WithStack(err)
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
	taskStruct += fmt.Sprintf("Requester: %v\n", t.FinishTime)
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
			if err = execTask.Archive(); err != nil {
				return errors.Wrap(err, "error archiving execution task")
			}
		}
	}

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
	archiveTask := *t
	archiveTask.Id = fmt.Sprintf("%v_%v", t.Id, t.Execution)
	archiveTask.OldTaskId = t.Id
	archiveTask.Archived = true
	err = db.Insert(OldCollection, &archiveTask)
	if err != nil {
		return errors.Wrap(err, "task.Archive() failed")
	}
	err = event.UpdateExecutions(t.HostId, t.Id, t.Execution)
	if err != nil {
		return errors.Wrap(err, "unable to update host event logs")
	}
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

// ExpectedTaskDuration takes a given project and buildvariant and computes
// the average duration - grouped by task display name - for tasks that have
// completed within a given threshold as determined by the window
func ExpectedTaskDuration(project, buildvariant string, window time.Duration) (map[string]time.Duration, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				BuildVariantKey: buildvariant,
				ProjectKey:      project,
				StatusKey: bson.M{
					"$in": []string{evergreen.TaskSucceeded, evergreen.TaskFailed},
				},
				DetailsKey + "." + TaskEndDetailTimedOut: bson.M{
					"$ne": true,
				},
				FinishTimeKey: bson.M{
					"$gte": time.Now().Add(-window),
				},
				StartTimeKey: bson.M{
					// make sure all documents have a valid start time so we don't
					// return tasks with runtimes of multiple years
					"$gt": util.ZeroTime,
				},
			},
		},
		{
			"$project": bson.M{
				DisplayNameKey: 1,
				TimeTakenKey:   1,
				IdKey:          0,
			},
		},
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%v", DisplayNameKey),
				"exp_dur": bson.M{
					"$avg": fmt.Sprintf("$%v", TimeTakenKey),
				},
			},
		},
	}

	// anonymous struct for unmarshalling result bson
	var results []struct {
		DisplayName      string `bson:"_id"`
		ExpectedDuration int64  `bson:"exp_dur"`
	}

	err := db.Aggregate(Collection, pipeline, &results)
	if err != nil {
		return nil, errors.Wrap(err, "error aggregating task average duration")
	}

	expDurations := make(map[string]time.Duration)
	for _, result := range results {
		expDuration := time.Duration(result.ExpectedDuration) * time.Nanosecond
		expDurations[result.DisplayName] = expDuration
	}

	return expDurations, nil
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

// MergeTestResultsBulk takes a slice of task structs and returns the slice with
// test results populated. Note that the order may change. The second parameter
// can be used to use a specific test result filtering query, otherwise all test
// results for the passed in tasks will be merged
func MergeTestResultsBulk(tasks []Task, query *db.Q) ([]Task, error) {
	out := []Task{}
	if query == nil {
		taskIds := []string{}
		for _, t := range tasks {
			taskIds = append(taskIds, t.Id)
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
			if result.TaskID == t.Id && result.Execution == t.Execution {
				t.LocalTestResults = append(t.LocalTestResults, ConvertToOld(&result))
			}
		}
		out = append(out, t)
	}

	return out, nil
}

func FindSchedulable() ([]Task, error) {
	return Find(db.Query(scheduleableTasksQuery()))
}

func FindRunnable() ([]Task, error) {
	expectedStatuses := []string{evergreen.TaskSucceeded, evergreen.TaskFailed, ""}

	matchActivatedUndispatchedTasks := bson.M{
		"$match": scheduleableTasksQuery(),
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
