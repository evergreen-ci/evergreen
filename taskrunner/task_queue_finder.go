package taskrunner

import (
	"10gen.com/mci/model"
)

// Interface responsible for finding the queues of tasks that need to be run
type TaskQueueFinder interface {
	// Find the queue of tasks to be run for the specified distro
	FindTaskQueue(distroId string) (*model.TaskQueue, error)
}

// Implementation of the TaskQueueFinder that fetches the tasks from the
// task queue collection in the database
type DBTaskQueueFinder struct{}

// Finds the task queue for the specified distro, by fetching the appropriate
// task queue document from the database
func (self *DBTaskQueueFinder) FindTaskQueue(distroId string) (*model.TaskQueue,
	error) {
	return model.FindTaskQueueForDistro(distroId)
}
