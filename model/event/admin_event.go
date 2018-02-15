package event

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

const (
	ResourceTypeAdmin = "ADMIN"
	EventType         = "VALUE_CHANGED"
)

// AdminEventData holds all potential data properties of a logged admin event
type AdminEventData struct {
	ResourceType string                                `bson:"r_type" json:"resource_type"`
	User         string                                `bson:"user" json:"user"`
	Section      string                                `bson:"section" json:"section"`
	Changes      map[string]evergreen.ConfigDataChange `bson:"changes" json:"changes"`
}

// IsValid checks if a given event is an event on an admin resource
func (evt AdminEventData) IsValid() bool {
	return evt.ResourceType == ResourceTypeAdmin
}

func LogAdminEvent(section string, changes map[string]evergreen.ConfigDataChange, user string) error {
	if len(changes) == 0 {
		return nil
	}
	eventData := AdminEventData{
		ResourceType: ResourceTypeAdmin,
		User:         user,
		Section:      section,
		Changes:      changes,
	}
	event := Event{
		Timestamp: time.Now(),
		EventType: EventType,
		Data:      DataWrapper{eventData},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(event); err != nil {
		return errors.Wrap(err, "Error logging admin event")
	}
	return nil
}
