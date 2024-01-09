package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	AllDependencies = "*"
	AllVariants     = "*"
	AllStatuses     = "*"
)

type RestartOptions struct {
	DryRun    bool      `bson:"dry_run" json:"dry_run"`
	StartTime time.Time `bson:"start_time" json:"start_time"`
	EndTime   time.Time `bson:"end_time" json:"end_time"`
	User      string    `bson:"user" json:"user"`

	// note that the bson tags are not quite accurate, but are kept around for backwards compatibility
	IncludeTestFailed  bool `bson:"only_red" json:"only_red"`
	IncludeSysFailed   bool `bson:"only_purple" json:"only_purple"`
	IncludeSetupFailed bool `bson:"include_setup_failed" json:"include_setup_failed"`
}

type RestartResults struct {
	ItemsRestarted []string
	ItemsErrored   []string
}

type VersionToRestart struct {
	VersionId *string  `json:"version_id"`
	TaskIds   []string `json:"task_ids"`
}

// SetVersionActivation updates the "active" state of all builds and tasks associated with a
// version to the given setting. It also updates the task cache for all builds affected.
func SetVersionActivation(versionId string, active bool, caller string) error {
	q := task.ByVersionWithChildTasks(versionId)
	q[task.StatusKey] = evergreen.TaskUndispatched

	var tasksToModify []task.Task
	var err error
	// If activating a task, set the ActivatedBy field to be the caller.
	if active {
		if err := SetVersionActivated(versionId, active); err != nil {
			return errors.Wrapf(err, "setting activated for version '%s'", versionId)
		}
		tasksToModify, err = task.FindAll(db.Query(q).WithFields(task.IdKey, task.DependsOnKey, task.ExecutionKey, task.BuildIdKey, task.ActivatedKey))
		if err != nil {
			return errors.Wrap(err, "getting tasks to activate")
		}
		if len(tasksToModify) > 0 {
			if err = task.ActivateTasks(tasksToModify, time.Now(), false, caller); err != nil {
				return errors.Wrap(err, "updating tasks for activation")
			}
		}
	} else {
		// If the caller is the default task activator, only deactivate tasks that have not been activated by a user.
		if evergreen.IsSystemActivator(caller) {
			q[task.ActivatedByKey] = bson.M{"$in": evergreen.SystemActivators}
		}

		tasksToModify, err = task.FindAll(db.Query(q).WithFields(task.IdKey, task.ExecutionKey, task.BuildIdKey))
		if err != nil {
			return errors.Wrap(err, "getting tasks to deactivate")
		}
		if len(tasksToModify) > 0 {
			if err = task.DeactivateTasks(tasksToModify, false, caller); err != nil {
				return errors.Wrap(err, "deactivating tasks")
			}
		}
	}

	if len(tasksToModify) == 0 {
		return nil
	}

	buildIdsMap := map[string]bool{}
	var buildIds []string
	for _, t := range tasksToModify {
		buildIdsMap[t.BuildId] = true
	}
	for buildId := range buildIdsMap {
		buildIds = append(buildIds, buildId)
	}
	if err := build.UpdateActivation(buildIds, active, caller); err != nil {
		return errors.Wrapf(err, "setting build activations to %t", active)
	}
	if err := UpdateVersionAndPatchStatusForBuilds(buildIds); err != nil {
		return errors.Wrapf(err, "updating build and version status for version '%s'", versionId)
	}
	return nil
}

// ActivateBuildsAndTasks updates the "active" state of this build and all associated tasks.
// It also updates the task cache for the build document.
func ActivateBuildsAndTasks(buildIds []string, active bool, caller string) error {
	if err := build.UpdateActivation(buildIds, active, caller); err != nil {
		return errors.Wrapf(err, "setting build activation to %t for builds '%v'", active, buildIds)
	}

	return errors.Wrapf(setTaskActivationForBuilds(buildIds, active, true, nil, caller),
		"setting task activation for builds '%v'", buildIds)
}

// setTaskActivationForBuilds updates the "active" state of all non-disabled tasks in buildIds.
// It also updates the task cache for the build document.
// If withDependencies is true, also set dependencies. Don't need to do this when the entire version is affected.
// If tasks are given to ignore, then we don't activate those tasks.
func setTaskActivationForBuilds(buildIds []string, active, withDependencies bool, ignoreTasks []string, caller string) error {
	// If activating a task, set the ActivatedBy field to be the caller
	if active {
		q := bson.M{
			task.BuildIdKey:  bson.M{"$in": buildIds},
			task.StatusKey:   evergreen.TaskUndispatched,
			task.PriorityKey: bson.M{"$gt": evergreen.DisabledTaskPriority},
		}
		if len(ignoreTasks) > 0 {
			q[task.IdKey] = bson.M{"$nin": ignoreTasks}
		}
		tasksToActivate, err := task.FindAll(db.Query(q).WithFields(task.IdKey, task.DependsOnKey, task.ExecutionKey, task.ActivatedKey))
		if err != nil {
			return errors.Wrap(err, "getting tasks to activate")
		}
		if withDependencies {
			dependOn, err := task.GetRecursiveDependenciesUp(tasksToActivate, nil)
			if err != nil {
				return errors.Wrap(err, "getting recursive dependencies")
			}
			tasksToActivate = append(tasksToActivate, dependOn...)

		}
		if err = task.ActivateTasks(tasksToActivate, time.Now(), withDependencies, caller); err != nil {
			return errors.Wrap(err, "updating tasks for activation")
		}

	} else {
		query := bson.M{
			task.BuildIdKey: bson.M{"$in": buildIds},
			task.StatusKey:  evergreen.TaskUndispatched,
		}
		// if the caller is the default task activator only deactivate tasks that have not been activated by a user
		if evergreen.IsSystemActivator(caller) {
			query[task.ActivatedByKey] = bson.M{"$in": evergreen.SystemActivators}
		}

		tasks, err := task.FindAll(db.Query(query).WithFields(task.IdKey, task.ExecutionKey))
		if err != nil {
			return errors.Wrap(err, "getting tasks to deactivate")
		}
		if err = task.DeactivateTasks(tasks, withDependencies, caller); err != nil {
			return errors.Wrap(err, "deactivating tasks")
		}
	}

	if err := UpdateVersionAndPatchStatusForBuilds(buildIds); err != nil {
		return errors.Wrapf(err, "updating status for builds '%s'", buildIds)
	}
	return nil
}

// AbortBuild marks the build as deactivated and sets the abort flag on all tasks associated
// with the build which are in an abortable state.
func AbortBuild(buildId string, caller string) error {
	if err := build.UpdateActivation([]string{buildId}, false, caller); err != nil {
		return errors.Wrapf(err, "deactivating build '%s'", buildId)
	}

	return errors.Wrapf(task.AbortBuildTasks(buildId, task.AbortInfo{User: caller}), "aborting tasks for build '%s'", buildId)
}

func TryMarkVersionStarted(versionId string, startTime time.Time) error {
	err := VersionUpdateOne(
		bson.M{
			VersionIdKey:     versionId,
			VersionStatusKey: bson.M{"$ne": evergreen.VersionStarted},
		},
		bson.M{"$set": bson.M{
			VersionStartTimeKey: startTime,
			VersionStatusKey:    evergreen.VersionStarted,
		}},
	)
	if adb.ResultsNotFound(err) {
		return nil
	}
	return err
}

// SetTaskPriority sets the priority for the given task. Any of the task's
// dependencies that have a lower priority than the one being set for this task
// will also have their priority increased.
func SetTaskPriority(ctx context.Context, t task.Task, priority int64, caller string) error {
	depTasks, err := task.GetRecursiveDependenciesUp([]task.Task{t}, nil)
	if err != nil {
		return errors.Wrap(err, "getting task dependencies")
	}

	ids := append([]string{t.Id}, t.ExecutionTasks...)
	depIDs := make([]string, 0, len(depTasks))
	for _, depTask := range depTasks {
		depIDs = append(depIDs, depTask.Id)
	}

	query := db.Query(bson.M{
		"$or": []bson.M{
			{task.IdKey: bson.M{"$in": ids}},
			{
				task.IdKey:       bson.M{"$in": depIDs},
				task.PriorityKey: bson.M{"$lt": priority},
			},
		},
	}).WithFields(task.ExecutionKey)
	tasks, err := task.FindAll(query)
	if err != nil {
		return errors.Wrap(err, "finding matching tasks")
	}

	taskIDs := make([]string, 0, len(tasks))
	for _, taskToUpdate := range tasks {
		taskIDs = append(taskIDs, taskToUpdate.Id)
	}
	_, err = task.UpdateAll(
		bson.M{task.IdKey: bson.M{"$in": taskIDs}},
		bson.M{"$set": bson.M{task.PriorityKey: priority}},
	)
	if err != nil {
		return errors.Wrap(err, "updating priority")
	}
	for _, modifiedTask := range tasks {
		event.LogTaskPriority(modifiedTask.Id, modifiedTask.Execution, caller, priority)
	}

	// negative priority - deactivate the task
	if priority <= evergreen.DisabledTaskPriority {
		if err = SetActiveState(ctx, caller, false, t); err != nil {
			return errors.Wrap(err, "deactivating task")
		}
	}

	return nil
}

