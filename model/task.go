package model

import (
	"10gen.com/mci"
	"10gen.com/mci/apimodels"
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

const (
	TasksCollection    = "tasks"
	OldTasksCollection = "old_tasks"
)

var ZeroTime time.Time = time.Unix(0, 0)

type RemoteArgs struct {
	Params  []string          `bson:"params" json:"params"`
	Options map[string]string `bson:"options" json:"options"`
}

func (self *RemoteArgs) ToArray() []string {
	var arr []string

	for k, v := range self.Options {
		arr = append(arr, "-"+k)
		arr = append(arr, v)
	}

	for _, p := range self.Params {
		arr = append(arr, p)
	}

	return arr
}

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
	Activated    bool     `bson:"activated" json:"activated"`
	BuildId      string   `bson:"build_id" json:"build_id"`
	DistroId     string   `bson:"distro" json:"distro"`
	BuildVariant string   `bson:"build_variant" json:"build_variant"`
	DependsOn    []string `bson:"depends_on" json:"depends_on"`
	RemoteArgs   `bson:"remote_args" json:"remote_args"`

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
	Status        string                   `bson:"status" json:"status"`
	StatusDetails apimodels.TaskEndDetails `bson:"status_details" json:"status_details"`
	Aborted       bool                     `bson:"abort,omitempty" json:"abort"`

	// how long the task took to execute.  meaningless if the task is not finished
	TimeTaken time.Duration `bson:"time_taken" json:"time_taken"`

	// how long we expect the task to take from start to finish
	ExpectedDuration time.Duration `bson:"expected_duration,omitempty" json:"expected_duration,omitempty"`

	// test results captured and sent back by agent
	TestResults []TestResult `bson:"test_results" json:"test_results"`

	// position in queue for the queue where it's closest to the top
	MinQueuePos int `bson:"min_queue_pos" json:"min_queue_pos,omitempty"`
}

// TestResults is only used when transferring data from agent to api.
type TestResults struct {
	Results []TestResult `json:"results"`
}

type TestResult struct {
	Status    string  `json:"status" bson:"status"`
	TestFile  string  `json:"test_file" bson:"test_file"`
	URL       string  `json:"url" bson:"url"`
	ExitCode  int     `json:"exit_code" bson:"exit_code"`
	StartTime float64 `json:"start" bson:"start"`
	EndTime   float64 `json:"end" bson:"end"`
}

