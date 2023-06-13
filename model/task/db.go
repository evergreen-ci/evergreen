package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/distro"
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
		{Key: OverrideDependenciesKey, Value: 1},
		{Key: bsonutil.GetDottedKeyName(DependsOnKey, DependencyUnattainableKey), Value: 1},
	}
)

var (
	// BSON fields for the task struct
	IdKey                          = bsonutil.MustHaveTag(Task{}, "Id")
	SecretKey                      = bsonutil.MustHaveTag(Task{}, "Secret")
	CreateTimeKey                  = bsonutil.MustHaveTag(Task{}, "CreateTime")
	DispatchTimeKey                = bsonutil.MustHaveTag(Task{}, "DispatchTime")
	ScheduledTimeKey               = bsonutil.MustHaveTag(Task{}, "ScheduledTime")
	ContainerAllocatedTimeKey      = bsonutil.MustHaveTag(Task{}, "ContainerAllocatedTime")
	StartTimeKey                   = bsonutil.MustHaveTag(Task{}, "StartTime")
	FinishTimeKey                  = bsonutil.MustHaveTag(Task{}, "FinishTime")
	ActivatedTimeKey               = bsonutil.MustHaveTag(Task{}, "ActivatedTime")
	DependenciesMetTimeKey         = bsonutil.MustHaveTag(Task{}, "DependenciesMetTime")
	VersionKey                     = bsonutil.MustHaveTag(Task{}, "Version")
	ProjectKey                     = bsonutil.MustHaveTag(Task{}, "Project")
	RevisionKey                    = bsonutil.MustHaveTag(Task{}, "Revision")
	LastHeartbeatKey               = bsonutil.MustHaveTag(Task{}, "LastHeartbeat")
	ActivatedKey                   = bsonutil.MustHaveTag(Task{}, "Activated")
	ContainerAllocatedKey          = bsonutil.MustHaveTag(Task{}, "ContainerAllocated")
	ContainerAllocationAttemptsKey = bsonutil.MustHaveTag(Task{}, "ContainerAllocationAttempts")
	DeactivatedForDependencyKey    = bsonutil.MustHaveTag(Task{}, "DeactivatedForDependency")
	BuildIdKey                     = bsonutil.MustHaveTag(Task{}, "BuildId")
	DistroIdKey                    = bsonutil.MustHaveTag(Task{}, "DistroId")
	SecondaryDistrosKey            = bsonutil.MustHaveTag(Task{}, "SecondaryDistros")
	BuildVariantKey                = bsonutil.MustHaveTag(Task{}, "BuildVariant")
	DependsOnKey                   = bsonutil.MustHaveTag(Task{}, "DependsOn")
	UnattainableDependencyKey      = bsonutil.MustHaveTag(Task{}, "UnattainableDependency")
	OverrideDependenciesKey        = bsonutil.MustHaveTag(Task{}, "OverrideDependencies")
	NumDepsKey                     = bsonutil.MustHaveTag(Task{}, "NumDependents")
	DisplayNameKey                 = bsonutil.MustHaveTag(Task{}, "DisplayName")
	ExecutionPlatformKey           = bsonutil.MustHaveTag(Task{}, "ExecutionPlatform")
	HostIdKey                      = bsonutil.MustHaveTag(Task{}, "HostId")
	PodIDKey                       = bsonutil.MustHaveTag(Task{}, "PodID")
	AgentVersionKey                = bsonutil.MustHaveTag(Task{}, "AgentVersion")
	ExecutionKey                   = bsonutil.MustHaveTag(Task{}, "Execution")
	LatestParentExecutionKey       = bsonutil.MustHaveTag(Task{}, "LatestParentExecution")
	OldTaskIdKey                   = bsonutil.MustHaveTag(Task{}, "OldTaskId")
	ArchivedKey                    = bsonutil.MustHaveTag(Task{}, "Archived")
	CanResetKey                    = bsonutil.MustHaveTag(Task{}, "CanReset")
	RevisionOrderNumberKey         = bsonutil.MustHaveTag(Task{}, "RevisionOrderNumber")
	RequesterKey                   = bsonutil.MustHaveTag(Task{}, "Requester")
	StatusKey                      = bsonutil.MustHaveTag(Task{}, "Status")
	DetailsKey                     = bsonutil.MustHaveTag(Task{}, "Details")
	AbortedKey                     = bsonutil.MustHaveTag(Task{}, "Aborted")
	AbortInfoKey                   = bsonutil.MustHaveTag(Task{}, "AbortInfo")
	TimeTakenKey                   = bsonutil.MustHaveTag(Task{}, "TimeTaken")
	ExpectedDurationKey            = bsonutil.MustHaveTag(Task{}, "ExpectedDuration")
	ExpectedDurationStddevKey      = bsonutil.MustHaveTag(Task{}, "ExpectedDurationStdDev")
	DurationPredictionKey          = bsonutil.MustHaveTag(Task{}, "DurationPrediction")
	PriorityKey                    = bsonutil.MustHaveTag(Task{}, "Priority")
	ActivatedByKey                 = bsonutil.MustHaveTag(Task{}, "ActivatedBy")
	StepbackDepthKey               = bsonutil.MustHaveTag(Task{}, "StepbackDepth")
	ExecutionTasksKey              = bsonutil.MustHaveTag(Task{}, "ExecutionTasks")
	DisplayOnlyKey                 = bsonutil.MustHaveTag(Task{}, "DisplayOnly")
	DisplayTaskIdKey               = bsonutil.MustHaveTag(Task{}, "DisplayTaskId")
	TaskGroupKey                   = bsonutil.MustHaveTag(Task{}, "TaskGroup")
	TaskGroupMaxHostsKey           = bsonutil.MustHaveTag(Task{}, "TaskGroupMaxHosts")
	TaskGroupOrderKey              = bsonutil.MustHaveTag(Task{}, "TaskGroupOrder")
	GenerateTaskKey                = bsonutil.MustHaveTag(Task{}, "GenerateTask")
	GeneratedTasksKey              = bsonutil.MustHaveTag(Task{}, "GeneratedTasks")
	GeneratedByKey                 = bsonutil.MustHaveTag(Task{}, "GeneratedBy")
	ResultsServiceKey              = bsonutil.MustHaveTag(Task{}, "ResultsService")
	HasCedarResultsKey             = bsonutil.MustHaveTag(Task{}, "HasCedarResults")
	ResultsFailedKey               = bsonutil.MustHaveTag(Task{}, "ResultsFailed")
	IsGithubCheckKey               = bsonutil.MustHaveTag(Task{}, "IsGithubCheck")
	HostCreateDetailsKey           = bsonutil.MustHaveTag(Task{}, "HostCreateDetails")

	GeneratedJSONAsStringKey    = bsonutil.MustHaveTag(Task{}, "GeneratedJSONAsString")
	GenerateTasksErrorKey       = bsonutil.MustHaveTag(Task{}, "GenerateTasksError")
	GeneratedTasksToActivateKey = bsonutil.MustHaveTag(Task{}, "GeneratedTasksToActivate")
	ResetWhenFinishedKey        = bsonutil.MustHaveTag(Task{}, "ResetWhenFinished")
	ResetFailedWhenFinishedKey  = bsonutil.MustHaveTag(Task{}, "ResetFailedWhenFinished")
	LogsKey                     = bsonutil.MustHaveTag(Task{}, "Logs")
	CommitQueueMergeKey         = bsonutil.MustHaveTag(Task{}, "CommitQueueMerge")
	DisplayStatusKey            = bsonutil.MustHaveTag(Task{}, "DisplayStatus")
	BaseTaskKey                 = bsonutil.MustHaveTag(Task{}, "BaseTask")
	BuildVariantDisplayNameKey  = bsonutil.MustHaveTag(Task{}, "BuildVariantDisplayName")
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
	DependencyTaskIdKey             = bsonutil.MustHaveTag(Dependency{}, "TaskId")
	DependencyStatusKey             = bsonutil.MustHaveTag(Dependency{}, "Status")
	DependencyUnattainableKey       = bsonutil.MustHaveTag(Dependency{}, "Unattainable")
	DependencyFinishedKey           = bsonutil.MustHaveTag(Dependency{}, "Finished")
	DependencyOmitGeneratedTasksKey = bsonutil.MustHaveTag(Dependency{}, "OmitGeneratedTasks")
)

