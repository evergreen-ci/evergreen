package task

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	Collection    = "tasks"
	OldCollection = "old_tasks"
)

var (
	// BSON fields for the task struct
	IdKey                       = bsonutil.MustHaveTag(Task{}, "Id")
	SecretKey                   = bsonutil.MustHaveTag(Task{}, "Secret")
	CreateTimeKey               = bsonutil.MustHaveTag(Task{}, "CreateTime")
	DispatchTimeKey             = bsonutil.MustHaveTag(Task{}, "DispatchTime")
	ScheduledTimeKey            = bsonutil.MustHaveTag(Task{}, "ScheduledTime")
	StartTimeKey                = bsonutil.MustHaveTag(Task{}, "StartTime")
	FinishTimeKey               = bsonutil.MustHaveTag(Task{}, "FinishTime")
	ActivatedTimeKey            = bsonutil.MustHaveTag(Task{}, "ActivatedTime")
	DependenciesMetTimeKey      = bsonutil.MustHaveTag(Task{}, "DependenciesMetTime")
	VersionKey                  = bsonutil.MustHaveTag(Task{}, "Version")
	ProjectKey                  = bsonutil.MustHaveTag(Task{}, "Project")
	RevisionKey                 = bsonutil.MustHaveTag(Task{}, "Revision")
	LastHeartbeatKey            = bsonutil.MustHaveTag(Task{}, "LastHeartbeat")
	ActivatedKey                = bsonutil.MustHaveTag(Task{}, "Activated")
	DeactivatedForDependencyKey = bsonutil.MustHaveTag(Task{}, "DeactivatedForDependency")
	BuildIdKey                  = bsonutil.MustHaveTag(Task{}, "BuildId")
	DistroIdKey                 = bsonutil.MustHaveTag(Task{}, "DistroId")
	DistroAliasesKey            = bsonutil.MustHaveTag(Task{}, "DistroAliases")
	BuildVariantKey             = bsonutil.MustHaveTag(Task{}, "BuildVariant")
	DependsOnKey                = bsonutil.MustHaveTag(Task{}, "DependsOn")
	OverrideDependenciesKey     = bsonutil.MustHaveTag(Task{}, "OverrideDependencies")
	NumDepsKey                  = bsonutil.MustHaveTag(Task{}, "NumDependents")
	DisplayNameKey              = bsonutil.MustHaveTag(Task{}, "DisplayName")
	HostIdKey                   = bsonutil.MustHaveTag(Task{}, "HostId")
	AgentVersionKey             = bsonutil.MustHaveTag(Task{}, "AgentVersion")
	ExecutionKey                = bsonutil.MustHaveTag(Task{}, "Execution")
	RestartsKey                 = bsonutil.MustHaveTag(Task{}, "Restarts")
	OldTaskIdKey                = bsonutil.MustHaveTag(Task{}, "OldTaskId")
	ArchivedKey                 = bsonutil.MustHaveTag(Task{}, "Archived")
	RevisionOrderNumberKey      = bsonutil.MustHaveTag(Task{}, "RevisionOrderNumber")
	RequesterKey                = bsonutil.MustHaveTag(Task{}, "Requester")
	StatusKey                   = bsonutil.MustHaveTag(Task{}, "Status")
	DetailsKey                  = bsonutil.MustHaveTag(Task{}, "Details")
	AbortedKey                  = bsonutil.MustHaveTag(Task{}, "Aborted")
	AbortInfoKey                = bsonutil.MustHaveTag(Task{}, "AbortInfo")
	TimeTakenKey                = bsonutil.MustHaveTag(Task{}, "TimeTaken")
	ExpectedDurationKey         = bsonutil.MustHaveTag(Task{}, "ExpectedDuration")
	ExpectedDurationStddevKey   = bsonutil.MustHaveTag(Task{}, "ExpectedDurationStdDev")
	DurationPredictionKey       = bsonutil.MustHaveTag(Task{}, "DurationPrediction")
	PriorityKey                 = bsonutil.MustHaveTag(Task{}, "Priority")
	ActivatedByKey              = bsonutil.MustHaveTag(Task{}, "ActivatedBy")
	ExecutionTasksKey           = bsonutil.MustHaveTag(Task{}, "ExecutionTasks")
	ExecutionTasksFullKey       = bsonutil.MustHaveTag(Task{}, "ExecutionTasksFull")
	DisplayOnlyKey              = bsonutil.MustHaveTag(Task{}, "DisplayOnly")
	TaskGroupKey                = bsonutil.MustHaveTag(Task{}, "TaskGroup")
	TaskGroupMaxHostsKey        = bsonutil.MustHaveTag(Task{}, "TaskGroupMaxHosts")
	TaskGroupOrderKey           = bsonutil.MustHaveTag(Task{}, "TaskGroupOrder")
	GenerateTaskKey             = bsonutil.MustHaveTag(Task{}, "GenerateTask")
	GeneratedTasksKey           = bsonutil.MustHaveTag(Task{}, "GeneratedTasks")
	GeneratedByKey              = bsonutil.MustHaveTag(Task{}, "GeneratedBy")
	HasLegacyResultsKey         = bsonutil.MustHaveTag(Task{}, "HasLegacyResults")
	HasCedarResultsKey          = bsonutil.MustHaveTag(Task{}, "HasCedarResults")
	CedarResultsFailedKey       = bsonutil.MustHaveTag(Task{}, "CedarResultsFailed")
	IsGithubCheckKey            = bsonutil.MustHaveTag(Task{}, "IsGithubCheck")
	HostCreateDetailsKey        = bsonutil.MustHaveTag(Task{}, "HostCreateDetails")

	// GeneratedJSONKey is no longer used but must be kept for old tasks.
	GeneratedJSONKey            = bsonutil.MustHaveTag(Task{}, "GeneratedJSON")
	GeneratedJSONAsStringKey    = bsonutil.MustHaveTag(Task{}, "GeneratedJSONAsString")
	GenerateTasksErrorKey       = bsonutil.MustHaveTag(Task{}, "GenerateTasksError")
	GeneratedTasksToActivateKey = bsonutil.MustHaveTag(Task{}, "GeneratedTasksToActivate")
	ResetWhenFinishedKey        = bsonutil.MustHaveTag(Task{}, "ResetWhenFinished")
	LogsKey                     = bsonutil.MustHaveTag(Task{}, "Logs")
	CommitQueueMergeKey         = bsonutil.MustHaveTag(Task{}, "CommitQueueMerge")
	DisplayStatusKey            = bsonutil.MustHaveTag(Task{}, "DisplayStatus")
	BaseTaskKey                 = bsonutil.MustHaveTag(Task{}, "BaseTask")
	BuildVariantDisplayNameKey  = bsonutil.MustHaveTag(Task{}, "BuildVariantDisplayName")

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

var (
	// BSON fields for task dependency struct
	DependencyTaskIdKey       = bsonutil.MustHaveTag(Dependency{}, "TaskId")
	DependencyStatusKey       = bsonutil.MustHaveTag(Dependency{}, "Status")
	DependencyUnattainableKey = bsonutil.MustHaveTag(Dependency{}, "Unattainable")
)

var BaseTaskStatusKey = bsonutil.GetDottedKeyName(BaseTaskKey, StatusKey)

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

	// Checks if task dependencies are attainable/ have met all their dependencies and are not blocked
	isAttainable = bson.M{
		"$ne": []interface{}{bson.M{"$reduce": bson.M{
			"input":        "$" + DependsOnKey,
			"initialValue": false,
			"in":           bson.M{"$or": []interface{}{"$$" + bsonutil.GetDottedKeyName("value", DependencyUnattainableKey), "$$" + bsonutil.GetDottedKeyName("this", DependencyUnattainableKey)}},
		},
		}, true},
	}
	// This should reflect Task.GetDisplayStatus()
	addDisplayStatus = bson.M{
		"$addFields": bson.M{
			DisplayStatusKey: displayStatusExpression,
		},
	}

	displayStatusExpression = bson.M{
		"$switch": bson.M{
			"branches": []bson.M{
				{
					"case": bson.M{
						"$ne": []interface{}{
							bson.M{
								"$size": bson.M{"$ifNull": []interface{}{"$annotation_docs", []bson.M{}}},
							}, 0,
						},
					},
					"then": evergreen.TaskKnownIssue,
				},
				{
					"case": bson.M{
						"$eq": []interface{}{"$" + AbortedKey, true},
					},
					"then": evergreen.TaskAborted,
				},
				{
					"case": bson.M{
						"$eq": []string{"$" + StatusKey, evergreen.TaskSucceeded},
					},
					"then": evergreen.TaskSucceeded,
				},
				{
					"case": bson.M{
						"$eq": []string{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailType), evergreen.CommandTypeSetup},
					},
					"then": evergreen.TaskSetupFailed,
				},
				{
					"case": bson.M{
						"$and": []bson.M{
							{"$eq": []string{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailType), evergreen.CommandTypeSystem}},
							{"$eq": []interface{}{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut), true}},
							{"$eq": []string{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailDescription), evergreen.TaskDescriptionHeartbeat}},
						},
					},
					"then": evergreen.TaskSystemUnresponse,
				},
				{
					"case": bson.M{
						"$and": []bson.M{
							{"$eq": []string{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailType), evergreen.CommandTypeSystem}},
							{"$eq": []interface{}{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut), true}},
						},
					},
					"then": evergreen.TaskSystemTimedOut,
				},
				{
					"case": bson.M{
						"$eq": []string{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailType), evergreen.CommandTypeSystem},
					},
					"then": evergreen.TaskSystemFailed,
				},
				{
					"case": bson.M{
						"$eq": []interface{}{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut), true},
					},
					"then": evergreen.TaskTimedOut,
				},
				// A task will be unscheduled if it is not activated
				{
					"case": bson.M{
						"$and": []bson.M{
							{"$eq": []interface{}{"$" + ActivatedKey, false}},
							{"$eq": []string{"$" + StatusKey, evergreen.TaskUndispatched}},
						},
					},
					"then": evergreen.TaskUnscheduled,
				},
				// A task will be blocked if it has dependencies that are not attainable
				{
					"case": bson.M{
						"$and": []bson.M{
							{"$eq": []string{"$" + StatusKey, evergreen.TaskUndispatched}},
							{"$eq": []interface{}{isAttainable, false}},
						},
					},
					"then": evergreen.TaskStatusBlocked,
				},
				// A task will run if it is activated and does not have any blocking deps
				{
					"case": bson.M{
						"$and": []bson.M{
							{"$eq": []string{"$" + StatusKey, evergreen.TaskUndispatched}},
							{"$eq": []interface{}{"$" + ActivatedKey, true}},
						},
					},
					"then": evergreen.TaskWillRun,
				},
			},
			"default": "$" + StatusKey,
		},
	}

	AddBuildVariantDisplayName = []bson.M{
		bson.M{"$lookup": bson.M{
			"from":         "builds",
			"localField":   BuildIdKey,
			"foreignField": "_id",
			"as":           BuildVariantDisplayNameKey,
		}},
		bson.M{"$unwind": bson.M{
			"path":                       "$" + BuildVariantDisplayNameKey,
			"preserveNullAndEmptyArrays": true,
		}},
		bson.M{"$addFields": bson.M{
			BuildVariantDisplayNameKey: "$" + bsonutil.GetDottedKeyName(BuildVariantDisplayNameKey, "display_name"),
		}},
	}
)

