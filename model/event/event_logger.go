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

func (e *EventLogEntry) Log(ctx context.Context) error {
	if err := e.validateEvent(); err != nil {
		return errors.Wrap(err, "not logging event, event is invalid")
	}

	return errors.Wrap(db.Insert(ctx, EventCollection, e), "inserting event")
}

func (e *EventLogEntry) MarkProcessed(ctx context.Context) error {
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

	if err := db.UpdateContext(ctx, EventCollection, filter, update); err != nil {
		return errors.Wrap(err, "updating 'processed at' time")
	}

	e.ProcessedAt = processedAt
	return nil
}

func LogManyEvents(ctx context.Context, events []EventLogEntry) error {
	catcher := grip.NewBasicCatcher()
	interfaces := make([]any, len(events))
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

	return db.InsertMany(ctx, EventCollection, interfaces...)
}

// LogManyUnorderedEventsWithContext logs many events without any ordering on
// insertion. Do not use this if the events must be inserted in order.
func LogManyUnorderedEventsWithContext(ctx context.Context, events []EventLogEntry) error {
	catcher := grip.NewBasicCatcher()
	interfaces := make([]any, len(events))
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
