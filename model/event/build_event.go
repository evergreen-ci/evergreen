package event

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeBuild, buildEventDataFactory)
	registry.AllowSubscription(ResourceTypeBuild, BuildStateChange)
}

func buildEventDataFactory() interface{} {
	return &BuildEventData{}
}

const (
	ResourceTypeBuild = "BUILD"

	BuildStateChange = "STATE_CHANGE"
)

type BuildEventData struct {
	Status string `bson:"status" json:"status"`
}

func LogBuildStateChangeEvent(id, status string) {
	event := EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: id,
		EventType:  BuildStateChange,
		Data: &BuildEventData{
			Status: status,
		},
		ResourceType: ResourceTypeBuild,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": event.ResourceType,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}
