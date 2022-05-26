package model

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/global"
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
	builds, err := build.Find(
		build.ByVersion(versionId).WithFields(build.IdKey),
	)
	if err != nil {
		return errors.Wrapf(err, "getting builds for version '%s'", versionId)
	}
	buildIDs := make([]string, 0, len(builds))
	for _, build := range builds {
		buildIDs = append(buildIDs, build.Id)
	}

	// Update activation for all builds before updating their tasks so the version won't spend
	// time in an intermediate state where only some builds are updated
	if err = build.UpdateActivation(buildIDs, active, caller); err != nil {
		return errors.Wrapf(err, "setting activation for builds in version '%s'", versionId)
	}

	return errors.Wrapf(setTaskActivationForBuilds(buildIDs, active, false, nil, caller),
		"setting activation for tasks in version '%s'", versionId)
}

// SetBuildActivation updates the "active" state of this build and all associated tasks.
// It also updates the task cache for the build document.
func SetBuildActivation(buildId string, active bool, caller string) error {
	if err := build.UpdateActivation([]string{buildId}, active, caller); err != nil {
		return errors.Wrapf(err, "setting build activation to %t for build '%s'", active, buildId)
	}

	return errors.Wrapf(setTaskActivationForBuilds([]string{buildId}, active, true, nil, caller),
		"setting task activation for build '%s'", buildId)
}

// setTaskActivationForBuilds updates the "active" state of all tasks in buildIds.
// It also updates the task cache for the build document.
// If withDependencies is true, also set dependencies. Don't need to do this when the entire version is affected.
// If tasks are given to ignore, then we don't activate those tasks.
func setTaskActivationForBuilds(buildIds []string, active, withDependencies bool, ignoreTasks []string, caller string) error {
	// If activating a task, set the ActivatedBy field to be the caller
	if active {
		q := bson.M{
			task.BuildIdKey: bson.M{"$in": buildIds},
			task.StatusKey:  evergreen.TaskUndispatched,
		}
		if len(ignoreTasks) > 0 {
			q[task.IdKey] = bson.M{"$nin": ignoreTasks}
		}
		tasksToActivate, err := task.FindAll(db.Query(q).WithFields(task.IdKey, task.DependsOnKey, task.ExecutionKey))
		if err != nil {
			return errors.Wrap(err, "getting tasks to deactivate")
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

	return errors.Wrapf(task.AbortBuild(buildId, task.AbortInfo{User: caller}), "aborting tasks for build '%s'", buildId)
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
func SetTaskPriority(t task.Task, priority int64, caller string) error {
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
	}).WithFields(ExecutionKey)
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
		if err = t.DeactivateTask(caller); err != nil {
			return errors.Wrap(err, "deactivating task")
		}
	}

	return nil
}

// SetBuildPriority updates the priority field of all tasks associated with the given build id.
func SetBuildPriority(buildId string, priority int64, caller string) error {
	_, err := task.UpdateAll(
		bson.M{task.BuildIdKey: buildId},
		bson.M{"$set": bson.M{task.PriorityKey: priority}},
	)
	if err != nil {
		return errors.Wrapf(err, "setting priority for build '%s'", buildId)
	}

	// negative priority - these tasks should never run, so unschedule now
	if priority < 0 {
		tasks, err := task.FindAll(db.Query(bson.M{task.BuildIdKey: buildId}).WithFields(task.IdKey, task.ExecutionKey))
		if err != nil {
			return errors.Wrapf(err, "getting tasks for build '%s'", buildId)
		}
		if err = task.DeactivateTasks(tasks, true, caller); err != nil {
			return errors.Wrapf(err, "deactivating tasks for build '%s'", buildId)
		}
	}

	return nil
}

// SetVersionPriority updates the priority field of all tasks associated with the given version id.
func SetVersionPriority(versionId string, priority int64, caller string) error {
	_, err := task.UpdateAll(
		bson.M{task.VersionKey: versionId},
		bson.M{"$set": bson.M{task.PriorityKey: priority}},
	)
	if err != nil {
		return errors.Wrapf(err, "setting priority for version '%s'", versionId)
	}

	// negative priority - these tasks should never run, so unschedule now
	if priority < 0 {
		var tasks []task.Task
		tasks, err = task.FindAll(db.Query(bson.M{task.VersionKey: versionId}).WithFields(task.IdKey, task.ExecutionKey))
		if err != nil {
			return errors.Wrapf(err, "getting tasks for version '%s'", versionId)
		}
		err = task.DeactivateTasks(tasks, false, caller)
		if err != nil {
			return errors.Wrapf(err, "deactivating tasks for version '%s'", versionId)
		}
	}

	return nil
}

// RestartTasksInVersion restarts completed tasks associated with a given versionId.
// If abortInProgress is true, it also sets the abort flag on any in-progress tasks. In addition, it
// updates all builds containing the tasks affected.
func RestartTasksInVersion(versionId string, abortInProgress bool, caller string) error {
	tasks, err := task.Find(task.ByVersion(versionId))
	if err != nil {
		return errors.Wrap(err, "error finding tasks in version")
	}
	if tasks == nil {
		return errors.New("no tasks found for version")
	}
	var taskIds []string
	for _, task := range tasks {
		taskIds = append(taskIds, task.Id)
	}

	toRestart := VersionToRestart{VersionId: &versionId, TaskIds: taskIds}
	return RestartVersions([]*VersionToRestart{&toRestart}, abortInProgress, caller)
}

