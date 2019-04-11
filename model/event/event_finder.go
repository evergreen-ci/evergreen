package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// unprocessedEvents returns a bson.M query to fetch all unprocessed events
func unprocessedEvents() bson.M {
	return bson.M{
		processedAtKey: bson.M{
			"$eq": time.Time{},
		},
	}
}

func ResourceTypeKeyIs(key string) bson.M {
	return bson.M{
		resourceTypeKey: key,
	}
}

// === DB Logic ===

// Find takes a collection storing events and a query, generated
// by one of the query functions, and returns a slice of events.
func Find(coll string, query db.Q) ([]EventLogEntry, error) {
	events := []EventLogEntry{}
	err := db.FindAllQ(coll, query, &events)
	if err != nil || adb.ResultsNotFound(err) {
		return nil, errors.WithStack(err)
	}

	return events, nil
}

// FindUnprocessedEvents returns all unprocessed events in AllLogCollection.
// Events are considered unprocessed if their "processed_at" time IsZero
func FindUnprocessedEvents() ([]EventLogEntry, error) {
	out := []EventLogEntry{}
	err := db.FindAllQ(AllLogCollection, db.Query(unprocessedEvents()).Limit(1000), &out)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch unprocssed events")
	}

	return out, nil
}

func FindLastProcessedEvent() (*EventLogEntry, error) {
	q := db.Query(bson.M{
		processedAtKey: bson.M{
			"$gt": time.Time{},
		},
	}).Sort([]string{"-" + processedAtKey})

	e := EventLogEntry{}
	if err := db.FindOneQ(AllLogCollection, q, &e); err != nil {
		if adb.ResultsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to fetch most recently processed event")
	}

	return &e, nil
}

func CountUnprocessedEvents() (int, error) {
	q := db.Query(unprocessedEvents())

	n, err := db.CountQ(AllLogCollection, q)
	if err != nil {
		return 0, errors.Wrap(err, "failed to fetch number of unprocessed events")
	}

	return n, nil
}

// === Queries ===

// Host Events
func HostEventsForId(id string) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeHost)
	filter[ResourceIdKey] = id

	return db.Query(filter)
}

func MostRecentHostEvents(id string, n int) db.Q {
	return HostEventsForId(id).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func HostEventsInOrder(id string) db.Q {
	return HostEventsForId(id).Sort([]string{TimestampKey})
}

// Task Events
func TaskEventsForId(id string) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeTask)
	filter[ResourceIdKey] = id

	return db.Query(filter)
}

func MostRecentTaskEvents(id string, n int) db.Q {
	return TaskEventsForId(id).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func TaskEventsInOrder(id string) db.Q {
	return TaskEventsForId(id).Sort([]string{TimestampKey})
}

// Distro Events
func DistroEventsForId(id string) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeDistro)
	filter[ResourceIdKey] = id

	return db.Query(filter)
}

func MostRecentDistroEvents(id string, n int) db.Q {
	return DistroEventsForId(id).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func DistroEventsInOrder(id string) db.Q {
	return DistroEventsForId(id).Sort([]string{TimestampKey})
}

// Scheduler Events
func SchedulerEventsForId(distroID string) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeScheduler)
	filter[ResourceIdKey] = distroID

	return db.Query(filter)
}

func RecentSchedulerEvents(distroId string, n int) db.Q {
	return SchedulerEventsForId(distroId).Sort([]string{"-" + TimestampKey}).Limit(n)
}

// Admin Events
// RecentAdminEvents returns the N most recent admin events
func RecentAdminEvents(n int) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeAdmin)
	filter[ResourceIdKey] = ""

	return db.Query(filter).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func ByGuid(guid string) db.Q {
	return db.Query(bson.M{
		bsonutil.GetDottedKeyName(DataKey, "guid"): guid,
	})
}

func AdminEventsBefore(before time.Time, n int) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeAdmin)
	filter[ResourceIdKey] = ""
	filter[TimestampKey] = bson.M{
		"$lt": before,
	}

	return db.Query(filter).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func FindAllByResourceID(resourceID string) ([]EventLogEntry, error) {
	return Find(AllLogCollection, db.Query(bson.M{ResourceIdKey: resourceID}))
}
