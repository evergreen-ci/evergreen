package task

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	Collection    = "tasks"
	OldCollection = "old_tasks"
	TestLogPath   = "/test_log/"
)

var (
	// BSON fields for the task struct
	IdKey                   = bsonutil.MustHaveTag(Task{}, "Id")
	SecretKey               = bsonutil.MustHaveTag(Task{}, "Secret")
	CreateTimeKey           = bsonutil.MustHaveTag(Task{}, "CreateTime")
	DispatchTimeKey         = bsonutil.MustHaveTag(Task{}, "DispatchTime")
	ScheduledTimeKey        = bsonutil.MustHaveTag(Task{}, "ScheduledTime")
	StartTimeKey            = bsonutil.MustHaveTag(Task{}, "StartTime")
	FinishTimeKey           = bsonutil.MustHaveTag(Task{}, "FinishTime")
	ActivatedTimeKey        = bsonutil.MustHaveTag(Task{}, "ActivatedTime")
	VersionKey              = bsonutil.MustHaveTag(Task{}, "Version")
	ProjectKey              = bsonutil.MustHaveTag(Task{}, "Project")
	RevisionKey             = bsonutil.MustHaveTag(Task{}, "Revision")
	LastHeartbeatKey        = bsonutil.MustHaveTag(Task{}, "LastHeartbeat")
	ActivatedKey            = bsonutil.MustHaveTag(Task{}, "Activated")
	BuildIdKey              = bsonutil.MustHaveTag(Task{}, "BuildId")
	DistroIdKey             = bsonutil.MustHaveTag(Task{}, "DistroId")
	BuildVariantKey         = bsonutil.MustHaveTag(Task{}, "BuildVariant")
	DependsOnKey            = bsonutil.MustHaveTag(Task{}, "DependsOn")
	OverrideDependenciesKey = bsonutil.MustHaveTag(Task{}, "OverrideDependencies")
	NumDepsKey              = bsonutil.MustHaveTag(Task{}, "NumDependents")
	DisplayNameKey          = bsonutil.MustHaveTag(Task{}, "DisplayName")
	HostIdKey               = bsonutil.MustHaveTag(Task{}, "HostId")
	ExecutionKey            = bsonutil.MustHaveTag(Task{}, "Execution")
	RestartsKey             = bsonutil.MustHaveTag(Task{}, "Restarts")
	OldTaskIdKey            = bsonutil.MustHaveTag(Task{}, "OldTaskId")
	ArchivedKey             = bsonutil.MustHaveTag(Task{}, "Archived")
	RevisionOrderNumberKey  = bsonutil.MustHaveTag(Task{}, "RevisionOrderNumber")
	RequesterKey            = bsonutil.MustHaveTag(Task{}, "Requester")
	StatusKey               = bsonutil.MustHaveTag(Task{}, "Status")
	DetailsKey              = bsonutil.MustHaveTag(Task{}, "Details")
	AbortedKey              = bsonutil.MustHaveTag(Task{}, "Aborted")
	TimeTakenKey            = bsonutil.MustHaveTag(Task{}, "TimeTaken")
	ExpectedDurationKey     = bsonutil.MustHaveTag(Task{}, "ExpectedDuration")
	DurationPredictionKey   = bsonutil.MustHaveTag(Task{}, "DurationPrediction")
	PriorityKey             = bsonutil.MustHaveTag(Task{}, "Priority")
	ActivatedByKey          = bsonutil.MustHaveTag(Task{}, "ActivatedBy")
	CostKey                 = bsonutil.MustHaveTag(Task{}, "Cost")
	SpawnedHostCostKey      = bsonutil.MustHaveTag(Task{}, "SpawnedHostCost")
	ExecutionTasksKey       = bsonutil.MustHaveTag(Task{}, "ExecutionTasks")
	DisplayOnlyKey          = bsonutil.MustHaveTag(Task{}, "DisplayOnly")
	TaskGroupKey            = bsonutil.MustHaveTag(Task{}, "TaskGroup")
	GenerateTaskKey         = bsonutil.MustHaveTag(Task{}, "GenerateTask")
	GeneratedTasksKey       = bsonutil.MustHaveTag(Task{}, "GeneratedTasks")
	GeneratedByKey          = bsonutil.MustHaveTag(Task{}, "GeneratedBy")
	ResetWhenFinishedKey    = bsonutil.MustHaveTag(Task{}, "ResetWhenFinished")
	LogsKey                 = bsonutil.MustHaveTag(Task{}, "Logs")

	// BSON fields for the test result struct
	TestResultStatusKey    = bsonutil.MustHaveTag(TestResult{}, "Status")
	TestResultLineNumKey   = bsonutil.MustHaveTag(TestResult{}, "LineNum")
	TestResultTestFileKey  = bsonutil.MustHaveTag(TestResult{}, "TestFile")
	TestResultURLKey       = bsonutil.MustHaveTag(TestResult{}, "URL")
	TestResultLogIdKey     = bsonutil.MustHaveTag(TestResult{}, "LogId")
	TestResultURLRawKey    = bsonutil.MustHaveTag(TestResult{}, "URLRaw")
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

// Queries

// All returns all tasks.
var All = db.Query(nil)

var (
	SelectorTaskInProgress = bson.M{
		"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched},
	}

	FinishedOpts = []bson.M{{
		StatusKey: bson.M{
			"$in": []string{
				evergreen.TaskFailed,
				evergreen.TaskSucceeded,
			},
		},
	},
	}
	CompletedStatuses = []string{evergreen.TaskSucceeded, evergreen.TaskFailed}
)

// ById creates a query that finds a task by its _id.
func ById(id string) db.Q {
	return db.Query(bson.D{{
		Key:   IdKey,
		Value: id,
	}})
}

func ByOldTaskID(id string) db.Q {
	return db.Query(bson.M{
		OldTaskIdKey: id,
	})
}

// ByIds creates a query that finds all tasks with the given ids.
func ByIds(ids []string) db.Q {
	return db.Query(bson.D{{
		Key:   IdKey,
		Value: bson.M{"$in": ids},
	}})
}

// ByBuildId creates a query to return tasks with a certain build id
func ByBuildId(buildId string) db.Q {
	return db.Query(bson.M{
		BuildIdKey: buildId,
	})
}

// ByAborted creates a query to return tasks with an aborted state
func ByAborted(aborted bool) db.Q {
	return db.Query(bson.M{
		AbortedKey: aborted,
	})
}

// ByAborted creates a query to return tasks with an aborted state
func ByActivation(active bool) db.Q {
	return db.Query(bson.M{
		ActivatedKey: active,
	})
}

// ByVersion creates a query to return tasks with a certain build id
func ByVersion(version string) db.Q {
	return db.Query(bson.M{
		VersionKey: version,
	})
}

// ByIdsBuildIdAndStatus creates a query to return tasks with a certain build id and statuses
func ByIdsBuildAndStatus(taskIds []string, buildId string, statuses []string) db.Q {
	return db.Query(bson.M{
		IdKey:      bson.M{"$in": taskIds},
		BuildIdKey: buildId,
		StatusKey: bson.M{
			"$in": statuses,
		},
	})
}

// ByCommit creates a query on Evergreen as the requester on a revision, buildVariant, displayName and project.
func ByCommit(revision, buildVariant, displayName, project, requester string) db.Q {
	return db.Query(bson.M{
		RevisionKey:     revision,
		RequesterKey:    requester,
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		ProjectKey:      project,
	})
}

// ByStatusAndActivation creates a query that returns tasks of a certain status and activation state.
func ByStatusAndActivation(status string, active bool) db.Q {
	return db.Query(bson.M{
		ActivatedKey: active,
		StatusKey:    status,
		//Filter out blacklisted tasks
		PriorityKey: bson.M{"$gte": 0},
	})
}

func ByVersionsForNameAndVariant(versions, displayNames []string, buildVariant string) db.Q {
	return db.Query(bson.M{
		VersionKey: bson.M{
			"$in": versions,
		},
		DisplayNameKey: bson.M{
			"$in": displayNames,
		},
		BuildVariantKey: buildVariant,
	})
}

// ByIntermediateRevisions creates a query that returns the tasks existing
// between two revision order numbers, exclusive.
func ByIntermediateRevisions(previousRevisionOrder, currentRevisionOrder int,
	buildVariant, displayName, project, requester string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey:    requester,
		RevisionOrderNumberKey: bson.M{
			"$lt": currentRevisionOrder,
			"$gt": previousRevisionOrder,
		},
		ProjectKey: project,
	})
}

