package model

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"time"
)

const (
	TasksCollection    = "tasks"
	OldTasksCollection = "old_tasks"
	TestLogPath        = "/test_log/"
)

var (
	ZeroTime       = time.Unix(0, 0)
	AgentHeartbeat = "heartbeat"
)

type Task struct {
	Id     string `bson:"_id" json:"id"`
	Secret string `bson:"secret" json:"secret"`

	// time information for task
	// create - the time we created this task in our database
	// dispatch - it has been run to start on a remote host
	// push - the time the commit generating this build was pushed to the remote
	// start - the time the remote host it was scheduled on responded as
	//      successfully started
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
	Priority int    `bson:"priority" json:"priority"`

	// only relevant if the task is running.  the time of the last heartbeat
	// sent back by the agent
	LastHeartbeat time.Time `bson:"last_heartbeat"`

	// used to indicate whether task should be scheduled to run
	Activated    bool         `bson:"activated" json:"activated"`
	BuildId      string       `bson:"build_id" json:"build_id"`
	DistroId     string       `bson:"distro" json:"distro"`
	BuildVariant string       `bson:"build_variant" json:"build_variant"`
	DependsOn    []Dependency `bson:"depends_on" json:"depends_on"`

	// Human-readable name
	DisplayName string `bson:"display_name" json:"display_name"`

	// The host the task was run on
	HostId string `bson:"host_id" json:"host_id"`

	// the number of times this task has been restarted
	Restarts            int    `bson:"restarts" json:"restarts",omitempty`
	Execution           int    `bson:"execution" json:"execution"`
	OldTaskId           string `bson:"old_task_id,omitempty" json:"old_task_id",omitempty`
	Archived            bool   `bson:"archived,omitempty" json:"archived",omitempty`
	RevisionOrderNumber int    `bson:"order,omitempty" json:"order,omitempty"`

	// task requester - this is used to help tell the
	// reason this task was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"r"`

	// this represents the various stages the task could be in
	Status  string                  `bson:"status" json:"status"`
	Details apimodels.TaskEndDetail `bson:"details" json:"task_end_details"`
	Aborted bool                    `bson:"abort,omitempty" json:"abort"`

	// how long the task took to execute.  meaningless if the task is not finished
	TimeTaken time.Duration `bson:"time_taken" json:"time_taken"`

	// how long we expect the task to take from start to finish
	ExpectedDuration time.Duration `bson:"expected_duration,omitempty" json:"expected_duration,omitempty"`

	// test results captured and sent back by agent
	TestResults []TestResult `bson:"test_results" json:"test_results"`

	// position in queue for the queue where it's closest to the top
	MinQueuePos int `bson:"min_queue_pos" json:"min_queue_pos,omitempty"`
}

// Dependency represents a task that must be completed before the owning
// task can be scheduled.
type Dependency struct {
	TaskId string `bson:"_id" json:"id"`
	Status string `bson:"status" json:"status"`
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
	strBytes, _ := bson.Marshal(bson.RawD{{"str", raw}})
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

// TestResults is only used when transferring data from agent to api.
type TestResults struct {
	Results []TestResult `json:"results"`
}

type TestResult struct {
	Status    string  `json:"status" bson:"status"`
	TestFile  string  `json:"test_file" bson:"test_file"`
	URL       string  `json:"url" bson:"url,omitempty"`
	LogId     string  `json:"log_id,omitempty" bson:"log_id,omitempty"`
	LineNum   int     `json:"line_num,omitempty" bson:"line_num,omitempty"`
	ExitCode  int     `json:"exit_code" bson:"exit_code"`
	StartTime float64 `json:"start" bson:"start"`
	EndTime   float64 `json:"end" bson:"end"`
}

var (
	// BSON fields for the task struct
	TaskIdKey                  = bsonutil.MustHaveTag(Task{}, "Id")
	TaskSecretKey              = bsonutil.MustHaveTag(Task{}, "Secret")
	TaskCreateTimeKey          = bsonutil.MustHaveTag(Task{}, "CreateTime")
	TaskDispatchTimeKey        = bsonutil.MustHaveTag(Task{}, "DispatchTime")
	TaskPushTimeKey            = bsonutil.MustHaveTag(Task{}, "PushTime")
	TaskScheduledTimeKey       = bsonutil.MustHaveTag(Task{}, "ScheduledTime")
	TaskStartTimeKey           = bsonutil.MustHaveTag(Task{}, "StartTime")
	TaskFinishTimeKey          = bsonutil.MustHaveTag(Task{}, "FinishTime")
	TaskVersionKey             = bsonutil.MustHaveTag(Task{}, "Version")
	TaskProjectKey             = bsonutil.MustHaveTag(Task{}, "Project")
	TaskRevisionKey            = bsonutil.MustHaveTag(Task{}, "Revision")
	TaskLastHeartbeatKey       = bsonutil.MustHaveTag(Task{}, "LastHeartbeat")
	TaskActivatedKey           = bsonutil.MustHaveTag(Task{}, "Activated")
	TaskBuildIdKey             = bsonutil.MustHaveTag(Task{}, "BuildId")
	TaskDistroIdKey            = bsonutil.MustHaveTag(Task{}, "DistroId")
	TaskBuildVariantKey        = bsonutil.MustHaveTag(Task{}, "BuildVariant")
	TaskDependsOnKey           = bsonutil.MustHaveTag(Task{}, "DependsOn")
	TaskDisplayNameKey         = bsonutil.MustHaveTag(Task{}, "DisplayName")
	TaskHostIdKey              = bsonutil.MustHaveTag(Task{}, "HostId")
	TaskExecutionKey           = bsonutil.MustHaveTag(Task{}, "Execution")
	TaskRestartsKey            = bsonutil.MustHaveTag(Task{}, "Restarts")
	TaskOldTaskIdKey           = bsonutil.MustHaveTag(Task{}, "OldTaskId")
	TaskArchivedKey            = bsonutil.MustHaveTag(Task{}, "Archived")
	TaskRevisionOrderNumberKey = bsonutil.MustHaveTag(Task{}, "RevisionOrderNumber")
	TaskRequesterKey           = bsonutil.MustHaveTag(Task{}, "Requester")
	TaskStatusKey              = bsonutil.MustHaveTag(Task{}, "Status")
	TaskDetailsKey             = bsonutil.MustHaveTag(Task{}, "Details")
	TaskAbortedKey             = bsonutil.MustHaveTag(Task{}, "Aborted")
	TaskTimeTakenKey           = bsonutil.MustHaveTag(Task{}, "TimeTaken")
	TaskExpectedDurationKey    = bsonutil.MustHaveTag(Task{}, "ExpectedDuration")
	TaskTestResultsKey         = bsonutil.MustHaveTag(Task{}, "TestResults")
	TaskPriorityKey            = bsonutil.MustHaveTag(Task{}, "Priority")
	TaskMinQueuePosKey         = bsonutil.MustHaveTag(Task{}, "MinQueuePos")

	// BSON fields for the test result struct
	TestResultStatusKey    = bsonutil.MustHaveTag(TestResult{}, "Status")
	TestResultTestFileKey  = bsonutil.MustHaveTag(TestResult{}, "TestFile")
	TestResultURLKey       = bsonutil.MustHaveTag(TestResult{}, "URL")
	TestResultExitCodeKey  = bsonutil.MustHaveTag(TestResult{}, "ExitCode")
	TestResultStartTimeKey = bsonutil.MustHaveTag(TestResult{}, "StartTime")
	TestResultEndTimeKey   = bsonutil.MustHaveTag(TestResult{}, "EndTime")
)

var (
	// BSON fields for task status details struct
	TaskEndDetailStatus      = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "Status")
	TaskEndDetailTimedOut    = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "TimedOut")
	TaskEndDetailType        = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "Type")
	TaskEndDetailDescription = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "Description")
)

