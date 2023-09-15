package model

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type StatusChanges struct {
	PatchNewStatus   string
	VersionNewStatus string
	VersionComplete  bool
	BuildNewStatus   string
	BuildComplete    bool
}

func SetActiveState(ctx context.Context, caller string, active bool, tasks ...task.Task) error {
	tasksToActivate := []task.Task{}
	versionIdsSet := map[string]bool{}
	buildToTaskMap := map[string]task.Task{}
	catcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		originalTasks := []task.Task{t}
		if t.DisplayOnly {
			execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
			catcher.Wrap(err, "getting execution tasks")
			originalTasks = append(originalTasks, execTasks...)
		}
		versionIdsSet[t.Version] = true
		buildToTaskMap[t.BuildId] = t
		if active {
			// if the task is being activated, and it doesn't override its dependencies
			// activate the task's dependencies as well
			if !t.OverrideDependencies {
				deps, err := task.GetRecursiveDependenciesUp(originalTasks, nil)
				catcher.Wrapf(err, "getting dependencies up for task '%s'", t.Id)
				if t.IsPartOfSingleHostTaskGroup() {
					for _, dep := range deps {
						// reset any already finished tasks in the same task group
						if dep.TaskGroup == t.TaskGroup && t.TaskGroup != "" && dep.IsFinished() {
							catcher.Wrapf(resetTask(ctx, dep.Id, caller), "resetting dependency '%s'", dep.Id)
						} else {
							tasksToActivate = append(tasksToActivate, dep)
						}
					}
				} else {
					tasksToActivate = append(tasksToActivate, deps...)
				}
			}

			// Investigating strange dispatch state as part of EVG-13144
			if t.IsHostTask() && !utility.IsZeroTime(t.DispatchTime) && t.Status == evergreen.TaskUndispatched {
				catcher.Wrapf(resetTask(ctx, t.Id, caller), "resetting task '%s'", t.Id)
			} else {
				tasksToActivate = append(tasksToActivate, originalTasks...)
			}

			// If the task was not activated by step back, and either the caller is not evergreen
			// or the task was originally activated by evergreen, deactivate the task
		} else if !evergreen.IsSystemActivator(caller) || evergreen.IsSystemActivator(t.ActivatedBy) {
			// deactivate later tasks in the group as well, since they won't succeed without this one
			if evergreen.IsCommitQueueRequester(t.Requester) {
				catcher.Wrapf(DequeueAndRestartForTask(ctx, nil, &t, message.GithubStateError, caller, fmt.Sprintf("deactivated by '%s'", caller)), "dequeueing and restarting task '%s'", t.Id)
			}
			tasksToActivate = append(tasksToActivate, originalTasks...)
		} else {
			continue
		}
	}

	if active {
		if err := task.ActivateTasks(tasksToActivate, time.Now(), true, caller); err != nil {
			return errors.Wrap(err, "activating tasks")
		}
		versionIdsToActivate := []string{}
		for v := range versionIdsSet {
			versionIdsToActivate = append(versionIdsToActivate, v)
		}
		if err := ActivateVersions(versionIdsToActivate); err != nil {
			return errors.Wrap(err, "marking version as activated")
		}
		buildIdsToActivate := []string{}
		for b := range buildToTaskMap {
			buildIdsToActivate = append(buildIdsToActivate, b)
		}
		if err := build.UpdateActivation(buildIdsToActivate, true, caller); err != nil {
			return errors.Wrap(err, "marking builds as activated")
		}
	} else {
		if err := task.DeactivateTasks(tasksToActivate, true, caller); err != nil {
			return errors.Wrap(err, "deactivating task")
		}
	}

	for _, t := range tasksToActivate {
		if t.IsPartOfDisplay() {
			catcher.Wrap(UpdateDisplayTaskForTask(&t), "updating display task")
		}
	}
	for b, item := range buildToTaskMap {
		t := buildToTaskMap[b]
		if err := UpdateBuildAndVersionStatusForTask(ctx, &item); err != nil {
			return errors.Wrapf(err, "updating build and version status for task '%s'", t.Id)
		}
	}

	return catcher.Resolve()
}

func SetActiveStateById(ctx context.Context, id, user string, active bool) error {
	t, err := task.FindOneId(id)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", id)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", id)
	}
	return SetActiveState(ctx, user, active, *t)
}

func DisableTasks(caller string, tasks ...task.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	tasksPresent := map[string]struct{}{}
	var taskIDs []string
	var execTaskIDs []string
	for _, t := range tasks {
		tasksPresent[t.Id] = struct{}{}
		taskIDs = append(taskIDs, t.Id)
		execTaskIDs = append(execTaskIDs, t.ExecutionTasks...)
	}

	_, err := task.UpdateAll(
		task.ByIds(append(taskIDs, execTaskIDs...)),
		bson.M{"$set": bson.M{task.PriorityKey: evergreen.DisabledTaskPriority}},
	)
	if err != nil {
		return errors.Wrap(err, "updating task priorities")
	}

	execTasks, err := findMissingTasks(execTaskIDs, tasksPresent)
	if err != nil {
		return errors.Wrap(err, "finding additional execution tasks")
	}
	tasks = append(tasks, execTasks...)

	for _, t := range tasks {
		t.Priority = evergreen.DisabledTaskPriority
		event.LogTaskPriority(t.Id, t.Execution, caller, evergreen.DisabledTaskPriority)
	}

	if err := task.DeactivateTasks(tasks, true, caller); err != nil {
		return errors.Wrap(err, "deactivating dependencies")
	}

	return nil
}

// findMissingTasks finds all tasks whose IDs are missing from tasksPresent.
func findMissingTasks(taskIDs []string, tasksPresent map[string]struct{}) ([]task.Task, error) {
	var missingTaskIDs []string
	for _, id := range taskIDs {
		if _, ok := tasksPresent[id]; ok {
			continue
		}
		missingTaskIDs = append(missingTaskIDs, id)
	}
	if len(missingTaskIDs) == 0 {
		return nil, nil
	}

	missingTasks, err := task.FindAll(db.Query(task.ByIds(missingTaskIDs)))
	if err != nil {
		return nil, err
	}

	return missingTasks, nil
}

// DisableStaleContainerTasks disables all container tasks that have been
// scheduled to run for a long time without actually dispatching the task.
func DisableStaleContainerTasks(caller string) error {
	query := task.ScheduledContainerTasksQuery()
	query[task.ActivatedTimeKey] = bson.M{"$lte": time.Now().Add(-task.UnschedulableThreshold)}

	tasks, err := task.FindAll(db.Query(query))
	if err != nil {
		return errors.Wrap(err, "finding tasks that need to be disabled")
	}

	grip.Info(message.Fields{
		"message":   "disabling container tasks that are still scheduled to run but are stale",
		"num_tasks": len(tasks),
		"caller":    caller,
	})

	if err := DisableTasks(caller, tasks...); err != nil {
		return errors.Wrap(err, "disabled stale container tasks")
	}

	return nil
}

// activatePreviousTask will set the Active state for the first task with a
// revision order number less than the current task's revision order number.
// originalStepbackTask is only specified if we're first activating the generator for a generated task.
// StepbackDepth should be reconsidered in EVG-17949 and is currently only used for logging.
// Depth passed in is the depth we should assign to the previous task.
func activatePreviousTask(ctx context.Context, taskId, caller string, originalStepbackTask *task.Task, stepbackDepth int) error {
	// find the task first
	t, err := task.FindOneId(taskId)
	if err != nil {
		return errors.WithStack(err)
	}
	if t == nil {
		return errors.Errorf("task '%s' does not exist", taskId)
	}

	// find previous task limiting to just the last one
	filter, sort := task.ByBeforeRevision(t.RevisionOrderNumber, t.BuildVariant, t.DisplayName, t.Project, t.Requester)
	query := db.Query(filter).Sort(sort)
	prevTask, err := task.FindOne(query)
	if err != nil {
		return errors.Wrap(err, "finding previous task")
	}

	// for generated tasks, try to activate the generator instead if the previous task we found isn't the actual last task
	if t.GeneratedBy != "" && prevTask != nil && prevTask.RevisionOrderNumber+1 != t.RevisionOrderNumber {
		return activatePreviousTask(ctx, t.GeneratedBy, caller, t, stepbackDepth)
	}

	// if this is the first time we're running the task, or it's finished, has a negative priority, or already activated
	if prevTask == nil || prevTask.IsFinished() || prevTask.Priority < 0 || prevTask.Activated {
		return nil
	}

	grip.Debug(message.Fields{
		"ticket":         "EVG-17949",
		"message":        "stepping back task",
		"stepback_depth": stepbackDepth,
		"project_id":     t.Project,
		"task_id":        t.Id,
	})

	// activate the task
	if err = SetActiveState(ctx, caller, true, *prevTask); err != nil {
		return errors.Wrapf(err, "setting task '%s' active", prevTask.Id)
	}
	// add the task that we're actually stepping back so that we know to activate it
	if prevTask.GenerateTask && originalStepbackTask != nil {
		if err = prevTask.SetGeneratedTasksToActivate(originalStepbackTask.BuildVariant, originalStepbackTask.DisplayName); err != nil {
			return errors.Wrap(err, "setting generated tasks to activate")
		}
	}
	if stepbackDepth > 0 {
		if err = prevTask.SetStepbackDepth(stepbackDepth); err != nil {
			return errors.Wrap(err, "setting stepback depth")
		}
	}
	return nil
}

func resetManyTasks(ctx context.Context, tasks []task.Task, caller string) error {
	catcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		catcher.Add(resetTask(ctx, t.Id, caller))
	}
	return catcher.Resolve()
}

