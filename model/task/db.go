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
	ActivatedTasksByDistroIndex = bson.D{
		{Key: DistroIdKey, Value: 1},
		{Key: StatusKey, Value: 1},
		{Key: ActivatedKey, Value: 1},
		{Key: PriorityKey, Value: 1},
	}
)

var (
	// BSON fields for the task struct
	IdKey                       = bsonutil.MustHaveTag(Task{}, "Id")
	SecretKey                   = bsonutil.MustHaveTag(Task{}, "Secret")
	CreateTimeKey               = bsonutil.MustHaveTag(Task{}, "CreateTime")
	DispatchTimeKey             = bsonutil.MustHaveTag(Task{}, "DispatchTime")
	ScheduledTimeKey            = bsonutil.MustHaveTag(Task{}, "ScheduledTime")
	ContainerAllocatedTimeKey   = bsonutil.MustHaveTag(Task{}, "ContainerAllocatedTime")
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
	ExecutionPlatformKey        = bsonutil.MustHaveTag(Task{}, "ExecutionPlatform")
	HostIdKey                   = bsonutil.MustHaveTag(Task{}, "HostId")
	AgentVersionKey             = bsonutil.MustHaveTag(Task{}, "AgentVersion")
	ExecutionKey                = bsonutil.MustHaveTag(Task{}, "Execution")
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
	DependencyFinishedKey     = bsonutil.MustHaveTag(Dependency{}, "Finished")
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
				// TODO (PM-2620): Handle these statuses properly in the UI.
				{
					"case": bson.M{
						"$eq": []string{"$" + StatusKey, evergreen.TaskContainerAllocated},
					},
					"then": evergreen.TaskContainerAllocated,
				},
				{
					"case": bson.M{
						"$eq": []string{"$" + StatusKey, evergreen.TaskContainerUnallocated},
					},
					"then": evergreen.TaskContainerUnallocated,
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
func ById(id string) bson.M {
	return bson.M{
		IdKey: id,
	}
}

func ByOldTaskID(id string) bson.M {
	return bson.M{
		OldTaskIdKey: id,
	}
}

// ByIds creates a query that finds all tasks with the given ids.
func ByIds(ids []string) bson.M {
	return bson.M{
		IdKey: bson.M{"$in": ids},
	}
}

// ByBuildId creates a query to return tasks with a certain build id
func ByBuildId(buildId string) bson.M {
	return bson.M{
		BuildIdKey: buildId,
	}
}

func ByBuildIdAndGithubChecks(buildId string) bson.M {
	return bson.M{
		BuildIdKey:       buildId,
		IsGithubCheckKey: true,
	}
}

// ByBuildIds creates a query to return tasks in buildsIds
func ByBuildIds(buildIds []string) bson.M {
	return bson.M{
		BuildIdKey: bson.M{"$in": buildIds},
	}
}

// ByAborted creates a query to return tasks with an aborted state
func ByAborted(aborted bool) bson.M {
	return bson.M{
		AbortedKey: aborted,
	}
}

// ByActivation creates a query to return tasks with an activated state
func ByActivation(active bool) bson.M {
	return bson.M{
		ActivatedKey: active,
	}
}

// ByVersion creates a query to return tasks with a certain build id
func ByVersion(version string) bson.M {
	return bson.M{
		VersionKey: version,
	}
}

// DisplayTasksByVersion produces a query that returns all display tasks for the given version.
func DisplayTasksByVersion(version string) bson.M {
	// assumes that all ExecutionTasks know of their corresponding DisplayTask (i.e. DisplayTaskIdKey not null or "")
	return bson.M{
		"$and": []bson.M{
			{
				VersionKey:       version,
				ActivatedTimeKey: bson.M{"$ne": utility.ZeroTime},
			},
			{"$or": []bson.M{
				{DisplayTaskIdKey: ""},                       // no 'parent' display task
				{DisplayOnlyKey: true},                       // ...
				{DisplayTaskIdKey: nil},                      // ...
				{ExecutionTasksKey: bson.M{"$exists": true}}, // has execution tasks
			},
			},
		},
	}
}

// FailedTasksByVersion produces a query that returns all failed tasks for the given version.
func FailedTasksByVersion(version string) bson.M {
	return bson.M{
		VersionKey: version,
		StatusKey:  bson.M{"$in": evergreen.TaskFailureStatuses},
	}
}

func FailedTasksByVersionAndBV(version string, variant string) bson.M {
	return bson.M{
		VersionKey:      version,
		BuildVariantKey: variant,
		StatusKey:       bson.M{"$in": evergreen.TaskFailureStatuses},
	}
}

// ByVersion produces a query that returns tasks for the given version.
func ByVersions(versions []string) bson.M {
	return bson.M{VersionKey: bson.M{"$in": versions}}
}

// NonExecutionTasksByVersion will filter out newer execution tasks that store if they have a display task.
// Old execution tasks without display task ID populated will still be returned.
func NonExecutionTasksByVersions(versions []string) bson.M {
	return bson.M{
		VersionKey: bson.M{"$in": versions},
		"$or": []bson.M{
			{
				DisplayTaskIdKey: bson.M{"$exists": false},
			},
			{
				DisplayTaskIdKey: "",
			},
		},
	}
}

// ByIdsBuildIdAndStatus creates a query to return tasks with a certain build id and statuses
func ByIdsAndStatus(taskIds []string, statuses []string) bson.M {
	return bson.M{
		IdKey: bson.M{"$in": taskIds},
		StatusKey: bson.M{
			"$in": statuses,
		},
	}
}

type StaleReason int

const (
	HeartbeatPastCutoff StaleReason = iota
	NoHeartbeatSinceDispatch
)

// ByStaleRunningTask creates a query that finds any running tasks
// whose last heartbeat was at least the specified threshold ago, or
// that has been dispatched but hasn't started in twice that long.
func ByStaleRunningTask(staleness time.Duration, reason StaleReason) bson.M {
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
	return reasonQuery
}

// ByCommit creates a query on Evergreen as the requester on a revision, buildVariant, displayName and project.
func ByCommit(revision, buildVariant, displayName, project, requester string) bson.M {
	return bson.M{
		RevisionKey:     revision,
		RequesterKey:    requester,
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		ProjectKey:      project,
	}
}

func ByVersionsForNameAndVariant(versions, displayNames []string, buildVariant string) bson.M {
	return bson.M{
		VersionKey: bson.M{
			"$in": versions,
		},
		DisplayNameKey: bson.M{
			"$in": displayNames,
		},
		BuildVariantKey: buildVariant,
	}
}

// ByIntermediateRevisions creates a query that returns the tasks existing
// between two revision order numbers, exclusive.
func ByIntermediateRevisions(previousRevisionOrder, currentRevisionOrder int,
	buildVariant, displayName, project, requester string) bson.M {
	return bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey:    requester,
		RevisionOrderNumberKey: bson.M{
			"$lt": currentRevisionOrder,
			"$gt": previousRevisionOrder,
		},
		ProjectKey: project,
	}
}

func ByBeforeRevision(revisionOrder int, buildVariant, displayName, project, requester string) (bson.M, []string) {
	return bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey:    requester,
		RevisionOrderNumberKey: bson.M{
			"$lt": revisionOrder,
		},
		ProjectKey: project,
	}, []string{"-" + RevisionOrderNumberKey}
}

