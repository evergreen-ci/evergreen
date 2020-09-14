package model

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
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
	var modifiedTasks []task.Task
	if active {
		// if the task is being activated and it doesn't override its dependencies
		// activate the task's dependencies as well
		tasksToActivate := []task.Task{}
		if !t.OverrideDependencies {
			deps, err := task.GetRecursiveDependenciesUp([]task.Task{*t}, nil)
			if err != nil {
				return errors.Wrapf(err, "error getting tasks '%s' depends on", t.Id)
			}
			tasksToActivate = append(tasksToActivate, deps...)
		}

		if t.DispatchTime != utility.ZeroTime && t.Status == evergreen.TaskUndispatched {
			if err := resetTask(t.Id, caller); err != nil {
				return errors.Wrap(err, "error resetting task")
			}
		} else {
			tasksToActivate = append(tasksToActivate, *t)
		}
		var err error
		modifiedTasks, err = task.ActivateTasks(tasksToActivate, time.Now(), caller)
		if err != nil {
			return errors.Wrapf(err, "can't activate tasks")
		}

		if t.DistroId == "" && !t.DisplayOnly {
			grip.Critical(message.Fields{
				"message": "task is missing distro id",
				"task_id": t.Id,
			})
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
		modifiedTasks, err = t.DeactivateTask(caller)
		if err != nil {
			return errors.Wrap(err, "error deactivating task")
		}
	} else {
		return nil
	}

	if t.IsPartOfDisplay() {
		if err := updateDisplayTaskAndCache(t); err != nil {
			return err
		}
	}

	return errors.WithStack(build.SetManyCachedTasksActivated(modifiedTasks, active))
}

// ActivatePreviousTask will set the Active state for the first task with a
// revision order number less than the current task's revision order number.
func ActivatePreviousTask(taskId, caller string) error {
	// find the task first
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}

	// find previous task limiting to just the last one
	prevTask, err := task.FindOne(task.ByBeforeRevision(t.RevisionOrderNumber, t.BuildVariant, t.DisplayName, t.Project, t.Requester))
	if err != nil {
		return errors.Wrap(err, "Error finding previous task")
	}

	// if this is the first time we're running the task, or it's finished, has a negative priority, or already activated
	if prevTask == nil || prevTask.IsFinished() || prevTask.Priority < 0 || prevTask.Activated {
		return nil
	}

	// activate the task
	return errors.WithStack(SetActiveState(prevTask, caller, true))
}

func resetManyTasks(tasks []task.Task, caller string) error {
	catcher := grip.NewBasicCatcher()
	for _, t := range tasks {
		catcher.Add(resetTask(t.Id, caller))
	}
	return catcher.Resolve()
}