// SetBuildPriority updates the priority field of all tasks associated with the given build id.
func SetBuildPriority(ctx context.Context, buildId string, priority int64, caller string) error {
	query := bson.M{task.BuildIdKey: buildId}
	return errors.Wrap(setTasksPriority(ctx, query, priority, caller), "setting priority for build")
}

// SetVersionsPriority updates the priority field of all tasks and child tasks associated with the given version ids.
func SetVersionsPriority(ctx context.Context, versionIds []string, priority int64, caller string) error {
	query := task.ByVersionsWithChildTasks(versionIds)
	return errors.Wrap(setTasksPriority(ctx, query, priority, caller), "setting priority for versions")
}

func setTasksPriority(ctx context.Context, query bson.M, priority int64, caller string) error {
	_, err := task.UpdateAll(query,
		bson.M{"$set": bson.M{task.PriorityKey: priority}},
	)
	if err != nil {
		return errors.Wrap(err, "setting priority")
	}
	tasks, err := task.FindAll(db.Query(query))
	if err != nil {
		return errors.Wrap(err, "getting tasks")
	}
	var taskIds []string
	for _, t := range tasks {
		taskIds = append(taskIds, t.Id)
	}
	event.LogManyTaskPriority(taskIds, caller, priority)

	// Tasks with negative priority should never run, so we unschedule them.
	if priority < 0 {
		return errors.Wrap(SetActiveState(ctx, caller, false, tasks...), "deactivating tasks")
	}

	return nil
}

// RestartVersion restarts completed tasks belonging to the given version ID.
// If no task IDs are provided, all completed task IDs in the version are
// restarted.
// If abortInProgress is true, it also sets the abort and reset flags on
// any in-progress tasks.
func RestartVersion(ctx context.Context, versionID string, taskIDs []string, abortInProgress bool, caller string) error {
	if abortInProgress {
		if err := task.AbortAndMarkResetTasksForVersion(ctx, versionID, taskIDs, caller); err != nil {
			return errors.WithStack(err)
		}
	}

	completedTasks, err := task.FindCompletedTasksByVersion(ctx, versionID, taskIDs)
	if err != nil {
		return errors.Wrap(err, "finding completed tasks for version")
	}
	if len(completedTasks) == 0 {
		return nil
	}

	return restartTasks(ctx, completedTasks, caller, versionID)
}

// RestartVersions restarts selected tasks for a set of versions.
// If abortInProgress is true for any version, it also sets the abort and reset
// flags on any in-progress tasks belonging to that version.
func RestartVersions(ctx context.Context, versionsToRestart []*VersionToRestart, abortInProgress bool, caller string) error {
	catcher := grip.NewBasicCatcher()
	for _, t := range versionsToRestart {
		err := RestartVersion(ctx, *t.VersionId, t.TaskIds, abortInProgress, caller)
		catcher.Wrapf(err, "restarting tasks for version '%s'", *t.VersionId)
	}
	return errors.Wrap(catcher.Resolve(), "restarting tasks")
}

// RestartBuild restarts completed tasks belonging to the given build.
// If no task IDs are provided, all completed task IDs in the build are
// restarted.
// If abortInProgress is true, it also sets the abort and reset flags on
// any in-progress tasks.
func RestartBuild(ctx context.Context, b *build.Build, taskIDs []string, abortInProgress bool, caller string) error {
	if abortInProgress {
		if err := task.AbortAndMarkResetTasksForBuild(ctx, b.Id, taskIDs, caller); err != nil {
			return errors.WithStack(err)
		}
	}

	completedTasks, err := task.FindCompletedTasksByBuild(ctx, b.Id, taskIDs)
	if err != nil {
		return errors.Wrap(err, "finding completed tasks for build")
	}
	if len(completedTasks) == 0 {
		return nil
	}

	return restartTasks(ctx, completedTasks, caller, b.Version)
}

// restartTasks restarts all finished tasks in the given list that are not part of
// a single host task group.
func restartTasks(ctx context.Context, allFinishedTasks []task.Task, caller, versionId string) error {
	toArchive := []task.Task{}
	for _, t := range allFinishedTasks {
		if !t.IsPartOfSingleHostTaskGroup() {
			// We do not archive single host TG tasks here because we must wait for
			// the task group to be fully complete, at which point we can
			// archive them all at once.
			toArchive = append(toArchive, t)
		}
	}
	if err := task.ArchiveMany(toArchive); err != nil {
		return errors.Wrap(err, "archiving tasks")
	}

	type taskGroupAndBuild struct {
		Build     string
		TaskGroup string
	}

	// Only need to check one task per task group / build combination
	taskGroupsToCheck := map[taskGroupAndBuild]task.Task{}
	restartIds := []string{}
	for _, t := range allFinishedTasks {
		if t.IsPartOfSingleHostTaskGroup() {
			if err := t.SetResetWhenFinished(); err != nil {
				return errors.Wrapf(err, "marking '%s' for restart when finished", t.Id)
			}
			taskGroupsToCheck[taskGroupAndBuild{
				Build:     t.BuildId,
				TaskGroup: t.TaskGroup,
			}] = t
		} else {
			// Only restart non-single host task group tasks
			restartIds = append(restartIds, t.Id)
			if t.DisplayOnly {
				restartIds = append(restartIds, t.ExecutionTasks...)
			}
		}
	}

	for tg, t := range taskGroupsToCheck {
		if err := checkResetSingleHostTaskGroup(ctx, &t, caller); err != nil {
			return errors.Wrapf(err, "resetting task group '%s' for build '%s'", tg.TaskGroup, tg.Build)
		}
	}

	// Set all the task fields to indicate restarted
	if err := MarkTasksReset(restartIds); err != nil {
		return errors.WithStack(err)
	}
	for _, t := range allFinishedTasks {
		if !t.IsPartOfSingleHostTaskGroup() { // this will be logged separately if task group is restarted
			event.LogTaskRestarted(t.Id, t.Execution, caller)
		}
	}

	if err := build.SetBuildStartedForTasks(allFinishedTasks, caller); err != nil {
		return errors.Wrap(err, "setting builds started")
	}
	builds, err := build.FindBuildsForTasks(allFinishedTasks)
	if err != nil {
		return errors.Wrap(err, "finding builds for tasks")
	}
	for _, b := range builds {
		if err = checkUpdateBuildPRStatusPending(ctx, &b); err != nil {
			return errors.Wrapf(err, "updating build '%s' PR status", b.Id)
		}
	}
	return errors.Wrap(setVersionStatus(versionId, evergreen.VersionStarted), "changing version status")
}

func CreateTasksCache(tasks []task.Task) []build.TaskCache {
	tasks = sortTasks(tasks)
	cache := make([]build.TaskCache, 0, len(tasks))
	for _, task := range tasks {
		if task.DisplayTask == nil {
			cache = append(cache, build.TaskCache{Id: task.Id})
		}
	}
	return cache
}

// RefreshTasksCache updates a build document so that the tasks cache reflects the correct current
// state of the tasks it represents.
func RefreshTasksCache(buildId string) error {
	tasks, err := task.FindAll(db.Query(task.ByBuildId(buildId)))
	if err != nil {
		return errors.WithStack(err)
	}
	tasks, err = task.AddParentDisplayTasks(tasks)
	if err != nil {
		return errors.WithStack(err)
	}
	// trim out tasks that are part of a display task
	execTaskMap := map[string]bool{}
	for _, t := range tasks {
		if t.DisplayOnly {
			for _, et := range t.ExecutionTasks {
				execTaskMap[et] = true
			}
		}
	}
	for i := len(tasks) - 1; i >= 0; i-- {
		if _, exists := execTaskMap[tasks[i].Id]; exists {
			tasks = append(tasks[:i], tasks[i+1:]...)
		}
	}

	cache := CreateTasksCache(tasks)
	return errors.WithStack(build.SetTasksCache(buildId, cache))
}

