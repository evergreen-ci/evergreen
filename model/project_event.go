package model

import (
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	EventResourceTypeProject = "PROJECT"
	EventTypeProjectModified = "PROJECT_MODIFIED"
	EventTypeProjectAdded    = "PROJECT_ADDED"
)

type ProjectSettingsEvent struct {
	ProjectRef         ProjectRef           `bson:"proj_ref" json:"proj_ref"`
	GitHubHooksEnabled bool                 `bson:"github_hooks_enabled" json:"github_hooks_enabled"`
	Vars               ProjectVars          `bson:"vars" json:"vars"`
	Aliases            []ProjectAlias       `bson:"aliases" json:"aliases"`
	Subscriptions      []event.Subscription `bson:"subscriptions" json:"subscriptions"`
}

type ProjectChangeEvent struct {
	User   string               `bson:"user" json:"user"`
	Before ProjectSettingsEvent `bson:"before" json:"before"`
	After  ProjectSettingsEvent `bson:"after" json:"after"`
}

type ProjectChangeEventEntry struct {
	event.EventLogEntry
}

func (e *ProjectChangeEventEntry) SetBSON(raw bson.Raw) error {
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
	case bson.ObjectId:
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
func MostRecentProjectEvents(id string, n int) ([]ProjectChangeEventEntry, error) {
	filter := event.ResourceTypeKeyIs(EventResourceTypeProject)
	filter[event.ResourceIdKey] = id

	query := db.Query(filter).Sort([]string{"-" + event.TimestampKey}).Limit(n)
	events := []ProjectChangeEventEntry{}
	err := db.FindAllQ(event.AllLogCollection, query, &events)

	return events, err
}

func ProjectEventsBefore(id string, before time.Time, n int) ([]ProjectChangeEventEntry, error) {
	filter := event.ResourceTypeKeyIs(EventResourceTypeProject)
	filter[event.ResourceIdKey] = id
	filter[event.TimestampKey] = bson.M{
		"$lt": before,
	}

	query := db.Query(filter).Sort([]string{"-" + event.TimestampKey}).Limit(n)
	events := []ProjectChangeEventEntry{}
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
		message.WrapError(err, message.Fields{
			"resource_type": EventResourceTypeProject,
			"message":       "error logging event",
			"source":        "event-log-fail",
		})
		return errors.Wrap(err, "Error logging project event")
	}

	return nil
}

func LogProjectAdded(projectId, username string) error {
	return LogProjectEvent(EventTypeProjectAdded, projectId, ProjectChangeEvent{User: username})
}

func LogProjectModified(projectId, username string, before, after ProjectSettingsEvent) error {
	// Stop if there are no changes
	if reflect.DeepEqual(before, after) {
		return nil
	}

	eventData := ProjectChangeEvent{
		User:   username,
		Before: before,
		After:  after,
	}
	return LogProjectEvent(EventTypeProjectModified, projectId, eventData)
}
