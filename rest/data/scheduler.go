package data

import (
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/pkg/errors"
)

type SchedulerConnector struct{}

func (s *SchedulerConnector) CompareTasks(taskIds []string, useLegacy bool) ([]string, map[string]map[string]string, error) {
	if len(taskIds) == 0 {
		return nil, nil, nil
	}
	tasks, err := task.Find(task.ByIds(taskIds))
	if err != nil {
		return nil, nil, errors.Wrap(err, "error finding tasks")
	}
	if len(tasks) != len(taskIds) {
		return nil, nil, errors.Errorf("%d tasks do not exist", len(taskIds)-len(tasks))
	}
	distroId := tasks[0].DistroId
	for _, t := range tasks {
		if t.DistroId != distroId {
			return nil, nil, errors.New("all tasks must have the same distroId")
		}
	}
	tasks, versions, err := scheduler.FilterTasksWithVersionCache(tasks)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to find versions for tasks")
	}

	cmp := scheduler.CmpBasedTaskPrioritizer{}
	logic := map[string]map[string]string{}
	if useLegacy {
		tasks, logic, err = cmp.PrioritizeTasks(distroId, tasks, versions)
		if err != nil {
			return nil, nil, errors.Wrap(err, "unable to prioritize tasks")
		}
		prioritizedIds := []string{}
		for _, t := range tasks {
			prioritizedIds = append(prioritizedIds, t.Id)
		}
	} else { // this is temporary: logic should be added in EVG-13795
		d, err := distro.FindByID(distroId)
		if err != nil {
			return nil, nil, errors.Wrap(err, "unable to find distro")
		}
		if d == nil {
			return nil, nil, errors.New("distro doesn't exist")
		}
		taskPlan := scheduler.PrepareTasksForPlanning(d, tasks)
		tasks = taskPlan.Export()
	}
	prioritizedIds := []string{}
	for _, t := range tasks {
		prioritizedIds = append(prioritizedIds, t.Id)
	}
	return prioritizedIds, logic, nil
}