// addTasksToBuild creates/activates the tasks for the given existing build.
func addTasksToBuild(ctx context.Context, creationInfo TaskCreationInfo) (*build.Build, task.Tasks, error) {
	// Find the build variant for this project/build
	creationInfo.BuildVariant = creationInfo.Project.FindBuildVariant(creationInfo.Build.BuildVariant)
	if creationInfo.BuildVariant == nil {
		return nil, nil, errors.Errorf("could not find build '%s' in project file '%s'",
			creationInfo.Build.BuildVariant, creationInfo.Project.Identifier)
	}

	createTime, err := getTaskCreateTime(creationInfo)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "getting create time for tasks in version '%s'", creationInfo.Version.Id)
	}

	var githubCheckAliases ProjectAliases
	if creationInfo.Version.Requester == evergreen.RepotrackerVersionRequester && creationInfo.ProjectRef.IsGithubChecksEnabled() {
		githubCheckAliases, err = FindAliasInProjectRepoOrConfig(creationInfo.Version.Identifier, evergreen.GithubChecksAlias)
		grip.Error(message.WrapError(err, message.Fields{
			"message":            "error getting github check aliases when adding tasks to build",
			"project":            creationInfo.Version.Identifier,
			"project_identifier": creationInfo.ProjectRef.Identifier,
			"version":            creationInfo.Version.Id,
		}))
	}
	creationInfo.GithubChecksAliases = githubCheckAliases
	creationInfo.TaskCreateTime = createTime
	// Create the new tasks for the build
	tasks, err := createTasksForBuild(creationInfo)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "creating tasks for build '%s'", creationInfo.Build.Id)
	}

	if err = tasks.InsertUnordered(ctx); err != nil {
		return nil, nil, errors.Wrapf(err, "inserting tasks for build '%s'", creationInfo.Build.Id)
	}

	var hasGitHubCheck bool
	var hasUnfinishedEssentialTask bool
	for _, t := range tasks {
		if t.IsGithubCheck {
			hasGitHubCheck = true
		}
		if t.IsEssentialToSucceed {
			hasUnfinishedEssentialTask = true
		}
	}
	if hasGitHubCheck {
		if err := creationInfo.Build.SetIsGithubCheck(); err != nil {
			return nil, nil, errors.Wrapf(err, "setting build '%s' as a GitHub check", creationInfo.Build.Id)
		}
	}
	if err := creationInfo.Build.SetHasUnfinishedEssentialTask(hasUnfinishedEssentialTask); err != nil {
		return nil, nil, errors.Wrapf(err, "setting build '%s' as having an unfinished essential task", creationInfo.Build.Id)
	}

	// update the build to hold the new tasks
	if err = RefreshTasksCache(creationInfo.Build.Id); err != nil {
		return nil, nil, errors.Wrapf(err, "updating task cache for '%s'", creationInfo.Build.Id)
	}

	batchTimeTaskStatuses := []BatchTimeTaskStatus{}
	tasksWithActivationTime := creationInfo.ActivationInfo.getActivationTasks(creationInfo.Build.BuildVariant)
	batchTimeCatcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		if !utility.StringSliceContains(tasksWithActivationTime, t.DisplayName) {
			continue
		}
		bvtu := creationInfo.Project.FindTaskForVariant(t.DisplayName, creationInfo.Build.BuildVariant)
		if bvtu.HasCheckRun() {
			t.HasCheckRun = true
		}
		if !bvtu.HasSpecificActivation() {
			continue
		}
		activateTaskAt, err := creationInfo.ProjectRef.GetActivationTimeForTask(bvtu)
		batchTimeCatcher.Wrapf(err, "getting activation time for task '%s'", t.DisplayName)
		batchTimeTaskStatuses = append(batchTimeTaskStatuses, BatchTimeTaskStatus{
			TaskName: t.DisplayName,
			TaskId:   t.Id,
			ActivationStatus: ActivationStatus{
				ActivateAt: activateTaskAt,
			},
		})
	}

	// update the build in the variant
	for i, status := range creationInfo.Version.BuildVariants {
		if status.BuildVariant != creationInfo.Build.BuildVariant {
			continue
		}
		creationInfo.Version.BuildVariants[i].BatchTimeTasks = append(creationInfo.Version.BuildVariants[i].BatchTimeTasks, batchTimeTaskStatuses...)
	}
	grip.Error(message.WrapError(batchTimeCatcher.Resolve(), message.Fields{
		"message": "unable to get activation time for tasks",
		"variant": creationInfo.Build.BuildVariant,
		"runner":  "addTasksToBuild",
		"version": creationInfo.Version.Id,
	}))

	return creationInfo.Build, tasks, nil
}

// CreateBuildFromVersionNoInsert creates a build given all of the necessary information
// from the corresponding version and project and a list of tasks. Note that the caller
// is responsible for inserting the created build and task documents
func CreateBuildFromVersionNoInsert(creationInfo TaskCreationInfo) (*build.Build, task.Tasks, error) {
	// avoid adding all tasks in the case of no tasks matching aliases
	if len(creationInfo.Aliases) > 0 && len(creationInfo.TaskNames) == 0 {
		return nil, nil, nil
	}
	// Find the build variant for this project/build
	buildVariant := creationInfo.Project.FindBuildVariant(creationInfo.BuildVariantName)
	if buildVariant == nil {
		return nil, nil, errors.Errorf("could not find build '%s' in project file '%s'", creationInfo.BuildVariantName, creationInfo.Project.Identifier)
	}

	rev := creationInfo.Version.Revision
	if evergreen.IsPatchRequester(creationInfo.Version.Requester) {
		rev = fmt.Sprintf("patch_%s_%s", creationInfo.Version.Revision, creationInfo.Version.Id)
	} else if creationInfo.Version.Requester == evergreen.TriggerRequester {
		rev = fmt.Sprintf("%s_%s", creationInfo.SourceRev, creationInfo.DefinitionID)
	} else if creationInfo.Version.Requester == evergreen.AdHocRequester {
		rev = creationInfo.Version.Id
	} else if creationInfo.Version.Requester == evergreen.GitTagRequester {
		rev = fmt.Sprintf("%s_%s", creationInfo.SourceRev, creationInfo.Version.TriggeredByGitTag.Tag)
	}

	// create a new build id
	buildId := fmt.Sprintf("%s_%s_%s_%s",
		creationInfo.ProjectRef.Identifier,
		creationInfo.BuildVariantName,
		rev,
		creationInfo.Version.CreateTime.Format(build.IdTimeLayout))

	activatedTime := utility.ZeroTime
	if creationInfo.ActivateBuild {
		activatedTime = time.Now()
	}

	// create the build itself
	b := &build.Build{
		Id:                  util.CleanName(buildId),
		CreateTime:          creationInfo.Version.CreateTime,
		Activated:           creationInfo.ActivateBuild,
		ActivatedTime:       activatedTime,
		Project:             creationInfo.Project.Identifier,
		Revision:            creationInfo.Version.Revision,
		Status:              evergreen.BuildCreated,
		BuildVariant:        creationInfo.BuildVariantName,
		Version:             creationInfo.Version.Id,
		DisplayName:         buildVariant.DisplayName,
		RevisionOrderNumber: creationInfo.Version.RevisionOrderNumber,
		Requester:           creationInfo.Version.Requester,
		ParentPatchID:       creationInfo.Version.ParentPatchID,
		ParentPatchNumber:   creationInfo.Version.ParentPatchNumber,
		TriggerID:           creationInfo.Version.TriggerID,
		TriggerType:         creationInfo.Version.TriggerType,
		TriggerEvent:        creationInfo.Version.TriggerEvent,
		Tags:                buildVariant.Tags,
	}

	// create all the necessary tasks for the build
	creationInfo.BuildVariant = buildVariant
	creationInfo.Build = b
	tasksForBuild, err := createTasksForBuild(creationInfo)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "creating tasks for build '%s'", b.Id)
	}

	// create task caches for all of the tasks, and place them into the build
	tasks := []task.Task{}
	containsActivatedTask := false
	hasUnfinishedEssentialTask := false
	for _, taskP := range tasksForBuild {
		if taskP.IsGithubCheck {
			b.IsGithubCheck = true
		}
		if taskP.Activated {
			containsActivatedTask = true
		}
		if taskP.IsEssentialToSucceed {
			hasUnfinishedEssentialTask = true
		}
		if taskP.IsPartOfDisplay() {
			continue // don't add execution parts of display tasks to the UI cache
		}
		tasks = append(tasks, *taskP)
	}
	b.Tasks = CreateTasksCache(tasks)
	b.Activated = containsActivatedTask
	b.HasUnfinishedEssentialTask = hasUnfinishedEssentialTask
	return b, tasksForBuild, nil
}