// reset task finds a task, attempts to archive it, and resets the task and resets the TaskCache in the build as well.
func resetTask(taskId, caller string) error {
	t, err := task.FindOneNoMerge(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	if t.IsPartOfDisplay() {
		return errors.Errorf("cannot restart execution task %s because it is part of a display task", t.Id)
	}
	if err = t.Archive(); err != nil {
		return errors.Wrap(err, "can't restart task because it can't be archived")
	}

	if err = MarkOneTaskReset(t); err != nil {
		return errors.WithStack(err)
	}
	event.LogTaskRestarted(t.Id, t.Execution, caller)

	var activatedTasks []task.Task
	if activatedTasks, err = t.ActivateTask(caller); err != nil {
		return errors.WithStack(err)
	}

	if err = build.SetManyCachedTasksActivated(activatedTasks, true); err != nil {
		return errors.WithStack(err)
	}

	// update the cached version of the task, in its build document
	if err = build.ResetCachedTask(t.BuildId, t.Id); err != nil {
		return errors.WithStack(err)
	}

	updates := StatusChanges{}
	return errors.WithStack(UpdateBuildAndVersionStatusForTask(t.Id, &updates))
}

// TryResetTask resets a task
func TryResetTask(taskId, user, origin string, detail *apimodels.TaskEndDetail) error {
	t, err := task.FindOneNoMerge(task.ById(taskId))
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
		} else {
			grip.Debugln(msg, "marking as failed")
			if detail != nil {
				updates := StatusChanges{}
				if t.DisplayOnly {
					for _, etId := range t.ExecutionTasks {
						execTask, err = task.FindOne(task.ById(etId))
						if err != nil {
							return errors.Wrap(err, "error finding execution task")
						}
						if err = MarkEnd(execTask, origin, time.Now(), detail, false, &updates); err != nil {
							return errors.Wrap(err, "error marking execution task as ended")
						}
					}
				}
				return errors.WithStack(MarkEnd(t, origin, time.Now(), detail, false, &updates))
			} else {
				panic(fmt.Sprintf("TryResetTask called with nil TaskEndDetail by %s", origin))
			}
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
					execTask, err = task.FindOne(task.ById(et))
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

	return errors.WithStack(resetTask(t.Id, caller))
}

func AbortTask(taskId, caller string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return err
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
	allTasks, err := task.FindWithDisplayTasks(task.ByActivatedBeforeRevisionWithStatuses(
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
			execTasks, err = task.FindWithDisplayTasks(task.ByIds(dt.ExecutionTasks))
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
	t, err := task.FindOne(task.ById(taskId))
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
	if t.GeneratedBy != "" {
		generator, err := task.FindOneId(t.GeneratedBy)
		if err != nil {
			return errors.Wrap(err, "error getting generated by task")
		}
		return doStepback(generator)
	}
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
	if prevTask == nil {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "Error locating previous successful task")
	}

	// activate the previous task to pinpoint regression
	return errors.WithStack(ActivatePreviousTask(t.Id, evergreen.StepbackTaskActivator))
}

// MarkEnd updates the task as being finished, performs a stepback if necessary, and updates the build status
func MarkEnd(t *task.Task, caller string, finishTime time.Time, detail *apimodels.TaskEndDetail,
	deactivatePrevious bool, updates *StatusChanges) error {
	const slowThreshold = time.Second

	if t.HasFailedTests() {
		detail.Status = evergreen.TaskFailed
	}

	t.Details = *detail

	if t.Status == detail.Status {
		grip.Warningf("Tried to mark task %s as finished twice", t.Id)
		return nil
	}
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
	err := t.MarkEnd(finishTime, detail)
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

	if err = UpdateUnblockedDependencies(t); err != nil {
		return errors.Wrap(err, "could not update unblocked dependencies")
	}

	if err = UpdateBlockedDependencies(t); err != nil {
		return errors.Wrap(err, "could not update blocked dependencies")
	}

	status := t.ResultStatus()
	event.LogTaskFinished(t.Id, t.Execution, t.HostId, status)

	if t.IsPartOfDisplay() {
		if err = UpdateDisplayTask(t.DisplayTask); err != nil {
			return err
		}
		if err = build.UpdateCachedTask(t.DisplayTask, t.TimeTaken); err != nil {
			b, findErr := build.FindOneId(t.BuildId)
			grip.Error(message.WrapError(err, message.Fields{
				"message":     "failed to update cached display task",
				"function":    "MarkEnd",
				"build_id":    t.DisplayTask.BuildId,
				"task_id":     t.DisplayTask.Id,
				"status":      t.DisplayTask.Status,
				"time_taken":  t.TimeTaken,
				"build_cache": b.Tasks,
				"find_err":    findErr,
			}))
		}
		if err = checkResetDisplayTask(t.DisplayTask); err != nil {
			return errors.Wrap(err, "can't check display task reset")
		}
	} else {
		err = build.SetCachedTaskFinished(t.BuildId, t.Id, detail.Status, detail, t.TimeTaken)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "failed to set cached task finished",
				"function":   "MarkEnd",
				"build_id":   t.BuildId,
				"task_id":    t.Id,
				"status":     detail.Status,
				"time_taken": t.TimeTaken,
			}))
			return errors.Wrap(err, "error updating build")
		}
		if t.IsPartOfSingleHostTaskGroup() {
			if err = checkResetSingleHostTaskGroup(t, caller); err != nil {
				return errors.Wrap(err, "problem resetting task group")
			}
		}
	}

	// activate/deactivate other task if this is not a patch request's task
	if !evergreen.IsPatchRequester(t.Requester) {
		if t.IsPartOfDisplay() {
			err = evalStepback(t.DisplayTask, caller, t.DisplayTask.Status, deactivatePrevious)
		} else {
			err = evalStepback(t, caller, status, deactivatePrevious)
		}
		if err != nil {
			return err
		}
	}

	// update the build
	if err := UpdateBuildAndVersionStatusForTask(t.Id, updates); err != nil {
		return errors.Wrap(err, "Error updating build status")
	}

	isBuildCompleteStatus := updates.BuildNewStatus == evergreen.BuildFailed || updates.BuildNewStatus == evergreen.BuildSucceeded
	if len(updates.BuildNewStatus) != 0 {
		if updates.BuildComplete || !isBuildCompleteStatus {
			event.LogBuildStateChangeEvent(t.BuildId, updates.BuildNewStatus)
		}
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

	for _, dependentTask := range dependentTasks {
		if err = dependentTask.MarkUnattainableDependency(t, true); err != nil {
			return errors.Wrap(err, "error marking dependency unattainable")
		}
		if err = UpdateBlockedDependencies(&dependentTask); err != nil {
			return errors.WithStack(err)
		}

		if !dependentTask.IsPartOfDisplay() { // execution tasks not cached in build so we should skip
			if err = build.SetCachedTaskBlocked(dependentTask.BuildId, dependentTask.Id, dependentTask.Blocked()); err != nil {
				return errors.Wrapf(err, "error marking cached task '%s' as blocked", dependentTask.Id)
			}
		}
	}
	return nil
}

