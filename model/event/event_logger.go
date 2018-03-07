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

func (self *DBEventLogger) LogEvent(event EventLogEntry) error {
	if event.Data == nil {
		return errors.New("event log entry cannot have nil Data")
	}
	if !event.ID.Valid() {
		event.ID = bson.NewObjectId()
	}
	return db.Insert(self.collection, event)
}

func (self *DBEventLogger) MarkProcessed(event *EventLogEntry) error {
	if !event.ID.Valid() {
		return errors.New("event has no ID")
	}
	event.ProcessedAt = time.Now()

	err := db.Update(self.collection, bson.M{
		idKey: event.ID,
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