func ByBeforeRevision(revisionOrder int, buildVariant, displayName, project, requester string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey:    requester,
		RevisionOrderNumberKey: bson.M{
			"$lt": revisionOrder,
		},
		ProjectKey: project,
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByBuildIdAfterTaskId provides a way to get an ordered list of tasks from a
// build. Providing a taskId allows indexing into the list of tasks that
// naturally exists when tasks are sorted by taskId.
func ByBuildIdAfterTaskId(buildId, taskId string) db.Q {
	return db.Query(bson.M{
		BuildIdKey: buildId,
		IdKey: bson.M{
			"$gte": taskId,
		},
	}).Sort([]string{"+" + IdKey})
}

func ByActivatedBeforeRevisionWithStatuses(revisionOrder int, statuses []string, buildVariant string, displayName string, project string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RevisionOrderNumberKey: bson.M{
			"$lt": revisionOrder,
		},
		StatusKey: bson.M{
			"$in": statuses,
		},
		ActivatedKey: true,
		ProjectKey:   project,
		RequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

func ByBeforeRevisionWithStatusesAndRequesters(revisionOrder int, statuses []string, buildVariant, displayName, project string, requesters []string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey: bson.M{
			"$in": requesters,
		},
		RevisionOrderNumberKey: bson.M{
			"$lt": revisionOrder,
		},
		StatusKey: bson.M{
			"$in": statuses,
		},
		ProjectKey: project,
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByTimeRun returns all tasks that are running in between two given times.
func ByTimeRun(startTime, endTime time.Time) db.Q {
	return db.Query(
		bson.M{
			"$or": []bson.M{
				bson.M{
					StartTimeKey:  bson.M{"$lte": endTime},
					FinishTimeKey: bson.M{"$gte": startTime},
					StatusKey:     evergreen.TaskFailed,
				},
				bson.M{
					StartTimeKey:  bson.M{"$lte": endTime},
					FinishTimeKey: bson.M{"$gte": startTime},
					StatusKey:     evergreen.TaskSucceeded,
				},
			}})
}

// ByTimeStartedAndFailed returns all failed tasks that started between 2 given times
func ByTimeStartedAndFailed(startTime, endTime time.Time, commandTypes []string) db.Q {
	query := bson.M{
		StartTimeKey: bson.M{"$lte": endTime},
		StartTimeKey: bson.M{"$gte": startTime},
		StatusKey:    evergreen.TaskFailed,
	}
	if len(commandTypes) > 0 {
		query[bsonutil.GetDottedKeyName(DetailsKey, "type")] = bson.M{
			"$in": commandTypes,
		}
	}
	return db.Query(query)
}

func ByStatuses(statuses []string, buildVariant, displayName, project, requester string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey:    requester,
		StatusKey: bson.M{
			"$in": statuses,
		},
		ProjectKey: project,
	})
}

// ByDifferentFailedBuildVariants returns a query for all failed tasks on a revision that are not of a buildVariant
func ByDifferentFailedBuildVariants(revision, buildVariant, displayName, project, requester string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: bson.M{
			"$ne": buildVariant,
		},
		DisplayNameKey: displayName,
		StatusKey:      evergreen.TaskFailed,
		ProjectKey:     project,
		RequesterKey:   requester,
		RevisionKey:    revision,
	})
}