var (
	// bson fields for the task struct
	TaskIdKey                  = MustHaveBsonTag(Task{}, "Id")
	TaskSecretKey              = MustHaveBsonTag(Task{}, "Secret")
	TaskCreateTimeKey          = MustHaveBsonTag(Task{}, "CreateTime")
	TaskDispatchTimeKey        = MustHaveBsonTag(Task{}, "DispatchTime")
	TaskPushTimeKey            = MustHaveBsonTag(Task{}, "PushTime")
	TaskScheduledTimeKey       = MustHaveBsonTag(Task{}, "ScheduledTime")
	TaskStartTimeKey           = MustHaveBsonTag(Task{}, "StartTime")
	TaskFinishTimeKey          = MustHaveBsonTag(Task{}, "FinishTime")
	TaskVersionKey             = MustHaveBsonTag(Task{}, "Version")
	TaskProjectKey             = MustHaveBsonTag(Task{}, "Project")
	TaskRevisionKey            = MustHaveBsonTag(Task{}, "Revision")
	TaskLastHeartbeatKey       = MustHaveBsonTag(Task{}, "LastHeartbeat")
	TaskActivatedKey           = MustHaveBsonTag(Task{}, "Activated")
	TaskBuildIdKey             = MustHaveBsonTag(Task{}, "BuildId")
	TaskDistroIdKey            = MustHaveBsonTag(Task{}, "DistroId")
	TaskBuildVariantKey        = MustHaveBsonTag(Task{}, "BuildVariant")
	TaskDependsOnKey           = MustHaveBsonTag(Task{}, "DependsOn")
	TaskRemoteArgsKey          = MustHaveBsonTag(Task{}, "RemoteArgs")
	TaskDisplayNameKey         = MustHaveBsonTag(Task{}, "DisplayName")
	TaskHostIdKey              = MustHaveBsonTag(Task{}, "HostId")
	TaskExecutionKey           = MustHaveBsonTag(Task{}, "Execution")
	TaskRestartsKey            = MustHaveBsonTag(Task{}, "Restarts")
	TaskOldTaskIdKey           = MustHaveBsonTag(Task{}, "OldTaskId")
	TaskArchivedKey            = MustHaveBsonTag(Task{}, "Archived")
	TaskRevisionOrderNumberKey = MustHaveBsonTag(Task{}, "RevisionOrderNumber")
	TaskRequesterKey           = MustHaveBsonTag(Task{}, "Requester")
	TaskStatusKey              = MustHaveBsonTag(Task{}, "Status")
	TaskStatusDetailsKey       = MustHaveBsonTag(Task{}, "StatusDetails")
	TaskAbortedKey             = MustHaveBsonTag(Task{}, "Aborted")
	TaskTimeTakenKey           = MustHaveBsonTag(Task{}, "TimeTaken")
	TaskExpectedDurationKey    = MustHaveBsonTag(Task{}, "ExpectedDuration")
	TaskTestResultsKey         = MustHaveBsonTag(Task{}, "TestResults")
	TaskPriorityKey            = MustHaveBsonTag(Task{}, "Priority")
	TaskMinQueuePosKey         = MustHaveBsonTag(Task{}, "MinQueuePos")

	// bson fields for the remote args struct
	RemoteArgsParamsKey  = MustHaveBsonTag(RemoteArgs{}, "Params")
	RemoteArgsOptionsKey = MustHaveBsonTag(RemoteArgs{}, "Options")

	// bson fields for the test result struct
	TestResultStatusKey    = MustHaveBsonTag(TestResult{}, "Status")
	TestResultTestFileKey  = MustHaveBsonTag(TestResult{}, "TestFile")
	TestResultURLKey       = MustHaveBsonTag(TestResult{}, "URL")
	TestResultExitCodeKey  = MustHaveBsonTag(TestResult{}, "ExitCode")
	TestResultStartTimeKey = MustHaveBsonTag(TestResult{}, "StartTime")
	TestResultEndTimeKey   = MustHaveBsonTag(TestResult{}, "EndTime")

	// bson fields for task status details struct
	TaskStatusDetailsTimeoutStage = MustHaveBsonTag(apimodels.TaskEndDetails{}, "TimeoutStage")
	TaskStatusDetailsTimedOut     = MustHaveBsonTag(apimodels.TaskEndDetails{}, "TimedOut")
)

func (self *Task) Abortable() bool {
	return self.Status == mci.TaskStarted ||
		self.Status == mci.TaskDispatched
}

func (task Task) IsStarted() bool {
	return task.Status == mci.TaskStarted
}

func (task Task) IsFinished() bool {
	return task.Status == mci.TaskFailed ||
		task.Status == mci.TaskCancelled ||
		task.Status == mci.TaskSucceeded ||
		(task.Status == mci.TaskUndispatched && task.DispatchTime != ZeroTime)
}