var BaseTaskStatusKey = bsonutil.GetDottedKeyName(BaseTaskKey, StatusKey)

// Queries

// All returns all tasks.
var All = db.Query(nil)

var (
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

	updateDisplayTasksAndTasksExpression = bson.M{
		"$set": bson.M{
			CanResetKey: true,
		},
		"$unset": bson.M{
			AbortedKey:              "",
			AbortInfoKey:            "",
			OverrideDependenciesKey: "",
		},
		"$inc": bson.M{ExecutionKey: 1},
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
							{"$ne": []interface{}{"$" + OverrideDependenciesKey, true}},
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
		{"$lookup": bson.M{
			"from":         "builds",
			"localField":   BuildIdKey,
			"foreignField": "_id",
			"as":           BuildVariantDisplayNameKey,
		}},
		{"$unwind": bson.M{
			"path":                       "$" + BuildVariantDisplayNameKey,
			"preserveNullAndEmptyArrays": true,
		}},
		{"$addFields": bson.M{
			BuildVariantDisplayNameKey: "$" + bsonutil.GetDottedKeyName(BuildVariantDisplayNameKey, "display_name"),
		}},
	}

	// AddAnnotations adds the annotations to the task document.
	AddAnnotations = []bson.M{
		{
			"$facet": bson.M{
				// We skip annotation lookup for non-failed tasks, because these can't have annotations
				"not_failed": []bson.M{
					{
						"$match": bson.M{
							StatusKey: bson.M{"$nin": evergreen.TaskFailureStatuses},
						},
					},
				},
				// for failed tasks, get any annotation that has at least one issue
				"failed": []bson.M{
					{
						"$match": bson.M{
							StatusKey: bson.M{"$in": evergreen.TaskFailureStatuses},
						},
					},
					{
						"$lookup": bson.M{
							"from": annotations.Collection,
							"let":  bson.M{"task_annotation_id": "$" + IdKey, "task_annotation_execution": "$" + ExecutionKey},
							"pipeline": []bson.M{
								{
									"$match": bson.M{
										"$expr": bson.M{
											"$and": []bson.M{
												{
													"$eq": []string{"$" + annotations.TaskIdKey, "$$task_annotation_id"},
												},
												{
													"$eq": []string{"$" + annotations.TaskExecutionKey, "$$task_annotation_execution"},
												},
												{
													"$ne": []interface{}{
														bson.M{
															"$size": bson.M{"$ifNull": []interface{}{"$" + annotations.IssuesKey, []bson.M{}}},
														}, 0,
													},
												},
											},
										},
									}}},
							"as": "annotation_docs",
						},
					},
				},
			},
		},
		{"$project": bson.M{
			"tasks": bson.M{
				"$setUnion": []string{"$not_failed", "$failed"},
			}},
		},
		{"$unwind": "$tasks"},
		{"$replaceRoot": bson.M{"newRoot": "$tasks"}},
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
func DisplayTasksByVersion(version string, includeNeverActivatedTasks bool) bson.M {
	// assumes that all ExecutionTasks know of their corresponding DisplayTask (i.e. DisplayTaskIdKey not null or "")

	matchOnVersion := bson.M{VersionKey: version}
	if !includeNeverActivatedTasks {
		matchOnVersion[ActivatedTimeKey] = bson.M{"$ne": utility.ZeroTime}
	}
	query := bson.M{
		"$and": []bson.M{
			matchOnVersion,
			{"$or": []bson.M{
				{DisplayTaskIdKey: ""},                       // no 'parent' display task
				{DisplayOnlyKey: true},                       // ...
				{DisplayTaskIdKey: nil},                      // ...
				{ExecutionTasksKey: bson.M{"$exists": true}}, // has execution tasks
			},
			},
		},
	}

	return query
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

func FailedTasksByIds(taskIds []string) bson.M {
	return bson.M{
		IdKey:     bson.M{"$in": taskIds},
		StatusKey: bson.M{"$in": evergreen.TaskFailureStatuses},
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

// ByStaleRunningTask creates a query that finds any running tasks
// whose last heartbeat was at least the specified threshold ago.
func ByStaleRunningTask(staleness time.Duration) bson.M {
	return bson.M{
		StatusKey: bson.M{
			"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched},
		},
		DisplayOnlyKey: bson.M{
			"$ne": true,
		},
		LastHeartbeatKey: bson.M{
			"$lte": time.Now().Add(-staleness),
		},
	}
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

// ByPreviousCommit creates a query on Evergreen as the requester on a previous revision with the same buildVariant, displayName and project
func ByPreviousCommit(buildVariant, displayName, project, requester string, order int) bson.M {
	return bson.M{
		RequesterKey:           requester,
		BuildVariantKey:        buildVariant,
		DisplayNameKey:         displayName,
		ProjectKey:             project,
		RevisionOrderNumberKey: order - 1,
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

// ByTimeStartedAndFailed returns all failed tasks that started or finished between 2 given times
func ByTimeStartedAndFailed(startTime, endTime time.Time, commandTypes []string) bson.M {
	query := bson.M{
		"$or": []bson.M{
			{"$and": []bson.M{
				{StartTimeKey: bson.M{"$lte": endTime}},
				{StartTimeKey: bson.M{"$gte": startTime}},
			}},
			{"$and": []bson.M{
				{FinishTimeKey: bson.M{"$lte": endTime}},
				{FinishTimeKey: bson.M{"$gte": startTime}},
			}},
		},
		StatusKey: evergreen.TaskFailed,
	}
	if len(commandTypes) > 0 {
		query[bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailType)] = bson.M{
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
	return FindAll(db.Query(needsContainerAllocation()).Sort([]string{ActivatedTimeKey}))
}

// needsContainerAllocation returns the query that filters for a task that
// currently needs a container to be allocated to run it.
func needsContainerAllocation() bson.M {
	q := IsContainerTaskScheduledQuery()
	q[ContainerAllocatedKey] = false
	return q
}

func IsContainerTaskScheduledQuery() bson.M {
	return bson.M{
		StatusKey:            evergreen.TaskUndispatched,
		ActivatedKey:         true,
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
}

// TasksByProjectAndCommitPipeline fetches the pipeline to get the retrieve all mainline commit tasks
// associated with a given project and commit hash. Filtering by task status, task name, and
// buildvariant name are optionally available.
func TasksByProjectAndCommitPipeline(opts GetTasksByProjectAndCommitOptions) []bson.M {
	matchFilter := bson.M{
		ProjectKey:   opts.Project,
		RevisionKey:  opts.CommitHash,
		IdKey:        bson.M{"$gte": opts.StartingTaskId},
		RequesterKey: evergreen.RepotrackerVersionRequester,
	}
	if opts.Status != "" {
		matchFilter[StatusKey] = opts.Status
	}
	if opts.VariantName != "" {
		matchFilter[BuildVariantKey] = opts.VariantName
	}
	if opts.VariantRegex != "" {
		matchFilter[BuildVariantKey] = bson.M{"$regex": opts.VariantRegex, "$options": "i"}
	}
	if opts.TaskName != "" {
		matchFilter[DisplayNameKey] = opts.TaskName
	}
	pipeline := []bson.M{{"$match": matchFilter}}
	if opts.Limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": opts.Limit})
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
		return nil, errors.Wrap(err, "getting recently-finished tasks")
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
		return nil, errors.Wrap(err, "aggregating recently-finished task stats")
	}

	return result, nil
}

// FindByExecutionTasksAndMaxExecution returns the tasks corresponding to the
// passed in taskIds and execution, or the most recent executions of those
// tasks if they do not have a matching execution.
func FindByExecutionTasksAndMaxExecution(taskIds []string, execution int, filters ...bson.E) ([]Task, error) {
	query := bson.M{
		IdKey: bson.M{
			"$in": taskIds,
		},
		ExecutionKey: bson.M{
			"$lte": execution,
		},
	}
	for _, filter := range filters {
		query[filter.Key] = filter.Value
	}
	tasks, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "finding tasks")
	}

	// Get the taskIds that were not found in the previous match stage.
	var foundIds []string
	for _, t := range tasks {
		foundIds = append(foundIds, t.Id)
	}

	missingTasks, _ := utility.StringSliceSymmetricDifference(taskIds, foundIds)
	if len(missingTasks) > 0 {
		var oldTaskPipeline []bson.M
		match := bson.M{
			OldTaskIdKey: bson.M{
				"$in": missingTasks,
			},
			ExecutionKey: bson.M{
				"$lte": execution,
			},
		}
		for _, filter := range filters {
			match[filter.Key] = filter.Value
		}
		oldTaskPipeline = append(oldTaskPipeline, bson.M{"$match": match})

		// If there are multiple previous executions, matching on
		// non-zero executions with $lte will return duplicate tasks.
		// We sort and group to find and return the old task with the
		// most recent execution.
		oldTaskPipeline = append(oldTaskPipeline, bson.M{
			"$sort": bson.D{bson.E{Key: ExecutionKey, Value: -1}},
		})
		oldTaskPipeline = append(oldTaskPipeline, bson.M{
			"$group": bson.M{
				"_id":  "$" + OldTaskIdKey,
				"root": bson.M{"$first": "$$ROOT"},
			},
		})
		oldTaskPipeline = append(oldTaskPipeline, bson.M{"$replaceRoot": bson.M{"newRoot": "$root"}})

		var oldTasks []Task
		if err := db.Aggregate(OldCollection, oldTaskPipeline, &oldTasks); err != nil {
			return nil, errors.Wrap(err, "finding old tasks")
		}
		tasks = append(tasks, oldTasks...)
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	return tasks, nil
}

// FindHostRunnable finds all host tasks that can be scheduled for a distro with
// an additional consideration for whether the task's dependencies are met. If
// removeDeps is true, tasks with unmet dependencies are excluded.
func FindHostRunnable(distroID string, removeDeps bool) ([]Task, error) {
	match := schedulableHostTasksQuery()
	var d distro.Distro
	var err error
	if distroID != "" {
		foundDistro, err := distro.FindOne(distro.ById(distroID).WithFields(distro.ValidProjectsKey))
		if err != nil {
			return nil, errors.Wrapf(err, "finding distro '%s'", distroID)
		}
		if foundDistro != nil {
			d = *foundDistro
		}
	}

	if err = addApplicableDistroFilter(distroID, DistroIdKey, match); err != nil {
		return nil, errors.WithStack(err)
	}

	matchActivatedUndispatchedTasks := bson.M{
		"$match": match,
	}

	filterInvalidDistros := bson.M{
		"$match": bson.M{ProjectKey: bson.M{"$in": d.ValidProjects}},
	}

	removeFields := bson.M{
		"$project": bson.M{
			LogsKey:      0,
			OldTaskIdKey: 0,
			DependsOnKey + "." + DependencyUnattainableKey: 0,
		},
	}

	graphLookupTaskDeps := bson.M{
		"$graphLookup": bson.M{
			"from":             Collection,
			"startWith":        "$" + DependsOnKey + "." + IdKey,
			"connectFromField": DependsOnKey + "." + IdKey,
			"connectToField":   IdKey,
			"as":               dependencyKey,
			// restrict graphLookup to only direct dependencies
			"maxDepth": 0,
		},
	}

	unwindDependencies := bson.M{
		"$unwind": bson.M{
			"path":                       "$" + dependencyKey,
			"preserveNullAndEmptyArrays": true,
		},
	}

	unwindDependsOn := bson.M{
		"$unwind": bson.M{
			"path":                       "$" + DependsOnKey,
			"preserveNullAndEmptyArrays": true,
		},
	}

	matchIds := bson.M{
		"$match": bson.M{
			"$expr": bson.M{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyTaskIdKey), "$" + bsonutil.GetDottedKeyName(dependencyKey, IdKey)}},
		},
	}

	projectSatisfied := bson.M{
		"$addFields": bson.M{
			"satisfied_dependencies": bson.M{
				"$cond": bson.A{
					bson.M{
						"$or": []bson.M{
							{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyStatusKey), "$" + bsonutil.GetDottedKeyName(dependencyKey, StatusKey)}},
							{"$and": []bson.M{
								{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyStatusKey), "*"}},
								{"$or": []bson.M{
									{"$in": bson.A{"$" + bsonutil.GetDottedKeyName(dependencyKey, StatusKey), evergreen.TaskCompletedStatuses}},
									{"$anyElementTrue": "$" + bsonutil.GetDottedKeyName(dependencyKey, DependsOnKey, DependencyUnattainableKey)},
								}},
							}},
						},
					},
					true,
					false,
				},
			},
		},
	}

	regroupTasks := bson.M{
		"$group": bson.M{
			"_id":           "$_id",
			"satisfied_set": bson.M{"$addToSet": "$satisfied_dependencies"},
			"root":          bson.M{"$first": "$$ROOT"},
		},
	}

	redactUnsatisfiedDependencies := bson.M{
		"$redact": bson.M{
			"$cond": bson.A{
				bson.M{"$allElementsTrue": "$satisfied_set"},
				"$$KEEP",
				"$$PRUNE",
			},
		},
	}

	replaceRoot := bson.M{"$replaceRoot": bson.M{"newRoot": "$root"}}

	joinProjectRef := bson.M{
		"$lookup": bson.M{
			"from":         "project_ref",
			"localField":   ProjectKey,
			"foreignField": "_id",
			"as":           "project_ref",
		},
	}

	filterDisabledProjects := bson.M{
		"$match": bson.M{
			bsonutil.GetDottedKeyName("project_ref", "0", "enabled"):              true,
			bsonutil.GetDottedKeyName("project_ref", "0", "dispatching_disabled"): bson.M{"$ne": true},
		},
	}

	filterPatchingDisabledProjects := bson.M{
		"$match": bson.M{"$or": []bson.M{
			{
				RequesterKey: bson.M{"$nin": evergreen.PatchRequesters},
			},
			{
				bsonutil.GetDottedKeyName("project_ref", "0", "patching_disabled"): false,
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
		removeFields,
		graphLookupTaskDeps,
	}

	if distroID != "" && len(d.ValidProjects) > 0 {
		pipeline = append(pipeline, filterInvalidDistros)
	}

	if removeDeps {
		pipeline = append(pipeline,
			unwindDependencies,
			unwindDependsOn,
			matchIds,
			projectSatisfied,
			regroupTasks,
			redactUnsatisfiedDependencies,
			replaceRoot,
		)
	}

	pipeline = append(pipeline,
		joinProjectRef,
		filterDisabledProjects,
		filterPatchingDisabledProjects,
		removeProjectRef,
	)

	runnableTasks := []Task{}
	if err := Aggregate(pipeline, &runnableTasks); err != nil {
		return nil, errors.Wrap(err, "fetching runnable host tasks")
	}

	return runnableTasks, nil
}

// FindVariantsWithTask returns a list of build variants between specified commits that contain a specific task name
func FindVariantsWithTask(taskName, project string, orderMin, orderMax int) ([]string, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				ProjectKey:     project,
				RequesterKey:   evergreen.RepotrackerVersionRequester,
				DisplayNameKey: taskName,
				"$and": []bson.M{
					{RevisionOrderNumberKey: bson.M{"$gte": orderMin}},
					{RevisionOrderNumberKey: bson.M{"$lte": orderMax}},
				},
			},
		},
		{
			"$group": bson.M{
				"_id": "$" + BuildVariantKey,
			},
		},
	}
	docs := []map[string]string{}
	err := Aggregate(pipeline, &docs)
	if err != nil {
		return nil, errors.Wrapf(err, "finding variants with task named '%s'", taskName)
	}
	variants := []string{}
	for _, doc := range docs {
		variants = append(variants, doc["_id"])
	}
	return variants, nil
}

type BuildVariantTuple struct {
	BuildVariant string `bson:"build_variant"`
	DisplayName  string `bson:"build_variant_display_name"`
}

var (
	buildVariantTupleVariantKey     = bsonutil.MustHaveTag(BuildVariantTuple{}, "BuildVariant")
	buildVariantTupleDisplayNameKey = bsonutil.MustHaveTag(BuildVariantTuple{}, "DisplayName")
)

const VersionLimit = 50

// FindUniqueBuildVariantNamesByTask returns a list of unique build variants names and their display names for a given task name.
// It attempts to return the most recent display name for each build variant to avoid returning duplicates caused by display names changing.
// It only checks the last 50 versions that ran for a given task name.
func FindUniqueBuildVariantNamesByTask(projectId string, taskName string, repoOrderNumber int, legacyLookup bool) ([]*BuildVariantTuple, error) {
	query := bson.M{
		ProjectKey:     projectId,
		DisplayNameKey: taskName,
		RequesterKey:   bson.M{"$in": evergreen.SystemVersionRequesterTypes},
		"$and": []bson.M{
			{RevisionOrderNumberKey: bson.M{"$gte": repoOrderNumber - VersionLimit}},
			{RevisionOrderNumberKey: bson.M{"$lte": repoOrderNumber}},
		},
	}
	if !legacyLookup {
		query[BuildVariantDisplayNameKey] = bson.M{"$exists": true, "$ne": ""}
	}
	pipeline := []bson.M{{"$match": query}}

	// group the build variants by unique build variant names and get a build id for each
	groupByBuildVariant := bson.M{
		"$group": bson.M{
			"_id": bson.M{
				BuildVariantKey:            "$" + BuildVariantKey,
				BuildVariantDisplayNameKey: "$" + BuildVariantDisplayNameKey,
			},
			BuildIdKey: bson.M{
				"$first": "$" + BuildIdKey,
			},
		},
	}
	pipeline = append(pipeline, groupByBuildVariant)

	// reorganize the results to get the build variant names and a corresponding build id
	projectBuildIdAndVariant := bson.M{
		"$project": bson.M{
			"_id":                      0,
			BuildVariantKey:            bsonutil.GetDottedKeyName("$_id", BuildVariantKey),
			BuildVariantDisplayNameKey: bsonutil.GetDottedKeyName("$_id", BuildVariantDisplayNameKey),
			BuildIdKey:                 "$" + BuildIdKey,
		},
	}
	pipeline = append(pipeline, projectBuildIdAndVariant)

	// legacy tasks do not have variant display name directly set on them
	// so need to lookup from builds collection
	if legacyLookup {
		// get the display name for each build variant
		pipeline = append(pipeline, AddBuildVariantDisplayName...)
	}

	// cleanup the results
	projectBvResults := bson.M{
		"$project": bson.M{
			"_id":                           0,
			buildVariantTupleVariantKey:     "$" + BuildVariantKey,
			buildVariantTupleDisplayNameKey: "$" + BuildVariantDisplayNameKey,
		},
	}
	pipeline = append(pipeline, projectBvResults)

	// sort build variant display names alphabetically
	sortByVariantDisplayName := bson.M{
		"$sort": bson.M{
			buildVariantTupleDisplayNameKey: 1,
		},
	}
	pipeline = append(pipeline, sortByVariantDisplayName)

	result := []*BuildVariantTuple{}
	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "getting build variant tasks")
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
		return nil, errors.Wrap(err, "getting build variant tasks")
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
		return nil, errors.Wrap(err, "finding task by ID")
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
		return nil, errors.Wrap(err, "finding task by ID and execution")
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
		return nil, errors.Wrap(err, "finding tasks")
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
		return nil, errors.Wrap(err, "finding stuck dispatching tasks")
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
		return nil, errors.Wrapf(err, "finding task IDs for version '%s'", versionId)
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

// HasActivatedDependentTasks returns true if there are active tasks waiting on the given task.
func HasActivatedDependentTasks(taskId string) (bool, error) {
	numDependentTasks, err := Count(db.Query(bson.M{
		bsonutil.GetDottedKeyName(DependsOnKey, DependencyTaskIdKey): taskId,
		ActivatedKey:            true,
		OverrideDependenciesKey: bson.M{"$ne": true},
	}))

	return numDependentTasks > 0, err
}

func FindTaskGroupFromBuild(buildId, taskGroup string) ([]Task, error) {
	tasks, err := FindWithSort(bson.M{
		BuildIdKey:   buildId,
		TaskGroupKey: taskGroup,
	}, []string{TaskGroupOrderKey})
	if err != nil {
		return nil, errors.Wrap(err, "getting tasks in task group")
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
		return nil, errors.Wrap(err, "getting current tasks")
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
			return nil, errors.Wrap(err, "getting old tasks")
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

func updateUnattainableDependency(taskID string, unattainableDependency bool) error {
	return UpdateOne(
		bson.M{IdKey: taskID},
		bson.M{"$set": bson.M{UnattainableDependencyKey: unattainableDependency}},
	)
}

// AbortAndMarkResetTasksForBuild aborts and marks tasks for a build to reset when finished.
func AbortAndMarkResetTasksForBuild(buildId string, taskIds []string, caller string) error {
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
				AbortedKey:           true,
				AbortInfoKey:         AbortInfo{User: caller},
				ResetWhenFinishedKey: true,
			},
		},
	)
	return err
}

func AbortAndMarkResetTasksForVersion(versionId string, taskIds []string, caller string) error {
	_, err := UpdateAll(
		bson.M{
			VersionKey: versionId, // Include to improve query.
			IdKey:      bson.M{"$in": taskIds},
			StatusKey:  bson.M{"$in": evergreen.TaskAbortableStatuses},
		},
		bson.M{"$set": bson.M{
			AbortedKey:           true,
			AbortInfoKey:         AbortInfo{User: caller},
			ResetWhenFinishedKey: true,
		}},
	)
	return err
}

// HasUnfinishedTaskForVersions returns true if there are any scheduled but
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
	return errors.Wrap(err, "adding details of host creation failure to task")
}

