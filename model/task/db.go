package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
		{Key: UnattainableDependencyKey, Value: 1},
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
	NumDependentsKey               = bsonutil.MustHaveTag(Task{}, "NumDependents")
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
	CheckRunIdKey                  = bsonutil.MustHaveTag(Task{}, "CheckRunId")
	RevisionOrderNumberKey         = bsonutil.MustHaveTag(Task{}, "RevisionOrderNumber")
	RequesterKey                   = bsonutil.MustHaveTag(Task{}, "Requester")
	StatusKey                      = bsonutil.MustHaveTag(Task{}, "Status")
	DetailsKey                     = bsonutil.MustHaveTag(Task{}, "Details")
	AbortedKey                     = bsonutil.MustHaveTag(Task{}, "Aborted")
	AbortInfoKey                   = bsonutil.MustHaveTag(Task{}, "AbortInfo")
	TimeTakenKey                   = bsonutil.MustHaveTag(Task{}, "TimeTaken")
	TaskCostKey                    = bsonutil.MustHaveTag(Task{}, "TaskCost")
	PredictedTaskCostKey           = bsonutil.MustHaveTag(Task{}, "PredictedTaskCost")
	S3UsageKey                     = bsonutil.MustHaveTag(Task{}, "S3Usage")
	ExpectedDurationKey            = bsonutil.MustHaveTag(Task{}, "ExpectedDuration")
	ExpectedDurationStddevKey      = bsonutil.MustHaveTag(Task{}, "ExpectedDurationStdDev")
	DurationPredictionKey          = bsonutil.MustHaveTag(Task{}, "DurationPrediction")
	PriorityKey                    = bsonutil.MustHaveTag(Task{}, "Priority")
	ActivatedByKey                 = bsonutil.MustHaveTag(Task{}, "ActivatedBy")
	StepbackInfoKey                = bsonutil.MustHaveTag(Task{}, "StepbackInfo")
	ExecutionTasksKey              = bsonutil.MustHaveTag(Task{}, "ExecutionTasks")
	DisplayOnlyKey                 = bsonutil.MustHaveTag(Task{}, "DisplayOnly")
	DisplayTaskIdKey               = bsonutil.MustHaveTag(Task{}, "DisplayTaskId")
	ParentPatchIDKey               = bsonutil.MustHaveTag(Task{}, "ParentPatchID")
	TaskGroupKey                   = bsonutil.MustHaveTag(Task{}, "TaskGroup")
	TaskGroupMaxHostsKey           = bsonutil.MustHaveTag(Task{}, "TaskGroupMaxHosts")
	TaskGroupOrderKey              = bsonutil.MustHaveTag(Task{}, "TaskGroupOrder")
	GenerateTaskKey                = bsonutil.MustHaveTag(Task{}, "GenerateTask")
	GeneratedTasksKey              = bsonutil.MustHaveTag(Task{}, "GeneratedTasks")
	GeneratedByKey                 = bsonutil.MustHaveTag(Task{}, "GeneratedBy")
	TaskOutputInfoKey              = bsonutil.MustHaveTag(Task{}, "TaskOutputInfo")
	ResultsServiceKey              = bsonutil.MustHaveTag(Task{}, "ResultsService")
	HasTestResultsKey              = bsonutil.MustHaveTag(Task{}, "HasTestResults")
	ResultsFailedKey               = bsonutil.MustHaveTag(Task{}, "ResultsFailed")
	IsGithubCheckKey               = bsonutil.MustHaveTag(Task{}, "IsGithubCheck")
	HostCreateDetailsKey           = bsonutil.MustHaveTag(Task{}, "HostCreateDetails")

	GeneratedJSONAsStringKey      = bsonutil.MustHaveTag(Task{}, "GeneratedJSONAsString")
	GeneratedJSONStorageMethodKey = bsonutil.MustHaveTag(Task{}, "GeneratedJSONStorageMethod")
	GenerateTasksErrorKey         = bsonutil.MustHaveTag(Task{}, "GenerateTasksError")
	GeneratedTasksToActivateKey   = bsonutil.MustHaveTag(Task{}, "GeneratedTasksToActivate")
	NumGeneratedTasksKey          = bsonutil.MustHaveTag(Task{}, "NumGeneratedTasks")
	EstimatedNumGeneratedTasksKey = bsonutil.MustHaveTag(Task{}, "EstimatedNumGeneratedTasks")
	NumActivatedGeneratedTasksKey = bsonutil.MustHaveTag(Task{}, "NumActivatedGeneratedTasks")
	ResetWhenFinishedKey          = bsonutil.MustHaveTag(Task{}, "ResetWhenFinished")
	ResetFailedWhenFinishedKey    = bsonutil.MustHaveTag(Task{}, "ResetFailedWhenFinished")
	NumAutomaticRestartsKey       = bsonutil.MustHaveTag(Task{}, "NumAutomaticRestarts")
	IsAutomaticRestartKey         = bsonutil.MustHaveTag(Task{}, "IsAutomaticRestart")
	DisplayStatusKey              = bsonutil.MustHaveTag(Task{}, "DisplayStatus")
	DisplayStatusCacheKey         = bsonutil.MustHaveTag(Task{}, "DisplayStatusCache")
	BaseTaskKey                   = bsonutil.MustHaveTag(Task{}, "BaseTask")
	BuildVariantDisplayNameKey    = bsonutil.MustHaveTag(Task{}, "BuildVariantDisplayName")
	IsEssentialToSucceedKey       = bsonutil.MustHaveTag(Task{}, "IsEssentialToSucceed")
	HasAnnotationsKey             = bsonutil.MustHaveTag(Task{}, "HasAnnotations")
	NumNextTaskDispatchesKey      = bsonutil.MustHaveTag(Task{}, "NumNextTaskDispatches")
	CachedProjectStorageMethodKey = bsonutil.MustHaveTag(Task{}, "CachedProjectStorageMethod")
)

var (
	// BSON fields for stepback information
	LastFailingStepbackTaskIdKey = bsonutil.MustHaveTag(StepbackInfo{}, "LastFailingStepbackTaskId")
	LastPassingStepbackTaskIdKey = bsonutil.MustHaveTag(StepbackInfo{}, "LastPassingStepbackTaskId")
	NextStepbackTaskIdKey        = bsonutil.MustHaveTag(StepbackInfo{}, "NextStepbackTaskId")
	PreviousStepbackTaskIdKey    = bsonutil.MustHaveTag(StepbackInfo{}, "PreviousStepbackTaskId")
	GeneratedStepbackInfoKey     = bsonutil.MustHaveTag(StepbackInfo{}, "GeneratedStepbackInfo")
	StepbackInfoDisplayNameKey   = bsonutil.MustHaveTag(StepbackInfo{}, "DisplayName")
	StepbackInfoBuildVariantKey  = bsonutil.MustHaveTag(StepbackInfo{}, "BuildVariant")
)

var (
	// BSON fields for task status details struct
	TaskEndDetailStatus         = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "Status")
	TaskEndDetailTimedOut       = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "TimedOut")
	TaskEndDetailType           = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "Type")
	TaskEndDetailDescription    = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "Description")
	TaskEndDetailDiskDevicesKey = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "DiskDevices")
	TaskEndDetailTraceIDKey     = bsonutil.MustHaveTag(apimodels.TaskEndDetail{}, "TraceID")
)

