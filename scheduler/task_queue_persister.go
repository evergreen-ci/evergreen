package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// PersistTaskQueue saves the task queue to the database.
// The number of tasks that it saves will not exceed the provided maxScheduledTasks.
// Returns an error if the db call returns an error.
func PersistTaskQueue(ctx context.Context, distro string, tasks []task.Task, distroQueueInfo model.DistroQueueInfo, maxScheduledTasks int) error {
	startAt := time.Now()
	// Cap queue-doc size to stay off the 16 MB cliff. distroQueueInfo stays uncapped so the host
	// allocator sees true demand.
	tasks = capTaskQueueLength(ctx, distro, tasks, maxScheduledTasks)

	taskQueue := make([]model.TaskQueueItem, 0, len(tasks))
	for _, t := range tasks {
		// Does this task have any dependencies?
		dependencies := make([]string, 0, len(t.DependsOn))
		for _, d := range t.DependsOn {
			dependencies = append(dependencies, d.TaskId)
		}
		taskQueue = append(taskQueue, model.TaskQueueItem{
			Id:                    t.Id,
			DisplayName:           t.DisplayName,
			BuildVariant:          t.BuildVariant,
			RevisionOrderNumber:   t.RevisionOrderNumber,
			Requester:             t.Requester,
			Revision:              t.Revision,
			Project:               t.Project,
			ExpectedDuration:      t.ExpectedDuration,
			Priority:              t.Priority,
			SortingValueBreakdown: t.SortingValueBreakdown,
			Group:                 t.TaskGroup,
			GroupMaxHosts:         t.TaskGroupMaxHosts,
			GroupIndex:            t.TaskGroupOrder,
			Version:               t.Version,
			ActivatedBy:           t.ActivatedBy,
			Dependencies:          dependencies,
			DependenciesMet:       t.HasDependenciesMet(),
		})
	}

	queue := model.NewTaskQueue(distro, taskQueue, distroQueueInfo)
	err := queue.Save(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	// Only capped-in tasks are marked scheduled; dropped ones wait for the next pass.
	if err := task.SetTasksScheduledAndDepsMetTime(ctx, tasks, startAt); err != nil {
		return errors.Wrapf(err, "setting scheduled time for prioritized tasks for distro '%s'", distro)
	}
	return nil
}

// capTaskQueueLength caps tasks at maxScheduledTasks (<=0 disables), always keeping a straddling task
// group whole since its members must dispatch to the same host together. Logs when it binds.
func capTaskQueueLength(ctx context.Context, distro string, tasks []task.Task, maxScheduledTasks int) []task.Task {
	if maxScheduledTasks <= 0 || len(tasks) <= maxScheduledTasks {
		return tasks
	}
	// Extend past the cap to keep a straddling task group whole — its members must dispatch together.
	cut := maxScheduledTasks
	for cut < len(tasks) && tasks[cut].TaskGroup != "" && tasks[cut].TaskGroup == tasks[cut-1].TaskGroup {
		cut++
	}
	grip.Warning(ctx, message.Fields{
		"message": "task queue capped at materialization limit",
		"distro":  distro,
		"planned": len(tasks),
		"cap":     cut,
		"limit":   maxScheduledTasks,
	})
	return tasks[:cut]
}
