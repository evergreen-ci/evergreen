package event

import (
	"context"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeTask, func() interface{} { return &TaskEventData{} })
	registry.AllowSubscription(ResourceTypeTask, TaskStarted)
	registry.AllowSubscription(ResourceTypeTask, TaskFinished)
	registry.AllowSubscription(ResourceTypeTask, TaskBlocked)
}

const (
	// resource type
	ResourceTypeTask = "TASK"

	// event types
	TaskCreated                = "TASK_CREATED"
	TaskDispatched             = "TASK_DISPATCHED"
	TaskUndispatched           = "TASK_UNDISPATCHED"
	TaskStarted                = "TASK_STARTED"
	TaskFinished               = "TASK_FINISHED"
	TaskBlocked                = "TASK_BLOCKED"
	TaskRestarted              = "TASK_RESTARTED"
	TaskActivated              = "TASK_ACTIVATED"
	TaskDeactivated            = "TASK_DEACTIVATED"
	TaskAbortRequest           = "TASK_ABORT_REQUEST"
	ContainerAllocated         = "CONTAINER_ALLOCATED"
	TaskPriorityChanged        = "TASK_PRIORITY_CHANGED"
	TaskJiraAlertCreated       = "TASK_JIRA_ALERT_CREATED"
	TaskDependenciesOverridden = "TASK_DEPENDENCIES_OVERRIDDEN"
	MergeTaskUnscheduled       = "MERGE_TASK_UNSCHEDULED"
)

// implements Data
type TaskEventData struct {
	Execution int    `bson:"execution" json:"execution"`
	HostId    string `bson:"h_id,omitempty" json:"host_id,omitempty"`
	PodID     string `bson:"pod_id,omitempty" json:"pod_id,omitempty"`
	UserId    string `bson:"u_id,omitempty" json:"user_id,omitempty"`
	Status    string `bson:"s,omitempty" json:"status,omitempty"`
	JiraIssue string `bson:"jira,omitempty" json:"jira,omitempty"`
	BlockedOn string `bson:"blocked_on,omitempty" json:"blocked_on,omitempty"`

	Timestamp time.Time `bson:"ts,omitempty" json:"timestamp,omitempty"`
	Priority  int64     `bson:"pri,omitempty" json:"priority,omitempty"`
}

func logTaskEvent(taskId string, eventType string, eventData TaskEventData) {
	event := getTaskEvent(taskId, eventType, eventData)
	grip.Error(message.WrapError(event.Log(), message.Fields{
		"resource_type": ResourceTypeTask,
		"message":       "error logging event",
		"source":        "event-log-fail",
	}))
}

func getTaskEvent(taskId string, eventType string, eventData TaskEventData) EventLogEntry {
	return EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   taskId,
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeTask,
	}
}

func logManyTaskEvents(taskIds []string, eventType string, eventData TaskEventData) {
	if len(taskIds) == 0 {
		grip.Error(message.Fields{
			"message":    "logManyTaskEvents cannot be called with no task IDs",
			"task_ids":   taskIds,
			"event_data": eventData,
			"event_type": eventType,
		})
		return
	}
	events := []EventLogEntry{}
	now := time.Now()
	for _, id := range taskIds {
		event := EventLogEntry{
			Timestamp:    now,
			ResourceId:   id,
			EventType:    eventType,
			Data:         eventData,
			ResourceType: ResourceTypeTask,
		}
		events = append(events, event)
	}
	if err := LogManyEvents(events); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeTask,
			"message":       "error logging events",
			"source":        "event-log-fail",
		}))
	}
}

func LogJiraIssueCreated(taskId string, execution int, jiraIssue string) {
	logTaskEvent(taskId, TaskJiraAlertCreated, TaskEventData{Execution: execution, JiraIssue: jiraIssue})
}

func LogTaskPriority(taskId string, execution int, userId string, priority int64) {
	logTaskEvent(taskId, TaskPriorityChanged, TaskEventData{Execution: execution, UserId: userId, Priority: priority})
}

func LogTaskCreated(taskId string, execution int) {
	logTaskEvent(taskId, TaskCreated, TaskEventData{Execution: execution})
}

// LogHostTaskDispatched logs an event for a host task being dispatched.
func LogHostTaskDispatched(taskId string, execution int, hostId string) {
	logTaskEvent(taskId, TaskDispatched, TaskEventData{Execution: execution, HostId: hostId})
}

// LogContainerTaskDispatched logs an event for a container task being
// dispatched to a pod.
func LogContainerTaskDispatched(taskID string, execution int, podID string) {
	logTaskEvent(taskID, TaskDispatched, TaskEventData{Execution: execution, PodID: podID})
}

// LogHostTaskUndispatched logs an event for a host being marked undispatched.
func LogHostTaskUndispatched(taskId string, execution int, hostId string) {
	logTaskEvent(taskId, TaskUndispatched, TaskEventData{Execution: execution, HostId: hostId})
}

func LogTaskStarted(taskId string, execution int) {
	logTaskEvent(taskId, TaskStarted, TaskEventData{Execution: execution, Status: evergreen.TaskStarted})
}

