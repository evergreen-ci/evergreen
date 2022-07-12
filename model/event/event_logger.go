package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func (e *EventLogEntry) Log() error {
	if err := e.validateEvent(); err != nil {
		return errors.Wrap(err, "not logging event, event is invalid")
	}

	grip.Error(message.WrapError(db.Insert(EventCollection, e), message.Fields{
		"message":       "inserting event",
		"collection":    EventCollection,
		"resource_type": e.ResourceType,
		"event_type":    e.EventType,
		"event_id":      e.ID,
	}))

	return errors.Wrap(db.Insert(LegacyEventLogCollection, e), "inserting event")
}

func (e *EventLogEntry) MarkProcessed() error {
	if e.ID == "" {
		return errors.New("event has no ID")
	}
	processedAt := time.Now()

	filter := bson.M{
		idKey: e.ID,
		processedAtKey: bson.M{
			"$eq": time.Time{},
		},
	}
	update := bson.M{
		"$set": bson.M{
			processedAtKey: processedAt,
		},
	}

	grip.Error(message.WrapError(db.Update(EventCollection, filter, update), message.Fields{
		"message":    "marking event processed",
		"collection": EventCollection,
		"event_id":   e.ID,
	}))

	if err := db.Update(LegacyEventLogCollection, filter, update); err != nil {
		return errors.Wrap(err, "updating 'processed at' time")
	}

	e.ProcessedAt = processedAt
	return nil
}

func LogManyEvents(events []EventLogEntry) error {
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

	grip.Error(message.WrapError(db.InsertMany(EventCollection, interfaces...), message.Fields{
		"message":    "inserting many events",
		"collection": EventCollection,
	}))

	return db.InsertMany(LegacyEventLogCollection, interfaces...)
}
