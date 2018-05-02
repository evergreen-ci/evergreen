package event

import (
	"time"

	"github.com/mongodb/grip"
)

func init() {
	registry.AddType(ResourceTypeBuild, buildEventFactory)
}

const (
	ResourceTypeBuild = "BUILD"

	BuildStateChange = "STATE_CHANGE"
)

type BuildEventData struct {
	Status string `bson:"status" json:"string"`
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
		grip.Errorf("Error logging host event: %+v", err)
	}
}
