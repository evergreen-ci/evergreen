package model

import (
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
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

func SetActiveState(t *task.Task, caller string, active bool) error {
	originalTasks := []task.Task{*t}
	if t.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
		if err != nil {
			return errors.Wrapf(err, "error getting execution tasks")
		}
		originalTasks = append(originalTasks, execTasks...)
	}

	if active {
		// if the task is being activated and it doesn't override its dependencies
		// activate the task's dependencies as well
		tasksToActivate := []task.Task{}
		if !t.OverrideDependencies {
			deps, err := task.GetRecursiveDependenciesUp(originalTasks, nil)
			if err != nil {
				return errors.Wrapf(err, "error getting tasks '%s' depends on", t.Id)
			}
			tasksToActivate = append(tasksToActivate, deps...)
		}

		if !utility.IsZeroTime(t.DispatchTime) && t.Status == evergreen.TaskUndispatched {
			if err := resetTask(t.Id, caller, false); err != nil {
				return errors.Wrap(err, "error resetting task")
			}
		} else {
			tasksToActivate = append(tasksToActivate, originalTasks...)
		}
		if err := task.ActivateTasks(tasksToActivate, time.Now(), caller); err != nil {
			return errors.Wrapf(err, "can't activate tasks")
		}

		if t.DistroId == "" && !t.DisplayOnly {
			grip.Critical(message.Fields{
				"message": "task is missing distro id",
				"task_id": t.Id,
			})
		}

		version, err := VersionFindOneId(t.Version)
		if err != nil {
			return errors.Wrapf(err, "error find associated version")
		}
		if version == nil {
			return errors.Errorf("could not find associated version : `%s`", t.Version)
		}
		if err = version.SetActivated(); err != nil {
			return errors.Wrapf(err, "Error marking version as activated")
		}

		// If the task was not activated by step back, and either the caller is not evergreen
		// or the task was originally activated by evergreen, deactivate the task
	} else if !evergreen.IsSystemActivator(caller) || evergreen.IsSystemActivator(t.ActivatedBy) {
		// We are trying to deactivate this task
		// So we check if the person trying to deactivate is evergreen.
		// If it is not, then we can deactivate it.
		// Otherwise, if it was originally activated by evergreen, anything can
		// deactivate it.
		var err error
		err = task.DeactivateTasks(originalTasks, caller)
		if err != nil {
			return errors.Wrap(err, "error deactivating task")
		}

		if t.Requester == evergreen.MergeTestRequester {
			_, err = commitqueue.RemoveCommitQueueItemForVersion(t.Project, t.Version, caller)
			if err != nil {
				return err
			}
			p, err := patch.FindOneId(t.Version)
			if err != nil {
				return errors.Wrap(err, "unable to find patch")
			}
			if p == nil {
				return errors.New("patch not found")
			}
			err = SendCommitQueueResult(p, message.GithubStateError, fmt.Sprintf("deactivated by '%s'", caller))
			grip.Error(message.WrapError(err, message.Fields{
				"message": "unable to send github status",
				"patch":   t.Version,
			}))
			err = RestartItemsAfterVersion(nil, t.Project, t.Version, caller)
			if err != nil {
				return errors.Wrap(err, "error restarting later commit queue items")
			}
		}
	} else {
		return nil
	}

	if t.IsPartOfDisplay() {
		if err := UpdateDisplayTaskForTask(t); err != nil {
			return errors.Wrap(err, "problem updating display task")
		}
	}

	if err := UpdateBuildAndVersionStatusForTask(t); err != nil {
		return errors.Wrap(err, "problem updating build and version status for task")
	}

	return nil
}

func SetActiveStateById(id, user string, active bool) error {
	t, err := task.FindOneId(id)
	if err != nil {
		return errors.Wrapf(err, "problem finding task '%s'", id)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", id)
	}
	return SetActiveState(t, user, active)
}

// activatePreviousTask will set the Active state for the first task with a
// revision order number less than the current task's revision order number.
// originalStepbackTask is only specified if we're first activating the generator for a generated task.
func activatePreviousTask(taskId, caller string, originalStepbackTask *task.Task) error {
	// find the task first
	t, err := task.FindOneId(taskId)
	if err != nil {
		return errors.WithStack(err)
	}
	if t == nil {
		return errors.Errorf("task '%s' does not exist", taskId)
	}

	// find previous task limiting to just the last one
	prevTask, err := task.FindOne(task.ByBeforeRevision(t.RevisionOrderNumber, t.BuildVariant, t.DisplayName, t.Project, t.Requester))
	if err != nil {
		return errors.Wrap(err, "Error finding previous task")
	}

	// for generated tasks, try to activate the generator instead if the previous task we found isn't the actual last task
	if t.GeneratedBy != "" && prevTask != nil && prevTask.RevisionOrderNumber+1 != t.RevisionOrderNumber {
		return activatePreviousTask(t.GeneratedBy, caller, t)
	}

	// if this is the first time we're running the task, or it's finished, has a negative priority, or already activated
	if prevTask == nil || prevTask.IsFinished() || prevTask.Priority < 0 || prevTask.Activated {
		return nil
	}

	// activate the task
	if err = SetActiveState(prevTask, caller, true); err != nil {
		return errors.Wrapf(err, "error setting task '%s' active", prevTask.Id)
	}
	// add the task that we're actually stepping back so that we know to activate it
	if prevTask.GenerateTask && originalStepbackTask != nil {
		return prevTask.SetGeneratedTasksToActivate(originalStepbackTask.BuildVariant, originalStepbackTask.DisplayName)
	}
	return nil
}

