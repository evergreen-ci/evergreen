package pod

import (
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
	ResourcesKey                 = bsonutil.MustHaveTag(Pod{}, "Resources")
	TimeInfoKey                  = bsonutil.MustHaveTag(Pod{}, "TimeInfo")

	TaskContainerCreationOptsImageKey    = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "Image")
	TaskContainerCreationOptsMemoryMBKey = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "MemoryMB")
	TaskContainerCreationOptsCPUKey      = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "CPU")
	TaskContainerCreationOptsPlatformKey = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "Platform")
	TaskContainerCreationOptsEnvVarsKey  = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "EnvVars")
	TaskContainerCreationOptsSecretsKey  = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "EnvSecrets")

	ResourceInfoID           = bsonutil.MustHaveTag(ResourceInfo{}, "ID")
	ResourceInfoDefinitionID = bsonutil.MustHaveTag(ResourceInfo{}, "DefinitionID")
	ResourceInfoCluster      = bsonutil.MustHaveTag(ResourceInfo{}, "Cluster")
	ResourceInfoSecretIDs    = bsonutil.MustHaveTag(ResourceInfo{}, "SecretIDs")

	TimeInfoInitializedKey = bsonutil.MustHaveTag(TimeInfo{}, "Initialized")
	TimeInfoStartedKey     = bsonutil.MustHaveTag(TimeInfo{}, "Started")
	TimeInfoProvisionedKey = bsonutil.MustHaveTag(TimeInfo{}, "Provisioned")
	TimeInfoTerminatedKey  = bsonutil.MustHaveTag(TimeInfo{}, "Terminated")
)

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
