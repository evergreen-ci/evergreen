package event

import (
	"context"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.AddType(ResourceTypeAdmin, func() interface{} { return &rawAdminEventData{} })
	registry.setUnexpirable(ResourceTypeAdmin, EventTypeValueChanged)
}

const (
	ResourceTypeAdmin     = "ADMIN"
	EventTypeValueChanged = "CONFIG_VALUE_CHANGED"
)

// AdminEventData holds all potential data properties of a logged admin event
type AdminEventData struct {
	GUID    string           `bson:"guid" json:"guid"`
	User    string           `bson:"user" json:"user"`
	Section string           `bson:"section" json:"section"`
	Changes ConfigDataChange `bson:"changes" json:"changes"`
}

type ConfigDataChange struct {
	Before evergreen.ConfigSection `bson:"before" json:"before"`
	After  evergreen.ConfigSection `bson:"after" json:"after"`
}

type rawConfigDataChange struct {
	Before mgobson.Raw `bson:"before" json:"before"`
	After  mgobson.Raw `bson:"after" json:"after"`
}

type rawAdminEventData struct {
	GUID    string              `bson:"guid"`
	User    string              `bson:"user"`
	Section string              `bson:"section"`
	Changes rawConfigDataChange `bson:"changes"`
}

func LogAdminEvent(section string, before, after evergreen.ConfigSection, user string) error {
	if section == evergreen.ConfigDocID {
		beforeSettings := before.(*evergreen.Settings)
		afterSettings := after.(*evergreen.Settings)
		before = stripInteriorSections(beforeSettings)
		after = stripInteriorSections(afterSettings)
	}
	if reflect.DeepEqual(before, after) {
		return nil
	}
	eventData := AdminEventData{
		User:    user,
		Section: section,
		Changes: ConfigDataChange{Before: before, After: after},
		GUID:    utility.RandomString(),
	}
	event := EventLogEntry{
		Timestamp:    time.Now(),
		EventType:    EventTypeValueChanged,
		Data:         eventData,
		ResourceType: ResourceTypeAdmin,
	}

	if err := event.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeAdmin,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
		return errors.Wrap(err, "logging admin event")
	}
	return nil
}

func stripInteriorSections(config *evergreen.Settings) *evergreen.Settings {
	copy := &evergreen.Settings{}
	*copy = *config
	valConfigPtr := reflect.ValueOf(copy)
	valConfig := reflect.Indirect(valConfigPtr)
	for i := 0; i < valConfig.NumField(); i++ {
		sectionId := valConfig.Type().Field(i).Tag.Get("id")
		if sectionId == "" {
			continue
		}

		propName := valConfig.Type().Field(i).Name
		propVal := valConfig.FieldByName(propName)
		propVal.Set(reflect.Zero(propVal.Type()))
	}

	configInterface := valConfigPtr.Interface()
	return configInterface.(*evergreen.Settings)
}

func FindAdmin(query db.Q) ([]EventLogEntry, error) {
	eventsRaw, err := Find(query)
	if err != nil {
		return nil, err
	}
	events := []EventLogEntry{}
	catcher := grip.NewSimpleCatcher()
	for _, event := range eventsRaw {
		eventDataRaw := event.Data.(*rawAdminEventData)
		eventData, err := convertRaw(*eventDataRaw)
		if err != nil {
			catcher.Add(err)
			continue
		}
		events = append(events, EventLogEntry{
			ResourceType: ResourceTypeAdmin,
			Timestamp:    event.Timestamp,
			ResourceId:   event.ResourceId,
			EventType:    event.EventType,
			Data:         eventData,
		})
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return events, nil
}

func convertRaw(in rawAdminEventData) (*AdminEventData, error) {
	out := AdminEventData{
		Section: in.Section,
		User:    in.User,
		GUID:    in.GUID,
	}

	// get the correct implementation of the interface from the registry
	section := evergreen.NewConfigSections().Sections[out.Section]
	if section == nil {
		return nil, errors.Errorf("getting section '%s' from config registry", out.Section)
	}

	// create 2 copies of the section interface for our value
	before := reflect.New(reflect.ValueOf(section).Elem().Type()).Interface().(evergreen.ConfigSection)
	after := reflect.New(reflect.ValueOf(section).Elem().Type()).Interface().(evergreen.ConfigSection)

	// deserialize the before/after values
	err := in.Changes.Before.Unmarshal(before)
	if err != nil {
		return nil, errors.Wrapf(err, "decoding before changes for section '%s'", out.Section)
	}
	err = in.Changes.After.Unmarshal(after)
	if err != nil {
		return nil, errors.Wrapf(err, "decoding after changes for section '%s'", out.Section)
	}
	out.Changes.Before = before
	out.Changes.After = after

	return &out, nil
}

// RevertConfig reverts one config section to the before state of the specified GUID in the event log
func RevertConfig(ctx context.Context, guid string, user string) error {
	events, err := FindAdmin(ByAdminGuid(guid))
	if err != nil {
		return errors.Wrap(err, "finding events")
	}
	if len(events) == 0 {
		return errors.Errorf("finding event with GUID '%s'", guid)
	}
	evt := events[0]
	data := evt.Data.(*AdminEventData)
	current := evergreen.NewConfigSections().Sections[data.Section]
	if current == nil {
		return errors.Errorf("finding section '%s'", data.Section)
	}
	err = current.Get(ctx)
	if err != nil {
		return errors.Wrapf(err, "reading section '%s'", current.SectionId())
	}
	err = data.Changes.Before.Set(ctx)
	if err != nil {
		return errors.Wrap(err, "reverting to before settings")
	}

	return LogAdminEvent(data.Section, current, data.Changes.Before, user)
}
