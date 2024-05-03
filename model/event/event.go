package event

import (
	"time"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const EventCollection = "events"

var notSubscribableTime = time.Date(2015, time.October, 21, 23, 29, 1, 0, time.UTC)

type EventLogEntry struct {
	ID           string    `bson:"_id" json:"-"`
	ResourceType string    `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	ProcessedAt  time.Time `bson:"processed_at" json:"processed_at"`

	Timestamp  time.Time   `bson:"ts" json:"timestamp"`
	Expirable  bool        `bson:"expirable,omitempty" json:"expirable,omitempty"`
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

type UnmarshalEventLogEntry struct {
	ID           interface{} `bson:"_id" json:"-"`
	ResourceType string      `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	ProcessedAt  time.Time   `bson:"processed_at" json:"processed_at"`

	Timestamp  time.Time   `bson:"ts" json:"timestamp"`
	Expirable  bool        `bson:"expirable,omitempty" json:"expirable,omitempty"`
	ResourceId string      `bson:"r_id" json:"resource_id"`
	EventType  string      `bson:"e_type" json:"event_type"`
	Data       mgobson.Raw `bson:"data" json:"data"`
}

var (
	// bson fields for the event struct
	idKey           = bsonutil.MustHaveTag(EventLogEntry{}, "ID")
	TimestampKey    = bsonutil.MustHaveTag(EventLogEntry{}, "Timestamp")
	ResourceIdKey   = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceId")
	ResourceTypeKey = bsonutil.MustHaveTag(EventLogEntry{}, "ResourceType")
	eventTypeKey    = bsonutil.MustHaveTag(EventLogEntry{}, "EventType")
	processedAtKey  = bsonutil.MustHaveTag(EventLogEntry{}, "ProcessedAt")
	TypeKey         = bsonutil.MustHaveTag(EventLogEntry{}, "EventType")
	DataKey         = bsonutil.MustHaveTag(EventLogEntry{}, "Data")
)

const resourceTypeKey = "r_type"

func (e *EventLogEntry) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, e) }
func (e *EventLogEntry) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(e) }

func (e *EventLogEntry) SetBSON(raw mgobson.Raw) error {
	temp := UnmarshalEventLogEntry{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "unmarshalling event into generic event log entry")
	}

	e.Data = registry.newEventFromType(temp.ResourceType)
	if e.Data == nil {
		return errors.Errorf("unknown resource type '%s'", temp.ResourceType)
	}
	if err := temp.Data.Unmarshal(e.Data); err != nil {
		return errors.Wrap(err, "unmarshalling data")
	}

	// IDs for events were ObjectIDs previously, so we need to do this
	// TODO (EVG-17214): Remove once old events are TTLed and/or migrated.
	switch v := temp.ID.(type) {
	case string:
		e.ID = v
	case mgobson.ObjectId:
		e.ID = v.Hex()
	case primitive.ObjectID:
		e.ID = v.Hex()
	default:
		return errors.Errorf("unrecognized ID format for event %v", v)
	}
	e.Timestamp = temp.Timestamp
	e.ResourceId = temp.ResourceId
	e.EventType = temp.EventType
	e.ProcessedAt = temp.ProcessedAt
	e.ResourceType = temp.ResourceType
	e.Expirable = temp.Expirable

	return nil
}

func (e *EventLogEntry) validateEvent() error {
	if e.Data == nil {
		return errors.New("event log entry cannot have nil data")
	}
	if len(e.ResourceType) == 0 {
		return errors.New("event log entry has no resource type")
	}
	if e.ID == "" {
		e.ID = mgobson.NewObjectId().Hex()
	}
	if !registry.IsSubscribable(e.ResourceType, e.EventType) {
		e.ProcessedAt = notSubscribableTime
	}
	e.Expirable = registry.isExpirable(e.ResourceType, e.EventType)

	return nil
}