var StatusFields = []string{
	BuildIdKey,
	DisplayNameKey,
	StatusKey,
	DetailsKey,
	StartTimeKey,
	TimeTakenKey,
	ActivatedKey,
	DependsOnKey,
}

// ById creates a query that finds a task by its _id.
func ById(id string) db.Q {
	return db.Query(bson.D{{
		Key:   IdKey,
		Value: id,
	}})
}

// ById creates a query that finds a task by its old task id.
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

func ByBuildIdAndGithubChecks(buildId string) db.Q {
	return db.Query(bson.M{
		BuildIdKey:       buildId,
		IsGithubCheckKey: true,
	})
}

// ByBuildIds creates a query to return tasks in buildsIds
func ByBuildIds(buildIds []string) db.Q {
	return db.Query(bson.M{
		BuildIdKey: bson.M{"$in": buildIds},
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

// ByVersion produces a query that returns tasks for the given version.
func ByVersions(versions []string) db.Q {
	return db.Query(bson.M{VersionKey: bson.M{"$in": versions}})
}

// ByIdsBuildIdAndStatus creates a query to return tasks with a certain build id and statuses
func ByIdsAndStatus(taskIds []string, statuses []string) db.Q {
	return db.Query(bson.M{
		IdKey: bson.M{"$in": taskIds},
		StatusKey: bson.M{
			"$in": statuses,
		},
	})
}

type StaleReason int

const (
	HeartbeatPastCutoff StaleReason = iota
	NoHeartbeatSinceDispatch
)

// ByStaleRunningTask creates a query that finds any running tasks
// whose last heartbeat was at least the specified threshold ago, or
// that has been dispatched but hasn't started in twice that long.
func ByStaleRunningTask(staleness time.Duration, reason StaleReason) db.Q {
	var reasonQuery bson.M
	switch reason {
	case HeartbeatPastCutoff:
		reasonQuery = bson.M{
			StatusKey:        SelectorTaskInProgress,
			DisplayOnlyKey:   bson.M{"$ne": true},
			LastHeartbeatKey: bson.M{"$lte": time.Now().Add(-staleness)},
		}
	case NoHeartbeatSinceDispatch:
		reasonQuery = bson.M{
			StatusKey:       evergreen.TaskDispatched,
			DisplayOnlyKey:  bson.M{"$ne": true},
			DispatchTimeKey: bson.M{"$lte": time.Now().Add(-2 * staleness)},
		}
	}
	return db.Query(reasonQuery)
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
	})
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
// If task not started (but is failed), returns if finished within the time range
func ByTimeStartedAndFailed(startTime, endTime time.Time, commandTypes []string) db.Q {
	query := bson.M{
		"$or": []bson.M{
			{"$and": []bson.M{
				{StartTimeKey: bson.M{"$lte": endTime}},
				{StartTimeKey: bson.M{"$gte": startTime}},
			}},
			{"$and": []bson.M{
				{StartTimeKey: time.Time{}},
				{FinishTimeKey: bson.M{"$lte": endTime}},
				{FinishTimeKey: bson.M{"$gte": startTime}},
			}},
		},
		StatusKey: evergreen.TaskFailed,
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

func ByExecutionTask(taskId string) db.Q {
	return db.Query(bson.M{
		ExecutionTasksKey: taskId,
	})
}

func ByExecutionTasks(ids []string) db.Q {
	return db.Query(bson.M{
		ExecutionTasksKey: bson.M{
			"$in": ids,
		},
	})
}

func BySubsetAborted(ids []string) db.Q {
	return db.Query(bson.M{
		IdKey:      bson.M{"$in": ids},
		AbortedKey: true,
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

		// Filter out tasks disabled by negative priority
		PriorityKey: bson.M{"$gt": evergreen.DisabledTaskPriority},

		// Filter tasks containing unattainable dependencies
		"$or": []bson.M{
			{bsonutil.GetDottedKeyName(DependsOnKey, DependencyUnattainableKey): bson.M{"$ne": true}},
			{OverrideDependenciesKey: true},
		},
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

type Stat struct {
	Name  string `bson:"name"`
	Count int    `bson:"count"`
}

type StatusItem struct {
	Status string `bson:"status"`
	Stats  []Stat `bson:"stats"`
}

func GetRecentTaskStats(period time.Duration, nameKey string) ([]StatusItem, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			StatusKey: bson.M{"$exists": true},
			FinishTimeKey: bson.M{
				"$gt": time.Now().Add(-period),
			},
		}},
		{"$group": bson.M{
			"_id":   bson.M{"status": "$" + StatusKey, "name": "$" + nameKey},
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{
			"count": -1,
		}},
		{"$group": bson.M{
			"_id":   "$_id.status",
			"stats": bson.M{"$push": bson.M{"name": "$_id.name", "count": "$count"}},
		}},
		{"$project": bson.M{
			"_id":    0,
			"status": "$_id",
			"stats":  1,
		}},
	}

	result := []StatusItem{}
	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "can't get stats list")
	}

	return result, nil
}

// FindUniqueBuildVariantNamesByTask returns  a list of unique build variants names for a given task name
func FindUniqueBuildVariantNamesByTask(projectId string, taskName string) ([]string, error) {
	buildVariantsKey := "build_variants"
	pipeline := []bson.M{
		{"$match": bson.M{
			ProjectKey:     projectId,
			DisplayNameKey: taskName,
			RequesterKey:   bson.M{"$in": evergreen.SystemVersionRequesterTypes}},
		}}

	group := bson.M{
		"$group": bson.M{
			"_id":            taskName,
			buildVariantsKey: bson.M{"$addToSet": "$" + BuildVariantKey},
		},
	}
	unwindAndSort := []bson.M{
		{
			"$unwind": "$build_variants",
		},
		{
			"$sort": bson.M{
				"build_variants": 1,
			},
		},
		{
			"$group": bson.M{
				"_id":            nil,
				buildVariantsKey: bson.M{"$push": "$" + buildVariantsKey},
			},
		},
	}

	pipeline = append(pipeline, group)
	pipeline = append(pipeline, unwindAndSort...)

	type taskBuildVariants struct {
		BuildVariants []string `bson:"build_variants"`
	}

	result := []taskBuildVariants{}
	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "can't get build variant tasks")
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result[0].BuildVariants, nil
}

// FindTaskNamesByBuildVariant returns a list of unique task names for a given build variant
func FindTaskNamesByBuildVariant(projectId string, buildVariant string) ([]string, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			ProjectKey:      projectId,
			BuildVariantKey: buildVariant,
			RequesterKey:    bson.M{"$in": evergreen.SystemVersionRequesterTypes}},
		}}

	group := bson.M{
		"$group": bson.M{
			"_id":   buildVariant,
			"tasks": bson.M{"$addToSet": "$" + DisplayNameKey},
		},
	}
	unwindAndSort := []bson.M{
		{
			"$unwind": "$tasks",
		},
		{
			"$sort": bson.M{
				"tasks": 1,
			},
		},
		{
			"$group": bson.M{
				"_id":   nil,
				"tasks": bson.M{"$push": "$tasks"},
			},
		},
	}

	pipeline = append(pipeline, group)
	pipeline = append(pipeline, unwindAndSort...)

	type buildVariantTasks struct {
		Tasks []string `bson:"tasks"`
	}

	result := []buildVariantTasks{}
	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "can't get build variant tasks")
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result[0].Tasks, nil
}

