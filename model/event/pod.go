package event

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypePod, func() interface{} { return &PodData{} })
}

// PodEventType represents a type of event related to a pod.
type PodEventType string

const (
	// ResourceTypePod represents a pod as a resource associated with events.
	ResourceTypePod = "POD"

	// EventPodStatusChange represents an event where a pod's status is
	// modified.
	EventPodStatusChange PodEventType = "STATUS_CHANGE"
	// EventPodAssignedTask represents an event where a pod is assigned a task
	// to run.
	EventPodAssignedTask PodEventType = "ASSIGNED_TASK"
	// EventPodClearedTask represents an event where a pod's current running
	// task is cleared.
	EventPodClearedTask PodEventType = "CLEARED_TASK"
	// EventPodFinishedTask represents an event where a pod's assigned task has
	// finished running.
	EventPodFinishedTask PodEventType = "CONTAINER_TASK_FINISHED"
)

// PodData contains information relevant to a pod event.
type PodData struct {
	OldStatus string `bson:"old_status,omitempty" json:"old_status,omitempty"`
	NewStatus string `bson:"new_status,omitempty" json:"new_status,omitempty"`
	Reason    string `bson:"reason,omitempty" json:"reason,omitempty"`

	// Fields related to pods running tasks
	TaskID        string `bson:"task_id,omitempty" json:"task_id,omitempty"`
	TaskExecution int    `bson:"task_execution,omitempty" json:"task_execution,omitempty"`
	TaskStatus    string `bson:"task_status,omitempty" json:"task_status,omitempty"`
}

// LogPodEvent logs an event for a pod to the event log.
func LogPodEvent(id string, kind PodEventType, data PodData) {
	e := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   id,
		ResourceType: ResourceTypePod,
		EventType:    string(kind),
		Data:         data,
	}

	if err := e.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "failed to log pod event",
			"pod":        id,
			"event_type": kind,
			"data":       data,
			"source":     "event-log-fail",
		}))
	}
}

// LogPodStatusChanged logs an event indicating that the pod's status has been
// updated.
func LogPodStatusChanged(id, oldStatus, newStatus, reason string) {
	LogPodEvent(id, EventPodStatusChange, PodData{
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Reason:    reason,
	})
}

// LogPodAssignedTask logs an event indicating that the pod has been assigned a
// task to run.
func LogPodAssignedTask(id, taskID string, execution int) {
	LogPodEvent(id, EventPodAssignedTask, PodData{TaskID: taskID, TaskExecution: execution})
}

// LogPodRunningTaskCleared logs an event indicating that the pod's current
// running task has been cleared, so it is no longer assigned to run the task.
func LogPodRunningTaskCleared(id, taskID string, execution int) {
	LogPodEvent(id, EventPodClearedTask, PodData{TaskID: taskID, TaskExecution: execution})
}