// RestartVersion restarts completed tasks associated with a versionId.
// If abortInProgress is true, it also sets the abort flag on any in-progress tasks.
func RestartVersion(versionId string, taskIds []string, abortInProgress bool, caller string) error {
	if abortInProgress {
		if err := task.AbortTasksForVersion(versionId, taskIds, caller); err != nil {
			return errors.WithStack(err)
		}
	}
	finishedTasks, err := task.FindAll(db.Query(task.ByIdsAndStatus(taskIds, evergreen.TaskCompletedStatuses)))
	if err != nil {
		return errors.WithStack(err)
	}
	allFinishedTasks, err := task.AddParentDisplayTasks(finishedTasks)
	if err != nil {
		return errors.WithStack(err)
	}
	// remove execution tasks in case the caller passed both display and execution tasks
	// the functions below are expected to work if just the display task is passed
	for i := len(allFinishedTasks) - 1; i >= 0; i-- {
		t := allFinishedTasks[i]
		if t.DisplayTask != nil {
			allFinishedTasks = append(allFinishedTasks[:i], allFinishedTasks[i+1:]...)
		}
	}

	// archive all the finished tasks
	toArchive := []task.Task{}
	for _, t := range allFinishedTasks {
		if !t.IsPartOfSingleHostTaskGroup() { // for single host task groups we don't archive until fully restarting
			toArchive = append(toArchive, t)
		}
	}
	if err = task.ArchiveMany(toArchive); err != nil {
		return errors.Wrap(err, "archiving tasks")
	}

	type taskGroupAndBuild struct {
		Build     string
		TaskGroup string
	}
	// Mark aborted tasks to reset when finished if not all tasks are finished.
	if abortInProgress && len(finishedTasks) < len(taskIds) {
		if err = task.SetAbortedTasksResetWhenFinished(taskIds); err != nil {
			return err
		}
	}

	// only need to check one task per task group / build combination
	taskGroupsToCheck := map[taskGroupAndBuild]task.Task{}
	tasksToRestart := allFinishedTasks

	restartIds := []string{}
	for _, t := range tasksToRestart {
		if t.IsPartOfSingleHostTaskGroup() {
			if err = t.SetResetWhenFinished(); err != nil {
				return errors.Wrapf(err, "marking '%s' for restart when finished", t.Id)
			}
			taskGroupsToCheck[taskGroupAndBuild{
				Build:     t.BuildId,
				TaskGroup: t.TaskGroup,
			}] = t
		} else {
			// only hard restart non-single host task group tasks
			restartIds = append(restartIds, t.Id)
			if t.DisplayOnly {
				restartIds = append(restartIds, t.ExecutionTasks...)
			}
		}
	}

	for tg, t := range taskGroupsToCheck {
		if err = checkResetSingleHostTaskGroup(&t, caller); err != nil {
			return errors.Wrapf(err, "resetting task group '%s' for build '%s'", tg.TaskGroup, tg.Build)
		}
	}

	// Set all the task fields to indicate restarted
	if err = MarkTasksReset(restartIds); err != nil {
		return errors.WithStack(err)
	}
	for _, t := range tasksToRestart {
		if !t.IsPartOfSingleHostTaskGroup() { // this will be logged separately if task group is restarted
			event.LogTaskRestarted(t.Id, t.Execution, caller)
		}
	}
	if err = build.SetBuildStartedForTasks(tasksToRestart, caller); err != nil {
		return errors.Wrap(err, "setting builds started")
	}
	version, err := VersionFindOneId(versionId)
	if err != nil {
		return errors.Wrap(err, "finding version")
	}
	return errors.Wrap(version.UpdateStatus(evergreen.VersionStarted), "changing version status")

}

// RestartVersions restarts selected tasks for a set of versions.
// If abortInProgress is true for any version, it also sets the abort flag on any in-progress tasks.
func RestartVersions(versionsToRestart []*VersionToRestart, abortInProgress bool, caller string) error {
	catcher := grip.NewBasicCatcher()
	for _, t := range versionsToRestart {
		err := RestartVersion(*t.VersionId, t.TaskIds, abortInProgress, caller)
		catcher.Wrapf(err, "restarting tasks for version '%s'", *t.VersionId)
	}
	return errors.Wrap(catcher.Resolve(), "restarting tasks")
}

// RestartBuild restarts completed tasks associated with a given buildId.
// If abortInProgress is true, it also sets the abort flag on any in-progress tasks.
func RestartBuild(buildId string, taskIds []string, abortInProgress bool, caller string) error {
	if abortInProgress {
		// abort in-progress tasks in this build
		if err := task.AbortTasksForBuild(buildId, taskIds, caller); err != nil {
			return errors.WithStack(err)
		}
	}

	// restart all the 'not in-progress' tasks for the build
	tasks, err := task.FindAll(db.Query(task.ByIdsAndStatus(taskIds, evergreen.TaskCompletedStatuses)))
	if err != nil {
		return errors.WithStack(err)
	}
	if len(tasks) == 0 {
		return nil
	}
	return restartTasksForBuild(buildId, tasks, caller)
}