func FindActivatedStepbackTasks(projectId string) ([]Task, error) {
	tasks, err := Find(bson.M{
		ProjectKey:     projectId,
		ActivatedKey:   true,
		ActivatedByKey: evergreen.StepbackTaskActivator,
		StatusKey:      bson.M{"$in": evergreen.TaskUncompletedStatuses},
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// FindActivatedStepbackTaskByName queries for running/scheduled stepback tasks with
// matching build variant and task name.
func FindActivatedStepbackTaskByName(projectId string, variantName string, taskName string) (*Task, error) {
	t, err := FindOne(db.Query(bson.M{
		ProjectKey:      projectId,
		BuildVariantKey: variantName,
		DisplayNameKey:  taskName,
		StatusKey:       bson.M{"$in": evergreen.TaskUncompletedStatuses},
		RequesterKey:    evergreen.RepotrackerVersionRequester,
		ActivatedKey:    true,
		ActivatedByKey:  evergreen.StepbackTaskActivator,
	}))
	if err != nil {
		return nil, err
	}
	return t, nil
}

// addStatusColorSort adds a stage which takes a task display status and returns an integer
// for the rank at which it should be sorted. the return value groups all statuses with the
// same color together. this should be kept consistent with the badge status colors in spruce
func addStatusColorSort(key string) bson.M {
	return bson.M{
		"$addFields": bson.M{
			"__" + key: bson.M{
				"$switch": bson.M{
					"branches": []bson.M{
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskFailed, evergreen.TaskTestTimedOut, evergreen.TaskTimedOut}},
							},
							"then": 1, // red
						},
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskKnownIssue}},
							},
							"then": 2,
						},
						{
							"case": bson.M{
								"$eq": []string{"$" + key, evergreen.TaskSetupFailed},
							},
							"then": 3, // lavender
						},
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskSystemFailed, evergreen.TaskSystemUnresponse, evergreen.TaskSystemTimedOut}},
							},
							"then": 4, // purple
						},
						{
							"case": bson.M{
								"$in": []interface{}{"$" + key, []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
							},
							"then": 5, // yellow
						},
						{
							"case": bson.M{
								"$eq": []string{"$" + key, evergreen.TaskSucceeded},
							},
							"then": 10, // green
						},
					},
					"default": 6, // all shades of grey
				},
			},
		},
	}
}

