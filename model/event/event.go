package event

import (
	"reflect"
	"strings"
	"time"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	// db constants
	AllLogCollection  = "event_log"
	TaskLogCollection = "task_event_log"
)

type EventLogEntry struct {
	ID           bson.ObjectId `bson:"_id" json:"-"`
	ResourceType string        `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	ProcessedAt  time.Time     `bson:"processed_at,omitempty" json:"processed_at,omitempty"`

	Timestamp  time.Time   `bson:"ts" json:"timestamp"`
	ResourceId string      `bson:"r_id" json:"resource_id"`
	EventType  string      `bson:"e_type" json:"event_type"`
	Data       interface{} `bson:"data" json:"data"`
}

// Processed is whether or not this event has been processed. An event
// which has been processed has successfully have notifications intents
// created and stored, but does not indicate whether or not these
// notifications have been successfully sent to all recipients
// If true, the time is the time that this event was marked as
// processed. If false, time is the zero time
func (e *EventLogEntry) Processed() (bool, time.Time) {
	return !e.ProcessedAt.IsZero(), e.ProcessedAt
}

type unmarshalEventLogEntry struct {
	ID           bson.ObjectId `bson:"_id" json:"-"`
	ResourceType string        `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	ProcessedAt  time.Time     `bson:"processed_at,omitempty" json:"processed_at,omitempty"`

	Timestamp  time.Time `bson:"ts" json:"timestamp"`
	ResourceId string    `bson:"r_id" json:"resource_id"`
	EventType  string    `bson:"e_type" json:"event_type"`
	Data       bson.Raw  `bson:"data" json:"data"`
}

var (
	// bson fields for the event struct
	idKey           = bsonutil.MustHaveTag(EventLogEntry{}, "ID")
	TimestampKey    = bsonutil.MustHaveTag(EventLogEntry{}, "Timestamp")
	ResourceIdKey   = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceId")
	ResourceTypeKey = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceType")
	processedAtKey  = bsonutil.MustHaveTag(EventLogEntry{}, "ProcessedAt")
	TypeKey         = bsonutil.MustHaveTag(EventLogEntry{}, "EventType")
	DataKey         = bsonutil.MustHaveTag(EventLogEntry{}, "Data")
)

const resourceTypeKey = "r_type"

func (e *EventLogEntry) SetBSON(raw bson.Raw) error {
	temp := unmarshalEventLogEntry{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal event container type")
	}

	if len(temp.ResourceType) == 0 {
		//TODO: EVG-3061 delete this line through until return errors.New("expected ...
		// Try and fetch r_type in the data subdoc
		rawD := bson.RawD{}
		if err := temp.Data.Unmarshal(&rawD); err != nil {
			return errors.Wrap(err, "can't unmarshal raw event data")
		}

		for i := range rawD {
			if rawD[i].Name == resourceTypeKey {
				if err := rawD[i].Value.Unmarshal(&temp.ResourceType); err != nil {
					return errors.Wrap(err, "failed to read resource type (r_type) from event data")
				}
				break
			}
		}

		if temp.ResourceType != EventTaskSystemInfo &&
			temp.ResourceType != EventTaskProcessInfo {
			grip.Alert(message.Fields{
				"message":  "unmigrated event was found",
				"event_id": temp.ID.String(),
				"r_type":   temp.ResourceType,
			})
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

	e.ID = temp.ID
	e.Timestamp = temp.Timestamp
	e.ResourceId = temp.ResourceId
	e.EventType = temp.EventType
	e.ProcessedAt = temp.ProcessedAt
	e.ResourceType = temp.ResourceType

	return nil
}

// findResourceTypeIn attempts to locate a bson tag with "r_type,omitempty" in it.
// If found, this function returns true, and the value of that field
// If not, this function returns false. If it the struct had "r_type" (without
// omitempty), it will return that field's value, otherwise it returns an
// empty string
func findResourceTypeIn(data interface{}) (bool, string) {
	const resourceTypeKeyWithOmitEmpty = resourceTypeKey + ",omitempty"

	if data == nil {
		return false, ""
	}

	v := reflect.ValueOf(data)
	t := reflect.TypeOf(data)

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false, ""
	}

	for i := 0; i < v.NumField(); i++ {
		fieldType := t.Field(i)
		bsonTag := fieldType.Tag.Get("bson")
		if len(bsonTag) == 0 {
			continue
		}

		if fieldType.Type.Kind() != reflect.String {
			return false, ""
		}

		if bsonTag != resourceTypeKey && !strings.HasPrefix(bsonTag, resourceTypeKey+",") {
			continue
		}

		structData := v.Field(i).String()
		return bsonTag == resourceTypeKeyWithOmitEmpty, structData
	}

	return false, ""
}