func resetManyTasks(tasks []task.Task, caller string, logIDs bool) error {
	catcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		catcher.Add(resetTask(t.Id, caller, logIDs))
	}
	return catcher.Resolve()
}

// reset task finds a task, attempts to archive it, and resets the task and resets the TaskCache in the build as well.
func resetTask(taskId, caller string, logIDs bool) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	if t.IsPartOfDisplay() {
		return errors.Errorf("cannot restart execution task %s because it is part of a display task", t.Id)
	}
	if err = t.Archive(); err != nil {
		return errors.Wrap(err, "can't restart task because it can't be archived")
	}

	if err = MarkOneTaskReset(t, logIDs); err != nil {
		return errors.WithStack(err)
	}
	event.LogTaskRestarted(t.Id, t.Execution, caller)

	if err = t.ActivateTask(caller); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(UpdateBuildAndVersionStatusForTask(t))
}

// TryResetTask resets a task
func TryResetTask(taskId, user, origin string, detail *apimodels.TaskEndDetail) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	if t == nil {
		return fmt.Errorf("cannot restart task %s because it could not be found", taskId)
	}
	if t.IsPartOfDisplay() {
		return fmt.Errorf("cannot restart execution task %s because it is part of a display task", t.Id)
	}

	var execTask *task.Task

	// if we've reached the max number of executions for this task, mark it as finished and failed
	if t.Execution >= evergreen.MaxTaskExecution {
		// restarting from the UI bypasses the restart cap
		msg := fmt.Sprintf("Task '%v' reached max execution (%v):", t.Id, evergreen.MaxTaskExecution)
		if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
			grip.Debugln(msg, "allowing exception for", user)
		} else if !t.IsFinished() {
			if detail != nil {
				grip.Debugln(msg, "marking as failed")
				if t.DisplayOnly {
					for _, etId := range t.ExecutionTasks {
						execTask, err = task.FindOneId(etId)
						if err != nil {
							return errors.Wrap(err, "error finding execution task")
						}
						if err = MarkEnd(execTask, origin, time.Now(), detail, false); err != nil {
							return errors.Wrap(err, "error marking execution task as ended")
						}
					}
				}
				return errors.WithStack(MarkEnd(t, origin, time.Now(), detail, false))
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
			return errors.Errorf("Task '%v' is currently '%v' - cannot reset task in this status",
				t.Id, t.Status)
		}
	}

	if detail != nil {
		if err = t.MarkEnd(time.Now(), detail); err != nil {
			return errors.Wrap(err, "Error marking task as ended")
		}
	}

	caller := origin
	if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
		caller = user
	}
	if t.IsPartOfSingleHostTaskGroup() {
		if err = t.SetResetWhenFinished(); err != nil {
			return errors.Wrap(err, "can't mark task group for reset")
		}
		return errors.Wrap(checkResetSingleHostTaskGroup(t, caller), "can't reset single host task group")
	}

	return errors.WithStack(resetTask(t.Id, caller, false))
}

func AbortTask(taskId, caller string) error {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return err
	}
	if t == nil {
		return errors.Errorf("task '%s' doesn't exist", taskId)
	}
	if t.DisplayOnly {
		for _, et := range t.ExecutionTasks {
			_ = AbortTask(et, caller) // discard errors because some execution tasks may not be abortable
		}
	}

	if !t.IsAbortable() {
		return errors.Errorf("Task '%v' is currently '%v' - cannot abort task"+
			" in this status", t.Id, t.Status)
	}

	// set the active state and then set the abort
	if err = SetActiveState(t, caller, false); err != nil {
		return err
	}
	event.LogTaskAbortRequest(t.Id, t.Execution, caller)
	return t.SetAborted(task.AbortInfo{User: caller})
}

// Deactivate any previously activated but undispatched
// tasks for the same build variant + display name + project combination
// as the task.
func DeactivatePreviousTasks(t *task.Task, caller string) error {
	statuses := []string{evergreen.TaskUndispatched}
	allTasks, err := task.FindAll(task.ByActivatedBeforeRevisionWithStatuses(
		t.RevisionOrderNumber,
		statuses,
		t.BuildVariant,
		t.DisplayName,
		t.Project,
	))
	if err != nil {
		return errors.Wrapf(err, "error finding tasks to deactivate for task %s", t.Id)
	}
	extraTasks := []task.Task{}
	if t.DisplayOnly {
		for _, dt := range allTasks {
			var execTasks []task.Task
			execTasks, err = task.Find(task.ByIds(dt.ExecutionTasks))
			if err != nil {
				return errors.Wrapf(err, "error finding execution tasks to deactivate for task %s", t.Id)
			}
			canDeactivate := true
			for _, et := range execTasks {
				if et.IsFinished() || et.IsAbortable() {
					canDeactivate = false
					break
				}
			}
			if canDeactivate {
				extraTasks = append(extraTasks, execTasks...)
			}
		}
	}
	allTasks = append(allTasks, extraTasks...)

	for _, t := range allTasks {
		if evergreen.IsPatchRequester(t.Requester) {
			// EVG-948, the query depends on patches not
			// having the revision order number, which they
			// got as part of 948. as we expect to add more
			// requesters in the future, we're doing this
			// filtering here rather than in the query.
			continue
		}

		if err = SetActiveState(&t, caller, false); err != nil {
			return err
		}
	}

	return nil
}