// TryResetTask resets a task. Individual execution tasks cannot be reset - to
// reset an execution task, the given task ID must be that of its parent display
// task.
func TryResetTask(ctx context.Context, settings *evergreen.Settings, taskId, user, origin string, detail *apimodels.TaskEndDetail) error {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return errors.WithStack(err)
	}
	if t == nil {
		return errors.Errorf("cannot restart task '%s' because it could not be found", taskId)
	}
	if t.IsPartOfDisplay() {
		return errors.Errorf("cannot restart execution task '%s' because it is part of a display task", t.Id)
	}

	var execTask *task.Task

	maxExecution := evergreen.MaxTaskExecution

	if evergreen.IsCommitQueueRequester(t.Requester) && evergreen.IsSystemFailedTaskStatus(t.Status) {
		maxSystemFailedTaskRetries := settings.CommitQueue.MaxSystemFailedTaskRetries
		if maxSystemFailedTaskRetries > 0 {
			maxExecution = maxSystemFailedTaskRetries
		}
	}
	// if we've reached the max number of executions for this task, mark it as finished and failed
	if t.Execution >= maxExecution {
		// restarting from the UI bypasses the restart cap
		msg := fmt.Sprintf("task '%s' reached max execution %d: ", t.Id, maxExecution)
		if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
			grip.Debugln(msg, "allowing exception for", user)
		} else if !t.IsFinished() {
			if detail != nil {
				grip.Debugln(msg, "marking as failed")
				if t.DisplayOnly {
					for _, etId := range t.ExecutionTasks {
						execTask, err = task.FindOneId(etId)
						if err != nil {
							return errors.Wrap(err, "finding execution task")
						}
						if err = MarkEnd(ctx, settings, execTask, origin, time.Now(), detail, false); err != nil {
							return errors.Wrap(err, "marking execution task as ended")
						}
					}
				}
				return errors.WithStack(MarkEnd(ctx, settings, t, origin, time.Now(), detail, false))
			} else {
				grip.Critical(message.Fields{
					"message":     "TryResetTask called with nil TaskEndDetail",
					"origin":      origin,
					"task_id":     taskId,
					"task_status": t.Status,
				})
			}
		} else {
			return nil
		}
	}

	// only allow re-execution for failed or successful tasks
	if !t.IsFinished() {
		// this is to disallow terminating running tasks via the UI
		if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
			grip.Debugf("Unsatisfiable '%s' reset request on '%s' (status: '%s')",
				user, t.Id, t.Status)
			if t.DisplayOnly {
				execTasks := map[string]string{}
				for _, et := range t.ExecutionTasks {
					execTask, err = task.FindOneId(et)
					if err != nil {
						continue
					}
					execTasks[execTask.Id] = execTask.Status
				}
				grip.Error(message.Fields{
					"message":    "attempt to restart unfinished display task",
					"task":       t.Id,
					"status":     t.Status,
					"exec_tasks": execTasks,
				})
			}
			return errors.Errorf("task '%s' currently has status '%s' - cannot reset task in this status",
				t.Id, t.Status)
		}
	}

	if detail != nil {
		if err = t.MarkEnd(time.Now(), detail); err != nil {
			return errors.Wrap(err, "marking task as ended")
		}
	}

	caller := origin
	if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
		caller = user
	}
	if t.IsPartOfSingleHostTaskGroup() {
		if err = t.SetResetWhenFinished(); err != nil {
			return errors.Wrap(err, "marking task group for reset")
		}
		return errors.Wrap(checkResetSingleHostTaskGroup(ctx, t, caller), "resetting single host task group")
	}

	return errors.WithStack(resetTask(ctx, t.Id, caller))
}

// resetTask finds a finished task, attempts to archive it, and resets the task and
// resets the TaskCache in the build as well.
func resetTask(ctx context.Context, taskId, caller string) error {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return errors.WithStack(err)
	}
	if t.IsPartOfDisplay() {
		return errors.Errorf("cannot restart execution task '%s' because it is part of a display task", t.Id)
	}
	if err = t.Archive(); err != nil {
		return errors.Wrap(err, "can't restart task because it can't be archived")
	}

	if err = MarkOneTaskReset(ctx, t); err != nil {
		return errors.WithStack(err)
	}
	event.LogTaskRestarted(t.Id, t.Execution, caller)

	if err := t.ActivateTask(caller); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(UpdateBuildAndVersionStatusForTask(ctx, t))
}

func AbortTask(ctx context.Context, taskId, caller string) error {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return err
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", taskId)
	}
	if t.DisplayOnly {
		for _, et := range t.ExecutionTasks {
			_ = AbortTask(ctx, et, caller) // discard errors because some execution tasks may not be abortable
		}
	}

	if !t.IsAbortable() {
		return errors.Errorf("task '%s' currently has status '%s' - cannot abort task"+
			" in this status", t.Id, t.Status)
	}

	// set the active state and then set the abort
	if err = SetActiveState(ctx, caller, false, *t); err != nil {
		return err
	}
	event.LogTaskAbortRequest(t.Id, t.Execution, caller)
	return t.SetAborted(task.AbortInfo{User: caller})
}

// DeactivatePreviousTasks deactivates any previously activated but undispatched
// tasks for the same build variant + display name + project combination
// as the task, provided nothing is waiting on it.
func DeactivatePreviousTasks(ctx context.Context, t *task.Task, caller string) error {
	filter, sort := task.ByActivatedBeforeRevisionWithStatuses(
		t.RevisionOrderNumber,
		[]string{evergreen.TaskUndispatched},
		t.BuildVariant,
		t.DisplayName,
		t.Project,
	)
	query := db.Query(filter).Sort(sort)
	allTasks, err := task.FindAll(query)
	if err != nil {
		return errors.Wrapf(err, "finding previous tasks to deactivate for task '%s'", t.Id)
	}
	for _, t := range allTasks {
		// Only deactivate tasks that other tasks aren't waiting for.
		hasDependentTasks, err := task.HasActivatedDependentTasks(t.Id)
		if err != nil {
			return errors.Wrapf(err, "getting activated dependencies for '%s'", t.Id)
		}
		if !hasDependentTasks {
			if err = SetActiveState(ctx, caller, false, t); err != nil {
				return err
			}
		}
	}

	return nil
}

// Returns true if the task should stepback upon failure, and false
// otherwise. Note that the setting is obtained from the top-level
// project, if not explicitly set on the task or disabled at the project level.
func getStepback(taskId string) (bool, error) {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return false, errors.Wrapf(err, "finding task '%s'", taskId)
	}
	if t == nil {
		return false, errors.Errorf("task '%s' not found", taskId)
	}
	projectRef, err := FindMergedProjectRef(t.Project, "", false)
	if err != nil {
		return false, errors.Wrapf(err, "finding merged project ref for task '%s'", taskId)
	}
	if projectRef == nil {
		return false, errors.Errorf("project for task '%s' not found", taskId)
	}
	// Disabling the feature at the project level takes precedent.
	if projectRef.IsStepbackDisabled() {
		return false, nil
	}

	project, err := FindProjectFromVersionID(t.Version)
	if err != nil {
		return false, errors.WithStack(err)
	}

	projectTask := project.FindProjectTask(t.DisplayName)
	// Check if the task overrides the stepback policy specified by the project
	if projectTask != nil && projectTask.Stepback != nil {
		return *projectTask.Stepback, nil
	}

	// Check if the build variant overrides the stepback policy specified by the project
	for _, buildVariant := range project.BuildVariants {
		if t.BuildVariant == buildVariant.Name {
			if buildVariant.Stepback != nil {
				return *buildVariant.Stepback, nil
			}
			break
		}
	}
	return project.Stepback, nil
}

// doStepBack performs a stepback on the task if there is a previous task and if not it returns nothing.
func doStepback(ctx context.Context, t *task.Task) error {
	if t.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
		if err != nil {
			return errors.Wrapf(err, "finding tasks for stepback of '%s'", t.Id)
		}
		catcher := grip.NewSimpleCatcher()
		for _, et := range execTasks {
			catcher.Add(doStepback(ctx, &et))
		}
		if catcher.HasErrors() {
			return catcher.Resolve()
		}
	}

	//See if there is a prior success for this particular task.
	//If there isn't, we should not activate the previous task because
	//it could trigger stepping backwards ad infinitum.
	prevTask, err := t.PreviousCompletedTask(t.Project, []string{evergreen.TaskSucceeded})
	if err != nil {
		return errors.Wrap(err, "locating previous successful task")
	}
	if prevTask == nil {
		return nil
	}

	// activate the previous task to pinpoint regression
	return errors.WithStack(activatePreviousTask(ctx, t.Id, evergreen.StepbackTaskActivator, nil, t.StepbackDepth+1))
}

// MarkEnd updates the task as being finished, performs a stepback if necessary, and updates the build status
func MarkEnd(ctx context.Context, settings *evergreen.Settings, t *task.Task, caller string, finishTime time.Time, detail *apimodels.TaskEndDetail,
	deactivatePrevious bool) error {

	const slowThreshold = time.Second

	detailsCopy := *detail
	if t.ResultsFailed && detailsCopy.Status != evergreen.TaskFailed {
		detailsCopy.Type = evergreen.CommandTypeTest
		detailsCopy.Status = evergreen.TaskFailed
		detailsCopy.Description = evergreen.TaskDescriptionResultsFailed
	}

	if t.Status == detailsCopy.Status {
		grip.Warning(message.Fields{
			"message": "tried to mark task as finished twice",
			"task":    t.Id,
		})
		return nil
	}
	if detailsCopy.Status == evergreen.TaskSucceeded && t.MustHaveResults && !t.HasResults() {
		detailsCopy.Type = evergreen.CommandTypeTest
		detailsCopy.Status = evergreen.TaskFailed
		detailsCopy.Description = evergreen.TaskDescriptionNoResults
	}

	t.Details = detailsCopy
	if utility.IsZeroTime(t.StartTime) {
		grip.Warning(message.Fields{
			"message":      "task is missing start time",
			"task_id":      t.Id,
			"execution":    t.Execution,
			"requester":    t.Requester,
			"activated_by": t.ActivatedBy,
		})
	}
	startPhaseAt := time.Now()
	err := t.MarkEnd(finishTime, &detailsCopy)

	grip.NoticeWhen(time.Since(startPhaseAt) > slowThreshold, message.Fields{
		"message":       "slow operation",
		"function":      "MarkEnd",
		"step":          "t.MarkEnd",
		"task":          t.Id,
		"duration_secs": time.Since(startPhaseAt).Seconds(),
	})

	if err != nil {
		return errors.Wrap(err, "marking task finished")
	}

	if err = UpdateBlockedDependencies(t); err != nil {
		return errors.Wrap(err, "updating blocked dependencies")
	}

	if err = t.MarkDependenciesFinished(true); err != nil {
		return errors.Wrap(err, "updating dependency met status")
	}

	status := t.GetDisplayStatus()

	switch t.ExecutionPlatform {
	case task.ExecutionPlatformHost:
		event.LogHostTaskFinished(t.Id, t.Execution, t.HostId, status)
	case task.ExecutionPlatformContainer:
		event.LogContainerTaskFinished(t.Id, t.Execution, t.PodID, status)
	default:
		event.LogTaskFinished(t.Id, t.Execution, status)
	}

	grip.Info(message.Fields{
		"message":            "marking task finished",
		"included_on":        evergreen.ContainerHealthDashboard,
		"task_id":            t.Id,
		"execution":          t.Execution,
		"status":             status,
		"operation":          "MarkEnd",
		"host_id":            t.HostId,
		"pod_id":             t.PodID,
		"execution_platform": t.ExecutionPlatform,
	})

	if t.IsPartOfDisplay() {
		if err = UpdateDisplayTaskForTask(t); err != nil {
			return errors.Wrap(err, "updating display task")
		}
		dt, err := t.GetDisplayTask()
		if err != nil {
			return errors.Wrap(err, "getting display task")
		}
		if err = checkResetDisplayTask(ctx, settings, dt); err != nil {
			return errors.Wrap(err, "checking display task reset")
		}
	} else {
		if t.IsPartOfSingleHostTaskGroup() {
			if err = checkResetSingleHostTaskGroup(ctx, t, caller); err != nil {
				return errors.Wrap(err, "resetting task group")
			}
		}
	}

	// activate/deactivate other task if this is not a patch request's task
	if !evergreen.IsPatchRequester(t.Requester) {
		if t.IsPartOfDisplay() {
			_, err = t.GetDisplayTask()
			if err != nil {
				return errors.Wrap(err, "getting display task")
			}
			err = evalStepback(ctx, t.DisplayTask, caller, t.DisplayTask.Status, deactivatePrevious)
		} else {
			err = evalStepback(ctx, t, caller, status, deactivatePrevious)
		}
		if err != nil {
			return errors.Wrap(err, "evaluating stepback")
		}
	}

	if err = UpdateBuildAndVersionStatusForTask(ctx, t); err != nil {
		return errors.Wrap(err, "updating build/version status")
	}

	if err = logTaskEndStats(ctx, t); err != nil {
		return errors.Wrap(err, "logging task end stats")
	}

	if (t.ResetWhenFinished || t.ResetFailedWhenFinished) && !t.IsPartOfDisplay() && !t.IsPartOfSingleHostTaskGroup() {
		return TryResetTask(ctx, settings, t.Id, evergreen.APIServerTaskActivator, "", detail)
	}

	return nil
}