// Checks whether the dependencies for the task have all completed successfully.
// If any of the dependencies exist in the map that is passed in, they are
// used to check rather than fetching from the database.
func (self *Task) DependenciesMet(depCaches map[string]Task) (bool, error) {

	if len(self.DependsOn) == 0 {
		return true, nil
	}

	deps := make([]Task, 0, len(self.DependsOn))

	depIdsToQueryFor := make([]string, 0, len(self.DependsOn))
	for _, depId := range self.DependsOn {
		if cachedDep, ok := depCaches[depId]; !ok {
			depIdsToQueryFor = append(depIdsToQueryFor, depId)
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
		for _, newDep := range newDeps {
			deps = append(deps, newDep)
			depCaches[newDep.Id] = newDep
		}
	}

	for _, depTask := range deps {
		if depTask.Status != mci.TaskSucceeded {
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
		"$in": []string{mci.TaskStarted, mci.TaskDispatched},
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

func (self *Task) FindTaskOnBaseCommit() (*Task, error) {
	return FindOneTask(
		bson.M{
			TaskRevisionKey:     self.Revision,
			TaskRequesterKey:    mci.RepotrackerVersionRequester,
			TaskBuildVariantKey: self.BuildVariant,
			TaskDisplayNameKey:  self.DisplayName,
			TaskProjectKey:      self.Project,
		},
		db.NoProjection,
		db.NoSort,
	)
}

func FindUndispatchedTasks() ([]Task, error) {
	return FindAllTasks(
		bson.M{
			TaskActivatedKey: true,
			TaskStatusKey:    mci.TaskUndispatched,
			//Filter out blacklisted tasks
			//TODO eventually this $or should be removed as new tasks
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

func FindTasksForBuild(build *Build) ([]Task, error) {
	tasks, err := FindAllTasks(
		bson.M{
			TaskBuildIdKey: build.Id,
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
		TaskStatusKey:      mci.TaskFailed,
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
		statuses = []string{mci.TaskCancelled, mci.TaskFailed,
			mci.TaskSucceeded}
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
					mci.TaskFailed,
					mci.TaskSucceeded,
					mci.TaskCancelled,
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

func (task *Task) FetchPatch() (*Patch, error) {
	// first get the version associated with this task
	version, err := FindVersion(task.Version)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("could not find version %v for task %v",
			task.Version, task.Id)
	}

	// find the patch associated with this version
	return FindPatchByVersion(version.Id)
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
			TaskStatusKey: bson.M{"$in": mci.CompletedStatuses},
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
		TaskStatusKey:       bson.M{"$in": mci.CompletedStatuses},
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

// Mark that the task has been dispatched onto a particular host.  Sets the
// running task field on the host and the host id field on the task, as well
// as updating the cache for the task in its build document in the db.
// Returns an error if any of the database updates fail.
func (self *Task) MarkAsDispatched(host *Host, dispatchTime time.Time) error {
	// then, update the task document
	self.DispatchTime = dispatchTime
	self.Status = mci.TaskDispatched
	self.HostId = host.Id
	self.LastHeartbeat = dispatchTime
	self.DistroId = host.Distro
	err := UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskDispatchTimeKey:  dispatchTime,
				TaskStatusKey:        mci.TaskDispatched,
				TaskHostIdKey:        host.Id,
				TaskLastHeartbeatKey: dispatchTime,
				TaskDistroIdKey:      host.Distro,
			},
			"$unset": bson.M{
				TaskAbortedKey:       "",
				TaskTestResultsKey:   "",
				TaskStatusDetailsKey: "",
				TaskMinQueuePosKey:   "",
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error updating task with id %v: %v", self.Id, err)
	}

	// the task was successfully dispatched, log the event
	LogTaskDispatchedEvent(self.Id, host.Id)

	// update the cached version of the task in its related build document
	err = UpdateOneBuild(
		bson.M{
			BuildIdKey:                           self.BuildId,
			BuildTasksKey + "." + TaskCacheIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				BuildTasksKey + ".$." + TaskCacheStatusKey: mci.TaskDispatched,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error updating task cache in build %v: %v",
			self.BuildId, err)
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
			TaskIdKey:            bson.M{"$in": ids},
			TaskScheduledTimeKey: bson.M{"$lte": ZeroTime},
		},
		bson.M{"$set": bson.M{TaskScheduledTimeKey: scheduledTime}},
	)

	if err != nil {
		return err
	}

	if info.Updated > 0 {
		for _, task := range tasks {
			LogTaskScheduledEvent(task.Id, scheduledTime)
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
		if task.DispatchTime != ZeroTime && task.Status == mci.TaskUndispatched {
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
		LogTaskActivatedEvent(taskId, caller)
	} else {
		LogTaskDeactivatedEvent(taskId, caller)
	}

	// update the cached version of the task, in its build document
	return UpdateOneBuild(
		bson.M{
			BuildIdKey:                           task.BuildId,
			BuildTasksKey + "." + TaskCacheIdKey: taskId,
		},
		bson.M{
			"$set": bson.M{
				BuildTasksKey + ".$." + TaskCacheActivatedKey: active,
			},
		},
	)
}

// TODO: this takes in an aborted parameter but always aborts the task
func (self *Task) Abort(caller string, aborted bool) error {
	if !self.Abortable() {
		return fmt.Errorf("Task '%v' is currently '%v' - cannot abort task"+
			" in this status", self.Id, self.Status)
	}

	mci.Logger.Logf(slogger.DEBUG, "Setting abort=%v for task %v", aborted, self.Id)

	err := SetTaskActivated(self.Id, caller, false)
	if err != nil {
		return err
	}

	err = UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
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

	LogTaskAbortRequestEvent(self.Id, caller)

	self.Aborted = aborted
	return nil
}

func (self *Task) UpdateHeartbeat() error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskLastHeartbeatKey: time.Now(),
			},
		},
	)
}

func (self *Task) SetPriority(priority int) error {
	self.Priority = priority
	modifier := bson.M{TaskPriorityKey: priority}

	//blacklisted - this task should never run, so unschedule it now
	if priority < 0 {
		modifier[TaskActivatedKey] = false
	}

	return UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		bson.M{"$set": modifier},
	)
}

func (self *Task) TryReset(user, origin string, project *Project,
	taskEndRequest *apimodels.TaskEndRequest) (err error) {
	// if we've reached the max # of executions
	// for this task, mark it as finished and failed
	if self.Execution >= mci.MaxTaskExecution {
		// restarting from the ui bypassed the restart cap
		if origin == mci.UIPackage {
			mci.Logger.Logf(slogger.DEBUG, "Task '%v' reached max execution"+
				" (%v); Allowing exception for %v", self.Id,
				mci.MaxTaskExecution, user)
		} else {
			mci.Logger.Logf(slogger.DEBUG, "Task '%v' reached max execution"+
				" (%v); marking as failed.", self.Id, mci.MaxTaskExecution)
			if taskEndRequest != nil {
				return self.MarkEnd(origin, time.Now(), taskEndRequest, project)
			} else {
				panic(fmt.Sprintf("TryReset called with nil TaskEndRequest "+
					"by %v", origin))
			}
		}
	}

	// only allow re-execution for failed, cancelled or successful tasks
	if !self.IsFinished() {
		// this is to disallow terminating running tasks via the UI
		if origin == mci.UIPackage {
			mci.Logger.Logf(slogger.DEBUG, "Will not satisfy '%v' requested"+
				" reset for '%v' - current status is '%v'", user, self.Id,
				self.Status)
			return fmt.Errorf("Task '%v' is currently '%v' - can not reset"+
				" task in this status", self.Id, self.Status)
		}
	}

	if taskEndRequest != nil {
		err = self.markEnd(origin, time.Now(), taskEndRequest)
		if err != nil {
			return fmt.Errorf("Error marking task as ended: %v", err)
		}
	}

	if err = self.reset(); err == nil {
		if origin == mci.UIPackage {
			LogTaskRestartedEvent(self.Id, user)
		} else {
			LogTaskRestartedEvent(self.Id, origin)
		}
	}
	return err
}

func (self *Task) reset() error {
	if err := self.Archive(); err != nil {
		return fmt.Errorf("Can't restart task because it can't be archived: %v", err)
	}

	reset := bson.M{
		"$set": bson.M{
			TaskActivatedKey:     true,
			TaskSecretKey:        util.RandomString(),
			TaskStatusKey:        mci.TaskUndispatched,
			TaskDispatchTimeKey:  ZeroTime,
			TaskStartTimeKey:     ZeroTime,
			TaskScheduledTimeKey: ZeroTime,
			TaskFinishTimeKey:    ZeroTime,
			TaskTestResultsKey:   []TestResult{}},
		"$unset": bson.M{
			TaskStatusDetailsKey: "",
		},
	}

	err := UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		reset,
	)
	if err != nil {
		return err
	}

	// update the cached version of the task, in its build document
	err = UpdateOneBuild(
		bson.M{
			BuildIdKey:                           self.BuildId,
			BuildTasksKey + "." + TaskCacheIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				BuildTasksKey + ".$." + TaskCacheStartTimeKey: ZeroTime,
				BuildTasksKey + ".$." + TaskCacheStatusKey:    mci.TaskUndispatched,
			},
		},
	)
	if err != nil {
		return err
	}

	return self.UpdateBuildStatus()
}

func (self *Task) MarkStart() error {
	// record the start time in the in-memory task
	startTime := time.Now()
	self.StartTime = startTime
	self.Status = mci.TaskStarted
	err := UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskStatusKey:    mci.TaskStarted,
				TaskStartTimeKey: startTime,
			},
		},
	)
	if err != nil {
		return err
	}

	LogTaskStartedEvent(self.Id)

	// ensure the appropriate build is marked as started if necessary
	if err = TryMarkBuildStarted(self.BuildId, startTime); err != nil {
		return err
	}

	// ensure the appropriate version is marked as started if necessary
	if err = TryMarkVersionStarted(self.Version, startTime); err != nil {
		return err
	}

	// if it's a patch, mark the patch as started if necessary
	if self.Requester == mci.PatchVersionRequester {
		if err = TryMarkPatchStarted(self.Version, startTime); err != nil {
			return err
		}
	}

	// update the cached version of the task, in its build document
	selector := bson.M{
		BuildIdKey:                           self.BuildId,
		BuildTasksKey + "." + TaskCacheIdKey: self.Id,
	}
	update := bson.M{
		"$set": bson.M{
			BuildTasksKey + ".$." + TaskCacheStartTimeKey: startTime,
			BuildTasksKey + ".$." + TaskCacheStatusKey:    mci.TaskStarted,
		},
	}
	return UpdateOneBuild(selector, update)
}

