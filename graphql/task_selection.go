package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

const (
	maxTasksToModify = 1000
)

// resolveTasksByIDs returns the tasks with the given IDs, which may belong to different versions.
func resolveTasksByIDs(ctx context.Context, taskIDs []string) ([]task.Task, error) {
	if len(taskIDs) > maxTasksToModify {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot select more than %d tasks by ID, use a filter instead", maxTasksToModify))
	}
	return findAllTasksByIds(ctx, taskIDs...)
}

// resolveTasksByFilter returns the tasks in the version that match the filter.
func resolveTasksByFilter(ctx context.Context, versionID string, f TaskFilterOptions) ([]task.Task, error) {
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}

	opts, err := getTasksByVersionOptions(ctx, v, f)
	if err != nil {
		return nil, err
	}
	opts.FieldsToProject = []string{
		task.IdKey,
		task.StatusKey,
		task.ActivatedKey,
		task.RequesterKey,
		task.DisplayTaskIdKey,
	}

	tasks, _, err := task.GetTasksByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for version '%s': %s", versionID, err.Error()))
	}
	return tasks, nil
}

// getSchedulableTasks returns the tasks that can be scheduled and an error for each task that can't be.
func getSchedulableTasks(tasks []task.Task) ([]task.Task, []*TaskError) {
	schedulable := make([]task.Task, 0, len(tasks))
	taskErrors := []*TaskError{}
	for _, t := range tasks {
		if evergreen.IsGithubMergeQueueRequester(t.Requester) {
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "merge queue tasks cannot be manually scheduled"})
			continue
		}
		if !t.IsUnscheduled() {
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "task is already scheduled or has already run"})
			continue
		}
		schedulable = append(schedulable, t)
	}
	return schedulable, taskErrors
}

// scheduleSelectedTasks schedules the schedulable tasks among the selected tasks and reports the rest as task errors.
func scheduleSelectedTasks(ctx context.Context, selectedTasks []task.Task) (*BulkTaskOperationResult, error) {
	result := &BulkTaskOperationResult{
		ModifiedTasks: []*restModel.APITask{},
		TaskErrors:    []*TaskError{},
	}

	// Execution tasks can't be scheduled individually.
	taskIDs := make([]string, 0, len(selectedTasks))
	for _, t := range selectedTasks {
		id := t.Id
		if displayTaskID := utility.FromStringPtr(t.DisplayTaskId); displayTaskID != "" {
			id = displayTaskID
		}
		taskIDs = append(taskIDs, id)
	}
	taskIDs = utility.UniqueStrings(taskIDs)
	if len(taskIDs) == 0 {
		return result, nil
	}
	if len(taskIDs) > maxTasksToModify {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot modify more than %d tasks at once, but the selection contains %d tasks; narrow the selection", maxTasksToModify, len(taskIDs)))
	}
	tasks, err := findAllTasksByIds(ctx, taskIDs...)
	if err != nil {
		return nil, err
	}

	tasksToSchedule, taskErrors := getSchedulableTasks(tasks)
	result.TaskErrors = taskErrors
	if len(tasksToSchedule) == 0 {
		return result, nil
	}
	logURL := evergreen.GetEnvironment().Settings().Ui.LogUrl
	scheduled, err := setTasksScheduled(ctx, logURL, true, tasksToSchedule)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("scheduling tasks: %s", err.Error()))
	}
	result.ModifiedTasks = scheduled
	return result, nil
}

// getUnschedulableTasks returns the tasks that can be unscheduled and an error for each task that can't be.
func getUnschedulableTasks(tasks []task.Task) ([]task.Task, []*TaskError) {
	unschedulable := make([]task.Task, 0, len(tasks))
	taskErrors := []*TaskError{}
	for _, t := range tasks {
		isScheduled := t.Activated && t.Status == evergreen.TaskUndispatched
		if !isScheduled {
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "task is not scheduled"})
			continue
		}
		unschedulable = append(unschedulable, t)
	}
	return unschedulable, taskErrors
}

// unscheduleSelectedTasks unschedules the unschedulable tasks among the selected tasks and reports the rest as task
// errors. If abort is true, the selected tasks that are already running are also aborted, as when unscheduling a whole
// version.
func unscheduleSelectedTasks(ctx context.Context, selectedTasks []task.Task, abort bool) (*BulkTaskOperationResult, error) {
	result := &BulkTaskOperationResult{
		ModifiedTasks: []*restModel.APITask{},
		TaskErrors:    []*TaskError{},
	}
	taskIDs := make([]string, 0, len(selectedTasks))
	for _, t := range selectedTasks {
		taskIDs = append(taskIDs, t.Id)
	}
	taskIDs = utility.UniqueStrings(taskIDs)
	if len(taskIDs) == 0 {
		return result, nil
	}
	if len(taskIDs) > maxTasksToModify {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot modify more than %d tasks at once, but the selection contains %d tasks; narrow the selection", maxTasksToModify, len(taskIDs)))
	}
	tasks, err := findAllTasksByIds(ctx, taskIDs...)
	if err != nil {
		return nil, err
	}

	tasksToUnschedule, taskErrors := getUnschedulableTasks(tasks)
	result.TaskErrors = taskErrors
	if len(tasksToUnschedule) > 0 {
		logURL := evergreen.GetEnvironment().Settings().Ui.LogUrl
		unscheduled, err := setTasksScheduled(ctx, logURL, false, tasksToUnschedule)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("unscheduling tasks: %s", err.Error()))
		}
		result.ModifiedTasks = unscheduled
	}

	if abort {
		abortInfo := task.AbortInfo{User: mustHaveUser(ctx).Username()}
		if err := task.AbortTasksByIDs(ctx, taskIDs, abortInfo); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("aborting tasks: %s", err.Error()))
		}
	}
	return result, nil
}
