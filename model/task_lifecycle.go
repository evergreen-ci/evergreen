package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

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
			err = resetTask(t.Id)
			if err != nil {
				return errors.Wrap(err, "error resetting task")
			}
		} else {
			err = t.ActivateTask(caller)
			if err != nil {
				return errors.Wrap(err, "error while activating task")
			}
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
	} else {
		return nil
	}

	if active {
		event.LogTaskActivated(taskId, caller)
	} else {
		event.LogTaskDeactivated(taskId, caller)
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

	// if this is the first time we're running the task, or it's finished or it is blacklisted
	if prevTask == nil || task.IsFinished(*prevTask) || prevTask.Priority < 0 {
		return nil
	}

	// activate the task
	return errors.WithStack(SetActiveState(prevTask.Id, caller, true))
}

// reset task finds a task, attempts to archive it, and resets the task and resets the TaskCache in the build as well.
func resetTask(taskId string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}

	if err = t.Archive(); err != nil {
		return errors.Wrap(err, "can't restart task because it can't be archived")
	}

	if err = t.Reset(); err != nil {
		return errors.WithStack(err)
	}

	// update the cached version of the task, in its build document
	if err = build.ResetCachedTask(t.BuildId, t.Id); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(UpdateBuildAndVersionStatusForTask(t.Id))
}

// TryResetTask resets a task
func TryResetTask(taskId, user, origin string, p *Project, detail *apimodels.TaskEndDetail) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	// if we've reached the max number of executions for this task, mark it as finished and failed
	if t.Execution >= evergreen.MaxTaskExecution {
		// restarting from the UI bypasses the restart cap
		message := fmt.Sprintf("Task '%v' reached max execution (%v):", t.Id, evergreen.MaxTaskExecution)
		if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
			grip.Debugln(message, "allowing exception for", user)
		} else {
			grip.Debugln(message, "marking as failed")
			if detail != nil {
				return errors.WithStack(MarkEnd(t.Id, origin, time.Now(), detail, p, false))
			} else {
				panic(fmt.Sprintf("TryResetTask called with nil TaskEndDetail by %s", origin))
			}
		}
	}

	// only allow re-execution for failed or successful tasks
	if !task.IsFinished(*t) {
		// this is to disallow terminating running tasks via the UI
		if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
			grip.Debugf("Unsatisfiable '%s' reset request on '%s' (status: '%s')",
				user, t.Id, t.Status)
			return errors.Errorf("Task '%v' is currently '%v' - cannot reset task in this status",
				t.Id, t.Status)
		}
	}

	if detail != nil {
		if err = t.MarkEnd(time.Now(), detail); err != nil {
			return errors.Wrap(err, "Error marking task as ended")
		}
	}

	if err = resetTask(t.Id); err == nil {
		if origin == evergreen.UIPackage || origin == evergreen.RESTV2Package {
			event.LogTaskRestarted(t.Id, user)
		} else {
			event.LogTaskRestarted(t.Id, origin)
		}
	}
	return errors.WithStack(err)
}

func AbortTask(taskId, caller string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return err
	}

	if !task.IsAbortable(*t) {
		return errors.Errorf("Task '%v' is currently '%v' - cannot abort task"+
			" in this status", t.Id, t.Status)
	}

	grip.Debugln("Aborting task", t.Id)
	// set the active state and then set the abort
	if err = SetActiveState(t.Id, caller, false); err != nil {
		return err
	}
	event.LogTaskAbortRequest(t.Id, caller)
	return t.SetAborted()
}

// Deactivate any previously activated but undispatched
// tasks for the same build variant + display name + project combination
// as the task.
func DeactivatePreviousTasks(taskId, caller string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return err
	}
	statuses := []string{evergreen.TaskUndispatched}
	allTasks, err := task.Find(task.ByActivatedBeforeRevisionWithStatuses(t.RevisionOrderNumber, statuses, t.BuildVariant,
		t.DisplayName, t.Project))
	if err != nil {
		return err
	}
	for _, t := range allTasks {
		err = SetActiveState(t.Id, caller, false)
		if err != nil {
			return err
		}
		event.LogTaskDeactivated(t.Id, caller)
		// update the cached version of the task, in its build document to be deactivated
		if err = build.SetCachedTaskActivated(t.BuildId, t.Id, false); err != nil {
			return err
		}
	}

	return nil
}

