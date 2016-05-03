package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	AllDependencies = "*"
	AllVariants     = "*"
	AllStatuses     = "*"
)

// cacheFromTask is helper for creating a build.TaskCache from a real Task model.
func cacheFromTask(t task.Task) build.TaskCache {
	return build.TaskCache{
		Id:            t.Id,
		DisplayName:   t.DisplayName,
		Status:        t.Status,
		StatusDetails: t.Details,
		StartTime:     t.StartTime,
		TimeTaken:     t.TimeTaken,
		Activated:     t.Activated,
	}
}

// SetVersionActivation updates the "active" state of all builds and tasks associated with a
// version to the given setting. It also updates the task cache for all builds affected.
func SetVersionActivation(versionId string, active bool, caller string) error {
	builds, err := build.Find(
		build.ByVersion(versionId).WithFields(build.IdKey),
	)
	if err != nil {
		return err
	}
	for _, b := range builds {
		err = SetBuildActivation(b.Id, active, caller)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetBuildActivation updates the "active" state of this build and all associated tasks.
// It also updates the task cache for the build document.
func SetBuildActivation(buildId string, active bool, caller string) error {
	var err error

	// If activating a task, set the ActivatedBy field to be the caller
	if active {
		_, err = task.UpdateAll(
			bson.M{
				task.BuildIdKey: buildId,
				task.StatusKey:  evergreen.TaskUndispatched,
			},
			bson.M{"$set": bson.M{task.ActivatedKey: active, task.ActivatedByKey: caller}},
		)
	} else {

		// if trying to deactivate a task then only deactivate tasks that have not been activated by a user.
		// if the caller is the default task activator,
		// only deactivate tasks that are activated by the default task activator
		if evergreen.IsSystemActivator(caller) {
			_, err = task.UpdateAll(
				bson.M{
					task.BuildIdKey:     buildId,
					task.StatusKey:      evergreen.TaskUndispatched,
					task.ActivatedByKey: caller,
				},
				bson.M{"$set": bson.M{task.ActivatedKey: active, task.ActivatedByKey: caller}},
			)

		} else {
			// update all tasks if the caller is not evergreen.
			_, err = task.UpdateAll(
				bson.M{
					task.BuildIdKey: buildId,
					task.StatusKey:  evergreen.TaskUndispatched,
				},
				bson.M{"$set": bson.M{task.ActivatedKey: active, task.ActivatedByKey: caller}},
			)
		}
	}

	if err != nil {
		return err
	}
	if err = build.UpdateActivation(buildId, active, caller); err != nil {
		return err
	}
	return RefreshTasksCache(buildId)
}

// AbortBuild sets the abort flag on all tasks associated with the build which are in an abortable
// state, and marks the build as deactivated.
func AbortBuild(buildId string, caller string) error {
	err := task.AbortBuild(buildId)
	if err != nil {
		return err
	}
	return build.UpdateActivation(buildId, false, caller)
}

// AbortVersion sets the abort flag on all tasks associated with the version which are in an
// abortable state
func AbortVersion(versionId string) error {
	_, err := task.UpdateAll(
		bson.M{
			task.VersionKey: versionId,
			task.StatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
		},
		bson.M{"$set": bson.M{task.AbortedKey: true}},
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
func SetBuildPriority(buildId string, priority int64) error {
	modifier := bson.M{task.PriorityKey: priority}
	//blacklisted - these tasks should never run, so unschedule now
	if priority < 0 {
		modifier[task.ActivatedKey] = false
	}

	_, err := task.UpdateAll(
		bson.M{task.BuildIdKey: buildId},
		bson.M{"$set": modifier},
	)
	return err
}

// SetVersionPriority updates the priority field of all tasks associated with the given build id.

func SetVersionPriority(versionId string, priority int64) error {
	modifier := bson.M{task.PriorityKey: priority}
	//blacklisted - these tasks should never run, so unschedule now
	if priority < 0 {
		modifier[task.ActivatedKey] = false
	}

	_, err := task.UpdateAll(
		bson.M{task.VersionKey: versionId},
		bson.M{"$set": modifier},
	)
	return err
}

// RestartVersion restarts completed tasks associated with a given versionId.
// If abortInProgress is true, it also sets the abort flag on any in-progress tasks.
func RestartVersion(versionId string, taskIds []string, abortInProgress bool, caller string) error {
	// restart all the 'not in-progress' tasks for the version
	allTasks, err := task.Find(task.ByDispatchedWithIdsVersionAndStatus(taskIds, versionId, task.CompletedStatuses))

	if err != nil && err != mgo.ErrNotFound {
		return err
	}

	// archive all the tasks
	for _, t := range allTasks {
		if err := t.Archive(); err != nil {
			return fmt.Errorf("failed to archive task: %v", err)
		}
	}

	// Set all the task fields to indicate restarted
	err = task.ResetTasks(taskIds)
	if err != nil {
		return err
	}

	// TODO figure out a way to coalesce updates for task cache for the same build, so we
	// only need to do one update per-build instead of one per-task here.
	// Doesn't seem to be possible as-is because $ can only apply to one array element matched per
	// document.
	buildIdSet := map[string]bool{}
	for _, t := range allTasks {
		buildIdSet[t.BuildId] = true
		err = build.ResetCachedTask(t.BuildId, t.Id)
		if err != nil {
			return err
		}
	}

	// reset the build statuses, once per build
	buildIdList := make([]string, 0, len(buildIdSet))
	for k, _ := range buildIdSet {
		buildIdList = append(buildIdList, k)
	}

	// Set the build status for all the builds containing the tasks that we touched
	_, err = build.UpdateAllBuilds(
		bson.M{build.IdKey: bson.M{"$in": buildIdList}},
		bson.M{"$set": bson.M{build.StatusKey: evergreen.BuildStarted}},
	)

	if abortInProgress {
		// abort in-progress tasks in this build
		_, err = task.UpdateAll(
			bson.M{
				task.VersionKey: versionId,
				task.IdKey:      bson.M{"$in": taskIds},
				task.StatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
			},
			bson.M{"$set": bson.M{task.AbortedKey: true}},
		)
		if err != nil {
			return err
		}
	}

	// update activation for all the builds
	for _, b := range buildIdList {
		err := build.UpdateActivation(b, true, caller)
		if err != nil {
			return err
		}
	}
	return nil

}

// RestartBuild restarts completed tasks associated with a given buildId.
// If abortInProgress is true, it also sets the abort flag on any in-progress tasks.
func RestartBuild(buildId string, taskIds []string, abortInProgress bool, caller string) error {
	// restart all the 'not in-progress' tasks for the build
	allTasks, err := task.Find(task.ByIdsBuildAndStatus(taskIds, buildId, task.CompletedStatuses))
	if err != nil && err != mgo.ErrNotFound {
		return err
	}

	for _, t := range allTasks {
		if t.DispatchTime != util.ZeroTime {
			err = resetTask(t.Id)
			if err != nil {
				return fmt.Errorf("Restarting build %v failed, could not task.reset on task: %v",
					buildId, t.Id, err)
			}
		}
	}

	if abortInProgress {
		// abort in-progress tasks in this build
		_, err = task.UpdateAll(
			bson.M{
				task.BuildIdKey: buildId,
				task.StatusKey: bson.M{
					"$in": evergreen.AbortableStatuses,
				},
			},
			bson.M{
				"$set": bson.M{
					task.AbortedKey: true,
				},
			},
		)
		if err != nil {
			return err
		}
	}

	return build.UpdateActivation(buildId, true, caller)
}

func CreateTasksCache(tasks []task.Task) []build.TaskCache {
	tasks = sortTasks(tasks)
	cache := make([]build.TaskCache, 0, len(tasks))
	for _, task := range tasks {
		cache = append(cache, cacheFromTask(task))
	}
	return cache
}

// RefreshTasksCache updates a build document so that the tasks cache reflects the correct current
// state of the tasks it represents.
func RefreshTasksCache(buildId string) error {
	tasks, err := task.Find(task.ByBuildId(buildId).WithFields(task.IdKey, task.DisplayNameKey, task.StatusKey,
		task.DetailsKey, task.StartTimeKey, task.TimeTakenKey, task.ActivatedKey, task.DependsOnKey))
	if err != nil {
		return err
	}
	cache := CreateTasksCache(tasks)
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
		project, buildVariant, b, v, NewTaskIdTable(project, v), taskNames)
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

	// update the build to hold the new tasks
	RefreshTasksCache(b.Id)

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
	tasks := make([]task.Task, 0, len(tasksForBuild))
	for _, taskP := range tasksForBuild {
		tasks = append(tasks, *taskP)
	}
	b.Tasks = CreateTasksCache(tasks)

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
	b *build.Build, v *version.Version, tt TaskIdTable, taskNames []string) ([]*task.Task, error) {

	// the list of tasks we should create.  if tasks are passed in, then
	// use those, else use the default set
	tasksToCreate := []BuildVariantTask{}
	createAll := len(taskNames) == 0
	for _, task := range buildVariant.Tasks {
		// get the task spec out of the project
		taskSpec := project.GetSpecForTask(task.Name)

		// sanity check that the config isn't malformed
		if taskSpec.Name == "" {
			return nil, fmt.Errorf("config is malformed: variant '%v' runs "+
				"task called '%v' but no such task exists for repo %v for "+
				"version %v", buildVariant.Name, task.Name, project.Identifier, v.Id)
		}

		// update task document with spec fields
		task.Populate(taskSpec)

		if ((task.Patchable != nil && *task.Patchable == false) || task.Name == evergreen.PushStage) && //TODO remove PushStage
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
	tasks := make([]*task.Task, 0, len(tasksToCreate))
	for _, t := range tasksToCreate {
		newTask := createOneTask(tt.GetId(b.BuildVariant, t.Name), t, project, buildVariant, b, v)

		// set Tags based on the spec
		newTask.Tags = project.GetSpecForTask(t.Name).Tags

		// set the new task's dependencies
		if len(t.DependsOn) == 1 &&
			t.DependsOn[0].Name == AllDependencies &&
			t.DependsOn[0].Variant != AllVariants {
			// the task depends on all of the other tasks in the build
			newTask.DependsOn = make([]task.Dependency, 0, len(tasksToCreate)-1)
			for _, dep := range tasksToCreate {
				status := evergreen.TaskSucceeded
				if t.DependsOn[0].Status != "" {
					status = t.DependsOn[0].Status
				}
				id := tt.GetId(b.BuildVariant, dep.Name)
				if len(id) == 0 || dep.Name == newTask.DisplayName {
					continue
				}
				newTask.DependsOn = append(newTask.DependsOn, task.Dependency{TaskId: id, Status: status})
			}
		} else {
			// the task has specific dependencies
			newTask.DependsOn = make([]task.Dependency, 0, len(t.DependsOn))
			for _, dep := range t.DependsOn {
				// only add as a dependency if the dependency is valid/exists
				status := evergreen.TaskSucceeded
				if dep.Status != "" {
					status = dep.Status
				}
				bv := b.BuildVariant
				if dep.Variant != "" {
					bv = dep.Variant
				}

				newDeps := []task.Dependency{}

				if dep.Variant == AllVariants {
					// for * case, we need to add all variants of the task
					var ids []string
					if dep.Name != AllDependencies {
						ids = tt.GetIdsForAllVariantsExcluding(
							dep.Name,
							TVPair{TaskName: newTask.DisplayName, Variant: newTask.BuildVariant},
						)
					} else {
						// edge case where variant and task are both *
						ids = tt.GetIdsForAllTasks(b.BuildVariant, newTask.DisplayName)
					}
					for _, id := range ids {
						if len(id) != 0 {
							newDeps = append(newDeps, task.Dependency{TaskId: id, Status: status})
						}
					}
				} else {
					// general case
					id := tt.GetId(bv, dep.Name)
					// only create the dependency if the task exists--it always will,
					// except for patches with patch_optional dependencies.
					if len(id) != 0 {
						newDeps = []task.Dependency{task.Dependency{TaskId: id, Status: status}}
					}
				}

				newTask.DependsOn = append(newTask.DependsOn, newDeps...)
			}
		}

		// append the task to the list of the created tasks
		tasks = append(tasks, newTask)
	}

	// Set the NumDependents field
	// Existing tasks in the db and tasks in other builds are not updated
	setNumDeps(tasks)

	// return all of the tasks created
	return tasks, nil
}

// setNumDeps sets NumDependents for each task in tasks.
// NumDependents is the number of tasks depending on the task. Only tasks created at the same time
// and in the same variant are included.
func setNumDeps(tasks []*task.Task) {
	idToTask := make(map[string]*task.Task)
	for i, task := range tasks {
		idToTask[task.Id] = tasks[i]
	}

	for _, task := range tasks {
		// Recursively find all tasks that task depends on and increments their NumDependents field
		setNumDepsRec(task, idToTask, make(map[string]bool))
	}

	return
}

// setNumDepsRec recursively finds all tasks that task depends on and increments their NumDependents field.
// tasks not in idToTasks are not affected.
func setNumDepsRec(task *task.Task, idToTasks map[string]*task.Task, seen map[string]bool) {
	for _, dep := range task.DependsOn {
		// Check whether this dependency is included in the tasks we're currently creating
		if depTask, ok := idToTasks[dep.TaskId]; ok {
			if !seen[depTask.Id] {
				seen[depTask.Id] = true
				depTask.NumDependents = depTask.NumDependents + 1
				setNumDepsRec(depTask, idToTasks, seen)
			}
		}
	}
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
	buildVariant *BuildVariant, b *build.Build, v *version.Version) *task.Task {
	return &task.Task{
		Id:                  id,
		Secret:              util.RandomString(),
		DisplayName:         buildVarTask.Name,
		BuildId:             b.Id,
		BuildVariant:        buildVariant.Name,
		CreateTime:          b.CreateTime,
		PushTime:            b.PushTime,
		ScheduledTime:       util.ZeroTime,
		StartTime:           util.ZeroTime, // Certain time fields must be initialized
		FinishTime:          util.ZeroTime, // to our own util.ZeroTime value (which is
		DispatchTime:        util.ZeroTime, // Unix epoch 0, not Go's time.Time{})
		LastHeartbeat:       util.ZeroTime,
		Status:              evergreen.TaskUndispatched,
		Activated:           b.Activated,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Requester:           v.Requester,
		Version:             v.Id,
		Revision:            v.Revision,
		Project:             project.Identifier,
		Priority:            buildVarTask.Priority,
	}
}

// DeleteBuild removes any record of the build by removing it and all of the tasks that
// are a part of it from the database.
func DeleteBuild(id string) error {
	err := task.RemoveAllWithBuild(id)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	return build.Remove(id)
}

// sortTasks topologically sorts the tasks by dependency, grouping tasks with common dependencies,
// and alphabetically sorting within groups.
// All tasks with cross-variant dependencies are at the far right.
func sortTasks(tasks []task.Task) []task.Task {
	// Separate out tasks with cross-variant dependencies
	taskPresent := make(map[string]bool)
	for _, task := range tasks {
		taskPresent[task.Id] = true
	}
	// depMap is a map from a task ID to the tasks that depend on it
	depMap := make(map[string][]task.Task)
	// crossVariantTasks will contain all tasks with cross-variant dependencies
	crossVariantTasks := make(map[string]task.Task)
	for _, task := range tasks {
		for _, dep := range task.DependsOn {
			if taskPresent[dep.TaskId] {
				depMap[dep.TaskId] = append(depMap[dep.TaskId], task)
			} else {
				crossVariantTasks[task.Id] = task
			}
		}
	}
	for id := range crossVariantTasks {
		for _, task := range depMap[id] {
			addDepChildren(task, crossVariantTasks, depMap)
		}
	}
	// normalTasks will contain all tasks with no cross-variant dependencies
	normalTasks := make(map[string]task.Task)
	for _, t := range tasks {
		if _, ok := crossVariantTasks[t.Id]; !ok {
			normalTasks[t.Id] = t
		}
	}

	// Construct a map of task Id to DisplayName, used to sort both sets of tasks
	idToDisplayName := make(map[string]string)
	for _, t := range tasks {
		idToDisplayName[t.Id] = t.DisplayName
	}

	// All tasks with cross-variant dependencies appear to the right
	sortedTasks := sortTasksHelper(normalTasks, idToDisplayName)
	sortedTasks = append(sortedTasks, sortTasksHelper(crossVariantTasks, idToDisplayName)...)
	return sortedTasks
}

// addDepChildren recursively adds task and all tasks depending on it to tasks
// depMap is a map from a task ID to the tasks that depend on it
func addDepChildren(task task.Task, tasks map[string]task.Task, depMap map[string][]task.Task) {
	if _, ok := tasks[task.Id]; !ok {
		tasks[task.Id] = task
		for _, dep := range depMap[task.Id] {
			addDepChildren(dep, tasks, depMap)
		}
	}
}

// sortTasksHelper sorts the tasks, assuming they all have cross-variant dependencies, or none have
// cross-variant dependencies
func sortTasksHelper(tasks map[string]task.Task, idToDisplayName map[string]string) []task.Task {
	layers := layerTasks(tasks)
	sortedTasks := make([]task.Task, 0, len(tasks))
	for _, layer := range layers {
		sortedTasks = append(sortedTasks, sortLayer(layer, idToDisplayName)...)
	}
	return sortedTasks
}

// layerTasks sorts the tasks into layers
// Layer n contains all tasks whose dependencies are contained in layers 0 through n-1, or are not
// included in tasks (for tasks with cross-variant dependencies)
func layerTasks(tasks map[string]task.Task) [][]task.Task {
	layers := make([][]task.Task, 0)
	for len(tasks) > 0 {
		// Create a new layer
		layer := make([]task.Task, 0)
		for _, task := range tasks {
			// Check if all dependencies are included in previous layers (or were not in tasks)
			if allDepsProcessed(task, tasks) {
				layer = append(layer, task)
			}
		}
		// Add current layer to list of layers
		layers = append(layers, layer)
		// Delete all tasks in this layer
		for _, task := range layer {
			delete(tasks, task.Id)
		}
	}
	return layers
}

// allDepsProcessed checks whether any dependencies of task are in unprocessedTasks
func allDepsProcessed(task task.Task, unprocessedTasks map[string]task.Task) bool {
	for _, dep := range task.DependsOn {
		if _, unprocessed := unprocessedTasks[dep.TaskId]; unprocessed {
			return false
		}
	}
	return true
}

// sortLayer groups tasks by common dependencies, sorting alphabetically within each group
func sortLayer(layer []task.Task, idToDisplayName map[string]string) []task.Task {
	sortKeys := make([]string, 0, len(layer))
	sortKeyToTask := make(map[string]task.Task)
	for _, t := range layer {
		// Construct a key to sort by, consisting of all dependency names, sorted alphabetically,
		// followed by the task name
		sortKeyWords := make([]string, 0, len(t.DependsOn)+1)
		for _, dep := range t.DependsOn {
			depName, ok := idToDisplayName[dep.TaskId]
			// Cross-variant dependencies will not be included in idToDisplayName
			if !ok {
				depName = dep.TaskId
			}
			sortKeyWords = append(sortKeyWords, depName)
		}
		sort.Strings(sortKeyWords)
		sortKeyWords = append(sortKeyWords, t.DisplayName)
		sortKey := strings.Join(sortKeyWords, " ")
		sortKeys = append(sortKeys, sortKey)
		sortKeyToTask[sortKey] = t
	}
	sort.Strings(sortKeys)
	sortedLayer := make([]task.Task, 0, len(layer))
	for _, sortKey := range sortKeys {
		sortedLayer = append(sortedLayer, sortKeyToTask[sortKey])
	}
	return sortedLayer
}
