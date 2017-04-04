package taskrunner

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DispatchTaskForHost assigns the task at the head of the task queue to the
// given host, dequeues the task and then marks it as dispatched for the host
func DispatchTaskForHost(taskQueue *model.TaskQueue, assignedHost *host.Host) (
	nextTask *task.Task, err error) {
	if assignedHost == nil {
		return nil, errors.New("can not assign task to a nil host")
	}

	// only proceed if there are pending tasks left
	for !taskQueue.IsEmpty() {
		queueItem := taskQueue.NextTask()
		// pin the task to the given host and fetch the full task document from
		// the database
		nextTask, err = task.FindOne(task.ById(queueItem.Id))
		if err != nil {
			return nil, errors.Wrapf(err, "error finding task with id %v",
				queueItem.Id)
		}
		if nextTask == nil {
			return nil, errors.Errorf("refusing to move forward because queued "+
				"task with id %v does not exist", queueItem.Id)
		}

		// dequeue the task from the queue
		if err = taskQueue.DequeueTask(nextTask.Id); err != nil {
			return nil, errors.Wrapf(err,
				"error pulling task with id %v from queue for distro %v",
				nextTask.Id, nextTask.DistroId)
		}

		// validate that the task can be run, if not fetch the next one in
		// the queue
		if shouldSkipTask(nextTask) {
			grip.Warningf("Skipping task %s, which was "+
				"picked up to be run but is not runnable - "+
				"status (%s) activated (%t)", nextTask.Id, nextTask.Status,
				nextTask.Activated)
			continue
		}

		// record that the task was dispatched on the host
		if err := model.MarkTaskDispatched(nextTask, assignedHost.Id, assignedHost.Distro.Id); err != nil {
			return nil, err
		}
		return nextTask, nil
	}
	return nil, nil
}

// Determines whether or not a task should be skipped over by the
// task runner. Checks if the task is not undispatched, as a sanity check that
// it is not already running.
func shouldSkipTask(task *task.Task) bool {
	return task.Status != evergreen.TaskUndispatched || !task.Activated
}

// Takes in a list of hosts, and returns the hosts sorted by distro, in the
// form of a map distro name -> list of hosts
func (self *TaskRunner) splitHostsByDistro(hostsToSplit []host.Host) map[string][]host.Host {
	hostsByDistro := make(map[string][]host.Host)
	for _, host := range hostsToSplit {
		hostsByDistro[host.Distro.Id] = append(hostsByDistro[host.Distro.Id], host)
	}
	return hostsByDistro
}