func recalculateTimeTaken() bson.M {
	return bson.M{
		"$set": bson.M{
			TimeTakenKey: bson.M{
				"$cond": bson.M{
					"if": bson.M{
						"$eq": []string{"$" + StatusKey, evergreen.TaskStarted},
					},
					// Time taken for a task is in nanoseconds. Since subtracting two dates in MongoDB yields milliseconds, we have
					// to multiply by time.Millisecond (1000000) to keep time taken consistently in nanoseconds.
					"then": bson.M{"$multiply": []interface{}{time.Millisecond, bson.M{"$subtract": []interface{}{"$$NOW", "$" + StartTimeKey}}}},
					"else": "$" + TimeTakenKey,
				},
			},
		},
	}
}

// GetTasksByVersion gets all tasks for a specific version
// Query results can be filtered by task name, variant name and status in addition to being paginated and limited
func GetTasksByVersion(ctx context.Context, versionID string, opts GetTasksByVersionOptions) ([]Task, int, error) {
	if opts.IncludeBuildVariantDisplayName {
		opts.UseLegacyAddBuildVariantDisplayName = shouldUseLegacyAddBuildVariantDisplayName(versionID)
	}

	pipeline, err := getTasksByVersionPipeline(versionID, opts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting tasks by version pipeline")
	}
	if len(opts.Sorts) > 0 {
		sortPipeline := []bson.M{}

		sortFields := bson.D{}
		for _, singleSort := range opts.Sorts {
			if singleSort.Key == DisplayStatusKey || singleSort.Key == BaseTaskStatusKey {
				sortPipeline = append(sortPipeline, addStatusColorSort(singleSort.Key))
				sortFields = append(sortFields, bson.E{Key: "__" + singleSort.Key, Value: singleSort.Order})
			} else if singleSort.Key == TimeTakenKey {
				sortPipeline = append(sortPipeline, recalculateTimeTaken())
				sortFields = append(sortFields, bson.E{Key: singleSort.Key, Value: singleSort.Order})
			} else {
				sortFields = append(sortFields, bson.E{Key: singleSort.Key, Value: singleSort.Order})
			}
		}
		sortFields = append(sortFields, bson.E{Key: IdKey, Value: 1})

		sortPipeline = append(sortPipeline, bson.M{
			"$sort": sortFields,
		})

		pipeline = append(pipeline, sortPipeline...)
	}

	if len(opts.FieldsToProject) > 0 {
		fieldKeys := bson.M{}
		for _, field := range opts.FieldsToProject {
			fieldKeys[field] = 1
		}
		pipeline = append(pipeline, bson.M{
			"$project": fieldKeys,
		})
	}

	// If there is a limit we should calculate the total count before we apply the limit and pagination
	if opts.Limit > 0 {
		paginatePipeline := []bson.M{}
		paginatePipeline = append(paginatePipeline, bson.M{
			"$skip": opts.Page * opts.Limit,
		})
		paginatePipeline = append(paginatePipeline, bson.M{
			"$limit": opts.Limit,
		})
		// Use a $facet to perform separate aggregations for $count and to sort and paginate the results in the same query
		tasksAndCountPipeline := bson.M{
			"$facet": bson.M{
				"count": []bson.M{
					{"$count": "count"},
				},
				"tasks": paginatePipeline,
			},
		}
		pipeline = append(pipeline, tasksAndCountPipeline)
	}

	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}

	var results []Task
	var count int

	// If there is no limit applied we should just return the tasks and compute the total count in go.
	// This avoids hitting the 16 MB limit on the aggregation pipeline in the $facet stage https://jira.mongodb.org/browse/EVG-15334
	if opts.Limit > 0 {
		type TasksAndCount struct {
			Tasks []Task           `bson:"tasks"`
			Count []map[string]int `bson:"count"`
		}
		taskAndCountResults := []TasksAndCount{}
		err = cursor.All(ctx, &taskAndCountResults)
		if err != nil {
			return nil, 0, err
		}
		if len(taskAndCountResults) > 0 && len(taskAndCountResults[0].Count) > 0 {
			count = taskAndCountResults[0].Count[0]["count"]
			results = taskAndCountResults[0].Tasks
		}
	} else {
		taskResults := []Task{}
		err = cursor.All(ctx, &taskResults)
		if err != nil {
			return nil, 0, err
		}
		results = taskResults
		count = len(results)
	}

	if len(results) == 0 {
		return nil, 0, nil
	}

	return results, count, nil
}

