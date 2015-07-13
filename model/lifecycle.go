package model

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"strconv"
	"time"
)

const (
	AllDependencies = "*"
	AllVariants     = "*"
	AllStatuses     = "*"
)

// cacheFromTask is helper for creating a build.TaskCache from a real Task model.
func cacheFromTask(t *Task) build.TaskCache {
	return build.TaskCache{
		Id:          t.Id,
		DisplayName: t.DisplayName,
		Status:      t.Status,
		StartTime:   t.StartTime,
		TimeTaken:   t.TimeTaken,
		Activated:   t.Activated,
	}
}

// SetVersionActivation updates the "active" state of all builds and tasks associated with a
// version to the given setting. It also updates the task cache for all builds affected.
func SetVersionActivation(versionId string, active bool) error {
	builds, err := build.Find(
		build.ByVersion(versionId).WithFields(build.IdKey),
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
			TaskStatusKey:  evergreen.TaskUndispatched,
		},
		bson.M{"$set": bson.M{TaskActivatedKey: active}},
	)
	if err != nil {
		return err
	}
	if err = build.UpdateActivation(buildId, active); err != nil {
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
			TaskStatusKey: bson.M{"$in": evergreen.AbortableStatuses},
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
			TaskStatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
		},
		bson.M{"$set": bson.M{TaskAbortedKey: true}},
	)
	if err != nil {
		return err
	}
	return build.UpdateActivation(buildId, false)
}

// AbortVersion sets the abort flag on all tasks associated with the version which are in an
// abortable state
func AbortVersion(versionId string) error {
	_, err := UpdateAllTasks(
		bson.M{
			TaskVersionKey: versionId,
			TaskStatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
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
			version.StatusKey:    evergreen.VersionStarted,
		}},
	)
}

