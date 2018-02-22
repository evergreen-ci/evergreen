package event

import (
	"encoding/json"
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
	case *SchedulerEventData:
		return json.Marshal(event)
	case *DistroEventData:
		return json.Marshal(event)
	case *TaskSystemResourceData:
		return json.Marshal(event)
	case *TaskProcessResourceData:
		return json.Marshal(event)
	case *AdminEventData:
		return json.Marshal(event)
	default:
		return nil, errors.Errorf("cannot marshal data of type %T", dw.Data)
	}
}

func (dw DataWrapper) GetBSON() (interface{}, error) {
	return dw.Data, nil
}

func (dw *DataWrapper) SetBSON(raw bson.Raw) error {
	impls := []interface{}{&TaskEventData{}, &HostEventData{}, &DistroEventData{}, &SchedulerEventData{},
		&TaskSystemResourceData{}, &TaskProcessResourceData{}, &rawAdminEventData{}}

	for _, impl := range impls {
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
	return errors.Errorf("No suitable type for %#v", m)
}