type StatusCount struct {
	Status string `bson:"status"`
	Count  int    `bson:"count"`
}

type TaskStats struct {
	Counts []StatusCount `bson:"counts"`
	ETA    *time.Time    `bson:"eta"`
}

type GroupedTaskStatusCount struct {
	Variant      string         `bson:"variant"`
	DisplayName  string         `bson:"display_name"`
	StatusCounts []*StatusCount `bson:"status_counts"`
}

func GetTaskStatsByVersion(versionID string, opts GetTasksByVersionOptions) (*TaskStats, error) {
	if opts.IncludeBuildVariantDisplayName {
		opts.UseLegacyAddBuildVariantDisplayName = shouldUseLegacyAddBuildVariantDisplayName(versionID)
	}
	pipeline, err := getTasksByVersionPipeline(versionID, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting tasks by version pipeline")
	}
	maxEtaPipeline := []bson.M{
		{
			"$match": bson.M{
				ExpectedDurationKey: bson.M{"$exists": true},
				StartTimeKey:        bson.M{"$exists": true},
				DisplayStatusKey:    bson.M{"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
			},
		},
		{
			"$project": bson.M{
				"eta": bson.M{
					"$add": []interface{}{
						bson.M{"$divide": []interface{}{"$" + ExpectedDurationKey, time.Millisecond}},
						"$" + StartTimeKey,
					},
				},
			},
		},
		{
			"$group": bson.M{
				"_id":     nil,
				"max_eta": bson.M{"$max": "$eta"},
			},
		},
		{
			"$project": bson.M{
				"_id":     0,
				"max_eta": 1,
			},
		},
	}
	groupPipeline := []bson.M{
		{"$group": bson.M{
			"_id":   "$" + DisplayStatusKey,
			"count": bson.M{"$sum": 1},
		}},
		{"$sort": bson.M{"_id": 1}},
		{"$project": bson.M{
			"status": "$_id",
			"count":  1,
		}},
	}
	facet := bson.M{"$facet": bson.M{
		"counts": groupPipeline,
		"eta":    maxEtaPipeline,
	}}
	pipeline = append(pipeline, facet)

	type maxETAForQuery struct {
		MaxETA time.Time `bson:"max_eta"`
	}

	type taskStatsForQueryResult struct {
		Counts []StatusCount    `bson:"counts"`
		ETA    []maxETAForQuery `bson:"eta"`
	}

	taskStats := []taskStatsForQueryResult{}
	if err := Aggregate(pipeline, &taskStats); err != nil {
		return nil, errors.Wrap(err, "aggregating task stats for version")
	}
	result := TaskStats{}
	result.Counts = taskStats[0].Counts
	if len(taskStats[0].ETA) > 0 {
		result.ETA = &taskStats[0].ETA[0].MaxETA
	}

	return &result, nil
}

func GetGroupedTaskStatsByVersion(versionID string, opts GetTasksByVersionOptions) ([]*GroupedTaskStatusCount, error) {
	opts.IncludeBuildVariantDisplayName = true
	opts.UseLegacyAddBuildVariantDisplayName = shouldUseLegacyAddBuildVariantDisplayName(versionID)
	pipeline, err := getTasksByVersionPipeline(versionID, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting tasks by version pipeline")
	}
	project := bson.M{"$project": bson.M{
		BuildVariantKey:            "$" + BuildVariantKey,
		BuildVariantDisplayNameKey: "$" + BuildVariantDisplayNameKey,
		DisplayStatusKey:           "$" + DisplayStatusKey,
	}}
	pipeline = append(pipeline, project)
	variantStatusesKey := "variant_statuses"
	statusCountsKey := "status_counts"
	groupByStatusPipeline := []bson.M{
		// Group tasks by variant
		{
			"$group": bson.M{
				"_id": "$" + BuildVariantKey,
				variantStatusesKey: bson.M{
					"$push": bson.M{
						DisplayStatusKey:           "$" + DisplayStatusKey,
						BuildVariantKey:            "$" + BuildVariantKey,
						BuildVariantDisplayNameKey: "$" + BuildVariantDisplayNameKey,
					},
				},
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$" + variantStatusesKey,
				"preserveNullAndEmptyArrays": false,
			},
		},
		{
			"$project": bson.M{
				variantStatusesKey: 1,
				"_id":              0,
			},
		},
		// Group tasks by variant and status and calculate count for each status
		{
			"$group": bson.M{
				"_id": bson.M{
					DisplayStatusKey:           "$" + bsonutil.GetDottedKeyName(variantStatusesKey, DisplayStatusKey),
					BuildVariantKey:            "$" + bsonutil.GetDottedKeyName(variantStatusesKey, BuildVariantKey),
					BuildVariantDisplayNameKey: "$" + bsonutil.GetDottedKeyName(variantStatusesKey, BuildVariantDisplayNameKey),
				},
				"count": bson.M{"$sum": 1},
			},
		},
		// Sort the values by status so they are sorted before being grouped. This will ensure that they are sorted in the array when they are grouped.
		{
			"$sort": bson.M{
				bsonutil.GetDottedKeyName("_id", DisplayStatusKey): 1,
			},
		},
		// Group the elements by build variant and status_counts
		{
			"$group": bson.M{
				"_id": bson.M{BuildVariantKey: "$" + bsonutil.GetDottedKeyName("_id", BuildVariantKey), BuildVariantDisplayNameKey: "$" + bsonutil.GetDottedKeyName("_id", BuildVariantDisplayNameKey)},
				statusCountsKey: bson.M{
					"$push": bson.M{
						"status": "$" + bsonutil.GetDottedKeyName("_id", DisplayStatusKey),
						"count":  "$count",
					},
				},
			},
		},
		{
			"$project": bson.M{
				"variant":       "$" + bsonutil.GetDottedKeyName("_id", BuildVariantKey),
				"display_name":  "$" + bsonutil.GetDottedKeyName("_id", BuildVariantDisplayNameKey),
				statusCountsKey: 1,
			},
		},
		// Sort build variants in alphanumeric order for final return
		{
			"$sort": bson.M{
				"display_name": 1,
			},
		},
	}
	pipeline = append(pipeline, groupByStatusPipeline...)
	result := []*GroupedTaskStatusCount{}

	if err := Aggregate(pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "aggregating task stats")
	}
	return result, nil

}