// CreateTasksFromGroup expands a task group into its individual tasks and
// returns a build variant task unit for each task in the task group.
func CreateTasksFromGroup(in BuildVariantTaskUnit, proj *Project, requester string) []BuildVariantTaskUnit {
	var willRun []BuildVariantTaskUnit
	for _, bvt := range proj.tasksFromGroup(in) {
		if !bvt.IsDisabled() && !bvt.SkipOnRequester(requester) {
			willRun = append(willRun, bvt)
		}
	}
	return willRun
}

// createTasksForBuild creates all of the necessary tasks for the build.  Returns a
// slice of all of the tasks created, as well as an error if any occurs.
// The slice of tasks will be in the same order as the project's specified tasks
// appear in the specified build variant.
// If tasksToActivate is nil, then all tasks will be activated.
func createTasksForBuild(creationInfo TaskCreationInfo) (task.Tasks, error) {

	// The list of tasks we should create.
	// If tasks are passed in, then use those, otherwise use the default set.
	tasksToCreate := []BuildVariantTaskUnit{}

	createAll := false
	if len(creationInfo.TaskNames) == 0 && len(creationInfo.DisplayNames) == 0 {
		createAll = true
	}
	// Tables includes only new and existing tasks.
	execTable := creationInfo.TaskIDs.ExecutionTasks
	displayTable := creationInfo.TaskIDs.DisplayTasks

	tgMap := map[string]TaskGroup{}
	for _, tg := range creationInfo.Project.TaskGroups {
		tgMap[tg.Name] = tg
	}
	for _, variant := range creationInfo.Project.BuildVariants {
		for _, t := range variant.Tasks {
			if t.TaskGroup != nil {
				tgMap[t.Name] = *t.TaskGroup
			}
		}
	}

	for _, task := range creationInfo.BuildVariant.Tasks {
		// Verify that the config isn't malformed.
		if task.Name != "" && !task.IsGroup {
			if task.IsDisabled() || task.SkipOnRequester(creationInfo.Build.Requester) {
				continue
			}
			if createAll || utility.StringSliceContains(creationInfo.TaskNames, task.Name) {
				tasksToCreate = append(tasksToCreate, task)
			}
		} else if _, ok := tgMap[task.Name]; ok {
			tasksFromVariant := CreateTasksFromGroup(task, creationInfo.Project, creationInfo.Build.Requester)
			for _, taskFromVariant := range tasksFromVariant {
				if task.IsDisabled() || taskFromVariant.SkipOnRequester(creationInfo.Build.Requester) {
					continue
				}
				if createAll || utility.StringSliceContains(creationInfo.TaskNames, taskFromVariant.Name) {
					tasksToCreate = append(tasksToCreate, taskFromVariant)
				}
			}
		} else {
			return nil, errors.Errorf("config is malformed: variant '%s' runs "+
				"task called '%s' but no such task exists for repo '%s' for "+
				"version '%s'", creationInfo.BuildVariant.Name, task.Name, creationInfo.Project.Identifier, creationInfo.Version.Id)
		}
	}

	// if any tasks already exist in the build, add them to the id table
	// so they can be used as dependencies
	for _, task := range creationInfo.TasksInBuild {
		execTable.AddId(creationInfo.Build.BuildVariant, task.DisplayName, task.Id)
	}
	generatorIsGithubCheck := false
	if creationInfo.GeneratedBy != "" {
		generateTask, err := task.FindOneId(creationInfo.GeneratedBy)
		if err != nil {
			return nil, errors.Wrapf(err, "finding generated task '%s'", creationInfo.GeneratedBy)
		}
		if generateTask == nil {
			return nil, errors.Errorf("generated task '%s' not found", creationInfo.GeneratedBy)
		}
		generatorIsGithubCheck = generateTask.IsGithubCheck
	}

	// create all the actual tasks
	taskMap := make(map[string]*task.Task)
	for _, t := range tasksToCreate {
		id := execTable.GetId(creationInfo.Build.BuildVariant, t.Name)
		newTask, err := createOneTask(id, creationInfo, t)
		if err != nil {
			return nil, errors.Wrapf(err, "creating task '%s'", id)
		}

		projectTask := creationInfo.Project.FindProjectTask(t.Name)
		if projectTask != nil {
			newTask.Tags = projectTask.Tags
		}
		newTask.DependsOn = makeDeps(t.DependsOn, newTask, execTable)
		newTask.GeneratedBy = creationInfo.GeneratedBy
		if generatorIsGithubCheck {
			newTask.IsGithubCheck = true
		}

		if shouldSyncTask(creationInfo.SyncAtEndOpts.VariantsTasks, newTask.BuildVariant, newTask.DisplayName) {
			newTask.CanSync = true
			newTask.SyncAtEndOpts = task.SyncAtEndOptions{
				Enabled:  true,
				Statuses: creationInfo.SyncAtEndOpts.Statuses,
				Timeout:  creationInfo.SyncAtEndOpts.Timeout,
			}
		} else {
			cmds, err := creationInfo.Project.CommandsRunOnTV(TVPair{TaskName: newTask.DisplayName, Variant: newTask.BuildVariant}, evergreen.S3PushCommandName)
			if err != nil {
				return nil, errors.Wrapf(err, "checking if task definition contains command '%s'", evergreen.S3PushCommandName)
			}
			if len(cmds) != 0 {
				newTask.CanSync = true
			}
		}

		taskMap[newTask.Id] = newTask
	}

	// Create and update display tasks
	tasks := task.Tasks{}
	loggedExecutionTaskNotFound := false
	for _, dt := range creationInfo.BuildVariant.DisplayTasks {
		id := displayTable.GetId(creationInfo.Build.BuildVariant, dt.Name)
		if id == "" {
			continue
		}
		execTasksThatNeedParentId := []string{}
		execTaskIds := []string{}
		displayTaskActivated := false
		displayTaskAlreadyExists := !createAll && !utility.StringSliceContains(creationInfo.DisplayNames, dt.Name)

		// get display task activations status and update exec tasks
		for _, et := range dt.ExecTasks {
			execTaskId := execTable.GetId(creationInfo.Build.BuildVariant, et)
			if execTaskId == "" {
				if !loggedExecutionTaskNotFound {
					grip.Debug(message.Fields{
						"message":                     "execution task not found",
						"variant":                     creationInfo.Build.BuildVariant,
						"exec_task":                   et,
						"project":                     creationInfo.Project.Identifier,
						"display_task":                id,
						"display_task_already_exists": displayTaskAlreadyExists,
					})
					loggedExecutionTaskNotFound = true
				}
				continue
			}
			execTaskIds = append(execTaskIds, execTaskId)
			if execTask, ok := taskMap[execTaskId]; ok {
				if execTask.Activated {
					displayTaskActivated = true
				}
				taskMap[execTaskId].DisplayTaskId = utility.ToStringPtr(id)
			} else {
				// exec task already exists so update its parent ID in the database
				execTasksThatNeedParentId = append(execTasksThatNeedParentId, execTaskId)
			}
		}

		// update existing exec tasks
		grip.Error(message.WrapError(task.AddDisplayTaskIdToExecTasks(id, execTasksThatNeedParentId), message.Fields{
			"message":              "problem adding display task ID to exec tasks",
			"exec_tasks_to_update": execTasksThatNeedParentId,
			"display_task_id":      id,
			"display_task":         dt.Name,
			"build_id":             creationInfo.Build.Id,
		}))

		// existing display task may need to be updated
		if displayTaskAlreadyExists {
			grip.Error(message.WrapError(task.AddExecTasksToDisplayTask(id, execTaskIds, displayTaskActivated), message.Fields{
				"message":      "problem adding exec tasks to display tasks",
				"exec_tasks":   execTaskIds,
				"display_task": dt.Name,
				"build_id":     creationInfo.Build.Id,
			}))
		} else { // need to create display task
			if len(execTaskIds) == 0 {
				continue
			}
			newDisplayTask, err := createDisplayTask(id, creationInfo, dt.Name, execTaskIds, creationInfo.TaskCreateTime, displayTaskActivated)
			if err != nil {
				return nil, errors.Wrapf(err, "creating display task '%s'", id)
			}
			newDisplayTask.GeneratedBy = creationInfo.GeneratedBy
			newDisplayTask.DependsOn, err = task.GetAllDependencies(newDisplayTask.ExecutionTasks, taskMap)
			if err != nil {
				return nil, errors.Wrapf(err, "getting dependencies for display task '%s'", newDisplayTask.Id)
			}

			tasks = append(tasks, newDisplayTask)
		}
	}
	addSingleHostTaskGroupDependencies(taskMap, creationInfo.Project, execTable)

	for _, t := range taskMap {
		tasks = append(tasks, t)
	}

	// Set the NumDependents field
	// Existing tasks in the db and tasks in other builds are not updated
	setNumDeps(tasks)

	sort.Stable(tasks)

	// return all of the tasks created
	return tasks, nil
}

