package pod

import (
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
	TimeInfoKey                  = bsonutil.MustHaveTag(Pod{}, "TimeInfo")
	ResourcesKey                 = bsonutil.MustHaveTag(Pod{}, "Resources")
	RunningTaskKey               = bsonutil.MustHaveTag(Pod{}, "RunningTask")

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

	ResourceInfoExternalIDKey   = bsonutil.MustHaveTag(ResourceInfo{}, "ExternalID")
	ResourceInfoDefinitionIDKey = bsonutil.MustHaveTag(ResourceInfo{}, "DefinitionID")
	ResourceInfoClusterKey      = bsonutil.MustHaveTag(ResourceInfo{}, "Cluster")
	ResourceInfoContainersKey   = bsonutil.MustHaveTag(ResourceInfo{}, "Containers")

	ContainerResourceInfoExternalIDKey = bsonutil.MustHaveTag(ContainerResourceInfo{}, "ExternalID")
	ContainerResourceInfoNameKey       = bsonutil.MustHaveTag(ContainerResourceInfo{}, "Name")
	ContainerResourceInfoSecretIDsKey  = bsonutil.MustHaveTag(ContainerResourceInfo{}, "SecretIDs")

	SecretNameKey       = bsonutil.MustHaveTag(Secret{}, "Name")
	SecretExternalIDKey = bsonutil.MustHaveTag(Secret{}, "ExternalID")
	SecretValueKey      = bsonutil.MustHaveTag(Secret{}, "Value")
	SecretExistsKey     = bsonutil.MustHaveTag(Secret{}, "Exists")
	SecretOwnedKey      = bsonutil.MustHaveTag(Secret{}, "Owned")
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
func Count(q db.Q) (int, error) {
	return db.CountQ(Collection, q)
}

// Find finds all pods matching the given query.
func Find(q db.Q) ([]Pod, error) {
	pods := []Pod{}
	return pods, errors.WithStack(db.FindAllQ(Collection, q, &pods))
}

// FindOne finds one pod by the given query.
func FindOne(q db.Q) (*Pod, error) {
	var p Pod
	err := db.FindOneQ(Collection, q, &p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &p, err
}

// FindOneByID finds one pod by its ID.
func FindOneByID(id string) (*Pod, error) {
	p, err := FindOne(db.Query(ByID(id)))
	if err != nil {
		return nil, errors.Wrapf(err, "finding pod '%s'", id)
	}
	return p, nil
}

// UpdateOne updates one pod.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

// FindByNeedsTermination finds all pods running agents that need to be
// terminated, which includes:
// * Pods that have been provisioning for too long.
// * Pods that are decommissioned.
func FindByNeedsTermination() ([]Pod, error) {
	staleCutoff := time.Now().Add(-15 * time.Minute)
	return Find(db.Query(bson.M{
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
			},
		},
	}))
}

// FindByInitializing find all pods that are initializing but have not started
// any containers.
func FindByInitializing() ([]Pod, error) {
	return Find(db.Query(bson.M{
		StatusKey: StatusInitializing,
	}))
}

// CountByInitializing counts the number of pods that are initializing but have
// not started any containers.
func CountByInitializing() (int, error) {
	return Count(db.Query(bson.M{
		StatusKey: StatusInitializing,
	}))
}

// FindOneByExternalID finds a pod that has a matching external identifier.
func FindOneByExternalID(id string) (*Pod, error) {
	return FindOne(db.Query(ByExternalID(id)))
}

// UpdateOneStatus updates a pod's status by ID along with any relevant metadata
// information about the status update. If the current status is identical to
// the updated one, this will no-op. If the current status does not match the
// stored status, this will error.
func UpdateOneStatus(id string, current, updated Status, ts time.Time) error {
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

	if err := UpdateOne(byIDAndStatus, bson.M{
		"$set": setFields,
	}); err != nil {
		return err
	}

	event.LogPodStatusChanged(id, string(current), string(updated))

	return nil
}
