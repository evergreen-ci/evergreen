package data

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// FindRecentTasks finds tasks that have recently finished.
func FindRecentTasks(minutes int) ([]task.Task, *task.ResultCounts, error) {
	tasks, err := task.GetRecentTasks(time.Duration(minutes) * time.Minute)
	if err != nil {
		return nil, nil, err
	}

	if tasks == nil {
		return []task.Task{}, &task.ResultCounts{}, err
	}
	stats := task.GetResultCounts(tasks)
	return tasks, stats, nil
}

func FindRecentTaskListDistro(minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(minutes, task.DistroIdKey)
	return apiList, errors.WithStack(err)
}

func FindRecentTaskListProject(minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(minutes, task.ProjectKey)
	return apiList, errors.WithStack(err)
}

func FindRecentTaskListAgentVersion(minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(minutes, task.AgentVersionKey)
	return apiList, errors.WithStack(err)
}

func FindRecentTaskList(minutes int, key string) (*model.APIRecentTaskStatsList, error) {
	stats, err := task.GetRecentTaskStats(time.Duration(minutes)*time.Minute, key)
	if err != nil {
		return nil, errors.Wrap(err, "can't get task stats")
	}
	list := task.GetResultCountList(stats)

	apiList := model.APIRecentTaskStatsList{}
	if err := apiList.BuildFromService(list); err != nil {
		return nil, errors.Wrap(err, "can't convert to API")
	}

	return &apiList, nil
}
