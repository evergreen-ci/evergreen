package event

import (
	"10gen.com/mci/db/bsonutil"
	"encoding/json"
	"fmt"
	"labix.org/v2/mgo/bson"
	"time"
)

const (
	// db constants
	Collection = "event_log"
)

type Event struct {
	Timestamp  time.Time   `bson:"ts" json:"timestamp"`
	ResourceId string      `bson:"r_id" json:"resource_id"`
	EventType  string      `bson:"e_type" json:"event_type"`
	Data       DataWrapper `bson:"data" json:"data"`
}

var (
	// bson fields for the event struct
	TimestampKey  = bsonutil.MustHaveTag(Event{}, "Timestamp")
	ResourceIdKey = bsonutil.MustHaveTag(Event{}, "ResourceId")
	TypeKey       = bsonutil.MustHaveTag(Event{}, "EventType")
	DataKey       = bsonutil.MustHaveTag(Event{}, "Data")

	// resource type key.  this doesn't exist a part of the event struct,
	// but has to be the same for all of the event types
	ResourceTypeKey = bsonutil.MustHaveTag(HostEventData{}, "ResourceType")
)

type DataWrapper struct {
	Data
}

type Data interface {
	IsValid() bool
}

// MarshalJSON returns proper JSON encoding by uncovering the Data interface.
func (dw DataWrapper) MarshalJSON() ([]byte, error) {
	switch event := dw.Data.(type) {
	case *TaskEventData:
		return json.Marshal(event)
	case *HostEventData:
		return json.Marshal(event)
	default:
		return nil, fmt.Errorf("cannot marshal data of type %T", dw.Data)
	}
}

func (dw DataWrapper) GetBSON() (interface{}, error) {
	return dw.Data, nil
}

func (dw *DataWrapper) SetBSON(raw bson.Raw) error {
	for _, impl := range []interface{}{&TaskEventData{}, &HostEventData{}, &DistroEventData{}} {
		err := raw.Unmarshal(impl)
		if err != nil {
			return err
		}
		if impl.(Data).IsValid() {
			dw.Data = impl.(Data)
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