func UpdateUnblockedDependencies(t *task.Task) error {
	blockedTasks, err := t.FindAllMarkedUnattainableDependencies()
	if err != nil {
		return errors.Wrap(err, "can't get dependencies marked unattainable")
	}

	for _, blockedTask := range blockedTasks {
		if err = blockedTask.MarkUnattainableDependency(t, false); err != nil {
			return errors.Wrap(err, "error marking dependency attainable")
		}
		if err = UpdateUnblockedDependencies(&blockedTask); err != nil {
			return errors.WithStack(err)
		}

		if !blockedTask.IsPartOfDisplay() { // execution tasks not cached in build so we should skip
			if err = build.SetCachedTaskBlocked(blockedTask.BuildId, blockedTask.Id, blockedTask.Blocked()); err != nil {
				return errors.Wrap(err, "error setting cached task '%s' as unblocked")
			}
		}
	}

	return nil
}

func TryDequeueAndAbortCommitQueueVersion(projectRef *ProjectRef, t *task.Task, caller string) error {
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

	// if task is part of a commit queue, dequeue and abort version
	cq, err := commitqueue.FindOneId(projectRef.Identifier)
	if err != nil {
		return errors.Wrapf(err, "can't get commit queue for id '%s'", projectRef.Identifier)
	}
	if cq == nil {
		return errors.Errorf("no commit queue found for '%s'", projectRef.Identifier)
	}
	issue := p.Id.Hex()
	if p.IsPRMergePatch() {
		issue = strconv.Itoa(p.GithubPatchData.PRNumber)
	}

	removed, err := cq.RemoveItemAndPreventMerge(issue, projectRef.CommitQueue.PatchType, true, caller)
	if err != nil {
		return errors.Wrapf(err, "can't remove and prevent merge for item '%s' from queue '%s'", t.Version, projectRef.Identifier)
	}
	if !removed {
		return nil
	}
	if p.IsPRMergePatch() {
		env := evergreen.GetEnvironment()
		sender, err := env.GetSender(evergreen.SenderGithubStatus)
		if err != nil {
			grip.Debug(message.WrapError(err, message.Fields{
				"message":      "error getting environment",
				"patch_id":     p.Id,
				"project":      p.Project,
				"pull_request": issue,
			}))
		} else {
			var url string
			uiConfig := evergreen.UIConfig{}
			if err := uiConfig.Get(env); err == nil {
				url = fmt.Sprintf("%s/version/%s", uiConfig.Url, t.Version)
			}
			status := message.GithubStatus{
				Context:     commitqueue.GithubContext,
				Description: "merge test failed",
				State:       message.GithubStateFailure,
				Owner:       p.GithubPatchData.BaseOwner,
				Repo:        p.GithubPatchData.BaseRepo,
				Ref:         p.GithubPatchData.HeadHash,
				URL:         url,
			}
			c := message.MakeGithubStatusMessageWithRepo(status)
			sender.Send(c)
		}
	}

	event.LogCommitQueueConcludeTest(p.Id.Hex(), evergreen.MergeTestFailed)
	return errors.Wrapf(CancelPatch(p, task.AbortInfo{TaskID: t.Id, User: caller}), "Error aborting failed commit queue patch")
}

