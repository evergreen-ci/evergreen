package event

import (
	"time"

	"github.com/mongodb/grip"
)

func init() {
	registry.AddType(ResourceTypeTask, taskEventDataFactory)
}

const (
	// resource type
	ResourceTypeTask = "TASK"

	// event types
	TaskCreated          = "TASK_CREATED"
	TaskDispatched       = "TASK_DISPATCHED"
	TaskUndispatched     = "TASK_UNDISPATCHED"
	TaskStarted          = "TASK_STARTED"
	TaskFinished         = "TASK_FINISHED"
	TaskRestarted        = "TASK_RESTARTED"
	TaskActivated        = "TASK_ACTIVATED"
	TaskDeactivated      = "TASK_DEACTIVATED"
	TaskAbortRequest     = "TASK_ABORT_REQUEST"
	TaskScheduled        = "TASK_SCHEDULED"
	TaskPriorityChanged  = "TASK_PRIORITY_CHANGED"
	TaskJiraAlertCreated = "TASK_JIRA_ALERT_CREATED"
)

// implements Data
type TaskEventData struct {
	HostId    string `bson:"h_id,omitempty" json:"host_id,omitempty"`
	UserId    string `bson:"u_id,omitempty" json:"user_id,omitempty"`
	Status    string `bson:"s,omitempty" json:"status,omitempty"`
	JiraIssue string `bson:"jira,omitempty" json:"jira,omitempty"`

	Timestamp time.Time `bson:"ts,omitempty" json:"timestamp,omitempty"`
	Priority  int64     `bson:"pri,omitempty" json:"priority,omitempty"`
}

func LogTaskEvent(taskId string, eventType string, eventData TaskEventData) {
	event := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   taskId,
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeTask,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Errorf("Error logging task event: %+v", err)
	}
}

func LogManyTaskEvents(taskIds []string, eventType string, eventData TaskEventData) {
	if len(taskIds) == 0 {
		grip.Error("LogManyTaskEvents called with empty taskIds")
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
		grip.Errorf("Error logging task events: %s", err)
	}
}

func LogJiraIssueCreated(taskId, jiraIssue string) {
	LogTaskEvent(taskId, TaskJiraAlertCreated, TaskEventData{JiraIssue: jiraIssue})
}

func LogTaskPriority(taskId, user string, priority int64) {
	LogTaskEvent(taskId, TaskPriorityChanged, TaskEventData{UserId: user, Priority: priority})
}

func LogTaskCreated(taskId string) {
	LogTaskEvent(taskId, TaskCreated, TaskEventData{})
}

func LogTaskDispatched(taskId, hostId string) {
	LogTaskEvent(taskId, TaskDispatched, TaskEventData{HostId: hostId})
}

func LogTaskUndispatched(taskId, hostId string) {
	LogTaskEvent(taskId, TaskUndispatched, TaskEventData{HostId: hostId})
}

func LogTaskStarted(taskId string) {
	LogTaskEvent(taskId, TaskStarted, TaskEventData{})
}

func LogTaskFinished(taskId string, hostId, status string) {
	LogTaskEvent(taskId, TaskFinished, TaskEventData{Status: status})
	LogHostEvent(hostId, EventTaskFinished, HostEventData{TaskStatus: status, TaskId: taskId})
}

func LogTaskRestarted(taskId string, userId string) {
	LogTaskEvent(taskId, TaskRestarted, TaskEventData{UserId: userId})
}

func LogTaskActivated(taskId string, userId string) {
	LogTaskEvent(taskId, TaskActivated, TaskEventData{UserId: userId})
}

func LogTaskDeactivated(taskId string, userId string) {
	LogTaskEvent(taskId, TaskDeactivated, TaskEventData{UserId: userId})
}

func LogTaskAbortRequest(taskId string, userId string) {
	LogTaskEvent(taskId, TaskAbortRequest,
		TaskEventData{UserId: userId})
}

func LogManyTaskAbortRequests(taskIds []string, userId string) {
	LogManyTaskEvents(taskIds, TaskAbortRequest,
		TaskEventData{UserId: userId})
}

func LogTaskScheduled(taskId string, scheduledTime time.Time) {
	LogTaskEvent(taskId, TaskScheduled,
		TaskEventData{Timestamp: scheduledTime})
}