// addSingleHostTaskGroupDependencies adds dependencies to any tasks in a single-host task group
func addSingleHostTaskGroupDependencies(taskMap map[string]*task.Task, p *Project, taskIds TaskIdTable) {
	for _, t := range taskMap {
		if t.TaskGroup == "" {
			continue
		}
		tg := p.FindTaskGroup(t.TaskGroup)
		if tg == nil || tg.MaxHosts > 1 {
			continue
		}
		singleHostTGDeps := []TaskUnitDependency{}
		// Iterate backwards until we find a task that exists in the taskMap. This task
		// will be the parent dependency for the current single host TG task.
		taskFound := false
		for i := len(tg.Tasks) - 1; i >= 0; i-- {
			// Check the task display names since no display name will appear twice
			// within the same task group
			if t.DisplayName == tg.Tasks[i] {
				taskFound = true
				continue
			}
			if _, ok := taskMap[taskIds.GetId(t.BuildVariant, tg.Tasks[i])]; ok && taskFound {
				singleHostTGDeps = append(singleHostTGDeps, TaskUnitDependency{
					Name:    tg.Tasks[i],
					Variant: t.BuildVariant,
				})
				break
			}
		}
		t.DependsOn = append(t.DependsOn, makeDeps(singleHostTGDeps, t, taskIds)...)
	}
}

// makeDeps takes dependency definitions in the project and sets them in the task struct.
// dependencies between commit queue merges are set outside this function
func makeDeps(deps []TaskUnitDependency, thisTask *task.Task, taskIds TaskIdTable) []task.Dependency {
	dependencySet := make(map[task.Dependency]bool)
	for _, dep := range deps {
		status := evergreen.TaskSucceeded
		if dep.Status != "" {
			status = dep.Status
		}

		// set unspecified fields to match thisTask
		if dep.Name == "" {
			dep.Name = thisTask.DisplayName
		}
		if dep.Variant == "" {
			dep.Variant = thisTask.BuildVariant
		}

		var depIDs []string
		if dep.Variant == AllVariants && dep.Name == AllDependencies {
			depIDs = taskIds.GetIdsForAllTasks()
		} else if dep.Variant == AllVariants {
			depIDs = taskIds.GetIdsForTaskInAllVariants(dep.Name)
		} else if dep.Name == AllDependencies {
			depIDs = taskIds.GetIdsForAllTasksInVariant(dep.Variant)
		} else {
			// don't add missing dependencies
			// patch_optional tasks aren't in the patch and will be missing from the table
			if id := taskIds.GetId(dep.Variant, dep.Name); id != "" {
				depIDs = []string{id}
			}
		}

		for _, id := range depIDs {
			// tasks don't depend on themselves
			if id == thisTask.Id {
				continue
			}
			dependencySet[task.Dependency{TaskId: id, Status: status, OmitGeneratedTasks: dep.OmitGeneratedTasks}] = true
		}
	}

	dependencies := make([]task.Dependency, 0, len(dependencySet))
	for dep := range dependencySet {
		dependencies = append(dependencies, dep)
	}

	return dependencies
}

// shouldSyncTask returns whether or not this task in this build variant should
// sync its task directory.
func shouldSyncTask(syncVariantsTasks []patch.VariantTasks, bv, task string) bool {
	for _, vt := range syncVariantsTasks {
		if vt.Variant != bv {
			continue
		}
		if utility.StringSliceContains(vt.Tasks, task) {
			return true
		}
		for _, dt := range vt.DisplayTasks {
			if utility.StringSliceContains(dt.ExecTasks, task) {
				return true
			}
		}
	}
	return false
}

// setNumDeps sets NumDependents for each task in tasks.
// NumDependents is the number of tasks depending on the task. Only tasks created at the same time
// and in the same variant are included.
func setNumDeps(tasks []*task.Task) {
	idToTask := make(map[string]*task.Task)
	for i, task := range tasks {
		idToTask[task.Id] = tasks[i]
	}
	deduplicatedTasks := []*task.Task{}
	for _, task := range idToTask {
		task.NumDependents = 0
		deduplicatedTasks = append(deduplicatedTasks, task)
	}
	for _, task := range deduplicatedTasks {
		// Recursively find all tasks that task depends on and increments their NumDependents field
		setNumDepsRec(task, idToTask, make(map[string]bool))
	}
}

// setNumDepsRec recursively finds all tasks that task depends on and increments their NumDependents field.
// tasks not in idToTasks are not affected.
func setNumDepsRec(t *task.Task, idToTasks map[string]*task.Task, seen map[string]bool) {
	for _, dep := range t.DependsOn {
		// Check whether this dependency is included in the tasks we're currently creating
		depTask, ok := idToTasks[dep.TaskId]
		if !ok {
			// TODO: if it becomes possible to depend on tasks outside a task's version in
			// a workflow other than the commit queue, add handling here
			continue
		}
		if !seen[depTask.Id] {
			seen[depTask.Id] = true
			depTask.NumDependents = depTask.NumDependents + 1
			setNumDepsRec(depTask, idToTasks, seen)
		}
	}
}

func RecomputeNumDependents(t task.Task) error {
	pipelineDown := getAllNodesInDepGraph(t.Id, bsonutil.GetDottedKeyName(task.DependsOnKey, task.DependencyTaskIdKey), task.IdKey)
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	cursor, err := env.DB().Collection(task.Collection).Aggregate(ctx, pipelineDown)
	if err != nil {
		return err
	}
	depTasks := []task.Task{}
	err = cursor.All(ctx, &depTasks)
	if err != nil {
		return err
	}
	taskPtrs := []*task.Task{}
	for i := range depTasks {
		taskPtrs = append(taskPtrs, &depTasks[i])
	}

	pipelineUp := getAllNodesInDepGraph(t.Id, task.IdKey, bsonutil.GetDottedKeyName(task.DependsOnKey, task.DependencyTaskIdKey))
	cursor, err = env.DB().Collection(task.Collection).Aggregate(ctx, pipelineUp)
	if err != nil {
		return errors.Wrap(err, "getting upstream dependencies of node")
	}
	depTasks = []task.Task{}
	err = cursor.All(ctx, &depTasks)
	if err != nil {
		return err
	}
	for i := range depTasks {
		taskPtrs = append(taskPtrs, &depTasks[i])
	}

	versionTasks, err := task.FindAll(db.Query(task.ByVersion(t.Version)))
	if err != nil {
		return errors.Wrap(err, "getting tasks in version")
	}
	for i := range versionTasks {
		taskPtrs = append(taskPtrs, &versionTasks[i])
	}

	setNumDeps(taskPtrs)
	catcher := grip.NewBasicCatcher()
	for _, t := range taskPtrs {
		catcher.Add(t.SetNumDependents())
	}

	return errors.Wrap(catcher.Resolve(), "setting num dependents")
}

func getAllNodesInDepGraph(startTaskId, startKey, linkKey string) []bson.M {
	return []bson.M{
		{
			"$match": bson.M{
				task.IdKey: startTaskId,
			},
		},
		{
			"$graphLookup": bson.M{
				"from":             task.Collection,
				"startWith":        "$" + startKey,
				"connectFromField": startKey,
				"connectToField":   linkKey,
				"as":               "dep_graph",
			},
		},
		{
			"$addFields": bson.M{
				"dep_graph": bson.M{
					"$concatArrays": []interface{}{"$dep_graph", []string{"$$ROOT"}},
				},
			},
		},
		{
			"$project": bson.M{
				"_id":     0,
				"results": "$dep_graph",
			},
		},
		{
			"$unwind": "$results",
		},
		{
			"$replaceRoot": bson.M{
				"newRoot": "$results",
			},
		},
		{
			"$project": bson.M{
				task.IdKey:        1,
				task.DependsOnKey: 1,
			},
		},
	}
}