// GetBaseStatusesForActivatedTasks returns the base statuses for activated tasks on a version.
func GetBaseStatusesForActivatedTasks(versionID string, baseVersionID string) ([]string, error) {
	pipeline := []bson.M{}
	taskField := "tasks"

	// Fetch all activated tasks from version, and all tasks from base version
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{
			"$or": []bson.M{
				{VersionKey: baseVersionID},
				{VersionKey: versionID, ActivatedTimeKey: bson.M{"$ne": utility.ZeroTime}},
			},
		}})
	// Add display status
	pipeline = append(pipeline, addDisplayStatus)
	// Group by display name and build variant, and keep track of DisplayStatus and Version fields
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": bson.M{DisplayNameKey: "$" + DisplayNameKey, BuildVariantKey: "$" + BuildVariantKey},
			taskField: bson.M{"$push": bson.M{
				DisplayStatusKey: "$" + DisplayStatusKey,
				VersionKey:       "$" + VersionKey,
			}},
		},
	})
	// Only keep records that exist both on the version & base version (i.e. there are 2 copies)
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{taskField: bson.M{"$size": 2}},
	})
	// Unwind to put tasks into a state where it's easier to filter
	pipeline = append(pipeline, bson.M{
		"$unwind": bson.M{
			"path": "$" + taskField,
		},
	})
	// Filter out tasks that aren't from base version
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{bsonutil.GetDottedKeyName(taskField, VersionKey): baseVersionID},
	})
	// Group to get rid of duplicate statuses
	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": "$" + bsonutil.GetDottedKeyName(taskField, DisplayStatusKey),
		},
	})
	// Sort to guarantee order
	pipeline = append(pipeline, bson.M{
		"$sort": bson.D{
			bson.E{Key: "_id", Value: 1},
		},
	})

	res := []map[string]string{}
	err := Aggregate(pipeline, &res)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating base task statuses")
	}
	statuses := []string{}
	for _, r := range res {
		statuses = append(statuses, r["_id"])
	}
	return statuses, nil
}

