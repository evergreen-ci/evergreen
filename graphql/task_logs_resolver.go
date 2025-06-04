package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

// AgentLogs is the resolver for the agentLogs field.
func (r *taskLogsResolver) AgentLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	return getTaskLogs(ctx, obj, task.TaskLogTypeAgent)
}

// AllLogs is the resolver for the allLogs field.
func (r *taskLogsResolver) AllLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	return getTaskLogs(ctx, obj, task.TaskLogTypeAll)
}

// EventLogs is the resolver for the eventLogs field.
func (r *taskLogsResolver) EventLogs(ctx context.Context, obj *TaskLogs) ([]*restModel.TaskAPIEventLogEntry, error) {
	const logMessageCount = 100
	// loggedEvents is ordered ts descending
	loggedEvents, err := event.Find(ctx, event.MostRecentTaskEvents(obj.TaskID, logMessageCount))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching EventLogs for task '%s': %s", obj.TaskID, err.Error()))
	}

	// reverse order so it is ascending
	for i := len(loggedEvents)/2 - 1; i >= 0; i-- {
		opp := len(loggedEvents) - 1 - i
		loggedEvents[i], loggedEvents[opp] = loggedEvents[opp], loggedEvents[i]
	}

	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.TaskAPIEventLogEntry{}
	for _, e := range loggedEvents {
		apiEventLog := restModel.TaskAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(ctx, e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("building APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	return apiEventLogPointers, nil
}

// SystemLogs is the resolver for the systemLogs field.
func (r *taskLogsResolver) SystemLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	return getTaskLogs(ctx, obj, task.TaskLogTypeSystem)
}

// TaskLogs is the resolver for the taskLogs field.
func (r *taskLogsResolver) TaskLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	return getTaskLogs(ctx, obj, task.TaskLogTypeTask)
}

// TaskLogs returns TaskLogsResolver implementation.
func (r *Resolver) TaskLogs() TaskLogsResolver { return &taskLogsResolver{r} }

type taskLogsResolver struct{ *Resolver }
