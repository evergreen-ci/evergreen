package scheduler

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// PersistTaskQueue saves the task queue to the database.
// Returns an error if the db call returns an error.
func PersistTaskQueue(distro string, tasks []task.Task, distroQueueInfo model.DistroQueueInfo) error {
	startAt := time.Now()
	taskQueue := make([]model.TaskQueueItem, 0, len(tasks))

	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	var duplicateTaskIDs []string
	if distroQueueInfo.AliasQueue {
		var err error
		duplicateTaskIDs, err = model.FindEnqueuedTaskIDs(taskIDs, distroQueueInfo.GetQueueCollection())
		if err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "could not find duplicate task IDs from other collection's task queues",
			}))
		}
	}

	for _, t := range tasks {
		// Ignore tasks that are already in the other queue.
		if util.StringSliceContains(duplicateTaskIDs, t.Id) {
			continue
		}

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
			GroupIndex:          t.TaskGroupOrder,
			Version:             t.Version,
			Dependencies:        dependencies,
		})
	}

	queue := model.NewTaskQueue(distro, taskQueue, distroQueueInfo)
	err = queue.Save()
	if err != nil {
		return errors.WithStack(err)
	}

	// track scheduled time for prioritized tasks
	if err = task.SetTasksScheduledTime(tasks, startAt); err != nil {
		return errors.Wrapf(err, "error setting scheduled time for prioritized tasks for distro '%s'", distro)
	}

	return nil
}
