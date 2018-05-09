package event

import (
	"time"

	"github.com/mongodb/grip"
)

func init() {
	registry.AddType(ResourceTypePatch, patchEventDataFactory)
	registry.AllowSubscription(ResourceTypePatch, PatchStateChange)
}

func patchEventDataFactory() interface{} {
	return &PatchEventData{}
}

const (
	ResourceTypePatch = "PATCH"

	PatchStateChange = "STATE_CHANGE"
)

type PatchEventData struct {
	Status string `bson:"status,omitempty" json:"status,omitempty"`
}

func LogPatchStateChangeEvent(id, newStatus string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypePatch,
		EventType:    PatchStateChange,
		Data: &PatchEventData{
			Status: newStatus,
		},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Errorf("Error logging patch event: %+v", err)
	}
}
