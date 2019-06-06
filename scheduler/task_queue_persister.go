package scheduler

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// TaskQueuePersister is responsible for taking a task queue for a particular distro
// and saving it.
type TaskQueuePersister interface {
	// distro, tasks, duration cache
	PersistTaskQueue(string, []task.Task, model.DistroQueueInfo) ([]model.TaskQueueItem, error)
}

// DBTaskQueuePersister saves a queue to the database.
type DBTaskQueuePersister struct{}

// PersistTaskQueue saves the task queue to the database.
// Returns an error if the db call returns an error.
func (self *DBTaskQueuePersister) PersistTaskQueue(distro string, tasks []task.Task, distroQueueInfo model.DistroQueueInfo) ([]model.TaskQueueItem, error) {
	taskQueue := make([]model.TaskQueueItem, 0, len(tasks))
	for _, t := range tasks {
		taskQueue = append(taskQueue, model.TaskQueueItem{
			Id:                  t.Id,
			DisplayName:         t.DisplayName,
			BuildVariant:        t.BuildVariant,
			RevisionOrderNumber: t.RevisionOrderNumber,
			Requester:           t.Requester,
			Revision:            t.Revision,
			Project:             t.Project,
			ExpectedDuration:    t.ExpectedDuration,
			Priority:            t.Priority,
			Group:               t.TaskGroup,
			GroupMaxHosts:       t.TaskGroupMaxHosts,
			Version:             t.Version,
		})
	}

	queue := model.NewTaskQueue(distro, taskQueue, distroQueueInfo)
	err := queue.Save() // queue.Save() will only save the first 2000 tasks

	return taskQueue, errors.WithStack(err)
}
