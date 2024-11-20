package data

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/pkg/errors"
)

// CompareTasks returns the order that the given tasks would be scheduled, along with the scheduling logic.
func CompareTasks(ctx context.Context, taskIds []string, useLegacy bool) ([]string, map[string]map[string]string, error) {
	if len(taskIds) == 0 {
		return nil, nil, nil
	}
	tasks, err := task.Find(task.ByIds(taskIds))
	if err != nil {
		return nil, nil, errors.Wrap(err, "finding tasks to compare")
	}
	if len(tasks) != len(taskIds) {
		return nil, nil, errors.Errorf("%d/%d tasks do not exist", len(taskIds)-len(tasks), len(taskIds))
	}
	distroId := tasks[0].DistroId
	for _, t := range tasks {
		if t.DistroId != distroId {
			return nil, nil, errors.New("all tasks must have the same distro ID")
		}
	}
	tasks, versions, err := scheduler.FilterTasksWithVersionCache(tasks)
	if err != nil {
		return nil, nil, errors.Wrap(err, "finding versions for tasks")
	}

	cmp := scheduler.CmpBasedTaskPrioritizer{}
	logic := map[string]map[string]string{}
	if useLegacy {
		tasks, logic, err = cmp.PrioritizeTasks(distroId, tasks, versions)
		if err != nil {
			return nil, nil, errors.Wrap(err, "prioritizing tasks")
		}
		prioritizedIds := []string{}
		for _, t := range tasks {
			prioritizedIds = append(prioritizedIds, t.Id)
		}
	} else { // this is temporary: logic should be added in DEVPROD-1849
		d, err := distro.FindOneId(ctx, distroId)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "finding distro '%s'", distroId)
		}
		if d == nil {
			return nil, nil, errors.Errorf("distro '%s' not found", distroId)
		}
		taskPlan := scheduler.PrepareTasksForPlanning(d, tasks)
		tasks = taskPlan.Export(ctx)
	}
	prioritizedIds := []string{}
	for _, t := range tasks {
		prioritizedIds = append(prioritizedIds, t.Id)
	}
	return prioritizedIds, logic, nil
}