var (
	// BSON fields for task dependency struct
	DependencyTaskIdKey             = bsonutil.MustHaveTag(Dependency{}, "TaskId")
	DependencyStatusKey             = bsonutil.MustHaveTag(Dependency{}, "Status")
	DependencyUnattainableKey       = bsonutil.MustHaveTag(Dependency{}, "Unattainable")
	DependencyFinishedKey           = bsonutil.MustHaveTag(Dependency{}, "Finished")
	DependencyFinishedAtKey         = bsonutil.MustHaveTag(Dependency{}, "FinishedAt")
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
			"in":           bson.M{"$or": []any{"$$value", bsonutil.GetDottedKeyName("$$this", DependencyUnattainableKey)}},
		},
	}

	addDisplayStatus = bson.M{
		"$addFields": bson.M{
			DisplayStatusKey: DisplayStatusExpression,
		},
	}

	addDisplayStatusCache = bson.M{
		"$addFields": bson.M{
			DisplayStatusCacheKey: DisplayStatusExpression,
		},
	}

	updateDisplayTasksAndTasksSet = bson.M{
		"$set": bson.M{
			CanResetKey: true,
			ExecutionKey: bson.M{
				"$add": bson.A{"$" + ExecutionKey, 1},
			},
		}}
	updateDisplayTasksAndTasksUnset = bson.M{
		"$unset": bson.A{
			AbortedKey,
			AbortInfoKey,
			OverrideDependenciesKey,
			HasAnnotationsKey,
		}}

	// This should reflect Task.GetDisplayStatus()
	DisplayStatusExpression = bson.M{
		"$switch": bson.M{
			"branches": []bson.M{
				{
					"case": bson.M{
						"$eq": []any{"$" + HasAnnotationsKey, true},
					},
					"then": evergreen.TaskKnownIssue,
				},
				{
					"case": bson.M{
						"$eq": []any{"$" + AbortedKey, true},
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
							{"$eq": []any{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut), true}},
							{"$eq": []string{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailDescription), evergreen.TaskDescriptionHeartbeat}},
						},
					},
					"then": evergreen.TaskSystemUnresponse,
				},
				{
					"case": bson.M{
						"$and": []bson.M{
							{"$eq": []string{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailType), evergreen.CommandTypeSystem}},
							{"$eq": []any{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut), true}},
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
						"$eq": []any{"$" + bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut), true},
					},
					"then": evergreen.TaskTimedOut,
				},
				// A task will be unscheduled if it is not activated
				{
					"case": bson.M{
						"$and": []bson.M{
							{"$eq": []any{"$" + ActivatedKey, false}},
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
							{"$ne": []any{"$" + OverrideDependenciesKey, true}},
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
							{"$eq": []any{"$" + ActivatedKey, true}},
						},
					},
					"then": evergreen.TaskWillRun,
				},
			},
			"default": "$" + StatusKey,
		},
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

// ByIdAndExecution creates a query that finds a task by its _id and execution.
func ByIdAndExecution(id string, execution int) bson.M {
	return bson.M{
		IdKey:        id,
		ExecutionKey: execution,
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

// ByBuildIdAndGithubChecks creates a query to return github check tasks with a certain build ID.
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

// ByVersion creates a query to return tasks with a certain version id
func ByVersion(version string) bson.M {
	return bson.M{
		VersionKey: version,
	}
}

// ByVersionWithChildTasks creates a query to return tasks or child tasks associated with the given version.
func ByVersionWithChildTasks(version string) bson.M {
	return bson.M{
		"$or": []bson.M{
			ByVersion(version),
			{ParentPatchIDKey: version},
		},
	}
}

// ByVersions produces a query that returns tasks for the given version.
func ByVersions(versionIDs []string) bson.M {
	return bson.M{VersionKey: bson.M{"$in": versionIDs}}
}

// ByVersionsWithChildTasks produces a query that returns tasks and child tasks for the given version.
func ByVersionsWithChildTasks(versionIDs []string) bson.M {
	return bson.M{
		"$or": []bson.M{
			ByVersions(versionIDs),
			{ParentPatchIDKey: bson.M{"$in": versionIDs}},
		},
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

// PotentiallyBlockedTasksByIds finds tasks with the given task ids
// that have dependencies (these could be completed or not), do not have
// override dependencies set to true, and the dependencies met time has not
// been set.
func PotentiallyBlockedTasksByIds(taskIds []string) bson.M {
	return bson.M{
		IdKey: bson.M{"$in": taskIds},
		"$and": []bson.M{
			{DependsOnKey: bson.M{
				"$exists": true,
				"$not":    bson.M{"$size": 0},
			}},
			{"$or": []bson.M{
				{OverrideDependenciesKey: bson.M{"$exists": false}},
				{OverrideDependenciesKey: false},
			}},
			{"$or": []bson.M{
				{DependenciesMetTimeKey: bson.M{"$exists": false}},
				{DependenciesMetTimeKey: bson.M{"$eq": utility.ZeroTime}},
			}},
		},
	}
}

func FailedTasksByIds(taskIds []string) bson.M {
	return bson.M{
		IdKey:     bson.M{"$in": taskIds},
		StatusKey: bson.M{"$in": evergreen.TaskFailureStatuses},
	}
}

// NonExecutionTasksByVersions will filter out newer execution tasks that store if they have a display task.
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
		RevisionOrderNumberKey: bson.M{"$lt": order},
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
			{UnattainableDependencyKey: false},
			{OverrideDependenciesKey: true},
		}},
	}

	return q
}

// FindNeedsContainerAllocation returns all container tasks that are waiting for
// a container to be allocated to them sorted by activation time.
func FindNeedsContainerAllocation(ctx context.Context) ([]Task, error) {
	return FindAll(ctx, db.Query(needsContainerAllocation()).Sort([]string{ActivatedTimeKey}))
}

// needsContainerAllocation returns the query that filters for a task that
// currently needs a container to be allocated to run it.
func needsContainerAllocation() bson.M {
	q := ScheduledContainerTasksQuery()
	q[ContainerAllocatedKey] = false
	return q
}

// ScheduledContainerTasksQuery returns a query indicating if the container is
// in a state where it is scheduled to run and is logically equivalent to
// (Task).isContainerScheduled. This encompasses two potential states:
//  1. A container is not yet allocated to the task but it's ready to be
//     allocated one. Note that this is a subset of all tasks that could
//     eventually run (i.e. evergreen.TaskWillRun from (Task).GetDisplayStatus),
//     because a container task is not scheduled until all of its dependencies
//     have been met.
//  2. The container is allocated but the agent has not picked up the task yet.
func ScheduledContainerTasksQuery() bson.M {
	query := UndispatchedContainerTasksQuery()
	query["$or"] = []bson.M{
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
	}
	return query
}

