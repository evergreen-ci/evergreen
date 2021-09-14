package event

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeTask, taskEventDataFactory)
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
	TaskScheduled              = "TASK_SCHEDULED"
	TaskPriorityChanged        = "TASK_PRIORITY_CHANGED"
	TaskJiraAlertCreated       = "TASK_JIRA_ALERT_CREATED"
	TaskDependenciesOverridden = "TASK_DEPENDENCIES_OVERRIDDEN"
	MergeTaskUnscheduled       = "MERGE_TASK_UNSCHEDULED"
)

// implements Data
type TaskEventData struct {
	Execution int    `bson:"execution" json:"execution"`
	HostId    string `bson:"h_id,omitempty" json:"host_id,omitempty"`
	UserId    string `bson:"u_id,omitempty" json:"user_id,omitempty"`
	Status    string `bson:"s,omitempty" json:"status,omitempty"`
	JiraIssue string `bson:"jira,omitempty" json:"jira,omitempty"`

	Timestamp time.Time `bson:"ts,omitempty" json:"timestamp,omitempty"`
	Priority  int64     `bson:"pri,omitempty" json:"priority,omitempty"`
}

func logTaskEvent(taskId string, eventType string, eventData TaskEventData) {
	event := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   taskId,
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeTask,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeTask,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func logManyTaskEvents(taskIds []string, eventType string, eventData TaskEventData) {
	if len(taskIds) == 0 {
		grip.Error("logManyTaskEvents called with empty taskIds")
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
	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogManyEvents(events); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeTask,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func LogJiraIssueCreated(taskId string, execution int, jiraIssue string) {
	logTaskEvent(taskId, TaskJiraAlertCreated, TaskEventData{Execution: execution, JiraIssue: jiraIssue})
}

func LogTaskPriority(taskId string, execution int, user string, priority int64) {
	logTaskEvent(taskId, TaskPriorityChanged, TaskEventData{Execution: execution, UserId: user, Priority: priority})
}

func LogTaskCreated(taskId string, execution int) {
	logTaskEvent(taskId, TaskCreated, TaskEventData{Execution: execution})
}

func LogTaskDispatched(taskId string, execution int, hostId string) {
	logTaskEvent(taskId, TaskDispatched, TaskEventData{Execution: execution, HostId: hostId})
}

func LogTaskUndispatched(taskId string, execution int, hostId string) {
	logTaskEvent(taskId, TaskUndispatched, TaskEventData{Execution: execution, HostId: hostId})
}

func LogTaskStarted(taskId string, execution int) {
	logTaskEvent(taskId, TaskStarted, TaskEventData{Execution: execution, Status: evergreen.TaskStarted})
}

func LogTaskFinished(taskId string, execution int, hostId, status string) {
	logTaskEvent(taskId, TaskFinished, TaskEventData{Execution: execution, Status: status})
	if hostId != "" {
		LogHostEvent(hostId, EventTaskFinished, HostEventData{TaskExecution: execution, TaskStatus: status, TaskId: taskId})
	}
}

func LogTaskRestarted(taskId string, execution int, userId string) {
	logTaskEvent(taskId, TaskRestarted, TaskEventData{Execution: execution, UserId: userId})
}

func LogTaskBlocked(taskId string, execution int) {
	logTaskEvent(taskId, TaskBlocked, TaskEventData{Execution: execution})
}

func LogTaskActivated(taskId string, execution int, userId string) {
	logTaskEvent(taskId, TaskActivated, TaskEventData{Execution: execution, UserId: userId})
}

func LogTaskDeactivated(taskId string, execution int, userId string) {
	logTaskEvent(taskId, TaskDeactivated, TaskEventData{Execution: execution, UserId: userId})
}

func LogTaskAbortRequest(taskId string, execution int, userId string) {
	logTaskEvent(taskId, TaskAbortRequest,
		TaskEventData{Execution: execution, UserId: userId})
}

func LogManyTaskAbortRequests(taskIds []string, userId string) {
	logManyTaskEvents(taskIds, TaskAbortRequest,
		TaskEventData{UserId: userId})
}

func LogTaskScheduled(taskId string, execution int, scheduledTime time.Time) {
	logTaskEvent(taskId, TaskScheduled,
		TaskEventData{Execution: execution, Timestamp: scheduledTime})
}

func LogTaskDependenciesOverridden(taskId string, execution int, userID string) {
	logTaskEvent(taskId, TaskDependenciesOverridden,
		TaskEventData{Execution: execution, UserId: userID})
}

func LogMergeTaskUnscheduled(taskId string, execution int, userID string) {
	logTaskEvent(taskId, MergeTaskUnscheduled,
		TaskEventData{Execution: execution, UserId: userID})
}
