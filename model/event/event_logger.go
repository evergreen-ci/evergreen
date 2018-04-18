package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const notSubscribableTimeString = "2015-10-21T16:29:01-07:00"

type EventLogger interface {
	LogEvent(event *EventLogEntry) error
}

type DBEventLogger struct {
	collection string
}

func NewDBEventLogger(collection string) *DBEventLogger {
	return &DBEventLogger{
		collection: collection,
	}
}

func (l *DBEventLogger) LogEvent(event *EventLogEntry) error {
	if event.Data == nil {
		return errors.New("event log entry cannot have nil Data")
	}
	if len(event.ResourceType) == 0 {
		return errors.New("event log entry has no r_type")
	}
	if !event.ID.Valid() {
		event.ID = bson.NewObjectId()
	}
	if !registry.IsSubscribable(event.ResourceType, event.EventType) {
		loc, _ := time.LoadLocation("UTC")
		notSubscribableTime, err := time.ParseInLocation(time.RFC3339, notSubscribableTimeString, loc)
		if err != nil {
			return errors.Wrap(err, "failed to set processed time")
		}
		event.ProcessedAt = notSubscribableTime
	}

	return db.Insert(l.collection, event)
}

func (l *DBEventLogger) MarkProcessed(event *EventLogEntry) error {
	if !event.ID.Valid() {
		return errors.New("event has no ID")
	}
	event.ProcessedAt = time.Now()

	err := db.Update(l.collection, bson.M{
		idKey: event.ID,
		processedAtKey: bson.M{
			"$eq": time.Time{},
		},
	}, bson.M{
		"$set": bson.M{
			processedAtKey: event.ProcessedAt,
		},
	})
	if err != nil {
		event.ProcessedAt = time.Time{}
		return errors.Wrap(err, "failed to update 'processed at' time")
	}

	return nil
}

// MarkAllEventsProcessed marks all events processed with the current time
func MarkAllEventsProcessed(collection string) error {
	_, err := db.UpdateAll(collection, UnprocessedEvents(), bson.M{
		"$set": bson.M{
			processedAtKey: time.Now(),
		},
	})
	return err
}
