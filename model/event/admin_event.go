package event

import (
	"fmt"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	ResourceTypeAdmin     = "ADMIN"
	EventTypeValueChanged = "CONFIG_VALUE_CHANGED"
)

// AdminEventData holds all potential data properties of a logged admin event
type AdminEventData struct {
	GUID         string           `bson:"guid" json:"guid"`
	ResourceType string           `bson:"r_type" json:"resource_type"`
	User         string           `bson:"user" json:"user"`
	Section      string           `bson:"section" json:"section"`
	Changes      ConfigDataChange `bson:"changes" json:"changes"`
}

type ConfigDataChange struct {
	Before evergreen.ConfigSection `bson:"before" json:"before"`
	After  evergreen.ConfigSection `bson:"after" json:"after"`
}

type rawConfigDataChange struct {
	Before bson.Raw `bson:"before" json:"before"`
	After  bson.Raw `bson:"after" json:"after"`
}

type rawAdminEventData struct {
	GUID         string              `bson:"guid"`
	ResourceType string              `bson:"r_type"`
	User         string              `bson:"user"`
	Section      string              `bson:"section"`
	Changes      rawConfigDataChange `bson:"changes"`
}

// IsValid checks if a given event is an event on an admin resource
func (evt AdminEventData) IsValid() bool {
	return evt.ResourceType == ResourceTypeAdmin
}

func (evt rawAdminEventData) IsValid() bool {
	return evt.ResourceType == ResourceTypeAdmin
}

func LogAdminEvent(section string, before, after evergreen.ConfigSection, user string) error {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	eventData := AdminEventData{
		ResourceType: ResourceTypeAdmin,
		User:         user,
		Section:      section,
		Changes:      ConfigDataChange{Before: before, After: after},
		GUID:         util.RandomString(),
	}
	event := Event{
		Timestamp: time.Now(),
		EventType: EventTypeValueChanged,
		Data:      eventData,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(event); err != nil {
		return errors.Wrap(err, "Error logging admin event")
	}
	return nil
}

func FindAdmin(query db.Q) ([]Event, error) {
	eventsRaw, err := Find(AllLogCollection, query)
	if err != nil {
		return nil, err
	}
	events := []Event{}
	catcher := grip.NewSimpleCatcher()
	for _, event := range eventsRaw {
		eventDataRaw := event.Data.(*rawAdminEventData)
		eventData, err := convertRaw(*eventDataRaw)
		if err != nil {
			catcher.Add(err)
			continue
		}
		events = append(events, Event{
			Timestamp:  event.Timestamp,
			ResourceId: event.ResourceId,
			EventType:  event.EventType,
			Data:       eventData,
		})
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return events, nil
}

func convertRaw(in rawAdminEventData) (*AdminEventData, error) {
	out := AdminEventData{
		ResourceType: in.ResourceType,
		Section:      in.Section,
		User:         in.User,
		GUID:         in.GUID,
	}

	// get the correct implementation of the interface from the registry
	section := evergreen.ConfigRegistry.GetSection(out.Section)
	if section == nil {
		return nil, fmt.Errorf("unable to determine section '%s'", out.Section)
	}

	// create 2 copies of the section interface for our value
	before := reflect.New(reflect.ValueOf(section).Elem().Type()).Interface().(evergreen.ConfigSection)
	after := reflect.New(reflect.ValueOf(section).Elem().Type()).Interface().(evergreen.ConfigSection)

	// deserialize the before/after values
	err := in.Changes.Before.Unmarshal(before)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to decode section '%s'", out.Section)
	}
	err = in.Changes.After.Unmarshal(after)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to decode section '%s'", out.Section)
	}
	out.Changes.Before = before
	out.Changes.After = after

	return &out, nil
}