// Returns true if the task should stepback upon failure, and false
// otherwise. Note that the setting is obtained from the top-level
// project, if not explicitly set on the task.
func getStepback(taskId string, project *Project) (bool, error) {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return false, err
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
func MarkEnd(taskId, caller string, finishTime time.Time, detail *apimodels.TaskEndDetail,
	p *Project, deactivatePrevious bool) error {

	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return err
	}
	if t == nil {
		return errors.Errorf("Task not found for taskId: %s", taskId)
	}

	for _, result := range t.TestResults {
		if result.Status == evergreen.TestFailedStatus {
			detail.Status = evergreen.TaskFailed
			break
		}
	}

	t.Details = *detail

	if t.Status == detail.Status {
		grip.Warningf("Tried to mark task %s as finished twice", t.Id)
		return nil
	}

	err = t.MarkEnd(finishTime, detail)
	if err != nil {
		return err
	}
	event.LogTaskFinished(t.Id, t.HostId, detail.Status)

	// update the cached version of the task, in its build document
	err = build.SetCachedTaskFinished(t.BuildId, t.Id, detail, t.TimeTaken)
	if err != nil {
		return errors.Wrap(err, "error updating build")
	}

	// no need to activate/deactivate other task if this is a patch request's task
	if t.Requester == evergreen.PatchVersionRequester {
		return errors.Wrap(UpdateBuildAndVersionStatusForTask(t.Id),
			"Error updating build status (1)")
	}
	if detail.Status == evergreen.TaskFailed {
		shouldStepBack, err := getStepback(t.Id, p)
		if err != nil {
			return errors.WithStack(err)
		}
		if shouldStepBack {
			if err = doStepback(t); err != nil {
				return errors.Wrap(err, "Error during step back")
			}
		} else {
			grip.Debugln("Not stepping backwards on task failure:", t.Id)
		}
	} else if deactivatePrevious {
		// if the task was successful, ignore running previous
		// activated tasks for this buildvariant

		if err = DeactivatePreviousTasks(t.Id, caller); err != nil {
			return errors.Wrap(err, "Error deactivating previous task")
		}
	}

	// update the build
	if err := UpdateBuildAndVersionStatusForTask(t.Id); err != nil {
		return errors.Wrap(err, "Error updating build status (2)")
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
func UpdateBuildAndVersionStatusForTask(taskId string) error {
	// retrieve the task by the task id
	t, err := task.FindOne(task.ById(taskId))
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

	pushTaskExists := false
	for _, t := range buildTasks {
		if t.DisplayName == evergreen.PushStage {
			pushTaskExists = true
		}
	}

	failedTask := false
	pushSuccess := true
	pushCompleted := false
	finishedTasks := 0

	// update the build's status based on tasks for this build
	for _, t := range buildTasks {
		if task.IsFinished(t) {
			finishedTasks++

			// if it was a compile task, mark the build status accordingly
			if t.DisplayName == evergreen.CompileStage {
				if t.Status != evergreen.TaskSucceeded {
					failedTask = true
					finishedTasks = -1
					err = b.MarkFinished(evergreen.BuildFailed, finishTime)
					if err != nil {
						err = errors.Wrap(err, "Error marking build as finished")
						grip.Error(err)
						return err
					}
					break
				}
			} else if t.DisplayName == evergreen.PushStage {
				pushCompleted = true
				// if it's a finished push, check if it was successful
				if t.Status != evergreen.TaskSucceeded {
					err = b.UpdateStatus(evergreen.BuildFailed)
					if err != nil {
						err = errors.Wrap(err, "Error updating build status")
						grip.Error(err)
						return err
					}
					pushSuccess = false
				}
			} else {
				// update the build's status when a test task isn't successful
				if t.Status != evergreen.TaskSucceeded {
					err = b.UpdateStatus(evergreen.BuildFailed)
					if err != nil {
						err = errors.Wrap(err, "Error updating build status")
						grip.Error(err)
						return err
					}
					failedTask = true
				}

				// update the cached version of the task, in its build document
				err = build.SetCachedTaskFinished(t.BuildId, t.Id, &t.Details, t.TimeTaken)
				if err != nil {
					return fmt.Errorf("error updating build: %v", err.Error())
				}

			}
		}
	}

	// if there are no failed tasks, mark the build as started
	if !failedTask {
		if err = b.UpdateStatus(evergreen.BuildStarted); err != nil {
			err = errors.Wrap(err, "Error updating build status")
			grip.Error(err)
			return err
		}
	}

	// if a compile task didn't fail, then the
	// build is only finished when both the compile
	// and test tasks are completed or when those are
	// both completed in addition to a push (a push
	// does not occur if there's a failed task)
	if finishedTasks >= len(buildTasks)-1 {

		if !failedTask {
			if pushTaskExists { // this build has a push task associated with it.
				if pushCompleted && pushSuccess { // the push succeeded, so mark the build as succeeded.
					err = b.MarkFinished(evergreen.BuildSucceeded, finishTime)
					if err != nil {
						err = errors.Wrap(err, "Error marking build as finished")
						grip.Error(err)
						return err
					}
				} else if pushCompleted && !pushSuccess { // the push failed, mark build failed.
					err = b.MarkFinished(evergreen.BuildFailed, finishTime)
					if err != nil {
						err = errors.Wrap(err, "Error marking build as finished")
						grip.Error(err)
						return err
					}
				}

				// Otherwise, this build does have a "push" task, but it hasn't finished yet
				// So do nothing, since we don't know the status yet.

				if err = MarkVersionCompleted(b.Version, finishTime); err != nil {
					err = errors.Wrap(err, "Error marking version as finished")
					grip.Error(err)
					return err
				}
			} else { // this build has no push task. so go ahead and mark it success/failure.
				if err = b.MarkFinished(evergreen.BuildSucceeded, finishTime); err != nil {
					err = errors.Wrap(err, "Error marking build as finished")
					grip.Error(err)
					return err
				}
				if b.Requester == evergreen.PatchVersionRequester {
					if err = TryMarkPatchBuildFinished(b, finishTime); err != nil {
						err = errors.Wrap(err, "Error marking patch as finished")
						grip.Error(err)
						return err
					}
				}
				if err = MarkVersionCompleted(b.Version, finishTime); err != nil {
					err = errors.Wrap(err, "Error marking version as finished")
					grip.Error(err)
					return err
				}
			}
		} else {
			// some task failed
			if err = b.MarkFinished(evergreen.BuildFailed, finishTime); err != nil {
				err = errors.Wrap(err, "Error marking build as finished")
				grip.Error(err)
				return err
			}
			if b.Requester == evergreen.PatchVersionRequester {
				if err = TryMarkPatchBuildFinished(b, finishTime); err != nil {
					err = errors.Wrap(err, "Error marking patch as finished")
					grip.Error(err)
					return err
				}
			}
			if err = MarkVersionCompleted(b.Version, finishTime); err != nil {
				err = errors.Wrap(err, "Error marking version as finished")
				grip.Error(err)
				return err
			}
		}

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
		if err != nil {
			err = errors.Wrap(err, "Error updating build status")
			grip.Error(err)
			return err
		}
	}

	return nil
}

// MarkStart updates the task, build, version and if necessary, patch documents with the task start time
func MarkStart(taskId string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	startTime := time.Now()
	if err = t.MarkStart(startTime); err != nil {
		return errors.WithStack(err)
	}
	event.LogTaskStarted(t.Id)

	// ensure the appropriate build is marked as started if necessary
	if err = build.TryMarkStarted(t.BuildId, startTime); err != nil {
		return errors.WithStack(err)
	}

	// ensure the appropriate version is marked as started if necessary
	if err = MarkVersionStarted(t.Version, startTime); err != nil {
		return errors.WithStack(err)
	}

	// if it's a patch, mark the patch as started if necessary
	if t.Requester == evergreen.PatchVersionRequester {
		if err = patch.TryMarkStarted(t.Version, startTime); err != nil {
			return errors.WithStack(err)
		}
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
	event.LogTaskUndispatched(t.Id, t.HostId)

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
	event.LogTaskDispatched(t.Id, hostId)

	// update the cached version of the task in its related build document
	if err := build.SetCachedTaskDispatched(t.BuildId, t.Id); err != nil {
		return errors.Wrapf(err, "error updating task cache in build %s", t.BuildId)
	}
	return nil
}
