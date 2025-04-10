package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Events is the resolver for the events field.
func (r *podResolver) Events(ctx context.Context, obj *model.APIPod, limit *int, page *int) (*PodEvents, error) {
	events, count, err := event.MostRecentPaginatedPodEvents(ctx, utility.FromStringPtr(obj.ID), *limit, *page)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding events for pod '%s': %s", utility.FromStringPtr(obj.ID), err.Error()))
	}
	apiEventLogEntries := []*model.PodAPIEventLogEntry{}
	for _, e := range events {
		apiEventLog := model.PodAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("building APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogEntries = append(apiEventLogEntries, &apiEventLog)
	}
	podEvents := PodEvents{
		EventLogEntries: apiEventLogEntries,
		Count:           count,
	}
	return &podEvents, nil
}

// Status is the resolver for the status field.
func (r *podResolver) Status(ctx context.Context, obj *model.APIPod) (string, error) {
	return string(obj.Status), nil
}

// Task is the resolver for the task field.
func (r *podResolver) Task(ctx context.Context, obj *model.APIPod) (*model.APITask, error) {
	task, err := task.FindByIdExecution(ctx, utility.FromStringPtr(obj.TaskRuntimeInfo.RunningTaskID), obj.TaskRuntimeInfo.RunningTaskExecution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", utility.FromStringPtr(obj.TaskRuntimeInfo.RunningTaskID), err.Error()))
	}
	if task == nil {
		return nil, nil
	}
	apiTask := &model.APITask{}
	err = apiTask.BuildFromService(ctx, task, &model.APITaskArgs{
		LogURL: r.sc.GetURL(),
	})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", task.Id, err.Error()))
	}
	return apiTask, nil
}

// Type is the resolver for the type field.
func (r *podResolver) Type(ctx context.Context, obj *model.APIPod) (string, error) {
	return string(obj.Type), nil
}

// Task is the resolver for the task field.
func (r *podEventLogDataResolver) Task(ctx context.Context, obj *model.PodAPIEventData) (*model.APITask, error) {
	if utility.FromStringPtr(obj.TaskID) == "" || obj.TaskExecution == nil {
		return nil, nil
	}
	return getTask(ctx, *obj.TaskID, obj.TaskExecution, r.sc.GetURL())
}

// Os is the resolver for the os field.
func (r *taskContainerCreationOptsResolver) Os(ctx context.Context, obj *model.APIPodTaskContainerCreationOptions) (string, error) {
	return string(obj.OS), nil
}

// Arch is the resolver for the arch field.
func (r *taskContainerCreationOptsResolver) Arch(ctx context.Context, obj *model.APIPodTaskContainerCreationOptions) (string, error) {
	return string(obj.Arch), nil
}

// Pod returns PodResolver implementation.
func (r *Resolver) Pod() PodResolver { return &podResolver{r} }

// PodEventLogData returns PodEventLogDataResolver implementation.
func (r *Resolver) PodEventLogData() PodEventLogDataResolver { return &podEventLogDataResolver{r} }

// TaskContainerCreationOpts returns TaskContainerCreationOptsResolver implementation.
func (r *Resolver) TaskContainerCreationOpts() TaskContainerCreationOptsResolver {
	return &taskContainerCreationOptsResolver{r}
}

type podResolver struct{ *Resolver }
type podEventLogDataResolver struct{ *Resolver }
type taskContainerCreationOptsResolver struct{ *Resolver }
