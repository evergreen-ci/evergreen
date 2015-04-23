package model

import (
	"10gen.com/mci"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

const (
	// resource type
	ResourceTypeTask = "TASK"

	// event types
	EventTaskCreated      = "TASK_CREATED"
	EventTaskDispatched   = "TASK_DISPATCHED"
	EventTaskStarted      = "TASK_STARTED"
	EventTaskFinished     = "TASK_FINISHED"
	EventTaskRestarted    = "TASK_RESTARTED"
	EventTaskActivated    = "TASK_ACTIVATED"
	EventTaskDeactivated  = "TASK_DEACTIVATED"
	EventTaskAbortRequest = "TASK_ABORT_REQUEST"
	EventTaskScheduled    = "TASK_SCHEDULED"
)

// implements EventData
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

func NewTaskEventFinder() *EventFinder {
	return &EventFinder{resourceType: ResourceTypeTask}
}

func FindMostRecentTaskEvents(taskId string, n int) ([]Event, error) {
	return NewTaskEventFinder().FindMostRecentEvents(taskId, n)
}

func FindAllTaskEventsInOrder(taskId string) ([]Event, error) {
	return NewTaskEventFinder().FindAllEventsInOrder(taskId)
}

func LogTaskEvent(taskId string, eventType string, eventData TaskEventData) {
	eventData.ResourceType = ResourceTypeTask
	event := Event{
		Timestamp:  time.Now(),
		ResourceId: taskId,
		EventType:  eventType,
		Data:       EventDataWrapper{eventData},
	}

	logger := NewDBEventLogger(EventLogCollection)
	if err := logger.LogEvent(event); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error logging task event: %v", err)
	}
}

func LogTaskCreatedEvent(taskId string) {
	LogTaskEvent(taskId, EventTaskCreated, TaskEventData{})
}

func LogTaskDispatchedEvent(taskId, hostId string) {
	LogTaskEvent(taskId, EventTaskDispatched, TaskEventData{HostId: hostId})
}

func LogTaskStartedEvent(taskId string) {
	LogTaskEvent(taskId, EventTaskStarted, TaskEventData{})
}

func LogTaskFinishedEvent(taskId string, status string) {
	LogTaskEvent(taskId, EventTaskFinished, TaskEventData{Status: status})
}

func LogTaskRestartedEvent(taskId string, userId string) {
	LogTaskEvent(taskId, EventTaskRestarted, TaskEventData{UserId: userId})
}

func LogTaskActivatedEvent(taskId string, userId string) {
	LogTaskEvent(taskId, EventTaskActivated, TaskEventData{UserId: userId})
}

func LogTaskDeactivatedEvent(taskId string, userId string) {
	LogTaskEvent(taskId, EventTaskDeactivated, TaskEventData{UserId: userId})
}

func LogTaskAbortRequestEvent(taskId string, userId string) {
	LogTaskEvent(taskId, EventTaskAbortRequest,
		TaskEventData{UserId: userId})
}

func LogTaskScheduledEvent(taskId string, scheduledTime time.Time) {
	LogTaskEvent(taskId, EventTaskScheduled,
		TaskEventData{Timestamp: scheduledTime})
}
