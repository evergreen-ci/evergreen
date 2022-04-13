package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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

func (l *DBEventLogger) LogEvent(e *EventLogEntry) error {
	if err := e.validateEvent(); err != nil {
		return errors.Wrap(err, "not logging event, event is invalid")
	}
	return db.Insert(l.collection, e)
}

func (l *DBEventLogger) LogManyEvents(events []EventLogEntry) error {
	catcher := grip.NewBasicCatcher()
	interfaces := make([]interface{}, len(events))
	for i := range events {
		e := &events[i]
		if err := e.validateEvent(); err != nil {
			catcher.Add(err)
			continue
		}
		interfaces[i] = &events[i]
	}
	if catcher.HasErrors() {
		return errors.Wrap(catcher.Resolve(), "invalid events")
	}
	return db.InsertMany(l.collection, interfaces...)
}

func (l *DBEventLogger) MarkProcessed(event *EventLogEntry) error {
	if event.ID == "" {
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
		return errors.Wrap(err, "updating 'processed at' time")
	}

	return nil
}