func ByActivatedBeforeRevisionWithStatuses(revisionOrder int, statuses []string, buildVariant string, displayName string, project string) (bson.M, []string) {
	return bson.M{
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
	}, []string{"-" + RevisionOrderNumberKey}
}

func ByBeforeRevisionWithStatusesAndRequesters(revisionOrder int, statuses []string, buildVariant, displayName, project string, requesters []string) bson.M {
	return bson.M{
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
	}
}

// ByTimeRun returns all tasks that are running in between two given times.
func ByTimeRun(startTime, endTime time.Time) bson.M {
	return bson.M{
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
		},
	}
}

// ByTimeStartedAndFailed returns all failed tasks that started between 2 given times
// If task not started (but is failed), returns if finished within the time range
func ByTimeStartedAndFailed(startTime, endTime time.Time, commandTypes []string) bson.M {
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
	return query
}

func ByStatuses(statuses []string, buildVariant, displayName, project, requester string) bson.M {
	return bson.M{
		BuildVariantKey: buildVariant,
		DisplayNameKey:  displayName,
		RequesterKey:    requester,
		StatusKey: bson.M{
			"$in": statuses,
		},
		ProjectKey: project,
	}
}

// ByDifferentFailedBuildVariants returns a query for all failed tasks on a revision that are not of a buildVariant
func ByDifferentFailedBuildVariants(revision, buildVariant, displayName, project, requester string) bson.M {
	return bson.M{
		BuildVariantKey: bson.M{
			"$ne": buildVariant,
		},
		DisplayNameKey: displayName,
		StatusKey:      evergreen.TaskFailed,
		ProjectKey:     project,
		RequesterKey:   requester,
		RevisionKey:    revision,
	}
}

