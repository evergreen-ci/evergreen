package event

import (
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
)

const (
	// resource type
	ResourceTypeDistro = "DISTRO"

	// event types
	EventDistroAdded    = "DISTRO_ADDED"
	EventDistroModified = "DISTRO_MODIFIED"
	EventDistroRemoved  = "DISTRO_REMOVED"
)

// DistroEventData implements EventData.
type DistroEventData struct {
	// necessary for IsValid
	ResourceType string      `bson:"r_type" json:"resource_type"`
	DistroId     string      `bson:"d_id,omitempty" json:"d_id,omitempty"`
	UserId       string      `bson:"u_id,omitempty" json:"u_id,omitempty"`
	Data         interface{} `bson:"dstr,omitempty" json:"dstr,omitempty"`
}

func (d DistroEventData) IsValid() bool {
	return d.ResourceType == ResourceTypeDistro
}

func LogDistroEvent(distroId string, eventType string, eventData DistroEventData) {
	eventData.ResourceType = ResourceTypeDistro
	event := Event{
		ResourceId: distroId,
		Timestamp:  time.Now(),
		EventType:  eventType,
		Data:       DataWrapper{eventData},
	}

	if err := NewDBEventLogger(Collection).LogEvent(event); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error logging distro event: %v", err)
	}
}

func LogDistroAdded(distroId, userId string, data interface{}) {
	LogDistroEvent(distroId, EventDistroAdded, DistroEventData{UserId: userId, Data: data})
}

func LogDistroModified(distroId, userId string, data interface{}) {
	LogDistroEvent(distroId, EventDistroModified, DistroEventData{UserId: userId, Data: data})
}

func LogDistroRemoved(distroId, userId string, data interface{}) {
	LogDistroEvent(distroId, EventDistroRemoved, DistroEventData{UserId: userId, Data: data})
}
