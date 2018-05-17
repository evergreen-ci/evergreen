package event

import (
	"time"

	"github.com/mongodb/grip"
)

func init() {
	registry.AddType(ResourceTypeVersion, versionEventDataFactory)
	registry.AllowSubscription(ResourceTypeVersion, VersionStateChange)
}

func versionEventDataFactory() interface{} {
	return &VersionEventData{}
}

const (
	ResourceTypeVersion = "VERSION"
	VersionStateChange  = "STATE_CHANGE"
)

type VersionEventData struct {
	Status string `bson:"status,omitempty" json:"status,omitempty"`
}

func LogVersionStateChangeEvent(id, newStatus string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypeVersion,
		EventType:    VersionStateChange,
		Data: &VersionEventData{
			Status: newStatus,
		},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Errorf("Error logging version event: %+v", err)
	}
}