// DB Boilerplate

// FindOneNoMerge returns a single task that satisfies the given query without
// merging the test results.
func FindOneNoMerge(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(Collection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

// FindOne returns a single task that satisfies the given query and merges test
// results.
func FindOne(query db.Q) (*Task, error) {
	task, err := FindOneNoMerge(query)
	if err != nil {
		return nil, errors.Wrap(err, "finding task")
	}
	if task == nil {
		return nil, nil
	}
	if err = task.MergeNewTestResults(); err != nil {
		return nil, errors.Wrapf(err, "merging new test results for '%s'", task.Id)
	}
	return task, err
}

// FindOneOldNoMerge returns a single task from the old tasks collection that
// satisfies the given query without merging test results.
func FindOneOldNoMerge(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(OldCollection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

// FindOneOld returns a single task from the old tasks collection that
// satisfies the query and merges test results.
func FindOneOld(query db.Q) (*Task, error) {
	task, err := FindOneOldNoMerge(query)
	if err != nil {
		return nil, errors.Wrap(err, "finding task")
	}
	if task == nil {
		return nil, nil
	}
	if err = task.MergeNewTestResults(); err != nil {
		return nil, errors.Wrapf(err, "merging new test results for '%s'", task.Id)
	}
	return task, err
}

// FindOneIdNoMerge returns one task with the given id without merging test
// results.
func FindOneIdNoMerge(id string) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{IdKey: id})
	err := db.FindOneQ(Collection, query, task)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding task by id")
	}

	return task, nil
}

// FindByIdExecutionNoMerge returns a single task with the given id and
// execution without merging test results. If execution is nil, the latest
// execution of the task is returned.
func FindByIdExecutionNoMerge(id string, execution *int) (*Task, error) {
	if execution == nil {
		return FindOneIdNoMerge(id)
	}
	return FindOneIdAndExecutionNoMerge(id, *execution)
}

// FindOneIdAndExecutionNoMerge returns a single task with the given id and
// execution without merging test results.
func FindOneIdAndExecutionNoMerge(id string, execution int) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{
		IdKey:        id,
		ExecutionKey: execution,
	})
	err := db.FindOneQ(Collection, query, task)

	if adb.ResultsNotFound(err) {
		return FindOneOldByIdAndExecutionNoMerge(id, execution)
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding task by id and execution")
	}

	return task, nil
}