// Returns true if the task should stepback upon failure, and false
// otherwise. Note that the setting is obtained from the top-level
// project, if not explicitly set on the task.
func getStepback(taskId string) (bool, error) {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return false, errors.Wrapf(err, "problem finding task %s", taskId)
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
func doStepback(t *task.Task) error {
	if t.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
		if err != nil {
			return errors.Wrapf(err, "error finding tasks for stepback of %s", t.Id)
		}
		catcher := grip.NewSimpleCatcher()
		for _, et := range execTasks {
			catcher.Add(doStepback(&et))
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
		return errors.Wrap(err, "error locating previous successful task")
	}
	if prevTask == nil {
		return nil
	}

	// activate the previous task to pinpoint regression
	return errors.WithStack(activatePreviousTask(t.Id, evergreen.StepbackTaskActivator, nil))
}

// MarkEnd updates the task as being finished, performs a stepback if necessary, and updates the build status
func MarkEnd(t *task.Task, caller string, finishTime time.Time, detail *apimodels.TaskEndDetail,
	deactivatePrevious bool) error {
	const slowThreshold = time.Second

	detailsCopy := *detail
	hasFailedTests, err := t.HasFailedTests()
	if err != nil {
		return errors.Wrap(err, "checking for failed tests")
	}
	if hasFailedTests {
		detailsCopy.Status = evergreen.TaskFailed
	}

	if t.Status == detailsCopy.Status {
		grip.Warningf("Tried to mark task %s as finished twice", t.Id)
		return nil
	}
	if !t.HasCedarResults { // Results not in cedar, check the db.
		count, err := testresult.Count(testresult.FilterByTaskIDAndExecution(t.Id, t.Execution))
		if err != nil {
			return errors.Wrap(err, "unable to count test results")
		}
		t.HasLegacyResults = utility.ToBoolPtr(count > 0) // cache if we even need to look this up in the future

		if detailsCopy.Status == evergreen.TaskSucceeded && count == 0 && t.MustHaveResults {
			detailsCopy.Status = evergreen.TaskFailed
			detailsCopy.Description = evergreen.TaskDescriptionNoResults
			detailsCopy.Type = evergreen.CommandTypeTest
		}
	}

	t.Details = detailsCopy
	if utility.IsZeroTime(t.StartTime) {
		grip.Warning(message.Fields{
			"message":      "Task is missing start time",
			"task_id":      t.Id,
			"execution":    t.Execution,
			"requester":    t.Requester,
			"activated_by": t.ActivatedBy,
		})
	}
	startPhaseAt := time.Now()
	err = t.MarkEnd(finishTime, &detailsCopy)
	grip.NoticeWhen(time.Since(startPhaseAt) > slowThreshold, message.Fields{
		"message":       "slow operation",
		"function":      "MarkEnd",
		"step":          "t.MarkEnd",
		"task":          t.Id,
		"duration_secs": time.Since(startPhaseAt).Seconds(),
	})

	if err != nil {
		return errors.Wrap(err, "could not mark task finished")
	}

	if err = UpdateUnblockedDependencies(t, true, "MarkEnd"); err != nil {
		return errors.Wrap(err, "could not update unblocked dependencies")
	}

	if err = UpdateBlockedDependencies(t); err != nil {
		return errors.Wrap(err, "could not update blocked dependencies")
	}

	status := t.ResultStatus()
	event.LogTaskFinished(t.Id, t.Execution, t.HostId, status)

	if t.IsPartOfDisplay() {
		if err = UpdateDisplayTaskForTask(t); err != nil {
			return errors.Wrap(err, "problem updating display task")
		}
		dt, err := t.GetDisplayTask()
		if err != nil {
			return errors.Wrap(err, "error getting display task")
		}
		if err = checkResetDisplayTask(dt); err != nil {
			return errors.Wrap(err, "can't check display task reset")
		}
	} else {
		if t.IsPartOfSingleHostTaskGroup() {
			if err = checkResetSingleHostTaskGroup(t, caller); err != nil {
				return errors.Wrap(err, "problem resetting task group")
			}
		}
	}

	// activate/deactivate other task if this is not a patch request's task
	if !evergreen.IsPatchRequester(t.Requester) {
		if t.IsPartOfDisplay() {
			_, err = t.GetDisplayTask()
			if err != nil {
				return errors.Wrap(err, "error getting display task")
			}
			err = evalStepback(t.DisplayTask, caller, t.DisplayTask.Status, deactivatePrevious)
		} else {
			err = evalStepback(t, caller, status, deactivatePrevious)
		}
		if err != nil {
			return err
		}
	}

	if err := UpdateBuildAndVersionStatusForTask(t); err != nil {
		return errors.Wrap(err, "updating build/version status")
	}

	if t.ResetWhenFinished && !t.IsPartOfDisplay() && !t.IsPartOfSingleHostTaskGroup() {
		return TryResetTask(t.Id, evergreen.APIServerTaskActivator, "", detail)
	}

	return nil
}

// UpdateBlockedDependencies traverses the dependency graph and recursively sets each
// parent dependency as unattainable in depending tasks.
func UpdateBlockedDependencies(t *task.Task) error {
	dependentTasks, err := t.FindAllUnmarkedBlockedDependencies()
	if err != nil {
		return errors.Wrapf(err, "can't get tasks depending on task '%s'", t.Id)
	}

	if t.IsPartOfSingleHostTaskGroup() && len(dependentTasks) > 0 {
		grip.Info(message.Fields{
			"message": "blocked task group dependent tasks",
			"ticket":  "EVG-12923",
			"task":    t.Id,
			"stack":   message.NewStack(2, "").Raw().(message.StackTrace).Frames,
		})
	}

	for _, dependentTask := range dependentTasks {
		if err = dependentTask.MarkUnattainableDependency(t.Id, true); err != nil {
			return errors.Wrap(err, "error marking dependency unattainable")
		}
		if err = UpdateBlockedDependencies(&dependentTask); err != nil {
			return errors.Wrapf(err, "error updating blocked dependencies for '%s'", t.Id)
		}
	}
	return nil
}

func UpdateUnblockedDependencies(t *task.Task, logIDs bool, caller string) error {
	blockedTasks, err := t.FindAllMarkedUnattainableDependencies()
	if err != nil {
		return errors.Wrap(err, "can't get dependencies marked unattainable")
	}

	if logIDs && t.IsPartOfSingleHostTaskGroup() && len(blockedTasks) > 0 {
		grip.Info(message.Fields{
			"message": "unblocked task group dependent tasks",
			"ticket":  "EVG-12923",
			"task":    t.Id,
			"caller":  caller,
		})
	}

	for _, blockedTask := range blockedTasks {
		if err = blockedTask.MarkUnattainableDependency(t.Id, false); err != nil {
			return errors.Wrap(err, "error marking dependency attainable")
		}
		if err = UpdateUnblockedDependencies(&blockedTask, logIDs, caller); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func DequeueAndRestart(t *task.Task, caller, reason string) error {
	cq, err := commitqueue.FindOneId(t.Project)
	if err != nil {
		return errors.Wrapf(err, "can't get commit queue for id '%s'", t.Project)
	}
	if cq == nil {
		return errors.Errorf("no commit queue found for '%s'", t.Project)
	}

	// this must be done before dequeuing so that we know which entries to restart
	if err = RestartItemsAfterVersion(cq, t.Project, t.Version, caller); err != nil {
		return errors.Wrap(err, "unable to restart versions")
	}

	if err = tryDequeueAndAbortCommitQueueVersion(t, *cq, caller); err != nil {
		return err
	}

	p, err := patch.FindOneId(t.Version)
	if err != nil {
		return errors.Wrap(err, "unable to find patch")
	}
	if p == nil {
		return errors.New("patch not found")
	}
	err = SendCommitQueueResult(p, message.GithubStateFailure, reason)
	grip.Error(message.WrapError(err, message.Fields{
		"message": "unable to send github status",
		"patch":   t.Version,
	}))

	return nil
}

func RestartItemsAfterVersion(cq *commitqueue.CommitQueue, project, version, caller string) error {
	if cq == nil {
		var err error
		cq, err = commitqueue.FindOneId(project)
		if err != nil {
			return errors.Wrapf(err, "can't get commit queue for id '%s'", project)
		}
		if cq == nil {
			return errors.Errorf("no commit queue found for '%s'", project)
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
			catcher.Add(RestartTasksInVersion(item.Version, true, caller))
		}
	}

	return catcher.Resolve()
}

func tryDequeueAndAbortCommitQueueVersion(t *task.Task, cq commitqueue.CommitQueue, caller string) error {
	p, err := patch.FindOne(patch.ByVersion(t.Version))
	if err != nil {
		return errors.Wrapf(err, "error finding patch")
	}
	if p == nil {
		return errors.Errorf("No patch for task")
	}

	if !p.IsCommitQueuePatch() {
		return nil
	}

	issue := p.Id.Hex()
	err = removeNextMergeTaskDependency(cq, issue)
	grip.Error(message.WrapError(err, message.Fields{
		"message": "error removing dependency",
		"patch":   issue,
	}))

	removed, err := cq.RemoveItemAndPreventMerge(issue, true, caller)
	grip.Debug(message.Fields{
		"message": "removing commit queue item",
		"issue":   issue,
		"err":     err,
		"removed": removed,
		"caller":  caller,
	})
	if err != nil {
		return errors.Wrapf(err, "can't remove and prevent merge for item '%s' from queue '%s'", t.Version, t.Project)
	}
	if removed == nil {
		return errors.Errorf("no commit queue entry removed for '%s'", issue)
	}

	if p.IsPRMergePatch() {
		err = SendCommitQueueResult(p, message.GithubStateFailure, "merge test failed")
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error sending github status",
			"patch":   p.Id.Hex(),
		}))
	}

	event.LogCommitQueueConcludeTest(p.Id.Hex(), evergreen.MergeTestFailed)
	return errors.Wrapf(CancelPatch(p, task.AbortInfo{TaskID: t.Id, User: caller}), "Error aborting failed commit queue patch")
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
		return errors.Wrap(err, "unable to find next merge task")
	}
	if nextMerge == nil {
		return errors.New("no merge task found")
	}
	currentMerge, err := task.FindMergeTaskForVersion(cq.Queue[currentIndex].Version)
	if err != nil {
		return errors.Wrap(err, "unable to find current merge task")
	}
	if err = nextMerge.RemoveDependency(currentMerge.Id); err != nil {
		return errors.Wrap(err, "unable to remove dependency")
	}

	if currentIndex > 0 {
		prevItem := cq.Queue[currentIndex-1]
		prevMerge, err := task.FindMergeTaskForVersion(prevItem.Version)
		if err != nil {
			return errors.Wrap(err, "unable to find previous merge task")
		}
		if prevMerge == nil {
			return errors.New("no merge task found")
		}
		d := task.Dependency{
			TaskId: prevMerge.Id,
			Status: AllStatuses,
		}
		if err = nextMerge.AddDependency(d); err != nil {
			return errors.Wrap(err, "unable to add dependency")
		}
	}

	return nil
}

