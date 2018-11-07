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
	ResourceTypeProject      = "PROJECT"
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

type ProjectChange struct {
	User   string          `bson:"user" json:"user"`
	Before ProjectSettings `bson:"before" json:"before"`
	After  ProjectSettings `bson:"after" json:"after"`
}

// Embed the EventLogEntry so we can reimplement the SetBSON method for Projects
type ProjectChangeEvent struct {
	event.EventLogEntry
}

func (e *ProjectChangeEvent) SetBSON(raw bson.Raw) error {
	temp := event.UnmarshalEventLogEntry{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal event container type")
	}

	e.Data = &ProjectChange{}

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
func ProjectEventsForId(id string) db.Q {
	filter := event.ResourceTypeKeyIs(ResourceTypeProject)
	filter[event.ResourceIdKey] = id

	return db.Query(filter)
}

func MostRecentProjectEvents(id string, n int) db.Q {
	return ProjectEventsForId(id).Sort([]string{"-" + event.TimestampKey}).Limit(n)
}

func ProjectEventsInOrder(id string) db.Q {
	return ProjectEventsForId(id).Sort([]string{event.TimestampKey})
}

func ProjectEventsBefore(id string, before time.Time, n int) db.Q {
	filter := event.ResourceTypeKeyIs(ResourceTypeProject)
	filter[event.ResourceIdKey] = id
	filter[event.TimestampKey] = bson.M{
		"$lt": before,
	}

	return db.Query(filter).Sort([]string{"-" + event.TimestampKey}).Limit(n)
}

func LogProjectEvent(eventType string, projectId string, eventData ProjectChange) error {
	projectEvent := event.EventLogEntry{
		Timestamp:    time.Now(),
		ResourceType: ResourceTypeProject,
		EventType:    eventType,
		ResourceId:   projectId,
		Data:         eventData,
	}

	logger := event.NewDBEventLogger(event.AllLogCollection)
	if err := logger.LogEvent(&projectEvent); err != nil {
		message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeProject,
			"message":       "error logging event",
			"source":        "event-log-fail",
		})
		return errors.Wrap(err, "Error logging project event")
	}

	return nil
}

func LogProjectAdded(projectId, username string) error {
	return LogProjectEvent(EventTypeProjectAdded, projectId, ProjectChange{User: username})
}

func LogProjectModified(projectId, username string, before, after ProjectSettings) error {
	// Stop if there are no changes
	if reflect.DeepEqual(before, after) {
		return nil
	}
	eventData := ProjectChange{
		User:   username,
		Before: before,
		After:  after,
	}
	return LogProjectEvent(EventTypeProjectModified, projectId, eventData)
}