func (t *Task) Abortable() bool {
	return t.Status == evergreen.TaskStarted ||
		t.Status == evergreen.TaskDispatched
}

func (task Task) IsStarted() bool {
	return task.Status == evergreen.TaskStarted
}

func (task Task) IsFinished() bool {
	return task.Status == evergreen.TaskFailed ||
		task.Status == evergreen.TaskCancelled ||
		task.Status == evergreen.TaskSucceeded ||
		(task.Status == evergreen.TaskUndispatched && task.DispatchTime != ZeroTime)
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
		newDeps, err := FindAllTasks(
			bson.M{
				"_id": bson.M{
					"$in": depIdsToQueryFor,
				},
			},
			bson.M{
				"status": 1,
			},
			db.NoSort,
			db.NoSkip,
			db.NoLimit,
		)
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

/******************************************************
Find
******************************************************/

func FindOneTask(query interface{}, projection interface{},
	sort []string) (*Task, error) {
	task := &Task{}
	err := db.FindOne(
		TasksCollection,
		query,
		projection,
		sort,
		task,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return task, err
}

func FindOneOldTask(query interface{}, projection interface{},
	sort []string) (*Task, error) {
	task := &Task{}
	err := db.FindOne(
		OldTasksCollection,
		query,
		projection,
		sort,
		task,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if task != nil {
		task.Id = task.OldTaskId
	}
	if task.Id == "" {
		return nil, fmt.Errorf("old task had nil id")
	}
	return task, err
}

func FindAllTasks(query interface{}, projection interface{},
	sort []string, skip int, limit int) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAll(
		TasksCollection,
		query,
		projection,
		sort,
		skip,
		limit,
		&tasks,
	)
	return tasks, err
}

var (
	SelectorTaskInProgress = bson.M{
		"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched},
	}
)

func FindTask(id string) (*Task, error) {
	return FindOneTask(
		bson.M{
			TaskIdKey: id,
		},
		db.NoProjection,
		db.NoSort,
	)
}

// find any running tasks whose last heartbeat was at least the specified
// threshold ago
func FindTasksWithNoHeartbeatSince(threshold time.Time) ([]Task, error) {
	query := bson.M{
		TaskStatusKey:        SelectorTaskInProgress,
		TaskLastHeartbeatKey: bson.M{"$lte": threshold},
	}

	return FindAllTasks(
		query,
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

// find any tasks that are currently in progress and have been running since
// at least the specified threshold
func FindTasksRunningSince(threshold time.Time) ([]Task, error) {
	return FindAllTasks(
		bson.M{
			TaskStatusKey:       SelectorTaskInProgress,
			TaskDispatchTimeKey: bson.M{"$lte": threshold},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindInProgressTasks() ([]Task, error) {
	return FindAllTasks(
		bson.M{
			TaskStatusKey: SelectorTaskInProgress,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindTasksByIds(ids []string) (tasks []Task, err error) {
	if len(ids) == 0 {
		return
	}
	return FindAllTasks(
		bson.M{
			TaskIdKey: bson.M{
				"$in": ids,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func (t *Task) FindTaskOnBaseCommit() (*Task, error) {
	return FindOneTask(
		bson.M{
			TaskRevisionKey:     t.Revision,
			TaskRequesterKey:    evergreen.RepotrackerVersionRequester,
			TaskBuildVariantKey: t.BuildVariant,
			TaskDisplayNameKey:  t.DisplayName,
			TaskProjectKey:      t.Project,
		},
		db.NoProjection,
		db.NoSort,
	)
}

func FindUndispatchedTasks() ([]Task, error) {
	return FindAllTasks(
		bson.M{
			TaskActivatedKey: true,
			TaskStatusKey:    evergreen.TaskUndispatched,
			//Filter out blacklisted tasks
			//TODO(MCI-2245) eventually this $or should be removed as new tasks
			//will not omit the priority key.
			"$or": []bson.M{
				{TaskPriorityKey: bson.M{"$exists": false}},
				{TaskPriorityKey: bson.M{"$gte": 0}},
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func (current *Task) FindIntermediateTasks(previous *Task) ([]Task, error) {

	intermediateRevisions := bson.M{
		"$lt": current.RevisionOrderNumber,
		"$gt": previous.RevisionOrderNumber,
	}

	intermediateTasks, err := FindAllTasks(
		bson.M{
			TaskBuildVariantKey:        current.BuildVariant,
			TaskDisplayNameKey:         current.DisplayName,
			TaskRequesterKey:           current.Requester,
			TaskRevisionOrderNumberKey: intermediateRevisions,
			TaskProjectKey:             current.Project,
		},
		db.NoProjection,
		[]string{"-" + TaskRevisionOrderNumberKey},
		db.NoSkip,
		db.NoLimit,
	)

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

func (task *Task) FindPreviousTasks(limit int) ([]Task, error) {

	priorRevisions := bson.M{"$lt": task.RevisionOrderNumber}

	previousTasks, err := FindAllTasks(
		bson.M{
			TaskIdKey:                  bson.M{"$ne": task.Id},
			TaskBuildVariantKey:        task.BuildVariant,
			TaskDisplayNameKey:         task.DisplayName,
			TaskRequesterKey:           task.Requester,
			TaskRevisionOrderNumberKey: priorRevisions,
			TaskProjectKey:             task.Project,
		},
		db.NoProjection,
		[]string{"-" + TaskRevisionOrderNumberKey},
		db.NoSkip,
		limit,
	)

	if err != nil {
		return nil, err
	}

	// reverse the slice of tasks
	reversed := make([]Task, len(previousTasks))
	for idx, t := range previousTasks {
		reversed[len(previousTasks)-idx-1] = t
	}

	return reversed, nil
}

func FindTasksForBuild(b *build.Build) ([]Task, error) {
	tasks, err := FindAllTasks(
		bson.M{
			TaskBuildIdKey: b.Id,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	return tasks, err
}

// CountSimilarFailingTasks returns a count of all tasks with the same project,
// same display name, and in other buildvariants, that have failed in the same
// revision
func (task *Task) CountSimilarFailingTasks() (int, error) {
	// find similar failing tasks
	query := bson.M{
		TaskBuildVariantKey: bson.M{
			"$ne": task.BuildVariant,
		},
		TaskDisplayNameKey: task.DisplayName,
		TaskStatusKey:      evergreen.TaskFailed,
		TaskProjectKey:     task.Project,
		TaskRequesterKey:   task.Requester,
		TaskRevisionKey:    task.Revision,
	}

	return db.Count(TasksCollection, query)
}

// Find the previously completed task for the same requester + project +
// build variant + display name combination as the specified task
func PreviousCompletedTask(task *Task, project string,
	statuses []string) (*Task, error) {

	if len(statuses) == 0 {
		statuses = []string{evergreen.TaskCancelled, evergreen.TaskFailed,
			evergreen.TaskSucceeded}
	}

	priorRevisions := bson.M{
		"$lt": task.RevisionOrderNumber,
	}

	// find previously completed task
	query := bson.M{
		TaskIdKey: bson.M{
			"$ne": task.Id,
		},
		TaskRevisionOrderNumberKey: priorRevisions,
		TaskRequesterKey:           task.Requester,
		TaskDisplayNameKey:         task.DisplayName,
		TaskBuildVariantKey:        task.BuildVariant,
		TaskStatusKey: bson.M{
			"$in": statuses,
		},
		TaskProjectKey: project,
	}

	return FindOneTask(
		query,
		db.NoProjection,
		[]string{"-" + TaskRevisionOrderNumberKey},
	)
}

func RecentlyFinishedTasks(finishTime time.Time, project string,
	requester string) ([]Task, error) {

	query := bson.M{}
	andClause := []bson.M{}
	finishedOpts := []bson.M{}

	// filter by finished builds
	finishedOpts = append(finishedOpts,
		bson.M{
			TaskStatusKey: bson.M{
				"$in": []string{
					evergreen.TaskFailed,
					evergreen.TaskSucceeded,
					evergreen.TaskCancelled,
				},
			},
		},
	)

	// filter by finish_time
	timeOpt := bson.M{
		TaskFinishTimeKey: bson.M{
			"$gt": finishTime,
		},
	}

	// filter by requester
	requesterOpt := bson.M{
		TaskRequesterKey: requester,
	}

	// build query
	andClause = append(andClause, bson.M{
		"$or": finishedOpts,
	})
	andClause = append(andClause, timeOpt)
	andClause = append(andClause, requesterOpt)

	// filter by project
	if project != "" {
		projectOpt := bson.M{
			TaskProjectKey: project,
		}
		andClause = append(andClause, projectOpt)
	}

	query["$and"] = andClause

	return FindAllTasks(
		query,
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)

}

func (task *Task) FetchPatch() (*patch.Patch, error) {
	// find the patch associated with this version
	return patch.FindOne(patch.ByVersion(task.Version))
}

// SetExpectedDuration updates the expected duration field for the task
func (task *Task) SetExpectedDuration(duration time.Duration) error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: task.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskExpectedDurationKey: duration,
			},
		},
	)
}

// Given a list of host ids, return the tasks that finished running on these
// hosts. For memory usage, projection only returns each task's host id
func FindTasksForHostIds(ids []string) ([]Task, error) {
	return FindAllTasks(
		bson.M{
			TaskStatusKey: bson.M{"$in": evergreen.CompletedStatuses},
			TaskHostIdKey: bson.M{"$in": ids},
		},
		// Only return host names and requester
		bson.M{TaskHostIdKey: 1, TaskRequesterKey: 1},
		db.NoSort,
		db.NoSkip,
		db.NoLimit)
}

// Get history of tasks for a given build variant, project, and display name.
func FindCompletedTasksByVariantAndName(project string, buildVariant string,
	taskName string, limit int, beforeTaskId string) ([]Task, error) {
	query := bson.M{
		TaskBuildVariantKey: buildVariant,
		TaskDisplayNameKey:  taskName,
		TaskStatusKey:       bson.M{"$in": evergreen.CompletedStatuses},
		TaskProjectKey:      project,
	}

	if beforeTaskId != "" {
		task, err := FindTask(beforeTaskId)
		if err != nil {
			return nil, err
		}
		if task == nil {
			return nil, fmt.Errorf("Task %v not found", beforeTaskId)
		}

		query[TaskRevisionOrderNumberKey] = bson.M{"$lte": task.RevisionOrderNumber}
	}

	tasks, err := FindAllTasks(
		query,
		bson.M{
			TaskCreateTimeKey:    1,
			TaskDispatchTimeKey:  1,
			TaskPushTimeKey:      1,
			TaskScheduledTimeKey: 1,
			TaskStartTimeKey:     1,
			TaskFinishTimeKey:    1,
			TaskVersionKey:       1,
			TaskHostIdKey:        1,
			TaskStatusKey:        1,
		},
		[]string{"-" + TaskRevisionOrderNumberKey},
		0,
		limit)

	if err != nil {
		return nil, err
	}

	return tasks, nil
}

/***********************
Update
***********************/

func UpdateOneTask(query interface{}, update interface{}) error {
	return db.Update(
		TasksCollection,
		query,
		update,
	)
}

func UpdateAllTasks(query interface{}, update interface{}) (*mgo.ChangeInfo, error) {
	return db.UpdateAll(
		TasksCollection,
		query,
		update,
	)
}

// Mark that the task has been dispatched onto a particular host. Sets the
// running task field on the host and the host id field on the task, as well
// as updating the cache for the task in its build document in the db.
// Returns an error if any of the database updates fail.
func (t *Task) MarkAsDispatched(host *host.Host, dispatchTime time.Time) error {
	// then, update the task document
	t.DispatchTime = dispatchTime
	t.Status = evergreen.TaskDispatched
	t.HostId = host.Id
	t.LastHeartbeat = dispatchTime
	t.DistroId = host.Distro.Id
	err := UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskDispatchTimeKey:  dispatchTime,
				TaskStatusKey:        evergreen.TaskDispatched,
				TaskHostIdKey:        host.Id,
				TaskLastHeartbeatKey: dispatchTime,
				TaskDistroIdKey:      host.Distro.Id,
			},
			"$unset": bson.M{
				TaskAbortedKey:     "",
				TaskTestResultsKey: "",
				TaskDetailsKey:     "",
				TaskMinQueuePosKey: "",
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error updating task with id %v: %v", t.Id, err)
	}

	// the task was successfully dispatched, log the event
	event.LogTaskDispatched(t.Id, host.Id)

	// update the cached version of the task in its related build document
	if err = build.SetCachedTaskDispatched(t.BuildId, t.Id); err != nil {
		return fmt.Errorf("error updating task cache in build %v: %v", t.BuildId, err)
	}
	return nil
}

// MarkAsUndispatched marks that the task has been undispatched from a
// particular host. Unsets the running task field on the host and the
// host id field on the task, as well as updating the cache for the task
// in its build document in the db.
// Returns an error if any of the database updates fail.
func (task *Task) MarkAsUndispatched(host *host.Host) error {
	// then, update the task document
	task.Status = evergreen.TaskUndispatched
	err := UpdateOneTask(
		bson.M{
			TaskIdKey: task.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskStatusKey: evergreen.TaskUndispatched,
			},
			"$unset": bson.M{
				TaskDispatchTimeKey:  ZeroTime,
				TaskLastHeartbeatKey: ZeroTime,
				TaskDistroIdKey:      "",
				TaskHostIdKey:        "",
				TaskAbortedKey:       "",
				TaskTestResultsKey:   "",
				TaskDetailsKey:       "",
				TaskMinQueuePosKey:   "",
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error updating task with id %v: %v", task.Id, err)
	}

	// the task was successfully dispatched, log the event
	event.LogTaskUndispatched(task.Id, host.Id)

	// update the cached version of the task in its related build document
	if err = build.SetCachedTaskUndispatched(task.BuildId, task.Id); err != nil {
		return fmt.Errorf("error updating task cache in build %v: %v", task.BuildId, err)
	}
	return nil
}

// UpdateMinQueuePos takes a taskId-to-MinQueuePosition map and updates
// the min_queue_pos field in each specified task to the specified MinQueuePos
func UpdateMinQueuePos(taskIdToMinQueuePos map[string]int) error {
	for taskId, minQueuePos := range taskIdToMinQueuePos {
		err := UpdateOneTask(
			bson.M{
				TaskIdKey: taskId,
			},
			bson.M{
				"$set": bson.M{
					TaskMinQueuePosKey: minQueuePos,
				},
			},
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetTasksScheduledTime takes a list of tasks and a time, and then sets
// the scheduled time in the database for the tasks if it is currently unset
func SetTasksScheduledTime(tasks []Task, scheduledTime time.Time) error {
	if len(tasks) == 0 {
		return nil
	}
	var ids []string
	for i := range tasks {
		tasks[i].ScheduledTime = scheduledTime
		ids = append(ids, tasks[i].Id)
	}
	info, err := UpdateAllTasks(
		bson.M{
			TaskIdKey: bson.M{
				"$in": ids,
			},
			TaskScheduledTimeKey: bson.M{
				"$lte": ZeroTime,
			},
		},
		bson.M{
			"$set": bson.M{
				TaskScheduledTimeKey: scheduledTime,
			},
		},
	)

	if err != nil {
		return err
	}

	if info.Updated > 0 {
		for _, task := range tasks {
			event.LogTaskScheduled(task.Id, scheduledTime)
		}
	}
	return nil
}

func SetTaskActivated(taskId string, caller string, active bool) error {
	task, err := FindTask(taskId)
	if err != nil {
		return err
	}

	if active {

		// if the task is being activated, make sure to activate all of the task's
		// dependencies as well
		for _, dep := range task.DependsOn {
			if err = SetTaskActivated(dep.TaskId, caller, true); err != nil {
				return fmt.Errorf("error activating dependency for %v with id %v: %v",
					taskId, dep.TaskId, err)
			}
		}

		if task.DispatchTime != ZeroTime && task.Status == evergreen.TaskUndispatched {
			err = task.reset()
		} else {
			err = UpdateOneTask(
				bson.M{
					TaskIdKey: taskId,
				},
				bson.M{
					"$set": bson.M{
						TaskActivatedKey: active,
					},
					"$unset": bson.M{
						TaskMinQueuePosKey: "",
					},
				},
			)
		}
	} else {
		err = UpdateOneTask(
			bson.M{
				TaskIdKey: taskId,
			},
			bson.M{
				"$set": bson.M{
					TaskActivatedKey:     active,
					TaskScheduledTimeKey: ZeroTime,
				},
				"$unset": bson.M{
					TaskMinQueuePosKey: "",
				},
			},
		)
	}

	if err != nil {
		return err
	}

	if active {
		event.LogTaskActivated(taskId, caller)
	} else {
		event.LogTaskDeactivated(taskId, caller)
	}

	// update the cached version of the task, in its build document
	return build.SetCachedTaskActivated(task.BuildId, taskId, active)
}

func (t *Task) Abort(caller string, aborted bool) error {
	if !t.Abortable() {
		return fmt.Errorf("Task '%v' is currently '%v' - cannot abort task"+
			" in this status", t.Id, t.Status)
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Setting abort=%v for task %v", aborted, t.Id)

	err := SetTaskActivated(t.Id, caller, false)
	if err != nil {
		return err
	}

	err = UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskAbortedKey: aborted,
			},
		},
	)
	if err != nil {
		return err
	}

	event.LogTaskAbortRequest(t.Id, caller)

	t.Aborted = aborted
	return nil
}

func (t *Task) UpdateHeartbeat() error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskLastHeartbeatKey: time.Now(),
			},
		},
	)
}

func (t *Task) SetPriority(priority int) error {
	t.Priority = priority
	modifier := bson.M{TaskPriorityKey: priority}

	//blacklisted - this task should never run, so unschedule it now
	if priority < 0 {
		modifier[TaskActivatedKey] = false
	}

	ids, err := t.getRecursiveDependencies()
	if err != nil {
		return fmt.Errorf("error getting task dependencies: %v", err)
	}

	_, err = UpdateAllTasks(
		bson.M{
			TaskIdKey:       bson.M{"$in": ids},
			TaskPriorityKey: bson.M{"$lt": priority},
		},
		bson.M{"$set": modifier},
	)
	return err
}

// getRecursiveDependencies creates a slice containing t.Id and the Ids of all recursive dependencies.
// We assume there are no dependency cycles.
func (t *Task) getRecursiveDependencies() ([]string, error) {
	recurIds := make([]string, 0, len(t.DependsOn))
	for _, dependency := range t.DependsOn {
		recurIds = append(recurIds, dependency.TaskId)
	}

	recurTasks, err := FindTasksByIds(recurIds)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0)
	for _, recurTask := range recurTasks {
		appendIds, err := recurTask.getRecursiveDependencies()
		if err != nil {
			return nil, err
		}
		ids = append(ids, appendIds...)
	}

	ids = append(ids, t.Id)
	return ids, nil
}

func (t *Task) TryReset(user, origin string, p *Project, detail *apimodels.TaskEndDetail) (err error) {
	// if we've reached the max number of executions for this task, mark it as finished and failed
	if t.Execution >= evergreen.MaxTaskExecution {
		// restarting from the UI bypasses the restart cap
		message := fmt.Sprintf("Task '%v' reached max execution (%v):", t.Id, evergreen.MaxTaskExecution)
		if origin == evergreen.UIPackage {
			evergreen.Logger.Logf(slogger.DEBUG, "%v allowing exception for %v", message, user)
		} else {
			evergreen.Logger.Logf(slogger.DEBUG, "%v marking as failed", message)
			if detail != nil {
				return t.MarkEnd(origin, time.Now(), detail, p, false)
			} else {
				panic(fmt.Sprintf("TryReset called with nil TaskEndDetail by %v", origin))
			}
		}
	}

	// only allow re-execution for failed or successful tasks
	if !t.IsFinished() {
		// this is to disallow terminating running tasks via the UI
		if origin == evergreen.UIPackage {
			evergreen.Logger.Logf(slogger.DEBUG, "Unsatisfiable '%v' reset request on '%v' (status: '%v')", user, t.Id, t.Status)
			return fmt.Errorf("Task '%v' is currently '%v' - can not reset task in this status", t.Id, t.Status)
		}
	}

	if detail != nil {
		if err = t.markEnd(origin, time.Now(), detail); err != nil {
			return fmt.Errorf("Error marking task as ended: %v", err)
		}
	}

	if err = t.reset(); err == nil {
		if origin == evergreen.UIPackage {
			event.LogTaskRestarted(t.Id, user)
		} else {
			event.LogTaskRestarted(t.Id, origin)
		}
	}
	return err
}

func (t *Task) reset() error {
	if err := t.Archive(); err != nil {
		return fmt.Errorf("Can't restart task because it can't be archived: %v", err)
	}

	reset := bson.M{
		"$set": bson.M{
			TaskActivatedKey:     true,
			TaskSecretKey:        util.RandomString(),
			TaskStatusKey:        evergreen.TaskUndispatched,
			TaskDispatchTimeKey:  ZeroTime,
			TaskStartTimeKey:     ZeroTime,
			TaskScheduledTimeKey: ZeroTime,
			TaskFinishTimeKey:    ZeroTime,
			TaskTestResultsKey:   []TestResult{},
		},
		"$unset": bson.M{
			TaskDetailsKey: "",
		},
	}

	err := UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		reset,
	)
	if err != nil {
		return err
	}

	// update the cached version of the task, in its build document
	if err = build.ResetCachedTask(t.BuildId, t.Id); err != nil {
		return err
	}

	return t.UpdateBuildStatus()
}

func (t *Task) MarkStart() error {
	// record the start time in the in-memory task
	startTime := time.Now()
	t.StartTime = startTime
	t.Status = evergreen.TaskStarted
	err := UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskStatusKey:    evergreen.TaskStarted,
				TaskStartTimeKey: startTime,
			},
		},
	)
	if err != nil {
		return err
	}

	event.LogTaskStarted(t.Id)

	// ensure the appropriate build is marked as started if necessary
	if err = build.TryMarkStarted(t.BuildId, startTime); err != nil {
		return err
	}

	// ensure the appropriate version is marked as started if necessary
	if err = MarkVersionStarted(t.Version, startTime); err != nil {
		return err
	}

	// if it's a patch, mark the patch as started if necessary
	if t.Requester == evergreen.PatchVersionRequester {
		if err = patch.TryMarkStarted(t.Version, startTime); err != nil {
			return err
		}
	}

	// update the cached version of the task, in its build document
	return build.SetCachedTaskStarted(t.BuildId, t.Id, startTime)
}

func (t *Task) UpdateBuildStatus() error {
	finishTime := time.Now()
	// get all of the tasks in the same build
	b, err := build.FindOne(build.ById(t.BuildId))
	if err != nil {
		return err
	}

	buildTasks, err := FindTasksForBuild(b)
	if err != nil {
		return err
	}

	pushTaskExists := false
	for _, task := range buildTasks {
		if task.DisplayName == evergreen.PushStage {
			pushTaskExists = true
		}
	}

	failedTask := false
	pushSuccess := true
	pushCompleted := false
	finishedTasks := 0

	// update the build's status based on tasks for this build
	for _, task := range buildTasks {
		if task.IsFinished() {
			finishedTasks += 1
			// if it was a compile task, mark the build status accordingly
			if task.DisplayName == evergreen.CompileStage {
				if task.Status != evergreen.TaskSucceeded {
					failedTask = true
					finishedTasks = -1
					err = b.MarkFinished(evergreen.BuildFailed, finishTime)
					if err != nil {
						evergreen.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
						return err
					}
					break
				}
			} else if task.DisplayName == evergreen.PushStage {
				pushCompleted = true
				// if it's a finished push, check if it was successful
				if task.Status != evergreen.TaskSucceeded {
					err = b.UpdateStatus(evergreen.BuildFailed)
					if err != nil {
						evergreen.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
						return err
					}
					pushSuccess = false
				}
			} else {
				// update the build's status when a test task isn't successful
				if task.Status != evergreen.TaskSucceeded {
					err = b.UpdateStatus(evergreen.BuildFailed)
					if err != nil {
						evergreen.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
						return err
					}
					failedTask = true
				}
			}
		}
	}

	// if there are no failed tasks, mark the build as started
	if !failedTask {
		err = b.UpdateStatus(evergreen.BuildStarted)
		if err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
			return err
		}
	}
	// if a compile task didn't fail, then the
	// build is only finished when both the compile
	// and test tasks are completed or when those are
	// both completed in addition to a push (a push
	// does not occur if there's a failed task)
	if finishedTasks >= len(buildTasks)-1 {
		if !failedTask {
			if pushTaskExists { // this build has a push task associated with it.
				if pushCompleted && pushSuccess { // the push succeeded, so mark the build as succeeded.
					err = b.MarkFinished(evergreen.BuildSucceeded, finishTime)
					if err != nil {
						evergreen.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
						return err
					}
				} else if pushCompleted && !pushSuccess { // the push failed, mark build failed.
					err = b.MarkFinished(evergreen.BuildFailed, finishTime)
					if err != nil {
						evergreen.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
						return err
					}
				} else {
					//This build does have a "push" task, but it hasn't finished yet
					//So do nothing, since we don't know the status yet.
				}
				if err = MarkVersionCompleted(b.Version, finishTime); err != nil {
					evergreen.Logger.Errorf(slogger.ERROR, "Error marking version as finished: %v", err)
					return err
				}
			} else { // this build has no push task. so go ahead and mark it success/failure.
				if err = b.MarkFinished(evergreen.BuildSucceeded, finishTime); err != nil {
					evergreen.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
					return err
				}
				if b.Requester == evergreen.PatchVersionRequester {
					if err = TryMarkPatchBuildFinished(b, finishTime); err != nil {
						evergreen.Logger.Errorf(slogger.ERROR, "Error marking patch as finished: %v", err)
						return err
					}
				}
				if err = MarkVersionCompleted(b.Version, finishTime); err != nil {
					evergreen.Logger.Errorf(slogger.ERROR, "Error marking version as finished: %v", err)
					return err
				}
			}
		} else {
			// some task failed
			if err = b.MarkFinished(evergreen.BuildFailed, finishTime); err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
				return err
			}
			if b.Requester == evergreen.PatchVersionRequester {
				if err = TryMarkPatchBuildFinished(b, finishTime); err != nil {
					evergreen.Logger.Errorf(slogger.ERROR, "Error marking patch as finished: %v", err)
					return err
				}
			}
			if err = MarkVersionCompleted(b.Version, finishTime); err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error marking version as finished: %v", err)
				return err
			}
		}
	}

	// this is helpful for when we restart a compile task
	if finishedTasks == 0 {
		err = b.UpdateStatus(evergreen.BuildCreated)
		if err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
			return err
		}
	}

	return nil
}

// Returns true if the task should stepback upon failure, and false
// otherwise. Note that the setting is obtained from the top-level
// project, if not explicitly set on the task itt.
func (t *Task) getStepback(project *Project) bool {
	projectTask := project.FindProjectTask(t.DisplayName)

	// Check if the task overrides the stepback policy specified by the project
	if projectTask != nil && projectTask.Stepback != nil {
		return *projectTask.Stepback
	}

	// Check if the build variant overrides the stepback policy specified by the project
	for _, buildVariant := range project.BuildVariants {
		if t.BuildVariant == buildVariant.Name {
			if buildVariant.Stepback != nil {
				return *buildVariant.Stepback
			}
			break
		}
	}

	return project.Stepback
}

func (t *Task) markEnd(caller string, finishTime time.Time, detail *apimodels.TaskEndDetail) error {
	// record that the task has finished, in memory and in the db
	t.Status = detail.Status
	t.FinishTime = finishTime
	t.TimeTaken = finishTime.Sub(t.StartTime)
	t.Details = *detail
	err := UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskFinishTimeKey: finishTime,
				TaskStatusKey:     detail.Status,
				TaskTimeTakenKey:  t.TimeTaken,
				TaskDetailsKey:    t.Details,
			},
			"$unset": bson.M{
				TaskAbortedKey: "",
			},
		})

	if err != nil {
		return fmt.Errorf("error updating task: %v", err.Error())
	}
	event.LogTaskFinished(t.Id, detail.Status)
	return nil
}

func (t *Task) MarkEnd(caller string, finishTime time.Time, detail *apimodels.TaskEndDetail, p *Project, deactivatePrevious bool) error {
	if t.Status == detail.Status {
		evergreen.Logger.Logf(slogger.WARN, "Tried to mark task %v as finished twice", t.Id)
		return nil
	}

	t.Details = *detail

	err := t.markEnd(caller, finishTime, detail)
	if err != nil {
		return err
	}

	// update the cached version of the task, in its build document
	err = build.SetCachedTaskFinished(t.BuildId, t.Id, detail, t.TimeTaken)
	if err != nil {
		return fmt.Errorf("error updating build: %v", err.Error())
	}

	// no need to activate/deactivate other task if this is a patch request's task
	if t.Requester == evergreen.PatchVersionRequester {
		err = t.UpdateBuildStatus()
		if err != nil {
			return fmt.Errorf("Error updating build status (1): %v", err.Error())
		}
		return nil
	}

	// Do stepback
	if detail.Status == evergreen.TaskFailed {
		if shouldStepBack := t.getStepback(p); shouldStepBack {
			//See if there is a prior success for this particular task.
			//If there isn't, we should not activate the previous task because
			//it could trigger stepping backwards ad infinitum.
			_, err := PreviousCompletedTask(t, t.Project, []string{evergreen.TaskSucceeded})
			if err != nil {
				if err == mgo.ErrNotFound {
					shouldStepBack = false
				} else {
					return fmt.Errorf("Error locating previous successful task: %v",
						err)
				}
			}

			if shouldStepBack {
				// activate the previous task to pinpoint regression
				err = t.ActivatePreviousTask(caller)
				if err != nil {
					return fmt.Errorf("Error activating previous task: %v", err)
				}
			} else {
				evergreen.Logger.Logf(slogger.DEBUG, "Not stepping backwards on task failure: %v", t.Id)
			}
		}
	} else if deactivatePrevious {
		// if the task was successful, ignore running previous
		// activated tasks for this buildvariant
		err = t.DeactivatePreviousTasks(caller)
		if err != nil {
			return fmt.Errorf("Error deactivating previous task: %v", err.Error())
		}
	}

	// update the build
	if err := t.UpdateBuildStatus(); err != nil {
		return fmt.Errorf("Error updating build status (2): %v", err.Error())
	}

	return nil
}

func (t *Task) SetResults(results []TestResult) error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskTestResultsKey: results,
			},
		},
	)
}

