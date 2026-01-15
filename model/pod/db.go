package pod

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	Collection = "pods"
)

var (
	IDKey                        = bsonutil.MustHaveTag(Pod{}, "ID")
	TypeKey                      = bsonutil.MustHaveTag(Pod{}, "Type")
	StatusKey                    = bsonutil.MustHaveTag(Pod{}, "Status")
	TaskContainerCreationOptsKey = bsonutil.MustHaveTag(Pod{}, "TaskContainerCreationOpts")
	FamilyKey                    = bsonutil.MustHaveTag(Pod{}, "Family")
	TimeInfoKey                  = bsonutil.MustHaveTag(Pod{}, "TimeInfo")
	ResourcesKey                 = bsonutil.MustHaveTag(Pod{}, "Resources")
	TaskRuntimeInfoKey           = bsonutil.MustHaveTag(Pod{}, "TaskRuntimeInfo")

	TaskContainerCreationOptsImageKey    = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "Image")
	TaskContainerCreationOptsMemoryMBKey = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "MemoryMB")
	TaskContainerCreationOptsCPUKey      = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "CPU")
	TaskContainerCreationOptsOSKey       = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "OS")
	TaskContainerCreationOptsArchKey     = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "Arch")
	TaskContainerCreationOptsEnvVarsKey  = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "EnvVars")
	TaskContainerCreationOptsSecretsKey  = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "EnvSecrets")

	TimeInfoInitializingKey     = bsonutil.MustHaveTag(TimeInfo{}, "Initializing")
	TimeInfoStartingKey         = bsonutil.MustHaveTag(TimeInfo{}, "Starting")
	TimeInfoLastCommunicatedKey = bsonutil.MustHaveTag(TimeInfo{}, "LastCommunicated")
	TimeInfoAgentStartedKey     = bsonutil.MustHaveTag(TimeInfo{}, "AgentStarted")

	ResourceInfoExternalIDKey   = bsonutil.MustHaveTag(ResourceInfo{}, "ExternalID")
	ResourceInfoDefinitionIDKey = bsonutil.MustHaveTag(ResourceInfo{}, "DefinitionID")
	ResourceInfoClusterKey      = bsonutil.MustHaveTag(ResourceInfo{}, "Cluster")
	ResourceInfoContainersKey   = bsonutil.MustHaveTag(ResourceInfo{}, "Containers")

	TaskRuntimeInfoRunningTaskIDKey        = bsonutil.MustHaveTag(TaskRuntimeInfo{}, "RunningTaskID")
	TaskRuntimeInfoRunningTaskExecutionKey = bsonutil.MustHaveTag(TaskRuntimeInfo{}, "RunningTaskExecution")

	ContainerResourceInfoExternalIDKey = bsonutil.MustHaveTag(ContainerResourceInfo{}, "ExternalID")
	ContainerResourceInfoNameKey       = bsonutil.MustHaveTag(ContainerResourceInfo{}, "Name")
	ContainerResourceInfoSecretIDsKey  = bsonutil.MustHaveTag(ContainerResourceInfo{}, "SecretIDs")

	SecretExternalIDKey = bsonutil.MustHaveTag(Secret{}, "ExternalID")
	SecretValueKey      = bsonutil.MustHaveTag(Secret{}, "Value")
)

func ByID(id string) bson.M {
	return bson.M{
		IDKey: id,
	}
}

func ByExternalID(id string) bson.M {
	return bson.M{
		bsonutil.GetDottedKeyName(ResourcesKey, ResourceInfoExternalIDKey): id,
	}
}

// Count counts the number of pods matching the given query.
func Count(ctx context.Context, q db.Q) (int, error) {
	return db.CountQ(ctx, Collection, q)
}

// Find finds all pods matching the given query.
func Find(ctx context.Context, q db.Q) ([]Pod, error) {
	pods := []Pod{}
	return pods, errors.WithStack(db.FindAllQ(ctx, Collection, q, &pods))
}

// FindOne finds one pod by the given query.
func FindOne(ctx context.Context, q db.Q) (*Pod, error) {
	var p Pod
	err := db.FindOneQContext(ctx, Collection, q, &p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &p, err
}

// FindOneByID finds one pod by its ID.
func FindOneByID(ctx context.Context, id string) (*Pod, error) {
	p, err := FindOne(ctx, db.Query(ByID(id)))
	if err != nil {
		return nil, errors.Wrapf(err, "finding pod '%s'", id)
	}
	return p, nil
}

// UpdateOne updates one pod.
func UpdateOne(ctx context.Context, query any, update any) error {
	return db.Update(
		ctx,
		Collection,
		query,
		update,
	)
}

// FindByNeedsTermination finds all pods running agents that need to be
// terminated, which includes:
// * Pods that have been provisioning for too long.
// * Pods that are decommissioned and have no running task.
func FindByNeedsTermination(ctx context.Context) ([]Pod, error) {
	staleCutoff := time.Now().Add(-15 * time.Minute)
	return Find(ctx, db.Query(bson.M{
		"$or": []bson.M{
			{
				StatusKey: StatusInitializing,
				bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoInitializingKey): bson.M{"$lte": staleCutoff},
			},
			{
				StatusKey: StatusStarting,
				bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoStartingKey): bson.M{"$lte": staleCutoff},
			},
			{
				StatusKey: StatusDecommissioned,
				bsonutil.GetDottedKeyName(TaskRuntimeInfoKey, TaskRuntimeInfoRunningTaskIDKey): nil,
			},
		},
	}))
}

