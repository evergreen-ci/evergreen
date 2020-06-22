package build

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Creates a new task cache with the specified id, display name, and value for
// activated.
func NewTaskCache(id string, displayName string, activated bool) TaskCache {
	return TaskCache{
		Id:          id,
		DisplayName: displayName,
		Status:      evergreen.TaskUndispatched,
		StartTime:   utility.ZeroTime,
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
			IdKey:                           buildId,
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

func SetCachedTaskBlocked(buildId, taskId string, blocked bool) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{
			TasksKey + ".$." + TaskCacheBlockedKey: blocked,
		},
	})
}

// SetCachedTaskFinished sets the given task to "finished"
// along with a time taken in the cache of the given build.
func SetCachedTaskFinished(buildId, taskId, status string, detail *apimodels.TaskEndDetail, timeTaken time.Duration) error {
	return updateOneTaskCache(buildId, taskId, bson.M{
		"$set": bson.M{
			TasksKey + ".$." + TaskCacheTimeTakenKey:     timeTaken,
			TasksKey + ".$." + TaskCacheStatusKey:        status,
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

// SetManyCachedTasksActivated activates tasks in their build caches
func SetManyCachedTasksActivated(tasks []task.Task, activated bool) error {
	buildMap := make(map[string]map[string]bool)
	for _, t := range tasks {
		taskID := t.Id
		if t.IsPartOfDisplay() {
			taskID = t.DisplayTask.Id
		}

		if _, ok := buildMap[t.BuildId]; !ok {
			buildMap[t.BuildId] = make(map[string]bool)
		}
		buildMap[t.BuildId][taskID] = true
	}

	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	for buildID, taskIDs := range buildMap {
		taskIDSlice := make([]string, 0, len(taskIDs))
		for taskID := range taskIDs {
			taskIDSlice = append(taskIDSlice, taskID)
		}

		res := env.DB().Collection(Collection).FindOneAndUpdate(ctx,
			bson.M{
				IdKey: buildID,
			},
			bson.M{
				"$set": bson.M{bsonutil.GetDottedKeyName(TasksKey, "$[task]", TaskCacheActivatedKey): activated},
			},
			options.FindOneAndUpdate().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
				bson.M{
					bsonutil.GetDottedKeyName("task", TaskCacheIdKey): bson.M{"$in": taskIDSlice},
				},
			}}),
		)
		if err := res.Err(); err != nil {
			return errors.Wrapf(err, "can't activate build '%s' cached tasks", buildID)
		}
	}

	return nil
}

// ResetCachedTask resets the given task
// in the cache of the given build.
func ResetCachedTask(buildId, taskId string) error {
	return UpdateOne(
		bson.M{
			IdKey:                           buildId,
			TasksKey + "." + TaskCacheIdKey: taskId,
		},
		bson.M{
			"$set": bson.M{
				TasksKey + ".$." + TaskCacheStartTimeKey: utility.ZeroTime,
				TasksKey + ".$." + TaskCacheStatusKey:    evergreen.TaskUndispatched,
				StatusKey:                                evergreen.BuildStarted,
			},
			"$unset": bson.M{
				TasksKey + ".$." + TaskCacheStatusDetailsKey: "",
				TasksKey + ".$." + TaskCacheBlockedKey:       "",
			},
		},
	)
}

// UpdateCachedTask sets the status and increments the time taken for a task
// in the build cache. It is intended for use with display tasks
func UpdateCachedTask(t *task.Task, timeTaken time.Duration) error {
	if t == nil {
		return errors.New("unable to update a cached task with a nil task")
	}
	if t.BuildId == "" {
		return errors.New("unable to update a cached task with a blank build")
	}
	if t.Id == "" {
		return errors.New("unable to update a cached task with a blank task")
	}
	if t.Status == "" {
		return errors.New("unable to update a cached task with a blank status")
	}
	return updateOneTaskCache(t.BuildId, t.Id, bson.M{
		"$set": bson.M{
			TasksKey + ".$." + TaskCacheStatusKey:        t.Status,
			TasksKey + ".$." + TaskCacheStatusDetailsKey: t.Details,
			TasksKey + ".$." + TaskCacheBlockedKey:       t.Blocked(),
		},
		"$inc": bson.M{
			TasksKey + ".$." + TaskCacheTimeTakenKey: timeTaken,
		},
	})
}