// FindOneIdAndExecutionWithDisplayStatusNoMerge returns a single task with the
// given id and execution with display statuses added and without merging test
// results. If execution is nil, the latest execution of the task is returned.
func FindOneIdAndExecutionWithDisplayStatusNoMerge(id string, execution *int) (*Task, error) {
	tasks := []Task{}
	match := bson.M{
		IdKey: id,
	}
	if execution != nil {
		match[ExecutionKey] = *execution
	}
	pipeline := []bson.M{
		{"$match": match},
		addDisplayStatus,
	}
	if err := Aggregate(pipeline, &tasks); err != nil {
		return nil, errors.Wrap(err, "finding task")
	}
	if len(tasks) != 0 {
		t := tasks[0]
		return &t, nil
	}
	return FindOneOldByIdAndExecutionWithDisplayStatusNoMerge(id, execution)

}

// FindOneOldByIdAndExecutionWithDisplayStatusNoMerge returns a single task
// with the given id and execution with display statuses added and without
// merging test results. If execution is nil, the latest execution of the task
// is returned.
func FindOneOldByIdAndExecutionWithDisplayStatusNoMerge(id string, execution *int) (*Task, error) {
	tasks := []Task{}
	match := bson.M{
		OldTaskIdKey: id,
	}
	if execution != nil {
		match[ExecutionKey] = *execution
	}
	pipeline := []bson.M{
		{"$match": match},
		addDisplayStatus,
	}

	if err := db.Aggregate(OldCollection, pipeline, &tasks); err != nil {
		return nil, errors.Wrap(err, "finding task")
	}
	if len(tasks) != 0 {
		t := tasks[0]
		return &t, nil
	}
	return nil, errors.New("cannot find task")
}

