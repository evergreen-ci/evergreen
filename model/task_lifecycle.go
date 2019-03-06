package model

import (
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type StatusChanges struct {
	PatchNewStatus   string
	VersionNewStatus string
	VersionComplete  bool
	BuildNewStatus   string
	BuildComplete    bool
}

func SetActiveState(taskId string, caller string, active bool) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return err
	}
	if active {
		// if the task is being activated, make sure to activate all of the task's
		// dependencies as well
		for _, dep := range t.DependsOn {
			if err = SetActiveState(dep.TaskId, caller, true); err != nil {
				return errors.Wrapf(err, "error activating dependency for %v with id %v",
					taskId, dep.TaskId)
			}
		}

		if t.DispatchTime != util.ZeroTime && t.Status == evergreen.TaskUndispatched {
			if err = resetTask(t.Id, caller); err != nil {
				return errors.Wrap(err, "error resetting task")
			}
		} else {
			if err = t.ActivateTask(caller); err != nil {
				return errors.Wrap(err, "error while activating task")
			}
			event.LogTaskActivated(taskId, t.Execution, caller)
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
		// decativate it.

		err = t.DeactivateTask(caller)
		if err != nil {
			return errors.Wrap(err, "error deactivating task")
		}
		event.LogTaskDeactivated(taskId, t.Execution, caller)
	} else {
		return nil
	}

	if t.IsPartOfDisplay() {
		err = updateDisplayTaskAndCache(t)
		if err != nil {
			return err
		}
		taskId = t.DisplayTask.Id
	}

	return errors.WithStack(build.SetCachedTaskActivated(t.BuildId, taskId, active))
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

	// if this is the first time we're running the task, or it's finished, blacklisted, or already activated
	if prevTask == nil || prevTask.IsFinished() || prevTask.Priority < 0 || prevTask.Activated {
		return nil
	}

	// activate the task
	return errors.WithStack(SetActiveState(prevTask.Id, caller, true))
}

