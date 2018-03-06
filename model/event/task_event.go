package event

import (
	"time"

	"github.com/mongodb/grip"
)

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
	// necessary for IsValid
	ResourceType string `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	HostId       string `bson:"h_id,omitempty" json:"host_id,omitempty"`
	UserId       string `bson:"u_id,omitempty" json:"user_id,omitempty"`
	Status       string `bson:"s,omitempty" json:"status,omitempty"`
	JiraIssue    string `bson:"jira,omitempty" json:"jira,omitempty"`

	Timestamp time.Time `bson:"ts,omitempty" json:"timestamp,omitempty"`
	Priority  int64     `bson:"pri,omitempty" json:"priority,omitempty"`
}

func (self TaskEventData) IsValid() bool {
	return self.ResourceType == ResourceTypeTask
}

func LogTaskEvent(taskId string, eventType string, eventData TaskEventData) {
	eventData.ResourceType = ResourceTypeTask
	event := EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: taskId,
		EventType:  eventType,
		Data:       eventData,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(event); err != nil {
		grip.Errorf("Error logging task event: %+v", err)
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

func LogTaskScheduled(taskId string, scheduledTime time.Time) {
	LogTaskEvent(taskId, TaskScheduled,
		TaskEventData{Timestamp: scheduledTime})
}