func ByRecentlyFinished(finishTime time.Time, project string, requester string) bson.M {
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
	return query
}

// Returns query which targets list of tasks
// And allow filter by project_id, status, start_time (gte), finish_time (lte)
func WithinTimePeriod(startedAfter, finishedBefore time.Time, project string, statuses []string) bson.M {
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

	return bson.M{
		"$and": q,
	}
}

func ByExecutionTask(taskId string) bson.M {
	return bson.M{
		ExecutionTasksKey: taskId,
	}
}

func ByExecutionTasks(ids []string) bson.M {
	return bson.M{
		ExecutionTasksKey: bson.M{
			"$in": ids,
		},
	}
}

func BySubsetAborted(ids []string) bson.M {
	return bson.M{
		IdKey:      bson.M{"$in": ids},
		AbortedKey: true,
	}
}

// ByExecutionPlatform returns the query to find tasks matching the given
// execution platform. If the empty string is given, the task is assumed to be
// the default of ExecutionPlatformHost.
func ByExecutionPlatform(platform ExecutionPlatform) bson.M {
	switch platform {
	case "", ExecutionPlatformHost:
		return bson.M{
			"$or": []bson.M{
				{ExecutionPlatformKey: bson.M{"$exists": false}},
				{ExecutionPlatformKey: platform},
			},
		}
	case ExecutionPlatformContainer:
		return bson.M{
			ExecutionPlatformKey: platform,
		}
	default:
		return bson.M{}
	}
}

