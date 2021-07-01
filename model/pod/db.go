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
	ExternalIDKey                = bsonutil.MustHaveTag(Pod{}, "ExternalID")
	TaskContainerCreationOptsKey = bsonutil.MustHaveTag(Pod{}, "TaskContainerCreationOpts")
	TimeInfoKey                  = bsonutil.MustHaveTag(Pod{}, "TimeInfo")

	TaskContainerCreationOptsImageKey              = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "Image")
	TaskContainerCreationOptsMemoryMBKey           = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "MemoryMB")
	TaskContainerCreationOptsCPUKey                = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "CPU")
	TaskContainerCreationOptsIsWindowsContainerKey = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "IsWindows")
	TaskContainerCreationOptsEnvVarsKey            = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "EnvVars")
	TaskContainerCreationOptsSecretsKey            = bsonutil.MustHaveTag(TaskContainerCreationOptions{}, "EnvSecrets")

	TimeInfoInitializedKey = bsonutil.MustHaveTag(TimeInfo{}, "Initialized")
	TimeInfoStartedKey     = bsonutil.MustHaveTag(TimeInfo{}, "Started")
	TimeInfoProvisionedKey = bsonutil.MustHaveTag(TimeInfo{}, "Provisioned")
)

// FindOne finds one pod by the given query.
func FindOne(query db.Q) (*Pod, error) {
	var p Pod
	err := db.FindOneQ(Collection, query, &p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &p, err
}

// FindOneByID finds one pod by its ID.
func FindOneByID(id string) (*Pod, error) {
	query := db.Query(bson.M{IDKey: id})
	p, err := FindOne(query)
	if err != nil {
		return nil, errors.Wrapf(err, "finding pod '%s'", id)
	}
	return p, nil
}