// logTaskEndStats logs information a task after it
// completes. It also logs information about the total runtime and instance
// type, which can be used to measure the cost of running a task.
func logTaskEndStats(ctx context.Context, t *task.Task) error {
	msg := message.Fields{
		"abort":                t.Aborted,
		"activated_by":         t.ActivatedBy,
		"build":                t.BuildId,
		"current_runtime_secs": t.FinishTime.Sub(t.StartTime).Seconds(),
		"display_task":         t.DisplayOnly,
		"execution":            t.Execution,
		"generator":            t.GenerateTask,
		"group":                t.TaskGroup,
		"group_max_hosts":      t.TaskGroupMaxHosts,
		"priority":             t.Priority,
		"project":              t.Project,
		"requester":            t.Requester,
		"stat":                 "task-end-stats",
		"status":               t.GetDisplayStatus(),
		"task":                 t.DisplayName,
		"task_id":              t.Id,
		"total_wait_secs":      t.FinishTime.Sub(t.ActivatedTime).Seconds(),
		"start_time":           t.StartTime,
		"scheduled_time":       t.ScheduledTime,
		"variant":              t.BuildVariant,
		"version":              t.Version,
	}

	if t.IsPartOfDisplay() {
		msg["display_task_id"] = t.DisplayTaskId
	}

	pRef, _ := FindBranchProjectRef(t.Project)
	if pRef != nil {
		msg["project_identifier"] = pRef.Identifier
	}

	isHostMode := t.IsHostTask()
	if isHostMode {
		taskHost, err := host.FindOneId(ctx, t.HostId)
		if err != nil {
			return err
		}
		if taskHost == nil {
			return errors.Errorf("host '%s' not found", t.HostId)
		}
		msg["host_id"] = taskHost.Id
		msg["distro"] = taskHost.Distro.Id
		msg["provider"] = taskHost.Distro.Provider
		if evergreen.IsEc2Provider(taskHost.Distro.Provider) && len(taskHost.Distro.ProviderSettingsList) > 0 {
			instanceType, ok := taskHost.Distro.ProviderSettingsList[0].Lookup("instance_type").StringValueOK()
			if ok {
				msg["instance_type"] = instanceType
			}
		}
	} else {
		taskPod, err := pod.FindOneByID(t.PodID)
		if err != nil {
			return errors.Wrapf(err, "finding pod '%s'", t.PodID)
		}
		if taskPod == nil {
			return errors.Errorf("pod '%s' not found", t.PodID)
		}
		msg["pod_id"] = taskPod.ID
		msg["pod_os"] = taskPod.TaskContainerCreationOpts.OS
		msg["pod_arch"] = taskPod.TaskContainerCreationOpts.Arch
		msg["cpu"] = taskPod.TaskContainerCreationOpts.CPU
		msg["memory_mb"] = taskPod.TaskContainerCreationOpts.MemoryMB
		if taskPod.TaskContainerCreationOpts.OS.Matches(evergreen.ECSOS(pod.OSWindows)) {
			msg["windows_version"] = taskPod.TaskContainerCreationOpts.WindowsVersion
		}
	}

	if !t.DependenciesMetTime.IsZero() {
		msg["dependencies_met_time"] = t.DependenciesMetTime
	}

	if !t.ContainerAllocatedTime.IsZero() {
		msg["container_allocated_time"] = t.ContainerAllocatedTime
	}

	grip.Info(msg)
	return nil
}

// UpdateBlockedDependencies traverses the dependency graph and recursively sets
// each parent dependency as unattainable in depending tasks. It updates the
// status of builds as well, in case they change due to blocking dependencies.
func UpdateBlockedDependencies(t *task.Task) error {
	dependentTasks, err := t.FindAllUnmarkedBlockedDependencies()
	if err != nil {
		return errors.Wrapf(err, "getting tasks depending on task '%s'", t.Id)
	}

	buildIDsSet := make(map[string]struct{})
	for _, dependentTask := range dependentTasks {
		if err = dependentTask.MarkUnattainableDependency(t.Id, true); err != nil {
			return errors.Wrap(err, "marking dependency unattainable")
		}
		if err = UpdateBlockedDependencies(&dependentTask); err != nil {
			return errors.Wrapf(err, "updating blocked dependencies for '%s'", t.Id)
		}
		buildIDsSet[dependentTask.BuildId] = struct{}{}
	}

	var buildIDs []string
	for buildID := range buildIDsSet {
		buildIDs = append(buildIDs, buildID)
	}
	if err = UpdateVersionAndPatchStatusForBuilds(buildIDs); err != nil {
		return errors.Wrap(err, "updating build, version, and patch statuses")
	}

	return nil
}

// UpdateUnblockedDependencies recursively marks all unattainable dependencies as attainable.
func UpdateUnblockedDependencies(t *task.Task) error {
	blockedTasks, err := t.FindAllMarkedUnattainableDependencies()
	if err != nil {
		return errors.Wrap(err, "getting dependencies marked unattainable")
	}

	buildsToUpdate := make(map[string]bool)
	for _, blockedTask := range blockedTasks {
		if err = blockedTask.MarkUnattainableDependency(t.Id, false); err != nil {
			return errors.Wrap(err, "marking dependency attainable")
		}

		if err := UpdateUnblockedDependencies(&blockedTask); err != nil {
			return errors.WithStack(err)
		}

		buildsToUpdate[blockedTask.BuildId] = true
	}

	var buildIDs []string
	for buildID := range buildsToUpdate {
		buildIDs = append(buildIDs, buildID)
	}
	if err := UpdateVersionAndPatchStatusForBuilds(buildIDs); err != nil {
		return errors.Wrapf(err, "updating build, version, and patch statuses")
	}

	return nil
}

func RestartItemsAfterVersion(ctx context.Context, cq *commitqueue.CommitQueue, project, version, caller string) error {
	if cq == nil {
		var err error
		cq, err = commitqueue.FindOneId(project)
		if err != nil {
			return errors.Wrapf(err, "getting commit queue for project '%s'", project)
		}
		if cq == nil {
			return errors.Errorf("commit queue for project '%s' not found", project)
		}
	}

	foundItem := false
	catcher := grip.NewBasicCatcher()
	for _, item := range cq.Queue {
		if item.Version == "" {
			return nil
		}
		if item.Version == version {
			foundItem = true
		} else if foundItem && item.Version != "" {
			grip.Info(message.Fields{
				"message":            "restarting items due to commit queue failure",
				"failing_version":    version,
				"restarting_version": item.Version,
				"project":            project,
				"caller":             caller,
			})
			// this block executes on all items after the given task
			catcher.Add(RestartTasksInVersion(ctx, item.Version, true, caller))
		}
	}

	return catcher.Resolve()
}

// DequeueAndRestartForTask restarts all items after the given task's version,
// aborts/dequeues the current version, and sends an updated status to GitHub.
func DequeueAndRestartForTask(ctx context.Context, cq *commitqueue.CommitQueue, t *task.Task, githubState message.GithubState, caller, reason string) error {
	mergeErrMsg := fmt.Sprintf("commit queue item '%s' is being dequeued: %s", t.Version, reason)
	if t.Details.Type == evergreen.CommandTypeSetup {
		// If the commit queue merge task failed on setup, there is likely a merge conflict.
		mergeErrMsg = "Merge task failed on setup, which likely means a merge conflict was introduced. Please try merging with the base branch."
	}

	_, err := dequeueAndRestartItem(ctx, dequeueAndRestartOptions{
		cq:            cq,
		projectID:     t.Project,
		itemVersionID: t.Version,
		taskID:        t.Id,
		caller:        caller,
		reason:        reason,
		mergeErrMsg:   mergeErrMsg,
		githubStatus:  githubState,
	})
	return err
}

// DequeueAndRestartForVersion restarts all items after the commit queue
// item, aborts/dequeues this version, and sends an updated status to GitHub. If
// it succeeds, it returns the removed item.
func DequeueAndRestartForVersion(ctx context.Context, cq *commitqueue.CommitQueue, project, version, user, reason string) (*commitqueue.CommitQueueItem, error) {
	return dequeueAndRestartItem(ctx, dequeueAndRestartOptions{
		cq:            cq,
		projectID:     project,
		itemVersionID: version,
		caller:        user,
		reason:        reason,
		mergeErrMsg:   fmt.Sprintf("commit queue item '%s' is being dequeued: %s", version, reason),
		githubStatus:  message.GithubStateFailure,
	})
}

type dequeueAndRestartOptions struct {
	cq            *commitqueue.CommitQueue
	projectID     string
	itemVersionID string
	taskID        string
	caller        string
	reason        string
	mergeErrMsg   string
	githubStatus  message.GithubState
}