func evalStepback(t *task.Task, caller, status string, deactivatePrevious bool) error {
	if status == evergreen.TaskFailed {
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
				return errors.Wrapf(err, "can't get task group for task '%s'", t.Id)
			}
			if len(tasks) == 0 {
				return errors.Errorf("no tasks in task group '%s' for task '%s'", t.TaskGroup, t.Id)
			}
			for _, tgTask := range tasks {
				catcher.Wrapf(doStepback(&tgTask), "error stepping back task group task '%s'", tgTask.DisplayName)
				if tgTask.Id == t.Id {
					break // don't need to stepback later tasks in the group
				}
			}

			return catcher.Resolve()
		}
		return errors.Wrap(doStepback(t), "error during stepback")

	} else if deactivatePrevious && status == evergreen.TaskSucceeded {
		// if the task was successful, ignore running previous
		// activated tasks for this buildvariant

		if err := DeactivatePreviousTasks(t, caller); err != nil {
			return errors.Wrap(err, "Error deactivating previous task")
		}
	}

	return nil
}

// updateMakespans
func updateMakespans(b *build.Build, buildTasks []task.Task) error {
	depPath := FindPredictedMakespan(buildTasks)
	return errors.WithStack(b.UpdateMakespans(depPath.TotalTime, CalculateActualMakespan(buildTasks)))
}