var (
	IsDispatchedOrStarted = bson.M{
		StatusKey: bson.M{"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
	}
)

func schedulableHostTasksQuery() bson.M {
	q := bson.M{
		ActivatedKey: true,
		StatusKey:    evergreen.TaskUndispatched,

		// Filter out tasks disabled by negative priority
		PriorityKey: bson.M{"$gt": evergreen.DisabledTaskPriority},
	}
	q["$and"] = []bson.M{
		ByExecutionPlatform(ExecutionPlatformHost),
		// Filter tasks containing unattainable dependencies
		{"$or": []bson.M{
			{bsonutil.GetDottedKeyName(DependsOnKey, DependencyUnattainableKey): bson.M{"$ne": true}},
			{OverrideDependenciesKey: true},
		}},
	}

	return q
}

// FindNeedsContainerAllocation returns all container tasks that are waiting for
// a container to be allocated to them sorted by activation time.
func FindNeedsContainerAllocation() ([]Task, error) {
	q := bson.M{
		ActivatedKey:         true,
		StatusKey:            evergreen.TaskContainerUnallocated,
		ExecutionPlatformKey: ExecutionPlatformContainer,
		PriorityKey:          bson.M{"$gt": evergreen.DisabledTaskPriority},
		"$or": []bson.M{
			{
				DependsOnKey: bson.M{"$size": 0},
			},
			{
				// Containers can only be allocated for tasks whose dependencies
				// are all met. All dependencies are met if they're all finished
				// running and are still attainable (i.e. the dependency's
				// required status matched the task's actual ending status).
				bsonutil.GetDottedKeyName(DependsOnKey, DependencyFinishedKey):     true,
				bsonutil.GetDottedKeyName(DependsOnKey, DependencyUnattainableKey): bson.M{"$ne": true},
			},
			{OverrideDependenciesKey: true},
		},
	}
	return FindAll(db.Query(q).Sort([]string{ActivatedTimeKey}))
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

const VersionLimit = 50

// FindUniqueBuildVariantNamesByTask returns a list of unique build variants names and their display names for a given task name.
// It attempts to return the most recent display name for each build variant to avoid returning duplicates caused by display names changing.
// It only checks the last 50 versions that ran for a given task name.
func FindUniqueBuildVariantNamesByTask(projectId string, taskName string, repoOrderNumber int) ([]*BuildVariantTuple, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			ProjectKey:     projectId,
			DisplayNameKey: taskName,
			RequesterKey:   bson.M{"$in": evergreen.SystemVersionRequesterTypes},
			"$and": []bson.M{
				{RevisionOrderNumberKey: bson.M{"$gte": repoOrderNumber - VersionLimit}},
				{RevisionOrderNumberKey: bson.M{"$lte": repoOrderNumber}},
			},
		},
		},
	}

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
func FindTaskNamesByBuildVariant(projectId string, buildVariant string, repoOrderNumber int) ([]string, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			ProjectKey:      projectId,
			BuildVariantKey: buildVariant,
			RequesterKey:    bson.M{"$in": evergreen.SystemVersionRequesterTypes},
			"$or": []bson.M{
				{DisplayTaskIdKey: bson.M{"$exists": false}},
				{DisplayTaskIdKey: ""},
			},
			"$and": []bson.M{
				{RevisionOrderNumberKey: bson.M{"$gt": repoOrderNumber - VersionLimit}},
				{RevisionOrderNumberKey: bson.M{"$lte": repoOrderNumber}},
			},
		},
		},
	}

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
func FindOneOld(filter bson.M) (*Task, error) {
	task := &Task{}
	query := db.Query(filter)
	err := db.FindOneQ(OldCollection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

func FindOneOldWithFields(filter bson.M, fields ...string) (*Task, error) {
	task := &Task{}
	query := db.Query(filter).WithFields(fields...)
	err := db.FindOneQ(OldCollection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

func FindOneOldId(id string) (*Task, error) {
	filter := bson.M{
		IdKey: id,
	}
	return FindOneOld(filter)
}

// FindOneOldByIdAndExecution returns a single task from the old tasks
// collection with the given ID and execution.
func FindOneOldByIdAndExecution(id string, execution int) (*Task, error) {
	filter := bson.M{
		OldTaskIdKey: id,
		ExecutionKey: execution,
	}
	return FindOneOld(filter)
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
	return FindWithFields(ByVersions(versionIds),
		IdKey, DisplayNameKey, StatusKey, TimeTakenKey, VersionKey, BuildVariantKey, AbortedKey, AbortInfoKey)
}

func CountActivatedTasksForVersion(versionId string) (int, error) {
	return Count(db.Query(bson.M{
		VersionKey:   versionId,
		ActivatedKey: true,
	}))
}

func FindTaskGroupFromBuild(buildId, taskGroup string) ([]Task, error) {
	tasks, err := FindWithSort(bson.M{
		BuildIdKey:   buildId,
		TaskGroupKey: taskGroup,
	}, []string{TaskGroupOrderKey})
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
func FindOld(filter bson.M) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter)
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, err
}

func FindOldWithFields(filter bson.M, fields ...string) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter).WithFields(fields...)
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, err
}

// FindOldWithDisplayTasks returns all display and execution tasks from the old
// collection that satisfy the given query.
func FindOldWithDisplayTasks(filter bson.M) ([]Task, error) {
	tasks := []Task{}
	query := db.Query(filter)
	err := db.FindAllQ(OldCollection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, err
}

// FindOneIdOldOrNew returns a single task with the given ID and execution,
// first looking in the old tasks collection, then the tasks collection.
func FindOneIdOldOrNew(id string, execution int) (*Task, error) {
	task, err := FindOneOldId(MakeOldID(id, execution))
	if task == nil || err != nil {
		return FindOneId(id)
	}

	return task, err
}

// FindOneIdNewOrOld returns a single task with the given ID and execution,
// first looking in the tasks collection, then the old tasks collection.
func FindOneIdNewOrOld(id string) (*Task, error) {
	task, err := FindOneId(id)
	if task == nil || err != nil {
		return FindOneOldId(id)
	}

	return task, err
}

func MakeOldID(taskID string, execution int) string {
	return fmt.Sprintf("%s_%d", taskID, execution)
}

func FindAllFirstExecution(query db.Q) ([]Task, error) {
	existingTasks, err := FindAll(query)
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
		oldTasks, err := FindAllOld(db.Query(ByIds(oldIDs)))
		if err != nil {
			return nil, errors.Wrap(err, "can't get old tasks")
		}
		tasks = append(tasks, oldTasks...)
	}

	return tasks, nil
}

// Find returns all tasks that satisfy the query it also filters out display tasks from the results.
func Find(filter bson.M) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter)
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, nil
}