func dequeueAndRestartItem(ctx context.Context, opts dequeueAndRestartOptions) (*commitqueue.CommitQueueItem, error) {
	if opts.cq == nil {
		cq, err := commitqueue.FindOneId(opts.projectID)
		if err != nil {
			return nil, errors.Wrapf(err, "getting commit queue for project '%s'", opts.projectID)
		}
		if cq == nil {
			return nil, errors.Errorf("commit queue for project '%s' not found", opts.projectID)
		}
		opts.cq = cq
	}

	// Restart later items before dequeueing this item so that we know which
	// entries to restart.
	if err := RestartItemsAfterVersion(ctx, opts.cq, opts.projectID, opts.itemVersionID, opts.caller); err != nil {
		return nil, errors.Wrap(err, "restarting later commit queue items")
	}

	p, err := patch.FindOneId(opts.itemVersionID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding patch '%s'", opts.itemVersionID)
	}
	if p == nil {
		return nil, errors.Errorf("patch '%s' not found", opts.itemVersionID)
	}

	removed, err := tryDequeueAndAbortCommitQueueItem(p, *opts.cq, opts.taskID, opts.mergeErrMsg, opts.caller)
	if err != nil {
		return nil, errors.Wrapf(err, "dequeueing and aborting commit queue item '%s'", opts.itemVersionID)
	}

	grip.Info(message.Fields{
		"message":      "commit queue item was dequeued and later items were restarted",
		"source":       "commit queue",
		"reason":       opts.reason,
		"caller":       opts.caller,
		"project":      opts.projectID,
		"issue":        removed.Issue,
		"patch":        removed.PatchId,
		"version":      opts.itemVersionID,
		"task":         opts.taskID,
		"queue_length": len(opts.cq.Queue),
	})

	if err := SendCommitQueueResult(ctx, p, opts.githubStatus, opts.reason); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "unable to send GitHub status",
			"patch":   p.Id.Hex(),
		}))
	}

	return removed, nil
}

// HandleEndTaskForCommitQueueTask handles necessary dequeues and stepback restarts for
// ending tasks that run on a commit queue.
func HandleEndTaskForCommitQueueTask(ctx context.Context, t *task.Task, status string) error {
	cq, err := commitqueue.FindOneId(t.Project)
	if err != nil {
		return errors.Wrapf(err, "can't get commit queue for id '%s'", t.Project)
	}
	if cq == nil {
		return errors.Errorf("no commit queue found for '%s'", t.Project)
	}

	if status != evergreen.TaskSucceeded && !t.Aborted {
		return dequeueAndRestartWithStepback(ctx, cq, t, evergreen.MergeTestRequester, fmt.Sprintf("task '%s' failed", t.DisplayName))
	} else if status == evergreen.TaskSucceeded {
		// Query for all cq version tasks after this one; they may have been waiting to see if this
		// one was the cause of the failure, in which case we should dequeue and restart.
		foundVersion := false
		for _, item := range cq.Queue {
			if item.Version == "" {
				return nil // no longer looking at scheduled versions
			}
			if item.Version == t.Version {
				foundVersion = true
				continue
			}
			if foundVersion {
				laterTask, err := task.FindTaskForVersion(item.Version, t.DisplayName, t.BuildVariant)
				if err != nil {
					return errors.Wrapf(err, "error finding task for version '%s'", item.Version)
				}
				if laterTask == nil {
					return errors.Errorf("couldn't find task for version '%s'", item.Version)
				}
				if evergreen.IsFailedTaskStatus(laterTask.Status) {
					// Because our task is successful, this task should have failed so we dequeue.
					return dequeueAndRestartWithStepback(ctx, cq, laterTask, evergreen.APIServerTaskActivator,
						fmt.Sprintf("task '%s' failed and was not impacted by previous task", t.DisplayName))
				}
				if !evergreen.IsFinishedTaskStatus(laterTask.Status) {
					// When this task finishes, it will handle stepping back any later commit queue item, so we're done.
					return nil
				}
			}
		}
	}
	return nil
}

// dequeueAndRestartWithStepback dequeues the current task and restarts later tasks, if earlier tasks have all run.
// Otherwise, the failure may be a result of those untested commits so we will wait for the earlier tasks to run
// and handle dequeuing (merge still won't run for failed task versions because of dependencies).
func dequeueAndRestartWithStepback(ctx context.Context, cq *commitqueue.CommitQueue, t *task.Task, caller, reason string) error {
	if i := cq.FindItem(t.Version); i > 0 {
		prevVersions := []string{}
		for j := 0; j < i; j++ {
			prevVersions = append(prevVersions, cq.Queue[j].Version)
		}
		// if any of the commit queue tasks higher on the queue haven't finished, then they will handle dequeuing.
		previousTaskNeedsToRun, err := task.HasUnfinishedTaskForVersions(prevVersions, t.DisplayName, t.BuildVariant)
		if err != nil {
			return errors.Wrap(err, "error checking early commit queue tasks")
		}
		if previousTaskNeedsToRun {
			return nil
		}
		// Otherwise, continue on and dequeue.
	}
	return DequeueAndRestartForTask(ctx, cq, t, message.GithubStateFailure, caller, reason)
}

func tryDequeueAndAbortCommitQueueItem(p *patch.Patch, cq commitqueue.CommitQueue, taskID, mergeErrMsg string, caller string) (*commitqueue.CommitQueueItem, error) {
	issue := p.Id.Hex()
	err := removeNextMergeTaskDependency(cq, issue)
	grip.Error(message.WrapError(err, message.Fields{
		"message": "error removing dependency",
		"patch":   issue,
	}))

	removed, err := RemoveItemAndPreventMerge(&cq, issue, caller)
	grip.Debug(message.Fields{
		"message": "removing commit queue item",
		"issue":   issue,
		"err":     err,
		"removed": removed,
		"caller":  caller,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "removing and preventing merge for item '%s' from queue '%s'", issue, p.Project)
	}
	if removed == nil {
		return nil, errors.Errorf("no commit queue entry removed for issue '%s'", issue)
	}

	event.LogCommitQueueConcludeWithErrorMessage(p.Id.Hex(), evergreen.MergeTestFailed, mergeErrMsg)
	if err := CancelPatch(p, task.AbortInfo{TaskID: taskID, User: caller}); err != nil {
		return nil, errors.Wrap(err, "aborting failed commit queue patch")
	}

	return removed, nil
}

// removeNextMergeTaskDependency basically removes the given merge task from a linked list of
// merge task dependencies. It makes the next merge not depend on the current one and also makes
// the next merge depend on the previous one, if there is one
func removeNextMergeTaskDependency(cq commitqueue.CommitQueue, currentIssue string) error {
	currentIndex := cq.FindItem(currentIssue)
	if currentIndex < 0 {
		return errors.New("commit queue item not found")
	}
	if currentIndex+1 >= len(cq.Queue) {
		return nil
	}

	nextItem := cq.Queue[currentIndex+1]
	if nextItem.Version == "" {
		return nil
	}
	nextMerge, err := task.FindMergeTaskForVersion(nextItem.Version)
	if err != nil {
		return errors.Wrap(err, "finding next merge task")
	}
	if nextMerge == nil {
		return errors.New("no merge task found")
	}
	currentMerge, err := task.FindMergeTaskForVersion(cq.Queue[currentIndex].Version)
	if err != nil {
		return errors.Wrap(err, "finding current merge task")
	}
	if err = nextMerge.RemoveDependency(currentMerge.Id); err != nil {
		return errors.Wrap(err, "removing dependency")
	}

	if currentIndex > 0 {
		prevItem := cq.Queue[currentIndex-1]
		prevMerge, err := task.FindMergeTaskForVersion(prevItem.Version)
		if err != nil {
			return errors.Wrap(err, "finding previous merge task")
		}
		if prevMerge == nil {
			return errors.New("no merge task found")
		}
		d := task.Dependency{
			TaskId: prevMerge.Id,
			Status: AllStatuses,
		}
		if err = nextMerge.AddDependency(d); err != nil {
			return errors.Wrap(err, "adding dependency")
		}
	}

	return nil
}

func evalStepback(ctx context.Context, t *task.Task, caller, status string, deactivatePrevious bool) error {
	// Stepback if the task failed regularly _or_ if we are currently stepping back and we encountered any failure.
	if (status == evergreen.TaskFailed && !t.Aborted) ||
		(evergreen.IsFailedTaskStatus(status) && t.ActivatedBy == evergreen.StepbackTaskActivator) {
		var shouldStepBack bool
		shouldStepBack, err := getStepback(t.Id)
		if err != nil {
			return errors.WithStack(err)
		}
		if !shouldStepBack {
			return nil
		}

		if t.IsPartOfSingleHostTaskGroup() {
			// Stepback earlier task group tasks as well because these need to be run sequentially.
			catcher := grip.NewBasicCatcher()
			tasks, err := task.FindTaskGroupFromBuild(t.BuildId, t.TaskGroup)
			if err != nil {
				return errors.Wrapf(err, "getting task group for task '%s'", t.Id)
			}
			if len(tasks) == 0 {
				return errors.Errorf("no tasks in task group '%s' for task '%s'", t.TaskGroup, t.Id)
			}
			for _, tgTask := range tasks {
				catcher.Wrapf(doStepback(ctx, &tgTask), "stepping back task group task '%s'", tgTask.DisplayName)
				if tgTask.Id == t.Id {
					break // don't need to stepback later tasks in the group
				}
			}

			return catcher.Resolve()
		}
		return errors.Wrap(doStepback(ctx, t), "performing stepback")

	} else if status == evergreen.TaskSucceeded && deactivatePrevious && t.Requester == evergreen.RepotrackerVersionRequester {
		// if the task was successful and is a mainline commit (not git tag or project trigger),
		// ignore running previous activated tasks for this buildvariant
		if err := DeactivatePreviousTasks(ctx, t, caller); err != nil {
			return errors.Wrap(err, "deactivating previous task")
		}
	}

	return nil
}

// updateMakespans updates the predicted and actual makespans for the tasks in
// the build.
func updateMakespans(b *build.Build, buildTasks []task.Task) error {
	depPath := FindPredictedMakespan(buildTasks)
	return errors.WithStack(b.UpdateMakespans(depPath.TotalTime, CalculateActualMakespan(buildTasks)))
}

type buildStatus struct {
	status              string
	allTasksBlocked     bool
	allTasksUnscheduled bool
	// hasUnfinishedEssentialTask indicates if the build has at least one
	// essential tasks which is not finished. If so, the build is not finished.
	hasUnfinishedEssentialTask bool
}