// FindOneOldByIdAndExecutionNoMerge returns a single task from the old tasks
// collection with the given id and exuection without merging test results.
func FindOneOldByIdAndExecutionNoMerge(id string, execution int) (*Task, error) {
	query := db.Query(bson.M{
		OldTaskIdKey: id,
		ExecutionKey: execution,
	})
	return FindOneOldNoMerge(query)
}

// FindOneIdWithFieldsNoMerge returns a single task with the given id and
// projects only the given fields without merging test results.
func FindOneIdWithFieldsNoMerge(id string, projected ...string) (*Task, error) {
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
		return nil, errors.Wrap(err, "finding task")
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
		return nil, errors.Wrap(err, "finding task ids for versions")
	}

	ids := []string{}
	for _, t := range tasks {
		ids = append(ids, t.Id)
	}

	return ids, nil
}

// FindStuckDispatchingNoMerge returns all tasks that have had a "dispatched"
// status for greater than 30 minutes without merging test results.
func FindStuckDispatching() ([]Task, error) {
	tasks, err := FindAllNoMerge(db.Query(bson.M{
		StatusKey:       evergreen.TaskDispatched,
		DispatchTimeKey: bson.M{"$gt": time.Now().Add(30 * time.Minute)},
		StartTimeKey:    utility.ZeroTime,
	}))
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding tasks")
	}
	return tasks, nil
}