func getTaskCreateTime(creationInfo TaskCreationInfo) (time.Time, error) {
	createTime := time.Time{}
	if evergreen.IsPatchRequester(creationInfo.Version.Requester) {
		baseVersion, err := VersionFindOne(BaseVersionByProjectIdAndRevision(creationInfo.Project.Identifier, creationInfo.Version.Revision))
		if err != nil {
			return createTime, errors.Wrap(err, "finding base version for patch version")
		}
		if baseVersion == nil {
			// The database data may be incomplete and missing the base Version
			// In that case we don't want to fail, we fallback to the patch version's CreateTime.
			return creationInfo.Version.CreateTime, nil
		}
		return baseVersion.CreateTime, nil
	} else {
		return creationInfo.Version.CreateTime, nil
	}
}

// createOneTask is a helper to create a single task.
func createOneTask(id string, creationInfo TaskCreationInfo, buildVarTask BuildVariantTaskUnit) (*task.Task, error) {
	activateTask := creationInfo.Build.Activated && !creationInfo.ActivationInfo.taskHasSpecificActivation(creationInfo.Build.BuildVariant, buildVarTask.Name)
	isStepback := creationInfo.ActivationInfo.isStepbackTask(creationInfo.Build.BuildVariant, buildVarTask.Name)

	buildVarTask.RunOn = creationInfo.DistroAliases.Expand(buildVarTask.RunOn)
	creationInfo.BuildVariant.RunOn = creationInfo.DistroAliases.Expand(creationInfo.BuildVariant.RunOn)

	activatedTime := utility.ZeroTime
	if activateTask {
		activatedTime = time.Now()
	}

	isGithubCheck := false
	if len(creationInfo.GithubChecksAliases) > 0 {
		var err error
		name, tags, ok := creationInfo.Project.GetTaskNameAndTags(buildVarTask)
		if ok {
			isGithubCheck, err = creationInfo.GithubChecksAliases.HasMatchingTask(name, tags)
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error checking if task matches aliases",
				"version": creationInfo.Version.Id,
				"task":    buildVarTask.Name,
				"variant": buildVarTask.Variant,
			}))
		}
	}

	t := &task.Task{
		Id:                      id,
		Secret:                  utility.RandomString(),
		DisplayName:             buildVarTask.Name,
		BuildId:                 creationInfo.Build.Id,
		BuildVariant:            creationInfo.BuildVariant.Name,
		BuildVariantDisplayName: creationInfo.BuildVariant.DisplayName,
		CreateTime:              creationInfo.TaskCreateTime,
		IngestTime:              time.Now(),
		ScheduledTime:           utility.ZeroTime,
		StartTime:               utility.ZeroTime, // Certain time fields must be initialized
		FinishTime:              utility.ZeroTime, // to our own utility.ZeroTime value (which is
		DispatchTime:            utility.ZeroTime, // Unix epoch 0, not Go's time.Time{})
		LastHeartbeat:           utility.ZeroTime,
		Status:                  evergreen.TaskUndispatched,
		Activated:               activateTask,
		ActivatedTime:           activatedTime,
		RevisionOrderNumber:     creationInfo.Version.RevisionOrderNumber,
		Requester:               creationInfo.Version.Requester,
		ParentPatchID:           creationInfo.Build.ParentPatchID,
		StepbackInfo:            &task.StepbackInfo{},
		ParentPatchNumber:       creationInfo.Build.ParentPatchNumber,
		Version:                 creationInfo.Version.Id,
		Revision:                creationInfo.Version.Revision,
		Project:                 creationInfo.Project.Identifier,
		Priority:                buildVarTask.Priority,
		GenerateTask:            creationInfo.Project.IsGenerateTask(buildVarTask.Name),
		TriggerID:               creationInfo.Version.TriggerID,
		TriggerType:             creationInfo.Version.TriggerType,
		TriggerEvent:            creationInfo.Version.TriggerEvent,
		CommitQueueMerge:        buildVarTask.CommitQueueMerge,
		IsGithubCheck:           isGithubCheck,
		DisplayTaskId:           utility.ToStringPtr(""), // this will be overridden if the task is an execution task
		IsEssentialToSucceed:    creationInfo.ActivatedTasksAreEssentialToSucceed && activateTask,
	}

	projectTask := creationInfo.Project.FindProjectTask(buildVarTask.Name)
	if projectTask != nil {
		t.MustHaveResults = utility.FromBoolPtr(projectTask.MustHaveResults)
	}

	t.ExecutionPlatform = shouldRunOnContainer(buildVarTask.RunOn, creationInfo.BuildVariant.RunOn, creationInfo.Project.Containers)
	if t.IsContainerTask() {
		var err error
		t.Container, err = getContainerFromRunOn(id, buildVarTask, creationInfo.BuildVariant)
		if err != nil {
			return nil, err
		}
		opts, err := getContainerOptions(creationInfo, t.Container)
		if err != nil {
			return nil, errors.Wrap(err, "getting container options")
		}
		t.ContainerOpts = *opts
	} else {
		distroID, secondaryDistros, err := getDistrosFromRunOn(id, buildVarTask, creationInfo.BuildVariant)
		if err != nil {
			return nil, err
		}
		t.DistroId = distroID
		t.SecondaryDistros = secondaryDistros
	}

	if isStepback {
		t.ActivatedBy = evergreen.StepbackTaskActivator
	} else if t.Activated {
		t.ActivatedBy = creationInfo.Version.Author
	}

	if buildVarTask.IsPartOfGroup {
		tg := buildVarTask.TaskGroup
		if tg == nil {
			tg = creationInfo.Project.FindTaskGroup(buildVarTask.GroupName)
		}
		if tg == nil {
			return nil, errors.Errorf("finding task group '%s' in project '%s'", buildVarTask.GroupName, creationInfo.Project.Identifier)
		}

		tg.InjectInfo(t)
	}

	return t, nil
}

func getDistrosFromRunOn(id string, buildVarTask BuildVariantTaskUnit, buildVariant *BuildVariant) (string, []string, error) {
	if len(buildVarTask.RunOn) > 0 {
		secondaryDistros := []string{}
		distroID := buildVarTask.RunOn[0]
		if len(buildVarTask.RunOn) > 1 {
			secondaryDistros = buildVarTask.RunOn[1:]
		}
		return distroID, secondaryDistros, nil
	} else if len(buildVariant.RunOn) > 0 {
		secondaryDistros := []string{}
		distroID := buildVariant.RunOn[0]
		if len(buildVariant.RunOn) > 1 {
			secondaryDistros = buildVariant.RunOn[1:]
		}
		return distroID, secondaryDistros, nil
	}
	return "", nil, errors.Errorf("task '%s' is not runnable as there is no distro specified", id)
}

func shouldRunOnContainer(taskRunOn, buildVariantRunOn []string, containers []Container) task.ExecutionPlatform {
	containerNameMap := map[string]bool{}
	for _, container := range containers {
		containerNameMap[container.Name] = true
	}
	var runOn []string
	if len(taskRunOn) > 0 {
		runOn = taskRunOn
	} else {
		runOn = buildVariantRunOn
	}
	for _, r := range runOn {
		if containerNameMap[r] {
			return task.ExecutionPlatformContainer
		}
	}
	return task.ExecutionPlatformHost
}

func getContainerFromRunOn(id string, buildVarTask BuildVariantTaskUnit, buildVariant *BuildVariant) (string, error) {
	var container string

	if len(buildVarTask.RunOn) > 0 {
		container = buildVarTask.RunOn[0]
	} else if len(buildVariant.RunOn) > 0 {
		container = buildVariant.RunOn[0]
	} else {
		return "", errors.Errorf("task '%s' on buildvariant '%s' is not runnable as there is no container specified", id, buildVariant.Name)
	}
	return container, nil
}