// UndispatchedContainerTasksQuery returns a query retrieving all undispatched container tasks.
func UndispatchedContainerTasksQuery() bson.M {
	return bson.M{
		StatusKey:            evergreen.TaskUndispatched,
		ActivatedKey:         true,
		ExecutionPlatformKey: ExecutionPlatformContainer,
		PriorityKey:          bson.M{"$gt": evergreen.DisabledTaskPriority},
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

// TasksByBuildIdPipeline fetches the pipeline to Get the retrieve all tasks
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

	// sort the tasks before limiting to Get the next [limit] tasks
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
func GetRecentTasks(ctx context.Context, period time.Duration) ([]Task, error) {
	query := db.Query(
		bson.M{
			StatusKey: bson.M{"$exists": true},
			FinishTimeKey: bson.M{
				"$gt": time.Now().Add(-period),
			},
		},
	)

	tasks := []Task{}
	err := db.FindAllQ(ctx, Collection, query, &tasks)
	if err != nil {
		return nil, errors.Wrap(err, "getting recently-finished tasks")
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

func GetRecentTaskStats(ctx context.Context, period time.Duration, nameKey string) ([]StatusItem, error) {
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
	if err := Aggregate(ctx, pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "aggregating recently-finished task stats")
	}

	return result, nil
}

// FindByExecutionTasksAndMaxExecution returns the tasks corresponding to the
// passed in taskIds and execution, or the most recent executions of those
// tasks if they do not have a matching execution.
func FindByExecutionTasksAndMaxExecution(ctx context.Context, taskIds []string, execution int, filters ...bson.E) ([]Task, error) {
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
	tasks, err := Find(ctx, query)
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
		if err := db.Aggregate(ctx, OldCollection, oldTaskPipeline, &oldTasks); err != nil {
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
func FindHostRunnable(ctx context.Context, distroID string, removeDeps bool) ([]Task, error) {
	match := schedulableHostTasksQuery()
	var d distro.Distro
	var err error
	if distroID != "" {
		foundDistro, err := distro.FindOne(ctx, distro.ById(distroID), options.FindOne().SetProjection(bson.M{distro.ValidProjectsKey: 1}))
		if err != nil {
			return nil, errors.Wrapf(err, "finding distro '%s'", distroID)
		}
		if foundDistro != nil {
			d = *foundDistro
		}
	}

	if err = addApplicableDistroFilter(ctx, distroID, DistroIdKey, match); err != nil {
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
	if err := Aggregate(ctx, pipeline, &runnableTasks); err != nil {
		return nil, errors.Wrap(err, "fetching runnable host tasks")
	}

	return runnableTasks, nil
}

// FindVariantsWithTask returns a list of build variants between specified commits that contain a specific task name
func FindVariantsWithTask(ctx context.Context, taskName, project string, orderMin, orderMax int) ([]string, error) {
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
	err := Aggregate(ctx, pipeline, &docs)
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
func FindUniqueBuildVariantNamesByTask(ctx context.Context, projectId string, taskName string, repoOrderNumber int) ([]*BuildVariantTuple, error) {
	query := bson.M{
		ProjectKey:     projectId,
		DisplayNameKey: taskName,
		RequesterKey:   bson.M{"$in": evergreen.SystemVersionRequesterTypes},
		"$and": []bson.M{
			{RevisionOrderNumberKey: bson.M{"$gte": repoOrderNumber - VersionLimit}},
			{RevisionOrderNumberKey: bson.M{"$lte": repoOrderNumber}},
		},
	}
	pipeline := []bson.M{{"$match": query}}

	// group the build variants by unique build variant names and Get a build id for each
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

	// reorganize the results to Get the build variant names and a corresponding build id
	projectBuildIdAndVariant := bson.M{
		"$project": bson.M{
			"_id":                      0,
			BuildVariantKey:            bsonutil.GetDottedKeyName("$_id", BuildVariantKey),
			BuildVariantDisplayNameKey: bsonutil.GetDottedKeyName("$_id", BuildVariantDisplayNameKey),
			BuildIdKey:                 "$" + BuildIdKey,
		},
	}
	pipeline = append(pipeline, projectBuildIdAndVariant)

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
	if err := Aggregate(ctx, pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "getting build variant tasks")
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// FindTaskNamesByBuildVariant returns a list of unique task names for a given build variant
func FindTaskNamesByBuildVariant(ctx context.Context, projectId string, buildVariant string, repoOrderNumber int) ([]string, error) {
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
	if err := Aggregate(ctx, pipeline, &result); err != nil {
		return nil, errors.Wrap(err, "getting build variant tasks")
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result[0].Tasks, nil
}

// DB Boilerplate

// FindOne returns a single task that satisfies the query.
func FindOne(ctx context.Context, query db.Q) (*Task, error) {
	task := &Task{}
	err := db.FindOneQ(ctx, Collection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

// FindOneId returns a single task with the given ID.
func FindOneId(ctx context.Context, id string) (*Task, error) {
	task, err := FindOne(ctx, db.Query(bson.M{IdKey: id}))

	return task, errors.Wrap(err, "finding task by ID")
}

// FindByIdExecution returns a single task with the given ID and execution. If
// execution is nil, the latest execution is returned.
func FindByIdExecution(ctx context.Context, id string, execution *int) (*Task, error) {
	if execution == nil {
		return FindOneId(ctx, id)
	}
	return FindOneIdAndExecution(ctx, id, *execution)
}

// FindOneIdAndExecution returns a single task with the given ID and execution.
func FindOneIdAndExecution(ctx context.Context, id string, execution int) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{
		IdKey:        id,
		ExecutionKey: execution,
	})
	err := db.FindOneQ(ctx, Collection, query, task)

	if adb.ResultsNotFound(err) {
		return FindOneOldByIdAndExecution(ctx, id, execution)
	}
	if err != nil {
		return nil, errors.Wrap(err, "finding task by ID and execution")
	}

	return task, nil
}

// FindOneIdAndExecutionWithDisplayStatus returns a single task with the given
// ID and execution, with display statuses added.
func FindOneIdAndExecutionWithDisplayStatus(ctx context.Context, id string, execution *int) (*Task, error) {
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
	if err := Aggregate(ctx, pipeline, &tasks); err != nil {
		return nil, errors.Wrap(err, "finding task")
	}
	if len(tasks) != 0 {
		t := tasks[0]
		return &t, nil
	}

	return findOneOldByIdAndExecutionWithDisplayStatus(ctx, id, execution)
}

// findOneOldByIdAndExecutionWithDisplayStatus returns a single task with the
// given ID and execution from the old tasks collection, with display statuses
// added.
func findOneOldByIdAndExecutionWithDisplayStatus(ctx context.Context, id string, execution *int) (*Task, error) {
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

	if err := db.Aggregate(ctx, OldCollection, pipeline, &tasks); err != nil {
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
func FindOneOld(ctx context.Context, filter bson.M) (*Task, error) {
	task := &Task{}
	query := db.Query(filter)
	err := db.FindOneQ(ctx, OldCollection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

func FindOneOldWithFields(ctx context.Context, filter bson.M, fields ...string) (*Task, error) {
	task := &Task{}
	query := db.Query(filter).WithFields(fields...)
	err := db.FindOneQ(ctx, OldCollection, query, task)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return task, err
}

func FindOneOldId(ctx context.Context, id string) (*Task, error) {
	filter := bson.M{
		IdKey: id,
	}
	return FindOneOld(ctx, filter)
}

// FindOneOldByIdAndExecution returns a single task from the old tasks
// collection with the given ID and execution.
func FindOneOldByIdAndExecution(ctx context.Context, id string, execution int) (*Task, error) {
	filter := bson.M{
		OldTaskIdKey: id,
		ExecutionKey: execution,
	}
	return FindOneOld(ctx, filter)
}

// FindOneIdWithFields returns a single task with the given ID, projecting only
// the given fields.
func FindOneIdWithFields(ctx context.Context, id string, projected ...string) (*Task, error) {
	task := &Task{}
	query := db.Query(bson.M{IdKey: id})

	if len(projected) > 0 {
		query = query.WithFields(projected...)
	}

	err := db.FindOneQ(ctx, Collection, query, task)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return task, nil
}

// findAllTaskIDs returns a list of task IDs associated with the given query.
func findAllTaskIDs(ctx context.Context, q db.Q) ([]string, error) {
	tasks := []Task{}
	err := db.FindAllQ(ctx, Collection, q, &tasks)
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
func FindStuckDispatching(ctx context.Context) ([]Task, error) {
	tasks, err := FindAll(ctx, db.Query(bson.M{
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

// FindAllTaskIDsFromVersion returns a list of task IDs associated with a version.
func FindAllTaskIDsFromVersion(ctx context.Context, versionId string) ([]string, error) {
	q := db.Query(ByVersion(versionId)).WithFields(IdKey)
	return findAllTaskIDs(ctx, q)
}

// FindAllTaskIDsFromBuild returns a list of task IDs associated with a build.
func FindAllTaskIDsFromBuild(ctx context.Context, buildId string) ([]string, error) {
	q := db.Query(ByBuildId(buildId)).WithFields(IdKey)
	return findAllTaskIDs(ctx, q)
}

// FindAllTasksFromVersionWithDependencies finds all tasks in a version and includes only their dependencies.
func FindAllTasksFromVersionWithDependencies(ctx context.Context, versionId string) ([]Task, error) {
	q := db.Query(ByVersion(versionId)).WithFields(IdKey, DependsOnKey)
	tasks := []Task{}
	err := db.FindAllQ(ctx, Collection, q, &tasks)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task IDs for version '%s'", versionId)
	}
	return tasks, nil
}

// FindTasksFromVersions returns all tasks associated with the given versions. Note that this only returns a few key fields.
func FindTasksFromVersions(ctx context.Context, versionIds []string) ([]Task, error) {
	return FindWithFields(ctx, ByVersions(versionIds),
		IdKey, DisplayNameKey, StatusKey, TimeTakenKey, VersionKey, BuildVariantKey, AbortedKey, AbortInfoKey)
}

func CountActivatedTasksForVersion(ctx context.Context, versionId string) (int, error) {
	return Count(ctx, db.Query(bson.M{
		VersionKey:   versionId,
		ActivatedKey: true,
	}))
}

// HasActivatedDependentTasks returns true if there are active tasks waiting on the given task.
func HasActivatedDependentTasks(ctx context.Context, taskId string) (bool, error) {
	numDependentTasks, err := Count(ctx, db.Query(bson.M{
		bsonutil.GetDottedKeyName(DependsOnKey, DependencyTaskIdKey): taskId,
		ActivatedKey:            true,
		OverrideDependenciesKey: bson.M{"$ne": true},
	}))

	return numDependentTasks > 0, err
}

func FindTaskGroupFromBuild(ctx context.Context, buildId, taskGroup string) ([]Task, error) {
	tasks, err := FindWithSort(ctx, bson.M{
		BuildIdKey:   buildId,
		TaskGroupKey: taskGroup,
	}, []string{TaskGroupOrderKey})
	if err != nil {
		return nil, errors.Wrap(err, "getting tasks in task group")
	}
	return tasks, nil
}

// FindOldWithDisplayTasks returns all display and execution tasks from the old
// collection that satisfy the given query.
func FindOldWithDisplayTasks(ctx context.Context, filter bson.M) ([]Task, error) {
	tasks := []Task{}
	query := db.Query(filter)
	err := db.FindAllQ(ctx, OldCollection, query, &tasks)

	return tasks, err
}

// FindOneIdOldOrNew returns a single task with the given ID and execution,
// first looking in the old tasks collection, then the tasks collection.
func FindOneIdOldOrNew(ctx context.Context, id string, execution int) (*Task, error) {
	task, err := FindOneOldId(ctx, MakeOldID(id, execution))
	if task == nil || err != nil {
		return FindOneId(ctx, id)
	}

	return task, err
}

func MakeOldID(taskID string, execution int) string {
	return fmt.Sprintf("%s_%d", taskID, execution)
}

func FindAllFirstExecution(ctx context.Context, query db.Q) ([]Task, error) {
	existingTasks, err := FindAll(ctx, query)
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
		oldTasks, err := FindAllOld(ctx, db.Query(ByIds(oldIDs)))
		if err != nil {
			return nil, errors.Wrap(err, "getting old tasks")
		}
		tasks = append(tasks, oldTasks...)
	}

	return tasks, nil
}

// Find returns all tasks that satisfy the query it also filters out display tasks from the results.
func Find(ctx context.Context, filter bson.M) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter)
	err := db.FindAllQ(ctx, Collection, query, &tasks)

	return tasks, err
}

func FindWithFields(ctx context.Context, filter bson.M, fields ...string) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter).WithFields(fields...)
	err := db.FindAllQ(ctx, Collection, query, &tasks)

	return tasks, err
}

func FindWithSort(ctx context.Context, filter bson.M, sort []string) ([]Task, error) {
	tasks := []Task{}
	_, exists := filter[DisplayOnlyKey]
	if !exists {
		filter[DisplayOnlyKey] = bson.M{"$ne": true}
	}
	query := db.Query(filter).Sort(sort)
	err := db.FindAllQ(ctx, Collection, query, &tasks)

	return tasks, err
}

// Find returns really all tasks that satisfy the query.
func FindAll(ctx context.Context, query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(ctx, Collection, query, &tasks)
	return tasks, err
}

// Find returns really all tasks that satisfy the query.
func FindAllOld(ctx context.Context, query db.Q) ([]Task, error) {
	tasks := []Task{}
	err := db.FindAllQ(ctx, OldCollection, query, &tasks)
	return tasks, err
}

// UpdateOne updates one task.
func UpdateOne(ctx context.Context, query any, update any) error {
	return db.Update(
		ctx,
		Collection,
		query,
		update,
	)
}

// updateOneOld updates one old task.
func updateOneOld(ctx context.Context, query any, update any) error {
	return db.Update(
		ctx,
		OldCollection,
		query,
		update,
	)
}

// updateOneByIdAndExecution attempts to update a task by ID and execution in both
// current and archived collections.
func updateOneByIdAndExecution(ctx context.Context, taskId string, execution int, update any) error {
	// First, try to update in the current tasks collection
	err := UpdateOne(ctx, ByIdAndExecution(taskId, execution), update)

	// If the task was not found in the current collection, it might be archived
	if adb.ResultsNotFound(err) {
		oldTaskId := MakeOldID(taskId, execution)
		err = updateOneOld(ctx, bson.M{IdKey: oldTaskId}, update)
		if adb.ResultsNotFound(err) {
			return errors.Errorf("task '%s' execution %d not found", taskId, execution)
		}
	}

	return err
}

func UpdateAll(ctx context.Context, query any, update any) (*adb.ChangeInfo, error) {
	return db.UpdateAll(
		ctx,
		Collection,
		query,
		update,
	)
}

func UpdateAllWithHint(ctx context.Context, query any, update any, hint any) (*adb.ChangeInfo, error) {
	res, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx, query, update, options.Update().SetHint(hint))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &adb.ChangeInfo{Updated: int(res.ModifiedCount)}, nil
}

// Remove deletes the task of the given id from the database
func Remove(ctx context.Context, id string) error {
	return db.Remove(
		ctx,
		Collection,
		bson.M{IdKey: id},
	)
}

func Aggregate(ctx context.Context, pipeline []bson.M, results any) error {
	return db.Aggregate(ctx,
		Collection,
		pipeline,
		results)
}

// Count returns the number of tasks that satisfy the given query.
func Count(ctx context.Context, query db.Q) (int, error) {
	return db.CountQ(ctx, Collection, query)
}

func FindProjectForTask(ctx context.Context, taskID string) (string, error) {
	query := db.Query(ById(taskID)).WithFields(ProjectKey)
	t, err := FindOne(ctx, query)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", errors.New("task not found")
	}
	return t.Project, nil
}

func FindActivatedByVersionWithoutDisplay(ctx context.Context, versionId string) ([]Task, error) {
	query := db.Query(bson.M{
		VersionKey:     versionId,
		ActivatedKey:   true,
		DisplayOnlyKey: bson.M{"$ne": true},
	})
	activatedTasks, err := FindAll(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "getting previous patch tasks")
	}

	return activatedTasks, nil

}

// updateAllTasksForAllMatchingDependencies updates all tasks (taskIDs) that
// have matching dependencies (dependencyIDs) to mark them as attainable or not.
// Secondly, for a given task, after updating individual dependency
// attainability, it updates the cache of whether the task has all attainable
// dependencies or not.
func updateAllTasksForAllMatchingDependencies(ctx context.Context, taskIDs []string, dependencyIDs []string, unattainable bool) error {
	ctx, span := tracer.Start(ctx, "update-task-dependencies", trace.WithAttributes(
		// Because some projects have huge dependency trees and task IDs are
		// long strings, this update has the potential to make an extremely
		// large query that exceeds the 16 MB aggregation size limit. Track the
		// number of tasks/dependencies that it's trying to update to diagnose
		// such problems.
		attribute.Int("num_tasks", len(taskIDs)),
		attribute.Int("num_dependencies", len(dependencyIDs)),
		attribute.Bool("unattainable", unattainable),
	))
	defer span.End()

	// Update the matching dependencies in the DependsOn array and the UnattainableDependency field that caches
	// whether any of the dependencies are blocked. Combining both these updates in a single update operation makes it
	// impervious to races because updates to single documents are atomic.
	if _, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx,
		bson.M{
			IdKey: bson.M{"$in": taskIDs},
		},
		[]bson.M{
			{
				// Iterate over the DependsOn array and set unattainable for
				// dependencies matching by IDs. Leave other dependencies
				// untouched.
				"$set": bson.M{DependsOnKey: bson.M{
					"$map": bson.M{
						"input": "$" + DependsOnKey,
						"as":    "dependency",
						"in": bson.M{
							"$cond": bson.M{
								"if":   bson.M{"$in": bson.A{bsonutil.GetDottedKeyName("$$dependency", DependencyTaskIdKey), dependencyIDs}},
								"then": bson.M{"$mergeObjects": bson.A{"$$dependency", bson.M{DependencyUnattainableKey: unattainable}}},
								"else": "$$dependency",
							},
						},
					}},
				},
			},
			{
				// Cache whether any dependencies are unattainable.
				"$set": bson.M{UnattainableDependencyKey: bson.M{"$cond": bson.M{
					"if":   bson.M{"$isArray": "$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyUnattainableKey)},
					"then": bson.M{"$anyElementTrue": "$" + bsonutil.GetDottedKeyName(DependsOnKey, DependencyUnattainableKey)},
					"else": false,
				}}},
			},
			addDisplayStatusCache,
		},
	); err != nil {
		return errors.Wrap(err, "updating matching dependencies")
	}

	return nil
}

func AddHostCreateDetails(ctx context.Context, taskId, hostId string, execution int, hostCreateError error) error {
	if hostCreateError == nil {
		return nil
	}
	err := UpdateOne(
		ctx,
		ByIdAndExecution(taskId, execution),
		bson.M{"$push": bson.M{
			HostCreateDetailsKey: HostCreateDetail{HostId: hostId, Error: hostCreateError.Error()},
		}})
	return errors.Wrap(err, "adding details of host creation failure to task")
}

// FindActivatedStepbackTaskByName queries for running/scheduled stepback tasks with
// matching build variant and task name.
func FindActivatedStepbackTaskByName(ctx context.Context, projectId string, variantName string, taskName string) (*Task, error) {
	t, err := FindOne(ctx, db.Query(bson.M{
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
								"$in": []any{"$" + key, []string{evergreen.TaskFailed, evergreen.TaskTestTimedOut, evergreen.TaskTimedOut}},
							},
							"then": 1, // red
						},
						{
							"case": bson.M{
								"$in": []any{"$" + key, []string{evergreen.TaskKnownIssue}},
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
								"$in": []any{"$" + key, evergreen.TaskSystemFailureStatuses},
							},
							"then": 4, // purple
						},
						{
							"case": bson.M{
								"$in": []any{"$" + key, []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
							},
							"then": 5, // yellow
						},
						{
							"case": bson.M{
								"$eq": []string{"$" + key, evergreen.TaskSucceeded},
							},
							"then": 10, // green
						},
						{
							"case": bson.M{
								"$in": []any{"$" + key, []string{evergreen.TaskUnscheduled, evergreen.TaskInactive, evergreen.TaskStatusBlocked, evergreen.TaskAborted}},
							},
							"then": 11, // light grey
						},
					},
					"default": 6, // dark grey
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
					"then": bson.M{"$multiply": []any{time.Millisecond, bson.M{"$subtract": []any{"$$NOW", "$" + StartTimeKey}}}},
					"else": "$" + TimeTakenKey,
				},
			},
		},
	}
}

// GetTasksByVersion gets all tasks for a specific version
// Query results can be filtered by task name, variant name and status in addition to being paginated and limited
func GetTasksByVersion(ctx context.Context, versionID string, opts GetTasksByVersionOptions) ([]Task, int, error) {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetTasksByVersion")})

	pipeline, err := getTasksByVersionPipeline(versionID, opts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting tasks by version pipeline")
	}
	if len(opts.Sorts) > 0 {
		sortPipeline := []bson.M{}

		sortFields := bson.D{}
		var hasIDSort bool
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
			if singleSort.Key == IdKey {
				hasIDSort = true
			}
		}
		// If we're not sorting by ID, we need to sort by ID as a tiebreaker to ensure a consistent sort order.
		if !hasIDSort {
			sortFields = append(sortFields, bson.E{Key: IdKey, Value: 1})
		}

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
	// This avoids hitting the 16 MB limit on the aggregation pipeline in the $facet stage (EVG-15334)
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

// GetTaskStatusesByVersion gets all unique task display statuses for a specific version
func GetTaskStatusesByVersion(ctx context.Context, versionID string) ([]string, error) {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetTaskStatusesByVersion")})

	opts := GetTasksByVersionOptions{
		FieldsToProject:            []string{DisplayStatusKey},
		IncludeNeverActivatedTasks: true,
		IncludeExecutionTasks:      false,
	}
	pipeline, err := getTasksByVersionPipeline(versionID, opts)

	if err != nil {
		return nil, errors.Wrap(err, "getting tasks by version pipeline")
	}

	pipeline = append(pipeline, bson.M{
		"$group": bson.M{
			"_id": nil,
			"statuses": bson.M{
				"$addToSet": "$" + DisplayStatusKey,
			},
		},
	})
	pipeline = append(pipeline, bson.M{
		"$project": bson.M{
			"_id": 0,
			"statuses": bson.M{
				"$sortArray": bson.M{
					"input":  "$statuses",
					"sortBy": 1,
				},
			},
		},
	})

	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	var results []struct {
		Statuses []string `bson:"statuses"`
	}
	err = cursor.All(ctx, &results)

	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errors.Errorf("task statuses for version '%s' not found", versionID)
	}
	return results[0].Statuses, nil

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

func GetTaskStatsByVersion(ctx context.Context, versionID string, opts GetTasksByVersionOptions) (*TaskStats, error) {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetTaskStatsByVersion")})

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
					"$add": []any{
						bson.M{"$divide": []any{"$" + ExpectedDurationKey, time.Millisecond}},
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
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating task stats for version")
	}
	if err := cursor.All(ctx, &taskStats); err != nil {
		return nil, errors.Wrap(err, "aggregating task stats for version")
	}
	result := TaskStats{}
	result.Counts = taskStats[0].Counts
	if len(taskStats[0].ETA) > 0 {
		result.ETA = &taskStats[0].ETA[0].MaxETA
	}

	return &result, nil
}

func GetGroupedTaskStatsByVersion(ctx context.Context, versionID string, opts GetTasksByVersionOptions) ([]*GroupedTaskStatusCount, error) {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "GetGroupedTaskStatsByVersion")})

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

	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating task stats")
	}
	err = cursor.All(ctx, &result)
	if err != nil {
		return nil, err
	}
	return result, nil

}

// GetBaseStatusesForActivatedTasks returns the base statuses for activated tasks on a version.
func GetBaseStatusesForActivatedTasks(ctx context.Context, versionID string, baseVersionID string) ([]string, error) {
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
	// Group to Get rid of duplicate statuses
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
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &res)
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
	UseSlowAnnotationsLookup   bool
}

// HasMatchingTasks returns true if the version has tasks with the given statuses
func HasMatchingTasks(ctx context.Context, versionID string, opts HasMatchingTasksOptions) (bool, error) {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "HasMatchingTasks")})
	options := GetTasksByVersionOptions{
		TaskNames:                  opts.TaskNames,
		Variants:                   opts.Variants,
		Statuses:                   opts.Statuses,
		IncludeNeverActivatedTasks: !opts.IncludeNeverActivatedTasks,
	}
	pipeline, err := getTasksByVersionPipeline(versionID, options)
	if err != nil {
		return false, errors.Wrap(err, "getting tasks by version pipeline")
	}
	pipeline = append(pipeline, bson.M{"$limit": 1})
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		// If the pipeline stage is too large we should use the slow annotations lookup
		if db.IsErrorCode(err, db.FacetPipelineStageTooLargeCode) && !opts.UseSlowAnnotationsLookup {
			opts.UseSlowAnnotationsLookup = true
			return HasMatchingTasks(ctx, versionID, opts)
		}
		return false, err
	}

	type hasMatchingTasksResult struct {
		Task Task `bson:"task"`
	}
	result := []*hasMatchingTasksResult{}
	err = cursor.All(ctx, &result)

	if err != nil {
		return false, err
	}

	return len(result) > 0, nil
}

