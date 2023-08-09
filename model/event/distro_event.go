package event

import (
	"reflect"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeDistro, func() interface{} { return &DistroEventData{} })
	registry.setUnexpirable(ResourceTypeDistro, EventDistroAdded)
	registry.setUnexpirable(ResourceTypeDistro, EventDistroModified)
	registry.setUnexpirable(ResourceTypeDistro, EventDistroAMIModfied)
	registry.setUnexpirable(ResourceTypeDistro, EventDistroRemoved)
}

const (
	// resource type
	ResourceTypeDistro = "DISTRO"

	// event types
	EventDistroAdded      = "DISTRO_ADDED"
	EventDistroModified   = "DISTRO_MODIFIED"
	EventDistroAMIModfied = "DISTRO_AMI_MODIFIED"
	EventDistroRemoved    = "DISTRO_REMOVED"
)

// DistroEventData implements EventData.
type DistroEventData struct {
	DistroId string      `bson:"d_id,omitempty" json:"d_id,omitempty"`
	UserId   string      `bson:"u_id,omitempty" json:"u_id,omitempty"`
	Data     interface{} `bson:"dstr,omitempty" json:"dstr,omitempty"`
	User     string      `bson:"user,omitempty" json:"user,omitempty"`
	Before   interface{} `bson:"before" json:"before"`
	After    interface{} `bson:"after" json:"after"`
}

func LogDistroEvent(distroId string, eventType string, eventData DistroEventData) {
	event := EventLogEntry{
		ResourceId:   distroId,
		Timestamp:    time.Now(),
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeDistro,
	}

	if err := event.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeDistro,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

// LogDistroAdded should take in DistroData in order to preserve the ProviderSettingsList
func LogDistroAdded(distroId, userId string, data interface{}) {
	LogDistroEvent(distroId, EventDistroAdded, DistroEventData{UserId: userId, Data: data})
}

// LogDistroModified should take in DistroData in order to preserve the ProviderSettingsList
func LogDistroModified(distroId, userId string, before, after interface{}) {
	// Stop if there are no changes
	if reflect.DeepEqual(before, after) {
		return
	}

	data := DistroEventData{
		UserId: userId,
		User:   userId,
		Data:   after,
		Before: before,
		After:  after,
	}

	LogDistroEvent(distroId, EventDistroModified, data)
}

// LogDistroRemoved should take in DistroData in order to preserve the ProviderSettingsList
func LogDistroRemoved(distroId, userId string, data interface{}) {
	LogDistroEvent(distroId, EventDistroRemoved, DistroEventData{UserId: userId, Data: data})
}

// LogDistroAMIModified logs when the default region's AMI is modified.
func LogDistroAMIModified(distroId, userId string) {
	LogDistroEvent(distroId, EventDistroAMIModfied, DistroEventData{UserId: userId})
}
