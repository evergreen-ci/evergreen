package event

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (e *EventLogEntry) Log() error {
	if err := e.validateEvent(); err != nil {
		return errors.Wrap(err, "not logging event, event is invalid")
	}

	return errors.Wrap(db.Insert(EventCollection, e), "inserting event")
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

	if err := db.Update(EventCollection, filter, update); err != nil {
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

	return db.InsertMany(EventCollection, interfaces...)
}

func LogManyEventsWithContext(ctx context.Context, events []EventLogEntry) error {
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

	_, err := evergreen.GetEnvironment().DB().Collection(EventCollection).InsertMany(ctx, interfaces, options.InsertMany().SetOrdered(false))
	return err
}
