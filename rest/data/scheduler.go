package data

import (
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/pkg/errors"
)

type SchedulerConnector struct{}

func (s *SchedulerConnector) CompareTasks(taskIds []string) ([]string, map[string]map[string]string, error) {
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
	distro := tasks[0].DistroId
	for _, t := range tasks {
		if t.DistroId != distro {
			return nil, nil, errors.New("all tasks must have the same distro")
		}
	}
	tasks, versions, err := scheduler.FilterTasksWithVersionCache(tasks)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to find versions for tasks")
	}

	cmp := scheduler.CmpBasedTaskPrioritizer{}
	tasks, logic, err := cmp.PrioritizeTasks(distro, tasks, versions)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to prioritize tasks")
	}
	prioritizedIds := []string{}
	for _, t := range tasks {
		prioritizedIds = append(prioritizedIds, t.Id)
	}
	return prioritizedIds, logic, nil
}

type MockSchedulerConnector struct{}

func (s *MockSchedulerConnector) CompareTasks(taskIds []string) ([]string, map[string]map[string]string, error) {
	return nil, nil, nil
}