func (self *Task) UpdateBuildStatus() error {
	finishTime := time.Now()
	// get all of the tasks in the same build
	build, err := FindBuild(self.BuildId)
	if err != nil {
		return err
	}

	buildTasks, err := FindTasksForBuild(build)
	if err != nil {
		return err
	}

	pushTaskExists := false
	for _, task := range buildTasks {
		if task.DisplayName == mci.PushStage {
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
			if task.DisplayName == mci.CompileStage {
				if task.Status != mci.TaskSucceeded {
					failedTask = true
					finishedTasks = -1
					err = build.MarkFinished(mci.BuildFailed, finishTime)
					if err != nil {
						mci.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
						return err
					}
					break
				}
			} else if task.DisplayName == mci.PushStage {
				pushCompleted = true
				// if it's a finished push, check if it was successful
				if task.Status != mci.TaskSucceeded {
					err = build.UpdateStatus(mci.BuildFailed)
					if err != nil {
						mci.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
						return err
					}
					pushSuccess = false
				}
			} else {
				// update the build's status when a test task isn't successful
				if task.Status != mci.TaskSucceeded {
					err = build.UpdateStatus(mci.BuildFailed)
					if err != nil {
						mci.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
						return err
					}
					failedTask = true
				}
			}
		}
	}

	// if there are no failed tasks, mark the build as started
	if !failedTask {
		err = build.UpdateStatus(mci.BuildStarted)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
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
					err = build.MarkFinished(mci.BuildSucceeded, finishTime)
					if err != nil {
						mci.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
						return err
					}
				} else if pushCompleted && !pushSuccess { // the push failed, mark build failed.
					err = build.MarkFinished(mci.BuildFailed, finishTime)
					if err != nil {
						mci.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
						return err
					}
				} else {
					//This build does have a "push" task, but it hasn't finished yet
					//So do nothing, since we don't know the status yet.
				}
				if err = build.TryMarkVersionFinished(finishTime); err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error marking version as finished: %v", err)
					return err
				}
			} else { // this build has no push task. so go ahead and mark it success/failure.
				if err = build.MarkFinished(mci.BuildSucceeded, finishTime); err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
					return err
				}
				if build.Requester == mci.PatchVersionRequester {
					if err = build.TryMarkPatchFinished(finishTime); err != nil {
						mci.Logger.Errorf(slogger.ERROR, "Error marking patch as finished: %v", err)
						return err
					}
				}
				if err = build.TryMarkVersionFinished(finishTime); err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error marking version as finished: %v", err)
					return err
				}
			}
		} else {
			// some task failed
			if err = build.MarkFinished(mci.BuildFailed, finishTime); err != nil {
				mci.Logger.Errorf(slogger.ERROR, "Error marking build as finished: %v", err)
				return err
			}
			if build.Requester == mci.PatchVersionRequester {
				if err = build.TryMarkPatchFinished(finishTime); err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error marking patch as finished: %v", err)
					return err
				}
			}
			if err = build.TryMarkVersionFinished(finishTime); err != nil {
				mci.Logger.Errorf(slogger.ERROR, "Error marking version as finished: %v", err)
				return err
			}
		}
	}

	// this is helpful for when we restart a compile task
	if finishedTasks == 0 {
		err = build.UpdateStatus(mci.BuildCreated)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error updating build status: %v", err)
			return err
		}
	}

	return nil
}

