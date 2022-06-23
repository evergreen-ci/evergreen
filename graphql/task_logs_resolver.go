package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

func (r *taskLogsResolver) AgentLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var agentLogs []apimodels.LogMessage
	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.AgentLogPrefix,
		}
		// agent logs
		agentLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		agentLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, agentLogReader)
	} else {
		var err error
		// agent logs
		agentLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.AgentLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding agent logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	agentLogPointers := []*apimodels.LogMessage{}

	for i := range agentLogs {
		agentLogPointers = append(agentLogPointers, &agentLogs[i])
	}
	return agentLogPointers, nil
}

func (r *taskLogsResolver) AllLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var allLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {

		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.AllTaskLevelLogs,
		}

		// all logs
		allLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}

		allLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, allLogReader)

	} else {
		var err error
		// all logs
		allLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{}, []string{})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding all logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	allLogPointers := []*apimodels.LogMessage{}
	for i := range allLogs {
		allLogPointers = append(allLogPointers, &allLogs[i])
	}
	return allLogPointers, nil
}

func (r *taskLogsResolver) EventLogs(ctx context.Context, obj *TaskLogs) ([]*restModel.TaskAPIEventLogEntry, error) {
	const logMessageCount = 100
	var loggedEvents []event.EventLogEntry
	// loggedEvents is ordered ts descending
	loggedEvents, err := event.Find(event.AllLogCollection, event.MostRecentTaskEvents(obj.TaskID, logMessageCount))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find EventLogs for task %s: %s", obj.TaskID, err.Error()))
	}

	// TODO (EVG-16969) remove once TaskScheduled events TTL
	// remove all scheduled events except the youngest and push to filteredEvents
	filteredEvents := []event.EventLogEntry{}
	foundScheduled := false
	for i := 0; i < len(loggedEvents); i++ {
		if !foundScheduled || loggedEvents[i].EventType != event.TaskScheduled {
			filteredEvents = append(filteredEvents, loggedEvents[i])
		}
		if loggedEvents[i].EventType == event.TaskScheduled {
			foundScheduled = true
		}
	}

	// reverse order so it is ascending
	for i := len(filteredEvents)/2 - 1; i >= 0; i-- {
		opp := len(filteredEvents) - 1 - i
		filteredEvents[i], filteredEvents[opp] = filteredEvents[opp], filteredEvents[i]
	}

	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.TaskAPIEventLogEntry{}
	for _, e := range filteredEvents {
		apiEventLog := restModel.TaskAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	return apiEventLogPointers, nil
}

func (r *taskLogsResolver) SystemLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var systemLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}

		// system logs
		opts.LogType = apimodels.SystemLogPrefix
		systemLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		systemLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, systemLogReader)
	} else {
		var err error

		systemLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.SystemLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding system logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}
	systemLogPointers := []*apimodels.LogMessage{}
	for i := range systemLogs {
		systemLogPointers = append(systemLogPointers, &systemLogs[i])
	}

	return systemLogPointers, nil
}

func (r *taskLogsResolver) TaskLogs(ctx context.Context, obj *TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var taskLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {

		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}
		// task logs
		taskLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)

		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Encountered an error while fetching build logger logs: %s", err.Error()))
		}

		taskLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, taskLogReader)

	} else {
		var err error

		// task logs
		taskLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.TaskLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	taskLogPointers := []*apimodels.LogMessage{}
	for i := range taskLogs {
		taskLogPointers = append(taskLogPointers, &taskLogs[i])
	}

	return taskLogPointers, nil
}

// TaskLogs returns TaskLogsResolver implementation.
func (r *Resolver) TaskLogs() TaskLogsResolver { return &taskLogsResolver{r} }

type taskLogsResolver struct{ *Resolver }
