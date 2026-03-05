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
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
			execTasks, err := task.Find(ctx, task.ByIds(t.ExecutionTasks))
			catcher.Wrap(err, "getting execution tasks")
			originalTasks = append(originalTasks, execTasks...)
		}
		versionIdsSet[t.Version] = true
		buildToTaskMap[t.BuildId] = t
		if active {
			// if the task is being activated, and it doesn't override its dependencies
			// activate the task's dependencies as well
			if !t.OverrideDependencies {
				deps, err := task.GetRecursiveDependenciesUp(ctx, originalTasks, nil)
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

			if t.IsHostTask() && !utility.IsZeroTime(t.DispatchTime) && t.Status == evergreen.TaskUndispatched {
				catcher.Wrapf(resetTask(ctx, t.Id, caller), "resetting task '%s'", t.Id)
			} else {
				tasksToActivate = append(tasksToActivate, originalTasks...)
			}

			// If the task was not activated by step back, and either the caller is not evergreen
			// or the task was originally activated by evergreen, deactivate the task
		} else if !evergreen.IsSystemActivator(caller) || evergreen.IsSystemActivator(t.ActivatedBy) {
			tasksToActivate = append(tasksToActivate, originalTasks...)
		} else {
			continue
		}
	}

	if active {
		if _, err := task.ActivateTasks(ctx, tasksToActivate, time.Now(), true, caller); err != nil {
			return errors.Wrap(err, "activating tasks")
		}
		versionIdsToActivate := []string{}
		for v := range versionIdsSet {
			versionIdsToActivate = append(versionIdsToActivate, v)
		}
		if err := ActivateVersions(ctx, versionIdsToActivate); err != nil {
			return errors.Wrap(err, "marking version as activated")
		}
		buildIdsToActivate := []string{}
		for b := range buildToTaskMap {
			buildIdsToActivate = append(buildIdsToActivate, b)
		}
		if err := build.UpdateActivation(ctx, buildIdsToActivate, true, caller); err != nil {
			return errors.Wrap(err, "marking builds as activated")
		}
	} else {
		if err := task.DeactivateTasks(ctx, tasksToActivate, true, caller); err != nil {
			return errors.Wrap(err, "deactivating task")
		}
	}

	for _, t := range tasksToActivate {
		if t.IsPartOfDisplay(ctx) {
			catcher.Wrap(UpdateDisplayTaskForTask(ctx, &t), "updating display task")
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
	t, err := task.FindOneId(ctx, id)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", id)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", id)
	}
	return SetActiveState(ctx, user, active, *t)
}

func DisableTasks(ctx context.Context, caller string, tasks ...task.Task) error {
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

	_, err := task.UpdateAll(ctx,
		task.ByIds(append(taskIDs, execTaskIDs...)),
		bson.M{"$set": bson.M{task.PriorityKey: evergreen.DisabledTaskPriority}},
	)
	if err != nil {
		return errors.Wrap(err, "updating task priorities")
	}

	execTasks, err := findMissingTasks(ctx, execTaskIDs, tasksPresent)
	if err != nil {
		return errors.Wrap(err, "finding additional execution tasks")
	}
	tasks = append(tasks, execTasks...)

	for _, t := range tasks {
		t.Priority = evergreen.DisabledTaskPriority
		event.LogTaskPriority(ctx, t.Id, t.Execution, caller, evergreen.DisabledTaskPriority)
	}

	if err := task.DeactivateTasks(ctx, tasks, true, caller); err != nil {
		return errors.Wrap(err, "deactivating dependencies")
	}

	return nil
}

// findMissingTasks finds all tasks whose IDs are missing from tasksPresent.
func findMissingTasks(ctx context.Context, taskIDs []string, tasksPresent map[string]struct{}) ([]task.Task, error) {
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

	missingTasks, err := task.FindAll(ctx, db.Query(task.ByIds(missingTaskIDs)))
	if err != nil {
		return nil, err
	}

	return missingTasks, nil
}