func (t *Task) MarkUnscheduled() error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskStatusKey: evergreen.TaskUndispatched,
			},
		},
	)

}

func (t *Task) ClearResults() error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskTestResultsKey: []TestResult{},
			},
		},
	)
}

func (t *Task) ActivatePreviousTask(caller string) (err error) {
	// find previous tasks limiting to just the last one
	tasks, err := t.FindPreviousTasks(1)
	if err != nil {
		return
	}

	// if this is the first time we're
	// running this task do nothing
	if len(tasks) == 0 {
		return nil
	}

	// there's nothing to do if the previous task already ran
	if tasks[0].IsFinished() {
		return nil
	}

	//The task is blacklisted, so don't activate it and stop the stepback chain
	if tasks[0].Priority < 0 {
		return nil
	}

	// activate the task
	return SetTaskActivated(tasks[0].Id, caller, true)
}

// Deactivate any previously activated but undispatched
// tasks for the same build variant + display name + project combination
// as the task.
func (t *Task) DeactivatePreviousTasks(caller string) (err error) {
	priorRevisions := bson.M{
		"$lt": t.RevisionOrderNumber,
	}

	query := bson.M{
		TaskBuildVariantKey:        t.BuildVariant,
		TaskDisplayNameKey:         t.DisplayName,
		TaskRevisionOrderNumberKey: priorRevisions,
		TaskStatusKey:              evergreen.TaskUndispatched,
		TaskRequesterKey:           evergreen.RepotrackerVersionRequester,
		TaskActivatedKey:           true,
		TaskProjectKey:             t.Project,
	}

	allTasks, err := FindAllTasks(
		query,
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		return err
	}

	for _, task := range allTasks {
		err = SetTaskActivated(task.Id, caller, false)
		if err != nil {
			return err
		}
	}

	return nil
}