type GetTasksByVersionOptions struct {
	Statuses                   []string
	BaseStatuses               []string
	Variants                   []string
	TaskNames                  []string
	Page                       int
	Limit                      int
	FieldsToProject            []string
	Sorts                      []TasksSortOrder
	IncludeExecutionTasks      bool
	IncludeNeverActivatedTasks bool // NeverActivated tasks are tasks that lack an activation time
	BaseVersionID              string
}

func getTasksByVersionPipeline(versionID string, opts GetTasksByVersionOptions) ([]bson.M, error) {
	shouldPopulateBaseTask := opts.BaseVersionID != ""
	match := bson.M{}

	if shouldPopulateBaseTask {
		match[VersionKey] = bson.M{"$in": []string{versionID, opts.BaseVersionID}}
	} else {
		match[VersionKey] = versionID
	}

	// GeneratedJSON can often be large, so we filter it out by default
	projectOut := bson.M{
		GeneratedJSONAsStringKey: 0,
	}

	// Filter on task name if it exists
	nonEmptyTaskNames := utility.FilterSlice(opts.TaskNames, func(s string) bool { return s != "" })
	if len(nonEmptyTaskNames) > 0 {
		taskNamesAsRegex := strings.Join(nonEmptyTaskNames, "|")
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

	if !opts.IncludeExecutionTasks {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				"$or": []bson.M{
					{DisplayTaskIdKey: ""},
					{DisplayOnlyKey: true},
				},
			},
		})
	}
	// Filter on Build Variants matching on display name or variant name if it exists
	nonEmptyVariants := utility.FilterSlice(opts.Variants, func(s string) bool { return s != "" })
	if len(nonEmptyVariants) > 0 {
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

	// Add a field for the display status of each task
	pipeline = append(pipeline,
		addDisplayStatus,
	)

	if shouldPopulateBaseTask {
		// First group by variant and task name to group all tasks and their base tasks together
		pipeline = append(pipeline, bson.M{
			"$group": bson.M{
				"_id": bson.M{
					BuildVariantKey: "$" + BuildVariantKey,
					DisplayNameKey:  "$" + DisplayNameKey,
				},
				"tasks": bson.M{
					"$push": "$$ROOT",
				},
			},
		})
		// Separate the root task and base task into separate arrays
		pipeline = append(pipeline, bson.M{
			"$addFields": bson.M{
				"root_task": bson.M{
					"$filter": bson.M{
						"input": "$tasks",
						"as":    "task",
						"cond": bson.M{
							"$eq": []string{"$$task." + VersionKey, versionID},
						},
					},
				},
				"base_task": bson.M{
					"$filter": bson.M{
						"input": "$tasks",
						"as":    "task",
						"cond": bson.M{
							"$eq": []string{"$$task." + VersionKey, opts.BaseVersionID},
						},
					},
				},
			},
		})

		// Project out the the tasks array since it is no longer needed
		pipeline = append(pipeline, bson.M{
			"$project": bson.M{
				"tasks": 0,
			},
		})

		// Unwind the root task and base task arrays so that each document is a single task
		pipeline = append(pipeline, bson.M{
			"$addFields": bson.M{
				"root_task": bson.M{
					"$first": "$root_task",
				},
				"base_task": bson.M{
					"$first": "$base_task",
				},
			},
		})

		// Ensure that the root task is not nil if it is the task does not exist in the current version
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				"root_task": bson.M{
					"$exists": true,
				},
			},
		})

		// Include the base task in the root task
		pipeline = append(pipeline, bson.M{
			"$addFields": bson.M{
				"root_task": bson.M{
					"base_task": "$base_task",
				},
			},
		})

		// Use display status from the base task as its status
		pipeline = append(pipeline, bson.M{
			"$addFields": bson.M{
				bsonutil.GetDottedKeyName("root_task", BaseTaskStatusKey): "$" + bsonutil.GetDottedKeyName("root_task", BaseTaskKey, DisplayStatusKey),
			},
		})

		// Replace the root document with the task
		pipeline = append(pipeline, bson.M{
			"$replaceRoot": bson.M{
				"newRoot": "$root_task",
			},
		})

		// Sort the tasks by their _id to ensure that they are in the same order as the original tasks
		pipeline = append(pipeline, bson.M{
			"$sort": bson.M{
				"_id": 1,
			},
		})
	}

	if len(opts.Statuses) > 0 {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				DisplayStatusKey: bson.M{"$in": opts.Statuses},
			},
		})
	}

	if shouldPopulateBaseTask && len(opts.BaseStatuses) > 0 {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{
				BaseTaskStatusKey: bson.M{"$in": opts.BaseStatuses},
			},
		})
	}

	return pipeline, nil
}