func FindWithFields(filter bson.M, fields ...string) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter).WithFields(fields...)
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, nil
}

func FindWithSort(filter bson.M, sort []string) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter).Sort(sort)
	err := db.FindAllQ(Collection, query, &tasks)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return tasks, nil
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

func UpdateAllWithHint(query interface{}, update interface{}, hint interface{}) (*adb.ChangeInfo, error) {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	res, err := env.DB().Collection(Collection).UpdateMany(ctx, query, update, options.Update().SetHint(hint))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &adb.ChangeInfo{Updated: int(res.ModifiedCount)}, nil
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
	query := db.Query(ById(taskID)).WithFields(ProjectKey)
	t, err := FindOne(query)
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
		StatusKey:  bson.M{"$in": evergreen.TaskAbortableStatuses},
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
			StatusKey:  bson.M{"$in": evergreen.TaskAbortableStatuses},
		},
		bson.M{"$set": bson.M{
			AbortedKey:   true,
			AbortInfoKey: AbortInfo{User: caller},
		}},
	)
	return err
}

// HasUnfinishedTaskForVersion returns true if there are any scheduled but
// unfinished tasks matching the given conditions.
func HasUnfinishedTaskForVersions(versionIds []string, taskName, variantName string) (bool, error) {
	count, err := Count(
		db.Query(bson.M{
			VersionKey:      bson.M{"$in": versionIds},
			DisplayNameKey:  taskName,
			BuildVariantKey: variantName,
			StatusKey:       bson.M{"$in": evergreen.TaskUncompletedStatuses},
		}))
	return count > 0, err
}

// FindTaskForVersion returns a task matching the given version and task info.
func FindTaskForVersion(versionId, taskName, variantName string) (*Task, error) {
	return FindOne(
		db.Query(bson.M{
			VersionKey:      versionId,
			DisplayNameKey:  taskName,
			BuildVariantKey: variantName,
		}))
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

func FindActivatedStepbackTasks(projectId string) ([]Task, error) {
	tasks, err := Find(bson.M{
		ProjectKey:     projectId,
		ActivatedKey:   true,
		ActivatedByKey: evergreen.StepbackTaskActivator,
		StatusKey:      bson.M{"$in": evergreen.TaskUncompletedStatuses},
	})
	if err != nil {

	}
	return tasks, nil
}
