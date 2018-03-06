package event

import (
	"reflect"
	"strings"
	"time"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	// db constants
	AllLogCollection  = "event_log"
	TaskLogCollection = "task_event_log"
)

type Event interface {
	ID() string

	// return the r_type bson field
	Type() string

	// Time returns the time that the event occurred
	Time() time.Time

	// Processed is whether or not this event has been processed. An event
	// which has been processed has successfully have notifications intents
	// created and stored, but does not indicate whether or not these
	// notifications have been successfully sent to all recipients
	// If true, the time is the time that this event was marked as
	// processed. If false, time is the zero time
	Processed() (bool, time.Time)

	// MarkProcessed marks this event as processed at the current
	// time
	MarkProcessed() error

	// Notifications fetches all subscriptions relevant to this event.
	// Events may have no subscribers.
	// TODO: specify return type in EVG-2861
	Notifications() ([]interface{}, error)
}

type EventLogEntry struct {
	ResourceType string    `bson:"r_type" json:"resource_type"`
	Timestamp    time.Time `bson:"ts" json:"timestamp"`
	ResourceId   string    `bson:"r_id" json:"resource_id"`
	EventType    string    `bson:"e_type" json:"event_type"`
	Data         Data      `bson:"data" json:"data"`
}

type unmarshalEventLogEntry struct {
	ResourceType string    `bson:"r_type" json:"resource_type"`
	Timestamp    time.Time `bson:"ts" json:"timestamp"`
	ResourceId   string    `bson:"r_id" json:"resource_id"`
	EventType    string    `bson:"e_type" json:"event_type"`
	Data         bson.Raw  `bson:"data" json:"data"`
}

var (
	// bson fields for the event struct
	TimestampKey    = bsonutil.MustHaveTag(EventLogEntry{}, "Timestamp")
	ResourceIdKey   = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceId")
	ResourceTypeKey = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceType")
	TypeKey         = bsonutil.MustHaveTag(EventLogEntry{}, "EventType")
	DataKey         = bsonutil.MustHaveTag(EventLogEntry{}, "Data")
)

const resourceTypeKey = "r_type"

type Data interface {
	IsValid() bool
}

func (e *EventLogEntry) SetBSON(raw bson.Raw) error {
	temp := unmarshalEventLogEntry{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal event container type")
	}

	if len(temp.ResourceType) == 0 {
		// Fetch r_type in the data subdoc
		rawD := bson.RawD{}
		if err := temp.Data.Unmarshal(&rawD); err != nil {
			return errors.Wrap(err, "can't unmarshal raw event data")
		}

		for i := range rawD {
			if rawD[i].Name == resourceTypeKey {
				if err := rawD[i].Value.Unmarshal(&temp.ResourceType); err != nil {
					return errors.Wrap(err, "failed to read resource type (r_type) from event data")
				}
			}
		}
	}
	if len(temp.ResourceType) == 0 {
		return errors.New("expected non-empty r_type while unmarshalling event data")
	}

	e.Data = NewEventFromType(temp.ResourceType)
	if e.Data == nil {
		return errors.Errorf("unknown resource type '%s'", temp.ResourceType)
	}
	if err := temp.Data.Unmarshal(e.Data); err != nil {
		return errors.Wrap(err, "failed to unmarshal data")
	}

	e.Timestamp = temp.Timestamp
	e.ResourceId = temp.ResourceId
	e.EventType = temp.EventType
	e.ResourceType = temp.ResourceType

	return nil
}

// findResourceTypeTagIn attempts locates a bson tag called "r_type" in t.
// If found, this function returns true, and the value of that field
// If not, this function returns false, and empty string
func findResourceTypeIn(t interface{}) (bool, string) {
	if t == nil {
		return false, ""
	}

	elem := reflect.TypeOf(t).Elem()
	for i := 0; i < elem.NumField(); i++ {
		f := elem.Field(i)
		bsonTag := f.Tag.Get("bson")
		if len(bsonTag) == 0 {
			continue
		}

		if f.Type.Kind() != reflect.String {
			return false, ""
		}

		if bsonTag != resourceTypeKey && !strings.HasPrefix(bsonTag, resourceTypeKey+",") {
			continue
		}

		structData := reflect.ValueOf(t).Elem().Field(i).String()
		if bsonTag == resourceTypeKey+",omitempty" {
			return true, structData
		}
		return false, structData
	}

	return false, ""
}