// reset task finds a task, attempts to archive it, and resets the task and resets the TaskCache in the build as well.
func resetTask(taskId, caller string) error {
	t, err := task.FindOneNoMerge(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	if t.IsPartOfDisplay() {
		return fmt.Errorf("cannot restart execution task %s because it is part of a display task", t.Id)
	}
	if err = t.Archive(); err != nil {
		return errors.Wrap(err, "can't restart task because it can't be archived")
	}

	if err = t.Reset(); err != nil {
		return errors.WithStack(err)
	}

	if err = t.ActivateTask(caller); err != nil {
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
	if !t.IsFinished() && !t.IsBlockedDisplayTask() {
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

	if err = resetTask(t.Id, user); err != nil {
		return err
	}
	if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
		event.LogTaskRestarted(t.Id, t.Execution, user)
	} else {
		event.LogTaskRestarted(t.Id, t.Execution, origin)
	}

	return errors.WithStack(err)
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

	if !task.IsAbortable(*t) {
		return errors.Errorf("Task '%v' is currently '%v' - cannot abort task"+
			" in this status", t.Id, t.Status)
	}

	// set the active state and then set the abort
	if err = SetActiveState(t.Id, caller, false); err != nil {
		return err
	}
	event.LogTaskAbortRequest(t.Id, t.Execution, caller)
	return t.SetAborted()
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
				if et.IsFinished() || task.IsAbortable(et) {
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

		err = SetActiveState(t.Id, caller, false)
		if err != nil {
			return err
		}
		event.LogTaskDeactivated(t.Id, t.Execution, caller)
		// update the cached version of the task, in its build document to be deactivated
		if !t.IsPartOfDisplay() {
			if err = build.SetCachedTaskActivated(t.BuildId, t.Id, false); err != nil {
				return err
			}
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

	project, err := FindProjectFromTask(t)
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

	if t.HasFailedTests() {
		detail.Status = evergreen.TaskFailed
	}

	t.Details = *detail

	if t.Status == detail.Status {
		grip.Warningf("Tried to mark task %s as finished twice", t.Id)
		return nil
	}
	if util.IsZeroTime(t.StartTime) {
		grip.Warning(message.Fields{
			"message":      "Task is missing start time",
			"task_id":      t.Id,
			"execution":    t.Execution,
			"requester":    t.Requester,
			"activated_by": t.ActivatedBy,
		})
	}
	err := t.MarkEnd(finishTime, detail)
	if err != nil {
		return err
	}
	status := t.ResultStatus()
	event.LogTaskFinished(t.Id, t.Execution, t.HostId, status)

	if t.IsPartOfDisplay() {
		if err = UpdateDisplayTask(t.DisplayTask); err != nil {
			return err
		}
		if err = build.UpdateCachedTask(t.DisplayTask.BuildId, t.DisplayTask.Id, t.DisplayTask.Status, t.TimeTaken); err != nil {
			return err
		}
	} else {
		err = build.SetCachedTaskFinished(t.BuildId, t.Id, detail.Status, detail, t.TimeTaken)
		if err != nil {
			return errors.Wrap(err, "error updating build")
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

// UpdateBuildStatusForTask finds all the builds for a task and updates the
// status of the build based on the task's status.
func UpdateBuildAndVersionStatusForTask(taskId string, updates *StatusChanges) error {
	// retrieve the task by the task id
	t, err := task.FindOneNoMerge(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}

	finishTime := time.Now()
	// get all of the tasks in the same build
	b, err := build.FindOne(build.ById(t.BuildId))
	if err != nil {
		return errors.WithStack(err)
	}

	buildTasks, err := task.Find(task.ByBuildId(b.Id))
	if err != nil {
		return errors.WithStack(err)
	}
	tasksWithDeps, err := task.FindAllTasksFromVersionWithDependencies(t.Version)
	if err != nil {
		return errors.Wrap(err, "error finding tasks with dependencies")
	}

	failedTask := false
	buildComplete := false
	finishedTasks := 0
	tasksToNotify := 0

	// update the build's status based on tasks for this build
	for _, t := range buildTasks {
		if !t.IsFinished() {
			state, _ := t.BlockedState(tasksWithDeps)
			if state == "blocked" {
				tasksToNotify += 1
				if evergreen.IsFailedTaskStatus(t.Status) {
					failedTask = true
				}
			}
			continue
		}
		var displayTask *task.Task
		status := ""
		finishedTasks++
		tasksToNotify += 1

		displayTask, err = t.GetDisplayTask()
		if err != nil {
			return err
		}
		if displayTask != nil {
			err = UpdateDisplayTask(displayTask)
			if err != nil {
				return err
			}
			t = *displayTask
			status = t.Status
			if t.IsFinished() {
				continue
			}
		}

		// update the build's status when a test task isn't successful
		if t.Status != evergreen.TaskSucceeded {
			err = b.UpdateStatus(evergreen.BuildFailed)
			if err != nil {
				err = errors.Wrap(err, "Error updating build status")
				grip.Error(err)
				return err
			}

			failedTask = true
			if t.DisplayName == evergreen.CompileStage {
				buildComplete = true
				break
			}
		}

		// update the cached version of the task, in its build document
		if status == "" {
			status = t.Details.Status
		}
		err = b.SetCachedTaskFinished(t.Id, status, &t.Details, t.TimeTaken)
		if err != nil {
			return fmt.Errorf("error updating build: %v", err.Error())
		}
	}

	if b.Status == evergreen.BuildCreated {
		if err = b.UpdateStatus(evergreen.BuildStarted); err != nil {
			err = errors.Wrap(err, "Error updating build status")
			grip.Error(err)
			return err
		}
		updates.BuildNewStatus = evergreen.BuildStarted
	}

	if finishedTasks >= len(buildTasks) {
		buildComplete = true
	}
	if buildComplete || tasksToNotify >= len(buildTasks) {
		updates.BuildNewStatus = evergreen.BuildSucceeded
		if failedTask {
			updates.BuildNewStatus = evergreen.BuildFailed
		}
		updates.BuildComplete = true
	}

	// if a compile task didn't fail, then the
	// build is only finished when both the compile
	// and test tasks are completed or when those are
	// both completed in addition to a push (a push
	// does not occur if there's a failed task)
	if buildComplete {
		if !failedTask {
			if err = b.MarkFinished(evergreen.BuildSucceeded, finishTime); err != nil {
				err = errors.Wrap(err, "Error marking build as finished")
				grip.Error(err)
				return err
			}

		} else {
			// some task failed
			if err = b.MarkFinished(evergreen.BuildFailed, finishTime); err != nil {
				err = errors.Wrap(err, "Error marking build as finished")
				grip.Error(err)
				return err
			}
		}
	}

	// These are deliberately out of the buildComplete block to ensure versions
	// are iterated so version and patch notifications can be sent out
	if err = MarkVersionCompleted(b.Version, finishTime, updates); err != nil {
		err = errors.Wrap(err, "Error marking version as finished")
		grip.Error(err)
		return err
	}

	if evergreen.IsPatchRequester(b.Requester) {
		if err = TryMarkPatchBuildFinished(b, finishTime, updates); err != nil {
			err = errors.Wrap(err, "Error marking patch as finished")
			grip.Error(err)
			return err
		}
		if updates.VersionComplete && len(updates.VersionNewStatus) != 0 {
			patchStatus := evergreen.PatchFailed
			if updates.VersionNewStatus == evergreen.VersionSucceeded {
				patchStatus = evergreen.PatchSucceeded
			}
			event.LogPatchStateChangeEvent(t.Version, patchStatus)
		}
	}

	if buildComplete {
		// update the build's makespan information if the task has finished
		if err = updateMakespans(b); err != nil {
			err = errors.Wrap(err, "Error updating makespan information")
			grip.Error(err)
			return err
		}
	}

	// this is helpful for when we restart a compile task
	if finishedTasks == 0 {
		err = b.UpdateStatus(evergreen.BuildCreated)
		updates.BuildNewStatus = evergreen.BuildCreated
		if err != nil {
			err = errors.Wrap(err, "Error updating build status")
			grip.Error(err)
			return err
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
		return errors.WithStack(err)
	}

	// ensure the appropriate version is marked as started if necessary
	if err = MarkVersionStarted(t.Version, startTime); err != nil {
		return errors.WithStack(err)
	}

	// if it's a patch, mark the patch as started if necessary
	if evergreen.IsPatchRequester(t.Requester) {
		err := patch.TryMarkStarted(t.Version, startTime)
		if err == nil {
			updates.PatchNewStatus = evergreen.PatchStarted

		} else if err != mgo.ErrNotFound {
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

func MarkTaskDispatched(t *task.Task, hostId, distroId string) error {
	// record that the task was dispatched on the host
	if err := t.MarkAsDispatched(hostId, distroId, time.Now()); err != nil {
		return errors.Wrapf(err, "error marking task %s as dispatched "+
			"on host %s", t.Id, hostId)
	}
	// the task was successfully dispatched, log the event
	event.LogTaskDispatched(t.Id, t.Execution, hostId)

	if t.IsPartOfDisplay() {
		return updateDisplayTaskAndCache(t)
	}

	// update the cached version of the task in its related build document
	if err := build.SetCachedTaskDispatched(t.BuildId, t.Id); err != nil {
		return errors.Wrapf(err, "error updating task cache in build %s", t.BuildId)
	}
	return nil
}

func updateDisplayTaskAndCache(t *task.Task) error {
	err := UpdateDisplayTask(t.DisplayTask)
	if err != nil {
		return errors.Wrap(err, "error updating display task")
	}
	return build.UpdateCachedTask(t.DisplayTask.BuildId, t.DisplayTask.Id, t.DisplayTask.Status, 0)
}

type RestartTaskOptions struct {
	DryRun    bool      `bson:"dry_run" json:"dry_run"`
	StartTime time.Time `bson:"start_time" json:"start_time"`
	EndTime   time.Time `bson:"end_time" json:"end_time"`
	User      string    `bson:"user" json:"user"`

	// note that the bson tags are not quite accurate, but are kept around for backwards compatibility
	IncludeTestFailed  bool `bson:"only_red" json:"only_red"`
	IncludeSysFailed   bool `bson:"only_purple" json:"only_purple"`
	IncludeSetupFailed bool `bson:"include_setup_failed" json:"include_setup_failed"`
}

type RestartTaskResults struct {
	TasksRestarted []string
	TasksErrored   []string
}

// RestartFailedTasks attempts to restart failed tasks that started between 2 times
// It returns a slice of task IDs that were successfully restarted as well as a slice
// of task IDs that failed to restart
// opts.dryRun will return the tasks that will be restarted if sent true
// opts.red and opts.purple will only restart tasks that were failed due to the test
// or due to the system, respectively
func RestartFailedTasks(opts RestartTaskOptions) (RestartTaskResults, error) {
	results := RestartTaskResults{}
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
	tasksToRestart, err := task.Find(task.ByTimeStartedAndFailed(opts.StartTime, opts.EndTime, failureTypes))
	if err != nil {
		return results, err
	}

	// if this is a dry run, immediately return the tasks found
	if opts.DryRun {
		for _, t := range tasksToRestart {
			results.TasksRestarted = append(results.TasksRestarted, t.Id)
		}
		return results, nil
	}

	return doRestartFailedTasks(tasksToRestart, opts.User, results), nil
}

func doRestartFailedTasks(tasks []task.Task, user string, results RestartTaskResults) RestartTaskResults {
	var tasksErrored []string

	for _, t := range tasks {
		projectRef, err := FindOneProjectRef(t.Project)
		if err != nil {
			tasksErrored = append(tasksErrored, t.Id)
			grip.Error(message.Fields{
				"task":    t.Id,
				"status":  "failed",
				"message": "error retrieving project ref",
				"error":   err.Error(),
			})
			continue
		}
		p, err := FindProject(t.Revision, projectRef)
		if err != nil || p == nil {
			tasksErrored = append(tasksErrored, t.Id)
			grip.Error(message.Fields{
				"task":    t.Id,
				"status":  "failed",
				"message": "error retrieving project",
				"error":   err.Error(),
			})
			continue
		}
		err = TryResetTask(t.Id, user, evergreen.RESTV2Package, nil)
		if err != nil {
			tasksErrored = append(tasksErrored, t.Id)
			grip.Error(message.Fields{
				"task":    t.Id,
				"status":  "failed",
				"message": "error restarting task",
				"error":   err.Error(),
			})
		} else {
			results.TasksRestarted = append(results.TasksRestarted, t.Id)
		}
	}
	results.TasksErrored = tasksErrored

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

	if err = t.MarkSystemFailed(); err != nil {
		return errors.Wrap(err, "problem marking task failed")
	}

	if time.Since(t.ActivatedTime) < task.UnschedulableThreshold {
		if t.IsPartOfDisplay() {
			return t.DisplayTask.SetResetWhenFinished()
		}
		detail := &apimodels.TaskEndDetail{
			Status: evergreen.TaskFailed,
			Type:   "system",
		}
		return errors.Wrap(TryResetTask(t.Id, "mci", evergreen.MonitorPackage, detail), "problem resetting task")
	}

	return nil
}

func UpdateDisplayTask(t *task.Task) error {
	if !t.DisplayOnly {
		return fmt.Errorf("%s is not a display task", t.Id)
	}

	statuses := []string{}
	var timeTaken time.Duration
	var status string
	wasFinished := t.IsFinished()
	execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
	if err != nil {
		return errors.Wrap(err, "error retrieving execution tasks")
	}
	hasFinishedTasks := false
	hasUnfinishedTasks := false
	startTime := time.Unix(1<<62, 0)
	endTime := util.ZeroTime
	for _, execTask := range execTasks {
		// if any of the execution tasks are scheduled, the display task is too
		if execTask.Activated {
			t.Activated = true
		}

		if execTask.IsFinished() {
			hasFinishedTasks = true
		} else if execTask.IsDispatchable() {
			hasUnfinishedTasks = true
		}

		// the display task's status will be the highest priority of its exec tasks
		statuses = append(statuses, execTask.ResultStatus())

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

	if hasFinishedTasks && hasUnfinishedTasks {
		// if the display task has a mix of finished and unfinished tasks, the status
		// will be "started"
		status = evergreen.TaskStarted
	} else if len(statuses) > 0 {
		// the status of the display task will be the status of its constituent task
		// that is logically the most exclusive
		sort.Sort(task.ByPriority(statuses))
		status = statuses[0]
	}

	grip.InfoWhen(status == evergreen.TaskUndispatched && t.DispatchTime != util.ZeroTime, message.Fields{
		"lookhere":                      "evg-3345",
		"message":                       "update display tasks",
		"task_id":                       t.Id,
		"status":                        status,
		"dispatch_time_is_go_zero_time": t.DispatchTime.IsZero(),
	})

	update := bson.M{
		task.StatusKey:    status,
		task.ActivatedKey: t.Activated,
		task.TimeTakenKey: timeTaken,
	}
	if startTime != time.Unix(1<<62, 0) {
		update[task.StartTimeKey] = startTime
	}
	if endTime != util.ZeroTime && !hasUnfinishedTasks {
		update[task.FinishTimeKey] = endTime
	}

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

	t.Status = status
	t.TimeTaken = timeTaken
	if !wasFinished && t.IsFinished() {
		event.LogDisplayTaskFinished(t.Id, t.Execution, t.Status)
		if t.ResetWhenFinished {

			details := &apimodels.TaskEndDetail{
				Type:   evergreen.CommandTypeSystem,
				Status: evergreen.TaskFailed,
			}
			return errors.Wrap(TryResetTask(t.Id, evergreen.User, evergreen.User, details), "error resetting display task")
		}
	}
	return nil
}
