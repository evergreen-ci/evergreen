package event

import (
	"time"

	"github.com/mongodb/grip"
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
	ResourceType string      `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	DistroId     string      `bson:"d_id,omitempty" json:"d_id,omitempty"`
	UserId       string      `bson:"u_id,omitempty" json:"u_id,omitempty"`
	Data         interface{} `bson:"dstr,omitempty" json:"dstr,omitempty"`
}

func (d DistroEventData) IsValid() bool {
	return d.ResourceType == ResourceTypeDistro
}

func LogDistroEvent(distroId string, eventType string, eventData DistroEventData) {
	eventData.ResourceType = ResourceTypeDistro
	event := EventLogEntry{
		ResourceId: distroId,
		Timestamp:  time.Now(),
		EventType:  eventType,
		Data:       eventData,
	}

	if err := NewDBEventLogger(AllLogCollection).LogEvent(&event); err != nil {
		grip.Errorf("Error logging distro event: %+v", err)
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
