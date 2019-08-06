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

func (self *DBTaskQueuePersister) PersistTaskQueue(distro string, tasks []task.Task, distroQueueInfo model.DistroQueueInfo) ([]model.TaskQueueItem, error) {
	taskQueue := make([]model.TaskQueueItem, 0, len(tasks))
	for _, t := range tasks {
		// Does this task have any dependencies?
		dependencies := make([]string, 0, len(t.DependsOn))
		for _, d := range t.DependsOn {
			dependencies = append(dependencies, d.TaskId)
		}
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
			Dependencies:        dependencies,
		})
	}

	queue := model.NewTaskQueue(distro, taskQueue, distroQueueInfo)
	err := queue.Save() // queue.Save() will only save the first 500 tasks

	return taskQueue, errors.WithStack(err)
}
