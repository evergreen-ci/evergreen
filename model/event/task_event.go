package event

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"time"
)

const (
	// resource type
	ResourceTypeTask = "TASK"

	// event types
	TaskCreated      = "TASK_CREATED"
	TaskDispatched   = "TASK_DISPATCHED"
	TaskStarted      = "TASK_STARTED"
	TaskFinished     = "TASK_FINISHED"
	TaskRestarted    = "TASK_RESTARTED"
	TaskActivated    = "TASK_ACTIVATED"
	TaskDeactivated  = "TASK_DEACTIVATED"
	TaskAbortRequest = "TASK_ABORT_REQUEST"
	TaskScheduled    = "TASK_SCHEDULED"
)

// implements Data
type TaskEventData struct {
	// necessary for IsValid
	ResourceType string    `bson:"r_type" json:"resource_type"`
	HostId       string    `bson:"h_id,omitempty" json:"host_id,omitempty"`
	UserId       string    `bson:"u_id,omitempty" json:"user_id,omitempty"`
	Status       string    `bson:"s,omitempty" json:"status,omitempty"`
	Timestamp    time.Time `bson:"ts,omitempty" json:"timestamp,omitempty"`
}

func (self TaskEventData) IsValid() bool {
	return self.ResourceType == ResourceTypeTask
}

func LogTaskEvent(taskId string, eventType string, eventData TaskEventData) {
	eventData.ResourceType = ResourceTypeTask
	event := Event{
		Timestamp:  time.Now(),
		ResourceId: taskId,
		EventType:  eventType,
		Data:       DataWrapper{eventData},
	}

	logger := NewDBEventLogger(Collection)
	if err := logger.LogEvent(event); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error logging task event: %v", err)
	}
}

func LogTaskCreated(taskId string) {
	LogTaskEvent(taskId, TaskCreated, TaskEventData{})
}

func LogTaskDispatched(taskId, hostId string) {
	LogTaskEvent(taskId, TaskDispatched, TaskEventData{HostId: hostId})
}

func LogTaskStarted(taskId string) {
	LogTaskEvent(taskId, TaskStarted, TaskEventData{})
}

func LogTaskFinished(taskId string, status string) {
	LogTaskEvent(taskId, TaskFinished, TaskEventData{Status: status})
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