// RestartAllBuildTasks restarts all the tasks associated with a given build.
func RestartAllBuildTasks(buildId string, caller string) error {
	if err := task.AbortTasksForBuild(buildId, nil, caller); err != nil {
		return errors.WithStack(err)
	}

	allTasks, err := task.FindAll(db.Query(task.ByBuildId(buildId)))
	if err != nil {
		return errors.WithStack(err)
	}
	if len(allTasks) == 0 {
		return nil
	}
	return restartTasksForBuild(buildId, allTasks, caller)
}

func restartTasksForBuild(buildId string, tasks []task.Task, caller string) error {
	// maps task group to a single task in the group so we only check once
	taskGroupsToCheck := map[string]task.Task{}
	restartIds := []string{}
	toArchive := []task.Task{}
	for _, t := range tasks {
		if t.IsPartOfSingleHostTaskGroup() {
			if err := t.SetResetWhenFinished(); err != nil {
				return errors.Wrapf(err, "marking task group '%s' to reset", t.TaskGroup)
			}
			taskGroupsToCheck[t.TaskGroup] = t
		} else {
			restartIds = append(restartIds, t.Id)
			if t.DisplayOnly {
				restartIds = append(restartIds, t.ExecutionTasks...)
			}
			if t.IsFinished() {
				toArchive = append(toArchive, t)
			}
		}
	}
	if err := task.ArchiveMany(toArchive); err != nil {
		return errors.Wrap(err, "archiving tasks")
	}
	// Set all the task fields to indicate restarted
	if err := MarkTasksReset(restartIds); err != nil {
		return errors.WithStack(err)
	}
	for _, t := range tasks {
		if !t.IsPartOfSingleHostTaskGroup() { // this will be logged separately if task group is restarted
			event.LogTaskRestarted(t.Id, t.Execution, caller)
		}
	}

	for tg, t := range taskGroupsToCheck {
		if err := checkResetSingleHostTaskGroup(&t, caller); err != nil {
			return errors.Wrapf(err, "resetting single host task group '%s'", tg)
		}
	}

	return errors.Wrap(build.SetBuildStartedForTasks(tasks, caller), "setting builds started")
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

// addTasksToBuild creates/activates the tasks for the given build of a project
func addTasksToBuild(ctx context.Context, b *build.Build, project *Project, pRef *ProjectRef, v *Version, taskNames []string,
	displayNames []string, activationInfo specificActivationInfo, generatedBy string, tasksInBuild []task.Task,
	syncAtEndOpts patch.SyncAtEndOptions, distroAliases map[string][]string, taskIds TaskIdConfig) (*build.Build, task.Tasks, error) {
	// find the build variant for this project/build
	buildVariant := project.FindBuildVariant(b.BuildVariant)
	if buildVariant == nil {
		return nil, nil, errors.Errorf("finding build '%s' in project file '%s'",
			b.BuildVariant, project.Identifier)
	}

	// create the new tasks for the build
	createTime, err := getTaskCreateTime(project.Identifier, v)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "getting create time for tasks in version '%s'", v.Id)
	}

	var githubCheckAliases ProjectAliases
	if v.Requester == evergreen.RepotrackerVersionRequester && pRef.IsGithubChecksEnabled() {
		githubCheckAliases, err = FindAliasInProjectRepoOrConfig(v.Identifier, evergreen.GithubChecksAlias)
		grip.Error(message.WrapError(err, message.Fields{
			"message":            "error getting github check aliases when adding tasks to build",
			"project":            v.Identifier,
			"project_identifier": pRef.Identifier,
			"version":            v.Id,
		}))
	}
	tasks, err := createTasksForBuild(project, pRef, buildVariant, b, v, taskIds, taskNames, displayNames, activationInfo,
		generatedBy, tasksInBuild, syncAtEndOpts, distroAliases, createTime, githubCheckAliases)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "creating tasks for build '%s'", b.Id)
	}

	if err = tasks.InsertUnordered(ctx); err != nil {
		return nil, nil, errors.Wrapf(err, "inserting tasks for build '%s'", b.Id)
	}

	for _, t := range tasks {
		if t.IsGithubCheck {
			if err = b.SetIsGithubCheck(); err != nil {
				return nil, nil, errors.Wrapf(err, "setting build '%s' as a GitHub check", b.Id)
			}
			break
		}
	}

	// update the build to hold the new tasks
	if err = RefreshTasksCache(b.Id); err != nil {
		return nil, nil, errors.Wrapf(err, "updating task cache for '%s'", b.Id)
	}

	batchTimeTaskStatuses := []BatchTimeTaskStatus{}
	tasksWithActivationTime := activationInfo.getActivationTasks(b.BuildVariant)
	batchTimeCatcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		if !utility.StringSliceContains(tasksWithActivationTime, t.DisplayName) {
			continue
		}
		activateTaskAt, err := pRef.GetActivationTimeForTask(project.FindTaskForVariant(t.DisplayName, b.BuildVariant))
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
	for i, status := range v.BuildVariants {
		if status.BuildVariant != b.BuildVariant {
			continue
		}
		v.BuildVariants[i].BatchTimeTasks = append(v.BuildVariants[i].BatchTimeTasks, batchTimeTaskStatuses...)
	}
	grip.Error(message.WrapError(batchTimeCatcher.Resolve(), message.Fields{
		"message": "unable to get activation time for tasks",
		"variant": b.BuildVariant,
		"runner":  "addTasksToBuild",
		"version": v.Id,
	}))

	return b, tasks, nil
}