// FindAllDependencyTasksToModify finds tasks that depend on the
// given tasks. The isBlocking parameter indicates whether we are fetching
// tasks to unblock them, and if so, for each task, all dependencies
// that have been marked unattainable will be retrieved. Otherwise, we are
// fetching tasks to block them, and for each task, all dependencies
// that have a status that that would block the task from running (i.e., it is
// inconsistent with the task's Dependency.Status field) and have not been marked
// unattainable will be retrieved. The ignoreDependencyStatusForBlocking parameter
// indicates whether all tasks that depend on the given need to be updated for a
// blocking operation (for example, if a single host task group host terminates
// before completing the group and we need to block all later tasks regardless
// of their Dependency.Status field).
//
// This must find tasks in smaller chunks to avoid the 16 MB query size limit -
// if the number of tasks is large, a single query could be too large and the DB
// will reject it.
func FindAllDependencyTasksToModify(ctx context.Context, tasks []Task, isBlocking, ignoreDependencyStatusForBlocking bool) ([]Task, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	const maxTasksPerQuery = 2000
	var unmatchedDep []bson.M
	allTasks := make([]Task, 0, len(tasks))

	for i, t := range tasks {
		elemMatchQuery := bson.M{
			DependencyTaskIdKey:       t.Id,
			DependencyUnattainableKey: !isBlocking,
		}
		// If the operation is unblocking, then we ignore the status the dependencies were waiting on,
		// and unblock all of them. Similarly, if the operation is blocking and ignoreDependencyStatusForBlocking
		// is set, we also want to ignore the status the dependencies were waiting on.
		if isBlocking && !ignoreDependencyStatusForBlocking {
			okStatusSet := []string{AllStatuses, t.Status}
			elemMatchQuery[DependencyStatusKey] = bson.M{"$nin": okStatusSet}
		}
		unmatchedDep = append(unmatchedDep, bson.M{
			DependsOnKey: bson.M{"$elemMatch": elemMatchQuery},
		})

		if i == len(tasks)-1 || len(unmatchedDep) >= maxTasksPerQuery {
			// Query the tasks now - either there's no remaining task
			// dependencies to check, or the max task dependencies that can be
			// checked per query has been reached.
			query := db.Query(bson.M{
				"$or": unmatchedDep,
			})
			tasks, err := FindAll(ctx, query)
			if err != nil {
				return nil, err
			}
			allTasks = append(allTasks, tasks...)
			unmatchedDep = nil
		}
	}

	return allTasks, nil
}