// LogTaskFinished logs an event indicating that the task has finished.
func LogTaskFinished(taskId string, execution int, status string) {
	logTaskEvent(taskId, TaskFinished, TaskEventData{Execution: execution, Status: status})
}

// LogHostTaskFinished logs an event for a host task being marked finished. If
// it was assigned to run on a host, it logs an additional host event indicating
// that its assigned task has finished.
func LogHostTaskFinished(taskId string, execution int, hostId, status string) {
	LogTaskFinished(taskId, execution, status)
	if hostId != "" {
		LogHostEvent(hostId, EventHostTaskFinished, HostEventData{Execution: strconv.Itoa(execution), TaskStatus: status, TaskId: taskId})
	}
}

// LogContainerTaskFinished logs an event for a container task being marked
// finished. If it was assigned to run on a pod, it logs an additional pod event
// indicating that its assigned task has finished.
func LogContainerTaskFinished(taskID string, execution int, podID, status string) {
	LogTaskFinished(taskID, execution, status)
	if podID != "" {
		LogPodEvent(podID, EventPodFinishedTask, PodData{TaskExecution: execution, TaskStatus: status, TaskID: taskID})
	}
}

// LogTaskRestarted updates the DB with a task restarted event.
func LogTaskRestarted(taskId string, execution int, userId string) {
	logTaskEvent(taskId, TaskRestarted, TaskEventData{Execution: execution, UserId: userId})
}

// TaskBlockedData is event data for logging a single task blocked event.
type TaskBlockedData struct {
	ID        string `bson:"-" json:"-"`
	Execution int    `bson:"-" json:"-"`
	BlockedOn string `bson:"-" json:"-"`
}

// LogManyTasksBlocked logs many task blocked events.
func LogManyTasksBlocked(ctx context.Context, data []TaskBlockedData) {
	if len(data) == 0 {
		return
	}

	events := make([]EventLogEntry, 0, len(data))
	now := time.Now()
	for _, d := range data {
		e := EventLogEntry{
			Timestamp:    now,
			ResourceId:   d.ID,
			ResourceType: ResourceTypeTask,
			EventType:    TaskBlocked,
			Data: TaskEventData{
				Execution: d.Execution,
				BlockedOn: d.BlockedOn,
			},
		}
		events = append(events, e)
	}
	if err := LogManyUnorderedEventsWithContext(ctx, events); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeTask,
			"event_type":    TaskBlocked,
			"message":       "error logging events",
			"source":        "event-log-fail",
		}))
	}
}

// LogTaskBlocked updates the DB with a task blocked event.

// LogTaskActivated updates the DB with a task activated event.
func LogTaskActivated(taskId string, execution int, userId string) {
	logTaskEvent(taskId, TaskActivated, TaskEventData{Execution: execution, UserId: userId})
}

// GetTaskActivatedEvent retrieves the task activated event.
func GetTaskActivatedEvent(taskId string, execution int, userId string) EventLogEntry {
	return getTaskEvent(taskId, TaskActivated, TaskEventData{Execution: execution, UserId: userId})
}

// GetTaskDeactivatedEvent retrieves the task deactivated event.
func GetTaskDeactivatedEvent(taskId string, execution int, userId string) EventLogEntry {
	return getTaskEvent(taskId, TaskDeactivated, TaskEventData{Execution: execution, UserId: userId})
}

// LogTaskAbortRequest updates the DB with a task abort request event.
func LogTaskAbortRequest(taskId string, execution int, userId string) {
	logTaskEvent(taskId, TaskAbortRequest,
		TaskEventData{Execution: execution, UserId: userId})
}

// LogManyTaskAbortRequests updates the DB with task abort request events.
func LogManyTaskAbortRequests(taskIds []string, userId string) {
	logManyTaskEvents(taskIds, TaskAbortRequest,
		TaskEventData{UserId: userId})
}

// LogManyTaskPriority updates the DB with a task started events.
func LogManyTaskPriority(taskIds []string, userId string, priority int64) {
	logManyTaskEvents(taskIds, TaskPriorityChanged,
		TaskEventData{UserId: userId, Priority: priority})
}

// LogTaskContainerAllocated updates the DB with a container allocated event.
func LogTaskContainerAllocated(taskId string, execution int, containerAllocatedTime time.Time) {
	logTaskEvent(taskId, ContainerAllocated,
		TaskEventData{Execution: execution, Timestamp: containerAllocatedTime})
}

// LogTaskDependenciesOverridden updates the DB with a task dependencies overridden event.
func LogTaskDependenciesOverridden(taskId string, execution int, userID string) {
	logTaskEvent(taskId, TaskDependenciesOverridden,
		TaskEventData{Execution: execution, UserId: userID})
}

// LogMergeTaskUnscheduled updates the DB with a merge task unscheduled event.
func LogMergeTaskUnscheduled(taskId string, execution int, userID string) {
	logTaskEvent(taskId, MergeTaskUnscheduled,
		TaskEventData{Execution: execution, UserId: userID})
}
