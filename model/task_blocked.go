package model

import (
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/tychoish/tarjan"
)

const (
	taskBlocked  = "blocked"
	taskPending  = "pending"
	taskRunnable = "runnable"
)

// BlockedState returns "blocked," "pending" (unsatisfied dependencies,
// but unblocked), or "" (runnable) to represent the state of the task
// with respect to its dependencies
func BlockedState(t *task.Task, tasksWithDeps []task.Task) (string, error) {
	if t.DisplayOnly {
		return blockedStateForDisplayTask(t, tasksWithDeps)
	}
	if t.IsPartOfSingleHostTaskGroup() {
		return blockedStateForTaskGroups(t, tasksWithDeps)
	}
	return blockedStatePrivate(t)
}

func blockedStatePrivate(t *task.Task) (string, error) {
	if len(t.DependsOn) == 0 {
		return taskRunnable, nil
	}
	dependencyIDs := []string{}
	for _, d := range t.DependsOn {
		dependencyIDs = append(dependencyIDs, d.TaskId)
	}
	dependentTasks, err := task.Find(task.ByIds(dependencyIDs).WithFields(task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))
	if err != nil {
		return "", errors.Wrap(err, "error finding dependencies")
	}
	taskMap := map[string]*task.Task{}
	for i := range dependentTasks {
		taskMap[dependentTasks[i].Id] = &dependentTasks[i]
	}
	for _, dependency := range t.DependsOn {
		depTask := taskMap[dependency.TaskId]
		state, err := getStateByDependency(depTask, dependency)
		if err != nil {
			return "", errors.Wrap(err, "")
		}
		if state != taskRunnable {
			return state, nil
		}
	}
	return taskRunnable, nil
}

// getStateByDependency determines if the task is still running, is blocked, or has violated the given dependency
func getStateByDependency(t *task.Task, dependency task.Dependency) (string, error) {
	if t == nil {
		grip.Error(message.Fields{
			"message": "task does not exist",
			"task_id": dependency.TaskId,
		})
		return taskRunnable, nil
	}
	state, err := blockedStatePrivate(t)
	if err != nil {
		return "", err
	}
	if state == taskBlocked {
		return taskBlocked, nil
	} else if t.Status == evergreen.TaskSucceeded || t.Status == evergreen.TaskFailed {
		if t.Status != dependency.Status && dependency.Status != AllStatuses {
			return taskBlocked, nil
		}
	} else {
		return taskPending, nil
	}
	return taskRunnable, nil
}

func blockedStateForTaskGroups(t *task.Task, tasksWithDeps []task.Task) (string, error) {
	tasks, err := GetTasksInTaskGroup(t)
	if err != nil {
		return "", errors.Wrap(err, "problem getting tasks in task group")
	}

	// get dependencies from all tasks in group
	dependsOn := []task.Dependency{}
	dependencyIDs := []string{}
	for _, curTask := range tasks {
		dependsOn = append(dependsOn, curTask.DependsOn...)
		for _, dependency := range curTask.DependsOn {
			dependencyIDs = append(dependencyIDs, dependency.TaskId)
		}
	}
	// get dependentTasks and make map as before
	dependentTasks, err := task.Find(task.ByIds(dependencyIDs).WithFields(task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))
	taskMap := map[string]*task.Task{} // maps ID of dependency to the relevant task
	for i := range dependentTasks {
		taskMap[dependentTasks[i].Id] = &dependentTasks[i]
	}
	//cache status for task in case of duplicates
	cachedStatus := map[string]string{}
	// determine if each dependency is satisfiable
	for _, dependency := range dependsOn {
		var state string
		depTask := taskMap[dependency.TaskId]
		if cachedStatus[depTask.Id] != "" {
			state = cachedStatus[depTask.Id]
		} else {
			var err error
			state, err = getStateByDependency(depTask, dependency)
			if err != nil {
				return "", errors.Wrap(err, "")
			}
			cachedStatus[depTask.Id] = state
		}

		if state != taskRunnable {
			return state, nil // task is blocked or pending
		}
	}
	return taskRunnable, nil
}

func blockedStateForDisplayTask(t *task.Task, tasksWithDeps []task.Task) (string, error) {
	execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
	if err != nil {
		return "", errors.Wrap(err, "error finding execution tasks")
	}
	state := taskRunnable
	for _, execTask := range execTasks {
		etState, err := BlockedState(&execTask, tasksWithDeps)
		if err != nil {
			return "", errors.Wrap(err, "error finding blocked state")
		}
		if etState == taskBlocked {
			return taskBlocked, nil
		} else if etState == taskPending {
			state = taskPending
		}
	}
	return state, nil
}

func CircularDependencies(t *task.Task) error {
	var err error
	tasksWithDeps, err := task.FindAllTasksFromVersionWithDependencies(t.Version)
	if err != nil {
		return errors.Wrap(err, "error finding tasks with dependencies")
	}
	if len(tasksWithDeps) == 0 {
		return nil
	}
	dependencyMap := map[string][]string{}
	for _, versionTask := range tasksWithDeps {
		for _, dependency := range versionTask.DependsOn {
			dependencyMap[versionTask.Id] = append(dependencyMap[versionTask.Id], dependency.TaskId)
		}
	}
	catcher := grip.NewBasicCatcher()
	cycles := tarjan.Connections(dependencyMap)
	for _, cycle := range cycles {
		if len(cycle) > 1 {
			catcher.Add(errors.Errorf("Dependency cycle detected: %s", strings.Join(cycle, ",")))
		}
	}
	return catcher.Resolve()
}

func IsBlockedDisplayTask(t *task.Task) bool {
	if !t.DisplayOnly {
		return false
	}

	tasksWithDeps, err := task.FindAllTasksFromVersionWithDependencies(t.Version)
	if err != nil {
		grip.Error(message.WrapError(err, "error finding tasks with dependencies"))
		return false
	}
	blockedState, err := BlockedState(t, tasksWithDeps)
	if err != nil {
		grip.Error(message.WrapError(err, "error determining blocked state"))
		return false
	}
	return blockedState == taskBlocked
}

// AllUnblockedTasksOrCompileFinished returns true when all activated tasks in the build have
// one of the statuses in IsFinishedTaskStatus or the task is considered blocked
//
// returns boolean to indicate if tasks are complete, string with either BuildFailed or
// BuildSucceded. The string is only valid when the boolean is true
func AllUnblockedTasksFinished(b build.Build, tasksWithDeps []task.Task) (bool, string, error) {
	if !b.Activated {
		return false, b.Status, nil
	}
	allFinished := true
	status := evergreen.BuildSucceeded
	tasks, err := task.Find(task.ByBuildId(b.Id))
	if err != nil {
		return false, "", errors.Wrapf(err, "can't get tasks for build '%s'", b.Id)
	}
	for _, t := range tasks {
		if evergreen.IsFailedTaskStatus(t.Status) {
			status = evergreen.BuildFailed
		}
		if !evergreen.IsFinishedTaskStatus(t.Status) {
			if !t.Activated {
				continue
			}
			var blockedStatus string
			blockedStatus, err = BlockedState(&t, tasksWithDeps)
			if err != nil {
				return false, status, err
			}
			if blockedStatus != taskBlocked {
				allFinished = false
			}
		}
	}
	if allFinished && err != nil {
		return false, status, err
	}

	return allFinished, status, nil
}