// getBuildStatus returns a string denoting the status of the build based on the
// state of its constituent tasks.
func getBuildStatus(buildTasks []task.Task) buildStatus {
	// Check if no tasks have started and if all tasks are blocked.
	noStartedTasks := true
	allTasksBlocked := true
	allTasksUnscheduled := true
	var hasUnfinishedEssentialTask bool
	for _, t := range buildTasks {
		if t.IsEssentialToSucceed && !t.IsFinished() {
			hasUnfinishedEssentialTask = true
		}
		if !t.IsUnscheduled() {
			allTasksUnscheduled = false
		}
		if !evergreen.IsUnstartedTaskStatus(t.Status) {
			noStartedTasks = false
			allTasksBlocked = false
		}
		if !t.Blocked() {
			allTasksBlocked = false
		}
	}

	if allTasksUnscheduled {
		return buildStatus{
			status:                     evergreen.BuildCreated,
			allTasksBlocked:            allTasksBlocked,
			allTasksUnscheduled:        allTasksUnscheduled,
			hasUnfinishedEssentialTask: hasUnfinishedEssentialTask,
		}
	}

	if noStartedTasks || allTasksBlocked {
		return buildStatus{
			status:                     evergreen.BuildCreated,
			allTasksBlocked:            allTasksBlocked,
			hasUnfinishedEssentialTask: hasUnfinishedEssentialTask,
		}
	}

	var hasUnfinishedTask bool
	for _, t := range buildTasks {
		if t.WillRun() || t.IsInProgress() {
			hasUnfinishedTask = true
		}
	}
	if hasUnfinishedTask {
		return buildStatus{
			status:                     evergreen.BuildStarted,
			hasUnfinishedEssentialTask: hasUnfinishedEssentialTask,
		}
	}

	// Check if tasks are finished but failed.
	for _, t := range buildTasks {
		if evergreen.IsFailedTaskStatus(t.Status) || t.Aborted {
			return buildStatus{
				status:                     evergreen.BuildFailed,
				hasUnfinishedEssentialTask: hasUnfinishedEssentialTask,
			}
		}
	}

	if hasUnfinishedEssentialTask {
		// If there are only successful and unfinished essential tasks, prevent
		// the build from being marked successful because the essential tasks
		// must run.
		return buildStatus{
			status:                     evergreen.BuildStarted,
			hasUnfinishedEssentialTask: hasUnfinishedEssentialTask,
		}
	}

	return buildStatus{
		status: evergreen.BuildSucceeded,
	}
}

// updateBuildGithubStatus updates the GitHub check status for a build. If the
// build has no GitHub checks, then it is a no-op. Note that this is for GitHub
// checks, which are *not* the same as GitHub PR statuses.
func updateBuildGithubStatus(b *build.Build, buildTasks []task.Task) error {
	githubStatusTasks := make([]task.Task, 0, len(buildTasks))
	for _, t := range buildTasks {
		if t.IsGithubCheck {
			githubStatusTasks = append(githubStatusTasks, t)
		}
	}
	if len(githubStatusTasks) == 0 {
		return nil
	}

	buildStatus := getBuildStatus(githubStatusTasks)

	if buildStatus.status == b.GithubCheckStatus {
		return nil
	}

	if evergreen.IsFinishedBuildStatus(buildStatus.status) {
		event.LogBuildGithubCheckFinishedEvent(b.Id, buildStatus.status)
	}

	return b.UpdateGithubCheckStatus(buildStatus.status)
}

// checkUpdateBuildPRStatusPending checks if the build is coming from a PR, and if so
// sends a pending status to GitHub to reflect the status of the build.
func checkUpdateBuildPRStatusPending(ctx context.Context, b *build.Build) error {
	if !evergreen.IsGitHubPatchRequester(b.Requester) {
		return nil
	}
	p, err := patch.FindOneId(b.Version)
	if err != nil {
		return errors.Wrapf(err, "finding patch '%s'", b.Version)
	}
	if p == nil {
		return errors.Errorf("patch '%s' not found", b.Version)
	}
	if p.IsGithubPRPatch() && !evergreen.IsFinishedBuildStatus(b.Status) {
		input := thirdparty.SendGithubStatusInput{
			VersionId: p.Id.Hex(),
			Owner:     p.GithubPatchData.BaseOwner,
			Repo:      p.GithubPatchData.BaseRepo,
			Ref:       p.GithubPatchData.HeadHash,
			Desc:      "patch status change",
			Caller:    "pr-task-reset",
			Context:   fmt.Sprintf("evergreen/%s", b.BuildVariant),
		}
		if err = thirdparty.SendPendingStatusToGithub(ctx, input, ""); err != nil {
			return errors.Wrapf(err, "sending patch '%s' status to GitHub", p.Id.Hex())
		}
	}
	return nil
}

// updateBuildStatus updates the status of the build based on its tasks' statuses
// Returns true if the build's status has changed or if all the build's tasks become blocked / unscheduled.
func updateBuildStatus(b *build.Build) (bool, error) {
	buildTasks, err := task.FindWithFields(task.ByBuildId(b.Id), task.StatusKey, task.ActivatedKey, task.DependsOnKey, task.IsGithubCheckKey, task.AbortedKey, task.IsEssentialToSucceedKey)
	if err != nil {
		return false, errors.Wrapf(err, "getting tasks in build '%s'", b.Id)
	}

	buildStatus := getBuildStatus(buildTasks)
	// If all the tasks are unscheduled, set active to false
	if buildStatus.allTasksUnscheduled {
		if err = b.SetActivated(false); err != nil {
			return true, errors.Wrapf(err, "setting build '%s' as inactive", b.Id)
		}
		return true, nil
	}

	if err := b.SetHasUnfinishedEssentialTask(buildStatus.hasUnfinishedEssentialTask); err != nil {
		return false, errors.Wrapf(err, "setting unfinished essential task state to %t for build '%s'", buildStatus.hasUnfinishedEssentialTask, b.Id)
	}

	blockedChanged := buildStatus.allTasksBlocked != b.AllTasksBlocked

	if err = b.SetAllTasksBlocked(buildStatus.allTasksBlocked); err != nil {
		return false, errors.Wrapf(err, "setting build '%s' as blocked", b.Id)
	}

	if buildStatus.status == b.Status {
		return blockedChanged, nil
	}

	// Only check aborted if status has changed.
	isAborted := false
	var taskStatuses []string
	for _, t := range buildTasks {
		if t.Aborted {
			isAborted = true
		} else {
			taskStatuses = append(taskStatuses, t.Status)
		}
	}
	isAborted = len(utility.StringSliceIntersection(taskStatuses, evergreen.TaskFailureStatuses)) == 0 && isAborted
	if isAborted != b.Aborted {
		if err = b.SetAborted(isAborted); err != nil {
			return false, errors.Wrapf(err, "setting build '%s' as aborted", b.Id)
		}
	}

	event.LogBuildStateChangeEvent(b.Id, buildStatus.status)

	shouldActivate := !buildStatus.allTasksBlocked && !buildStatus.allTasksUnscheduled

	// if the status has changed, re-activate the build if it's not blocked
	if shouldActivate {
		if err = b.SetActivated(true); err != nil {
			return true, errors.Wrapf(err, "setting build '%s' as active", b.Id)
		}
	}

	if evergreen.IsFinishedBuildStatus(buildStatus.status) {
		if err = b.MarkFinished(buildStatus.status, time.Now()); err != nil {
			return true, errors.Wrapf(err, "marking build as finished with status '%s'", buildStatus.status)
		}
		if err = updateMakespans(b, buildTasks); err != nil {
			return true, errors.Wrapf(err, "updating makespan information for '%s'", b.Id)
		}
	} else {
		if err = b.UpdateStatus(buildStatus.status); err != nil {
			return true, errors.Wrap(err, "updating build status")
		}
	}

	if err = updateBuildGithubStatus(b, buildTasks); err != nil {
		return true, errors.Wrap(err, "updating build GitHub status")
	}

	return true, nil
}

// getVersionActivationAndStatus returns if the version is activated, as well as its status.
// Need to differentiate activated to distinguish between a version that's created
// but will run vs a version that's created but nothing is scheduled.
func getVersionActivationAndStatus(builds []build.Build) (bool, string) {
	// Check if no builds have started in the version.
	noStartedBuilds := true
	versionActivated := false
	for _, b := range builds {
		if b.Activated {
			versionActivated = true
		}
		if b.Status != evergreen.BuildCreated {
			noStartedBuilds = false
			break
		}
	}
	if noStartedBuilds {
		return versionActivated, evergreen.VersionCreated
	}

	var hasUnfinishedEssentialTask bool
	// Check if builds are started but not finished.
	for _, b := range builds {
		if b.Activated && !evergreen.IsFinishedBuildStatus(b.Status) && !b.AllTasksBlocked {
			return true, evergreen.VersionStarted
		}
		if b.HasUnfinishedEssentialTask {
			hasUnfinishedEssentialTask = true
		}
	}

	// Check if all builds are finished but have failures.
	for _, b := range builds {
		if b.Status == evergreen.BuildFailed || b.Aborted {
			return true, evergreen.VersionFailed
		}
	}

	if hasUnfinishedEssentialTask {
		// If there are only successful and unfinished essential builds, prevent
		// the version from being marked successful because the essential tasks
		// must run.
		return true, evergreen.VersionStarted
	}

	return true, evergreen.VersionSucceeded
}

// updateVersionStatus updates the status of the version based on the status of
// its constituent builds. It assumes that the build statuses have already
// been updated prior to this.
func updateVersionGithubStatus(v *Version, builds []build.Build) error {
	githubStatusBuilds := make([]build.Build, 0, len(builds))
	for _, b := range builds {
		if b.IsGithubCheck {
			b.Status = b.GithubCheckStatus
			githubStatusBuilds = append(githubStatusBuilds, b)
		}
	}
	if len(githubStatusBuilds) == 0 {
		return nil
	}

	_, githubBuildStatus := getVersionActivationAndStatus(githubStatusBuilds)

	if evergreen.IsFinishedBuildStatus(githubBuildStatus) {
		event.LogVersionGithubCheckFinishedEvent(v.Id, githubBuildStatus)
	}

	return nil
}

// updateVersionStatus updates the status of the version based on the status of
// its constituent builds, as well as a boolean indicating if any of them have
// unfinished essential tasks. It assumes that the build statuses have already
// been updated prior to this.
func updateVersionStatus(v *Version) (string, error) {
	builds, err := build.Find(build.ByVersion(v.Id).WithFields(build.ActivatedKey, build.StatusKey,
		build.IsGithubCheckKey, build.GithubCheckStatusKey, build.AbortedKey, build.AllTasksBlockedKey, build.HasUnfinishedEssentialTaskKey))
	if err != nil {
		return "", errors.Wrapf(err, "getting builds for version '%s'", v.Id)
	}

	// Regardless of whether the overall version status has changed, the Github status subset may have changed.
	if err = updateVersionGithubStatus(v, builds); err != nil {
		return "", errors.Wrap(err, "updating version GitHub status")
	}

	versionActivated, versionStatus := getVersionActivationAndStatus(builds)
	// If all the builds are unscheduled and nothing has run, set active to false
	if versionStatus == evergreen.VersionCreated && !versionActivated {
		if err = v.SetActivated(false); err != nil {
			return "", errors.Wrapf(err, "setting version '%s' as inactive", v.Id)
		}
	}

	if versionStatus == v.Status {
		return versionStatus, nil
	}

	// only need to check aborted if status has changed
	isAborted := false
	for _, b := range builds {
		if b.Aborted {
			isAborted = true
			break
		}
	}
	if isAborted != v.Aborted {
		if err = v.SetAborted(isAborted); err != nil {
			return "", errors.Wrapf(err, "setting version '%s' as aborted", v.Id)
		}
	}

	event.LogVersionStateChangeEvent(v.Id, versionStatus)

	if evergreen.IsFinishedVersionStatus(versionStatus) {
		if err = v.MarkFinished(versionStatus, time.Now()); err != nil {
			return "", errors.Wrapf(err, "marking version '%s' as finished with status '%s'", v.Id, versionStatus)
		}
	} else {
		if err = v.UpdateStatus(versionStatus); err != nil {
			return "", errors.Wrapf(err, "updating version '%s' with status '%s'", v.Id, versionStatus)
		}
	}

	return versionStatus, nil
}