// activatePreviousTask will set the active state for the first task with a
// revision order number less than the current task's revision order number.
// originalStepbackTask is only specified while we're stepping back the generator
// for a generated task.
func activatePreviousTask(ctx context.Context, taskId, caller string, originalStepbackTask *task.Task) error {
	// find the task first
	t, err := task.FindOneId(ctx, taskId)
	if err != nil {
		return errors.WithStack(err)
	}
	if t == nil {
		return errors.Errorf("task '%s' does not exist", taskId)
	}

	// find previous task limiting to just the last one
	filter, sort := task.ByBeforeRevision(t.RevisionOrderNumber, t.BuildVariant, t.DisplayName, t.Project, t.Requester)
	query := db.Query(filter).Sort(sort)
	prevTask, err := task.FindOne(ctx, query)
	if err != nil {
		return errors.Wrap(err, "finding previous task")
	}
	if prevTask == nil {
		return errors.Errorf("previous task for '%s' not found", taskId)
	}

	// for generated tasks, try to activate the generator instead if the previous task we found isn't the actual last task
	if t.GeneratedBy != "" && prevTask.RevisionOrderNumber+1 != t.RevisionOrderNumber {
		return activatePreviousTask(ctx, t.GeneratedBy, caller, t)
	}

	// If this is a valid, unfinished, non-disabled, and unactive task- we should activate it.
	if !prevTask.IsFinished() && prevTask.Priority >= 0 && !prevTask.Activated {
		if err = SetActiveState(ctx, caller, true, *prevTask); err != nil {
			return errors.Wrapf(err, "setting task '%s' active", prevTask.Id)
		}
	}

	// If this is a generator task and we originally were stepping back a generated task, activate the generated task
	// once the generator finishes.
	if prevTask.GenerateTask && originalStepbackTask != nil {
		if err = prevTask.SetGeneratedTasksToActivate(ctx, originalStepbackTask.BuildVariant, originalStepbackTask.DisplayName); err != nil {
			return errors.Wrap(err, "setting generated tasks to activate")
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
	t, err := task.FindOneId(ctx, taskId)
	if err != nil {
		return errors.WithStack(err)
	}
	if t == nil {
		return errors.Errorf("cannot restart task '%s' because it could not be found", taskId)
	}
	if t.IsPartOfDisplay(ctx) {
		return errors.Errorf("cannot restart execution task '%s' because it is part of a display task", t.Id)
	}

	var execTask *task.Task

	maxExecution := settings.TaskLimits.MaxTaskExecution

	// For system failures, we restart once for tasks on their first execution, if configured.
	if !settings.ServiceFlags.SystemFailedTaskRestartDisabled &&
		!detail.IsEmpty() && detail.Type == evergreen.CommandTypeSystem {
		maxExecution = 1
	}

	// If we've reached the max number of executions for this task, mark it as finished and failed.
	// Restarting from the UI/API bypasses the restart cap.
	if t.Execution >= maxExecution && !utility.StringSliceContains(evergreen.UserTriggeredOrigins, origin) {
		if !t.IsFinished() {
			if detail != nil {
				if t.DisplayOnly {
					for _, etId := range t.ExecutionTasks {
						execTask, err = task.FindOneId(ctx, etId)
						if err != nil {
							return errors.Wrap(err, "finding execution task")
						}
						if err = MarkEnd(ctx, settings, execTask, origin, time.Now(), detail); err != nil {
							return errors.Wrap(err, "marking execution task as ended")
						}
					}
				}
				return errors.WithStack(MarkEnd(ctx, settings, t, origin, time.Now(), detail))
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
		if utility.StringSliceContains(evergreen.UserTriggeredOrigins, origin) {
			if t.DisplayOnly {
				execTasks := map[string]string{}
				for _, et := range t.ExecutionTasks {
					execTask, err = task.FindOneId(ctx, et)
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
		if err = t.MarkEnd(ctx, time.Now(), detail); err != nil {
			return errors.Wrap(err, "marking task as ended")
		}
	}

	caller := origin
	if utility.StringSliceContains(evergreen.UserTriggeredOrigins, origin) || user == evergreen.AutoRestartActivator {
		caller = user
	}
	if t.IsPartOfSingleHostTaskGroup() {
		if err = t.SetResetWhenFinished(ctx, user); err != nil {
			return errors.Wrap(err, "marking task group for reset")
		}
		return errors.Wrap(checkResetSingleHostTaskGroup(ctx, t, caller), "resetting single host task group")
	}

	return errors.WithStack(resetTask(ctx, t.Id, caller))
}

// resetTask finds a finished task, attempts to archive it, and resets the task and
// resets the TaskCache in the build as well.
func resetTask(ctx context.Context, taskId, caller string) error {
	t, err := task.FindOneId(ctx, taskId)
	if err != nil {
		return errors.WithStack(err)
	}
	if t.IsPartOfDisplay(ctx) {
		return errors.Errorf("cannot restart execution task '%s' because it is part of a display task", t.Id)
	}
	if err = task.CheckUsersPatchTaskLimit(ctx, t.Requester, caller, false, *t); err != nil {
		return errors.Wrap(err, "updating patch task limit for user")
	}
	if err = t.Archive(ctx); err != nil {
		return errors.Wrap(err, "can't restart task because it can't be archived")
	}
	if err = MarkOneTaskReset(ctx, t, caller); err != nil {
		return errors.WithStack(err)
	}

	event.LogTaskRestarted(ctx, t.Id, t.Execution, caller)

	return errors.WithStack(UpdateBuildAndVersionStatusForTask(ctx, t))
}

func AbortTask(ctx context.Context, taskId, caller string) error {
	t, err := task.FindOneId(ctx, taskId)
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
	event.LogTaskAbortRequest(ctx, t.Id, t.Execution, caller)
	return t.SetAborted(ctx, task.AbortInfo{User: caller})
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
	allTasks, err := task.FindAll(ctx, query)
	if err != nil {
		return errors.Wrapf(err, "finding previous tasks to deactivate for task '%s'", t.Id)
	}
	for _, t := range allTasks {
		// Only deactivate tasks that other tasks aren't waiting for.
		hasDependentTasks, err := task.HasActivatedDependentTasks(ctx, t.Id)
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

type stepbackInstructions struct {
	shouldStepback bool
	// If true, bisect should be used, if false, linear should be used.
	bisect bool
}

// getStepback returns what type of stepback and if a task should stepback.
// If it should stepback is retrieved from the top-level project if not explicitly
// set on the task or disabled at the project level. And the stepback type is
// either linear or bisect, which is retrieved from the project ref.
func getStepback(ctx context.Context, taskId string, projectRef *ProjectRef, project *Project) (stepbackInstructions, error) {
	t, err := task.FindOneId(ctx, taskId)
	if err != nil {
		return stepbackInstructions{}, errors.Wrapf(err, "finding task '%s'", taskId)
	}
	if t == nil {
		return stepbackInstructions{}, errors.Errorf("task '%s' not found", taskId)
	}

	// Disabling the feature at the project level takes precedent.
	if projectRef.IsStepbackDisabled() {
		return stepbackInstructions{}, nil
	}

	s := stepbackInstructions{
		bisect: utility.FromBoolPtr(projectRef.StepbackBisect),
	}

	// Check if the bvtask overrides the stepback policy specified by the project
	bvtu := project.FindBuildVariantTaskUnit(t.BuildVariant, t.DisplayName)
	if bvtu != nil && bvtu.Stepback != nil {
		s.shouldStepback = utility.FromBoolPtr(bvtu.Stepback)
		return s, nil
	}

	// Check if the task overrides the stepback policy specified by the project
	projectTask := project.FindProjectTask(t.DisplayName)
	if projectTask != nil && projectTask.Stepback != nil {
		s.shouldStepback = utility.FromBoolPtr(projectTask.Stepback)
		return s, nil
	}

	// Check if the build variant overrides the stepback policy specified by the project
	for _, buildVariant := range project.BuildVariants {
		if t.BuildVariant == buildVariant.Name {
			if buildVariant.Stepback != nil {
				s.shouldStepback = utility.FromBoolPtr(buildVariant.Stepback)
				return s, nil
			}
			break
		}
	}
	s.shouldStepback = project.Stepback
	return s, nil
}

// doLinearStepback performs a stepback on the task linearly (the previous
// tasks one by one). If there is a previous task and if not it returns nothing.
func doLinearStepback(ctx context.Context, t *task.Task) error {
	if t.DisplayOnly {
		execTasks, err := task.Find(ctx, task.ByIds(t.ExecutionTasks))
		if err != nil {
			return errors.Wrapf(err, "finding tasks for stepback of '%s'", t.Id)
		}
		catcher := grip.NewSimpleCatcher()
		for _, et := range execTasks {
			catcher.Add(doLinearStepback(ctx, &et))
		}
		if catcher.HasErrors() {
			return catcher.Resolve()
		}
	}

	//See if there is a prior success for this particular task.
	//If there isn't, we should not activate the previous task because
	//it could trigger stepping backwards ad infinitum.
	prevTask, err := t.PreviousCompletedTask(ctx, t.Project, []string{evergreen.TaskSucceeded})
	if err != nil {
		return errors.Wrap(err, "locating previous successful task")
	}
	if prevTask == nil {
		return nil
	}

	// activate the previous task to pinpoint regression
	return errors.WithStack(activatePreviousTask(ctx, t.Id, evergreen.StepbackTaskActivator, nil))
}

// doBisectStepback performs a bisect stepback on the task.
// If there are no tasks to bisect, it no-ops.
func doBisectStepback(ctx context.Context, t *task.Task) error {
	// Do stepback for all execution tasks.
	if t.DisplayOnly {
		execTasks, err := task.Find(ctx, task.ByIds(t.ExecutionTasks))
		if err != nil {
			return errors.Wrapf(err, "finding tasks for stepback of '%s'", t.Id)
		}
		catcher := grip.NewSimpleCatcher()
		for _, et := range execTasks {
			catcher.Add(doBisectStepback(ctx, &et))
		}
		if catcher.HasErrors() {
			return errors.Wrapf(catcher.Resolve(), "performing bisect stepback on display task '%s' execution tasks", t.Id)
		}
	}

	var s task.StepbackInfo
	if !t.StepbackInfo.IsZero() {
		// Carry over from the last task.
		s = *t.StepbackInfo
	} else {
		// If this is the first iteration of stepback, we must get the initial condition (last successful passing task).
		lastPassing, err := t.PreviousCompletedTask(ctx, t.Project, []string{evergreen.TaskSucceeded})
		if err != nil {
			return errors.Wrap(err, "locating previous successful task")
		}
		if lastPassing == nil {
			return nil
		}
		s = task.StepbackInfo{
			LastPassingStepbackTaskId: lastPassing.Id,
		}
		grip.Info(message.Fields{
			"message":                       "starting bisect stepback",
			"last_passing_stepback_task_id": s.LastPassingStepbackTaskId,
			"task_id":                       t.Id,
			"gap":                           t.RevisionOrderNumber - lastPassing.RevisionOrderNumber,
			"project_id":                    t.Project,
		})
	}
	// The previous iteration is the task we are currently processing.
	s.PreviousStepbackTaskId = t.Id

	// Depending on the task status, we want to update the
	// last failing or last passing task.
	if t.Status == evergreen.TaskSucceeded {
		s.LastPassingStepbackTaskId = t.Id
	} else if t.Status == evergreen.TaskFailed {
		s.LastFailingStepbackTaskId = t.Id
	} else {
		grip.Warningf("stopping task '%s' stepback due to status '%s'", t.Id, t.Status)
		return nil
	}

	// The midway task is our next stepback target.
	nextTask, err := task.ByBeforeMidwayTaskFromIds(ctx, s.LastFailingStepbackTaskId, s.LastPassingStepbackTaskId)
	if err != nil {
		return errors.Wrap(err, "finding previous task")
	}
	if nextTask == nil {
		return errors.Errorf("midway task could not be found for tasks '%s' '%s'", s.LastFailingStepbackTaskId, s.LastPassingStepbackTaskId)
	}
	// If our next task is last passing Id, we have finished stepback.
	if nextTask.Id == s.LastPassingStepbackTaskId {
		return nil
	}
	s.NextStepbackTaskId = nextTask.Id
	// Store our next task to our current task.
	if err := task.SetNextStepbackId(ctx, t.Id, s); err != nil {
		return errors.Wrapf(err, "could not set next stepback task id for stepback task '%s'", t.Id)
	}
	// If the next task has finished, negative priority, or already activated, no-op.
	if nextTask.IsFinished() || nextTask.Priority < 0 || nextTask.Activated {
		return nil
	}
	// Store our last and previous stepback tasks in our upcoming/next task.
	if err = task.SetLastAndPreviousStepbackIds(ctx, nextTask.Id, s); err != nil {
		return errors.Wrapf(err, "setting stepback info for task '%s'", nextTask.Id)
	}

	grip.Info(message.Fields{
		"message":                       "bisect stepback",
		"last_failing_stepback_task_id": s.LastFailingStepbackTaskId,
		"last_passing_stepback_task_id": s.LastPassingStepbackTaskId,
		"next_task_id":                  nextTask.Id,
		"next_task_display_name":        nextTask.DisplayName,
		"next_task_build_id":            nextTask.BuildId,
		"last_stepback_task_id":         t.Id,
		"last_stepback_task_status":     t.Status,
		"project_id":                    t.Project,
	})

	// Activate the next task.
	if err = SetActiveState(ctx, evergreen.StepbackTaskActivator, true, *nextTask); err != nil {
		return errors.Wrapf(err, "setting task '%s' active", nextTask.Id)
	}

	// If this is a generator task, activate generated tasks.
	if nextTask.GenerateTask {
		if err = nextTask.SetGeneratedTasksToActivate(ctx, nextTask.BuildVariant, nextTask.DisplayName); err != nil {
			return errors.Wrap(err, "setting generated tasks to activate")
		}
	}

	return nil
}

func doBisectStepbackForGeneratedTask(ctx context.Context, generator *task.Task, generated *task.Task) error {
	var s task.StepbackInfo
	if lastStepbackInfo := generator.StepbackInfo.GetStepbackInfoForGeneratedTask(generated.DisplayName, generated.BuildVariant); lastStepbackInfo != nil {
		// Carry over from the last task.
		s = *lastStepbackInfo
	} else {
		lastPassingGenerated, err := generated.PreviousCompletedTask(ctx, generated.Project, []string{evergreen.TaskSucceeded})
		if err != nil {
			return errors.Wrap(err, "locating previous successful task")
		}
		// Noop if there is no previous passing task.
		if lastPassingGenerated == nil {
			return nil
		}
		if lastPassingGenerated.GeneratedBy == "" {
			return errors.Errorf("last passing generated task '%s' must have a generator task", lastPassingGenerated.Id)
		}
		s = task.StepbackInfo{
			DisplayName:               generated.DisplayName,
			BuildVariant:              generated.BuildVariant,
			LastPassingStepbackTaskId: lastPassingGenerated.GeneratedBy,
		}
		grip.Info(message.Fields{
			"message":                       "starting bisect stepback on generator task",
			"last_passing_stepback_task_id": s.LastPassingStepbackTaskId,
			"generator_task_id":             generator.Id,
			"generated_task_id":             generated.Id,
			"display_name":                  generated.DisplayName,
			"BuildVariant":                  generated.BuildVariant,
			"gap":                           generated.RevisionOrderNumber - lastPassingGenerated.RevisionOrderNumber,
			"project_id":                    generated.Project,
		})
	}

	// The previous iteration is the task we are currently processing.
	s.PreviousStepbackTaskId = generator.Id

	// Depending on the task status, we want to update the
	// last failing or last passing task.
	if generated.Status == evergreen.TaskSucceeded {
		s.LastPassingStepbackTaskId = generator.Id
	} else if generated.Status == evergreen.TaskFailed {
		s.LastFailingStepbackTaskId = generator.Id
	} else {
		grip.Warningf("stopping task '%s' stepback due to status '%s'", generated.Id, generated.Status)
		return nil
	}

	// The midway task is our next stepback target.
	nextTask, err := task.ByBeforeMidwayTaskFromIds(ctx, s.LastFailingStepbackTaskId, s.LastPassingStepbackTaskId)
	if err != nil {
		return errors.Wrapf(err, "finding midway task between tasks '%s' and '%s'", s.LastFailingStepbackTaskId, s.LastPassingStepbackTaskId)
	}
	if nextTask == nil {
		return errors.Errorf("midway task could not be found for tasks '%s' '%s'", s.LastFailingStepbackTaskId, s.LastPassingStepbackTaskId)
	}
	// If our next task is last passing Id, we have finished stepback.
	if nextTask.Id == s.LastPassingStepbackTaskId {
		return nil
	}
	s.NextStepbackTaskId = nextTask.Id
	// Store our next task to our current task.
	if err := task.SetGeneratedStepbackInfoForGenerator(ctx, generator.Id, s); err != nil {
		return errors.Wrapf(err, "could not set generated stepback task id for stepback task '%s'", generator.Id)
	}
	// This is only for UI purposes. The generated task needs these fields populated
	// to show stepback options in the UI. We create a new stepback info because we do
	// not want to copy over the generated related fields.
	if err := task.SetLastAndPreviousStepbackIds(ctx, generated.Id, task.StepbackInfo{
		LastFailingStepbackTaskId: s.LastFailingStepbackTaskId,
		LastPassingStepbackTaskId: s.LastPassingStepbackTaskId,
		PreviousStepbackTaskId:    s.PreviousStepbackTaskId,
		NextStepbackTaskId:        s.NextStepbackTaskId,
	}); err != nil {
		return errors.Wrapf(err, "could not set stepback info for generated task '%s'", generated.Id)
	}
	// Store our last and previous stepback tasks in our upcoming/next task.
	if err = task.AddGeneratedStepbackInfoForGenerator(ctx, nextTask.Id, s); err != nil {
		return errors.Wrapf(err, "setting stepback info for task '%s'", nextTask.Id)
	}

	grip.Info(message.Fields{
		"message":                         "bisect stepback on generator task",
		"last_failing_stepback_task_id":   s.LastFailingStepbackTaskId,
		"last_passing_stepback_task_id":   s.LastPassingStepbackTaskId,
		"next_task_id":                    nextTask.Id,
		"next_task_display_name":          nextTask.DisplayName,
		"next_task_build_id":              nextTask.BuildId,
		"last_stepback_task_generator_id": generator.Id,
		"last_stepback_task_id":           generated.Id,
		"last_stepback_task_status":       generated.Status,
		"project_id":                      generated.Project,
	})

	// Activate the next task.
	if err = SetActiveState(ctx, evergreen.StepbackTaskActivator, true, *nextTask); err != nil {
		return errors.Wrapf(err, "setting task '%s' active", nextTask.Id)
	}

	// If this is a generator task, activate generated tasks.
	if nextTask.GenerateTask {
		if err = nextTask.SetGeneratedTasksToActivate(ctx, generated.BuildVariant, generated.DisplayName); err != nil {
			return errors.Wrap(err, "setting generated tasks to activate")
		}
	}

	return nil
}

// MarkEnd updates the task as being finished, performs a stepback if necessary, and updates the build status.
func MarkEnd(ctx context.Context, settings *evergreen.Settings, t *task.Task, caller string,
	finishTime time.Time, detail *apimodels.TaskEndDetail) error {
	ctx, span := tracer.Start(ctx, "mark-end")
	defer span.End()
	catcher := grip.NewBasicCatcher()

	const slowThreshold = time.Second

	detailsCopy := *detail
	if t.Status == detailsCopy.Status {
		grip.Warning(message.Fields{
			"message": "tried to mark task as finished twice",
			"task":    t.Id,
		})
		return nil
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
	err := t.MarkEnd(ctx, finishTime, &detailsCopy)

	grip.NoticeWhen(time.Since(startPhaseAt) > slowThreshold, message.Fields{
		"message":       "slow operation",
		"function":      "MarkEnd",
		"step":          "t.MarkEnd",
		"task":          t.Id,
		"duration_secs": time.Since(startPhaseAt).Seconds(),
	})

	// Add cost attributes to the context for otel tracing
	if !t.TaskCost.IsZero() {
		costAttrs := []attribute.KeyValue{
			attribute.Float64(evergreen.TaskOnDemandCostOtelAttribute, t.TaskCost.OnDemandEC2Cost),
			attribute.Float64(evergreen.TaskAdjustedCostOtelAttribute, t.TaskCost.AdjustedEC2Cost),
		}
		ctx = utility.ContextWithAppendedAttributes(ctx, costAttrs)
		span.SetAttributes(costAttrs...)
	}

	// If the error is from marking the task as finished, we want to
	// return early as every functionality depends on this succeeding.
	if err != nil {
		return errors.Wrapf(err, "marking task '%s' finished", t.Id)
	}

	catcher.Wrap(UpdateBlockedDependencies(ctx, []task.Task{*t}, false), "updating blocked dependencies")
	catcher.Wrap(t.MarkDependenciesFinished(ctx, true), "updating dependency finished status")

	status := t.GetDisplayStatus()

	switch t.ExecutionPlatform {
	case task.ExecutionPlatformHost:
		event.LogHostTaskFinished(ctx, t.Id, t.Execution, t.HostId, status)
	default:
		event.LogTaskFinished(ctx, t.Id, t.Execution, status)
	}

	grip.Info(message.Fields{
		"message":            "marking task finished",
		"included_on":        evergreen.ContainerHealthDashboard,
		"task_id":            t.Id,
		"execution":          t.Execution,
		"status":             status,
		"operation":          "MarkEnd",
		"host_id":            t.HostId,
		"execution_platform": t.ExecutionPlatform,
	})
	origin := evergreen.APIServerTaskActivator
	if t.IsPartOfDisplay(ctx) {
		catcher.Add(markEndDisplayTask(ctx, settings, t, caller, origin))
	} else if t.IsPartOfSingleHostTaskGroup() {
		catcher.Wrap(checkResetSingleHostTaskGroup(ctx, t, caller), "resetting single host task group")
	}

	// Stepback and deactivating previous tasks are non-essential, so we log but don't catch the error.
	attemptStepbackAndDeactivatePrevious(ctx, t, status, caller)

	catcher.Wrap(UpdateBuildAndVersionStatusForTask(ctx, t), "updating build/version status")
	catcher.Wrap(logTaskEndStats(ctx, t), "logging task end stats")

	if (t.ResetWhenFinished || t.ResetFailedWhenFinished) && !t.IsPartOfDisplay(ctx) && !t.IsPartOfSingleHostTaskGroup() {
		if t.IsAutomaticRestart {
			caller = evergreen.AutoRestartActivator
		}
		return TryResetTask(ctx, settings, t.Id, caller, "", detail)
	}

	return catcher.Resolve()
}

func markEndDisplayTask(ctx context.Context, settings *evergreen.Settings, t *task.Task, caller, origin string) error {
	if err := UpdateDisplayTaskForTask(ctx, t); err != nil {
		return errors.Wrap(err, "updating display task")
	}
	dt, err := t.GetDisplayTask(ctx)
	if err != nil {
		return errors.Wrap(err, "getting display task")
	}
	return errors.Wrap(checkResetDisplayTask(ctx, settings, caller, origin, dt), "checking display task reset")
}

func getDeactivatePrevious(t *task.Task, pRef *ProjectRef, project *Project) bool {
	// If this is disabled at the project level, this takes precedence.
	if pRef.IsDeactivatePreviousDisabled() {
		return false
	}

	// Check if the build variant overrides the policy specified by the project.
	for _, buildVariant := range project.BuildVariants {
		if t.BuildVariant == buildVariant.Name {
			if buildVariant.DeactivatePrevious != nil {
				return utility.FromBoolPtr(buildVariant.DeactivatePrevious)
			}
			break
		}
	}
	return pRef.ShouldDeactivatePrevious()
}

func attemptStepbackAndDeactivatePrevious(ctx context.Context, t *task.Task, status, caller string) {
	catcher := grip.NewBasicCatcher()
	pRef, err := FindMergedProjectRef(ctx, t.Project, t.Version, false)
	if err != nil {
		catcher.Wrapf(err, "finding merged project ref for task '%s'", t.Id)
	}
	if pRef == nil {
		catcher.Errorf("merged project ref for task '%s' is nil", t.Id)
	}

	project, err := FindProjectFromVersionID(ctx, t.Version)
	if err != nil {
		catcher.Wrapf(err, "finding project for task '%s'", t.Id)
	}
	if catcher.HasErrors() {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "unable to perform stepback/deactivate previous",
			"project": t.Project,
			"task_id": t.Id,
		}))
		return
	}

	if !evergreen.IsPatchRequester(t.Requester) {
		if t.IsPartOfDisplay(ctx) {
			var dt *task.Task
			dt, err = t.GetDisplayTask(ctx)
			if err != nil {
				err = errors.Wrap(err, "getting display task")
			} else if dt != nil {
				err = evalStepback(ctx, t.DisplayTask, status, pRef, project)
			}
		} else {
			err = evalStepback(ctx, t, status, pRef, project)
		}
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem evaluating stepback",
			"project": t.Project,
			"task_id": t.Id,
		}))
	}

	// Deactivate previous occurrences of the same task only if this one passed on mainline commits.
	if t.Status == evergreen.TaskSucceeded && t.Requester == evergreen.RepotrackerVersionRequester && t.ActivatedBy != evergreen.StepbackTaskActivator {
		shouldDeactivatePrevious := getDeactivatePrevious(t, pRef, project)
		if shouldDeactivatePrevious {
			grip.Error(message.WrapError(DeactivatePreviousTasks(ctx, t, caller), message.Fields{
				"message": "problem evaluating deactivate previous",
				"project": t.Project,
				"task_id": t.Id,
			}))
		}
	}
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

	if t.IsPartOfDisplay(ctx) {
		msg["display_task_id"] = t.DisplayTaskId
	}

	pRef, _ := FindBranchProjectRef(ctx, t.Project)
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
	}

	if !t.DependenciesMetTime.IsZero() {
		msg["dependencies_met_time"] = t.DependenciesMetTime
	}

	grip.Info(msg)
	return nil
}

// getVersionCtxForTracing returns a context with version attributes for tracing
func getVersionCtxForTracing(ctx context.Context, v *Version, project string, p *patch.Patch) (context.Context, error) {
	if v == nil {
		return nil, errors.New("version is nil")
	}

	timeTaken, makespan, err := v.GetTimeSpent(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting time spent")
	}

	attrs := []attribute.KeyValue{
		attribute.String(evergreen.VersionIDOtelAttribute, v.Id),
		attribute.String(evergreen.VersionRequesterOtelAttribute, v.Requester),
		attribute.String(evergreen.ProjectIDOtelAttribute, project),
		attribute.String(evergreen.ProjectIdentifierOtelAttribute, v.Identifier),
		attribute.String(evergreen.VersionStatusOtelAttribute, v.Status),
		attribute.String(evergreen.VersionCreateTimeOtelAttribute, v.CreateTime.String()),
		attribute.String(evergreen.VersionStartTimeOtelAttribute, v.StartTime.String()),
		attribute.String(evergreen.VersionFinishTimeOtelAttribute, v.FinishTime.String()),
		attribute.Int(evergreen.VersionTimeTakenSecondsOtelAttribute, int(timeTaken.Seconds())),
		attribute.Int(evergreen.VersionMakespanSecondsOtelAttribute, int(makespan.Seconds())),
		attribute.String(evergreen.VersionAuthorOtelAttribute, v.Author),
		attribute.String(evergreen.VersionBranchOtelAttribute, v.Branch),
	}

	if !v.Cost.IsZero() {
		attrs = append(attrs,
			attribute.Float64(evergreen.VersionOnDemandCostOtelAttribute, v.Cost.OnDemandEC2Cost),
			attribute.Float64(evergreen.VersionAdjustedCostOtelAttribute, v.Cost.AdjustedEC2Cost),
		)
	}
	if !v.PredictedCost.IsZero() {
		attrs = append(attrs,
			attribute.Float64(evergreen.VersionPredictedOnDemandCostOtelAttribute, v.PredictedCost.OnDemandEC2Cost),
			attribute.Float64(evergreen.VersionPredictedAdjustedCostOtelAttribute, v.PredictedCost.AdjustedEC2Cost),
		)
	}

	if p != nil && p.IsReconfigured {
		attrs = append(attrs, attribute.Bool(evergreen.PatchIsReconfiguredOtelAttribute, true))
	}

	ctx = utility.ContextWithAttributes(ctx, attrs)

	return ctx, nil
}

// UpdateBlockedDependencies traverses the dependency graph and recursively sets
// each parent dependency in dependencies as unattainable in depending tasks. It
// updates the status of builds as well, in case they change due to blocking
// dependencies. The ignoreDependencyStatusForBlocking indicates whether all tasks that depend
// on the given tasks will be updated (i.e., whether the task's Dependency.Status will be ignored).
func UpdateBlockedDependencies(ctx context.Context, dependencies []task.Task, ignoreDependencyStatusForBlocking bool) error {
	ctx, span := tracer.Start(ctx, "update-blocked-dependencies")
	defer span.End()

	dependencyIDs := make([]string, 0, len(dependencies))
	for _, dep := range dependencies {
		dependencyIDs = append(dependencyIDs, dep.Id)
	}

	dependentTasks, err := task.FindAllDependencyTasksToModify(ctx, dependencies, true, ignoreDependencyStatusForBlocking)
	if err != nil {
		return errors.Wrapf(err, "getting all tasks depending on tasks")
	}
	if len(dependentTasks) == 0 {
		return nil
	}

	dependentTasks, err = task.MarkAllForUnattainableDependencies(ctx, dependentTasks, dependencyIDs, true)
	if err != nil {
		return errors.Wrap(err, "marking unattainable dependencies for tasks")
	}

	if err := UpdateBlockedDependencies(ctx, dependentTasks, ignoreDependencyStatusForBlocking); err != nil {
		return errors.Wrap(err, "updating many blocked dependencies recursively")
	}

	// Add the build IDs to update and use a set to make lookup for duplicates
	// faster.
	buildIDsSet := make(map[string]struct{})
	var buildIDs []string
	for _, dependentTask := range dependentTasks {
		buildID := dependentTask.BuildId
		if _, ok := buildIDsSet[buildID]; ok {
			continue
		}
		buildIDsSet[buildID] = struct{}{}
		buildIDs = append(buildIDs, buildID)
	}

	if err = UpdateVersionAndPatchStatusForBuilds(ctx, buildIDs); err != nil {
		return errors.Wrap(err, "updating build, version, and patch statuses")
	}

	return nil
}

// UpdateUnblockedDependencies recursively marks all unattainable dependencies as attainable.
func UpdateUnblockedDependencies(ctx context.Context, dependencies []task.Task) error {
	ctx, span := tracer.Start(ctx, "update-unblocked-dependencies")
	defer span.End()

	dependencyIDs := make([]string, 0, len(dependencies))
	for _, dep := range dependencies {
		dependencyIDs = append(dependencyIDs, dep.Id)
	}

	tasksToUnblock, err := task.FindAllDependencyTasksToModify(ctx, dependencies, false, false)
	if err != nil {
		return errors.Wrapf(err, "getting all tasks depending on tasks")
	}
	if len(tasksToUnblock) == 0 {
		return nil
	}

	tasksToUnblock, err = task.MarkAllForUnattainableDependencies(ctx, tasksToUnblock, dependencyIDs, false)
	if err != nil {
		return errors.Wrap(err, "marking unattainable dependencies for tasks")
	}

	if err = UpdateUnblockedDependencies(ctx, tasksToUnblock); err != nil {
		return errors.Wrap(err, "updating many unblocked dependencies recursively")
	}

	buildIDsSet := make(map[string]struct{})
	var buildIDs []string
	for _, dependentTask := range tasksToUnblock {
		buildID := dependentTask.BuildId
		if _, ok := buildIDsSet[buildID]; ok {
			continue
		}
		buildIDsSet[buildID] = struct{}{}
		buildIDs = append(buildIDs, buildID)
	}

	if err := UpdateVersionAndPatchStatusForBuilds(ctx, buildIDs); err != nil {
		return errors.Wrapf(err, "updating build, version, and patch statuses")
	}

	return nil
}

// evalStepback runs linear or bisect stepback depending on project, build variant, and task settings.
// The status passed in is a display status, initially stepback only activates if the task
// has failed but not on system failure.
func evalStepback(ctx context.Context, t *task.Task, status string,
	pRef *ProjectRef, project *Project) error {
	s, err := getStepback(ctx, t.Id, pRef, project)
	if err != nil {
		return errors.WithStack(err)
	}
	if !s.shouldStepback {
		return nil
	}
	// A new stepback happens when the display status is failed (not system failure)
	// and it is not aborted.
	newStepback := status == evergreen.TaskFailed && !t.Aborted
	if s.bisect {
		return evalBisectStepback(ctx, t, newStepback)
	}
	return evalLinearStepback(ctx, t, newStepback)
}

// evalLinearStepback performs linear stepback on the task or cleans up after previous iterations of lienar
// stepback.
func evalLinearStepback(ctx context.Context, t *task.Task, newStepback bool) error {
	existingStepback := t.Status == evergreen.TaskFailed && t.ActivatedBy == evergreen.StepbackTaskActivator
	if newStepback || existingStepback {
		if t.IsPartOfSingleHostTaskGroup() {
			// Stepback earlier task group tasks as well because these need to be run sequentially.
			catcher := grip.NewBasicCatcher()
			tasks, err := task.FindTaskGroupFromBuild(ctx, t.BuildId, t.TaskGroup)
			if err != nil {
				return errors.Wrapf(err, "getting task group for task '%s'", t.Id)
			}
			if len(tasks) == 0 {
				return errors.Errorf("no tasks in task group '%s' for task '%s'", t.TaskGroup, t.Id)
			}
			for _, tgTask := range tasks {
				catcher.Wrapf(doLinearStepback(ctx, &tgTask), "stepping back task group task '%s'", tgTask.DisplayName)
				if tgTask.Id == t.Id {
					break // don't need to stepback later tasks in the group
				}
			}

			return catcher.Resolve()
		}
		return errors.Wrap(doLinearStepback(ctx, t), "performing linear stepback")
	}
	return nil
}

// evalBisectStepback performs bisect stepback on the task.
func evalBisectStepback(ctx context.Context, t *task.Task, newStepback bool) error {
	// If the task is aborted then no-op.
	if t.Aborted {
		return nil
	}

	existingStepback := !t.StepbackInfo.IsZero() && t.ActivatedBy == evergreen.StepbackTaskActivator
	if (newStepback || existingStepback) && t.GeneratedBy == "" {
		return errors.Wrap(doBisectStepback(ctx, t), "performing bisect stepback")
	}

	if t.GeneratedBy == "" {
		return nil
	}

	generator, err := task.FindOneId(ctx, t.GeneratedBy)
	if err != nil {
		return errors.Wrapf(err, "finding generator '%s'", t.GeneratedBy)
	}
	if generator == nil {
		return errors.Errorf("nil generator '%s'", t.GeneratedBy)
	}
	// Check if the task is in stepback (if it is not a new one).
	if generator.StepbackInfo.GetStepbackInfoForGeneratedTask(t.DisplayName, t.BuildVariant) == nil && !newStepback {
		return nil
	}

	return errors.Wrapf(doBisectStepbackForGeneratedTask(ctx, generator, t), "bisect stepback on generator '%s' for generated '%s'", t.GeneratedBy, t.Id)
}

// updateMakespans updates the predicted and actual makespans for the tasks in
// the build.
func updateMakespans(ctx context.Context, b *build.Build, buildTasks []task.Task) error {
	depPath := FindPredictedMakespan(buildTasks)
	return errors.WithStack(b.UpdateMakespans(ctx, depPath.TotalTime, CalculateActualMakespan(buildTasks)))
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
func updateBuildGithubStatus(ctx context.Context, b *build.Build, buildTasks []task.Task) error {
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
		event.LogBuildGithubCheckFinishedEvent(ctx, b.Id, buildStatus.status)
	}

	return b.UpdateGithubCheckStatus(ctx, buildStatus.status)
}

// checkUpdateBuildPRStatusPending checks if the build is coming from a PR, and if so
// sends a pending status to GitHub to reflect the status of the build.
func checkUpdateBuildPRStatusPending(ctx context.Context, b *build.Build) error {
	ctx, span := tracer.Start(ctx, "check-update-build-pr-status-pending")
	defer span.End()

	if !evergreen.IsGitHubPatchRequester(b.Requester) {
		return nil
	}
	p, err := patch.FindOneId(ctx, b.Version)
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
func updateBuildStatus(ctx context.Context, b *build.Build) (bool, error) {
	buildTasks, err := task.Find(ctx, task.ByBuildId(b.Id))
	if err != nil {
		return false, errors.Wrapf(err, "getting tasks in build '%s'", b.Id)
	}

	buildStatus := getBuildStatus(buildTasks)
	// If all the tasks are unscheduled, set active to false
	if buildStatus.allTasksUnscheduled {
		if b.Activated {
			if err = b.SetActivated(ctx, false); err != nil {
				return true, errors.Wrapf(err, "setting build '%s' as inactive", b.Id)
			}
			return true, nil
		}
	}

	if err := b.SetHasUnfinishedEssentialTask(ctx, buildStatus.hasUnfinishedEssentialTask); err != nil {
		return false, errors.Wrapf(err, "setting unfinished essential task state to %t for build '%s'", buildStatus.hasUnfinishedEssentialTask, b.Id)
	}

	blockedChanged := buildStatus.allTasksBlocked != b.AllTasksBlocked

	if err = b.SetAllTasksBlocked(ctx, buildStatus.allTasksBlocked); err != nil {
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
		if err = b.SetAborted(ctx, isAborted); err != nil {
			return false, errors.Wrapf(err, "setting build '%s' as aborted", b.Id)
		}
	}

	event.LogBuildStateChangeEvent(ctx, b.Id, buildStatus.status)

	shouldActivate := !buildStatus.allTasksBlocked && !buildStatus.allTasksUnscheduled

	// if the status has changed, re-activate the build if it's not blocked
	if shouldActivate {
		if err = b.SetActivated(ctx, true); err != nil {
			return true, errors.Wrapf(err, "setting build '%s' as active", b.Id)
		}
	}

	if evergreen.IsFinishedBuildStatus(buildStatus.status) {
		if err = b.MarkFinished(ctx, buildStatus.status, time.Now()); err != nil {
			return true, errors.Wrapf(err, "marking build as finished with status '%s'", buildStatus.status)
		}
		if err = updateMakespans(ctx, b, buildTasks); err != nil {
			return true, errors.Wrapf(err, "updating makespan information for '%s'", b.Id)
		}
	} else {
		if err = b.UpdateStatus(ctx, buildStatus.status); err != nil {
			return true, errors.Wrap(err, "updating build status")
		}
	}

	if err = updateBuildGithubStatus(ctx, b, buildTasks); err != nil {
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
func updateVersionGithubStatus(ctx context.Context, v *Version, builds []build.Build) error {
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
		event.LogVersionGithubCheckFinishedEvent(ctx, v.Id, githubBuildStatus)
	}

	return nil
}

// updateVersionStatus updates the status of the version based on the status of
// its constituent builds, as well as a boolean indicating if any of them have
// unfinished essential tasks. It assumes that the build statuses have already
// been updated prior to this.
func updateVersionStatus(ctx context.Context, v *Version) (versionStatus string, statusChanged bool, err error) {
	builds, err := build.Find(ctx, build.ByVersion(v.Id))
	if err != nil {
		return "", false, errors.Wrapf(err, "getting builds for version '%s'", v.Id)
	}

	// Regardless of whether the overall version status has changed, the Github status subset may have changed.
	if err = updateVersionGithubStatus(ctx, v, builds); err != nil {
		return "", false, errors.Wrap(err, "updating version GitHub status")
	}

	versionActivated, versionStatus := getVersionActivationAndStatus(builds)
	// If all the builds are unscheduled and nothing has run, set active to false
	if versionStatus == evergreen.VersionCreated && !versionActivated {
		if err = v.SetActivated(ctx, false); err != nil {
			return "", false, errors.Wrapf(err, "setting version '%s' as inactive", v.Id)
		}
	}

	if versionStatus == v.Status {
		return versionStatus, false, nil
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
		if err = v.SetAborted(ctx, isAborted); err != nil {
			return "", false, errors.Wrapf(err, "setting version '%s' as aborted", v.Id)
		}
	}

	statusChanged, err = v.UpdateStatus(ctx, versionStatus)
	if err != nil {
		return "", false, errors.Wrapf(err, "updating version '%s' with status '%s'", v.Id, versionStatus)
	}

	if statusChanged {
		event.LogVersionStateChangeEvent(ctx, v.Id, versionStatus)
	}

	return versionStatus, statusChanged, nil
}

type patchStatusUpdate struct {
	patchStatusChanged                  bool
	isPatchFamilyDone                   bool
	parentPatch                         *patch.Patch
	patchFamilyFinishedCollectiveStatus string
}

// updatePatchStatus updates the status of a patch. It returns information about
// the patch status. If the patch status changed, it also includes patch family
// information.
func updatePatchStatus(ctx context.Context, p *patch.Patch, status string) (patchStatusUpdate, error) {
	var psu patchStatusUpdate
	if status == p.Status {
		return psu, nil
	}

	statusChanged, err := p.UpdateStatus(ctx, status)
	if err != nil {
		return psu, errors.Wrapf(err, "updating patch '%s' with status '%s'", p.Id.Hex(), status)
	}

	if statusChanged {
		psu.patchStatusChanged = true

		event.LogPatchStateChangeEvent(ctx, p.Version, status)

		isDone, parentPatch, err := p.GetFamilyInformation(ctx)
		if err != nil {
			return psu, errors.Wrapf(err, "getting family information for patch '%s'", p.Id.Hex())
		}

		psu.parentPatch = parentPatch
		psu.isPatchFamilyDone = isDone

		if isDone {
			collectiveStatus, err := p.CollectiveStatus(ctx)
			if err != nil {
				return psu, errors.Wrapf(err, "getting collective status for patch '%s'", p.Id.Hex())
			}
			if parentPatch != nil {
				event.LogPatchChildrenCompletionEvent(ctx, parentPatch.Id.Hex(), collectiveStatus, parentPatch.Author)
				if err = p.SetChildrenCompletedTime(ctx, parentPatch.FinishTime); err != nil {
					return psu, errors.Wrapf(err, "setting finish time for patch '%s'", p.Id.Hex())
				}
			} else {
				event.LogPatchChildrenCompletionEvent(ctx, p.Id.Hex(), collectiveStatus, p.Author)
				if err = p.SetChildrenCompletedTime(ctx, p.FinishTime); err != nil {
					return psu, errors.Wrapf(err, "setting finish time for patch '%s'", p.Id.Hex())
				}
			}
			psu.patchFamilyFinishedCollectiveStatus = collectiveStatus
		}

	}

	return psu, nil
}

// UpdateBuildAndVersionStatusForTask updates the status of the task's build based on all the tasks in the build
// and the task's version based on all the builds in the version.
// Also update build and version Github statuses based on the subset of tasks and builds included in github checks
func UpdateBuildAndVersionStatusForTask(ctx context.Context, t *task.Task) error {
	ctx, span := tracer.Start(ctx, "save-builds-and-tasks")
	defer span.End()

	taskBuild, err := build.FindOneId(ctx, t.BuildId)
	if err != nil {
		return errors.Wrapf(err, "getting build for task '%s'", t.Id)
	}
	if taskBuild == nil {
		return errors.Errorf("no build '%s' found for task '%s'", t.BuildId, t.Id)
	}
	buildStatusChanged, err := updateBuildStatus(ctx, taskBuild)
	if err != nil {
		return errors.Wrapf(err, "updating build '%s' status", taskBuild.Id)
	}
	// If the build status and activation have not changed,
	// then the version and patch statuses and activation must have also not changed.
	if !buildStatusChanged {
		return nil
	}

	taskVersion, err := VersionFindOneId(ctx, t.Version)
	if err != nil {
		return errors.Wrapf(err, "getting version '%s' for task '%s'", t.Version, t.Id)
	}
	if taskVersion == nil {
		return errors.Errorf("no version '%s' found for task '%s'", t.Version, t.Id)
	}

	newVersionStatus, versionStatusChanged, err := updateVersionStatus(ctx, taskVersion)
	if err != nil {
		return errors.Wrapf(err, "updating version '%s' status", taskVersion.Id)
	}

	if !evergreen.IsFinishedVersionStatus(newVersionStatus) && evergreen.IsFinishedVersionStatus(taskVersion.Status) {
		if err = checkUpdateBuildPRStatusPending(ctx, taskBuild); err != nil {
			return errors.Wrapf(err, "updating build '%s' PR status", taskBuild.Id)
		}
	}

	if versionStatusChanged && evergreen.IsFinishedVersionStatus(newVersionStatus) && !evergreen.IsPatchRequester(taskVersion.Requester) {
		// only add tracing for versions, patches need to wait for child patches
		traceContext, err := getVersionCtxForTracing(ctx, taskVersion, t.Project, nil)
		if err != nil {
			return errors.Wrap(err, "getting context for tracing")
		}
		// use a new root span so that it logs it every time instead of only logging a small sample set as an http call span
		_, span := tracer.Start(traceContext, "version-completion", trace.WithNewRoot())
		defer span.End()

		return nil
	}

	if evergreen.IsPatchRequester(taskVersion.Requester) {
		p, err := patch.FindOneId(ctx, taskVersion.Id)
		if err != nil {
			return errors.Wrapf(err, "getting patch for version '%s'", taskVersion.Id)
		}
		if p == nil {
			return errors.Errorf("no patch found for version '%s'", taskVersion.Id)
		}
		psu, err := updatePatchStatus(ctx, p, newVersionStatus)
		if err != nil {
			return errors.Wrapf(err, "updating patch '%s' status", p.Id.Hex())
		}

		if psu.patchStatusChanged && psu.isPatchFamilyDone {
			rootPatch := p
			if psu.parentPatch != nil {
				rootPatch = psu.parentPatch
			}

			event.LogVersionChildrenCompletionEvent(ctx, rootPatch.Id.Hex(), psu.patchFamilyFinishedCollectiveStatus, rootPatch.Author)

			traceContext, err := getVersionCtxForTracing(ctx, taskVersion, t.Project, rootPatch)
			if err != nil {
				return errors.Wrap(err, "getting context for tracing")
			}
			_, span := tracer.Start(traceContext, "version-completion", trace.WithNewRoot())
			defer span.End()

			grip.Error(message.WrapError(emitMergeQueueCompletionMetrics(ctx, rootPatch, taskVersion, psu.patchFamilyFinishedCollectiveStatus), message.Fields{
				"message":           "error emitting merge queue completion metrics",
				"version_id":        taskVersion.Id,
				"patch_id":          rootPatch.Id.Hex(),
				"collective_status": psu.patchFamilyFinishedCollectiveStatus,
			}))
		}
	}

	return nil
}

type mergeQueueTaskMetrics struct {
	variantMap        map[string]bool
	hasRunningTasks   bool
	failedCount       int
	hasTestFailure    bool
	hasSystemFailure  bool
	hasSetupFailure   bool
	hasTimeoutFailure bool
	slowestTask       *task.Task
	slowestDuration   time.Duration
}

// gatherMergeQueueTaskMetrics analyzes tasks and collects metrics for merge queue completion.
func gatherMergeQueueTaskMetrics(tasks []task.Task) mergeQueueTaskMetrics {
	metrics := mergeQueueTaskMetrics{
		variantMap: make(map[string]bool),
	}

	for i := range tasks {
		t := &tasks[i]
		metrics.variantMap[t.BuildVariant] = true

		if t.Status == evergreen.TaskStarted || t.Status == evergreen.TaskDispatched {
			metrics.hasRunningTasks = true
		}

		if evergreen.IsFailedTaskStatus(t.Status) || t.Aborted {
			metrics.failedCount++

			if t.Details.TimedOut {
				metrics.hasTimeoutFailure = true
			}

			if !t.Aborted {
				displayStatus := t.GetDisplayStatus()
				switch displayStatus {
				case evergreen.TaskSystemFailed, evergreen.TaskSystemTimedOut, evergreen.TaskSystemUnresponse:
					metrics.hasSystemFailure = true
				case evergreen.TaskSetupFailed:
					metrics.hasSetupFailure = true
				default:
					metrics.hasTestFailure = true
				}
			}
		}

		if t.FinishTime.After(t.StartTime) {
			duration := t.FinishTime.Sub(t.StartTime)
			if metrics.slowestTask == nil || duration > metrics.slowestDuration {
				metrics.slowestTask = t
				metrics.slowestDuration = duration
			}
		}
	}

	return metrics
}

// emitMergeQueueCompletionMetrics emits OpenTelemetry metrics for merge queue version completion.
func emitMergeQueueCompletionMetrics(ctx context.Context, p *patch.Patch, v *Version, collectiveStatus string) error {
	if p.Alias != evergreen.CommitQueueAlias || v.Requester != evergreen.GithubMergeRequester {
		return nil
	}

	githubHeadPRURL := thirdparty.BuildGithubHeadPRURL(p.GithubMergeData.Org, p.GithubMergeData.Repo, p.GithubMergeData.HeadBranch)

	projectRef, err := FindBranchProjectRef(ctx, p.Project)
	if err != nil {
		return errors.Wrap(err, "finding project ref for merge queue metrics")
	}

	baseAttrs := patch.BuildMergeQueueSpanAttributes(
		p.GithubMergeData.Org,
		p.GithubMergeData.Repo,
		p.GithubMergeData.BaseBranch,
		p.GithubMergeData.HeadSHA,
		githubHeadPRURL,
	)
	baseAttrs = append(baseAttrs,
		attribute.String(patch.MergeQueueAttrPatchID, p.Id.Hex()),
		attribute.String(patch.MergeQueueAttrProjectID, projectRef.Identifier),
	)
	ctx, span := tracer.Start(ctx, patch.MergeQueuePatchCompletedSpan,
		trace.WithAttributes(baseAttrs...))
	defer span.End()

	if !p.FinishTime.IsZero() && !p.CreateTime.IsZero() {
		timeInQueue := p.FinishTime.Sub(p.CreateTime).Milliseconds()
		span.SetAttributes(attribute.Int64(patch.MergeQueueAttrTimeInQueueMs, timeInQueue))
	}

	// Collect all version IDs for the patch family (parent + all children).
	versionIDs, err := patch.GetFinalizedChildPatchIdsForPatch(ctx, p.Id.Hex())
	if err != nil {
		return errors.Wrap(err, "getting child patches for merge queue metrics")
	}
	versionIDs = append([]string{p.Version}, versionIDs...)

	// Find the earliest task start time across all versions in the patch family.
	var firstTaskStartTime time.Time
	for _, versionID := range versionIDs {
		startTime, err := task.GetFirstTaskStartTimeForVersion(ctx, versionID)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "error getting first task start time for merge queue version",
				"version_id": versionID,
				"patch_id":   p.Id.Hex(),
			}))
			continue
		}
		if !startTime.IsZero() && (firstTaskStartTime.IsZero() || startTime.Before(firstTaskStartTime)) {
			firstTaskStartTime = startTime
		}
	}
	if !firstTaskStartTime.IsZero() && !p.CreateTime.IsZero() {
		timeToFirstTask := firstTaskStartTime.Sub(p.CreateTime).Milliseconds()
		span.SetAttributes(attribute.Int64(patch.MergeQueueAttrTimeToFirstTaskMs, timeToFirstTask))
	}

	tasks, err := task.FindAll(ctx, db.Query(task.ByVersions(versionIDs)))
	if err != nil {
		return errors.Wrap(err, "querying tasks for merge queue version")
	}

	// Determine status: if GitHub sent an explicit removal reason (via destroyed webhook), use that;
	// otherwise infer status from the collective status of the patch family (parent patch + all child patches)
	var mergeQueueStatus string
	if !p.GithubMergeData.RemovedFromQueueAt.IsZero() && p.GithubMergeData.RemovalReason != "" {
		mergeQueueStatus = thirdparty.GetMergeQueueStatusFromReason(p.GithubMergeData.RemovalReason)
		span.SetAttributes(attribute.String(patch.MergeQueueAttrRemovalReason, p.GithubMergeData.RemovalReason))
	} else {
		if collectiveStatus == evergreen.VersionSucceeded {
			mergeQueueStatus = thirdparty.MergeQueueStatusSuccess
		} else {
			mergeQueueStatus = thirdparty.MergeQueueStatusFailed
		}
	}
	span.SetAttributes(attribute.String(patch.MergeQueueAttrStatus, mergeQueueStatus))

	totalCount := len(tasks)
	metrics := gatherMergeQueueTaskMetrics(tasks)

	if mergeQueueStatus == thirdparty.MergeQueueStatusFailed {
		span.SetAttributes(
			attribute.Bool(patch.MergeQueueAttrHasTestFailure, metrics.hasTestFailure),
			attribute.Bool(patch.MergeQueueAttrHasSystemFailure, metrics.hasSystemFailure),
			attribute.Bool(patch.MergeQueueAttrHasSetupFailure, metrics.hasSetupFailure),
			attribute.Bool(patch.MergeQueueAttrHasTimeoutFailure, metrics.hasTimeoutFailure),
			attribute.Int64(patch.MergeQueueAttrFailedTaskCount, int64(metrics.failedCount)),
		)
	}
	span.SetAttributes(
		attribute.Int64(patch.MergeQueueAttrTotalTaskCount, int64(totalCount)),
		attribute.Bool(patch.MergeQueueAttrHasRunningTasks, metrics.hasRunningTasks),
	)

	variants := make([]string, 0, len(metrics.variantMap))
	for variant := range metrics.variantMap {
		variants = append(variants, variant)
	}
	sort.Strings(variants)
	span.SetAttributes(attribute.StringSlice(patch.MergeQueueAttrVariants, variants))

	if metrics.slowestTask != nil {
		span.SetAttributes(
			attribute.String(patch.MergeQueueAttrSlowestTaskID, metrics.slowestTask.Id),
			attribute.String(patch.MergeQueueAttrSlowestTaskName, metrics.slowestTask.DisplayName),
			attribute.Int64(patch.MergeQueueAttrSlowestTaskDurationMs, metrics.slowestDuration.Milliseconds()),
			attribute.String(patch.MergeQueueAttrSlowestTaskVariant, metrics.slowestTask.BuildVariant),
		)
	}

	return nil
}

// UpdateVersionAndPatchStatusForBuilds updates the status of all versions,
// patches and builds associated with the given input list of build IDs.
func UpdateVersionAndPatchStatusForBuilds(ctx context.Context, buildIds []string) error {
	if len(buildIds) == 0 {
		return nil
	}
	builds, err := build.Find(ctx, build.ByIds(buildIds))
	if err != nil {
		return errors.Wrapf(err, "fetching builds")
	}

	versionsToUpdate := make(map[string]bool)
	for _, build := range builds {
		buildStatusChanged, err := updateBuildStatus(ctx, &build)
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
		buildVersion, err := VersionFindOneId(ctx, versionId)
		if err != nil {
			return errors.Wrapf(err, "getting version '%s'", versionId)
		}
		if buildVersion == nil {
			return errors.Errorf("no version '%s' found", versionId)
		}
		newVersionStatus, versionStatusChanged, err := updateVersionStatus(ctx, buildVersion)
		if err != nil {
			return errors.Wrapf(err, "updating version '%s' status", buildVersion.Id)
		}

		if !versionStatusChanged {
			// If the version stayed the same, then the patch status has also
			// stayed the same.
			continue
		}

		if evergreen.IsPatchRequester(buildVersion.Requester) {
			p, err := patch.FindOneId(ctx, buildVersion.Id)
			if err != nil {
				return errors.Wrapf(err, "getting patch for version '%s'", buildVersion.Id)
			}
			if p == nil {
				return errors.Errorf("no patch found for version '%s'", buildVersion.Id)
			}
			if _, err = updatePatchStatus(ctx, p, newVersionStatus); err != nil {
				return errors.Wrapf(err, "updating patch '%s' status", p.Id.Hex())
			}
		}
	}

	return nil
}

// MarkStart updates the task, build, version and if necessary, patch documents with the task start time
func MarkStart(ctx context.Context, t *task.Task, updates *StatusChanges) error {
	var err error

	startTime := time.Now().Round(time.Millisecond)

	if err = t.MarkStart(ctx, startTime); err != nil {
		return errors.WithStack(err)
	}
	event.LogTaskStarted(ctx, t.Id, t.Execution)

	// ensure the appropriate build is marked as started if necessary
	if err = build.TryMarkStarted(ctx, t.BuildId, startTime); err != nil {
		return errors.Wrap(err, "marking build started")
	}

	// ensure the appropriate version is marked as started if necessary
	if err = TryMarkVersionStarted(ctx, t.Version, startTime); err != nil {
		return errors.Wrap(err, "marking version started")
	}

	// if it's a patch, mark the patch as started if necessary
	if evergreen.IsPatchRequester(t.Requester) {
		err := patch.TryMarkStarted(ctx, t.Version, startTime)
		if err == nil {
			updates.PatchNewStatus = evergreen.VersionStarted

		} else if !adb.ResultsNotFound(err) {
			return errors.WithStack(err)
		}
	}

	if t.IsPartOfDisplay(ctx) {
		return UpdateDisplayTaskForTask(ctx, t)
	}

	return nil
}

// MarkHostTaskDispatched marks a task as being dispatched to the host. If it's
// part of a display task, update the display task as necessary.
func MarkHostTaskDispatched(ctx context.Context, t *task.Task, h *host.Host) error {
	if err := t.MarkAsHostDispatched(ctx, h.Id, h.Distro.Id, h.AgentRevision, time.Now()); err != nil {
		return errors.Wrapf(err, "marking task '%s' as dispatched "+
			"on host '%s'", t.Id, h.Id)
	}

	event.LogHostTaskDispatched(ctx, t.Id, t.Execution, h.Id)

	if t.IsPartOfDisplay(ctx) {
		return UpdateDisplayTaskForTask(ctx, t)
	}

	return nil
}

func MarkOneTaskReset(ctx context.Context, t *task.Task, caller string) error {
	if t.DisplayOnly {
		execTaskIdsToRestart, err := task.FindExecTasksToReset(ctx, t)
		if err != nil {
			return errors.Wrap(err, "finding execution tasks to restart")
		}
		if err = MarkTasksReset(ctx, execTaskIdsToRestart, caller); err != nil {
			return errors.Wrap(err, "resetting failed execution tasks")
		}

		grip.Error(message.WrapError(logExecutionTasksRestarted(ctx, t, execTaskIdsToRestart, caller), message.Fields{
			"message":                      "could not log task restart events for some execution tasks",
			"display_task_id":              t.Id,
			"restarted_execution_task_ids": execTaskIdsToRestart,
		}))
	}

	if err := t.Reset(ctx, caller); err != nil && !adb.ResultsNotFound(err) {
		return errors.Wrap(err, "resetting task in database")
	}

	if err := UpdateUnblockedDependencies(ctx, []task.Task{*t}); err != nil {
		return errors.Wrap(err, "clearing unattainable dependencies")
	}

	if err := t.MarkDependenciesFinished(ctx, false); err != nil {
		return errors.Wrap(err, "marking direct dependencies unfinished")
	}

	return nil
}

func logExecutionTasksRestarted(ctx context.Context, displayTask *task.Task, execTaskIDsRestarted []string, caller string) error {
	if !displayTask.DisplayOnly {
		return nil
	}

	catcher := grip.NewBasicCatcher()
	for _, etID := range execTaskIDsRestarted {
		execTask, err := task.FindOneId(ctx, etID)
		if err != nil {
			catcher.Wrapf(err, "finding execution task '%s'", etID)
			continue
		}
		if execTask == nil {
			catcher.Errorf("execution task '%s' not found", etID)
			continue
		}
		// Use the previous execution number rather than latest execution
		// number. The task restart event is supposed to log the previous
		// execution that was restarted (e.g. if it restarted from execution 2
		// to execution 3, the restart event should log for execution 2). This
		// logic always logs after the execution task has already been
		// restarted, so it needs to use the previous execution number.
		event.LogTaskRestarted(ctx, execTask.Id, execTask.Execution-1, caller)
	}

	return catcher.Resolve()
}

// MarkTasksReset resets many tasks by their IDs. For execution tasks, this also
// resets their parent display tasks.
func MarkTasksReset(ctx context.Context, taskIds []string, caller string) error {
	tasks, err := task.FindAll(ctx, db.Query(task.ByIds(taskIds)))
	if err != nil {
		return errors.WithStack(err)
	}
	tasks, err = task.AddParentDisplayTasks(ctx, tasks)
	if err != nil {
		return errors.WithStack(err)
	}

	if err = task.ResetTasks(ctx, tasks, caller); err != nil {
		return errors.Wrap(err, "resetting tasks in database")
	}

	catcher := grip.NewBasicCatcher()
	catcher.Wrapf(UpdateUnblockedDependencies(ctx, tasks), "clearing unattainable dependencies for tasks")
	for _, t := range tasks {
		catcher.Wrapf(t.MarkDependenciesFinished(ctx, false), "marking direct dependencies unfinished for task '%s'", t.Id)
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
	tasksToRestart, err := task.FindAll(ctx, db.Query(task.ByTimeStartedAndFailed(opts.StartTime, opts.EndTime, failureTypes)))
	if err != nil {
		return results, errors.WithStack(err)
	}
	tasksToRestart, err = task.AddParentDisplayTasks(ctx, tasksToRestart)
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
		if t.IsPartOfDisplay(ctx) {
			dt, err := t.GetDisplayTask(ctx)
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
			if err = dt.SetResetWhenFinished(ctx, opts.User); err != nil {
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

// ClearAndResetStrandedHostTask clears the host task dispatched to the host due
// to being stranded on a bad host (e.g. one that has been terminated). It also
// marks the current task execution as finished and, if possible, a new
// execution is created to restart the task.
func ClearAndResetStrandedHostTask(ctx context.Context, settings *evergreen.Settings, h *host.Host) error {
	if h.RunningTask == "" {
		return nil
	}

	t, err := task.FindOneIdAndExecution(ctx, h.RunningTask, h.RunningTaskExecution)
	if err != nil {
		return errors.Wrapf(err, "finding running task '%s' execution '%d' from host '%s'", h.RunningTask, h.RunningTaskExecution, h.Id)
	} else if t == nil {
		return nil
	}

	if err = h.ClearRunningTask(ctx); err != nil {
		return errors.Wrapf(err, "clearing running task from host '%s'", h.Id)
	}

	if err := endAndResetSystemFailedTask(ctx, settings, t, evergreen.TaskDescriptionStranded); err != nil {
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
	failureDesc := evergreen.TaskDescriptionHeartbeat
	if t.Aborted {
		failureDesc = evergreen.TaskDescriptionAborted
		if err := finishStaleAbortedTask(ctx, settings, t); err != nil {
			return errors.Wrapf(err, "finishing stale aborted task '%s'", t.Id)
		}
	} else {
		if err := endAndResetSystemFailedTask(ctx, settings, t, failureDesc); err != nil {
			if !t.IsPartOfDisplay(ctx) {
				return errors.Wrap(err, "resetting heartbeat task")
			}
			// It's possible for display tasks to race, since multiple execution tasks can system fail at the same time.
			// Only error if the display task hasn't actually been reset.
			dt, dbErr := t.GetDisplayTask(ctx)
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
	if err := MarkEnd(ctx, settings, t, evergreen.APIServerTaskActivator, time.Now(), failureDetails); err != nil {
		return errors.Wrapf(err, "calling mark finish on task '%s'", t.Id)
	}
	return nil
}

// endAndResetSystemFailedTask finishes and resets a task that has encountered a system failure
// such as being stranded on a terminated host/container or failing to send a
// heartbeat.
func endAndResetSystemFailedTask(ctx context.Context, settings *evergreen.Settings, t *task.Task, description string) error {
	if t.IsFinished() {
		return nil
	}

	unschedulableTask := time.Since(t.ActivatedTime) > task.UnschedulableThreshold

	// Mark the task as finished (without restarting) if restarts are disabled, or the task isn't on its first execution.
	shouldSkipRetry := settings.ServiceFlags.SystemFailedTaskRestartDisabled || t.Execution > 0
	if unschedulableTask || shouldSkipRetry || t.IsStuckTask() {
		failureDetails := task.GetSystemFailureDetails(description)
		// If the task has already exceeded the unschedulable threshold, we
		// don't want to restart it, so just mark it as finished.
		if t.DisplayOnly {
			execTasks, err := task.FindAll(ctx, db.Query(task.ByIds(t.ExecutionTasks)))
			if err != nil {
				return errors.Wrap(err, "finding execution tasks")
			}
			for _, execTask := range execTasks {
				if !evergreen.IsFinishedTaskStatus(execTask.Status) {
					if err = MarkEnd(ctx, settings, &execTask, evergreen.MonitorPackage, time.Now(), &failureDetails); err != nil {
						return errors.Wrap(err, "marking execution task as ended")
					}
				}
			}
		}
		return errors.WithStack(MarkEnd(ctx, settings, t, evergreen.MonitorPackage, time.Now(), &failureDetails))
	}

	if err := t.MarkSystemFailed(ctx, description); err != nil {
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
	if taskToReset.IsPartOfDisplay(ctx) { // if given an execution task, attempt to restart the full display task
		dt, err := taskToReset.GetDisplayTask(ctx)
		if err != nil {
			return errors.Wrap(err, "getting display task")
		}
		if dt != nil {
			taskToReset = *dt
		}
	}
	if taskToReset.DisplayOnly {
		if failedOnly {
			if err := taskToReset.SetResetFailedWhenFinished(ctx, user); err != nil {
				return errors.Wrap(err, "marking display task for reset")
			}
		} else {
			if err := taskToReset.SetResetWhenFinished(ctx, user); err != nil {
				return errors.Wrap(err, "marking display task for reset")
			}
		}
		return errors.Wrap(checkResetDisplayTask(ctx, settings, user, origin, &taskToReset), "checking and resetting display task")
	}

	return errors.Wrap(TryResetTask(ctx, settings, t.Id, user, origin, detail), "resetting task")
}

// UpdateDisplayTaskForTask updates the status of the given execution task's display task
func UpdateDisplayTaskForTask(ctx context.Context, t *task.Task) error {
	if !t.IsPartOfDisplay(ctx) {
		return errors.Errorf("task '%s' is not an execution task", t.Id)
	}

	// The display task status update can retry in case it temporarily
	// conflicts with other execution tasks that are updating it at the same
	// time (e.g. because end task is running for multiple execution tasks in
	// parallel, so some see it as still running, but some see it as finished).
	// While there's no exact number of times that this update can fail
	// theoretically, the number of attempts is arbitrarily kept small, because
	// the practical likelihood of an execution task racing more than once in a
	// row is low.
	const maxUpdateAttempts = 3
	var (
		originalDisplayTask *task.Task
		updatedDisplayTask  *task.Task
		err                 error
	)
	for i := 0; i < maxUpdateAttempts; i++ {
		// Clear the cached display task, if any (e.g. due to a prior
		// GetDisplayTask). The display task fetched here must always contain
		// the latest display task data.
		t.DisplayTask = nil

		originalDisplayTask, err = t.GetDisplayTask(ctx)
		if err != nil {
			return errors.Wrap(err, "getting display task for task")
		}
		if originalDisplayTask == nil {
			grip.Error(message.Fields{
				"message":         "task may hold a display task that doesn't exist",
				"task_id":         t.Id,
				"display_task_id": t.DisplayTaskId,
			})
			return errors.Errorf("display task not found for task '%s'", t.Id)
		}
		if !originalDisplayTask.DisplayOnly {
			return errors.Errorf("task '%s' is not a display task", originalDisplayTask.Id)
		}

		updatedDisplayTask, err = tryUpdateDisplayTaskAtomically(ctx, *originalDisplayTask)
		if err == nil {
			// Update the cached display task in case it's used later on.
			t.DisplayTask = updatedDisplayTask
			break
		}

		if i >= maxUpdateAttempts-1 {
			return errors.Wrapf(err, "updating display task '%s' for execution task '%s'", originalDisplayTask.Id, t.Id)
		}
	}

	if !originalDisplayTask.IsFinished() && updatedDisplayTask.IsFinished() {
		event.LogTaskFinished(ctx, originalDisplayTask.Id, originalDisplayTask.Execution, updatedDisplayTask.GetDisplayStatus())
		grip.Info(message.Fields{
			"message":   "display task finished",
			"task_id":   originalDisplayTask.Id,
			"status":    originalDisplayTask.Status,
			"operation": "UpdateDisplayTaskForTask",
		})
	}

	return nil
}

func tryUpdateDisplayTaskAtomically(ctx context.Context, dt task.Task) (updated *task.Task, err error) {
	originalStatus := dt.Status

	execTasks, err := task.Find(ctx, task.ByIds(dt.ExecutionTasks))
	if err != nil {
		return &dt, errors.Wrap(err, "retrieving execution tasks")
	}
	if len(execTasks) == 0 {
		return nil, errors.Errorf("display task '%s' has no execution tasks", dt.Id)
	}

	hasFinishedTasks := false
	hasTasksToRun := false
	startTime := time.Unix(1<<62, 0)
	endTime := utility.ZeroTime
	noActiveTasks := true
	var timeTaken time.Duration
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
		} else if (execTask.IsHostDispatchable() || execTask.IsAbortable()) && !execTask.Blocked() {
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
	statusTask := execTasks[0]
	if hasFinishedTasks && hasTasksToRun {
		// if an unblocked display task has a mix of finished and unfinished tasks, the display task is still
		// "started" even if there aren't currently running tasks
		statusTask.Status = evergreen.TaskStarted
		statusTask.Details = apimodels.TaskEndDetail{}
	}

	dt.Status = statusTask.Status
	dt.Details = statusTask.Details
	dt.Details.TraceID = "" // Unset TraceID because display tasks don't have corresponding traces.
	dt.TimeTaken = timeTaken
	dt.DisplayStatusCache = statusTask.DetermineDisplayStatus()

	update := bson.M{
		task.StatusKey:             dt.Status,
		task.ActivatedKey:          dt.Activated,
		task.ActivatedTimeKey:      dt.ActivatedTime,
		task.TimeTakenKey:          dt.TimeTaken,
		task.DetailsKey:            dt.Details,
		task.DisplayStatusCacheKey: dt.DisplayStatusCache,
	}

	if startTime != time.Unix(1<<62, 0) {
		dt.StartTime = startTime
		update[task.StartTimeKey] = dt.StartTime
	}
	if endTime != utility.ZeroTime && !hasTasksToRun {
		dt.FinishTime = endTime
		update[task.FinishTimeKey] = dt.FinishTime
	}

	checkStatuses := []string{originalStatus}
	if dt.Status != evergreen.TaskUndispatched {
		// This is a small optimization to reduce update conflicts in the case
		// where the updated status is the same as the current display task
		// status.
		// The display task update is allowed to go through if the display task
		// status hasn't been changed (i.e. there was no race to update the
		// status) or the updated status is the same as the current status in
		// the DB (i.e. there was a race, but the race would ultimately still
		// set the display task to the same resulting status). The one exception
		// is if the display task is being updated to undispatched, then it's
		// not sufficient to just check the status was not updated. In the case
		// of updating to undispatched, the activation status must be up-to-date
		// since that determines if it's scheduled to run or will not run.
		checkStatuses = append(checkStatuses, dt.Status)
	}

	if err := task.UpdateOne(
		ctx,
		bson.M{
			task.IdKey: dt.Id,
			// Require that the status/activation state is updated atomically.
			// For the update to succeed, either the status has not changed
			// since the status was originally calculated, or the updated status
			// is already the current display task status.
			// If the status doesn't match the original or updated status, then
			// then this update is potentially invalid because it's based on
			// outdated data from the execution tasks.
			task.StatusKey: bson.M{"$in": checkStatuses},
		},
		bson.M{
			"$set": update,
		}); err != nil {
		return &dt, errors.Wrap(err, "updating display task")
	}

	return &dt, nil
}

// checkResetSingleHostTaskGroup attempts to reset all tasks that are part of
// the same single-host task group as t once all tasks in the task group are
// finished running.
func checkResetSingleHostTaskGroup(ctx context.Context, t *task.Task, caller string) error {
	if !t.IsPartOfSingleHostTaskGroup() {
		return nil
	}
	tasks, err := task.FindTaskGroupFromBuild(ctx, t.BuildId, t.TaskGroup)
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
		if tgTask.IsAutomaticRestart {
			caller = evergreen.AutoRestartActivator
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
func checkResetDisplayTask(ctx context.Context, setting *evergreen.Settings, user, origin string, t *task.Task) (theErr error) {
	if !t.ResetWhenFinished && !t.ResetFailedWhenFinished {
		return nil
	}
	execTasks, err := task.Find(ctx, task.ByIds(t.ExecutionTasks))
	if err != nil {
		return errors.Wrapf(err, "getting execution tasks for display task '%s'", t.Id)
	}
	hasFailedExecTask := false
	for _, execTask := range execTasks {
		if !execTask.IsFinished() && !execTask.Blocked() && execTask.Activated {
			return nil // all tasks not finished
		}
		if execTask.Status == evergreen.TaskFailed {
			hasFailedExecTask = true
		}
	}
	if t.IsRestartFailedOnly() && !hasFailedExecTask {
		// Do not reset the display task if the execution tasks have all
		// succeeded because it's only supposed to reset on failure.
		return nil
	}

	details := &t.Details
	// Assign task end details to indicate system failure if we receive no valid details
	if details.IsEmpty() && !t.IsFinished() {
		details = &apimodels.TaskEndDetail{
			Type:   evergreen.CommandTypeSystem,
			Status: evergreen.TaskFailed,
		}
	}
	if t.IsAutomaticRestart {
		user = evergreen.AutoRestartActivator
	}
	return errors.Wrap(TryResetTask(ctx, setting, t.Id, user, origin, details), "resetting display task")
}

// HandleEndTaskForGithubMergeQueueTask stops running GitHub merge queue tasks as soon as one task is finished.
// This is done to save resources and speed up the CI processing by preventing unnecessary tasks from running.
func HandleEndTaskForGithubMergeQueueTask(ctx context.Context, t *task.Task, status string) error {
	// If the task has succeeded, we don't need to do anything.
	// If the task is already aborted, we shouldn't do anything, because the version has already been aborted.
	if status == evergreen.TaskSucceeded || t.Aborted {
		return nil
	}

	reason := fmt.Sprintf("task '%s' on variant '%s' failed", t.DisplayName, t.BuildVariantDisplayName)
	if err := SetVersionActivation(ctx, t.Version, false, reason); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(task.AbortVersionTasks(ctx, t.Version, task.AbortInfo{TaskID: t.Id, User: evergreen.GithubMergeRequester}))
}

// UpdateOtelMetadata is called to update the task's Details with DiskDevices and TraceID.
// If there's an error here, we log but we don't return an error.
func UpdateOtelMetadata(ctx context.Context, t *task.Task, diskDevices []string, traceID string) {
	// Update the task's Details with DiskDevices and TraceID
	update := bson.M{}
	if len(diskDevices) > 0 {
		update[bsonutil.GetDottedKeyName(task.DetailsKey, task.TaskEndDetailDiskDevicesKey)] = diskDevices
	}
	if traceID != "" {
		update[bsonutil.GetDottedKeyName(task.DetailsKey, task.TaskEndDetailTraceIDKey)] = traceID
	}

	if len(update) > 0 {
		err := task.UpdateOne(
			ctx,
			task.ById(t.Id),
			bson.M{"$set": update},
		)
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem updating otel metadata",
			"task_id": t.Id,
			"update":  update,
		}))
	}
}