// FindByInitializing find all pods that are initializing but have not started
// any containers.
func FindByInitializing(ctx context.Context) ([]Pod, error) {
	return Find(ctx, db.Query(bson.M{
		StatusKey: StatusInitializing,
	}))
}

// CountByInitializing counts the number of pods that are initializing but have
// not started any containers.
func CountByInitializing(ctx context.Context) (int, error) {
	return Count(ctx, db.Query(bson.M{
		StatusKey: StatusInitializing,
	}))
}

// FindOneByExternalID finds a pod that has a matching external identifier.
func FindOneByExternalID(ctx context.Context, id string) (*Pod, error) {
	return FindOne(ctx, db.Query(ByExternalID(id)))
}

// FindIntentByFamily finds intent pods that have a matching family name.
func FindIntentByFamily(ctx context.Context, family string) ([]Pod, error) {
	return Find(ctx, db.Query(bson.M{
		StatusKey: StatusInitializing,
		FamilyKey: family,
	}))
}

// UpdateOneStatus updates a pod's status by ID along with any relevant metadata
// information about the status update. If the current status is identical to
// the updated one, this will no-op. If the current status does not match the
// stored status, this will error.
func UpdateOneStatus(ctx context.Context, id string, current, updated Status, ts time.Time, reason string) error {
	if current == updated {
		return nil
	}

	byIDAndStatus := ByID(id)
	byIDAndStatus[StatusKey] = current

	setFields := bson.M{StatusKey: updated}
	switch updated {
	case StatusInitializing:
		setFields[bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoInitializingKey)] = ts
	case StatusStarting:
		setFields[bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoStartingKey)] = ts
	}

	if err := UpdateOne(ctx, byIDAndStatus, bson.M{
		"$set": setFields,
	}); err != nil {
		return err
	}

	event.LogPodStatusChanged(ctx, id, string(current), string(updated), reason)

	return nil
}

// FindByLastCommunicatedBefore finds all active pods whose last communication
// was before the given threshold.
func FindByLastCommunicatedBefore(ctx context.Context, ts time.Time) ([]Pod, error) {
	lastCommunicatedKey := bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoLastCommunicatedKey)
	return Find(ctx, db.Query(bson.M{
		StatusKey:           bson.M{"$in": []Status{StatusStarting, StatusRunning}},
		lastCommunicatedKey: bson.M{"$lte": ts},
	}))
}

// StatusCount contains the total number of pods and total number of running
// tasks for a particular pod status.
type StatusCount struct {
	Status          Status `bson:"status"`
	Count           int    `bson:"count"`
	NumRunningTasks int    `bson:"num_running_tasks"`
}

// GetStatsByStatus gets aggregate usage statistics on pods that are intended
// for running tasks. For each pod status, it returns the counts for the number
// of pods and number of running tasks in that particular status. Terminated
// pods are excluded from these statistics.
func GetStatsByStatus(ctx context.Context, statuses ...Status) ([]StatusCount, error) {
	if len(statuses) == 0 {
		return []StatusCount{}, nil
	}

	runningTaskKey := bsonutil.GetDottedKeyName(TaskRuntimeInfoKey, TaskRuntimeInfoRunningTaskIDKey)
	pipeline := []bson.M{
		{
			"$match": bson.M{
				StatusKey: bson.M{
					"$in": statuses,
				},
				TypeKey: TypeAgent,
			},
		},
		{
			"$group": bson.M{
				"_id": "$" + StatusKey,
				"count": bson.M{
					"$sum": 1,
				},
				"running_tasks": bson.M{
					"$addToSet": "$" + runningTaskKey,
				},
			},
		},
		{
			"$project": bson.M{
				"_id":               0,
				"status":            "$_id",
				"count":             1,
				"num_running_tasks": bson.M{"$size": "$running_tasks"},
			},
		},
	}

	stats := []StatusCount{}
	if err := db.Aggregate(ctx, Collection, pipeline, &stats); err != nil {
		return nil, err
	}

	return stats, nil
}
