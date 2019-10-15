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
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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
		err = SetBuildActivation(b.Id, active, caller, false)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetBuildActivation updates the "active" state of this build and all associated tasks.
// It also updates the task cache for the build document.
func SetBuildActivation(buildId string, active bool, caller string, skipDependencies bool) error {
	var err error
	// If activating a task, set the ActivatedBy field to be the caller
	if active {
		_, err = task.UpdateAll(
			bson.M{
				task.BuildIdKey: buildId,
				task.StatusKey:  evergreen.TaskUndispatched,
			},
			bson.M{"$set": bson.M{task.ActivatedKey: active, task.ActivatedByKey: caller, task.ActivatedTimeKey: time.Now()}},
		)
		if err != nil {
			return errors.Wrap(err, "problem updating tasks for activation")
		}
		if !skipDependencies {
			var tasks []task.Task
			tasks, err = task.FindTasksFromBuildWithDependencies(buildId)
			if err != nil {
				return errors.Wrapf(err, "problem finding tasks with dependencies for build %s", buildId)
			}
			catcher := grip.NewBasicCatcher()
			for _, t := range tasks {
				for _, d := range t.DependsOn {
					catcher.Add(SetActiveState(d.TaskId, caller, active))
				}
			}
			grip.Error(errors.Wrapf(catcher.Resolve(), "problem settings dependencies for build %s", buildId))
		}
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
	err := task.AbortBuild(buildId, caller)
	if err != nil {
		return err
	}
	return build.UpdateActivation(buildId, false, caller)
}

// AbortVersion sets the abort flag on all tasks associated with the version which are in an
// abortable state
func AbortVersion(versionId, caller string) error {
	_, err := task.UpdateAll(
		bson.M{
			task.VersionKey: versionId,
			task.StatusKey:  bson.M{"$in": evergreen.AbortableStatuses},
		},
		bson.M{"$set": bson.M{task.AbortedKey: true}},
	)
	if err != nil {
		return errors.Wrap(err, "error setting aborted statuses")
	}
	ids, err := task.FindAllTaskIDsFromVersion(versionId)
	if err != nil {
		return errors.Wrap(err, "error finding tasks by version id")
	}
	if len(ids) > 0 {
		event.LogManyTaskAbortRequests(ids, caller)
	}
	return nil
}

func MarkVersionStarted(versionId string, startTime time.Time) error {
	return VersionUpdateOne(
		bson.M{VersionIdKey: versionId},
		bson.M{"$set": bson.M{
			VersionStartTimeKey: startTime,
			VersionStatusKey:    evergreen.VersionStarted,
		}},
	)
}

// MarkVersionCompleted updates the status of a completed version to reflect its correct state by
// checking the status of its individual builds.
func MarkVersionCompleted(versionId string, finishTime time.Time, updates *StatusChanges) error {
	status := evergreen.VersionSucceeded

	// Find the statuses for all builds in the version so we can figure out the version's status
	builds, err := build.Find(
		build.ByVersion(versionId).WithFields(build.ActivatedKey, build.StatusKey, build.TasksKey),
	)
	if err != nil {
		return err
	}

	versionStatusFromTasks := evergreen.VersionSucceeded
	buildsWithAllActiveTasksComplete := 0
	activeBuilds := 0
	finished := true
	if err != nil {
		return errors.Wrap(err, "error finding tasks with dependencies")
	}
	startPhaseAt := time.Now()
	tasks, err := task.Find(task.ByVersion(versionId).WithFields(task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey))
	if err != nil {
		return errors.Wrapf(err, "problem finding tasks for version %s", versionId)
	}
	for _, b := range builds {
		if b.Activated {
			activeBuilds++
		}
		complete, buildStatus, err := b.AllUnblockedTasksFinished(tasks)
		if err != nil {
			return errors.WithStack(err)
		}
		if complete {
			buildsWithAllActiveTasksComplete++
			if buildStatus != evergreen.BuildSucceeded {
				versionStatusFromTasks = evergreen.VersionFailed
			}
		}
		if !b.IsFinished() {
			finished = false
			continue
		}
		if b.Status != evergreen.BuildSucceeded {
			status = evergreen.VersionFailed
		}
	}
	grip.DebugWhen(time.Since(startPhaseAt) > time.Second, message.Fields{
		"function":      "MarkVersionCompleted",
		"operation":     "build loop",
		"message":       "slow operation",
		"duration_secs": time.Since(startPhaseAt).Seconds(),
		"version":       versionId,
		"num_builds":    len(builds),
	})
	if activeBuilds > 0 && buildsWithAllActiveTasksComplete >= activeBuilds {
		updates.VersionComplete = true
		updates.VersionNewStatus = versionStatusFromTasks
		event.LogVersionStateChangeEvent(versionId, status)
	}
	if !finished {
		return nil
	}
	if err := VersionUpdateOne(
		bson.M{VersionIdKey: versionId},
		bson.M{"$set": bson.M{
			VersionFinishTimeKey: finishTime,
			VersionStatusKey:     status,
		}},
	); err != nil {
		return errors.WithStack(err)
	}
	return nil
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
	allTasks, err := task.FindWithDisplayTasks(task.ByDispatchedWithIdsVersionAndStatus(taskIds, versionId, task.CompletedStatuses))
	if err != nil && !adb.ResultsNotFound(err) {
		return errors.WithStack(err)
	}

	restartIds := make([]string, 0)
	// archive all the tasks
	for _, t := range allTasks {
		if err = t.Archive(); err != nil {
			return errors.Wrap(err, "failed to archive task")
		}
		if t.DisplayOnly {
			restartIds = append(restartIds, t.ExecutionTasks...)
		}
	}

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
			return errors.WithStack(err)
		}
	}

	if abortInProgress {
		restartIds = append(restartIds, taskIds...)
	} else {
		for _, t := range allTasks {
			restartIds = append(restartIds, t.Id)
		}
	}

	// Set all the task fields to indicate restarted
	if err = task.ResetTasks(restartIds); err != nil {
		return errors.WithStack(err)
	}

	// TODO figure out a way to coalesce updates for task cache for the same build, so we
	// only need to do one update per-build instead of one per-task here.
	// Doesn't seem to be possible as-is because $ can only apply to one array element matched per
	// document.
	buildIdSet := map[string]bool{}
	for _, t := range allTasks {
		buildIdSet[t.BuildId] = true
		if err = build.ResetCachedTask(t.BuildId, t.Id); err != nil {
			return errors.WithStack(err)
		}
	}

	// reset the build statuses, once per build
	buildIdList := make([]string, 0, len(buildIdSet))
	for k := range buildIdSet {
		buildIdList = append(buildIdList, k)
	}

	// Set the build status for all the builds containing the tasks that we touched
	_, err = build.UpdateAllBuilds(
		bson.M{build.IdKey: bson.M{"$in": buildIdList}},
		bson.M{"$set": bson.M{build.StatusKey: evergreen.BuildStarted}},
	)

	if err != nil {
		return errors.WithStack(err)
	}

	// update activation for all the builds
	for _, b := range buildIdList {
		if err := build.UpdateActivation(b, true, caller); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// RestartBuild restarts completed tasks associated with a given buildId.
// If abortInProgress is true, it also sets the abort flag on any in-progress tasks.
func RestartBuild(buildId string, taskIds []string, abortInProgress bool, caller string) error {
	// restart all the 'not in-progress' tasks for the build
	allTasks, err := task.FindWithDisplayTasks(task.ByIdsBuildAndStatus(taskIds, buildId, task.CompletedStatuses))
	if err != nil && err != mgo.ErrNotFound {
		return errors.WithStack(err)
	}

	for _, t := range allTasks {
		if t.DispatchTime != util.ZeroTime {
			err = resetTask(t.Id, caller)
			if err != nil {
				return errors.Wrapf(err,
					"Restarting build '%s' failed, could not task.reset on task '%s'",
					buildId, t.Id)
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
			return errors.WithStack(err)
		}
	}

	return errors.WithStack(build.UpdateActivation(buildId, true, caller))
}

// RestartBuildTasks restarts all the tasks associated with a given build.
func RestartBuildTasks(buildId string, caller string) error {
	allTasks, err := task.FindWithDisplayTasks(task.ByBuildId(buildId))
	if err != nil && err != mgo.ErrNotFound {
		return errors.WithStack(err)
	}

	for _, t := range allTasks {
		if t.DispatchTime != util.ZeroTime {
			err = resetTask(t.Id, caller)
			if err != nil {
				return errors.Wrapf(err,
					"Restarting build '%s' failed, could not task.reset on task '%s'",
					buildId, t.Id)
			}
		}
	}
	return errors.WithStack(build.UpdateActivation(buildId, true, caller))
}

func CreateTasksCache(tasks []task.Task) []build.TaskCache {
	tasks = sortTasks(tasks)
	cache := make([]build.TaskCache, 0, len(tasks))
	for _, task := range tasks {
		if task.DisplayTask == nil {
			cache = append(cache, cacheFromTask(task))
		}
	}
	return cache
}

// RefreshTasksCache updates a build document so that the tasks cache reflects the correct current
// state of the tasks it represents.
func RefreshTasksCache(buildId string) error {
	tasks, err := task.FindWithDisplayTasks(task.ByBuildId(buildId))
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

// AddTasksToBuild creates the tasks for the given build of a project
func AddTasksToBuild(ctx context.Context, b *build.Build, project *Project, v *Version, taskNames []string,
	displayNames []string, generatedBy string, tasksInBuild []task.Task, distroAliases map[string][]string) (*build.Build, error) {
	// find the build variant for this project/build
	buildVariant := project.FindBuildVariant(b.BuildVariant)
	if buildVariant == nil {
		return nil, errors.Errorf("Could not find build %v in %v project file",
			b.BuildVariant, project.Identifier)
	}

	// create the new tasks for the build
	taskIds := NewTaskIdTable(project, v, "", "")
	tasks, err := createTasksForBuild(project, buildVariant, b, v, taskIds, taskNames, displayNames, generatedBy, nil, tasksInBuild, distroAliases)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating tasks for build '%s'", b.Id)
	}

	if err = tasks.InsertUnordered(ctx); err != nil {
		return nil, errors.Wrapf(err, "error inserting tasks for build '%s'", b.Id)
	}

	// update the build to hold the new tasks
	if err := RefreshTasksCache(b.Id); err != nil {
		return nil, errors.Wrapf(err, "error updating task cache for '%s'", b.Id)
	}

	return b, nil
}

// BuildCreateArgs is the set of parameters used in CreateBuildFromVersionNoInsert
type BuildCreateArgs struct {
	Project       Project                 // project to create the build for
	Version       Version                 // the version the build belong to
	TaskIDs       TaskIdConfig            // pre-generated IDs for the tasks to be created
	BuildName     string                  // name of the buildvariant
	Activated     bool                    // true if the build should be scheduled
	TaskNames     []string                // names of tasks to create (used in patches). Will create all if nil
	DisplayNames  []string                // names of display tasks to create (used in patches). Will create all if nil
	GeneratedBy   string                  // ID of the task that generated this build
	SourceRev     string                  // githash of the revision that triggered this build
	DefinitionID  string                  // definition ID of the trigger used to create this build
	Aliases       ProjectAliases          // project aliases to use to filter tasks created
	DistroAliases distro.AliasLookupTable // map of distro aliases to names of distros
}

// CreateBuildFromVersionNoInsert creates a build given all of the necessary information
// from the corresponding version and project and a list of tasks. Note that the caller
// is responsible for inserting the created build and task documents
func CreateBuildFromVersionNoInsert(args BuildCreateArgs) (*build.Build, task.Tasks, error) {
	// find the build variant for this project/build
	buildVariant := args.Project.FindBuildVariant(args.BuildName)
	if buildVariant == nil {
		return nil, nil, errors.Errorf("could not find build %v in %v project file", args.BuildName, args.Project.Identifier)
	}

	rev := args.Version.Revision
	if evergreen.IsPatchRequester(args.Version.Requester) {
		rev = fmt.Sprintf("patch_%s_%s", args.Version.Revision, args.Version.Id)
	} else if args.Version.Requester == evergreen.TriggerRequester {
		rev = fmt.Sprintf("%s_%s", args.SourceRev, args.DefinitionID)
	} else if args.Version.Requester == evergreen.AdHocRequester {
		rev = args.Version.Id
	}

	// create a new build id
	buildId := fmt.Sprintf("%s_%s_%s_%s",
		args.Project.Identifier,
		args.BuildName,
		rev,
		args.Version.CreateTime.Format(build.IdTimeLayout))

	activatedTime := util.ZeroTime
	if args.Activated {
		activatedTime = time.Now()
	}

	// create the build itself
	b := &build.Build{
		Id:                  util.CleanName(buildId),
		CreateTime:          args.Version.CreateTime,
		Activated:           args.Activated,
		ActivatedTime:       activatedTime,
		Project:             args.Project.Identifier,
		Revision:            args.Version.Revision,
		Status:              evergreen.BuildCreated,
		BuildVariant:        args.BuildName,
		Version:             args.Version.Id,
		DisplayName:         buildVariant.DisplayName,
		RevisionOrderNumber: args.Version.RevisionOrderNumber,
		Requester:           args.Version.Requester,
		TriggerID:           args.Version.TriggerID,
		TriggerType:         args.Version.TriggerType,
		TriggerEvent:        args.Version.TriggerEvent,
	}

	// get a new build number for the build
	buildNumber, err := db.GetNewBuildVariantBuildNumber(args.BuildName)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not get build number for build variant"+
			" %v in %v project file", args.BuildName, args.Project.Identifier)
	}
	b.BuildNumber = strconv.FormatUint(buildNumber, 10)

	// create all of the necessary tasks for the build
	tasksForBuild, err := createTasksForBuild(&args.Project, buildVariant, b, &args.Version, args.TaskIDs, args.TaskNames, args.DisplayNames, args.GeneratedBy, args.Aliases, nil, args.DistroAliases)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating tasks for build %s", b.Id)
	}

	// create task caches for all of the tasks, and place them into the build
	tasks := []task.Task{}
	for _, taskP := range tasksForBuild {
		if taskP.DisplayTask != nil {
			continue // don't add execution parts of display tasks to the UI cache
		}
		tasks = append(tasks, *taskP)
	}
	b.Tasks = CreateTasksCache(tasks)

	return b, tasksForBuild, nil
}

