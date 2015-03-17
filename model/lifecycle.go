package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model/version"
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

// SetVersionActivation updates the "active" state of all builds and tasks associated with a
// version to the given setting. It also updates the task cache for all builds affected.
func SetVersionActivation(versionId string, active bool) error {
	builds, err := FindAllBuilds(
		bson.M{BuildVersionKey: versionId},
		bson.M{BuildIdKey: 1},
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		return err
	}
	for _, b := range builds {
		err = SetBuildActivation(b.Id, active)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetBuildActivation updates the "active" state of this build and all associated tasks.
// It also updates the task cache for the build document.
func SetBuildActivation(buildId string, active bool) error {
	_, err := UpdateAllTasks(
		bson.M{
			TaskBuildIdKey: buildId,
			TaskStatusKey:  mci.TaskUndispatched,
		},
		bson.M{"$set": bson.M{TaskActivatedKey: active}},
	)
	if err != nil {
		return err
	}
	err = UpdateOneBuild(
		bson.M{BuildIdKey: buildId},
		bson.M{
			"$set": bson.M{
				BuildActivatedKey:     active,
				BuildActivatedTimeKey: time.Now(),
			},
		},
	)
	if err != nil {
		return err
	}
	return RefreshTasksCache(buildId)
}

// SetTaskActivation updates the task's active state to the given value.
// Does not update the task cache for the associated build, so this may need to be done separately
// after this function is called.
func SetTaskActivation(taskId string, active bool) error {
	return UpdateOneTask(
		bson.M{TaskIdKey: taskId},
		bson.M{"$set": bson.M{TaskActivatedKey: active}},
	)
}

// AbortTask sets the "abort" flag on a given task ID, if it is currently in a state which allows
// abort to be triggered.
func AbortTask(taskId string) error {
	err := UpdateOneTask(
		bson.M{
			TaskIdKey:     taskId,
			TaskStatusKey: bson.M{"$in": mci.AbortableStatuses},
		},
		bson.M{"$set": bson.M{TaskAbortedKey: true}},
	)
	return err
}

// AbortBuild sets the abort flag on all tasks associated with the build which are in an abortable
// state, and marks the build as deactivated.
func AbortBuild(buildId string) error {
	_, err := UpdateAllTasks(
		bson.M{
			TaskBuildIdKey: buildId,
			TaskStatusKey:  bson.M{"$in": mci.AbortableStatuses},
		},
		bson.M{"$set": bson.M{TaskAbortedKey: true}},
	)
	if err != nil {
		return err
	}
	return UpdateOneBuild(
		bson.M{BuildIdKey: buildId},
		bson.M{
			"$set": bson.M{
				BuildActivatedKey: false,
			},
		},
	)
}

// AbortVersion sets the abort flag on all tasks associated with the version which are in an
// abortable state
func AbortVersion(versionId string) error {
	_, err := UpdateAllTasks(
		bson.M{
			TaskVersionKey: versionId,
			TaskStatusKey:  bson.M{"$in": mci.AbortableStatuses},
		},
		bson.M{"$set": bson.M{TaskAbortedKey: true}},
	)
	return err
}

func MarkVersionStarted(versionId string, startTime time.Time) error {
	return version.UpdateOne(
		bson.M{version.IdKey: versionId},
		bson.M{"$set": bson.M{
			version.StartTimeKey: startTime,
			version.StatusKey:    mci.VersionStarted,
		}},
	)
}

// MarkVersionCompleted updates the status of a completed version to reflect its correct state by
// checking the status of its individual builds.
func MarkVersionCompleted(versionId string, finishTime time.Time) error {
	status := mci.VersionSucceeded

	// Find the statuses for all builds in the version so we can figure out the version's status
	builds, err := FindAllBuilds(
		bson.M{BuildVersionKey: versionId},
		bson.M{BuildStatusKey: 1},
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		return err
	}

	for _, build := range builds {
		if !build.IsFinished() {
			return nil
		}
		if build.Status != mci.BuildSucceeded {
			status = mci.VersionFailed
		}
	}

	return version.UpdateOne(
		bson.M{version.IdKey: versionId},
		bson.M{"$set": bson.M{
			version.FinishTimeKey: finishTime,
			version.StatusKey:     status,
		}},
	)
}

// SetBuildPriority updates the priority field of all tasks associated with the given build id.
func SetBuildPriority(buildId string, priority int) error {
	modifier := bson.M{TaskPriorityKey: priority}
	//blacklisted - these tasks should never run, so unschedule now
	if priority < 0 {
		modifier[TaskActivatedKey] = false
	}

	_, err := UpdateAllTasks(
		bson.M{TaskBuildIdKey: buildId},
		bson.M{"$set": modifier},
	)
	return err
}

// SetVersionPriority updates the priority field of all tasks associated with the given build id.
func SetVersionPriority(versionId string, priority int) error {
	modifier := bson.M{TaskPriorityKey: priority}
	//blacklisted - these tasks should never run, so unschedule now
	if priority < 0 {
		modifier[TaskActivatedKey] = false
	}

	_, err := UpdateAllTasks(
		bson.M{TaskVersionKey: versionId},
		bson.M{"$set": modifier},
	)
	return err
}

// RestartBuild restarts completed tasks associated with a given buildId.
// If abortInProgress is true, it also sets the abort flag on any in-progress tasks.
func RestartBuild(buildId string, abortInProgress bool) error {
	// restart all the 'not in-progress' tasks for the build

	allTasks, err := FindAllTasks(
		bson.M{
			TaskBuildIdKey: buildId,
			TaskStatusKey: bson.M{
				"$in": []string{
					mci.TaskSucceeded,
					mci.TaskFailed,
				},
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)

	if err != nil && err != mgo.ErrNotFound {
		return err
	}

	for _, task := range allTasks {
		if task.DispatchTime != ZeroTime {
			err = task.reset()
			if err != nil {
				return fmt.Errorf("Restarting build %v failed, could not task.reset on task: %v",
					buildId, task.Id, err)
			}
		}
	}

	if abortInProgress {
		// abort in-progress tasks in this build
		_, err = UpdateAllTasks(
			bson.M{
				TaskBuildIdKey: buildId,
				TaskStatusKey: bson.M{
					"$in": mci.AbortableStatuses,
				},
			},
			bson.M{
				"$set": bson.M{
					TaskAbortedKey: true,
				},
			},
		)

		if err != nil {
			return err
		}
	}

	return UpdateOneBuild(
		bson.M{BuildIdKey: buildId},
		bson.M{
			"$set": bson.M{
				BuildActivatedKey: true},
		},
	)
}

// RefreshTasksCache updates a build document so that the tasks cache reflects the correct current
// state of the tasks it represents.
func RefreshTasksCache(buildId string) error {
	tasks, err := FindAllTasks(
		bson.M{TaskBuildIdKey: buildId},
		bson.M{
			TaskIdKey:          1,
			TaskDisplayNameKey: 1,
			TaskStatusKey:      1,
			TaskStartTimeKey:   1,
			TaskTimeTakenKey:   1,
			TaskActivatedKey:   1,
		},
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
	if err != nil {
		return err
	}

	cache := make([]TaskCache, 0, len(tasks))
	for _, t := range tasks {
		cache = append(cache, TaskCache{
			Id:          t.Id,
			DisplayName: t.DisplayName,
			Status:      t.Status,
			StartTime:   t.StartTime,
			TimeTaken:   t.TimeTaken,
			Activated:   t.Activated,
		})
	}

	return UpdateOneBuild(bson.M{BuildIdKey: buildId}, bson.M{"$set": bson.M{BuildTasksKey: cache}})
}