// BuildCreateArgs is the set of parameters used in CreateBuildFromVersionNoInsert.
type BuildCreateArgs struct {
	Project             Project                 // project to create the build for
	ProjectRef          ProjectRef              // project ref associated with the build
	Version             Version                 // the version the build belong to
	TaskIDs             TaskIdConfig            // pre-generated IDs for the tasks to be created
	BuildName           string                  // name of the buildvariant
	ActivateBuild       bool                    // true if the build should be scheduled
	ActivationInfo      specificActivationInfo  // indicates if the task has a specific activation or is a stepback task
	TaskNames           []string                // names of tasks to create (used in patches). Will create all if nil
	DisplayNames        []string                // names of display tasks to create (used in patches). Will create all if nil
	GeneratedBy         string                  // ID of the task that generated this build
	SourceRev           string                  // githash of the revision that triggered this build
	DefinitionID        string                  // definition ID of the trigger used to create this build
	Aliases             ProjectAliases          // project aliases to use to filter tasks created
	DistroAliases       distro.AliasLookupTable // map of distro aliases to names of distros
	TaskCreateTime      time.Time               // create time of tasks in the build
	GithubChecksAliases ProjectAliases          // project aliases to use to filter tasks to count towards the github checks, if any
	SyncAtEndOpts       patch.SyncAtEndOptions
}