func getBuildStatus(buildTasks []task.Task) string {
	// not started
	noStartedTasks := true
	for _, t := range buildTasks {
		if !evergreen.IsUnstartedTaskStatus(t.Status) {
			noStartedTasks = false
			break
		}
	}
	if noStartedTasks {
		return evergreen.BuildCreated
	}

	// started but not finished
	for _, t := range buildTasks {
		if t.Status == evergreen.TaskStarted {
			return evergreen.BuildStarted
		}
		if t.Activated && !t.Blocked() && !t.IsFinished() {
			return evergreen.BuildStarted
		}
	}

	// finished but failed
	for _, t := range buildTasks {
		if evergreen.IsFailedTaskStatus(t.Status) {
			return evergreen.BuildFailed
		}
	}

	return evergreen.BuildSucceeded
}

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

	githubBuildStatus := getBuildStatus(githubStatusTasks)

	if githubBuildStatus == b.GithubCheckStatus {
		return nil
	}

	if evergreen.IsFinishedBuildStatus(githubBuildStatus) {
		event.LogBuildGithubCheckFinishedEvent(b.Id, githubBuildStatus)
	}

	return b.UpdateGithubCheckStatus(githubBuildStatus)
}

// UpdateBuildStatus updates the status of the build based on its tasks' statuses.
// Returns true if the build's status has changed.
func UpdateBuildStatus(b *build.Build) (bool, error) {
	buildTasks, err := task.Find(task.ByBuildId(b.Id).WithFields(task.StatusKey, task.ActivatedKey, task.DependsOnKey, task.IsGithubCheckKey))
	if err != nil {
		return false, errors.Wrapf(err, "getting tasks in build '%s'", b.Id)
	}

	buildStatus := getBuildStatus(buildTasks)

	if buildStatus == b.Status {
		return false, nil
	}

	event.LogBuildStateChangeEvent(b.Id, buildStatus)

	if evergreen.IsFinishedBuildStatus(buildStatus) {
		if err = b.MarkFinished(buildStatus, time.Now()); err != nil {
			return true, errors.Wrapf(err, "marking build as finished with status '%s'", buildStatus)
		}
		if err = updateMakespans(b, buildTasks); err != nil {
			return true, errors.Wrapf(err, "updating makespan information for '%s'", b.Id)
		}
	} else {
		if err = b.UpdateStatus(buildStatus); err != nil {
			return true, errors.Wrap(err, "updating build status")
		}
	}

	if err = updateBuildGithubStatus(b, buildTasks); err != nil {
		return true, errors.Wrap(err, "updating build github status")
	}

	return true, nil
}