func activateTasks(ctx context.Context, taskIDs []string, caller string, activationTime time.Time) error {
	tasks, err := FindAll(ctx, db.Query(bson.M{
		IdKey:        bson.M{"$in": taskIDs},
		ActivatedKey: false,
	}))
	if err != nil {
		return errors.Wrap(err, "fetching tasks for activation")
	}

	// Separate tasks that need predictions from those that already have them
	var tasksNeedingPredictions []Task
	var taskIDsWithPredictions []string
	for _, t := range tasks {
		if t.PredictedTaskCost.IsZero() {
			tasksNeedingPredictions = append(tasksNeedingPredictions, t)
		} else {
			taskIDsWithPredictions = append(taskIDsWithPredictions, t.Id)
		}
	}

	// Activate tasks that already have predictions (no need to compute or set predictions)
	if len(taskIDsWithPredictions) > 0 {
		_, err := UpdateAll(
			ctx,
			bson.M{
				IdKey:        bson.M{"$in": taskIDsWithPredictions},
				ActivatedKey: false,
			},
			[]bson.M{
				{
					"$set": bson.M{
						ActivatedKey:     true,
						ActivatedByKey:   caller,
						ActivatedTimeKey: activationTime,
					},
				},
				addDisplayStatusCache,
			})
		if err != nil {
			return errors.Wrap(err, "activating tasks with existing predictions")
		}
	}

	// Compute and update predictions for tasks that need them
	if len(tasksNeedingPredictions) > 0 {
		predictions, err := computeCostPredictionsInParallel(ctx, tasksNeedingPredictions)
		if err != nil {
			return errors.Wrap(err, "computing cost predictions")
		}

		env := evergreen.GetEnvironment()
		coll := env.DB().Collection(Collection)
		var writes []mongo.WriteModel

		for _, t := range tasksNeedingPredictions {
			prediction := predictions[t.Id]
			setFields := bson.M{
				ActivatedKey:     true,
				ActivatedByKey:   caller,
				ActivatedTimeKey: activationTime,
			}

			addPredictedCostToUpdate(setFields, prediction.PredictedCost)

			writes = append(writes, mongo.NewUpdateOneModel().
				SetFilter(bson.M{IdKey: t.Id, ActivatedKey: false}).
				SetUpdate([]bson.M{
					{"$set": setFields},
					addDisplayStatusCache,
				}))
		}

		if len(writes) > 0 {
			_, err := coll.BulkWrite(ctx, writes)
			if err != nil {
				return errors.Wrap(err, "bulk updating tasks with new predictions")
			}
		}
	}

	if err = enableDisabledTasks(ctx, taskIDs); err != nil {
		return errors.Wrap(err, "enabling disabled tasks")
	}
	return nil
}

func enableDisabledTasks(ctx context.Context, taskIDs []string) error {
	_, err := UpdateAll(
		ctx,
		bson.M{
			IdKey:       bson.M{"$in": taskIDs},
			PriorityKey: evergreen.DisabledTaskPriority,
		},
		bson.M{
			"$set": bson.M{
				PriorityKey: 0,
			},
		})
	return err
}

// ComputePredictedCostsForTasks computes predicted costs for activated tasks
func ComputePredictedCostsForTasks(ctx context.Context, tasks Tasks) (map[string]cost.Cost, error) {
	if len(tasks) == 0 {
		return map[string]cost.Cost{}, nil
	}

	activatedTasks := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if t.Activated {
			activatedTasks = append(activatedTasks, *t)
		}
	}

	if len(activatedTasks) == 0 {
		return map[string]cost.Cost{}, nil
	}

	// Use background context to avoid MongoDB session races in parallel queries.
	// The input ctx may have a transaction session which is not thread-safe.
	bgCtx := context.Background()
	predictions, err := computeCostPredictionsInParallel(bgCtx, activatedTasks)
	if err != nil {
		return nil, errors.Wrap(err, "computing cost predictions")
	}

	result := make(map[string]cost.Cost, len(predictions))
	for taskID, prediction := range predictions {
		result[taskID] = prediction.PredictedCost
	}

	return result, nil
}

// taskVariantKey represents a unique combination of project, variant, and task name for batching cost predictions
type taskVariantKey struct {
	project      string
	buildVariant string
	displayName  string
}