// MarkVersionCompleted updates the status of a completed version to reflect its correct state by
// checking the status of its individual builds.
func MarkVersionCompleted(versionId string, finishTime time.Time) error {
	status := evergreen.VersionSucceeded

	// Find the statuses for all builds in the version so we can figure out the version's status
	builds, err := build.Find(
		build.ByVersion(versionId).WithFields(build.StatusKey),
	)
	if err != nil {
		return err
	}

	for _, b := range builds {
		if !b.IsFinished() {
			return nil
		}
		if b.Status != evergreen.BuildSucceeded {
			status = evergreen.VersionFailed
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
					evergreen.TaskSucceeded,
					evergreen.TaskFailed,
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
					"$in": evergreen.AbortableStatuses,
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

	return build.UpdateActivation(buildId, true)
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

	cache := make([]build.TaskCache, 0, len(tasks))
	for _, t := range tasks {
		cache = append(cache, cacheFromTask(&t))
	}
	return build.SetTasksCache(buildId, cache)
}

//AddTasksToBuild creates the tasks for the given build of a project
func AddTasksToBuild(b *build.Build, project *Project, v *version.Version,
	taskNames []string) (*build.Build, error) {

	// find the build variant for this project/build
	buildVariant := project.FindBuildVariant(b.BuildVariant)
	if buildVariant == nil {
		return nil, fmt.Errorf("Could not find build %v in %v project file",
			b.BuildVariant, project.Identifier)
	}

	// create the new tasks for the build
	tasks, err := createTasksForBuild(
		project, buildVariant, b, v, BuildTaskIdTable(project, v), taskNames)
	if err != nil {
		return nil, fmt.Errorf("error creating tasks for build %v: %v",
			b.Id, err)
	}

	// insert the tasks into the db
	for _, task := range tasks {
		evergreen.Logger.Logf(slogger.INFO, "Creating task “%v”", task.DisplayName)
		if err := task.Insert(); err != nil {
			return nil, fmt.Errorf("error inserting task %v: %v", task.Id, err)
		}
	}

	// create task caches for all of the tasks, and add them into the build
	for _, t := range tasks {
		b.Tasks = append(b.Tasks, cacheFromTask(t))
	}

	// update the build to hold the new tasks
	if err = build.SetTasksCache(b.Id, b.Tasks); err != nil {
		return nil, err
	}

	return b, nil
}

// CreateBuildFromVersion creates a build given all of the necessary information
// from the corresponding version and project and a list of tasks.
func CreateBuildFromVersion(project *Project, v *version.Version, tt TaskIdTable,
	buildName string, activated bool, taskNames []string) (string, error) {

	evergreen.Logger.Logf(slogger.DEBUG, "Creating %v %v build, activated: %v", v.Requester, buildName, activated)

	// find the build variant for this project/build
	buildVariant := project.FindBuildVariant(buildName)
	if buildVariant == nil {
		return "", fmt.Errorf("could not find build %v in %v project file", buildName, project.Identifier)
	}

	// create a new build id
	buildId := util.CleanName(
		fmt.Sprintf("%v_%v_%v_%v",
			project.Identifier,
			buildName,
			v.Revision,
			v.CreateTime.Format(build.IdTimeLayout)))

	// create the build itself
	b := &build.Build{
		Id:                  buildId,
		CreateTime:          v.CreateTime,
		PushTime:            v.CreateTime,
		Activated:           activated,
		Project:             project.Identifier,
		Revision:            v.Revision,
		Status:              evergreen.BuildCreated,
		BuildVariant:        buildName,
		Version:             v.Id,
		DisplayName:         buildVariant.DisplayName,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Requester:           v.Requester,
	}

	// get a new build number for the build
	buildNumber, err := db.GetNewBuildVariantBuildNumber(buildName)
	if err != nil {
		return "", fmt.Errorf("could not get build number for build variant"+
			" %v in %v project file", buildName, project.Identifier)
	}
	b.BuildNumber = strconv.FormatUint(buildNumber, 10)

	// create all of the necessary tasks for the build
	tasksForBuild, err := createTasksForBuild(project, buildVariant, b, v, tt, taskNames)
	if err != nil {
		return "", fmt.Errorf("error creating tasks for build %v: %v", b.Id, err)
	}

	// insert all of the build's tasks into the db
	for _, task := range tasksForBuild {
		if err := task.Insert(); err != nil {
			return "", fmt.Errorf("error inserting task %v: %v", task.Id, err)
		}
	}

	// create task caches for all of the tasks, and place them into the build
	b.Tasks = make([]build.TaskCache, 0, len(tasksForBuild))
	for _, t := range tasksForBuild {
		b.Tasks = append(b.Tasks, cacheFromTask(t))
	}

	// insert the build
	if err := b.Insert(); err != nil {
		return "", fmt.Errorf("error inserting build %v: %v", b.Id, err)
	}

	// success!
	return b.Id, nil
}

// createTasksForBuild creates all of the necessary tasks for the build.  Returns a
// slice of all of the tasks created, as well as an error if any occurs.
// The slice of tasks will be in the same order as the project's specified tasks
// appear in the specified build variant.
func createTasksForBuild(project *Project, buildVariant *BuildVariant,
	b *build.Build, v *version.Version, tt TaskIdTable, taskNames []string) ([]*Task, error) {

	// the list of tasks we should create.  if tasks are passed in, then
	// use those, else use the default set
	tasksToCreate := []BuildVariantTask{}
	createAll := len(taskNames) == 0
	for _, task := range buildVariant.Tasks {
		if task.Name == evergreen.PushStage &&
			b.Requester == evergreen.PatchVersionRequester {
			continue
		}
		if createAll || util.SliceContains(taskNames, task.Name) {
			tasksToCreate = append(tasksToCreate, task)
		}
	}

	// if any tasks already exist in the build, add them to the id table
	// so they can be used as dependencies
	for _, task := range b.Tasks {
		tt.AddId(b.BuildVariant, task.DisplayName, task.Id)
	}

	// create and insert all of the actual tasks
	tasks := make([]*Task, 0, len(tasksToCreate))
	for _, task := range tasksToCreate {

		// get the task spec out of the project
		var taskSpec ProjectTask
		for _, projectTask := range project.Tasks {
			if projectTask.Name == task.Name {
				taskSpec = projectTask
				break
			}
		}

		// sanity check that the config isn't malformed
		if taskSpec.Name == "" {
			return nil, fmt.Errorf("config is malformed: variant '%v' runs "+
				"task called '%v' but no such task exists for repo %v for "+
				"version %v", buildVariant.Name, task.Name, project.Identifier,
				v.Id)
		}

		newTask := createOneTask(tt.GetId(b.BuildVariant, task.Name), task, project, buildVariant, b, v)

		// set the new task's dependencies
		// TODO encapsulate better
		if len(taskSpec.DependsOn) == 1 &&
			taskSpec.DependsOn[0].Name == AllDependencies &&
			taskSpec.DependsOn[0].Variant != AllVariants {
			// the task depends on all of the other tasks in the build
			newTask.DependsOn = make([]Dependency, 0, len(tasksToCreate)-1)
			for _, dep := range tasksToCreate {
				status := evergreen.TaskSucceeded
				if taskSpec.DependsOn[0].Status != "" {
					status = taskSpec.DependsOn[0].Status
				}
				newDep := Dependency{
					TaskId: tt.GetId(b.BuildVariant, dep.Name),
					Status: status,
				}
				if dep.Name != newTask.DisplayName {
					newTask.DependsOn = append(newTask.DependsOn, newDep)
				}
			}
		} else {
			// the task has specific dependencies
			newTask.DependsOn = make([]Dependency, 0, len(taskSpec.DependsOn))
			for _, dep := range taskSpec.DependsOn {
				// only add as a dependency if the dependency is valid/exists
				status := evergreen.TaskSucceeded
				if dep.Status != "" {
					status = dep.Status
				}
				bv := b.BuildVariant
				if dep.Variant != "" {
					bv = dep.Variant
				}

				newDeps := []Dependency{}

				if dep.Variant == AllVariants {
					// for * case, we need to add all variants of the task
					ids := tt.GetIdsForAllVariants(b.BuildVariant, dep.Name)
					for _, id := range ids {
						newDeps = append(newDeps, Dependency{TaskId: id, Status: status})
					}
				} else {
					// general case
					newDep := Dependency{
						TaskId: tt.GetId(bv, dep.Name),
						Status: status,
					}
					if newDep.TaskId != "" {
						newDeps = []Dependency{newDep}
					}
				}

				newTask.DependsOn = append(newTask.DependsOn, newDeps...)
			}
		}

		// append the task to the list of the created tasks
		tasks = append(tasks, newTask)
	}

	// return all of the tasks created
	return tasks, nil
}

// TryMarkPatchBuildFinished attempts to mark a patch as finished if all
// the builds for the patch are finished as well
func TryMarkPatchBuildFinished(b *build.Build, finishTime time.Time) error {
	v, err := version.FindOne(version.ById(b.Version))
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("Cannot find version for build %v with version %v", b.Id, b.Version)
	}

	// ensure all builds for this patch are finished as well
	builds, err := build.Find(build.ByIds(v.BuildIds).WithFields(build.StatusKey))
	if err != nil {
		return err
	}

	patchCompleted := true
	status := evergreen.PatchSucceeded
	for _, build := range builds {
		if !build.IsFinished() {
			patchCompleted = false
		}
		if build.Status != evergreen.BuildSucceeded {
			status = evergreen.PatchFailed
		}
	}

	// nothing to do if the patch isn't completed
	if !patchCompleted {
		return nil
	}

	return patch.TryMarkFinished(v.Id, finishTime, status)
}

// createOneTask is a helper to create a single task.
func createOneTask(id string, buildVarTask BuildVariantTask, project *Project,
	buildVariant *BuildVariant, b *build.Build, v *version.Version) *Task {
	return &Task{
		Id:                  id,
		Secret:              util.RandomString(),
		DisplayName:         buildVarTask.Name,
		BuildId:             b.Id,
		BuildVariant:        buildVariant.Name,
		CreateTime:          b.CreateTime,
		PushTime:            b.PushTime,
		ScheduledTime:       ZeroTime,
		StartTime:           ZeroTime, // Certain time fields must be initialized
		FinishTime:          ZeroTime, // to our own ZeroTime value (which is
		DispatchTime:        ZeroTime, // Unix epoch 0, not Go's time.Time{})
		LastHeartbeat:       ZeroTime,
		Status:              evergreen.TaskUndispatched,
		Activated:           b.Activated,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Requester:           v.Requester,
		Version:             v.Id,
		Revision:            v.Revision,
		Project:             project.Identifier,
	}
}

// DeleteBuild removes any record of the build by removing it and all of the tasks that
// are a part of it from the database.
func DeleteBuild(id string) error {
	err := RemoveAllTasks(bson.M{TaskBuildIdKey: id})
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	return build.Remove(id)
}
