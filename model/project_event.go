package model

import (
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	EventResourceTypeProject = "PROJECT"
	EventTypeProjectModified = "PROJECT_MODIFIED"
	EventTypeProjectAdded    = "PROJECT_ADDED"
)

type ProjectSettings struct {
	ProjectRef         ProjectRef           `bson:"proj_ref" json:"proj_ref"`
	GitHubHooksEnabled bool                 `bson:"github_hooks_enabled" json:"github_hooks_enabled"`
	Vars               ProjectVars          `bson:"vars" json:"vars"`
	Aliases            []ProjectAlias       `bson:"aliases" json:"aliases"`
	Subscriptions      []event.Subscription `bson:"subscriptions" json:"subscriptions"`
}

type ProjectChangeEvent struct {
	User   string          `bson:"user" json:"user"`
	Before ProjectSettings `bson:"before" json:"before"`
	After  ProjectSettings `bson:"after" json:"after"`
}

type ProjectChangeEvents []ProjectChangeEventEntry

func (p *ProjectChangeEvents) RedactPrivateVars() {
	for _, event := range *p {
		changeEvent, isChangeEvent := event.Data.(*ProjectChangeEvent)
		if !isChangeEvent {
			continue
		}
		changeEvent.After.Vars = *changeEvent.After.Vars.RedactPrivateVars()
		changeEvent.Before.Vars = *changeEvent.Before.Vars.RedactPrivateVars()
		event.EventLogEntry.Data = changeEvent
	}
}

type ProjectChangeEventEntry struct {
	event.EventLogEntry
}

func (e *ProjectChangeEventEntry) UnmarshalBSON(in []byte) error {
	return mgobson.Unmarshal(in, e)
}

func (e *ProjectChangeEventEntry) MarshalBSON() ([]byte, error) {
	return mgobson.Marshal(e)
}

func (e *ProjectChangeEventEntry) SetBSON(raw mgobson.Raw) error {
	temp := event.UnmarshalEventLogEntry{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal event container type")
	}

	e.Data = &ProjectChangeEvent{}

	if err := temp.Data.Unmarshal(e.Data); err != nil {
		return errors.Wrap(err, "failed to unmarshal data")
	}

	// IDs for events were ObjectIDs previously, so we need to do this
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

	return nil
}

// Project Events queries
func MostRecentProjectEvents(id string, n int) (ProjectChangeEvents, error) {
	filter := event.ResourceTypeKeyIs(EventResourceTypeProject)
	filter[event.ResourceIdKey] = id

	query := db.Query(filter).Sort([]string{"-" + event.TimestampKey}).Limit(n)
	events := ProjectChangeEvents{}
	err := db.FindAllQ(event.AllLogCollection, query, &events)

	return events, err
}

func ProjectEventsBefore(id string, before time.Time, n int) (ProjectChangeEvents, error) {
	filter := event.ResourceTypeKeyIs(EventResourceTypeProject)
	filter[event.ResourceIdKey] = id
	filter[event.TimestampKey] = bson.M{
		"$lt": before,
	}

	query := db.Query(filter).Sort([]string{"-" + event.TimestampKey}).Limit(n)
	events := ProjectChangeEvents{}
	err := db.FindAllQ(event.AllLogCollection, query, &events)

	return events, err
}

func LogProjectEvent(eventType string, projectId string, eventData ProjectChangeEvent) error {
	projectEvent := event.EventLogEntry{
		Timestamp:    time.Now(),
		ResourceType: EventResourceTypeProject,
		EventType:    eventType,
		ResourceId:   projectId,
		Data:         eventData,
	}

	logger := event.NewDBEventLogger(event.AllLogCollection)
	if err := logger.LogEvent(&projectEvent); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": EventResourceTypeProject,
			"message":       "error logging event",
			"source":        "event-log-fail",
			"projectId":     projectId,
		}))
		return errors.Wrap(err, "Error logging project event")
	}

	return nil
}

func LogProjectAdded(projectId, username string) error {
	return LogProjectEvent(EventTypeProjectAdded, projectId, ProjectChangeEvent{User: username})
}

func GetAndLogProjectModified(id, userId string, isRepo bool, before *ProjectSettings) error {
	after, err := GetProjectSettingsById(id, isRepo)
	if err != nil {
		return errors.Wrap(err, "error getting after project settings event")
	}
	return errors.Wrapf(LogProjectModified(id, userId, before, after), "error logging project modified")
}

func LogProjectModified(projectId, username string, before, after *ProjectSettings) error {
	if before == nil || after == nil {
		return nil
	}
	// Stop if there are no changes
	if reflect.DeepEqual(*before, *after) {
		return nil
	}

	eventData := ProjectChangeEvent{
		User:   username,
		Before: *before,
		After:  *after,
	}
	return LogProjectEvent(EventTypeProjectModified, projectId, eventData)
}