func (self *Task) markEnd(caller string, finishTime time.Time,
	taskEndRequest *apimodels.TaskEndRequest) error {
	// record that the task has finished, in memory and in the db
	self.Status = taskEndRequest.Status
	self.FinishTime = finishTime
	self.TimeTaken = finishTime.Sub(self.StartTime)
	self.StatusDetails = taskEndRequest.StatusDetails

	err := UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskFinishTimeKey:    finishTime,
				TaskStatusKey:        taskEndRequest.Status,
				TaskTimeTakenKey:     self.TimeTaken,
				TaskStatusDetailsKey: taskEndRequest.StatusDetails,
			},
			"$unset": bson.M{
				TaskAbortedKey: "",
			},
		})

	if err != nil {
		return fmt.Errorf("error updating task: %v", err.Error())
	}
	LogTaskFinishedEvent(self.Id, taskEndRequest.Status)
	return nil
}

// Returns true if the task should stepback upon failure, and false
// otherwise. Note that the setting is obtained from the top-level
// project, if not explicitly set on the task itself.
func (self *Task) getStepback(project *Project) bool {
	projectTask := project.FindProjectTask(self.DisplayName)

	// Check if the task overrides the stepback policy specified by the project
	if projectTask != nil && projectTask.Stepback != nil {
		return *projectTask.Stepback
	}

	// Check if the build variant overrides the stepback policy specified by the project
	for _, buildVariant := range project.BuildVariants {
		if self.BuildVariant == buildVariant.Name {
			if buildVariant.Stepback != nil {
				return *buildVariant.Stepback
			}
			break
		}
	}

	return project.Stepback
}