// computeCostPredictionsInParallel computes cost predictions for multiple tasks in parallel.
// It groups tasks by (project, variant, name) to batch queries and runs them concurrently
// using a worker pool to limit database load.
// Returns a map from task ID to cost prediction result.
func computeCostPredictionsInParallel(ctx context.Context, tasks []Task) (map[string]CostPredictionResult, error) {
	if len(tasks) == 0 {
		return map[string]CostPredictionResult{}, nil
	}

	tasksByVariant := make(map[taskVariantKey][]Task)
	for _, t := range tasks {
		key := taskVariantKey{
			project:      t.Project,
			buildVariant: t.BuildVariant,
			displayName:  t.DisplayName,
		}
		tasksByVariant[key] = append(tasksByVariant[key], t)
	}

	type workItem struct {
		key   taskVariantKey
		tasks []Task
	}

	type predictionResult struct {
		taskID     string
		prediction CostPredictionResult
		err        error
	}

	workQueue := make(chan workItem, len(tasksByVariant))
	for key, variantTasks := range tasksByVariant {
		workQueue <- workItem{key: key, tasks: variantTasks}
	}
	close(workQueue)

	// Limit concurrent database queries to avoid overwhelming the connection pool.
	// Each query runs an aggregation pipeline with grouping/statistics which is resource-intensive.
	const maxWorkers = 20
	numWorkers := util.Min(maxWorkers, len(tasksByVariant))
	resultChan := make(chan predictionResult, len(tasks))

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for work := range workQueue {
				// Compute prediction once for this variant group (use first task as representative)
				prediction, err := work.tasks[0].ComputePredictedCostForWeek(ctx)

				// Send result for each task in group
				for _, t := range work.tasks {
					resultChan <- predictionResult{
						taskID:     t.Id,
						prediction: prediction,
						err:        err,
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	predictions := make(map[string]CostPredictionResult)
	for result := range resultChan {
		if result.err != nil {
			grip.Warning(message.WrapError(result.err, message.Fields{
				"message": "error computing cost prediction for task, using zero prediction",
				"task_id": result.taskID,
			}))
			predictions[result.taskID] = CostPredictionResult{}
		} else {
			predictions[result.taskID] = result.prediction
		}
	}

	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context cancelled while computing cost predictions")
	}

	return predictions, nil
}

// IncNumNextTaskDispatches sets the number of times a host has requested this
// task and execution as its next task.
func (t *Task) IncNumNextTaskDispatches(ctx context.Context) error {
	if err := UpdateOne(
		ctx,
		ByIdAndExecution(t.Id, t.Execution),
		bson.M{
			"$inc": bson.M{NumNextTaskDispatchesKey: 1},
		}); err != nil {
		return errors.Wrapf(err, "setting next task count for task '%s'", t.Id)
	}
	t.NumNextTaskDispatches = t.NumNextTaskDispatches + 1
	return nil
}

// UpdateHasAnnotations updates a task's HasAnnotations flag, indicating if there
// are any annotations with populated IssuesKey for its id / execution pair.
// This function handles both current tasks (in 'tasks' collection) and archived
// tasks (in 'old_tasks' collection).
func UpdateHasAnnotations(ctx context.Context, taskId string, execution int, hasAnnotations bool) error {
	update := []bson.M{
		{"$set": bson.M{
			HasAnnotationsKey: hasAnnotations,
		}},
		addDisplayStatusCache,
	}

	err := updateOneByIdAndExecution(ctx, taskId, execution, update)
	return errors.Wrapf(err, "updating HasAnnotations field for task '%s' execution %d", taskId, execution)
}

type NumExecutionsForIntervalInput struct {
	ProjectId    string
	BuildVarName string
	TaskName     string
	Requesters   []string
	StartTime    time.Time
	EndTime      time.Time
}

func CountNumExecutionsForInterval(ctx context.Context, input NumExecutionsForIntervalInput) (int, error) {
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
	numTasks, err := db.Count(ctx, Collection, query)
	if err != nil {
		return 0, errors.Wrap(err, "counting task executions")
	}
	numOldTasks, err := db.Count(ctx, OldCollection, query)
	if err != nil {
		return 0, errors.Wrap(err, "counting old task executions")
	}
	return numTasks + numOldTasks, nil
}

// AbortAndMarkResetTasksForBuild aborts and marks in-progress tasks to reset
// from the specified task IDs and build ID. If no task IDs are specified, all
// in-progress tasks belonging to the build are aborted and marked to reset.
func AbortAndMarkResetTasksForBuild(ctx context.Context, buildID string, taskIDs []string, caller string) error {
	return abortAndMarkResetTasks(ctx, ByBuildId(buildID), taskIDs, caller)
}

// AbortAndMarkResetTasksForVersion aborts and marks in-progress tasks to reset
// from the specified task IDs and version ID. If no task IDs are specified,
// all in-progress tasks belonging to the version are aborted and marked to
// reset.
func AbortAndMarkResetTasksForVersion(ctx context.Context, versionID string, taskIDs []string, caller string) error {
	return abortAndMarkResetTasks(ctx, ByVersion(versionID), taskIDs, caller)
}

func abortAndMarkResetTasks(ctx context.Context, filter bson.M, taskIDs []string, caller string) error {
	filter[StatusKey] = bson.M{"$in": evergreen.TaskInProgressStatuses}
	if len(taskIDs) > 0 {
		filter["$or"] = []bson.M{
			{IdKey: bson.M{"$in": taskIDs}},
			{DisplayTaskIdKey: bson.M{"$in": taskIDs}},
			{ExecutionTasksKey: bson.M{"$in": taskIDs}},
		}
	}

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(
		ctx,
		filter,
		[]bson.M{
			{
				"$set": bson.M{
					AbortedKey:           true,
					AbortInfoKey:         AbortInfo{User: caller},
					ResetWhenFinishedKey: true,
				},
			},
			{
				"$unset": []string{
					ResetFailedWhenFinishedKey,
				},
			},
			addDisplayStatusCache,
		},
	)

	return err
}

// FindCompletedTasksByBuild returns all completed tasks belonging to the
// given build ID. Excludes execution tasks. If no taskIDs are specified, all
// completed tasks belonging to the build are returned.
func FindCompletedTasksByBuild(ctx context.Context, buildID string, taskIDs []string) ([]Task, error) {
	return findCompletedTasks(ctx, ByBuildId(buildID), taskIDs)
}

// FindCompletedTasksByVersion returns all completed tasks belonging to the
// given version ID. Excludes execution tasks. If no task IDs are specified,
// all completed tasks belonging to the version are returned.
func FindCompletedTasksByVersion(ctx context.Context, versionID string, taskIDs []string) ([]Task, error) {
	return findCompletedTasks(ctx, ByVersion(versionID), taskIDs)
}

func findCompletedTasks(ctx context.Context, filter bson.M, taskIDs []string) ([]Task, error) {
	filter[StatusKey] = bson.M{"$in": evergreen.TaskCompletedStatuses}
	filter[DisplayTaskIdKey] = ""
	if len(taskIDs) > 0 {
		filter["$or"] = []bson.M{
			{IdKey: bson.M{"$in": taskIDs}},
			{ExecutionTasksKey: bson.M{"$in": taskIDs}},
		}
	}

	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Find(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "finding completed tasks")
	}

	var out []Task
	if err = cur.All(ctx, &out); err != nil {
		return nil, errors.Wrap(err, "decoding task documents")
	}

	return out, nil
}

// GeneratedTaskInfo contains basic information about a generated task.
type GeneratedTaskInfo struct {
	TaskID                  string
	TaskName                string
	BuildID                 string
	BuildVariant            string
	BuildVariantDisplayName string
}

// FindGeneratedTasksFromID finds all tasks that were generated by the given
// generator task and returns the filtered information.
func FindGeneratedTasksFromID(ctx context.Context, generatorID string) ([]GeneratedTaskInfo, error) {
	tasks, err := FindAll(ctx, db.Query(bson.M{GeneratedByKey: generatorID}).Project(bson.M{
		IdKey:                      1,
		BuildIdKey:                 1,
		DisplayNameKey:             1,
		BuildVariantKey:            1,
		BuildVariantDisplayNameKey: 1,
	}))
	if err != nil {
		return nil, err
	}

	var out []GeneratedTaskInfo
	for _, t := range tasks {
		out = append(out, GeneratedTaskInfo{
			TaskID:                  t.Id,
			TaskName:                t.DisplayName,
			BuildID:                 t.BuildId,
			BuildVariant:            t.BuildVariant,
			BuildVariantDisplayName: t.BuildVariantDisplayName,
		})
	}

	return out, nil
}

type generateTasksEstimationsResults struct {
	EstimatedCreated   float64 `bson:"est_created"`
	EstimatedActivated float64 `bson:"est_activated"`
}

func getGenerateTasksEstimation(ctx context.Context, project, buildVariant, displayName string, lookBackTime time.Duration) ([]generateTasksEstimationsResults, error) {
	match := bson.M{
		ProjectKey:        project,
		BuildVariantKey:   buildVariant,
		DisplayNameKey:    displayName,
		GeneratedTasksKey: true,
		StatusKey:         evergreen.TaskSucceeded,
		StartTimeKey: bson.M{
			"$gt": time.Now().Add(-1 * lookBackTime),
		},
		FinishTimeKey: bson.M{
			"$lte": time.Now(),
		},
	}

	pipeline := []bson.M{
		{
			"$match": match,
		},
		{
			"$project": bson.M{
				DisplayNameKey:                1,
				NumGeneratedTasksKey:          1,
				NumActivatedGeneratedTasksKey: 1,
				IdKey:                         0,
			},
		},
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%s", DisplayNameKey),
				"est_created": bson.M{
					"$avg": fmt.Sprintf("$%s", NumGeneratedTasksKey),
				},
				"est_activated": bson.M{
					"$avg": fmt.Sprintf("$%s", NumActivatedGeneratedTasksKey),
				},
			},
		},
	}

	results := []generateTasksEstimationsResults{}

	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	dbCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cursor, err := coll.Aggregate(dbCtx, pipeline, options.Aggregate().SetHint(TaskHistoricalDataIndex))
	if err != nil {
		return nil, errors.Wrap(err, "aggregating generate tasks estimations")
	}
	err = cursor.All(dbCtx, &results)
	if err != nil {
		return nil, errors.Wrap(err, "iterating and decoding generate tasks estimations")
	}

	return results, nil
}