/***********************
Create
***********************/

// Inserts the task into the tasks collection, and logs an event that the task
// was created.
func (t *Task) Insert() error {
	event.LogTaskCreated(t.Id)
	return db.Insert(TasksCollection, t)
}

// Inserts the task into the old_tasks collection
func (t *Task) Archive() error {
	var update bson.M
	// only increment restarts if have a current restarts
	// this way restarts will never be set for new tasks but will be
	// maintained for old ones
	if t.Restarts > 0 {
		update = bson.M{"$inc": bson.M{
			TaskExecutionKey: 1,
			TaskRestartsKey:  1,
		}}
	} else {
		update = bson.M{
			"$inc": bson.M{TaskExecutionKey: 1},
		}
	}
	err := UpdateOneTask(
		bson.M{TaskIdKey: t.Id},
		update)
	if err != nil {
		return fmt.Errorf("task.Archive() failed: %v", err)
	}
	archive_task := *t
	archive_task.Id = fmt.Sprintf("%v_%v", t.Id, t.Execution)
	archive_task.OldTaskId = t.Id
	archive_task.Archived = true
	err = db.Insert(OldTasksCollection, &archive_task)
	if err != nil {
		return fmt.Errorf("task.Archive() failed: %v", err)
	}
	return nil
}

/***********************
Remove
***********************/

