package model

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	"time"
)

const (
	// db constants
	EventLogCollection = "event_log"
)

type Event struct {
	Timestamp  time.Time        `bson:"ts" json:"timestamp"`
	ResourceId string           `bson:"r_id" json:"resource_id"`
	EventType  string           `bson:"e_type" json:"event_type"`
	Data       EventDataWrapper `bson:"data" json:"data"`
}

var (
	// bson fields for the event struct
	EventTimestampKey  = MustHaveBsonTag(Event{}, "Timestamp")
	EventResourceIdKey = MustHaveBsonTag(Event{}, "ResourceId")
	EventTypeKey       = MustHaveBsonTag(Event{}, "EventType")
	EventDataKey       = MustHaveBsonTag(Event{}, "Data")

	// resource type key.  this doesn't exist here, but has to be the same
	// for all of the event types
	EventDataResourceTypeKey = MustHaveBsonTag(HostEventData{},
		"ResourceType")
)

type EventDataWrapper struct {
	EventData
}

type EventData interface {
	IsValid() bool
}

func (self EventDataWrapper) GetBSON() (interface{}, error) {
	return self.EventData, nil
}

func (self *EventDataWrapper) SetBSON(raw bson.Raw) error {
	for _, impl := range []interface{}{&TaskEventData{}, &HostEventData{}} {
		err := raw.Unmarshal(impl)
		if err != nil {
			return err
		}
		if impl.(EventData).IsValid() {
			self.EventData = impl.(EventData)
			return nil
		}
	}
	m := bson.M{}
	err := raw.Unmarshal(m)
	if err != nil {
		return err
	}
	return fmt.Errorf("No suitable type for %#v", m)
}