type pendingGenerateTasksResults struct {
	NumPendingGenerateTasks int `bson:"pending_generate_tasks"`
}

// GetPendingGenerateTasks returns an estimated number of tasks the current dispatched tasks will generate.
func GetPendingGenerateTasks(ctx context.Context) (int, error) {
	match := bson.M{
		GeneratedTasksKey: bson.M{
			"$ne": true,
		},
		StatusKey: bson.M{
			"$in": evergreen.TaskInProgressStatuses,
		},
	}

	pipeline := []bson.M{
		{
			"$match": match,
		},
		{
			"$group": bson.M{
				"_id": nil,
				"pending_generate_tasks": bson.M{
					"$sum": fmt.Sprintf("$%s", EstimatedNumGeneratedTasksKey),
				},
			},
		},
	}

	results := []pendingGenerateTasksResults{}

	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	dbCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cursor, err := coll.Aggregate(dbCtx, pipeline)
	if err != nil {
		return 0, errors.Wrap(err, "aggregating pending generate tasks")
	}
	if err = cursor.All(dbCtx, &results); err != nil {
		return 0, errors.Wrap(err, "iterating and decoding pending generate tasks")
	}
	if len(results) == 0 {
		return 0, nil
	} else if len(results) != 1 {
		return 0, errors.New("expected exactly one result from pending generate tasks aggregation")
	} else {
		return results[0].NumPendingGenerateTasks, nil
	}
}

// CountLargeParserProjectTasks counts the number of tasks running with parser projects stored in s3.
func CountLargeParserProjectTasks(ctx context.Context) (int, error) {
	return Count(ctx, db.Query(bson.M{
		StatusKey: bson.M{
			"$in": evergreen.TaskInProgressStatuses,
		},
		CachedProjectStorageMethodKey: evergreen.ProjectStorageMethodS3,
	}))
}

// GetLatestTaskFromImage retrieves the latest task from all the distros corresponding to the imageID.
func GetLatestTaskFromImage(ctx context.Context, imageID string) (*Task, error) {
	distros, err := distro.GetDistrosForImage(ctx, imageID)
	if err != nil {
		return nil, errors.Wrap(err, "retrieving distros from imageID")
	}
	distroNames := make([]string, len(distros))
	for i, d := range distros {
		distroNames[i] = d.Id
	}
	if len(distroNames) == 0 {
		return nil, errors.Errorf("no distros found for image '%s'", imageID)
	}
	pipeline := []bson.M{
		{
			"$match": bson.M{
				DistroIdKey: bson.M{
					"$in": distroNames,
				},
			},
		},
		{
			"$sort": bson.M{FinishTimeKey: -1},
		},
		{
			"$limit": 1,
		},
	}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "finding latest task")
	}
	if cursor.Next(ctx) {
		task := Task{}
		if err := cursor.Decode(&task); err != nil {
			return nil, errors.Wrap(err, "decoding task")
		}
		return &task, nil
	}
	return nil, nil
}

type predictedCostResults struct {
	DisplayName        string  `bson:"_id"`
	AvgOnDemandCost    float64 `bson:"avg_on_demand_cost"`
	AvgAdjustedCost    float64 `bson:"avg_adjusted_cost"`
	StdDevOnDemandCost float64 `bson:"std_dev_on_demand_cost"`
	StdDevAdjustedCost float64 `bson:"std_dev_adjusted_cost"`
}

func getPredictedCostsForWindow(ctx context.Context, name, project, buildVariant string, start, end time.Time) ([]predictedCostResults, error) {
	if end.Before(start) {
		return nil, errors.New("end time must be after start time")
	}
	match := bson.M{
		BuildVariantKey: buildVariant,
		ProjectKey:      project,
		StatusKey: bson.M{
			"$in": evergreen.TaskCompletedStatuses,
		},
		bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut): bson.M{
			"$ne": true,
		},
		FinishTimeKey: bson.M{
			"$gte": start,
			"$lte": end,
		},
		bsonutil.GetDottedKeyName(TaskCostKey, "on_demand_ec2_cost"): bson.M{
			"$gt": 0,
		},
	}

	if name != "" {
		match[DisplayNameKey] = name
	}

	pipeline := []bson.M{
		{
			"$match": match,
		},
		{
			"$project": bson.M{
				DisplayNameKey: 1,
				bsonutil.GetDottedKeyName(TaskCostKey, "on_demand_ec2_cost"): 1,
				bsonutil.GetDottedKeyName(TaskCostKey, "adjusted_ec2_cost"):  1,
				IdKey: 0,
			},
		},
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%s", DisplayNameKey),
				"avg_on_demand_cost": bson.M{
					"$avg": fmt.Sprintf("$%s.on_demand_ec2_cost", TaskCostKey),
				},
				"avg_adjusted_cost": bson.M{
					"$avg": fmt.Sprintf("$%s.adjusted_ec2_cost", TaskCostKey),
				},
				"std_dev_on_demand_cost": bson.M{
					"$stdDevPop": fmt.Sprintf("$%s.on_demand_ec2_cost", TaskCostKey),
				},
				"std_dev_adjusted_cost": bson.M{
					"$stdDevPop": fmt.Sprintf("$%s.adjusted_ec2_cost", TaskCostKey),
				},
			},
		},
	}

	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	// Use a fresh context to avoid sharing MongoDB sessions across goroutines.
	dbCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cursor, err := coll.Aggregate(dbCtx, pipeline, options.Aggregate().SetHint(TaskHistoricalDataIndex))
	if err != nil {
		return nil, errors.Wrap(err, "aggregating task average cost")
	}

	var results []predictedCostResults
	if err := cursor.All(dbCtx, &results); err != nil {
		return nil, errors.Wrap(err, "iterating and decoding task average cost")
	}

	return results, nil
}

// CountRunningTasksForVersions returns the number of running or dispatched tasks for the given versions.
func CountRunningTasksForVersions(ctx context.Context, versionIDs []string) (int, error) {
	if len(versionIDs) == 0 {
		return 0, nil
	}

	count, err := Count(ctx, db.Query(bson.M{
		VersionKey: bson.M{"$in": versionIDs},
		StatusKey:  bson.M{"$in": []string{evergreen.TaskStarted, evergreen.TaskDispatched}},
	}))
	if err != nil {
		return 0, errors.Wrap(err, "counting running tasks")
	}

	return count, nil
}

// GetFirstTaskStartTimeForVersion returns the start time of the first task to start for a version.
func GetFirstTaskStartTimeForVersion(ctx context.Context, versionID string) (time.Time, error) {
	filter := bson.M{
		VersionKey: versionID,
		// Exclude tasks that haven't started yet.
		StartTimeKey: bson.M{"$ne": time.Time{}},
	}
	task, err := FindOne(ctx, db.Query(filter).WithFields(StartTimeKey).Sort([]string{StartTimeKey}).Limit(1))
	if err != nil {
		return time.Time{}, errors.Wrap(err, "querying for first started task")
	}
	if task == nil {
		return time.Time{}, nil
	}
	return task.StartTime, nil
}
