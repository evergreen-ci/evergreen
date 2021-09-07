package event

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypePod, podEventDataFactory)
}

// PodEventType represents a type of event related to a pod.
type PodEventType string

const (
	// ResourceTypePod represents a pod as a resource associated with events.
	ResourceTypePod = "POD"

	EventPodStatusChange PodEventType = "STATUS_CHANGE"
)

// podData contains information relevant to a pod event.
type podData struct {
	OldStatus string `bson:"old_status,omitempty" json:"old_status,omitempty"`
	NewStatus string `bson:"new_status,omitempty" json:"new_status,omitempty"`
}

// LogPodEvent logs an event for a pod to the event log.
func LogPodEvent(id string, kind PodEventType, data podData) {
	e := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   id,
		ResourceType: ResourceTypePod,
		EventType:    string(kind),
		Data:         data,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&e); err != nil {
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
func LogPodStatusChanged(id, oldStatus, newStatus string) {
	LogPodEvent(id, EventPodStatusChange, podData{
		OldStatus: oldStatus,
		NewStatus: newStatus,
	})
}