// CreateBuildFromVersionNoInsert creates a build given all of the necessary information
// from the corresponding version and project and a list of tasks. Note that the caller
// is responsible for inserting the created build and task documents
func CreateBuildFromVersionNoInsert(args BuildCreateArgs) (*build.Build, task.Tasks, error) {
	// avoid adding all tasks in the case of no tasks matching aliases
	if len(args.Aliases) > 0 && len(args.TaskNames) == 0 {
		return nil, nil, nil
	}
	// find the build variant for this project/build
	buildVariant := args.Project.FindBuildVariant(args.BuildName)
	if buildVariant == nil {
		return nil, nil, errors.Errorf("could not find build '%s' in project file '%s'", args.BuildName, args.Project.Identifier)
	}

	rev := args.Version.Revision
	if evergreen.IsPatchRequester(args.Version.Requester) {
		rev = fmt.Sprintf("patch_%s_%s", args.Version.Revision, args.Version.Id)
	} else if args.Version.Requester == evergreen.TriggerRequester {
		rev = fmt.Sprintf("%s_%s", args.SourceRev, args.DefinitionID)
	} else if args.Version.Requester == evergreen.AdHocRequester {
		rev = args.Version.Id
	} else if args.Version.Requester == evergreen.GitTagRequester {
		rev = fmt.Sprintf("%s_%s", args.SourceRev, args.Version.TriggeredByGitTag.Tag)
	}

	// create a new build id
	buildId := fmt.Sprintf("%s_%s_%s_%s",
		args.ProjectRef.Identifier,
		args.BuildName,
		rev,
		args.Version.CreateTime.Format(build.IdTimeLayout))

	activatedTime := utility.ZeroTime
	if args.ActivateBuild {
		activatedTime = time.Now()
	}

	// create the build itself
	b := &build.Build{
		Id:                  util.CleanName(buildId),
		CreateTime:          args.Version.CreateTime,
		Activated:           args.ActivateBuild,
		ActivatedTime:       activatedTime,
		Project:             args.Project.Identifier,
		Revision:            args.Version.Revision,
		Status:              evergreen.BuildCreated,
		BuildVariant:        args.BuildName,
		Version:             args.Version.Id,
		DisplayName:         buildVariant.DisplayName,
		RevisionOrderNumber: args.Version.RevisionOrderNumber,
		Requester:           args.Version.Requester,
		ParentPatchID:       args.Version.ParentPatchID,
		ParentPatchNumber:   args.Version.ParentPatchNumber,
		TriggerID:           args.Version.TriggerID,
		TriggerType:         args.Version.TriggerType,
		TriggerEvent:        args.Version.TriggerEvent,
		Tags:                buildVariant.Tags,
	}

	// get a new build number for the build
	buildNumber, err := global.GetNewBuildVariantBuildNumber(args.BuildName)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "getting build number for build variant"+
			" %s in %s project file", args.BuildName, args.Project.Identifier)
	}
	b.BuildNumber = strconv.FormatUint(buildNumber, 10)

	// create all of the necessary tasks for the build
	tasksForBuild, err := createTasksForBuild(&args.Project, &args.ProjectRef, buildVariant, b, &args.Version, args.TaskIDs,
		args.TaskNames, args.DisplayNames, args.ActivationInfo, args.GeneratedBy,
		nil, args.SyncAtEndOpts, args.DistroAliases, args.TaskCreateTime, args.GithubChecksAliases)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "creating tasks for build '%s'", b.Id)
	}

	for _, t := range tasksForBuild {
		if t.IsGithubCheck {
			b.IsGithubCheck = true
		}
		break
	}

	// create task caches for all of the tasks, and place them into the build
	tasks := []task.Task{}
	for _, taskP := range tasksForBuild {
		if taskP.IsPartOfDisplay() {
			continue // don't add execution parts of display tasks to the UI cache
		}
		tasks = append(tasks, *taskP)
	}
	b.Tasks = CreateTasksCache(tasks)

	return b, tasksForBuild, nil
}

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
func createTasksForBuild(project *Project, pRef *ProjectRef, buildVariant *BuildVariant, b *build.Build, v *Version,
	taskIds TaskIdConfig, taskNames []string, displayNames []string, activationInfo specificActivationInfo, generatedBy string,
	tasksInBuild []task.Task, syncAtEndOpts patch.SyncAtEndOptions, distroAliases map[string][]string, createTime time.Time,
	githubChecksAliases ProjectAliases) (task.Tasks, error) {

	// The list of tasks we should create.
	// If tasks are passed in, then use those, otherwise use the default set.
	tasksToCreate := []BuildVariantTaskUnit{}

	createAll := false
	if len(taskNames) == 0 && len(displayNames) == 0 {
		createAll = true
	}
	// Tables includes only new and existing tasks.
	execTable := taskIds.ExecutionTasks
	displayTable := taskIds.DisplayTasks

	tgMap := map[string]TaskGroup{}
	for _, tg := range project.TaskGroups {
		tgMap[tg.Name] = tg
	}

	for _, task := range buildVariant.Tasks {
		// Verify that the config isn't malformed.
		if task.Name != "" && !task.IsGroup {
			if task.IsDisabled() || task.SkipOnRequester(b.Requester) {
				continue
			}
			if createAll || utility.StringSliceContains(taskNames, task.Name) {
				tasksToCreate = append(tasksToCreate, task)
			}
		} else if _, ok := tgMap[task.Name]; ok {
			tasksFromVariant := CreateTasksFromGroup(task, project, b.Requester)
			for _, taskFromVariant := range tasksFromVariant {
				if task.IsDisabled() || taskFromVariant.SkipOnRequester(b.Requester) {
					continue
				}
				if createAll || utility.StringSliceContains(taskNames, taskFromVariant.Name) {
					tasksToCreate = append(tasksToCreate, taskFromVariant)
				}
			}
		} else {
			return nil, errors.Errorf("config is malformed: variant '%s' runs "+
				"task called '%s' but no such task exists for repo '%s' for "+
				"version '%s'", buildVariant.Name, task.Name, project.Identifier, v.Id)
		}
	}

	// if any tasks already exist in the build, add them to the id table
	// so they can be used as dependencies
	for _, task := range tasksInBuild {
		execTable.AddId(b.BuildVariant, task.DisplayName, task.Id)
	}
	generatorIsGithubCheck := false
	if generatedBy != "" {
		generateTask, err := task.FindOneId(generatedBy)
		if err != nil {
			return nil, errors.Wrapf(err, "finding generated task '%s'", generatedBy)
		}
		if generateTask == nil {
			return nil, errors.Errorf("generated task '%s' not found", generatedBy)
		}
		generatorIsGithubCheck = generateTask.IsGithubCheck
	}

	// create all the actual tasks
	taskMap := make(map[string]*task.Task)
	for _, t := range tasksToCreate {
		id := execTable.GetId(b.BuildVariant, t.Name)
		newTask, err := createOneTask(id, t, project, pRef, buildVariant, b, v, distroAliases, createTime, activationInfo, githubChecksAliases)
		if err != nil {
			return nil, errors.Wrapf(err, "creating task '%s'", id)
		}

		// set Tags based on the spec
		newTask.Tags = project.GetSpecForTask(t.Name).Tags
		newTask.DependsOn = makeDeps(t, newTask, execTable)
		newTask.GeneratedBy = generatedBy
		if generatorIsGithubCheck {
			newTask.IsGithubCheck = true
		}

		if shouldSyncTask(syncAtEndOpts.VariantsTasks, newTask.BuildVariant, newTask.DisplayName) {
			newTask.CanSync = true
			newTask.SyncAtEndOpts = task.SyncAtEndOptions{
				Enabled:  true,
				Statuses: syncAtEndOpts.Statuses,
				Timeout:  syncAtEndOpts.Timeout,
			}
		} else {
			cmds, err := project.CommandsRunOnTV(TVPair{TaskName: newTask.DisplayName, Variant: newTask.BuildVariant}, evergreen.S3PushCommandName)
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
	for _, dt := range buildVariant.DisplayTasks {
		id := displayTable.GetId(b.BuildVariant, dt.Name)
		if id == "" {
			continue
		}
		execTasksThatNeedParentId := []string{}
		execTaskIds := []string{}
		displayTaskActivated := false
		displayTaskAlreadyExists := !createAll && !utility.StringSliceContains(displayNames, dt.Name)

		// get display task activations status and update exec tasks
		for _, et := range dt.ExecTasks {
			execTaskId := execTable.GetId(b.BuildVariant, et)
			if execTaskId == "" {
				grip.Error(message.Fields{
					"message":                     "execution task not found",
					"variant":                     b.BuildVariant,
					"exec_task":                   et,
					"available_tasks":             execTable,
					"project":                     project.Identifier,
					"display_task":                id,
					"display_task_already_exists": displayTaskAlreadyExists,
				})
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
			"build_id":             b.Id,
		}))

		// existing display task may need to be updated
		if displayTaskAlreadyExists {
			grip.Error(message.WrapError(task.AddExecTasksToDisplayTask(id, execTaskIds, displayTaskActivated), message.Fields{
				"message":      "problem adding exec tasks to display tasks",
				"exec_tasks":   execTaskIds,
				"display_task": dt.Name,
				"build_id":     b.Id,
			}))
		} else { // need to create display task
			if len(execTaskIds) == 0 {
				continue
			}
			newDisplayTask, err := createDisplayTask(id, dt.Name, execTaskIds, buildVariant, b, v, project, createTime, displayTaskActivated)
			if err != nil {
				return nil, errors.Wrapf(err, "creating display task '%s'", id)
			}
			newDisplayTask.GeneratedBy = generatedBy
			newDisplayTask.DependsOn, err = task.GetAllDependencies(newDisplayTask.ExecutionTasks, taskMap)
			if err != nil {
				return nil, errors.Wrapf(err, "getting dependencies for display task '%s'", newDisplayTask.Id)
			}

			tasks = append(tasks, newDisplayTask)
		}
	}

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

// makeDeps takes dependency definitions in the project and sets them in the task struct.
// dependencies between commit queue merges are set outside this function
func makeDeps(t BuildVariantTaskUnit, thisTask *task.Task, taskIds TaskIdTable) []task.Dependency {
	dependencySet := make(map[task.Dependency]bool)
	for _, dep := range t.DependsOn {
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
			dependencySet[task.Dependency{TaskId: id, Status: status}] = true
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

func getTaskCreateTime(projectId string, v *Version) (time.Time, error) {
	createTime := time.Time{}
	if evergreen.IsPatchRequester(v.Requester) {
		baseVersion, err := VersionFindOne(BaseVersionByProjectIdAndRevision(projectId, v.Revision))
		if err != nil {
			return createTime, errors.Wrap(err, "finding base version for patch version")
		}
		if baseVersion == nil {
			// The database data may be incomplete and missing the base Version
			// In that case we don't want to fail, we fallback to the patch version's CreateTime.
			return v.CreateTime, nil
		}
		return baseVersion.CreateTime, nil
	} else {
		return v.CreateTime, nil
	}
}

// createOneTask is a helper to create a single task.
func createOneTask(id string, buildVarTask BuildVariantTaskUnit, project *Project, pRef *ProjectRef, buildVariant *BuildVariant,
	b *build.Build, v *Version, dat distro.AliasLookupTable, createTime time.Time, activationInfo specificActivationInfo,
	githubChecksAliases ProjectAliases) (*task.Task, error) {

	activateTask := b.Activated && !activationInfo.taskHasSpecificActivation(b.BuildVariant, buildVarTask.Name)
	isStepback := activationInfo.isStepbackTask(b.BuildVariant, buildVarTask.Name)

	buildVarTask.RunOn = dat.Expand(buildVarTask.RunOn)
	buildVariant.RunOn = dat.Expand(buildVariant.RunOn)

	activatedTime := utility.ZeroTime
	if activateTask {
		activatedTime = time.Now()
	}

	isGithubCheck := false
	if len(githubChecksAliases) > 0 {
		var err error
		name, tags, ok := project.GetTaskNameAndTags(buildVarTask)
		if ok {
			isGithubCheck, err = githubChecksAliases.HasMatchingTask(name, tags)
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error checking if task matches aliases",
				"version": v.Id,
				"task":    buildVarTask.Name,
				"variant": buildVarTask.Variant,
			}))
		}
	}

	t := &task.Task{
		Id:                  id,
		Secret:              utility.RandomString(),
		DisplayName:         buildVarTask.Name,
		BuildId:             b.Id,
		BuildVariant:        buildVariant.Name,
		CreateTime:          createTime,
		IngestTime:          time.Now(),
		ScheduledTime:       utility.ZeroTime,
		StartTime:           utility.ZeroTime, // Certain time fields must be initialized
		FinishTime:          utility.ZeroTime, // to our own utility.ZeroTime value (which is
		DispatchTime:        utility.ZeroTime, // Unix epoch 0, not Go's time.Time{})
		LastHeartbeat:       utility.ZeroTime,
		Status:              evergreen.TaskUndispatched,
		Activated:           activateTask,
		ActivatedTime:       activatedTime,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Requester:           v.Requester,
		ParentPatchID:       b.ParentPatchID,
		ParentPatchNumber:   b.ParentPatchNumber,
		Version:             v.Id,
		Revision:            v.Revision,
		MustHaveResults:     utility.FromBoolPtr(project.GetSpecForTask(buildVarTask.Name).MustHaveResults),
		Project:             project.Identifier,
		Priority:            buildVarTask.Priority,
		GenerateTask:        project.IsGenerateTask(buildVarTask.Name),
		TriggerID:           v.TriggerID,
		TriggerType:         v.TriggerType,
		TriggerEvent:        v.TriggerEvent,
		CommitQueueMerge:    buildVarTask.CommitQueueMerge,
		IsGithubCheck:       isGithubCheck,
		DisplayTaskId:       utility.ToStringPtr(""), // this will be overridden if the task is an execution task
	}

	t.ExecutionPlatform = shouldRunOnContainer(buildVarTask.RunOn, buildVariant.RunOn, project.Containers)
	if t.IsContainerTask() {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return nil, errors.Wrap(err, "getting service flags")
		}
		if flags.ContainerConfigurationsDisabled {
			return nil, errors.Errorf("container configurations are disabled; task '%s' cannot run", t.DisplayName)
		}

		t.Container, err = getContainerFromRunOn(id, buildVarTask, buildVariant)
		if err != nil {
			return nil, err
		}
		opts, err := getContainerOptions(project, pRef, t.Container)
		if err != nil {
			return nil, errors.Wrap(err, "getting container options")
		}
		t.ContainerOpts = *opts
	} else {
		distroID, distroAliases, err := getDistrosFromRunOn(id, buildVarTask, buildVariant, project, v)
		if err != nil {
			return nil, err
		}
		t.DistroId = distroID
		t.DistroAliases = distroAliases
	}

	if isStepback {
		t.ActivatedBy = evergreen.StepbackTaskActivator
	} else if t.Activated {
		t.ActivatedBy = v.Author
	}

	if buildVarTask.IsGroup {
		tg := project.FindTaskGroup(buildVarTask.GroupName)
		if tg == nil {
			return nil, errors.Errorf("finding task group '%s' in project '%s'", buildVarTask.GroupName, project.Identifier)
		}

		tg.InjectInfo(t)
	}

	return t, nil
}