// getContainerOptions resolves the task's container configuration based on the
// task's container name and the container definitions available to the project.
func getContainerOptions(creationInfo TaskCreationInfo, container string) (*task.ContainerOptions, error) {
	for _, c := range creationInfo.Project.Containers {
		if c.Name != container {
			continue
		}

		opts := task.ContainerOptions{
			WorkingDir:     c.WorkingDir,
			Image:          c.Image,
			RepoCredsName:  c.Credential,
			OS:             c.System.OperatingSystem,
			Arch:           c.System.CPUArchitecture,
			WindowsVersion: c.System.WindowsVersion,
		}

		if c.Resources != nil {
			opts.CPU = c.Resources.CPU
			opts.MemoryMB = c.Resources.MemoryMB
			return &opts, nil
		}

		var containerSize *ContainerResources
		for _, size := range creationInfo.ProjectRef.ContainerSizeDefinitions {
			if size.Name == c.Size {
				containerSize = &size
				break
			}
		}
		if containerSize == nil {
			return nil, errors.Errorf("container size '%s' not found", c.Size)
		}

		opts.CPU = containerSize.CPU
		opts.MemoryMB = containerSize.MemoryMB
		return &opts, nil
	}

	return nil, errors.Errorf("definition for container '%s' not found", container)
}