// UpdatePatchStatus updates the status of a patch.
func UpdatePatchStatus(p *patch.Patch, versionStatus string) error {
	patchStatus := evergreen.VersionStatusToPatchStatus(versionStatus)

	if patchStatus == p.Status {
		return nil
	}

	event.LogPatchStateChangeEvent(p.Version, patchStatus)

	if evergreen.IsFinishedVersionStatus(patchStatus) {
		if err := p.MarkFinished(patchStatus, time.Now()); err != nil {
			return errors.Wrapf(err, "marking patch '%s' as finished with status '%s'", p.Id.Hex(), patchStatus)
		}
	} else if err := p.UpdateStatus(patchStatus); err != nil {
		return errors.Wrapf(err, "updating patch '%s' with status '%s'", p.Id.Hex(), patchStatus)
	}

	isDone, parentPatch, err := p.GetFamilyInformation()
	if err != nil {
		return errors.Wrapf(err, "getting family information for patch '%s'", p.Id.Hex())
	}
	if isDone {
		collectiveStatus, err := p.CollectiveStatus()
		if err != nil {
			return errors.Wrapf(err, "getting collective status for patch '%s'", p.Id.Hex())
		}
		if parentPatch != nil {
			event.LogPatchChildrenCompletionEvent(parentPatch.Id.Hex(), collectiveStatus, parentPatch.Author)
		} else {
			event.LogPatchChildrenCompletionEvent(p.Id.Hex(), collectiveStatus, p.Author)
		}
	}

	return nil
}

// UpdateBuildAndVersionStatusForTask updates the status of the task's build based on all the tasks in the build
// and the task's version based on all the builds in the version.
// Also update build and version Github statuses based on the subset of tasks and builds included in github checks
func UpdateBuildAndVersionStatusForTask(ctx context.Context, t *task.Task) error {
	taskBuild, err := build.FindOneId(t.BuildId)
	if err != nil {
		return errors.Wrapf(err, "getting build for task '%s'", t.Id)
	}
	if taskBuild == nil {
		return errors.Errorf("no build '%s' found for task '%s'", t.BuildId, t.Id)
	}
	buildStatusChanged, err := updateBuildStatus(taskBuild)
	if err != nil {
		return errors.Wrapf(err, "updating build '%s' status", taskBuild.Id)
	}
	// If the build status and activation have not changed,
	// then the version and patch statuses and activation must have also not changed.
	if !buildStatusChanged {
		return nil
	}

	taskVersion, err := VersionFindOneId(t.Version)
	if err != nil {
		return errors.Wrapf(err, "getting version '%s' for task '%s'", t.Version, t.Id)
	}
	if taskVersion == nil {
		return errors.Errorf("no version '%s' found for task '%s'", t.Version, t.Id)
	}

	newVersionStatus, err := updateVersionStatus(taskVersion)
	if err != nil {
		return errors.Wrapf(err, "updating version '%s' status", taskVersion.Id)
	}

	if !evergreen.IsFinishedVersionStatus(newVersionStatus) && evergreen.IsFinishedVersionStatus(taskVersion.Status) {
		if err = checkUpdateBuildPRStatusPending(ctx, taskBuild); err != nil {
			return errors.Wrapf(err, "updating build '%s' PR status", taskBuild.Id)
		}
	}

	if evergreen.IsPatchRequester(taskVersion.Requester) {
		p, err := patch.FindOneId(taskVersion.Id)
		if err != nil {
			return errors.Wrapf(err, "getting patch for version '%s'", taskVersion.Id)
		}
		if p == nil {
			return errors.Errorf("no patch found for version '%s'", taskVersion.Id)
		}
		if err = UpdatePatchStatus(p, newVersionStatus); err != nil {
			return errors.Wrapf(err, "updating patch '%s' status", p.Id.Hex())
		}

		isDone, parentPatch, err := p.GetFamilyInformation()
		if err != nil {
			return errors.Wrapf(err, "getting family information for patch '%s'", p.Id.Hex())
		}
		if isDone {
			collectiveStatus, err := p.CollectiveStatus()
			if err != nil {
				return errors.Wrapf(err, "getting collective status for patch '%s'", p.Id.Hex())
			}
			versionStatus := evergreen.PatchStatusToVersionStatus(collectiveStatus)
			if parentPatch != nil {
				event.LogVersionChildrenCompletionEvent(parentPatch.Id.Hex(), versionStatus, parentPatch.Author)
			} else {
				event.LogVersionChildrenCompletionEvent(p.Id.Hex(), versionStatus, p.Author)
			}

		}

	}

	return nil
}

// UpdateVersionAndPatchStatusForBuilds updates the status of all versions,
// patches and builds associated with the given input list of build IDs.
func UpdateVersionAndPatchStatusForBuilds(buildIds []string) error {
	if len(buildIds) == 0 {
		return nil
	}
	builds, err := build.Find(build.ByIds(buildIds))
	if err != nil {
		return errors.Wrapf(err, "fetching builds")
	}

	versionsToUpdate := make(map[string]bool)
	for _, build := range builds {
		buildStatusChanged, err := updateBuildStatus(&build)
		if err != nil {
			return errors.Wrapf(err, "updating build '%s' status", build.Id)
		}
		// If no build has changed status, then we can assume the version and patch statuses have also stayed the same.
		if !buildStatusChanged {
			continue
		}
		versionsToUpdate[build.Version] = true
	}
	for versionId := range versionsToUpdate {
		buildVersion, err := VersionFindOneId(versionId)
		if err != nil {
			return errors.Wrapf(err, "getting version '%s'", versionId)
		}
		if buildVersion == nil {
			return errors.Errorf("no version '%s' found", versionId)
		}
		newVersionStatus, err := updateVersionStatus(buildVersion)
		if err != nil {
			return errors.Wrapf(err, "updating version '%s' status", buildVersion.Id)
		}

		if evergreen.IsPatchRequester(buildVersion.Requester) {
			p, err := patch.FindOneId(buildVersion.Id)
			if err != nil {
				return errors.Wrapf(err, "getting patch for version '%s'", buildVersion.Id)
			}
			if p == nil {
				return errors.Errorf("no patch found for version '%s'", buildVersion.Id)
			}
			if err = UpdatePatchStatus(p, newVersionStatus); err != nil {
				return errors.Wrapf(err, "updating patch '%s' status", p.Id.Hex())
			}
		}
	}

	return nil
}

// MarkStart updates the task, build, version and if necessary, patch documents with the task start time
func MarkStart(t *task.Task, updates *StatusChanges) error {
	var err error

	startTime := time.Now().Round(time.Millisecond)

	if err = t.MarkStart(startTime); err != nil {
		return errors.WithStack(err)
	}
	event.LogTaskStarted(t.Id, t.Execution)

	// ensure the appropriate build is marked as started if necessary
	if err = build.TryMarkStarted(t.BuildId, startTime); err != nil {
		return errors.Wrap(err, "marking build started")
	}

	// ensure the appropriate version is marked as started if necessary
	if err = TryMarkVersionStarted(t.Version, startTime); err != nil {
		return errors.Wrap(err, "marking version started")
	}

	// if it's a patch, mark the patch as started if necessary
	if evergreen.IsPatchRequester(t.Requester) {
		err := patch.TryMarkStarted(t.Version, startTime)
		if err == nil {
			updates.PatchNewStatus = evergreen.VersionStarted

		} else if !adb.ResultsNotFound(err) {
			return errors.WithStack(err)
		}
	}

	if t.IsPartOfDisplay() {
		return UpdateDisplayTaskForTask(t)
	}

	return nil
}

// MarkHostTaskDispatched marks a task as being dispatched to the host. If it's
// part of a display task, update the display task as necessary.
func MarkHostTaskDispatched(t *task.Task, h *host.Host) error {
	if err := t.MarkAsHostDispatched(h.Id, h.Distro.Id, h.AgentRevision, time.Now()); err != nil {
		return errors.Wrapf(err, "marking task '%s' as dispatched "+
			"on host '%s'", t.Id, h.Id)
	}

	event.LogHostTaskDispatched(t.Id, t.Execution, h.Id)

	if t.IsPartOfDisplay() {
		return UpdateDisplayTaskForTask(t)
	}

	return nil
}

func MarkOneTaskReset(ctx context.Context, t *task.Task) error {
	if t.DisplayOnly {
		if !t.ResetFailedWhenFinished {
			if err := MarkTasksReset(t.ExecutionTasks); err != nil {
				return errors.Wrap(err, "resetting execution tasks")
			}
		} else {
			failedExecTasks, err := task.FindWithFields(task.FailedTasksByIds(t.ExecutionTasks), task.IdKey)
			if err != nil {
				return errors.Wrap(err, "retrieving failed execution tasks")
			}
			failedExecTaskIds := []string{}
			for _, et := range failedExecTasks {
				failedExecTaskIds = append(failedExecTaskIds, et.Id)
			}
			if err := MarkTasksReset(failedExecTaskIds); err != nil {
				return errors.Wrap(err, "resetting failed execution tasks")
			}
		}
	}

	if err := t.Reset(ctx); err != nil && !adb.ResultsNotFound(err) {
		return errors.Wrap(err, "resetting task in database")
	}

	if err := UpdateUnblockedDependencies(t); err != nil {
		return errors.Wrap(err, "clearing unattainable dependencies")
	}

	if err := t.MarkDependenciesFinished(false); err != nil {
		return errors.Wrap(err, "marking direct dependencies unfinished")
	}

	return nil
}

// MarkTasksReset resets many tasks by their IDs. For execution tasks, this also
// resets their parent display tasks.
func MarkTasksReset(taskIds []string) error {
	tasks, err := task.FindAll(db.Query(task.ByIds(taskIds)))
	if err != nil {
		return errors.WithStack(err)
	}
	tasks, err = task.AddParentDisplayTasks(tasks)
	if err != nil {
		return errors.WithStack(err)
	}

	if err = task.ResetTasks(tasks); err != nil {
		return errors.Wrap(err, "resetting tasks in database")
	}

	catcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		catcher.Wrapf(UpdateUnblockedDependencies(&t), "clearing unattainable dependencies for task '%s'", t.Id)
		catcher.Wrapf(t.MarkDependenciesFinished(false), "marking direct dependencies unfinished for task '%s'", t.Id)
	}

	return catcher.Resolve()
}

