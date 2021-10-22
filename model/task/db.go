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
	DisplayTaskIdKey            = bsonutil.MustHaveTag(Task{}, "DisplayTaskId")
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
	isUnattainable = bson.M{
		"$reduce": bson.M{
			"input":        "$" + DependsOnKey,
			"initialValue": false,
			"in":           bson.M{"$or": []interface{}{"$$value", bsonutil.GetDottedKeyName("$$this", DependencyUnattainableKey)}},
		},
	}

	addDisplayStatus = bson.M{
		"$addFields": bson.M{
			DisplayStatusKey: displayStatusExpression,
		},
	}

	// This should reflect Task.GetDisplayStatus()
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
							{OverrideDependenciesKey: false},
							isUnattainable,
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

// FailedTasksByVersion produces a query that returns all failed tasks for the given version.
func FailedTasksByVersion(version string) db.Q {
	return db.Query(bson.M{
		VersionKey: version,
		StatusKey:  bson.M{"$in": evergreen.TaskFailureStatuses},
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

// FindByExecutionTasksAndMaxExecution returns the tasks corresponding to the passed in taskIds and execution,
// or the most recent executions of those tasks if they do not have a matching execution
func FindByExecutionTasksAndMaxExecution(taskIds []*string, execution int) ([]Task, error) {
	pipeline := []bson.M{}
	match := bson.M{
		"$match": bson.M{
			IdKey: bson.M{
				"$in": taskIds,
			},
			ExecutionKey: bson.M{
				"$lte": execution,
			},
		},
	}
	pipeline = append(pipeline, match)
	result := []Task{}
	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "Error finding tasks in task collection")
	}
	// Get the taskIds that were not found in the previous match stage
	foundIds := []string{}
	for _, t := range result {
		foundIds = append(foundIds, t.Id)
	}

	missingTasks, _ := utility.StringSliceSymmetricDifference(utility.FromStringPtrSlice(taskIds), foundIds)
	if len(missingTasks) > 0 {
		oldTasks := []Task{}
		oldTaskPipeline := []bson.M{}
		match = bson.M{
			"$match": bson.M{
				OldTaskIdKey: bson.M{
					"$in": missingTasks,
				},
				ExecutionKey: bson.M{
					"$lte": execution,
				},
			},
		}
		oldTaskPipeline = append(oldTaskPipeline, match)
		if err := db.Aggregate(OldCollection, oldTaskPipeline, &oldTasks); err != nil {
			return nil, errors.Wrap(err, "error finding tasks in old tasks collection")
		}

		result = append(result, oldTasks...)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

type BuildVariantTuple struct {
	BuildVariant string `bson:"build_variant"`
	DisplayName  string `bson:"display_name"`
}

// FindUniqueBuildVariantNamesByTask returns a list of unique build variants names and their display names for a given task name,
// it tries to return the most recent display name for each build variant to avoid duplicates from display names changing
func FindUniqueBuildVariantNamesByTask(projectId string, taskName string) ([]*BuildVariantTuple, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			ProjectKey:     projectId,
			DisplayNameKey: taskName,
			RequesterKey:   bson.M{"$in": evergreen.SystemVersionRequesterTypes}},
		}}

	// sort by most recent version to get the most recent display names for the build variants first
	sortByOrderNumber := bson.M{
		"$sort": bson.M{
			CreateTimeKey: -1,
		},
	}

	pipeline = append(pipeline, sortByOrderNumber)

	// group the build variants by unique build variant names and get a build id for each
	groupByBuildVariant := bson.M{
		"$group": bson.M{
			"_id": bson.M{
				BuildVariantKey: "$" + BuildVariantKey,
			},
			BuildIdKey: bson.M{
				"$first": "$" + BuildIdKey,
			},
		},
	}

	pipeline = append(pipeline, groupByBuildVariant)

	// reorganize the results to get the build variant names and a corresponding build id
	projectBuildId := bson.M{
		"$project": bson.M{
			"_id":           0,
			BuildVariantKey: bsonutil.GetDottedKeyName("$_id", BuildVariantKey),
			BuildIdKey:      "$" + BuildIdKey,
		},
	}

	pipeline = append(pipeline, projectBuildId)

	// get the display name for each build variant
	pipeline = append(pipeline, AddBuildVariantDisplayName...)

	// cleanup the results
	project := bson.M{
		"$project": bson.M{
			"_id":           0,
			"build_variant": "$" + BuildVariantKey,
			"display_name":  "$" + BuildVariantDisplayNameKey,
		},
	}
	pipeline = append(pipeline, project)

	sort := bson.M{
		"$sort": bson.M{
			"display_name": 1,
		},
	}
	pipeline = append(pipeline, sort)

	result := []*BuildVariantTuple{}
	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "can't get build variant tasks")
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
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

// FindOne returns a single task that satisfies the query.
func FindOne(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(Collection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

// FindOneId returns a single task with the given ID.
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

// FindByIdExecution returns a single task with the given ID and execution. If
// execution is nil, the latest execution is returned.
func FindByIdExecution(id string, execution *int) (*Task, error) {
	if execution == nil {
		return FindOneId(id)
	}
	return FindOneIdAndExecution(id, *execution)
}

// FindOneIdAndExecution returns a single task with the given ID and execution.
func FindOneIdAndExecution(id string, execution int) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{
		IdKey:        id,
		ExecutionKey: execution,
	})
	err := db.FindOneQ(Collection, query, task)

	if adb.ResultsNotFound(err) {
		return FindOneOldByIdAndExecution(id, execution)
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding task by id and execution")
	}

	return task, nil
}

