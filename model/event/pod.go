package event

import (
	"time"

	"github.com/pkg/errors"
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
func LogPodEvent(id string, kind PodEventType, data podData) error {
	e := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   id,
		ResourceType: ResourceTypePod,
		EventType:    string(kind),
		Data:         data,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&e); err != nil {
		return errors.Wrap(err, "logging pod event")
	}

	return nil
}

// LogPodStatusChanged logs an event indicating that the pod's status has been
// updated.
func LogPodStatusChanged(id, oldStatus, newStatus string) error {
	return LogPodEvent(id, EventPodStatusChange, podData{
		OldStatus: oldStatus,
		NewStatus: newStatus,
	})
}