// FindAllTaskIDsFromVersion returns all ids of tasks with the given version
// id.
func FindAllTaskIDsFromVersion(versionId string) ([]string, error) {
	q := db.Query(bson.M{VersionKey: versionId}).WithFields(IdKey)
	return findAllTaskIDs(q)
}

// FindALlTaskIDsFromBuild returns all ids of tasks with the given build id.
func FindAllTaskIDsFromBuild(buildId string) ([]string, error) {
	q := db.Query(bson.M{BuildIdKey: buildId}).WithFields(IdKey)
	return findAllTaskIDs(q)
}

// FindAllTasksFromVersionWithDependenciesNoMerge returns all tasks in with the
// given version id and includes only their dependencies without merging test
// results.
func FindAllTasksFromVersionWithDependenciesNoMerge(versionId string) ([]Task, error) {
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

// FindTasksFromVersionsNoMerge returns all tasks with the given version
// ids without merging test results.
func FindTasksFromVersionsNoMerge(versionIds []string) ([]Task, error) {
	return FindNoMerge(ByVersions(versionIds).
		WithFields(IdKey, DisplayNameKey, StatusKey, TimeTakenKey, VersionKey, BuildVariantKey, AbortedKey, AbortInfoKey))
}

// FindTaskGroupFromBuild returns all tasks in the given task group and with
// the given build id without merging test results.
func FindTaskGroupFromBuild(buildId, taskGroup string) ([]Task, error) {
	tasks, err := FindNoMerge(db.Query(bson.M{
		BuildIdKey:   buildId,
		TaskGroupKey: taskGroup,
	}).Sort([]string{TaskGroupOrderKey}))
	if err != nil {
		return nil, errors.Wrap(err, "error getting tasks in task group")
	}
	return tasks, nil
}

// FindMergeTaskForVersionNoMerge returns a single commit queue merge task with
// the given version id without merging test results.
func FindMergeTaskForVersionNoMerge(versionId string) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{
		VersionKey:          versionId,
		CommitQueueMergeKey: true,
	})
	err := db.FindOneQ(Collection, query, task)

	return task, err
}

