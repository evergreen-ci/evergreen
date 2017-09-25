package task

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection    = "tasks"
	OldCollection = "old_tasks"
	TestLogPath   = "/test_log/"
)

var (
	// BSON fields for the task struct
	IdKey                  = bsonutil.MustHaveTag(Task{}, "Id")
	SecretKey              = bsonutil.MustHaveTag(Task{}, "Secret")
	CreateTimeKey          = bsonutil.MustHaveTag(Task{}, "CreateTime")
	DispatchTimeKey        = bsonutil.MustHaveTag(Task{}, "DispatchTime")
	PushTimeKey            = bsonutil.MustHaveTag(Task{}, "PushTime")
	ScheduledTimeKey       = bsonutil.MustHaveTag(Task{}, "ScheduledTime")
	StartTimeKey           = bsonutil.MustHaveTag(Task{}, "StartTime")
	FinishTimeKey          = bsonutil.MustHaveTag(Task{}, "FinishTime")
	VersionKey             = bsonutil.MustHaveTag(Task{}, "Version")
	ProjectKey             = bsonutil.MustHaveTag(Task{}, "Project")
	RevisionKey            = bsonutil.MustHaveTag(Task{}, "Revision")
	LastHeartbeatKey       = bsonutil.MustHaveTag(Task{}, "LastHeartbeat")
	ActivatedKey           = bsonutil.MustHaveTag(Task{}, "Activated")
	BuildIdKey             = bsonutil.MustHaveTag(Task{}, "BuildId")
	DistroIdKey            = bsonutil.MustHaveTag(Task{}, "DistroId")
	BuildVariantKey        = bsonutil.MustHaveTag(Task{}, "BuildVariant")
	DependsOnKey           = bsonutil.MustHaveTag(Task{}, "DependsOn")
	NumDepsKey             = bsonutil.MustHaveTag(Task{}, "NumDependents")
	DisplayNameKey         = bsonutil.MustHaveTag(Task{}, "DisplayName")
	HostIdKey              = bsonutil.MustHaveTag(Task{}, "HostId")
	ExecutionKey           = bsonutil.MustHaveTag(Task{}, "Execution")
	RestartsKey            = bsonutil.MustHaveTag(Task{}, "Restarts")
	OldTaskIdKey           = bsonutil.MustHaveTag(Task{}, "OldTaskId")
	ArchivedKey            = bsonutil.MustHaveTag(Task{}, "Archived")
	RevisionOrderNumberKey = bsonutil.MustHaveTag(Task{}, "RevisionOrderNumber")
	RequesterKey           = bsonutil.MustHaveTag(Task{}, "Requester")
	StatusKey              = bsonutil.MustHaveTag(Task{}, "Status")
	DetailsKey             = bsonutil.MustHaveTag(Task{}, "Details")
	AbortedKey             = bsonutil.MustHaveTag(Task{}, "Aborted")
	TimeTakenKey           = bsonutil.MustHaveTag(Task{}, "TimeTaken")
	ExpectedDurationKey    = bsonutil.MustHaveTag(Task{}, "ExpectedDuration")
	TestResultsKey         = bsonutil.MustHaveTag(Task{}, "TestResults")
	PriorityKey            = bsonutil.MustHaveTag(Task{}, "Priority")
	ActivatedByKey         = bsonutil.MustHaveTag(Task{}, "ActivatedBy")
	CostKey                = bsonutil.MustHaveTag(Task{}, "Cost")

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
	return db.Query(bson.D{{IdKey, id}})
}