func RemoveAllTasks(query interface{}) error {
	return db.RemoveAll(
		TasksCollection,
		query,
	)
}

/*************************
String
*************************/
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

/*************************
Aggregation
*************************/

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

	err := db.Aggregate(TasksCollection, pipeline, &results)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error aggregating task times by [%v, %v]: %v", field1, field2, err)
		return nil, err
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
				TaskBuildVariantKey: buildvariant,
				TaskProjectKey:      project,
				TaskStatusKey: bson.M{
					"$in": []string{evergreen.TaskSucceeded, evergreen.TaskFailed},
				},
				TaskDetailsKey + "." + TaskEndDetailTimedOut: bson.M{
					"$ne": true,
				},
				TaskFinishTimeKey: bson.M{
					"$gte": time.Now().Add(-window),
				},
			},
		},
		{
			"$project": bson.M{
				TaskDisplayNameKey: 1,
				TaskTimeTakenKey:   1,
				TaskIdKey:          0,
			},
		},
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%v", TaskDisplayNameKey),
				"exp_dur": bson.M{
					"$avg": fmt.Sprintf("$%v", TaskTimeTakenKey),
				},
			},
		},
	}

	// anonymous struct for unmarshalling result bson
	var results []struct {
		DisplayName      string `bson:"_id"`
		ExpectedDuration int64  `bson:"exp_dur"`
	}

	err := db.Aggregate(TasksCollection, pipeline, &results)
	if err != nil {
		return nil, fmt.Errorf("error aggregating task average duration: %v", err)
	}

	expDurations := make(map[string]time.Duration)
	for _, result := range results {
		expDuration := time.Duration(result.ExpectedDuration) * time.Nanosecond
		expDurations[result.DisplayName] = expDuration
	}

	return expDurations, nil
}

// getTestUrl returns the correct relative URL to a test log, given a
// TestResult structure
func getTestUrl(tr *TestResult) string {
	// Return url if it exists. If there is no test, return empty string.
	if tr.URL != "" || tr.LogId == "" { // If LogId is empty, URL must also be empty
		return tr.URL
	}
	return TestLogPath + tr.LogId
}

func FindTasks(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(TasksCollection, query, &tasks)
	return tasks, err
}