// FindOld returns all execution tasks from the old tasks collection that
// satisfy the given query and merges test results.
func FindOld(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	for i, task := range tasks {
		if err = task.MergeNewTestResults(); err != nil {
			return nil, errors.Wrap(err, "merging new test results")
		}
		tasks[i] = task
	}

	// Remove display tasks from results.
	for i := len(tasks) - 1; i >= 0; i-- {
		t := tasks[i]
		if t.DisplayOnly {
			tasks = append(tasks[:i], tasks[i+1:]...)
		}
	}
	return tasks, err
}

// FindOldWithDisplayTasks returns display and execution tasks that satisfy the
// given query in the old tasks collection and merges test results.
func FindOldWithDisplayTasks(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	for i, task := range tasks {
		if err = task.MergeNewTestResults(); err != nil {
			return nil, errors.Wrap(err, "merging new test results")
		}
		tasks[i] = task
	}

	return tasks, err
}

// FindOneIdOldOrNew returns a single task with the given id and execution and
// merges test results. The query is first executed in the old tasks
// collection, then, if the task is not found, in the tasks collection.
func FindOneIdOldOrNew(id string, execution int) (*Task, error) {
	task, err := FindOneOld(ById(MakeOldID(id, execution)))
	if task == nil || err != nil {
		return FindOne(ById(id))
	}

	return task, err
}

// FindOneIdNewOrOld returns a single task with the given id and execution and
// merges test results. The query is first executed in the tasks collection,
// then, if the task is not found, in the old tasks collection.
func FindOneIdNewOrOld(id string) (*Task, error) {
	task, err := FindOne(ById(id))
	if task == nil || err != nil {
		return FindOneOld(ById(id))
	}

	return task, err
}

// MakeOldID returns the old tasks collection _id of the given task id and
// execution combination.
func MakeOldID(taskID string, execution int) string {
	return fmt.Sprintf("%s_%d", taskID, execution)
}

// FindAllFirstExceutionNoMerge returns the first execution of all tasks that
// satisfy the given query without merging test results.
func FindAllFirstExecutionNoMerge(q db.Q) ([]Task, error) {
	existingTasks, err := FindAllNoMerge(q)
	if err != nil {
		return nil, errors.Wrap(err, "finding existing tasks")
	}
	tasks := []Task{}
	oldIDs := []string{}
	for _, t := range existingTasks {
		if t.Execution == 0 {
			tasks = append(tasks, t)
		} else {
			oldIDs = append(oldIDs, MakeOldID(t.Id, 0))
		}
	}

	if len(oldIDs) > 0 {
		oldTasks, err := FindAllOldNoMerge(ByIds(oldIDs))
		if err != nil {
			return nil, errors.Wrap(err, "finding old tasks")
		}
		tasks = append(tasks, oldTasks...)
	}

	return tasks, nil
}

// FindNoMerge returns all tasks that satisfy the given query, filtering out
// display tasks and without merging test results.
func FindNoMerge(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	filtered := []Task{}

	// Remove display tasks from results.
	for idx := range tasks {
		t := tasks[idx]
		if t.DisplayOnly {
			continue
		}
		filtered = append(filtered, t)

	}

	return filtered, err
}

