package event

import (
	"context"
	"reflect"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeDistro, func() any { return &DistroEventData{} })
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
	DistroId string `bson:"d_id,omitempty" json:"d_id"`
	User     string `bson:"user,omitempty" json:"user"`
	Before   any    `bson:"before" json:"before"`
	After    any    `bson:"after" json:"after"`

	// Fields used by legacy UI
	Data   any    `bson:"dstr,omitempty" json:"dstr"`
	UserId string `bson:"u_id,omitempty" json:"u_id"`
}

func LogDistroEvent(ctx context.Context, distroId string, eventType string, eventData DistroEventData) {
	event := EventLogEntry{
		ResourceId:   distroId,
		Timestamp:    time.Now(),
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeDistro,
	}

	if err := event.Log(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeDistro,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

// LogDistroAdded should take in DistroData in order to preserve the ProviderSettingsList
func LogDistroAdded(ctx context.Context, distroId, userId string, data any) {
	LogDistroEvent(ctx, distroId, EventDistroAdded, DistroEventData{UserId: userId, Data: data})
}

// LogDistroModified should take in DistroData in order to preserve the ProviderSettingsList
func LogDistroModified(ctx context.Context, distroId, userId string, before, after any) {
	// Stop if there are no changes
	if reflect.DeepEqual(before, after) {
		grip.Info(message.Fields{
			"message":   "no changes found when logging modified distro",
			"distro_id": distroId,
			"source":    "event-log-fail",
			"user_id":   userId,
		})
		return
	}

	data := DistroEventData{
		User:   userId,
		Before: before,
		After:  after,
	}

	LogDistroEvent(ctx, distroId, EventDistroModified, data)
}

// LogDistroRemoved should take in DistroData in order to preserve the ProviderSettingsList
func LogDistroRemoved(ctx context.Context, distroId, userId string, data any) {
	LogDistroEvent(ctx, distroId, EventDistroRemoved, DistroEventData{UserId: userId, Data: data})
}

// LogDistroAMIModified logs when the default region's AMI is modified.
func LogDistroAMIModified(ctx context.Context, distroId, userId string) {
	LogDistroEvent(ctx, distroId, EventDistroAMIModfied, DistroEventData{UserId: userId})
}