// ByIds creates a query that finds all tasks with the given ids.
func ByIds(ids []string) db.Q {
	return db.Query(bson.D{{IdKey, bson.D{{"$in", ids}}}})
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

// ByRunningLastHeartbeat creates a query that finds any running tasks whose last heartbeat
// was at least the specified threshold ago
func ByRunningLastHeartbeat(threshold time.Time) db.Q {
	return db.Query(bson.M{
		StatusKey:        SelectorTaskInProgress,
		LastHeartbeatKey: bson.M{"$lte": threshold},
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

func ByOrderNumbersForNameAndVariant(revisionOrder []int, displayName, buildVariant string) db.Q {
	return db.Query(bson.M{
		RevisionOrderNumberKey: bson.M{
			"$in": revisionOrder,
		},
		DisplayNameKey:  displayName,
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

func ByBeforeRevisionWithStatuses(revisionOrder int, statuses []string, buildVariant, displayName, project string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RevisionOrderNumberKey: bson.M{
			"$lt": revisionOrder,
		},
		StatusKey: bson.M{
			"$in": statuses,
		},
		ProjectKey: project,
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

func ByActivatedBeforeRevisionWithStatuses(revisionOrder int, statuses []string, buildVariant, displayName, project string) db.Q {
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
	}).Sort([]string{"-" + RevisionOrderNumberKey})
}

func ByBeforeRevisionWithStatusesAndRequester(revisionOrder int, statuses []string, buildVariant, displayName, project, requester string) db.Q {
	return db.Query(bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey:    requester,
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
func ByTimeStartedAndFailed(startTime, endTime time.Time) db.Q {
	return db.Query(bson.M{
		StartTimeKey: bson.M{"$lte": endTime},
		StartTimeKey: bson.M{"$gte": startTime},
		StatusKey:    evergreen.TaskFailed,
	})
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

var (
	IsUndispatched        = ByStatusAndActivation(evergreen.TaskUndispatched, true)
	IsDispatchedOrStarted = db.Query(bson.M{
		StatusKey: bson.M{"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
	})
)

// TODO Rewrite this rest v2 route for new test results collection after migration
// getTestResultsPipeline returns an aggregation pipeline for fetching a list
// of test from a task by its Id.
func TestResultsByTaskIdPipeline(taskId, testFilename, testStatus string, limit,
	sortDir int) []bson.M {
	sortOperator := "$gte"
	if sortDir < 0 {
		sortOperator = "$lte"
	}
	pipeline := []bson.M{
		{"$match": bson.M{"_id": taskId}},
		{"$unwind": fmt.Sprintf("$%s", TestResultsKey)},
		{"$project": bson.M{
			"status":    fmt.Sprintf("$%s.%s", TestResultsKey, TestResultStatusKey),
			"test_file": fmt.Sprintf("$%s.%s", TestResultsKey, TestResultTestFileKey),
			"log_id":    fmt.Sprintf("$%s.%s", TestResultsKey, TestResultLogIdKey),
			"line_num":  fmt.Sprintf("$%s.%s", TestResultsKey, TestResultLineNumKey),
			"exit_code": fmt.Sprintf("$%s.%s", TestResultsKey, TestResultExitCodeKey),
			"url":       fmt.Sprintf("$%s.%s", TestResultsKey, TestResultURLKey),
			"url_raw":   fmt.Sprintf("$%s.%s", TestResultsKey, TestResultURLRawKey),
			"start":     fmt.Sprintf("$%s.%s", TestResultsKey, TestResultStartTimeKey),
			"end":       fmt.Sprintf("$%s.%s", TestResultsKey, TestResultEndTimeKey),
			"_id":       0,
		}},
	}
	if testStatus != "" {
		statusMatch := bson.M{
			"$match": bson.M{TestResultStatusKey: testStatus},
		}
		pipeline = append(pipeline, statusMatch)
	}
	equalityStage := bson.M{
		"$match": bson.M{TestResultTestFileKey: bson.M{sortOperator: testFilename}},
	}
	pipeline = append(pipeline, equalityStage)
	sortStage := bson.M{
		"$sort": bson.M{TestResultTestFileKey: 1},
	}
	pipeline = append(pipeline, sortStage)
	if limit > 0 {
		limitStage := bson.M{
			"$limit": limit,
		}
		pipeline = append(pipeline, limitStage)
	}
	return pipeline
}

// TasksByProjectAndCommitPipeline fetches the pipeline to get the retrieve all tasks
// associated with a given project and commit hash.
func TasksByProjectAndCommitPipeline(projectId, commitHash, taskId, taskStatus string,
	limit, sortDir int) []bson.M {
	sortOperator := "$gte"
	if sortDir < 0 {
		sortOperator = "$lte"
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			ProjectKey:  projectId,
			RevisionKey: commitHash,
			IdKey:       bson.M{sortOperator: taskId},
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
		}},
		{"$project": bson.M{
			"_id":                0,
			"distro_id":          "$_id",
			"sum_time_taken":     1,
			"sum_estimated_cost": 1,
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

// GetRecentTasks runs a query, returning a very limited projection of
// the task results used in the GetTaskResultCounts query to support
// returning tasks. Most fields are projected out.
func GetRecentTasks(period time.Duration) ([]Task, error) {
	query := db.Query(
		bson.M{
			StatusKey: bson.M{"$exists": true},
			FinishTimeKey: bson.M{
				"$gt": time.Now().Add(-period),
			},
		},
	).Project(
		bson.M{
			IdKey:          0,
			StatusKey:      1,
			DetailsKey:     1,
			ActivatedKey:   1,
			TestResultsKey: 1,
		})

	out, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "problem with stats query")
	}

	return out, nil
}

// DB Boilerplate

// FindOne returns one task that satisfies the query.
func FindOne(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(Collection, query, task)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err := task.MergeNewTestResults(); err != nil {
		return nil, errors.Wrap(err, "errors merging new test results")
	}
	return task, err
}

// FindOneOld returns one task from the old tasks collection that satisfies the query.
func FindOneOld(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(OldCollection, query, task)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err := task.MergeNewTestResults(); err != nil {
		return nil, errors.Wrap(err, "errors merging new test results")
	}
	return task, err
}

// FindOld returns all task from the old tasks collection that satisfies the query.
func FindOld(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	for i, task := range tasks {
		if err := task.MergeNewTestResults(); err != nil {
			return nil, errors.Wrap(err, "error merging new test results")
		}
		tasks[i] = task
	}
	return tasks, err
}

// Find returns all tasks that satisfy the query.
func Find(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	for i, task := range tasks {
		if err := task.MergeNewTestResults(); err != nil {
			return nil, errors.Wrap(err, "error merging new test results")
		}
		tasks[i] = task
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

func UpdateAll(query interface{}, update interface{}) (*mgo.ChangeInfo, error) {
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