func ByRecentlyFinished(finishTime time.Time, project string, requester string) db.Q {
	query := bson.M{}
	andClause := []bson.M{}

	// filter by finish_time
	timeOpt := bson.M{
		FinishTimeKey: bson.M{
			"$gt": finishTime,
		},
	}

	// filter by requester
	requesterOpt := bson.M{
		RequesterKey: requester,
	}

	// build query
	andClause = append(andClause, bson.M{
		"$or": FinishedOpts,
	})

	andClause = append(andClause, timeOpt)
	andClause = append(andClause, requesterOpt)

	// filter by project
	if project != "" {
		projectOpt := bson.M{
			ProjectKey: project,
		}
		andClause = append(andClause, projectOpt)
	}

	query["$and"] = andClause
	return db.Query(query)
}

// Returns query which targets list of tasks
// And allow filter by project_id, status, start_time (gte), finish_time (lte)
func WithinTimePeriod(startedAfter, finishedBefore time.Time, project string, statuses []string) db.Q {
	q := []bson.M{}

	if !startedAfter.IsZero() {
		q = append(q, bson.M{
			StartTimeKey: bson.M{
				"$gte": startedAfter,
			},
		})
	}

	// Filter by end date
	if !finishedBefore.IsZero() {
		q = append(q, bson.M{
			FinishTimeKey: bson.M{
				"$lte": finishedBefore,
			},
		})
	}

	// Filter by status
	if len(statuses) > 0 {
		q = append(q, bson.M{
			StatusKey: bson.M{
				"$in": statuses,
			},
		})
	}

	// Filter by project id
	if project != "" {
		q = append(q, bson.M{
			ProjectKey: project,
		})
	}

	return db.Query(bson.M{
		"$and": q,
	})
}