func (self *Task) MarkEnd(caller string, finishTime time.Time,
	taskEndRequest *apimodels.TaskEndRequest, project *Project) error {
	if self.Status == taskEndRequest.Status {
		mci.Logger.Logf(slogger.WARN, "Tried to mark task %v as finished twice",
			self.Id)
		return nil
	}
	err := self.markEnd(caller, finishTime, taskEndRequest)
	if err != nil {
		return err
	}

	// update the cached version of the task, in its build document
	err = UpdateOneBuild(
		bson.M{
			BuildIdKey:                           self.BuildId,
			BuildTasksKey + "." + TaskCacheIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				BuildTasksKey + ".$." + TaskCacheStatusKey:    self.Status,
				BuildTasksKey + ".$." + TaskCacheTimeTakenKey: self.TimeTaken,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error updating build: %v", err.Error())
	}

	// no need to activate/deactivate other task if this is a patch request's
	// task
	if self.Requester == mci.PatchVersionRequester {
		err = self.UpdateBuildStatus()
		if err != nil {
			return fmt.Errorf("Error updating build status (1): %v", err.Error())
		}
		return nil
	}

	if taskEndRequest.Status == mci.TaskFailed {
		if shouldStepBack := self.getStepback(project); shouldStepBack {
			//See if there is a prior success for this particular task.
			//If there isn't, we should not activate the previous task because
			//it could trigger stepping backwards ad infinitum.
			_, err := PreviousCompletedTask(self, self.RemoteArgs.Options["branch"],
				[]string{mci.TaskSucceeded})
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
				err = self.ActivatePreviousTask(caller)
				if err != nil {
					return fmt.Errorf("Error activating previous task: %v", err)
				}
			} else {
				mci.Logger.Logf(slogger.DEBUG, "Not stepping backwards on task"+
					" failure: %v", self.Id)
			}
		}
	} else {
		// if the task was successful, ignore running previous
		// activated tasks for this buildvariant
		err = self.DeactivatePreviousTasks(caller)
		if err != nil {
			return fmt.Errorf("Error deactivating previous task: %v",
				err.Error())
		}
	}

	if err := self.UpdateBuildStatus(); err != nil {
		return fmt.Errorf("Error updating build status (2): %v", err.Error())
	}

	return nil
}