func createDisplayTask(id string, creationInfo TaskCreationInfo, displayName string, execTasks []string, createTime time.Time, displayTaskActivated bool) (*task.Task, error) {

	activatedTime := utility.ZeroTime
	if displayTaskActivated {
		activatedTime = time.Now()
	}

	t := &task.Task{
		Id:                      id,
		DisplayName:             displayName,
		BuildVariant:            creationInfo.BuildVariant.Name,
		BuildVariantDisplayName: creationInfo.BuildVariant.DisplayName,
		BuildId:                 creationInfo.Build.Id,
		CreateTime:              createTime,
		RevisionOrderNumber:     creationInfo.Version.RevisionOrderNumber,
		Version:                 creationInfo.Version.Id,
		Revision:                creationInfo.Version.Revision,
		Project:                 creationInfo.Project.Identifier,
		Requester:               creationInfo.Version.Requester,
		ParentPatchID:           creationInfo.Build.ParentPatchID,
		ParentPatchNumber:       creationInfo.Build.ParentPatchNumber,
		DisplayOnly:             true,
		ExecutionTasks:          execTasks,
		Status:                  evergreen.TaskUndispatched,
		IngestTime:              time.Now(),
		StartTime:               utility.ZeroTime,
		FinishTime:              utility.ZeroTime,
		Activated:               displayTaskActivated,
		ActivatedTime:           activatedTime,
		DispatchTime:            utility.ZeroTime,
		ScheduledTime:           utility.ZeroTime,
		TriggerID:               creationInfo.Version.TriggerID,
		TriggerType:             creationInfo.Version.TriggerType,
		TriggerEvent:            creationInfo.Version.TriggerEvent,
		DisplayTaskId:           utility.ToStringPtr(""),
	}
	return t, nil
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

// Given a patch version and a list of variant/task pairs, creates the set of new builds that
// do not exist yet out of the set of pairs. No tasks are added for builds which already exist
// (see AddNewTasksForPatch). New builds/tasks are activated depending on their batchtime.
// Returns activated task IDs.
func addNewBuilds(ctx context.Context, creationInfo TaskCreationInfo, existingBuilds []build.Build) ([]string, error) {
	ctx, span := tracer.Start(ctx, "add-new-builds")
	defer span.End()
	taskIdTables, err := getTaskIdConfig(creationInfo)
	if err != nil {
		return nil, errors.Wrap(err, "making task ID table")
	}

	newBuildIds := make([]string, 0)
	newActivatedTaskIds := make([]string, 0)
	newBuildStatuses := make([]VersionBuildStatus, 0)

	variantsProcessed := map[string]bool{}
	for _, b := range existingBuilds {
		variantsProcessed[b.BuildVariant] = true
	}

	createTime, err := getTaskCreateTime(creationInfo)
	if err != nil {
		return nil, errors.Wrap(err, "getting create time for tasks")
	}
	batchTimeCatcher := grip.NewBasicCatcher()
	for _, pair := range creationInfo.Pairs.ExecTasks {
		if _, ok := variantsProcessed[pair.Variant]; ok { // skip variant that was already processed
			continue
		}
		variantsProcessed[pair.Variant] = true
		// Extract the unique set of task names for the variant we're about to create
		taskNames := creationInfo.Pairs.ExecTasks.TaskNames(pair.Variant)
		displayNames := creationInfo.Pairs.DisplayTasks.TaskNames(pair.Variant)
		activateVariant := !creationInfo.ActivationInfo.variantHasSpecificActivation(pair.Variant)
		buildCreationArgs := TaskCreationInfo{
			Project:                             creationInfo.Project,
			ProjectRef:                          creationInfo.ProjectRef,
			Version:                             creationInfo.Version,
			TaskIDs:                             taskIdTables,
			BuildVariantName:                    pair.Variant,
			ActivateBuild:                       activateVariant,
			TaskNames:                           taskNames,
			DisplayNames:                        displayNames,
			ActivationInfo:                      creationInfo.ActivationInfo,
			GeneratedBy:                         creationInfo.GeneratedBy,
			TaskCreateTime:                      createTime,
			SyncAtEndOpts:                       creationInfo.SyncAtEndOpts,
			ActivatedTasksAreEssentialToSucceed: creationInfo.ActivatedTasksAreEssentialToSucceed,
		}

		grip.Info(message.Fields{
			"op":        "creating build for version",
			"variant":   pair.Variant,
			"activated": activateVariant,
			"version":   creationInfo.Version.Id,
		})
		build, tasks, err := CreateBuildFromVersionNoInsert(buildCreationArgs)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if len(tasks) == 0 {
			grip.Info(message.Fields{
				"op":        "skipping empty build for version",
				"variant":   pair.Variant,
				"activated": activateVariant,
				"version":   creationInfo.Version.Id,
			})
			continue
		}

		if err = build.Insert(); err != nil {
			return nil, errors.Wrapf(err, "inserting build '%s'", build.Id)
		}
		if err = tasks.InsertUnordered(ctx); err != nil {
			return nil, errors.Wrapf(err, "inserting tasks for build '%s'", build.Id)
		}
		newBuildIds = append(newBuildIds, build.Id)

		batchTimeTasksToIds := map[string]string{}
		for _, t := range tasks {
			if t.Activated {
				newActivatedTaskIds = append(newActivatedTaskIds, t.Id)
			}
			if evergreen.ShouldConsiderBatchtime(t.Requester) && creationInfo.ActivationInfo.taskHasSpecificActivation(t.BuildVariant, t.DisplayName) {
				batchTimeTasksToIds[t.DisplayName] = t.Id
			}
		}

		var activateVariantAt time.Time
		batchTimeTaskStatuses := []BatchTimeTaskStatus{}
		if !activateVariant {
			activateVariantAt, err = creationInfo.ProjectRef.GetActivationTimeForVariant(creationInfo.Project.FindBuildVariant(pair.Variant))
			batchTimeCatcher.Wrapf(err, "getting activation time for variant '%s'", pair.Variant)
		}
		for taskName, id := range batchTimeTasksToIds {
			activateTaskAt, err := creationInfo.ProjectRef.GetActivationTimeForTask(
				creationInfo.Project.FindTaskForVariant(taskName, pair.Variant))
			batchTimeCatcher.Wrapf(err, "getting activation time for task '%s' in variant '%s'", taskName, pair.Variant)
			batchTimeTaskStatuses = append(batchTimeTaskStatuses, BatchTimeTaskStatus{
				TaskId:   id,
				TaskName: taskName,
				ActivationStatus: ActivationStatus{
					ActivateAt: activateTaskAt,
				},
			})
		}
		newBuildStatuses = append(newBuildStatuses,
			VersionBuildStatus{
				BuildVariant:   pair.Variant,
				BuildId:        build.Id,
				BatchTimeTasks: batchTimeTaskStatuses,
				ActivationStatus: ActivationStatus{
					Activated:  activateVariant,
					ActivateAt: activateVariantAt,
				},
			},
		)
	}

	grip.Error(message.WrapError(batchTimeCatcher.Resolve(), message.Fields{
		"message": "unable to get all activation times",
		"runner":  "addNewBuilds",
		"version": creationInfo.Version.Id,
	}))

	return newActivatedTaskIds, errors.WithStack(VersionUpdateOne(
		bson.M{VersionIdKey: creationInfo.Version.Id},
		bson.M{
			"$push": bson.M{
				VersionBuildIdsKey:      bson.M{"$each": newBuildIds},
				VersionBuildVariantsKey: bson.M{"$each": newBuildStatuses},
			},
		},
	))
}

// Given a version and set of variant/task pairs, creates any tasks that don't exist yet,
// within the set of already existing builds. Returns activated task IDs.
func addNewTasksToExistingBuilds(ctx context.Context, creationInfo TaskCreationInfo, existingBuilds []build.Build, caller string) ([]string, error) {
	ctx, span := tracer.Start(ctx, "add-new-tasks")
	defer span.End()
	if creationInfo.Version.BuildIds == nil {
		return nil, nil
	}
	distroAliases, err := distro.NewDistroAliasesLookupTable(ctx)
	if err != nil {
		return nil, err
	}

	taskIdTables, err := getTaskIdConfig(creationInfo)
	if err != nil {
		return nil, errors.Wrap(err, "getting table of task IDs")
	}

	activatedTaskIds := []string{}
	activatedTasks := []task.Task{}
	var buildIdsToActivate []string
	for _, b := range existingBuilds {
		wasActivated := b.Activated
		// Find the set of task names that already exist for the given build, including display tasks.
		tasksInBuild, err := task.FindAll(db.Query(task.ByBuildId(b.Id)).WithFields(task.DisplayNameKey, task.ActivatedKey))
		if err != nil {
			return nil, err
		}
		existingTasksIndex := map[string]bool{}
		hasActivatedTask := false
		for _, t := range tasksInBuild {
			if t.Activated {
				hasActivatedTask = true
			}
			existingTasksIndex[t.DisplayName] = true
		}
		projectBV := creationInfo.Project.FindBuildVariant(b.BuildVariant)
		if projectBV != nil && hasActivatedTask {
			b.Activated = utility.FromBoolTPtr(projectBV.Activate)
		}

		// Build a list of tasks that haven't been created yet for the given variant, but have
		// a record in the TVPairSet indicating that it should exist
		tasksToAdd := []string{}
		for _, taskName := range creationInfo.Pairs.ExecTasks.TaskNames(b.BuildVariant) {
			if ok := existingTasksIndex[taskName]; ok {
				continue
			}
			tasksToAdd = append(tasksToAdd, taskName)
		}
		displayTasksToAdd := []string{}
		for _, taskName := range creationInfo.Pairs.DisplayTasks.TaskNames(b.BuildVariant) {
			if ok := existingTasksIndex[taskName]; ok {
				continue
			}
			displayTasksToAdd = append(displayTasksToAdd, taskName)
		}
		if len(tasksToAdd) == 0 && len(displayTasksToAdd) == 0 { // no tasks to add, so we do nothing.
			continue
		}
		// Add the new set of tasks to the build.
		creationInfo.Build = &b
		creationInfo.TasksInBuild = tasksInBuild
		creationInfo.TaskIDs = taskIdTables
		creationInfo.TaskNames = tasksToAdd
		creationInfo.DisplayNames = displayTasksToAdd
		creationInfo.DistroAliases = distroAliases
		_, tasks, err := addTasksToBuild(ctx, creationInfo)
		if err != nil {
			return nil, err
		}

		for _, t := range tasks {
			if t.Activated {
				activatedTaskIds = append(activatedTaskIds, t.Id)
				activatedTasks = append(activatedTasks, *t)
				b.Activated = true
			}
			if t.Activated && creationInfo.ActivationInfo.isStepbackTask(t.BuildVariant, t.DisplayName) {
				event.LogTaskActivated(t.Id, t.Execution, evergreen.StepbackTaskActivator)
			}
		}
		// update build activation status if tasks have since been activated
		if !wasActivated && b.Activated {
			buildIdsToActivate = append(buildIdsToActivate, b.Id)
		}
	}
	if len(buildIdsToActivate) > 0 {
		if err := build.UpdateActivation(buildIdsToActivate, true, caller); err != nil {
			return nil, err
		}
	}
	if creationInfo.ActivationInfo.hasActivationTasks() {
		if err = creationInfo.Version.ActivateAndSetBuildVariants(); err != nil {
			return nil, errors.Wrap(err, "activating version and adding batchtime tasks")
		}
	} else {
		if err = creationInfo.Version.SetActivated(true); err != nil {
			return nil, errors.Wrap(err, "setting version activation to true")
		}
	}

	activatedTaskDependencies, err := task.GetRecursiveDependenciesUp(activatedTasks, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting dependencies for activated tasks")
	}
	if err = task.ActivateTasks(activatedTaskDependencies, time.Now(), true, evergreen.User); err != nil {
		return nil, errors.Wrap(err, "activating existing dependencies for new tasks")
	}

	return activatedTaskIds, nil
}

// activateExistingInactiveTasks will find existing inactive tasks in the patch that need to be activated as
// part of the patch re-configuration.
func activateExistingInactiveTasks(ctx context.Context, creationInfo TaskCreationInfo, existingBuilds []build.Build, caller string) error {
	existingTasksToActivate := []task.Task{}
	for _, b := range existingBuilds {
		tasksInBuild, err := task.FindAll(db.Query(task.ByBuildId(b.Id)).WithFields(task.DisplayNameKey, task.ActivatedKey, task.BuildIdKey, task.VersionKey))
		if err != nil {
			return err
		}
		existingTasksIndex := map[string]task.Task{}
		for i := range tasksInBuild {
			existingTasksIndex[tasksInBuild[i].DisplayName] = tasksInBuild[i]
		}
		execAndDisplayTasks := append(creationInfo.Pairs.ExecTasks.TaskNames(b.BuildVariant), creationInfo.Pairs.DisplayTasks.TaskNames(b.BuildVariant)...)
		for _, taskName := range execAndDisplayTasks {
			if t, ok := existingTasksIndex[taskName]; ok && !t.Activated {
				existingTasksToActivate = append(existingTasksToActivate, t)
			}
		}
	}
	if len(existingTasksToActivate) > 0 {
		if err := SetActiveState(ctx, caller, true, existingTasksToActivate...); err != nil {
			return errors.Wrap(err, "setting tasks to active")
		}
	}
	return nil
}

// getTaskIdConfig takes the pre-determined set of task IDs and combines it with
// new task IDs for the task-variant pairs to be created. If there are duplicate
// task-variant pairs, the new task-variant pairs will overwrite the existing
// ones.
func getTaskIdConfig(creationInfo TaskCreationInfo) (TaskIdConfig, error) {
	// The table should include only new and existing tasks
	taskIdTable, err := NewTaskIdConfig(creationInfo.Project, creationInfo.Version, creationInfo.Pairs, creationInfo.ProjectRef.Identifier)
	if err != nil {
		return TaskIdConfig{}, errors.Wrap(err, "creating patch's task ID table")
	}
	existingTasks, err := task.FindAll(db.Query(task.ByVersion(creationInfo.Version.Id)).WithFields(task.DisplayOnlyKey, task.DisplayNameKey, task.BuildVariantKey))
	if err != nil {
		return TaskIdConfig{}, errors.Wrap(err, "getting existing task IDs")
	}
	for _, t := range existingTasks {
		if t.DisplayOnly {
			taskIdTable.DisplayTasks.AddId(t.BuildVariant, t.DisplayName, t.Id)
		} else {
			taskIdTable.ExecutionTasks.AddId(t.BuildVariant, t.DisplayName, t.Id)
		}
	}

	// Merge the pre-determined task IDs with the new ones that were just
	// created.
	mergedTaskIDTable := creationInfo.TaskIDs

	if len(mergedTaskIDTable.ExecutionTasks) == 0 && len(mergedTaskIDTable.DisplayTasks) == 0 {
		return taskIdTable, nil
	}

	if len(mergedTaskIDTable.ExecutionTasks) == 0 {
		mergedTaskIDTable.ExecutionTasks = TaskIdTable{}
	}
	if len(mergedTaskIDTable.DisplayTasks) == 0 {
		mergedTaskIDTable.DisplayTasks = TaskIdTable{}
	}

	for k, v := range taskIdTable.ExecutionTasks {
		mergedTaskIDTable.ExecutionTasks.AddId(k.Variant, k.TaskName, v)
	}
	for k, v := range taskIdTable.DisplayTasks {
		mergedTaskIDTable.DisplayTasks.AddId(k.Variant, k.TaskName, v)
	}

	return mergedTaskIDTable, nil
}
