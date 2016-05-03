package build

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/mgo.v2/bson"
)

// Creates a new task cache with the specified id, display name, and value for
// activated.
func NewTaskCache(id string, displayName string, activated bool) TaskCache {
	return TaskCache{
		Id:          id,
		DisplayName: displayName,
		Status:      evergreen.TaskUndispatched,
		StartTime:   util.ZeroTime,
		TimeTaken:   time.Duration(0),
		Activated:   activated,
	}
}

// SetTasksCache updates one build with the given id
// to contain the given task caches.
func SetTasksCache(buildId string, tasks []TaskCache) error {
	return UpdateOne(
		bson.M{IdKey: buildId},
		bson.M{"$set": bson.M{TasksKey: tasks}},
	)
}

// updateOneTaskCache is a helper for updating a single cached task for a build.
func updateOneTaskCache(buildId, taskId string, updateDoc bson.M) error {
	return UpdateOne(
		bson.M{
			IdKey: buildId,
			TasksKey + "." + TaskCacheIdKey: taskId,
		},
		updateDoc,
	)
}

// SetCachedTaskDispatched sets the given task to "dispatched"
// in the cache of the given build.
func SetCachedTaskDispatched(buildId, taskId string) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{TasksKey + ".$." + TaskCacheStatusKey: evergreen.TaskDispatched},
	})
}

// SetCachedTaskUndispatched sets the given task to "undispatched"
// in the cache of the given build.
func SetCachedTaskUndispatched(buildId, taskId string) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{TasksKey + ".$." + TaskCacheStatusKey: evergreen.TaskUndispatched},
	})
}

// SetCachedTaskStarted sets the given task to "started"
// in the cache of the given build.
func SetCachedTaskStarted(buildId, taskId string, startTime time.Time) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{
			TasksKey + ".$." + TaskCacheStartTimeKey: startTime,
			TasksKey + ".$." + TaskCacheStatusKey:    evergreen.TaskStarted,
		},
	})
}

// SetCachedTaskFinished sets the given task to "finished"
// along with a time taken in the cache of the given build.
func SetCachedTaskFinished(buildId, taskId string, detail *apimodels.TaskEndDetail, timeTaken time.Duration) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{
			TasksKey + ".$." + TaskCacheTimeTakenKey:     timeTaken,
			TasksKey + ".$." + TaskCacheStatusKey:        detail.Status,
			TasksKey + ".$." + TaskCacheStatusDetailsKey: detail,
		},
	})
}

// SetCachedTaskActivated sets the given task to active or inactive
// in the cache of the given build.
func SetCachedTaskActivated(buildId, taskId string, active bool) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{TasksKey + ".$." + TaskCacheActivatedKey: active},
	})
}

// ResetCachedTask resets the given task
// in the cache of the given build.
func ResetCachedTask(buildId, taskId string) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{
			TasksKey + ".$." + TaskCacheStartTimeKey: util.ZeroTime,
			TasksKey + ".$." + TaskCacheStatusKey:    evergreen.TaskUndispatched,
		},
		"$unset": bson.M{
			TasksKey + ".$." + TaskCacheStatusDetailsKey: "",
		},
	})
}
