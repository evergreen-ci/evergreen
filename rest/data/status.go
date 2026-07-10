package data

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// FindRecentTasks finds tasks that have recently finished.
func FindRecentTasks(ctx context.Context, minutes int) ([]task.Task, *task.ResultCounts, error) {
	tasks, err := task.GetRecentTasks(ctx, time.Duration(minutes)*time.Minute)
	if err != nil {
		return nil, nil, err
	}

	if tasks == nil {
		return []task.Task{}, &task.ResultCounts{}, err
	}
	stats := task.GetResultCounts(tasks)
	return tasks, stats, nil
}

func FindRecentTaskListDistro(ctx context.Context, minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(ctx, minutes, task.DistroIdKey)
	return apiList, errors.WithStack(err)
}

func FindRecentTaskListProject(ctx context.Context, minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(ctx, minutes, task.ProjectKey)
	return apiList, errors.WithStack(err)
}

func FindRecentTaskListAgentVersion(ctx context.Context, minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(ctx, minutes, task.AgentVersionKey)
	return apiList, errors.WithStack(err)
}

func FindRecentTaskList(ctx context.Context, minutes int, key string) (*model.APIRecentTaskStatsList, error) {
	stats, err := task.GetRecentTaskStats(ctx, time.Duration(minutes)*time.Minute, key)
	if err != nil {
		return nil, errors.Wrapf(err, "getting recent task stats from the last %d minutes", minutes)
	}
	list := task.GetResultCountList(stats)

	apiList := model.APIRecentTaskStatsList{}
	apiList.BuildFromService(list)
	return &apiList, nil
}