// Find returns all tasks that satisfy the given query without merging test
// results.
func FindAllNoMerge(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tasks, err
}

// FindAllOldNoMerge returns all tasks from the old collection that satisfy the
// given query without merging test results.
func FindAllOldNoMerge(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tasks, err
}

// FindWithDisplayTasks returns all tasks that satisfy the given query,
// fetching any parent display tasks and merging test results.
func FindWithDisplayTasks(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	for i, t := range tasks {
		_, err = t.GetDisplayTask()
		if err != nil {
			return nil, errors.Wrap(err, "getting parent display task")
		}
		tasks[i] = t
	}

	return tasks, err
}

// UpdateOne updates a single task that matches the given query.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

// UpdateAll updates all tasks that match the given query.
func UpdateAll(query interface{}, update interface{}) (*adb.ChangeInfo, error) {
	return db.UpdateAll(
		Collection,
		query,
		update,
	)
}

// Remove deletes the task with the given id from the database.
func Remove(id string) error {
	return db.Remove(
		Collection,
		bson.M{IdKey: id},
	)
}

// Remove all deletes all tasks with the given buildId.
func RemoveAllWithBuild(buildId string) error {
	return db.RemoveAll(
		Collection,
		bson.M{BuildIdKey: buildId})
}

// Aggregate runs the given aggregation against the tasks collection.
func Aggregate(pipeline []bson.M, results interface{}) error {
	return db.Aggregate(
		Collection,
		pipeline,
		results)
}

// Count returns the number of tasks that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}

// FindProjectForTask retuns the project of the task with the given id.
func FindProjectForTask(taskID string) (string, error) {
	t, err := FindOneNoMerge(ById(taskID).Project(bson.M{ProjectKey: 1}))
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", errors.New("task not found")
	}
	return t.Project, nil
}

func updateAllMatchingDependenciesForTask(taskId, dependencyId string, unattainable bool) error {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	res := env.DB().Collection(Collection).FindOneAndUpdate(ctx,
		bson.M{
			IdKey: taskId,
		},
		bson.M{
			"$set": bson.M{bsonutil.GetDottedKeyName(DependsOnKey, "$[elem]", DependencyUnattainableKey): unattainable},
		},
		options.FindOneAndUpdate().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
			bson.M{
				bsonutil.GetDottedKeyName("elem", DependencyTaskIdKey): dependencyId,
			},
		}}),
	)
	return res.Err()
}

// AbortTasksForBuild udpates tasks with the given build id and ids to aborted
// if they have an abortable status. If taskIds is empty, all task ids with the
// given build id and abortable status are set to aborted.
func AbortTasksForBuild(buildId string, taskIds []string, caller string) error {
	q := bson.M{
		BuildIdKey: buildId,
		StatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
	}
	if len(taskIds) > 0 {
		q[IdKey] = bson.M{"$in": taskIds}
	}
	_, err := UpdateAll(
		q,
		bson.M{
			"$set": bson.M{
				AbortedKey:   true,
				AbortInfoKey: AbortInfo{User: caller},
			},
		},
	)
	return err
}

// AbortTasksForBuild udpates tasks with the given version id and ids to
// aborted if they have an abortable status.
func AbortTasksForVersion(versionId string, taskIds []string, caller string) error {
	_, err := UpdateAll(
		bson.M{
			VersionKey: versionId,
			IdKey:      bson.M{"$in": taskIds},
			StatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
		},
		bson.M{"$set": bson.M{
			AbortedKey:   true,
			AbortInfoKey: AbortInfo{User: caller},
		}},
	)
	return err
}

// AddHostCreateDetails adds host create details to the task with the given
// task id.
func AddHostCreateDetails(taskId, hostId string, hostCreateError error) error {
	if hostCreateError == nil {
		return nil
	}
	err := UpdateOne(
		bson.M{
			IdKey: taskId,
		},
		bson.M{"$push": bson.M{
			HostCreateDetailsKey: HostCreateDetail{HostId: hostId, Error: hostCreateError.Error()},
		}})
	return errors.Wrapf(err, "adding details of host creation failure to task")
}