// RestartFailedTasks attempts to restart failed tasks that started or failed between 2 times.
// It returns a slice of task IDs that were successfully restarted as well as a slice
// of task IDs that failed to restart.
// opts.dryRun will return the tasks that will be restarted if set to true.
// opts.red and opts.purple will only restart tasks that were failed due to the test
// or due to the system, respectively.
func RestartFailedTasks(ctx context.Context, opts RestartOptions) (RestartResults, error) {
	results := RestartResults{}
	if !opts.IncludeTestFailed && !opts.IncludeSysFailed && !opts.IncludeSetupFailed {
		opts.IncludeTestFailed = true
		opts.IncludeSysFailed = true
		opts.IncludeSetupFailed = true
	}
	failureTypes := []string{}
	if opts.IncludeTestFailed {
		failureTypes = append(failureTypes, evergreen.CommandTypeTest)
	}
	if opts.IncludeSysFailed {
		failureTypes = append(failureTypes, evergreen.CommandTypeSystem)
	}
	if opts.IncludeSetupFailed {
		failureTypes = append(failureTypes, evergreen.CommandTypeSetup)
	}
	tasksToRestart, err := task.FindAll(db.Query(task.ByTimeStartedAndFailed(opts.StartTime, opts.EndTime, failureTypes)))
	if err != nil {
		return results, errors.WithStack(err)
	}
	tasksToRestart, err = task.AddParentDisplayTasks(tasksToRestart)
	if err != nil {
		return results, errors.WithStack(err)
	}

	type taskGroupAndBuild struct {
		Build     string
		TaskGroup string
	}
	// only need to check one task per task group / build combination, and once per display task
	taskGroupsToCheck := map[taskGroupAndBuild]string{}
	displayTasksToCheck := map[string]task.Task{}
	idsToRestart := []string{}
	for _, t := range tasksToRestart {
		if t.IsPartOfDisplay() {
			dt, err := t.GetDisplayTask()
			if err != nil {
				return results, errors.Wrap(err, "getting display task")
			}
			displayTasksToCheck[t.DisplayTask.Id] = *dt
		} else if t.DisplayOnly {
			displayTasksToCheck[t.Id] = t
		} else if t.IsPartOfSingleHostTaskGroup() {
			taskGroupsToCheck[taskGroupAndBuild{
				TaskGroup: t.TaskGroup,
				Build:     t.BuildId,
			}] = t.Id
		} else {
			idsToRestart = append(idsToRestart, t.Id)
		}
	}

	for id, dt := range displayTasksToCheck {
		if dt.IsFinished() {
			idsToRestart = append(idsToRestart, id)
		} else {
			if err = dt.SetResetWhenFinished(); err != nil {
				return results, errors.Wrapf(err, "marking display task '%s' for reset", id)
			}
		}
	}
	for _, tg := range taskGroupsToCheck {
		idsToRestart = append(idsToRestart, tg)
	}

	// if this is a dry run, immediately return the tasks found
	if opts.DryRun {
		results.ItemsRestarted = idsToRestart
		return results, nil
	}

	return doRestartFailedTasks(ctx, idsToRestart, opts.User, results), nil
}

func doRestartFailedTasks(ctx context.Context, tasks []string, user string, results RestartResults) RestartResults {
	var tasksErrored []string

	for _, id := range tasks {
		if err := TryResetTask(ctx, evergreen.GetEnvironment().Settings(), id, user, evergreen.RESTV2Package, nil); err != nil {
			tasksErrored = append(tasksErrored, id)
			grip.Error(message.Fields{
				"task":    id,
				"status":  "failed",
				"message": "error restarting task",
				"error":   err.Error(),
			})
		} else {
			results.ItemsRestarted = append(results.ItemsRestarted, id)
		}
	}
	results.ItemsErrored = tasksErrored

	return results
}

// ClearAndResetStrandedContainerTask clears the container task dispatched to a
// pod. It also resets the task so that the current task execution is marked as
// finished and, if necessary, a new execution is created to restart the task.
// TODO (PM-2618): should probably block single-container task groups once
// they're supported.
func ClearAndResetStrandedContainerTask(ctx context.Context, settings *evergreen.Settings, p *pod.Pod) error {
	runningTaskID := p.TaskRuntimeInfo.RunningTaskID
	runningTaskExecution := p.TaskRuntimeInfo.RunningTaskExecution
	if runningTaskID == "" {
		return nil
	}

	// Note that clearing the pod and resetting the task are not atomic
	// operations, so it's possible for the pod's running task to be cleared but
	// the stranded task fails to reset.
	// In this case, there are other cleanup jobs to detect when a task is
	// stranded on a terminated pod.
	if err := p.ClearRunningTask(); err != nil {
		return errors.Wrapf(err, "clearing running task '%s' execution %d from pod '%s'", runningTaskID, runningTaskExecution, p.ID)
	}

	t, err := task.FindOneIdAndExecution(runningTaskID, runningTaskExecution)
	if err != nil {
		return errors.Wrapf(err, "finding running task '%s' execution %d from pod '%s'", runningTaskID, runningTaskExecution, p.ID)
	}
	if t == nil {
		return nil
	}

	if t.Archived {
		grip.Warning(message.Fields{
			"message":   "stranded container task has already been archived, refusing to fix it",
			"task":      t.Id,
			"execution": t.Execution,
			"status":    t.Status,
		})
		return nil
	}

	if err := resetSystemFailedTask(ctx, settings, t, evergreen.TaskDescriptionStranded); err != nil {
		return errors.Wrapf(err, "resetting stranded task '%s'", t.Id)
	}

	grip.Info(message.Fields{
		"message":            "successfully fixed stranded container task",
		"task":               t.Id,
		"execution":          t.Execution,
		"execution_platform": t.ExecutionPlatform,
	})

	return nil
}

// ClearAndResetStrandedHostTask clears the host task dispatched to the host due
// to being stranded on a bad host (e.g. one that has been terminated). It also
// marks the current task execution as finished and, if possible, a new
// execution is created to restart the task.
func ClearAndResetStrandedHostTask(ctx context.Context, settings *evergreen.Settings, h *host.Host) error {
	if h.RunningTask == "" {
		return nil
	}

	t, err := task.FindOneIdAndExecution(h.RunningTask, h.RunningTaskExecution)
	if err != nil {
		return errors.Wrapf(err, "finding running task '%s' execution '%d' from host '%s'", h.RunningTask, h.RunningTaskExecution, h.Id)
	} else if t == nil {
		return nil
	}

	err = UpdateBlockedDependencies(t)
	if err != nil {
		return errors.Wrapf(err, "updating blocked dependencies for task '%s'", t.Id)
	}

	if err = h.ClearRunningTask(ctx); err != nil {
		return errors.Wrapf(err, "clearing running task from host '%s'", h.Id)
	}

	if err := resetSystemFailedTask(ctx, settings, t, evergreen.TaskDescriptionStranded); err != nil {
		return errors.Wrapf(err, "resetting stranded task '%s'", t.Id)
	}

	grip.Info(message.Fields{
		"message":            "successfully fixed stranded host task",
		"task":               t.Id,
		"execution":          t.Execution,
		"execution_platform": t.ExecutionPlatform,
		"version":            t.Version,
		"failure_desc":       t.Details.Description,
	})

	return nil
}

// FixStaleTask fixes a task that has exceeded the heartbeat timeout.
// The current task execution is marked as finished and, if the task was not
// aborted, the task is reset. If the task was aborted, we do not reset the task
// and it is just marked as failed alongside other necessary updates to finish the task.
func FixStaleTask(ctx context.Context, settings *evergreen.Settings, t *task.Task) error {
	if err := UpdateBlockedDependencies(t); err != nil {
		return errors.Wrapf(err, "updating blocked dependencies for task '%s'", t.Id)
	}

	failureDesc := evergreen.TaskDescriptionHeartbeat
	if t.Aborted {
		failureDesc = evergreen.TaskDescriptionAborted
		if err := finishStaleAbortedTask(ctx, settings, t); err != nil {
			return errors.Wrapf(err, "finishing stale aborted task '%s'", t.Id)
		}
	} else {
		if err := resetSystemFailedTask(ctx, settings, t, failureDesc); err != nil {
			if !t.IsPartOfDisplay() {
				return errors.Wrap(err, "resetting heartbeat task")
			}
			// It's possible for display tasks to race, since multiple execution tasks can system fail at the same time.
			// Only error if the display task hasn't actually been reset.
			dt, dbErr := t.GetDisplayTask()
			if dbErr != nil {
				return errors.Wrap(dbErr, "confirming display task status")
			}
			if utility.StringSliceContains(evergreen.TaskCompletedStatuses, dt.Status) {
				return errors.Wrap(err, "resetting heartbeat task")
			}
		}
	}

	grip.Info(message.Fields{
		"message":            "successfully fixed stale task",
		"task":               t.Id,
		"execution":          t.Execution,
		"execution_platform": t.ExecutionPlatform,
		"description":        failureDesc,
	})
	return nil
}

func finishStaleAbortedTask(ctx context.Context, settings *evergreen.Settings, t *task.Task) error {
	failureDetails := &apimodels.TaskEndDetail{
		Status:      evergreen.TaskFailed,
		Type:        evergreen.CommandTypeSystem,
		Description: evergreen.TaskDescriptionAborted,
	}
	projectRef, err := FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		return errors.Wrapf(err, "getting project ref for task '%s'", t.Id)
	}
	if projectRef == nil {
		return errors.Errorf("project ref for task '%s' not found", t.Id)
	}
	if err = MarkEnd(ctx, settings, t, evergreen.APIServerTaskActivator, time.Now(), failureDetails, utility.FromBoolPtr(projectRef.DeactivatePrevious)); err != nil {
		return errors.Wrapf(err, "calling mark finish on task '%s'", t.Id)
	}
	return nil
}

