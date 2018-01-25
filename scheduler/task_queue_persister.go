package scheduler

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// TaskQueuePersister is responsible for taking a task queue for a particular distro
// and saving it.
type TaskQueuePersister interface {
	PersistTaskQueue(distro string, tasks []task.Task,
		taskExpectedDuration model.ProjectTaskDurations) ([]model.TaskQueueItem,
		error)
}

// DBTaskQueuePersister saves a queue to the database.
type DBTaskQueuePersister struct{}

// PersistTaskQueue saves the task queue to the database.
// Returns an error if the db call returns an error.
func (self *DBTaskQueuePersister) PersistTaskQueue(distro string,
	tasks []task.Task,
	taskDurations model.ProjectTaskDurations) ([]model.TaskQueueItem, error) {
	taskQueue := make([]model.TaskQueueItem, 0, len(tasks))
	for _, t := range tasks {
		expectedTaskDuration := model.GetTaskExpectedDuration(t, taskDurations)
		taskQueue = append(taskQueue, model.TaskQueueItem{
			Id:                  t.Id,
			DisplayName:         t.DisplayName,
			BuildVariant:        t.BuildVariant,
			RevisionOrderNumber: t.RevisionOrderNumber,
			Requester:           t.Requester,
			Revision:            t.Revision,
			Project:             t.Project,
			ExpectedDuration:    expectedTaskDuration,
			Priority:            t.Priority,
		})

		if err := t.SetExpectedDuration(expectedTaskDuration); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  RunnerName,
				"task":    t.Id,
				"message": "problem updating projected task duration",
			}))
		}
	}
	return taskQueue, model.UpdateTaskQueue(distro, taskQueue)
}
