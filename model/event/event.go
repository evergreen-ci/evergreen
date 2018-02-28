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
	rawD := bson.RawD{}
	if err := raw.Unmarshal(&rawD); err != nil {
		return err
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

	if err := raw.Unmarshal(data); err != nil {
		return errors.Wrap(err, "failed to unmarshall data")
	}
	if !data.IsValid() {
		return errors.New("unmarshalled data was invalid. This should NEVER happen")
	}
	dw.Data = data

	return nil
}
