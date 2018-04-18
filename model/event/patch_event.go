package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
)

func init() {
	registry.AddType(ResourceTypePatch, patchEventFactory)
	registry.AllowSubscription(ResourceTypePatch, PatchOutcomeEvent)
	registry.RegisterExtraData(ResourceTypePatch, "time-exceeds-n-constant", 0)
	registry.RegisterExtraData(ResourceTypePatch, "time-exceeds-n-percent", 0.0)
}

func patchEventFactory() interface{} {
	return &PatchEventData{}
}

const (
	ResourceTypePatch = "PATCH"

	PatchOutcomeEvent = "OUTCOME"
)

type PatchEventData struct {
	Author   string        `bson:"author" json:"author"`
	Version  string        `bson:"version" json:"version"`
	Status   string        `bson:"status,omitempty" json:"status,omitempty"`
	Duration time.Duration `bson:"duration" json:"duration"`
}

func LogPatchStateChangeEvent(p *patch.Patch) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   p.Id.Hex(),
		ResourceType: ResourceTypePatch,
		EventType:    PatchOutcomeEvent,
		Data: &PatchEventData{
			Author:   p.Author,
			Version:  p.Version,
			Status:   p.Status,
			Duration: p.FinishTime.Sub(p.StartTime),
		},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Errorf("Error logging patch event: %+v", err)
	}
}