func getVersionStatus(builds []build.Build) string {
	// not started
	noStartedBuilds := true
	for _, b := range builds {
		if b.Status != evergreen.BuildCreated {
			noStartedBuilds = false
			break
		}
	}
	if noStartedBuilds {
		return evergreen.VersionCreated
	}

	// started but not finished
	for _, b := range builds {
		if b.Activated && !evergreen.IsFinishedBuildStatus(b.Status) {
			return evergreen.VersionStarted
		}
	}

	// finished but failed
	for _, b := range builds {
		if b.Status == evergreen.BuildFailed {
			return evergreen.VersionFailed
		}
	}

	return evergreen.VersionSucceeded
}

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

	githubBuildStatus := getVersionStatus(githubStatusBuilds)

	if evergreen.IsFinishedBuildStatus(githubBuildStatus) {
		event.LogVersionGithubCheckFinishedEvent(v.Id, githubBuildStatus)
	}

	return nil
}

// Update the status of the version based on its constituent builds
func UpdateVersionStatus(v *Version) (string, error) {
	builds, err := build.Find(build.ByVersion(v.Id).WithFields(build.ActivatedKey, build.StatusKey, build.IsGithubCheckKey))
	if err != nil {
		return "", errors.Wrapf(err, "getting builds for version '%s'", v.Id)
	}
	versionStatus := getVersionStatus(builds)

	if versionStatus == v.Status {
		return versionStatus, nil
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

	if err = updateVersionGithubStatus(v, builds); err != nil {
		return "", errors.Wrap(err, "updating version github status")
	}

	return versionStatus, nil
}

func UpdatePatchStatus(p *patch.Patch, versionStatus string) error {
	patchStatus, err := evergreen.VersionStatusToPatchStatus(versionStatus)
	if err != nil {
		return errors.Wrapf(err, "getting patch status from version status '%s'", versionStatus)
	}

	if patchStatus == p.Status {
		return nil
	}

	event.LogPatchStateChangeEvent(p.Version, patchStatus)
	if evergreen.IsFinishedPatchStatus(patchStatus) {
		if err = p.MarkFinished(patchStatus, time.Now()); err != nil {
			return errors.Wrapf(err, "marking patch '%s' as finished with status '%s'", p.Id.Hex(), patchStatus)
		}
	} else {
		if err = p.UpdateStatus(patchStatus); err != nil {
			return errors.Wrapf(err, "updating patch '%s' with status '%s'", p.Id.Hex(), patchStatus)
		}
	}

	return nil
}

// UpdateBuildAndVersionStatusForTask updates the status of the task's build based on all the tasks in the build
// and the task's version based on all the builds in the version.
// Also update build and version Github statuses based on the subset of tasks and builds included in github checks
func UpdateBuildAndVersionStatusForTask(t *task.Task) error {
	taskBuild, err := build.FindOneId(t.BuildId)
	if err != nil {
		return errors.Wrapf(err, "getting build for task '%s'", t.Id)
	}
	if taskBuild == nil {
		return errors.Errorf("no build '%s' found for task '%s'", t.BuildId, t.Id)
	}
	buildStatusChanged, err := UpdateBuildStatus(taskBuild)
	if err != nil {
		return errors.Wrapf(err, "updating build '%s' status", taskBuild.Id)
	}
	// If no build has changed status, then we can assume the version and patch statuses have also stayed the same.
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
	newVersionStatus, err := UpdateVersionStatus(taskVersion)
	if err != nil {
		return errors.Wrapf(err, "updating version '%s' status", taskVersion.Id)
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
		return errors.Wrap(err, "error marking build started")
	}

	// ensure the appropriate version is marked as started if necessary
	if err = TryMarkVersionStarted(t.Version, startTime); err != nil {
		return errors.Wrap(err, "error marking version started")
	}

	// if it's a patch, mark the patch as started if necessary
	if evergreen.IsPatchRequester(t.Requester) {
		err := patch.TryMarkStarted(t.Version, startTime)
		if err == nil {
			updates.PatchNewStatus = evergreen.PatchStarted

		} else if !adb.ResultsNotFound(err) {
			return errors.WithStack(err)
		}
	}

	if t.IsPartOfDisplay() {
		return UpdateDisplayTaskForTask(t)
	}

	return nil
}

func MarkTaskUndispatched(t *task.Task) error {
	// record that the task as undispatched on the host
	if err := t.MarkAsUndispatched(); err != nil {
		return errors.WithStack(err)
	}
	// the task was successfully dispatched, log the event
	event.LogTaskUndispatched(t.Id, t.Execution, t.HostId)

	if t.IsPartOfDisplay() {
		return UpdateDisplayTaskForTask(t)
	}

	return nil
}

func MarkTaskDispatched(t *task.Task, h *host.Host) error {
	// record that the task was dispatched on the host
	if err := t.MarkAsDispatched(h.Id, h.Distro.Id, h.AgentRevision, time.Now()); err != nil {
		return errors.Wrapf(err, "error marking task %s as dispatched "+
			"on host %s", t.Id, h.Id)
	}
	// the task was successfully dispatched, log the event
	event.LogTaskDispatched(t.Id, t.Execution, h.Id)

	if t.IsPartOfDisplay() {
		return UpdateDisplayTaskForTask(t)
	}

	return nil
}

func MarkOneTaskReset(t *task.Task, logIDs bool) error {
	if t.DisplayOnly {
		for _, et := range t.ExecutionTasks {
			execTask, err := task.FindOneId(et)
			if err != nil {
				return errors.Wrap(err, "error retrieving execution task")
			}
			if err = MarkOneTaskReset(execTask, logIDs); err != nil {
				return errors.Wrap(err, "error resetting execution task")
			}
		}
	}

	if err := t.Reset(); err != nil {
		return errors.Wrap(err, "error resetting task in database")
	}

	return errors.Wrap(UpdateUnblockedDependencies(t, logIDs, "MarkOneTaskReset"), "can't clear cached unattainable dependencies")
}

func MarkTasksReset(taskIds []string) error {
	tasks, err := task.FindAll(task.ByIds(taskIds))
	if err != nil {
		return errors.WithStack(err)
	}
	tasks, err = task.AddParentDisplayTasks(tasks)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, t := range tasks {
		if t.DisplayOnly {
			taskIds = append(taskIds, t.Id)
		}
	}

	if err = task.ResetTasks(taskIds); err != nil {
		return errors.Wrap(err, "error resetting tasks in database")
	}

	catcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		catcher.Add(errors.Wrap(UpdateUnblockedDependencies(&t, false, ""), "can't clear cached unattainable dependencies"))
	}
	return catcher.Resolve()
}