// FindOneIdAndExecutionWithDisplayStatus returns a single task with the given
// ID and execution, with display statuses added.
func FindOneIdAndExecutionWithDisplayStatus(id string, execution *int) (*Task, error) {
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

	return FindOneOldByIdAndExecutionWithDisplayStatus(id, execution)
}

// FindOneOldByIdAndExecutionWithDisplayStatus returns a single task with the
// given ID and execution from the old tasks collection, with display statuses
// added.
func FindOneOldByIdAndExecutionWithDisplayStatus(id string, execution *int) (*Task, error) {
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

	return nil, errors.New("task not found")
}

// FindOneOld returns a single task from the old tasks collection that
// satifisfies the given query.
func FindOneOld(query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(OldCollection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

// FindOneOldByIdAndExecution returns a single task from the old tasks
// collection with the given ID and execution.
func FindOneOldByIdAndExecution(id string, execution int) (*Task, error) {
	query := db.Query(bson.M{
		OldTaskIdKey: id,
		ExecutionKey: execution,
	})
	return FindOneOld(query)
}

// FindOneIdWithFields returns a single task with the given ID, projecting only
// the given fields.
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

// FindStuckDispatching returns all "stuck" tasks. A task is considered stuck
// if it has a "dispatched" status for more than 30 minutes.
func FindStuckDispatching() ([]Task, error) {
	tasks, err := FindAll(db.Query(bson.M{
		StatusKey:       evergreen.TaskDispatched,
		DispatchTimeKey: bson.M{"$gt": time.Now().Add(30 * time.Minute)},
		StartTimeKey:    utility.ZeroTime,
	}))
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem finding stuck dispatching tasks")
	}
	return tasks, nil
}

func FindAllTaskIDsFromVersion(versionId string) ([]string, error) {
	q := db.Query(bson.M{VersionKey: versionId}).WithFields(IdKey)
	return findAllTaskIDs(q)
}

func FindAllTaskIDsFromBuild(buildId string) ([]string, error) {
	q := db.Query(bson.M{BuildIdKey: buildId}).WithFields(IdKey)
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

func FindTasksFromVersions(versionIds []string) ([]Task, error) {
	return Find(ByVersions(versionIds).
		WithFields(IdKey, DisplayNameKey, StatusKey, TimeTakenKey, VersionKey, BuildVariantKey, AbortedKey, AbortInfoKey))
}

func FindTaskGroupFromBuild(buildId, taskGroup string) ([]Task, error) {
	tasks, err := Find(db.Query(bson.M{
		BuildIdKey:   buildId,
		TaskGroupKey: taskGroup,
	}).Sort([]string{TaskGroupOrderKey}))
	if err != nil {
		return nil, errors.Wrap(err, "error getting tasks in task group")
	}
	return tasks, nil
}

func FindMergeTaskForVersion(versionId string) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{
		VersionKey:          versionId,
		CommitQueueMergeKey: true,
	})
	err := db.FindOneQ(Collection, query, task)

	return task, err
}

// FindOld returns all non-display tasks from the old tasks collection that
// satisfy the given query.
func FindOld(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
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

// FindOldWithDisplayTasks returns all display and execution tasks from the old
// collection that satisfy the given query.
func FindOldWithDisplayTasks(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, err
}

// FindOneIdOldOrNew returns a single task with the given ID and execution,
// first looking in the old tasks collection, then the tasks collection.
func FindOneIdOldOrNew(id string, execution int) (*Task, error) {
	task, err := FindOneOld(ById(MakeOldID(id, execution)))
	if task == nil || err != nil {
		return FindOne(ById(id))
	}

	return task, err
}

// FindOneIdNewOrOld returns a single task with the given ID and execution,
// first looking in the tasks collection, then the old tasks collection.
func FindOneIdNewOrOld(id string) (*Task, error) {
	task, err := FindOne(ById(id))
	if task == nil || err != nil {
		return FindOneOld(ById(id))
	}

	return task, err
}

func MakeOldID(taskID string, execution int) string {
	return fmt.Sprintf("%s_%d", taskID, execution)
}

func FindAllFirstExecution(q db.Q) ([]Task, error) {
	existingTasks, err := FindAll(q)
	if err != nil {
		return nil, errors.Wrap(err, "can't get current tasks")
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
		oldTasks, err := FindAllOld(ByIds(oldIDs))
		if err != nil {
			return nil, errors.Wrap(err, "can't get old tasks")
		}
		tasks = append(tasks, oldTasks...)
	}

	return tasks, nil
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

// Find returns really all tasks that satisfy the query.
func FindAllOld(query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
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

func FindProjectForTask(taskID string) (string, error) {
	t, err := FindOne(ById(taskID).Project(bson.M{ProjectKey: 1}))
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

func AddHostCreateDetails(taskId, hostId string, execution int, hostCreateError error) error {
	if hostCreateError == nil {
		return nil
	}
	err := UpdateOne(
		bson.M{
			IdKey:        taskId,
			ExecutionKey: execution,
		},
		bson.M{"$push": bson.M{
			HostCreateDetailsKey: HostCreateDetail{HostId: hostId, Error: hostCreateError.Error()},
		}})
	return errors.Wrapf(err, "error adding details of host creation failure to task")
}
