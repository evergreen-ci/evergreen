package event

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	AllLogCollection           = "event_log"
	TaskLogCollection          = "task_event_log"
	NotificationsLogCollection = "notifications_log"
)

var (
	eventLogEntryProcessedAtKey = bsonutil.MustHaveTag(EventLogEntry{}, "ProcessedAt")
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
	Notifications() ([]NotificationEvent, error)
}

type EventLogEntry struct {
	DocID        bson.ObjectId `bson:"_id" json:"-"`
	ResourceType string        `bson:"r_type" json:"resource_type"`

	Timestamp  time.Time `bson:"ts" json:"timestamp"`
	ResourceId string    `bson:"r_id" json:"resource_id"`
	EventType  string    `bson:"e_type" json:"event_type"`
	Data       Data      `bson:"data" json:"data"`

	ProcessedAt *time.Time `bson:"processed_at,omitempty" json:"processed_at,omitempty"`
}

func (e *EventLogEntry) ID() string {
	return e.DocID.Hex()
}

func (e *EventLogEntry) Type() string {
	if len(e.ResourceType) == 0 {
		_, rType := findResourceTypeIn(e.Data)
		return rType
	}
	return e.ResourceType
}

func (e *EventLogEntry) Time() time.Time {
	return e.Timestamp
}

func (e *EventLogEntry) Processed() (bool, time.Time) {
	if e.ProcessedAt == nil {
		return true, time.Time{}
	}
	return e.ProcessedAt.Equal(time.Time{}), *e.ProcessedAt
}

func (e *EventLogEntry) MarkProcessed() error {
	now := time.Now().Round(time.Millisecond).Truncate(time.Millisecond)
	e.ProcessedAt = &now

	// TODO: WRONG
	err := db.UpdateId(AllLogCollection, e.ID, bson.M{
		"$set": bson.M{
			eventLogEntryProcessedAtKey: e.ProcessedAt,
		},
	})
	if err != nil {
		now = time.Time{}
		e.ProcessedAt = &now
		return err
	}

	return nil
}

func (e *EventLogEntry) Notifications() ([]NotificationEvent, error) {
	return nil, nil
}

type unmarshalEventLogEntry struct {
	DocID        bson.ObjectId `bson:"_id" json:"-"`
	ResourceType string        `bson:"r_type" json:"resource_type"`

	Timestamp  time.Time `bson:"ts" json:"timestamp"`
	ResourceId string    `bson:"r_id" json:"resource_id"`
	EventType  string    `bson:"e_type" json:"event_type"`
	Data       bson.Raw  `bson:"data" json:"data"`

	ProcessedAt *time.Time `bson:"processed_at,omitempty" json:"processed_at,omitempty"`
}

var (
	// bson fields for the event struct
	TimestampKey  = bsonutil.MustHaveTag(EventLogEntry{}, "Timestamp")
	ResourceIdKey = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceId")
	TypeKey       = bsonutil.MustHaveTag(EventLogEntry{}, "EventType")
	DataKey       = bsonutil.MustHaveTag(EventLogEntry{}, "Data")
)

const resourceTypeKey = "r_type"

type Data interface {
	IsValid() bool
}

// MarshalJSON returns proper JSON encoding by uncovering the Data interface.
func (e *EventLogEntry) MarshalJSON() ([]byte, error) {
	found, rType := findResourceTypeIn(e.Data)
	if !found {
		return nil, errors.Errorf("cannot find resource type of type %T", e.Data)
	}
	if NewEventFromType(rType) == nil {
		return nil, errors.Errorf("cannot marshal data of type %T", e.Data)
	}

	return json.Marshal(e.Data)
}

func (e *EventLogEntry) SetBSON(raw bson.Raw) error {
	temp := unmarshalEventLogEntry{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal event container type")
	}

	e.DocID = temp.DocID
	e.EventType = temp.EventType
	e.ResourceId = temp.ResourceId
	e.Timestamp = temp.Timestamp
	e.ProcessedAt = temp.ProcessedAt

	rawD := bson.RawD{}
	if err := temp.Data.Unmarshal(&rawD); err != nil {
		return errors.Wrap(err, "can't unmarshal raw event data")
	}

	dataType := ""
	for i := range rawD {
		if rawD[i].Name == "r_type" {
			if err := rawD[i].Value.Unmarshal(&dataType); err != nil {
				return errors.Wrap(err, "failed to read resource type (r_type) from event")
			}
		}
	}
	if len(dataType) == 0 {
		return errors.New("expected non-empty r_type while unmarshalling event data")
	}

	data := NewEventFromType(dataType)
	if data == nil {
		return errors.Errorf("unknown resource type '%s'", dataType)
	}

	if err := temp.Data.Unmarshal(data); err != nil {
		return errors.Wrap(err, "failed to unmarshal data")
	}

	e.Data = data

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

		if bsonTag == resourceTypeKey {
			if f.Type.Kind() != reflect.String {
				return false, ""
			}

			structData := reflect.ValueOf(t).Elem().Field(i).String()
			return true, structData
		}
	}

	return false, ""
}