// RestartFailedTasks attempts to restart failed tasks that started between 2 times
// It returns a slice of task IDs that were successfully restarted as well as a slice
// of task IDs that failed to restart
// opts.dryRun will return the tasks that will be restarted if sent true
// opts.red and opts.purple will only restart tasks that were failed due to the test
// or due to the system, respectively
func RestartFailedTasks(opts RestartOptions) (RestartResults, error) {
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
	tasksToRestart, err := task.FindAll(task.ByTimeStartedAndFailed(opts.StartTime, opts.EndTime, failureTypes))
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
				return results, errors.Wrapf(err, "error getting display task")
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
				return results, errors.Wrapf(err, "error marking display task '%s' for reset", id)
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

	return doRestartFailedTasks(idsToRestart, opts.User, results), nil
}

func doRestartFailedTasks(tasks []string, user string, results RestartResults) RestartResults {
	var tasksErrored []string

	for _, id := range tasks {
		if err := TryResetTask(id, user, evergreen.RESTV2Package, nil); err != nil {
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

func ClearAndResetStrandedTask(h *host.Host) error {
	if h.RunningTask == "" {
		return nil
	}

	t, err := task.FindOneId(h.RunningTask)
	if err != nil {
		return errors.Wrapf(err, "database error clearing task '%s' from host '%s'",
			t.Id, h.Id)
	} else if t == nil {
		return nil
	}

	if err = h.ClearRunningTask(); err != nil {
		return errors.Wrapf(err, "problem clearing running task from host '%s'", h.Id)
	}

	if t.IsFinished() {
		return nil
	}

	if err = t.MarkSystemFailed(evergreen.TaskDescriptionStranded); err != nil {
		return errors.Wrap(err, "problem marking task failed")
	}

	// For a single-host task group, block and dequeue later tasks in that group.
	if t.IsPartOfSingleHostTaskGroup() {
		if err = BlockTaskGroupTasks(t.Id); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem blocking task group tasks",
				"task_id": t.Id,
			}))
			return errors.Wrapf(err, "problem blocking task group tasks")
		}
		grip.Debug(message.Fields{
			"message": "blocked task group tasks for task",
			"task_id": t.Id,
		})
	}

	if time.Since(t.ActivatedTime) > task.UnschedulableThreshold {
		if t.DisplayOnly {
			for _, etID := range t.ExecutionTasks {
				var execTask *task.Task
				execTask, err = task.FindOneId(etID)
				if err != nil {
					return errors.Wrap(err, "error finding execution task")
				}
				if execTask == nil {
					return errors.New("execution task not found")
				}
				if err = MarkEnd(execTask, evergreen.MonitorPackage, time.Now(), &t.Details, false); err != nil {
					return errors.Wrap(err, "error marking execution task as ended")
				}
			}
		}
		return errors.WithStack(MarkEnd(t, evergreen.MonitorPackage, time.Now(), &t.Details, false))
	}

	if t.IsPartOfDisplay() {
		dt, err := t.GetDisplayTask()
		if err != nil {
			return errors.Wrap(err, "error getting display task")
		}
		if err = dt.SetResetWhenFinished(); err != nil {
			return errors.Wrap(err, "can't mark display task for reset")
		}
		return errors.Wrap(checkResetDisplayTask(dt), "can't check display task reset")
	}

	return errors.Wrap(TryResetTask(t.Id, evergreen.User, evergreen.MonitorPackage, &t.Details), "problem resetting task")
}