func ByDispatchedWithIdsVersionAndStatus(taskIds []string, versionId string, statuses []string) db.Q {
	return db.Query(bson.M{
		IdKey: bson.M{
			"$in": taskIds,
		},
		VersionKey:      versionId,
		DispatchTimeKey: bson.M{"$ne": util.ZeroTime},
		StatusKey:       bson.M{"$in": statuses},
	})
}

func ByExecutionTask(taskId string) db.Q {
	return db.Query(bson.M{
		ExecutionTasksKey: taskId,
	})
}

var (
	IsDispatchedOrStarted = db.Query(bson.M{
		StatusKey: bson.M{"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
	})
)

func scheduleableTasksQuery() bson.M {
	return bson.M{
		ActivatedKey: true,
		StatusKey:    evergreen.TaskUndispatched,
		//Filter out blacklisted tasks
		PriorityKey: bson.M{"$gte": 0},
	}
}

// TasksByProjectAndCommitPipeline fetches the pipeline to get the retrieve all tasks
// associated with a given project and commit hash.
func TasksByProjectAndCommitPipeline(projectId, commitHash, taskId, taskStatus string, limit int) []bson.M {
	pipeline := []bson.M{
		{"$match": bson.M{
			ProjectKey:  projectId,
			RevisionKey: commitHash,
			IdKey:       bson.M{"$gte": taskId},
		}},
	}
	if taskStatus != "" {
		statusMatch := bson.M{
			"$match": bson.M{StatusKey: taskStatus},
		}
		pipeline = append(pipeline, statusMatch)
	}
	if limit > 0 {
		limitStage := bson.M{
			"$limit": limit,
		}
		pipeline = append(pipeline, limitStage)
	}
	return pipeline
}

// TasksByBuildIdPipeline fetches the pipeline to get the retrieve all tasks
// associated with a given build.
func TasksByBuildIdPipeline(buildId, taskId, taskStatus string,
	limit, sortDir int) []bson.M {
	sortOperator := "$gte"
	if sortDir < 0 {
		sortOperator = "$lte"
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			BuildIdKey: buildId,
			IdKey:      bson.M{sortOperator: taskId},
		}},
	}

	// sort the tasks before limiting to get the next [limit] tasks
	pipeline = append(pipeline, bson.M{"$sort": bson.M{IdKey: sortDir}})

	if taskStatus != "" {
		statusMatch := bson.M{
			"$match": bson.M{StatusKey: taskStatus},
		}
		pipeline = append(pipeline, statusMatch)
	}
	if limit > 0 {
		limitStage := bson.M{
			"$limit": limit,
		}
		pipeline = append(pipeline, limitStage)
	}
	return pipeline
}

// CostDataByVersionIdPipeline returns an aggregation pipeline for fetching
// cost data (sum of time taken) from a version by its Id.
func CostDataByVersionIdPipeline(versionId string) []bson.M {
	pipeline := []bson.M{
		{"$match": bson.M{VersionKey: versionId}},
		{"$group": bson.M{
			"_id":                "$" + VersionKey,
			"sum_time_taken":     bson.M{"$sum": "$" + TimeTakenKey},
			"sum_estimated_cost": bson.M{"$sum": "$" + CostKey},
		}},
		{"$project": bson.M{
			"_id":                0,
			"version_id":         "$_id",
			"sum_time_taken":     1,
			"sum_estimated_cost": 1,
		}},
	}

	return pipeline
}