func CreateTasksFromGroup(in BuildVariantTaskUnit, proj *Project) []BuildVariantTaskUnit {
	tasks := []BuildVariantTaskUnit{}
	tg := proj.FindTaskGroup(in.Name)
	if tg == nil {
		return tasks
	}

	taskMap := map[string]ProjectTask{}
	for _, projTask := range proj.Tasks {
		taskMap[projTask.Name] = projTask
	}

	for _, t := range tg.Tasks {
		bvt := BuildVariantTaskUnit{
			Name: t,
			// IsGroup is not persisted, and indicates here that the
			// task is a member of a task group.
			IsGroup:          true,
			GroupName:        in.Name,
			Patchable:        in.Patchable,
			Priority:         in.Priority,
			DependsOn:        in.DependsOn,
			Requires:         in.Requires,
			Distros:          in.Distros,
			ExecTimeoutSecs:  in.ExecTimeoutSecs,
			Stepback:         in.Stepback,
			CommitQueueMerge: in.CommitQueueMerge,
		}
		bvt.Populate(taskMap[t])
		tasks = append(tasks, bvt)
	}
	return tasks
}

// createTasksForBuild creates all of the necessary tasks for the build.  Returns a
// slice of all of the tasks created, as well as an error if any occurs.
// The slice of tasks will be in the same order as the project's specified tasks
// appear in the specified build variant.
func createTasksForBuild(project *Project, buildVariant *BuildVariant, b *build.Build, v *Version,
	taskIds TaskIdConfig, taskNames []string, displayNames []string, generatedBy string,
	aliases ProjectAliases, tasksInBuild []task.Task, distroAliases map[string][]string) (task.Tasks, error) {

	// the list of tasks we should create.  if tasks are passed in, then
	// use those, else use the default set
	tasksToCreate := []BuildVariantTaskUnit{}

	createAll := false
	if len(taskNames) == 0 && len(displayNames) == 0 {
		createAll = true
	}
	execTable := taskIds.ExecutionTasks
	displayTable := taskIds.DisplayTasks

	tgMap := map[string]TaskGroup{}
	for _, tg := range project.TaskGroups {
		tgMap[tg.Name] = tg
	}

	for _, task := range buildVariant.Tasks {
		if aliases != nil {
			match, err := aliases.HasMatchingTask(buildVariant.Name, buildVariant.Tags, project.FindProjectTask(task.Name))
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "error creating tasks with alias filter",
					"task":    task.Name,
					"project": project.Identifier,
					"alias":   aliases,
				}))
				continue
			}
			if !match {
				continue
			}
		}
		// get the task spec out of the project
		taskSpec := project.GetSpecForTask(task.Name)
		// sanity check that the config isn't malformed
		if taskSpec.Name != "" {
			task.Populate(taskSpec)
			if skipTask := b.IsPatchBuild() && task.SkipOnPatchBuild() ||
				!b.IsPatchBuild() && task.SkipOnNonPatchBuild(); skipTask {
				continue
			}
			if createAll || util.StringSliceContains(taskNames, task.Name) {
				tasksToCreate = append(tasksToCreate, task)
			}
		} else if _, ok := tgMap[task.Name]; ok {
			tasksFromVariant := CreateTasksFromGroup(task, project)
			for _, taskFromVariant := range tasksFromVariant {
				if skipTask := b.IsPatchBuild() && taskFromVariant.SkipOnPatchBuild() ||
					!b.IsPatchBuild() && taskFromVariant.SkipOnNonPatchBuild(); skipTask {
					continue
				}
				if createAll || util.StringSliceContains(taskNames, taskFromVariant.Name) {
					tasksToCreate = append(tasksToCreate, taskFromVariant)
				}
			}
		} else {
			return nil, errors.Errorf("config is malformed: variant '%v' runs "+
				"task called '%v' but no such task exists for repo %v for "+
				"version %v", buildVariant.Name, task.Name, project.Identifier, v.Id)
		}
	}

	// if any tasks already exist in the build, add them to the id table
	// so they can be used as dependencies
	for _, task := range tasksInBuild {
		execTable.AddId(b.BuildVariant, task.DisplayName, task.Id)
	}

	// create and insert all of the actual tasks
	tasks := task.Tasks{}
	displayTasks := make(map[string]*task.Task)

	// Create display tasks
	for _, dt := range buildVariant.DisplayTasks {
		id := displayTable.GetId(b.BuildVariant, dt.Name)
		if id == "" {
			continue
		}
		if !createAll && !util.StringSliceContains(displayNames, dt.Name) {
			continue
		}
		execTaskIds := []string{}
		for _, et := range dt.ExecutionTasks {
			execTaskId := execTable.GetId(b.BuildVariant, et)
			if execTaskId == "" {
				grip.Error(message.Fields{
					"message":         "execution task not found",
					"variant":         b.BuildVariant,
					"exec_task":       et,
					"available_tasks": execTable,
					"project":         project.Identifier,
				})
				continue
			}
			execTaskIds = append(execTaskIds, execTaskId)
		}
		t, err := createDisplayTask(id, dt.Name, execTaskIds, buildVariant, b, v, project)
		if err != nil {
			return tasks, errors.Wrapf(err, "Failed to create display task %s", id)
		}
		t.GeneratedBy = generatedBy
		tasks = append(tasks, t)
		for _, et := range dt.ExecutionTasks {
			displayTasks[et] = t
		}
	}

	for _, t := range tasksToCreate {
		id := execTable.GetId(b.BuildVariant, t.Name)
		newTask, err := createOneTask(id, t, project, buildVariant, b, v, distroAliases)
		if err != nil {
			return tasks, errors.Wrapf(err, "Failed to create task %s", id)
		}

		// set Tags based on the spec
		newTask.Tags = project.GetSpecForTask(t.Name).Tags

		// set the new task's dependencies
		if len(t.DependsOn) == 1 &&
			t.DependsOn[0].Name == AllDependencies &&
			t.DependsOn[0].Variant != AllVariants {
			// the task depends on all of the other tasks in the build
			newTask.DependsOn = make([]task.Dependency, 0, len(tasksToCreate)-1)
			status := evergreen.TaskSucceeded
			if t.DependsOn[0].Status != "" {
				status = t.DependsOn[0].Status
			}
			for _, dep := range tasksToCreate {
				id := execTable.GetId(b.BuildVariant, dep.Name)
				if len(id) == 0 || dep.Name == newTask.DisplayName {
					continue
				}
				newTask.DependsOn = append(newTask.DependsOn, task.Dependency{TaskId: id, Status: status})
			}
			for _, existingTask := range tasksInBuild {
				newTask.DependsOn = append(newTask.DependsOn, task.Dependency{TaskId: existingTask.Id, Status: status})
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
						ids = execTable.GetIdsForAllVariantsExcluding(
							dep.Name,
							TVPair{TaskName: newTask.DisplayName, Variant: newTask.BuildVariant},
						)
					} else {
						// edge case where variant and task are both *
						ids = execTable.GetIdsForAllTasks(b.BuildVariant, newTask.DisplayName)
					}
					for _, id := range ids {
						if len(id) != 0 {
							newDeps = append(newDeps, task.Dependency{TaskId: id, Status: status})
						}
					}
				} else {
					// general case
					id := execTable.GetId(bv, dep.Name)
					// only create the dependency if the task exists--it always will,
					// except for patches with patch_optional dependencies.
					if len(id) != 0 {
						newDeps = []task.Dependency{{TaskId: id, Status: status}}
					}
				}

				newTask.DependsOn = append(newTask.DependsOn, newDeps...)
			}
		}

		// Display tasks depend on all exec task dependencies
		if displayTask, ok := displayTasks[newTask.DisplayName]; ok {
			displayTask.DependsOn = append(displayTask.DependsOn, newTask.DependsOn...)
			newTask.DisplayTask = displayTask
		}

		newTask.GeneratedBy = generatedBy
		// append the task to the list of the created tasks
		tasks = append(tasks, newTask)
	}

	// Set the NumDependents field
	// Existing tasks in the db and tasks in other builds are not updated
	setNumDeps(tasks)

	sort.Stable(tasks)

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
func TryMarkPatchBuildFinished(b *build.Build, finishTime time.Time, updates *StatusChanges) error {
	v, err := VersionFindOne(VersionById(b.Version))
	if err != nil {
		return errors.WithStack(err)
	}
	if v == nil {
		return errors.Errorf("Cannot find version for build %v with version %v", b.Id, b.Version)
	}

	// ensure all builds for this patch are finished as well
	builds, err := build.Find(build.ByIds(v.BuildIds).WithFields(build.StatusKey, build.TasksKey))
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
	if err := patch.TryMarkFinished(v.Id, finishTime, status); err != nil {
		return errors.WithStack(err)
	}
	updates.PatchNewStatus = status

	return nil
}

