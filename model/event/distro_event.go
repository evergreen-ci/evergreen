package event

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeDistro, distroEventDataFactory)
}

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
	DistroId string      `bson:"d_id,omitempty" json:"d_id,omitempty"`
	UserId   string      `bson:"u_id,omitempty" json:"u_id,omitempty"`
	Data     interface{} `bson:"dstr,omitempty" json:"dstr,omitempty"`
}

func LogDistroEvent(distroId string, eventType string, eventData DistroEventData) {
	event := EventLogEntry{
		ResourceId:   distroId,
		Timestamp:    time.Now(),
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeDistro,
	}

	if err := NewDBEventLogger(AllLogCollection).LogEvent(&event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeDistro,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
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
