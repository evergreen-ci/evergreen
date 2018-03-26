package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

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
	if !isSubscribable(event.ResourceType) {
		const notSubscribableTime = "2015-10-21T16:29:01-07:00"

		loc, _ := time.LoadLocation("UTC")
		bttf, _ := time.ParseInLocation(time.RFC3339, notSubscribableTime, loc)
		event.ProcessedAt = bttf
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
			"$exists": false,
		},
	}, bson.M{
		"$set": bson.M{
			processedAtKey: event.ProcessedAt,
		},
	})
	if err != nil {
		event.ProcessedAt = time.Time{}
		return errors.Wrap(err, "failed to update process time")
	}

	return nil
}
