package scheduler

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
)

// TaskQueuePersister is responsible for taking a task queue for a particular distro
// and saving it.
type TaskQueuePersister interface {
	PersistTaskQueue(distro string, tasks []model.Task,
		taskExpectedDuration model.ProjectTaskDurations) ([]model.TaskQueueItem,
		error)
}

// DBTaskQueuePersister saves a queue to the database.
type DBTaskQueuePersister struct{}

// PersistTaskQueue saves the task queue to the database.
// Returns an error if the db call returns an error.
func (self *DBTaskQueuePersister) PersistTaskQueue(distro string,
	tasks []model.Task,
	taskDurations model.ProjectTaskDurations) ([]model.TaskQueueItem, error) {
	taskQueue := make([]model.TaskQueueItem, 0, len(tasks))
	for _, task := range tasks {
		expectedTaskDuration := model.GetTaskExpectedDuration(task, taskDurations)
		taskQueue = append(taskQueue, model.TaskQueueItem{
			Id:                  task.Id,
			DisplayName:         task.DisplayName,
			BuildVariant:        task.BuildVariant,
			RevisionOrderNumber: task.RevisionOrderNumber,
			Requester:           task.Requester,
			Revision:            task.Revision,
			Project:             task.Project,
			ExpectedDuration:    expectedTaskDuration,
		})
		if err := task.SetExpectedDuration(expectedTaskDuration); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error updating projected task "+
				"duration for %v: %v", task.Id, err)
		}
	}
	return taskQueue, model.UpdateTaskQueue(distro, taskQueue)
}