func getTaskCreateTime(projectId string, v *Version) (time.Time, error) {
	createTime := time.Time{}
	if evergreen.IsPatchRequester(v.Requester) {
		baseVersion, err := VersionFindOne(VersionBaseVersionFromPatch(projectId, v.Revision))
		if err != nil {
			return createTime, errors.Wrap(err, "Error finding base version for patch version")
		}
		if baseVersion == nil {
			grip.Warningf("Could not find base version for patch version %s", v.Id)
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
func createOneTask(id string, buildVarTask BuildVariantTaskUnit, project *Project,
	buildVariant *BuildVariant, b *build.Build, v *Version, dat distro.AliasLookupTable) (*task.Task, error) {

	buildVarTask.Distros = dat.Expand(buildVarTask.Distros)
	buildVariant.RunOn = dat.Expand(buildVariant.RunOn)

	var (
		distroID      string
		distroAliases []string
	)

	if len(buildVarTask.Distros) > 0 {
		distroID = buildVarTask.Distros[0]

		if len(buildVarTask.Distros) > 1 {
			distroAliases = append(distroAliases, buildVarTask.Distros[1:]...)
		}

	} else if len(buildVariant.RunOn) > 0 {
		distroID = buildVariant.RunOn[0]

		if len(buildVariant.RunOn) > 1 {
			distroAliases = append(distroAliases, buildVariant.RunOn[1:]...)
		}
	} else {
		grip.Warning(message.Fields{
			"task_id":   id,
			"message":   "task is not runnable as there is no distro specified",
			"variant":   buildVariant.Name,
			"project":   project.Identifier,
			"version":   v.Revision,
			"requester": v.Requester,
		})
	}

	createTime, err := getTaskCreateTime(project.Identifier, v)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get create time for task %s", id)
	}

	activatedTime := util.ZeroTime
	if b.Activated {
		activatedTime = time.Now()
	}

	t := &task.Task{
		Id:                  id,
		Secret:              util.RandomString(),
		DisplayName:         buildVarTask.Name,
		BuildId:             b.Id,
		BuildVariant:        buildVariant.Name,
		DistroId:            distroID,
		DistroAliases:       distroAliases,
		CreateTime:          createTime,
		IngestTime:          time.Now(),
		ScheduledTime:       util.ZeroTime,
		StartTime:           util.ZeroTime, // Certain time fields must be initialized
		FinishTime:          util.ZeroTime, // to our own util.ZeroTime value (which is
		DispatchTime:        util.ZeroTime, // Unix epoch 0, not Go's time.Time{})
		LastHeartbeat:       util.ZeroTime,
		Status:              evergreen.TaskUndispatched,
		Activated:           b.Activated,
		ActivatedTime:       activatedTime,
		RevisionOrderNumber: v.RevisionOrderNumber,
		Requester:           v.Requester,
		Version:             v.Id,
		Revision:            v.Revision,
		Project:             project.Identifier,
		Priority:            buildVarTask.Priority,
		GenerateTask:        project.IsGenerateTask(buildVarTask.Name),
		TriggerID:           v.TriggerID,
		TriggerType:         v.TriggerType,
		TriggerEvent:        v.TriggerEvent,
		CommitQueueMerge:    buildVarTask.CommitQueueMerge,
	}
	if buildVarTask.IsGroup {
		tg := project.FindTaskGroup(buildVarTask.GroupName)
		if tg == nil {
			return nil, errors.Errorf("unable to find task group %s in project %s", buildVarTask.GroupName, project.Identifier)
		}

		tg.InjectInfo(t)
	}
	return t, nil
}

func createDisplayTask(id string, displayName string, execTasks []string,
	bv *BuildVariant, b *build.Build, v *Version, p *Project) (*task.Task, error) {

	createTime, err := getTaskCreateTime(p.Identifier, v)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get create time for task %s", id)
	}

	activatedTime := util.ZeroTime
	if b.Activated {
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
		DisplayOnly:         true,
		ExecutionTasks:      execTasks,
		Status:              evergreen.TaskUndispatched,
		IngestTime:          time.Now(),
		StartTime:           util.ZeroTime,
		FinishTime:          util.ZeroTime,
		Activated:           b.Activated,
		ActivatedTime:       activatedTime,
		DispatchTime:        util.ZeroTime,
		ScheduledTime:       util.ZeroTime,
		TriggerID:           v.TriggerID,
		TriggerType:         v.TriggerType,
		TriggerEvent:        v.TriggerEvent,
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
// (see AddNewTasksForPatch).
func AddNewBuilds(ctx context.Context, activated bool, v *Version, p *Project, tasks TaskVariantPairs, generatedBy string) error {
	taskIds := NewPatchTaskIdTable(p, v, tasks)

	newBuildIds := make([]string, 0)
	newBuildStatuses := make([]VersionBuildStatus, 0)

	existingBuilds, err := build.Find(build.ByVersion(v.Id).WithFields(build.BuildVariantKey, build.IdKey))
	if err != nil {
		return errors.WithStack(err)
	}
	variantsProcessed := map[string]bool{}
	for _, b := range existingBuilds {
		variantsProcessed[b.BuildVariant] = true
	}

	for _, pair := range tasks.ExecTasks {
		if _, ok := variantsProcessed[pair.Variant]; ok { // skip variant that was already processed
			continue
		}
		variantsProcessed[pair.Variant] = true
		// Extract the unique set of task names for the variant we're about to create
		taskNames := tasks.ExecTasks.TaskNames(pair.Variant)
		displayNames := tasks.DisplayTasks.TaskNames(pair.Variant)
		buildArgs := BuildCreateArgs{
			Project:      *p,
			Version:      *v,
			TaskIDs:      taskIds,
			BuildName:    pair.Variant,
			Activated:    activated,
			TaskNames:    taskNames,
			DisplayNames: displayNames,
			GeneratedBy:  generatedBy,
		}

		grip.Info(message.Fields{
			"op":        "creating build for version",
			"variant":   pair.Variant,
			"activated": activated,
			"version":   v.Id,
		})
		build, tasks, err := CreateBuildFromVersionNoInsert(buildArgs)
		if err != nil {
			return errors.WithStack(err)
		}
		if len(tasks) == 0 {
			grip.Info(message.Fields{
				"op":        "skipping empty build for version",
				"variant":   pair.Variant,
				"activated": activated,
				"version":   v.Id,
			})
			continue
		}

		if err = build.Insert(); err != nil {
			return errors.Wrapf(err, "error inserting build %s", build.Id)
		}
		if err = tasks.InsertUnordered(ctx); err != nil {
			return errors.Wrapf(err, "error inserting tasks for build %s", build.Id)
		}

		newBuildIds = append(newBuildIds, build.Id)
		newBuildStatuses = append(newBuildStatuses,
			VersionBuildStatus{
				BuildVariant: pair.Variant,
				BuildId:      build.Id,
				Activated:    activated,
			},
		)
	}

	return errors.WithStack(VersionUpdateOne(
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
// within the set of already existing builds.
func AddNewTasks(ctx context.Context, activated bool, v *Version, p *Project, pairs TaskVariantPairs, generatedBy string) error {
	if v.BuildIds == nil {
		return nil
	}

	builds, err := build.Find(build.ByIds(v.BuildIds).WithFields(build.IdKey, build.BuildVariantKey, build.CreateTimeKey))
	if err != nil {
		return err
	}
	distroAliases, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return err
	}

	for _, b := range builds {
		// Find the set of task names that already exist for the given build
		tasksInBuild, err := task.Find(task.ByBuildId(b.Id).WithFields(task.DisplayNameKey))
		if err != nil {
			return err
		}
		// build an index to keep track of which tasks already exist
		existingTasksIndex := map[string]bool{}
		for _, t := range tasksInBuild {
			existingTasksIndex[t.DisplayName] = true
		}
		// if the patch is activated, treat the build as activated
		b.Activated = activated

		// build a list of tasks that haven't been created yet for the given variant, but have
		// a record in the TVPairSet indicating that it should exist
		tasksToAdd := []string{}
		for _, taskname := range pairs.ExecTasks.TaskNames(b.BuildVariant) {
			if _, ok := existingTasksIndex[taskname]; ok {
				continue
			}
			tasksToAdd = append(tasksToAdd, taskname)
		}
		displayTasksToAdd := []string{}
		for _, taskname := range pairs.DisplayTasks.TaskNames(b.BuildVariant) {
			if _, ok := existingTasksIndex[taskname]; ok {
				continue
			}
			displayTasksToAdd = append(displayTasksToAdd, taskname)
		}
		if len(tasksToAdd) == 0 && len(displayTasksToAdd) == 0 { // no tasks to add, so we do nothing.
			continue
		}
		// Add the new set of tasks to the build.
		if _, err = AddTasksToBuild(ctx, &b, p, v, tasksToAdd, displayTasksToAdd, generatedBy, tasksInBuild, distroAliases); err != nil {
			return err
		}
	}
	return nil
}
