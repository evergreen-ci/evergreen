package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var eventCollections = []string{LegacyEventLogCollection, EventCollection}

func (e *EventLogEntry) Log() error {
	if err := e.validateEvent(); err != nil {
		return errors.Wrap(err, "not logging event, event is invalid")
	}

	catcher := grip.NewBasicCatcher()
	for _, col := range eventCollections {
		catcher.Add(db.Insert(col, e))
	}

	return catcher.Resolve()
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

	catcher := grip.NewBasicCatcher()
	for _, col := range eventCollections {
		catcher.Add(errors.Wrap(db.Update(col, filter, update), "updating 'processed at' time"))
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
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

	for _, col := range eventCollections {
		catcher.Add(db.InsertMany(col, events))
	}

	return catcher.Resolve()
}
