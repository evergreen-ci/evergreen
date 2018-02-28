package event

import (
	"encoding/json"
	"reflect"
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

type EventLogEntry struct {
	Timestamp  time.Time `bson:"ts" json:"timestamp"`
	ResourceId string    `bson:"r_id" json:"resource_id"`
	EventType  string    `bson:"e_type" json:"event_type"`
	Data       Data      `bson:"data" json:"data"`
}

type unmarshalEventLogEntry struct {
	Timestamp  time.Time `bson:"ts" json:"timestamp"`
	ResourceId string    `bson:"r_id" json:"resource_id"`
	EventType  string    `bson:"e_type" json:"event_type"`
	Data       bson.Raw  `bson:"data" json:"data"`
}

var (
	// bson fields for the event struct
	TimestampKey  = bsonutil.MustHaveTag(EventLogEntry{}, "Timestamp")
	ResourceIdKey = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceId")
	TypeKey       = bsonutil.MustHaveTag(EventLogEntry{}, "EventType")
	DataKey       = bsonutil.MustHaveTag(EventLogEntry{}, "Data")

	resourceTypeKey = "r_type"
)

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

	e.EventType = temp.EventType
	e.ResourceId = temp.ResourceId
	e.Timestamp = temp.Timestamp

	rawD := bson.RawD{}
	if err := temp.Data.Unmarshal(&rawD); err != nil {
		return errors.Wrap(err, "can't unmarshal raw event data")
	}

	dataType := ""
	for i := range rawD {
		if rawD[i].Name == "r_type" {
			if err := rawD[i].Value.Unmarshal(&dataType); err != nil {
				return errors.Wrap(err, "failed to read r_type")
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
			if f.Type.String() != "string" {
				return false, ""
			}

			structData := reflect.ValueOf(t).Elem().Field(i).String()
			return true, structData
		}
	}

	return false, ""
}
