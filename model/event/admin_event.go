package event

import (
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/pkg/errors"
)

const (
	ResourceTypeAdmin = "ADMIN"

	// event types
	BannerChanged  = "BANNER_CHANGED"
	ServiceChanged = "SERVICE_FLAGS_CHANGED"
)

// AdminEventData holds all potential data properties of a logged admin event
type AdminEventData struct {
	ResourceType string             `bson:"r_type" json:"resource_type"`
	OldVal       string             `bson:"old_val,omitempty" json:"old_val,omitempty"`
	NewVal       string             `bson:"new_val,omitempty" json:"new_val,omitempty"`
	OldFlags     admin.ServiceFlags `bson:"old_flags,omitempty" json:"old_flags,omitempty"`
	NewFlags     admin.ServiceFlags `bson:"new_flags,omitempty" json:"new_flags,omitempty"`
}

// IsValid checks if a given event is an event on an admin resource
func (evt AdminEventData) IsValid() bool {
	return evt.ResourceType == ResourceTypeAdmin
}

// logAdminEventBase is a helper function to log an admin event
func logAdminEventBase(eventType string, eventData AdminEventData) error {
	eventData.ResourceType = ResourceTypeAdmin
	event := Event{
		Timestamp: time.Now(),
		EventType: eventType,
		Data:      DataWrapper{eventData},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(event); err != nil {
		return errors.Wrap(err, "Error logging admin event")
	}
	return nil
}

// LogBannerChanged will log a change to the banner field
func LogBannerChanged(oldText, newText string) error {
	if oldText == newText {
		return nil
	}
	return logAdminEventBase(BannerChanged, AdminEventData{OldVal: oldText, NewVal: newText})
}

// LogServiceChanged will log a change to the service flags
func LogServiceChanged(oldFlags, newFlags admin.ServiceFlags) error {
	if reflect.DeepEqual(oldFlags, newFlags) {
		return nil
	}
	return logAdminEventBase(ServiceChanged, AdminEventData{OldFlags: oldFlags, NewFlags: newFlags})
}