func evalStepback(t *task.Task, caller, status string, deactivatePrevious bool) error {
	if status == evergreen.TaskFailed {
		var shouldStepBack bool
		shouldStepBack, err := getStepback(t.Id)
		if err != nil {
			return errors.WithStack(err)
		}
		if shouldStepBack {
			if err = doStepback(t); err != nil {
				return errors.Wrap(err, "Error during step back")
			}
		}
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
func updateMakespans(b *build.Build) error {
	// find all tasks associated with the build
	tasks, err := task.Find(task.ByBuildId(b.Id))
	if err != nil {
		return errors.WithStack(err)
	}

	depPath := FindPredictedMakespan(tasks)
	return errors.WithStack(b.UpdateMakespans(depPath.TotalTime, CalculateActualMakespan(tasks)))
}

// UpdateBuildAndVersionStatusForTask finds all the builds for a task and updates the
// status of the build and version based on the tasks' statuses.
func UpdateBuildAndVersionStatusForTask(taskId string, updates *StatusChanges) error {
	const slowMS = 100 * time.Millisecond
	// retrieve the task by the task id
	t, err := task.FindOneNoMerge(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", taskId)
	}

	finishTime := time.Now()
	// get all of the tasks in the same build
	b, err := build.FindOne(build.ById(t.BuildId))
	if err != nil {
		return errors.WithStack(err)
	}
	if b == nil {
		return errors.Errorf("build '%s' not found", b.Id)
	}

	buildTasks, err := task.Find(task.ByBuildId(b.Id))
	if err != nil {
		return errors.WithStack(err)
	}

	failedTask := false
	finishedTasks := 0
	blockedTasks := 0

	cache := task.NewDisplayTaskCache()
	// update the build's status based on tasks for this build
	for _, t := range buildTasks {
		if !t.IsFinished() {
			if t.Blocked() {
				blockedTasks++
			}
			continue
		}
		finishedTasks++

		var displayTask *task.Task
		displayTask, err = cache.Get(&t)
		if err != nil {
			return errors.WithStack(err)
		}
		if displayTask != nil {
			t = *displayTask
		}

		// update the build's status when a test task isn't successful
		if evergreen.IsFailedTaskStatus(t.Status) {
			err = b.UpdateStatus(evergreen.BuildFailed)
			if err != nil {
				err = errors.Wrap(err, "Error updating build status")
				grip.Error(err)
				return err
			}

			failedTask = true
		}
	}

	cachedTasks := buildTasks

	// update the display task for the given task
	var displayTask *task.Task
	displayTask, err = cache.Get(t)
	if err != nil {
		return errors.WithStack(err)
	}
	if displayTask != nil {
		if err = UpdateDisplayTask(displayTask); err != nil {
			return errors.Wrap(errors.WithStack(err), "error updating display task")
		}
		cachedTasks = append(cachedTasks, *displayTask)
	}

	if err = b.UpdateCachedTasks(cachedTasks); err != nil {
		return err
	}

	if b.Status == evergreen.BuildCreated {
		if err = b.UpdateStatus(evergreen.BuildStarted); err != nil {
			err = errors.Wrap(err, "Error updating build status")
			grip.Error(err)
			return err
		}
		updates.BuildNewStatus = evergreen.BuildStarted
	}

	if (finishedTasks + blockedTasks) >= len(buildTasks) {
		updates.BuildComplete = true
		updates.BuildNewStatus = evergreen.BuildSucceeded
		if failedTask {
			updates.BuildNewStatus = evergreen.BuildFailed
		}
		if err = b.MarkFinished(updates.BuildNewStatus, finishTime); err != nil {
			return errors.Wrapf(err, "Error marking build as finished with status '%s'", updates.BuildNewStatus)
		}

		// update the build's makespan information
		if err = updateMakespans(b); err != nil {
			return errors.Wrap(err, "Error updating makespan information")
		}
	}

	startPhaseAt := time.Now()

	// These are deliberately out of the buildComplete block to ensure versions
	// are iterated so version and patch notifications can be sent out
	if err = MarkVersionCompleted(b.Version, finishTime, updates); err != nil {
		err = errors.Wrap(err, "Error marking version as finished")
		grip.Error(err)
		return err
	}
	if time.Since(startPhaseAt) > slowMS {
		grip.Debug(message.Fields{
			"function":      "UpdateBuildAndVersionStatusForTask",
			"operation":     "b.MarkVersionCompleted()",
			"message":       "slow operation",
			"duration_secs": time.Since(startPhaseAt).Seconds(),
			"task":          t.Id,
		})
	}

	if evergreen.IsPatchRequester(b.Requester) {
		if err = TryMarkPatchBuildFinished(b, finishTime, updates); err != nil {
			return errors.Wrap(err, "error marking patch as finished")
		}
		if updates.VersionComplete && len(updates.VersionNewStatus) != 0 {
			patchStatus := evergreen.PatchFailed
			if updates.VersionNewStatus == evergreen.VersionSucceeded {
				patchStatus = evergreen.PatchSucceeded
			}
			event.LogPatchStateChangeEvent(t.Version, patchStatus)
		}
	}

	// this is helpful for when we restart a compile task
	if finishedTasks == 0 {
		err = b.UpdateStatus(evergreen.BuildCreated)
		updates.BuildNewStatus = evergreen.BuildCreated
		if err != nil {
			return errors.Wrap(err, "error updating build status")
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
		return updateDisplayTaskAndCache(t)
	}

	// update the cached version of the task, in its build document
	return build.SetCachedTaskStarted(t.BuildId, t.Id, startTime)
}

func MarkTaskUndispatched(t *task.Task) error {
	// record that the task as undispatched on the host
	if err := t.MarkAsUndispatched(); err != nil {
		return errors.WithStack(err)
	}
	// the task was successfully dispatched, log the event
	event.LogTaskUndispatched(t.Id, t.Execution, t.HostId)

	if t.IsPartOfDisplay() {
		return updateDisplayTaskAndCache(t)
	}

	// update the cached version of the task in its related build document
	if err := build.SetCachedTaskUndispatched(t.BuildId, t.Id); err != nil {
		return errors.WithStack(err)
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
		return updateDisplayTaskAndCache(t)
	}

	// update the cached version of the task in its related build document
	if err := build.SetCachedTaskDispatched(t.BuildId, t.Id); err != nil {
		return errors.Wrapf(err, "error updating task cache in build %s", t.BuildId)
	}
	return nil
}

func MarkOneTaskReset(t *task.Task) error {
	if t.DisplayOnly {
		for _, et := range t.ExecutionTasks {
			execTask, err := task.FindOneId(et)
			if err != nil {
				return errors.Wrap(err, "error retrieving execution task")
			}
			if err = MarkOneTaskReset(execTask); err != nil {
				return errors.Wrap(err, "error resetting execution task")
			}
		}
	}

	if err := UpdateUnblockedDependencies(t); err != nil {
		return errors.Wrap(err, "can't clear cached unattainable dependencies")
	}
	return errors.Wrap(t.Reset(), "error resetting task in database")
}

func MarkTasksReset(taskIds []string) error {
	tasks, err := task.FindWithDisplayTasks(task.ByIds(taskIds))
	if err != nil {
		return errors.WithStack(err)
	}
	for _, t := range tasks {
		if t.DisplayOnly {
			taskIds = append(taskIds, t.Id)
		}
		if err = UpdateUnblockedDependencies(&t); err != nil {
			return errors.Wrap(err, "can't clear cached unattainable dependencies")
		}
	}
	return errors.Wrap(task.ResetTasks(taskIds), "error resetting tasks in database")
}

func updateDisplayTaskAndCache(t *task.Task) error {
	err := UpdateDisplayTask(t.DisplayTask)
	if err != nil {
		return errors.Wrap(err, "error updating display task")
	}
	err = build.UpdateCachedTask(t.DisplayTask, 0)
	if err != nil {
		b, findErr := build.FindOneId(t.BuildId)
		grip.Error(message.WrapError(err, message.Fields{
			"message":      "failed to update cached display task",
			"function":     "updateDisplayTaskAndCache",
			"build_id":     t.BuildId,
			"task_id":      t.Id,
			"display_task": t.DisplayTask.Id,
			"status":       t.Status,
			"build_cache":  b.Tasks,
			"find_err":     findErr,
		}))
	}
	return nil
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
	tasksToRestart, err := task.FindWithDisplayTasks(task.ByTimeStartedAndFailed(opts.StartTime, opts.EndTime, failureTypes))
	if err != nil {
		return results, err
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
			displayTasksToCheck[t.DisplayTask.Id] = *t.DisplayTask
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

	t, err := task.FindOne(task.ById(h.RunningTask))
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
	if !t.IsPartOfDisplay() {
		if err = build.UpdateCachedTask(t, 0); err != nil {
			return errors.Wrap(err, "problem resetting cached task")
		}
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
		updates := StatusChanges{}
		if t.DisplayOnly {
			for _, etID := range t.ExecutionTasks {
				var execTask *task.Task
				execTask, err = task.FindOne(task.ById(etID))
				if err != nil {
					return errors.Wrap(err, "error finding execution task")
				}
				if execTask == nil {
					return errors.New("execution task not found")
				}
				if err = MarkEnd(execTask, evergreen.MonitorPackage, time.Now(), &t.Details, false, &updates); err != nil {
					return errors.Wrap(err, "error marking execution task as ended")
				}
			}
		}
		return errors.WithStack(MarkEnd(t, evergreen.MonitorPackage, time.Now(), &t.Details, false, &updates))
	}

	if t.IsPartOfDisplay() {
		if err = t.DisplayTask.SetResetWhenFinished(); err != nil {
			return errors.Wrap(err, "can't mark display task for reset")
		}
		return errors.Wrap(checkResetDisplayTask(t.DisplayTask), "can't check display task reset")
	}

	return errors.Wrap(TryResetTask(t.Id, evergreen.User, evergreen.MonitorPackage, &t.Details), "problem resetting task")
}

func UpdateDisplayTask(t *task.Task) error {
	if !t.DisplayOnly {
		return fmt.Errorf("%s is not a display task", t.Id)
	}

	var timeTaken time.Duration
	var statusTask task.Task
	execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
	if err != nil {
		return errors.Wrap(err, "error retrieving execution tasks")
	}
	hasFinishedTasks := false
	hasUnstartedTasks := false
	startTime := time.Unix(1<<62, 0)
	endTime := utility.ZeroTime
	for _, execTask := range execTasks {
		// if any of the execution tasks are scheduled, the display task is too
		if execTask.Activated {
			t.Activated = true
		}
		if execTask.IsFinished() {
			hasFinishedTasks = true
		}
		if execTask.IsDispatchable() {
			hasUnstartedTasks = true
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
	if hasFinishedTasks && hasUnstartedTasks {
		// if the display task has a mix of finished and unfinished tasks, the display task is still
		// "started" even if there aren't currently running tasks
		statusTask.Status = evergreen.TaskStarted
		statusTask.Details = apimodels.TaskEndDetail{}
	}

	update := bson.M{
		task.StatusKey:    statusTask.Status,
		task.ActivatedKey: t.Activated,
		task.TimeTakenKey: timeTaken,
		task.DetailsKey:   statusTask.Details,
	}

	if startTime != time.Unix(1<<62, 0) {
		update[task.StartTimeKey] = startTime
	}
	if endTime != utility.ZeroTime && !hasUnstartedTasks {
		update[task.FinishTimeKey] = endTime
	}

	// refresh task status from db in case of race
	taskWithStatus, err := task.FindOneNoMerge(task.ById(t.Id).WithFields(task.StatusKey))
	if err != nil {
		return errors.Wrap(err, "error refreshing task status from db")
	}
	if taskWithStatus == nil {
		return errors.New("task not found")
	}
	wasFinished := taskWithStatus.IsFinished()
	err = task.UpdateOne(
		bson.M{
			task.IdKey: t.Id,
		},
		bson.M{
			"$set": update,
		})
	if err != nil {
		return errors.Wrap(err, "error updating display task")
	}
	t.Status = statusTask.Status
	t.Details = statusTask.Details
	t.TimeTaken = timeTaken
	if !wasFinished && t.IsFinished() {
		event.LogTaskFinished(t.Id, t.Execution, "", t.ResultStatus())
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

	return resetManyTasks(tasks, caller)
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
		if !execTask.IsFinished() && execTask.Activated {
			return nil // all tasks not finished
		}
	}
	details := &apimodels.TaskEndDetail{
		Type:   evergreen.CommandTypeSystem,
		Status: evergreen.TaskFailed,
	}
	return errors.Wrap(TryResetTask(t.Id, evergreen.User, evergreen.User, details), "error resetting display task")
}
