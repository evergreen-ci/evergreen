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

// Find gets all Pods for the given BSON document.
func Find(doc bson.M) ([]Pod, error) {
	pods := []Pod{}
	return pods, errors.WithStack(db.FindAllQ(Collection, db.Query(doc), &pods))
}

// FindByInitializing find all pods that are initializing but have not started any containers.
func FindByInitializing() ([]Pod, error) {
	return Find(bson.M{
		StatusKey: StatusInitializing,
	})
}