// CostDataByDistroIdPipeline returns an aggregation pipeline for fetching
// cost data (sum of time taken) from a distro by its Id.
func CostDataByDistroIdPipeline(distroId string, starttime time.Time, duration time.Duration) []bson.M {
	pipeline := []bson.M{
		{"$match": bson.M{
			DistroIdKey:   distroId,
			FinishTimeKey: bson.M{"$gte": starttime, "$lte": starttime.Add(duration)},
		}},
		{"$group": bson.M{
			"_id":                "$" + DistroIdKey,
			"sum_time_taken":     bson.M{"$sum": "$" + TimeTakenKey},
			"sum_estimated_cost": bson.M{"$sum": "$" + CostKey},
			"num_tasks":          bson.M{"$sum": 1},
		}},
		{"$project": bson.M{
			"_id":                0,
			"distro_id":          "$_id",
			"sum_time_taken":     1,
			"sum_estimated_cost": 1,
			"num_tasks":          1,
		}},
	}

	return pipeline
}

// FindCostTaskByProject fetches all tasks of a project matching the
// given time range, starting at task's IdKey in sortDir direction.
func FindCostTaskByProject(project, taskId string, starttime,
	endtime time.Time, limit, sortDir int) ([]Task, error) {
	sortSpec := IdKey // Sort on IdKey
	filter := bson.M{}
	filter[ProjectKey] = project
	filter[FinishTimeKey] = bson.M{"$gte": starttime, "$lte": endtime}
	if sortDir < 0 {
		sortSpec = "-" + sortSpec
		filter[IdKey] = bson.M{"$lt": taskId}
	} else {
		filter[IdKey] = bson.M{"$gte": taskId}
	}

	// Only project the fields relevant for the cost route
	projection := bson.M{
		DisplayNameKey:  1,
		DistroIdKey:     1,
		BuildVariantKey: 1,
		FinishTimeKey:   1,
		TimeTakenKey:    1,
		RevisionKey:     1,
		CostKey:         1,
	}

	tasks := []Task{} // Tasks to be returned
	err := db.FindAll(
		Collection,
		filter,
		projection,
		[]string{sortSpec},
		db.NoSkip,
		limit,
		&tasks,
	)
	return tasks, err
}

// GetRecentTasks returns the task results used by the recent_tasks endpoints.
func GetRecentTasks(period time.Duration) ([]Task, error) {
	query := db.Query(
		bson.M{
			StatusKey: bson.M{"$exists": true},
			FinishTimeKey: bson.M{
				"$gt": time.Now().Add(-period),
			},
		},
	)

	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if err != nil {
		return nil, errors.Wrap(err, "problem with stats query")
	}
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, nil
}

// DB Boilerplate

// FindOneNoMerge is a FindOne without merging test results.
func FindOneNoMerge(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(Collection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

// FindOne returns one task that satisfies the query.
func FindOne(query db.Q) (*Task, error) {
	task, err := FindOneNoMerge(query)
	if err != nil {
		return nil, errors.Wrap(err, "error finding task")
	}
	if task == nil {
		return nil, nil
	}
	if err = task.MergeNewTestResults(); err != nil {
		return nil, errors.Wrap(err, "errors merging new test results")
	}
	return task, err
}

func FindOneId(id string) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{IdKey: id})
	err := db.FindOneQ(Collection, query, task)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "error finding task by id")
	}

	return task, nil
}

func FindOneIdAndExecution(id string, execution int) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{
		IdKey:        id,
		ExecutionKey: execution,
	})
	err := db.FindOneQ(Collection, query, task)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "error finding task by id and execution")
	}

	return task, nil
}

// FindOneOldNoMerge is a FindOneOld without merging test results.
func FindOneOldNoMerge(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(OldCollection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

// FindOneOldNoMergeByIdAndExecution finds a task from the old tasks collection without test results.
func FindOneOldNoMergeByIdAndExecution(id string, execution int) (*Task, error) {
	query := db.Query(bson.M{
		OldTaskIdKey: id,
		ExecutionKey: execution,
	})
	return FindOneOldNoMerge(query)
}

func FindOneIdWithFields(id string, projected ...string) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{IdKey: id})

	if len(projected) > 0 {
		query = query.WithFields(projected...)
	}

	err := db.FindOneQ(Collection, query, task)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return task, nil
}

func findAllTaskIDs(q db.Q) ([]string, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, q, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "error finding task ids for versions")
	}

	ids := []string{}
	for _, t := range tasks {
		ids = append(ids, t.Id)
	}

	return ids, nil
}

