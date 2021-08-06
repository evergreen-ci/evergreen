package pod

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
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
	StatusKey                    = bsonutil.MustHaveTag(Pod{}, "Status")
	SecretKey                    = bsonutil.MustHaveTag(Pod{}, "Secret")
	TaskContainerCreationOptsKey = bsonutil.MustHaveTag(Pod{}, "TaskContainerCreationOpts")
	TimeInfoKey                  = bsonutil.MustHaveTag(Pod{}, "TimeInfo")
	ResourcesKey                 = bsonutil.MustHaveTag(Pod{}, "Resources")

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
	ContainerResourceInfoStatusKey     = bsonutil.MustHaveTag(ContainerResourceInfo{}, "Status")
	ContainerResourceInfoSecretIDsKey  = bsonutil.MustHaveTag(ContainerResourceInfo{}, "SecretIDs")
)

// Find finds all pods matching the given query.
func Find(q bson.M) ([]Pod, error) {
	pods := []Pod{}
	return pods, errors.WithStack(db.FindAllQ(Collection, db.Query(q), &pods))
}

// FindOne finds one pod by the given query.
func FindOne(q bson.M) (*Pod, error) {
	var p Pod
	err := db.FindOneQ(Collection, db.Query(q), &p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &p, err
}

// FindOneByID finds one pod by its ID.
func FindOneByID(id string) (*Pod, error) {
	p, err := FindOne(ByID(id))
	if err != nil {
		return nil, errors.Wrapf(err, "finding pod '%s'", id)
	}
	return p, nil
}

func ByID(id string) bson.M {
	return bson.M{
		IDKey: id,
	}
}

// UpdateOne updates one pod.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

// FindByStaleStarting finds all pods running tasks that have been stuck in
// the starting state for an extended period of time.
func FindByStaleStarting() ([]Pod, error) {
	startingCutoff := time.Now().Add(-15 * time.Minute)
	return Find(bson.M{
		bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoStartingKey): bson.M{"$lte": startingCutoff},
		StatusKey: StatusStarting,
	})
}

// FindByInitializing find all pods that are initializing but have not started any containers.
func FindByInitializing() ([]Pod, error) {
	return Find(bson.M{
		StatusKey: StatusInitializing,
	})
}