// UpdateDisplayTaskForTask updates the status of the given execution task's display task
func UpdateDisplayTaskForTask(t *task.Task) error {
	if !t.IsPartOfDisplay() {
		return errors.Errorf("%s is not an execution task", t.Id)
	}
	dt, err := t.GetDisplayTask()
	if err != nil {
		return errors.Wrapf(err, "error getting display task for task")
	}
	if !dt.DisplayOnly {
		return errors.Errorf("%s is not a display task", dt.Id)
	}

	var timeTaken time.Duration
	var statusTask task.Task
	execTasks, err := task.Find(task.ByIds(dt.ExecutionTasks))
	if err != nil {
		return errors.Wrap(err, "error retrieving execution tasks")
	}
	hasFinishedTasks := false
	hasDispatchableTasks := false
	startTime := time.Unix(1<<62, 0)
	endTime := utility.ZeroTime
	for _, execTask := range execTasks {
		// if any of the execution tasks are scheduled, the display task is too
		if execTask.Activated {
			dt.Activated = true
			if utility.IsZeroTime(dt.ActivatedTime) {
				dt.ActivatedTime = time.Now()
			}
		}
		if execTask.IsFinished() {
			hasFinishedTasks = true
		}
		if execTask.IsDispatchable() && !execTask.Blocked() {
			hasDispatchableTasks = true
		}

		// add up the duration of the execution tasks as the cumulative time taken
		timeTaken += execTask.TimeTaken

		// set the start/end time of the display task as the earliest/latest task
		if execTask.StartTime.Before(startTime) {
			startTime = execTask.StartTime
		}
		if execTask.FinishTime.After(endTime) {
			endTime = execTask.FinishTime
		}
	}

	sort.Sort(task.ByPriority(execTasks))
	statusTask = execTasks[0]
	if hasFinishedTasks && hasDispatchableTasks {
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
	if endTime != utility.ZeroTime && !hasDispatchableTasks {
		update[task.FinishTimeKey] = endTime
	}

	// refresh task status from db in case of race
	taskWithStatus, err := task.FindOne(task.ById(dt.Id).WithFields(task.StatusKey))
	if err != nil {
		return errors.Wrap(err, "error refreshing task status from db")
	}
	if taskWithStatus == nil {
		return errors.New("task not found")
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
		return errors.Wrap(err, "error updating display task")
	}
	dt.Status = statusTask.Status
	dt.Details = statusTask.Details
	dt.TimeTaken = timeTaken
	if !wasFinished && dt.IsFinished() {
		event.LogTaskFinished(dt.Id, dt.Execution, "", dt.ResultStatus())
	}
	return nil
}

func checkResetSingleHostTaskGroup(t *task.Task, caller string) error {
	if !t.IsPartOfSingleHostTaskGroup() {
		return nil
	}
	tasks, err := task.FindTaskGroupFromBuild(t.BuildId, t.TaskGroup)
	if err != nil {
		return errors.Wrapf(err, "can't get task group for task '%s'", t.Id)
	}
	if len(tasks) == 0 {
		return errors.Errorf("no tasks in task group '%s' for task '%s'", t.TaskGroup, t.Id)
	}
	shouldReset := false
	for _, tgTask := range tasks {
		if tgTask.ResetWhenFinished {
			shouldReset = true
		}
		if !tgTask.IsFinished() && !tgTask.Blocked() && tgTask.Activated { // task in group still needs to  run
			return nil
		}
	}

	if !shouldReset { // no task in task group has requested a reset
		return nil
	}

	if err = resetManyTasks(tasks, caller, true); err != nil {
		return errors.Wrap(err, "can't reset task group tasks")
	}

	tasks, err = task.FindTaskGroupFromBuild(t.BuildId, t.TaskGroup)
	if err != nil {
		return errors.Wrapf(err, "can't get task group for task '%s'", t.Id)
	}
	taskSet := map[string]bool{}
	for _, t := range tasks {
		taskSet[t.Id] = true
		for _, dep := range t.DependsOn {
			if taskSet[dep.TaskId] && dep.Unattainable {
				grip.Info(message.Fields{
					"message": "task group task was blocked on an earlier task group task after reset",
					"task":    t.Id,
					"ticket":  "EVG-12923",
				})
			}
		}
	}

	return nil
}

func checkResetDisplayTask(t *task.Task) error {
	if !t.ResetWhenFinished {
		return nil
	}
	execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
	if err != nil {
		return errors.Wrapf(err, "can't get exec tasks for '%s'", t.Id)
	}
	for _, execTask := range execTasks {
		if !execTask.IsFinished() && !execTask.Blocked() && execTask.Activated {
			return nil // all tasks not finished
		}
	}
	details := &t.Details
	if details == nil {
		details = &apimodels.TaskEndDetail{
			Type:   evergreen.CommandTypeSystem,
			Status: evergreen.TaskFailed,
		}
	}
	return errors.Wrap(TryResetTask(t.Id, evergreen.User, evergreen.User, details), "error resetting display task")
}