func (self *Task) SetResults(results []TestResult) error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskTestResultsKey: results,
			},
		},
	)
}

func (self *Task) ClearResults() error {
	return UpdateOneTask(
		bson.M{
			TaskIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				TaskTestResultsKey: []TestResult{},
			},
		},
	)
}

func (self *Task) ActivatePreviousTask(caller string) (err error) {
	// find previous tasks limiting to just the last one
	tasks, err := self.FindPreviousTasks(1)
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

	// activate the task's dependencies
	for _, taskId := range tasks[0].DependsOn {
		// activate the task's dependencies
		if err = SetTaskActivated(taskId, caller, true); err != nil {
			return err
		}
	}

	// then activate the task itself
	return SetTaskActivated(tasks[0].Id, caller, true)
}

// Deactivate any previously activated but undispatched
// tasks for the same build variant + display name + project combination
// as the task.
func (self *Task) DeactivatePreviousTasks(caller string) (err error) {
	priorRevisions := bson.M{
		"$lt": self.RevisionOrderNumber,
	}

	query := bson.M{
		TaskBuildVariantKey:        self.BuildVariant,
		TaskDisplayNameKey:         self.DisplayName,
		TaskRevisionOrderNumberKey: priorRevisions,
		TaskStatusKey:              mci.TaskUndispatched,
		TaskRequesterKey:           mci.RepotrackerVersionRequester,
		TaskActivatedKey:           true,
		TaskProjectKey:             self.Project,
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
func (self *Task) Insert() error {
	LogTaskCreatedEvent(self.Id)
	return db.Insert(TasksCollection, self)
}

// Inserts the task into the old_tasks collection
func (self *Task) Archive() error {
	var update bson.M
	// only increment restarts if have a current restarts
	// this way restarts will never be set for new tasks but will be
	// maintained for old ones
	if self.Restarts > 0 {
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
		bson.M{TaskIdKey: self.Id},
		update)
	if err != nil {
		return fmt.Errorf("task.Archive() failed: %v", err)
	}
	archive_task := *self
	archive_task.Id = fmt.Sprintf("%v_%v", self.Id, self.Execution)
	archive_task.OldTaskId = self.Id
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
			"avg_time": bson.M{"$avg": bson.M{
				"$subtract": []string{"$" + field2, "$" + field1},
			}}}},
	}

	// anonymous struct for unmarshalling result bson
	// NOTE: This means we can only group by string fields currently
	var results []struct {
		GroupId     string `bson:"_id"`
		AverageTime int64  `bson:"avg_time"`
	}

	err := db.Aggregate(TasksCollection, pipeline, &results)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR,
			"Error aggregating task times by [%v, %v]: %v",
			field1, field2, err)
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
func ExpectedTaskDuration(project,
	buildvariant string, window time.Duration) (map[string]time.Duration, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				TaskBuildVariantKey: buildvariant,
				TaskProjectKey:      project,
				TaskStatusKey: bson.M{
					"$in": []string{mci.TaskSucceeded, mci.TaskFailed},
				},
				TaskStatusDetailsKey + "." + TaskStatusDetailsTimedOut: bson.M{
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
		return nil, fmt.Errorf("error aggregating task average duration: %v",
			err)
	}

	expDurations := make(map[string]time.Duration)
	for _, result := range results {
		expDuration := time.Duration(result.ExpectedDuration) * time.Nanosecond
		expDurations[result.DisplayName] = expDuration
	}

	return expDurations, nil
}