// resetSystemFailedTask resets a task that has encountered a system failure
// such as being stranded on a terminated host/container or failing to send a
// heartbeat.
func resetSystemFailedTask(ctx context.Context, settings *evergreen.Settings, t *task.Task, description string) error {
	if t.IsFinished() {
		return nil
	}

	unschedulableTask := time.Since(t.ActivatedTime) > task.UnschedulableThreshold
	maxExecutionTask := t.Execution >= evergreen.MaxTaskExecution

	if evergreen.IsCommitQueueRequester(t.Requester) && evergreen.IsSystemFailedTaskStatus(t.Status) {
		maxSystemFailedTaskRetries := settings.CommitQueue.MaxSystemFailedTaskRetries
		if maxSystemFailedTaskRetries > 0 {
			maxExecutionTask = t.Execution >= maxSystemFailedTaskRetries
		}
	}

	if unschedulableTask || maxExecutionTask {
		failureDetails := task.GetSystemFailureDetails(description)
		// If the task has already exceeded the unschedulable threshold, we
		// don't want to restart it, so just mark it as finished.
		if t.DisplayOnly {
			execTasks, err := task.FindAll(db.Query(task.ByIds(t.ExecutionTasks)))
			if err != nil {
				return errors.Wrap(err, "finding execution tasks")
			}
			for _, execTask := range execTasks {
				if !evergreen.IsFinishedTaskStatus(execTask.Status) {
					if err = MarkEnd(ctx, settings, &execTask, evergreen.MonitorPackage, time.Now(), &failureDetails, false); err != nil {
						return errors.Wrap(err, "marking execution task as ended")
					}
				}
			}
		}
		return errors.WithStack(MarkEnd(ctx, settings, t, evergreen.MonitorPackage, time.Now(), &failureDetails, false))
	}

	if err := t.MarkSystemFailed(description); err != nil {
		return errors.Wrap(err, "marking task as system failed")
	}
	if err := logTaskEndStats(ctx, t); err != nil {
		return errors.Wrap(err, "logging task end stats")
	}

	return errors.Wrap(ResetTaskOrDisplayTask(ctx, settings, t, evergreen.User, evergreen.MonitorPackage, true, &t.Details), "resetting task")
}

// ResetTaskOrDisplayTask is a wrapper for TryResetTask that handles execution and display tasks that are restarted
// from sources separate from marking the task finished. If an execution task, attempts to restart the display task instead.
// Marks display tasks as reset when finished and then check if it can be reset immediately.
func ResetTaskOrDisplayTask(ctx context.Context, settings *evergreen.Settings, t *task.Task, user, origin string, failedOnly bool, detail *apimodels.TaskEndDetail) error {
	taskToReset := *t
	if taskToReset.IsPartOfDisplay() { // if given an execution task, attempt to restart the full display task
		dt, err := taskToReset.GetDisplayTask()
		if err != nil {
			return errors.Wrap(err, "getting display task")
		}
		if dt != nil {
			taskToReset = *dt
		}
	}
	if taskToReset.DisplayOnly {
		if failedOnly {
			if err := taskToReset.SetResetFailedWhenFinished(); err != nil {
				return errors.Wrap(err, "marking display task for reset")
			}
		} else {
			if err := taskToReset.SetResetWhenFinished(); err != nil {
				return errors.Wrap(err, "marking display task for reset")
			}
		}
		return errors.Wrap(checkResetDisplayTask(ctx, settings, &taskToReset), "checking display task reset")
	}

	return errors.Wrap(TryResetTask(ctx, settings, t.Id, user, origin, detail), "resetting task")
}

// UpdateDisplayTaskForTask updates the status of the given execution task's display task
func UpdateDisplayTaskForTask(t *task.Task) error {
	if !t.IsPartOfDisplay() {
		return errors.Errorf("task '%s' is not an execution task", t.Id)
	}
	dt, err := t.GetDisplayTask()
	if err != nil {
		return errors.Wrap(err, "getting display task for task")
	}
	if dt == nil {
		grip.Error(message.Fields{
			"message":         "task may hold a display task that doesn't exist",
			"task_id":         t.Id,
			"display_task_id": t.DisplayTaskId,
		})
		return errors.Errorf("display task not found for task '%s'", t.Id)
	}
	if !dt.DisplayOnly {
		return errors.Errorf("task '%s' is not a display task", dt.Id)
	}

	var timeTaken time.Duration
	var statusTask task.Task
	execTasks, err := task.Find(task.ByIds(dt.ExecutionTasks))
	if err != nil {
		return errors.Wrap(err, "retrieving execution tasks")
	}
	hasFinishedTasks := false
	hasTasksToRun := false
	startTime := time.Unix(1<<62, 0)
	endTime := utility.ZeroTime
	noActiveTasks := true
	for _, execTask := range execTasks {
		// if any of the execution tasks are scheduled, the display task is too
		if execTask.Activated {
			dt.Activated = true
			noActiveTasks = false
			if utility.IsZeroTime(dt.ActivatedTime) {
				dt.ActivatedTime = time.Now()
			}
		}
		if execTask.IsFinished() {
			hasFinishedTasks = true
			// Need to consider tasks that have been dispatched since the last exec task finished.
		} else if (execTask.IsDispatchable() || execTask.IsAbortable()) && !execTask.Blocked() {
			hasTasksToRun = true
		}

		// add up the duration of the execution tasks as the cumulative time taken
		timeTaken += execTask.TimeTaken

		// set the start/end time of the display task as the earliest/latest task
		if !utility.IsZeroTime(execTask.StartTime) && execTask.StartTime.Before(startTime) {
			startTime = execTask.StartTime
		}
		if execTask.FinishTime.After(endTime) {
			endTime = execTask.FinishTime
		}
	}
	if noActiveTasks {
		dt.Activated = false
	}

	sort.Sort(task.ByPriority(execTasks))
	statusTask = execTasks[0]
	if hasFinishedTasks && hasTasksToRun {
		// if an unblocked display task has a mix of finished and unfinished tasks, the display task is still
		// "started" even if there aren't currently running tasks
		statusTask.Status = evergreen.TaskStarted
		statusTask.Details = apimodels.TaskEndDetail{}
	}

	update := bson.M{
		task.StatusKey:        statusTask.Status,
		task.ActivatedKey:     dt.Activated,
		task.ActivatedTimeKey: dt.ActivatedTime,
		task.TimeTakenKey:     timeTaken,
		task.DetailsKey:       statusTask.Details,
	}

	if startTime != time.Unix(1<<62, 0) {
		update[task.StartTimeKey] = startTime
	}
	if endTime != utility.ZeroTime && !hasTasksToRun {
		update[task.FinishTimeKey] = endTime
	}

	// refresh task status from db in case of race
	taskWithStatus, err := task.FindOneIdWithFields(dt.Id, task.StatusKey)
	if err != nil {
		return errors.Wrapf(err, "refreshing task '%s'", dt.Id)
	}
	if taskWithStatus == nil {
		return errors.Errorf("task '%s' not found", dt.Id)
	}
	wasFinished := taskWithStatus.IsFinished()
	err = task.UpdateOne(
		bson.M{
			task.IdKey: dt.Id,
		},
		bson.M{
			"$set": update,
		})
	if err != nil {
		return errors.Wrap(err, "updating display task")
	}
	dt.Status = statusTask.Status
	dt.Details = statusTask.Details
	dt.TimeTaken = timeTaken
	if !wasFinished && dt.IsFinished() {
		event.LogTaskFinished(dt.Id, dt.Execution, dt.GetDisplayStatus())
		grip.Info(message.Fields{
			"message":   "display task finished",
			"task_id":   dt.Id,
			"status":    dt.Status,
			"operation": "UpdateDisplayTaskForTask",
		})
	}
	return nil
}

// checkResetSingleHostTaskGroup attempts to reset all tasks that are part of
// the same single-host task group as t once all tasks in the task group are
// finished running.
func checkResetSingleHostTaskGroup(ctx context.Context, t *task.Task, caller string) error {
	if !t.IsPartOfSingleHostTaskGroup() {
		return nil
	}
	tasks, err := task.FindTaskGroupFromBuild(t.BuildId, t.TaskGroup)
	if err != nil {
		return errors.Wrapf(err, "getting task group for task '%s'", t.Id)
	}
	if len(tasks) == 0 {
		return errors.Errorf("no tasks in task group '%s' for task '%s'", t.TaskGroup, t.Id)
	}
	shouldReset := false
	for _, tgTask := range tasks {
		if tgTask.ResetWhenFinished {
			shouldReset = true
		}
		if !tgTask.IsFinished() && !tgTask.Blocked() && tgTask.Activated { // task in group still needs to run
			return nil
		}
	}

	if !shouldReset { // no task in task group has requested a reset
		return nil
	}

	return errors.Wrap(resetManyTasks(ctx, tasks, caller), "resetting task group tasks")
}

// checkResetDisplayTask attempts to reset all tasks that are under the same
// parent display task as t once all tasks under the display task are finished
// running.
func checkResetDisplayTask(ctx context.Context, setting *evergreen.Settings, t *task.Task) error {
	if !t.ResetWhenFinished && !t.ResetFailedWhenFinished {
		return nil
	}
	execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
	if err != nil {
		return errors.Wrapf(err, "getting execution tasks for display task '%s'", t.Id)
	}
	for _, execTask := range execTasks {
		if !execTask.IsFinished() && !execTask.Blocked() && execTask.Activated {
			return nil // all tasks not finished
		}
	}
	details := &t.Details
	// Assign task end details to indicate system failure if we receive no valid details
	if details.IsEmpty() && !t.IsFinished() {
		details = &apimodels.TaskEndDetail{
			Type:   evergreen.CommandTypeSystem,
			Status: evergreen.TaskFailed,
		}
	}
	return errors.Wrap(TryResetTask(ctx, setting, t.Id, evergreen.User, evergreen.User, details), "resetting display task")
}

// MarkUnallocatableContainerTasksSystemFailed marks any container task within
// the candidate task IDs that needs to re-allocate a container but has used up
// all of its container allocation attempts as finished due to system failure.
func MarkUnallocatableContainerTasksSystemFailed(ctx context.Context, settings *evergreen.Settings, candidateTaskIDs []string) error {
	var unallocatableTasks []task.Task
	for _, taskID := range candidateTaskIDs {
		tsk, err := task.FindOneId(taskID)
		if err != nil {
			return errors.Wrapf(err, "finding task '%s'", taskID)
		}
		if tsk == nil {
			continue
		}
		if !tsk.IsContainerTask() {
			continue
		}
		if !tsk.ContainerAllocated {
			continue
		}
		if tsk.RemainingContainerAllocationAttempts() > 0 {
			continue
		}

		unallocatableTasks = append(unallocatableTasks, *tsk)
	}

	catcher := grip.NewBasicCatcher()
	for _, tsk := range unallocatableTasks {
		details := apimodels.TaskEndDetail{
			Status:      evergreen.TaskFailed,
			Type:        evergreen.CommandTypeSystem,
			Description: evergreen.TaskDescriptionContainerUnallocatable,
		}
		if err := MarkEnd(ctx, settings, &tsk, evergreen.APIServerTaskActivator, time.Now(), &details, false); err != nil {
			catcher.Wrapf(err, "marking task '%s' as a failure due to inability to allocate", tsk.Id)
		}
	}

	return catcher.Resolve()
}
