package task

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/util"
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
	TestResultTestFileKey  = bsonutil.MustHaveTag(TestResult{}, "TestFile")
	TestResultURLKey       = bsonutil.MustHaveTag(TestResult{}, "URL")
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

// DB Boilerplate

// FindOne returns one task that satisfies the query.
func FindOne(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(Collection, query, task)
	if err == mgo.ErrNotFound {
		return nil, nil
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
	return task, err
}

// FindOld returns all task from the old tasks collection that satisfies the query.
func FindOld(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if err == mgo.ErrNotFound {
		return nil, nil
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

// Count returns the number of hosts that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}