func getDistrosFromRunOn(id string, buildVarTask BuildVariantTaskUnit, buildVariant *BuildVariant, project *Project, v *Version) (string, []string, error) {
	if len(buildVarTask.RunOn) > 0 {
		distroAliases := []string{}
		distroID := buildVarTask.RunOn[0]
		if len(buildVarTask.RunOn) > 1 {
			distroAliases = buildVarTask.RunOn[1:]
		}
		return distroID, distroAliases, nil
	} else if len(buildVariant.RunOn) > 0 {
		distroAliases := []string{}
		distroID := buildVariant.RunOn[0]
		if len(buildVariant.RunOn) > 1 {
			distroAliases = buildVariant.RunOn[1:]
		}
		return distroID, distroAliases, nil
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
func getContainerOptions(project *Project, pRef *ProjectRef, container string) (*task.ContainerOptions, error) {
	for _, c := range project.Containers {
		if c.Name != container {
			continue
		}

		opts := task.ContainerOptions{
			WorkingDir:     c.WorkingDir,
			Image:          c.Image,
			OS:             c.System.OperatingSystem,
			Arch:           c.System.CPUArchitecture,
			WindowsVersion: c.System.WindowsVersion,
		}

		if c.Resources != nil {
			opts.CPU = c.Resources.CPU
			opts.MemoryMB = c.Resources.MemoryMB
			return &opts, nil
		}

		size, ok := pRef.ContainerSizes[c.Size]
		if !ok {
			return nil, errors.Errorf("container size '%s' not found", c.Size)
		}

		opts.CPU = size.CPU
		opts.MemoryMB = size.MemoryMB
		return &opts, nil
	}

	return nil, errors.Errorf("definition for container '%s' not found", container)
}

func createDisplayTask(id string, displayName string, execTasks []string, bv *BuildVariant, b *build.Build,
	v *Version, p *Project, createTime time.Time, displayTaskActivated bool) (*task.Task, error) {

	activatedTime := utility.ZeroTime
	if displayTaskActivated {
		activatedTime = time.Now()
	}

	t := &task.Task{
		Id:                  id,
		DisplayName:         displayName,
		BuildVariant:        bv.Name,
		BuildId:             b.Id,
		CreateTime:          createTime,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Version:             v.Id,
		Revision:            v.Revision,
		Project:             p.Identifier,
		Requester:           v.Requester,
		ParentPatchID:       b.ParentPatchID,
		ParentPatchNumber:   b.ParentPatchNumber,
		DisplayOnly:         true,
		ExecutionTasks:      execTasks,
		Status:              evergreen.TaskUndispatched,
		IngestTime:          time.Now(),
		StartTime:           utility.ZeroTime,
		FinishTime:          utility.ZeroTime,
		Activated:           displayTaskActivated,
		ActivatedTime:       activatedTime,
		DispatchTime:        utility.ZeroTime,
		ScheduledTime:       utility.ZeroTime,
		TriggerID:           v.TriggerID,
		TriggerType:         v.TriggerType,
		TriggerEvent:        v.TriggerEvent,
		DisplayTaskId:       utility.ToStringPtr(""),
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
func addNewBuilds(ctx context.Context, activationInfo specificActivationInfo, v *Version, p *Project, tasks TaskVariantPairs,
	existingBuilds []build.Build, syncAtEndOpts patch.SyncAtEndOptions, projectRef *ProjectRef, generatedBy string) ([]string, error) {

	taskIdTables, err := getTaskIdTables(v, p, tasks, projectRef.Identifier)
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

	createTime, err := getTaskCreateTime(p.Identifier, v)
	if err != nil {
		return nil, errors.Wrap(err, "getting create time for tasks")
	}
	batchTimeCatcher := grip.NewBasicCatcher()
	for _, pair := range tasks.ExecTasks {
		if _, ok := variantsProcessed[pair.Variant]; ok { // skip variant that was already processed
			continue
		}
		variantsProcessed[pair.Variant] = true
		// Extract the unique set of task names for the variant we're about to create
		taskNames := tasks.ExecTasks.TaskNames(pair.Variant)
		displayNames := tasks.DisplayTasks.TaskNames(pair.Variant)
		activateVariant := !activationInfo.variantHasSpecificActivation(pair.Variant)
		buildArgs := BuildCreateArgs{
			Project:        *p,
			ProjectRef:     *projectRef,
			Version:        *v,
			TaskIDs:        taskIdTables,
			BuildName:      pair.Variant,
			ActivateBuild:  activateVariant,
			TaskNames:      taskNames,
			DisplayNames:   displayNames,
			ActivationInfo: activationInfo,
			GeneratedBy:    generatedBy,
			TaskCreateTime: createTime,
			SyncAtEndOpts:  syncAtEndOpts,
		}

		grip.Info(message.Fields{
			"op":        "creating build for version",
			"variant":   pair.Variant,
			"activated": activateVariant,
			"version":   v.Id,
		})
		build, tasks, err := CreateBuildFromVersionNoInsert(buildArgs)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if len(tasks) == 0 {
			grip.Info(message.Fields{
				"op":        "skipping empty build for version",
				"variant":   pair.Variant,
				"activated": activateVariant,
				"version":   v.Id,
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
			if activationInfo.taskHasSpecificActivation(t.BuildVariant, t.DisplayName) {
				batchTimeTasksToIds[t.DisplayName] = t.Id
			}
		}

		var activateVariantAt time.Time
		batchTimeTaskStatuses := []BatchTimeTaskStatus{}
		if !activateVariant {
			activateVariantAt, err = projectRef.GetActivationTimeForVariant(p.FindBuildVariant(pair.Variant))
			batchTimeCatcher.Wrapf(err, "getting activation time for variant '%s'", pair.Variant)
		}
		for taskName, id := range batchTimeTasksToIds {
			activateTaskAt, err := projectRef.GetActivationTimeForTask(p.FindTaskForVariant(taskName, pair.Variant))
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
		"version": v.Id,
	}))

	return newActivatedTaskIds, errors.WithStack(VersionUpdateOne(
		bson.M{VersionIdKey: v.Id},
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
func addNewTasks(ctx context.Context, activationInfo specificActivationInfo, v *Version, p *Project, pRef *ProjectRef, pairs TaskVariantPairs,
	existingBuilds []build.Build, syncAtEndOpts patch.SyncAtEndOptions, generatedBy string) ([]string, error) {
	if v.BuildIds == nil {
		return nil, nil
	}

	distroAliases, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return nil, err
	}

	taskIdTables, err := getTaskIdTables(v, p, pairs, pRef.Identifier)
	if err != nil {
		return nil, errors.Wrap(err, "getting table of task IDs")
	}

	activatedTaskIds := []string{}
	activatedTasks := []task.Task{}
	for _, b := range existingBuilds {
		wasActivated := b.Activated
		// Find the set of task names that already exist for the given build, including display tasks.
		tasksInBuild, err := task.FindAll(db.Query(task.ByBuildId(b.Id)).WithFields(task.DisplayNameKey, task.ActivatedKey))
		if err != nil {
			return nil, err
		}

		existingTasksIndex := map[string]bool{}
		for _, t := range tasksInBuild {
			existingTasksIndex[t.DisplayName] = true
		}
		projectBV := p.FindBuildVariant(b.BuildVariant)
		if projectBV != nil {
			b.Activated = utility.FromBoolTPtr(projectBV.Activate) // activate unless explicitly set otherwise
		}

		// Build a list of tasks that haven't been created yet for the given variant, but have
		// a record in the TVPairSet indicating that it should exist
		tasksToAdd := []string{}
		for _, taskName := range pairs.ExecTasks.TaskNames(b.BuildVariant) {
			if ok := existingTasksIndex[taskName]; ok {
				continue
			}
			tasksToAdd = append(tasksToAdd, taskName)
		}
		displayTasksToAdd := []string{}
		for _, taskName := range pairs.DisplayTasks.TaskNames(b.BuildVariant) {
			if ok := existingTasksIndex[taskName]; ok {
				continue
			}
			displayTasksToAdd = append(displayTasksToAdd, taskName)
		}
		if len(tasksToAdd) == 0 && len(displayTasksToAdd) == 0 { // no tasks to add, so we do nothing.
			continue
		}
		// Add the new set of tasks to the build.
		_, tasks, err := addTasksToBuild(ctx, &b, p, pRef, v, tasksToAdd, displayTasksToAdd, activationInfo,
			generatedBy, tasksInBuild, syncAtEndOpts, distroAliases, taskIdTables)
		if err != nil {
			return nil, err
		}

		for _, t := range tasks {
			if t.Activated {
				activatedTaskIds = append(activatedTaskIds, t.Id)
				activatedTasks = append(activatedTasks, *t)
				b.Activated = true
			}
			if t.Activated && activationInfo.isStepbackTask(t.BuildVariant, t.DisplayName) {
				event.LogTaskActivated(t.Id, t.Execution, evergreen.StepbackTaskActivator)
			}
		}
		// update build activation status if tasks have since been activated
		if !wasActivated && b.Activated {
			if err := build.UpdateActivation([]string{b.Id}, true, evergreen.DefaultTaskActivator); err != nil {
				return nil, err
			}
		}
	}
	if activationInfo.hasActivationTasks() {
		grip.Error(message.WrapError(v.UpdateBuildVariants(), message.Fields{
			"message": "unable to add batchtime tasks",
			"version": v.Id,
		}))
	}
	if err = v.SetActivated(); err != nil {
		return nil, errors.Wrap(err, "setting version activation to true")
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

func getTaskIdTables(v *Version, p *Project, newPairs TaskVariantPairs, projectName string) (TaskIdConfig, error) {
	// The table should include only new and existing tasks
	taskIdTable := NewPatchTaskIdTable(p, v, newPairs, projectName)
	existingTasks, err := task.FindAll(db.Query(task.ByVersion(v.Id)).WithFields(task.DisplayOnlyKey, task.DisplayNameKey, task.BuildVariantKey))
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

	return taskIdTable, nil
}