func FindAllTaskIDsFromVersion(versionId string) ([]string, error) {
	q := db.Query(bson.M{VersionKey: versionId}).WithFields(IdKey)
	return findAllTaskIDs(q)
}

// FindAllTasksFromVersionWithDependencies finds all tasks in a version and includes only their dependencies.
func FindAllTasksFromVersionWithDependencies(versionId string) ([]Task, error) {
	q := db.Query(bson.M{
		VersionKey: versionId,
	}).WithFields(IdKey, DependsOnKey)
	tasks := []Task{}
	err := db.FindAllQ(Collection, q, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "error finding task ids for versions")
	}
	return tasks, nil
}

func FindAllTaskIDsFromBuild(buildId string) ([]string, error) {
	q := db.Query(bson.M{BuildIdKey: buildId}).WithFields(IdKey)
	return findAllTaskIDs(q)
}

// FindTasksFromBuildWithDependencies finds tasks from a build that have dependencies.
func FindTasksFromBuildWithDependencies(buildId string) ([]Task, error) {
	q := db.Query(bson.M{
		BuildIdKey:   buildId,
		DependsOnKey: bson.M{"$not": bson.M{"$size": 0}},
	}).WithFields(IdKey, DependsOnKey)
	tasks := []Task{}
	err := db.FindAllQ(Collection, q, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "error finding task ids for versions")
	}
	return tasks, nil
}

// FindOneOld returns one task from the old tasks collection that satisfies the query.
func FindOneOld(query db.Q) (*Task, error) {
	task, err := FindOneOldNoMerge(query)
	if err != nil {
		return nil, errors.Wrap(err, "error finding task")
	}
	if task == nil {
		return nil, nil
	}
	if err = task.MergeNewTestResults(); err != nil {
		return nil, errors.Wrap(err, "errors merging new test results")
	}
	return task, err
}

// FindOld returns all task from the old tasks collection that satisfies the query.
func FindOld(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	for i, task := range tasks {
		if err = task.MergeNewTestResults(); err != nil {
			return nil, errors.Wrap(err, "error merging new test results")
		}
		tasks[i] = task
	}

	// remove display tasks from results
	for i := len(tasks) - 1; i >= 0; i-- {
		t := tasks[i]
		if t.DisplayOnly {
			tasks = append(tasks[:i], tasks[i+1:]...)
		}
	}
	return tasks, err
}

// FindOldWithDisplayTasks finds display and execution tasks in the old collection
func FindOldWithDisplayTasks(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	for i, task := range tasks {
		if err = task.MergeNewTestResults(); err != nil {
			return nil, errors.Wrap(err, "error merging new test results")
		}
		tasks[i] = task
	}

	return tasks, err
}

// FindOneIdOldOrNew attempts to find a given task ID by first looking in the
// old collection, then the tasks collection
func FindOneIdOldOrNew(id string, execution int) (*Task, error) {
	task, err := FindOneOld(ById(fmt.Sprintf("%s_%d", id, execution)))
	if task == nil || err != nil {
		return FindOne(ById(id))
	}

	return task, err
}

// Find returns all tasks that satisfy the query.
func Find(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	filtered := []Task{}

	// remove display tasks from results
	for idx := range tasks {
		t := tasks[idx]
		if t.DisplayOnly {
			continue
		}
		filtered = append(filtered, t)

	}

	return filtered, err
}

// Find returns really all tasks that satisfy the query.
func FindAll(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tasks, err
}

func FindWithDisplayTasks(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	for _, t := range tasks {
		_, err = t.GetDisplayTask()
		if err != nil {
			return nil, errors.Wrap(err, "unable to retrieve parent display task")
		}
	}

	return tasks, err
}

// UpdateOne updates one task.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

func UpdateAll(query interface{}, update interface{}) (*adb.ChangeInfo, error) {
	return db.UpdateAll(
		Collection,
		query,
		update,
	)
}

// Remove deletes the task of the given id from the database
func Remove(id string) error {
	return db.Remove(
		Collection,
		bson.M{IdKey: id},
	)
}

// Remove all deletes all tasks with a given buildId
func RemoveAllWithBuild(buildId string) error {
	return db.RemoveAll(
		Collection,
		bson.M{BuildIdKey: buildId})
}

func Aggregate(pipeline []bson.M, results interface{}) error {
	return db.Aggregate(
		Collection,
		pipeline,
		results)
}

// Count returns the number of hosts that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}