type HasMatchingTasksOptions struct {
	TaskNames                  []string
	Variants                   []string
	Statuses                   []string
	IncludeNeverActivatedTasks bool
}

// HasMatchingTasks returns true if the version has tasks with the given statuses
func HasMatchingTasks(ctx context.Context, versionID string, opts HasMatchingTasksOptions) (bool, error) {
	options := GetTasksByVersionOptions{
		TaskNames:                      opts.TaskNames,
		Variants:                       opts.Variants,
		Statuses:                       opts.Statuses,
		IncludeNeverActivatedTasks:     !opts.IncludeNeverActivatedTasks,
		IncludeBuildVariantDisplayName: true,
	}
	if len(opts.Variants) > 0 {
		options.UseLegacyAddBuildVariantDisplayName = shouldUseLegacyAddBuildVariantDisplayName(versionID)
	}
	pipeline, err := getTasksByVersionPipeline(versionID, options)
	if err != nil {
		return false, errors.Wrap(err, "getting tasks by version pipeline")
	}
	pipeline = append(pipeline, bson.M{"$count": "count"})
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return false, err
	}

	type Count struct {
		Count int `bson:"count"`
	}
	count := []*Count{}
	err = cursor.All(ctx, &count)
	if err != nil {
		return false, err
	}
	if len(count) == 0 {
		return false, nil
	}
	return count[0].Count > 0, nil
}

// shouldUseLegacyAddBuildVariantDisplayName returns a boolean indicating whether the given version uses the buildVariantDisplayName field that was added in https://jira.mongodb.org/browse/EVG-16761
// This is used to determine whether we should use the buildVariantDisplayName field in the task document or if we should use a $lookup to retrieve it
func shouldUseLegacyAddBuildVariantDisplayName(versionID string) bool {
	query := db.Query(ByVersion(versionID))
	task, err := FindOne(query)
	if err != nil {
		return false
	}
	if task == nil {
		return false
	}

	return task.BuildVariantDisplayName == ""
}

type GetTasksByVersionOptions struct {
	Statuses                            []string
	BaseStatuses                        []string
	Variants                            []string
	TaskNames                           []string
	Page                                int
	Limit                               int
	FieldsToProject                     []string
	Sorts                               []TasksSortOrder
	IncludeExecutionTasks               bool
	IncludeBaseTasks                    bool
	IncludeNeverActivatedTasks          bool // NeverActivated tasks are tasks that lack an activation time
	IncludeBuildVariantDisplayName      bool
	IsMainlineCommit                    bool
	UseLegacyAddBuildVariantDisplayName bool
}

func getTasksByVersionPipeline(versionID string, opts GetTasksByVersionOptions) ([]bson.M, error) {
	match := bson.M{}
	if !opts.IncludeBuildVariantDisplayName && opts.UseLegacyAddBuildVariantDisplayName {
		return nil, errors.New("should not use UseLegacyAddBuildVariantDisplayName with !IncludeBuildVariantDisplayName")
	}
	match[VersionKey] = versionID

	// GeneratedJSON can often be large, so we filter it out by default
	projectOut := bson.M{
		GeneratedJSONAsStringKey: 0,
	}

	if len(opts.TaskNames) > 0 {
		taskNamesAsRegex := strings.Join(opts.TaskNames, "|")
		match[DisplayNameKey] = bson.M{"$regex": taskNamesAsRegex, "$options": "i"}
	}
	// Activated Time is needed to filter out generated tasks that have been generated but not yet activated
	if !opts.IncludeNeverActivatedTasks {
		match[ActivatedTimeKey] = bson.M{"$ne": utility.ZeroTime}
	}
	pipeline := []bson.M{
		{"$project": projectOut},
		{"$match": match},
	}

	// Add BuildVariantDisplayName to all the results if it we need to match on the entire set of results
	// This is an expensive operation so we only want to do it if we have to
	if len(opts.Variants) > 0 && opts.IncludeBuildVariantDisplayName {
		if opts.UseLegacyAddBuildVariantDisplayName {
			pipeline = append(pipeline, AddBuildVariantDisplayName...)
		}

		// Allow searching by either variant name or variant display
		variantsAsRegex := strings.Join(opts.Variants, "|")
		match = bson.M{
			"$or": []bson.M{
				{BuildVariantDisplayNameKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
				{BuildVariantKey: bson.M{"$regex": variantsAsRegex, "$options": "i"}},
			},
		}
		pipeline = append(pipeline, bson.M{"$match": match})
	}

	if !opts.IncludeExecutionTasks {
		const tempParentKey = "_parent"
		// Split tasks so that we only look up if the task is an execution task if display task ID is unset and
		// display only is false (i.e. we don't know if it's a display task or not).
		facet := bson.M{
			"$facet": bson.M{
				// We skip lookup for anything we already know is not part of a display task
				"id_empty": []bson.M{
					{
						"$match": bson.M{
							"$or": []bson.M{
								{DisplayTaskIdKey: ""},
								{DisplayOnlyKey: true},
							},
						},
					},
				},
				// No ID and not display task: lookup if it's an execution task for some task, and then filter it out if it is
				"no_id": []bson.M{
					{
						"$match": bson.M{
							DisplayTaskIdKey: nil,
							DisplayOnlyKey:   bson.M{"$ne": true},
						},
					},
					{"$lookup": bson.M{
						"from":         Collection,
						"localField":   IdKey,
						"foreignField": ExecutionTasksKey,
						"as":           tempParentKey,
					}},
					{
						"$match": bson.M{
							tempParentKey: []interface{}{},
						},
					},
				},
			},
		}
		pipeline = append(pipeline, facet)

		// Recombine the tasks so that we can continue the pipeline on the joined tasks
		recombineTasks := []bson.M{
			{"$project": bson.M{
				"tasks": bson.M{
					"$setUnion": []string{"$no_id", "$id_empty"},
				}},
			},
			{"$unwind": "$tasks"},
			{"$replaceRoot": bson.M{"newRoot": "$tasks"}},
		}

		pipeline = append(pipeline, recombineTasks...)
	}

	pipeline = append(pipeline, AddAnnotations...)
	pipeline = append(pipeline,
		// Add a field for the display status of each task
		addDisplayStatus,
	)
	// Filter on the computed display status before continuing to add additional fields.
	if len(opts.Statuses) > 0 {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				DisplayStatusKey: bson.M{"$in": opts.Statuses},
			},
		})
	}
	if opts.IncludeBaseTasks {
		baseCommitMatch := []bson.M{
			{"$eq": []string{"$" + BuildVariantKey, "$$" + BuildVariantKey}},
			{"$eq": []string{"$" + DisplayNameKey, "$$" + DisplayNameKey}},
		}

		// If we are requesting a mainline commit's base task we want to use the previous commit instead.
		if opts.IsMainlineCommit {
			baseCommitMatch = append(baseCommitMatch, bson.M{
				"$eq": []interface{}{"$" + RevisionOrderNumberKey, bson.M{
					"$subtract": []interface{}{"$$" + RevisionOrderNumberKey, 1},
				}},
			})
		} else {
			baseCommitMatch = append(baseCommitMatch, bson.M{
				"$eq": []string{"$" + RevisionKey, "$$" + RevisionKey},
			})
		}
		pipeline = append(pipeline, []bson.M{
			// Add data about the base task
			{"$lookup": bson.M{
				"from": Collection,
				"let": bson.M{
					RevisionKey:            "$" + RevisionKey,
					BuildVariantKey:        "$" + BuildVariantKey,
					DisplayNameKey:         "$" + DisplayNameKey,
					RevisionOrderNumberKey: "$" + RevisionOrderNumberKey,
				},
				"as": BaseTaskKey,
				"pipeline": []bson.M{
					{"$match": bson.M{
						RequesterKey: evergreen.RepotrackerVersionRequester,
						"$expr": bson.M{
							"$and": baseCommitMatch,
						},
					}},
					{"$project": bson.M{
						IdKey:     1,
						StatusKey: displayStatusExpression,
					}},
					{"$limit": 1},
				},
			}},
			{
				"$unwind": bson.M{
					"path":                       "$" + BaseTaskKey,
					"preserveNullAndEmptyArrays": true,
				},
			},
		}...,
		)
	}
	// Add the build variant display name to the returned subset of results if it wasn't added earlier
	if len(opts.Variants) == 0 && opts.IncludeBuildVariantDisplayName {
		if opts.UseLegacyAddBuildVariantDisplayName {
			pipeline = append(pipeline, AddBuildVariantDisplayName...)
		}
	}

	if opts.IncludeBaseTasks && len(opts.BaseStatuses) > 0 {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				BaseTaskStatusKey: bson.M{"$in": opts.BaseStatuses},
			},
		})
	}

	return pipeline, nil
}

func (t *Task) FindAllUnmarkedBlockedDependencies() ([]Task, error) {
	okStatusSet := []string{AllStatuses, t.Status}
	query := db.Query(bson.M{
		DependsOnKey: bson.M{"$elemMatch": bson.M{
			DependencyTaskIdKey:       t.Id,
			DependencyStatusKey:       bson.M{"$nin": okStatusSet},
			DependencyUnattainableKey: false,
		},
		}},
	)
	return FindAll(query)
}

func (t *Task) FindAllMarkedUnattainableDependencies() ([]Task, error) {
	query := db.Query(bson.M{
		DependsOnKey: bson.M{"$elemMatch": bson.M{
			DependencyTaskIdKey:       t.Id,
			DependencyUnattainableKey: true,
		},
		}},
	)
	return FindAll(query)
}

func activateTasks(taskIDs []string, caller string, activationTime time.Time) error {
	_, err := UpdateAll(
		bson.M{
			IdKey: bson.M{"$in": taskIDs},
		},
		bson.M{
			"$set": bson.M{
				ActivatedKey:     true,
				ActivatedByKey:   caller,
				ActivatedTimeKey: activationTime,
			},
		})
	if err != nil {
		return errors.Wrap(err, "setting tasks to active")
	}
	if err = enableDisabledTasks(taskIDs); err != nil {
		return errors.Wrap(err, "enabling disabled tasks")
	}
	return nil
}

func enableDisabledTasks(taskIDs []string) error {
	_, err := UpdateAll(
		bson.M{
			IdKey:       bson.M{"$in": taskIDs},
			PriorityKey: evergreen.DisabledTaskPriority,
		},
		bson.M{
			"$unset": bson.M{
				PriorityKey: 1,
			},
		})
	return err
}

type NumExecutionsForIntervalInput struct {
	ProjectId    string
	BuildVarName string
	TaskName     string
	Requesters   []string
	StartTime    time.Time
	EndTime      time.Time
}

func CountNumExecutionsForInterval(input NumExecutionsForIntervalInput) (int, error) {
	query := bson.M{
		ProjectKey:      input.ProjectId,
		BuildVariantKey: input.BuildVarName,
		DisplayNameKey:  input.TaskName,
		StatusKey:       bson.M{"$in": evergreen.TaskCompletedStatuses},
	}
	if len(input.Requesters) > 0 {
		query[RequesterKey] = bson.M{"$in": input.Requesters}
	} else {
		query[RequesterKey] = bson.M{"$in": evergreen.SystemVersionRequesterTypes}
	}
	if !utility.IsZeroTime(input.EndTime) {
		query["$and"] = []bson.M{
			{FinishTimeKey: bson.M{"$gt": input.StartTime}},
			{FinishTimeKey: bson.M{"$lte": input.EndTime}},
		}
	} else {
		query[FinishTimeKey] = bson.M{"$gt": input.StartTime}
	}
	numTasks, err := db.Count(Collection, query)
	if err != nil {
		return 0, errors.Wrap(err, "counting task executions")
	}
	numOldTasks, err := db.Count(OldCollection, query)
	if err != nil {
		return 0, errors.Wrap(err, "counting old task executions")
	}
	return numTasks + numOldTasks, nil
}
