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
func Find(query db.Q) ([]EventLogEntry, error) {
	events := []EventLogEntry{}
	err := db.FindAllQ(EventCollection, query, &events)
	if err != nil || adb.ResultsNotFound(err) {
		return nil, errors.WithStack(err)
	}

	return events, nil
}

func FindPaginatedWithTotalCount(query db.Q, limit, page int) ([]EventLogEntry, int, error) {
	events := []EventLogEntry{}
	skip := page * limit
	if skip > 0 {
		query = query.Skip(skip)
	}

	err := db.FindAllQ(EventCollection, query, &events)
	if err != nil || adb.ResultsNotFound(err) {
		return nil, 0, errors.WithStack(err)
	}

	// Count ignores skip and limit by default, so this will return the total number of events.
	count, err := db.CountQ(EventCollection, query)
	if err != nil {
		return nil, 0, errors.Wrap(err, "fetching total count for events")
	}

	return events, count, nil
}

// FindUnprocessedEvents returns all unprocessed events in EventCollection.
// Events are considered unprocessed if their "processed_at" time IsZero
func FindUnprocessedEvents(limit int) ([]EventLogEntry, error) {
	out := []EventLogEntry{}
	query := db.Query(unprocessedEvents()).Sort([]string{TimestampKey})
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := db.FindAllQ(EventCollection, query, &out)
	if err != nil {
		return nil, errors.Wrap(err, "fetching unprocessed events")
	}

	return out, nil
}

// FindByID finds a single event matching the given event ID.
func FindByID(eventID string) (*EventLogEntry, error) {
	query := bson.M{
		idKey: eventID,
	}

	var e EventLogEntry
	if err := db.FindOneQ(EventCollection, db.Query(query), &e); err != nil {
		if adb.ResultsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "finding event by ID")
	}
	return &e, nil
}

func FindLastProcessedEvent() (*EventLogEntry, error) {
	q := db.Query(bson.M{
		processedAtKey: bson.M{
			"$gt": time.Time{},
		},
	}).Sort([]string{"-" + processedAtKey})

	e := EventLogEntry{}
	if err := db.FindOneQ(EventCollection, q, &e); err != nil {
		if adb.ResultsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "fetching most recently processed event")
	}

	return &e, nil
}

func CountUnprocessedEvents() (int, error) {
	q := db.Query(unprocessedEvents())

	n, err := db.CountQ(EventCollection, q)
	if err != nil {
		return 0, errors.Wrap(err, "fetching number of unprocessed events")
	}

	return n, nil
}

// === Queries ===

// Host Events
func MostRecentHostEvents(id string, tag string, n int) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeHost)
	if tag != "" {
		filter[ResourceIdKey] = bson.M{"$in": []string{id, tag}}
	} else {
		filter[ResourceIdKey] = id
	}

	return db.Query(filter).Sort([]string{"-" + TimestampKey}).Limit(n)
}

// MostRecentPaginatedHostEvents returns a limited and paginated list of host events for the given
// host ID and tag sorted in descending order by timestamp as well as the total number of events.
func MostRecentPaginatedHostEvents(id string, tag string, limit, page int) ([]EventLogEntry, int, error) {
	recentHostsQuery := MostRecentHostEvents(id, tag, limit)
	return FindPaginatedWithTotalCount(recentHostsQuery, limit, page)
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

// FindLatestPrimaryDistroEvents return the most recent non-AMI events for the distro.
// The before parameter returns only events before the specified time and is used for pagination on the UI.
func FindLatestPrimaryDistroEvents(id string, n int, before time.Time) ([]EventLogEntry, error) {
	events := []EventLogEntry{}
	err := db.Aggregate(EventCollection, latestDistroEventsPipeline(id, n, false, before), &events)
	if err != nil {
		return nil, err
	}
	return events, err
}

// FindLatestAMIModifiedDistroEvent returns the most recent AMI event. Returns an empty struct if nothing exists.
func FindLatestAMIModifiedDistroEvent(id string) (EventLogEntry, error) {
	events := []EventLogEntry{}
	res := EventLogEntry{}
	err := db.Aggregate(EventCollection, latestDistroEventsPipeline(id, 1, true, time.Now()), &events)
	if err != nil {
		return res, err
	}
	if len(events) > 0 {
		res = events[0]
	}
	return res, nil
}

func latestDistroEventsPipeline(id string, n int, amiOnly bool, before time.Time) []bson.M {
	// We use two different match stages to use the most efficient index.
	resourceFilter := ResourceTypeKeyIs(ResourceTypeDistro)
	resourceFilter[ResourceIdKey] = id
	resourceFilter[TimestampKey] = bson.M{"$lt": before}

	var eventFilter = bson.M{}
	if amiOnly {
		eventFilter[TypeKey] = EventDistroAMIModfied
	} else {
		eventFilter[TypeKey] = bson.M{"$ne": EventDistroAMIModfied}
	}
	return []bson.M{
		{"$match": resourceFilter},
		{"$sort": bson.M{TimestampKey: -1}},
		{"$match": eventFilter},
		{"$limit": n},
	}
}

// Admin Events
// RecentAdminEvents returns the N most recent admin events
func RecentAdminEvents(n int) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeAdmin)
	filter[ResourceIdKey] = ""

	return db.Query(filter).Sort([]string{"-" + TimestampKey}).Limit(n)
}

// ByAdminGuid returns a query for the admin events with the given guid.
func ByAdminGuid(guid string) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeAdmin)
	filter[ResourceIdKey] = ""
	filter[bsonutil.GetDottedKeyName(DataKey, "guid")] = guid
	return db.Query(filter)
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
	return Find(db.Query(bson.M{ResourceIdKey: resourceID}))
}

// Pod events

// MostRecentPodEvents creates a query to find the n most recent pod events for
// the given pod ID.
func MostRecentPodEvents(id string, n int) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypePod)
	filter[ResourceIdKey] = id

	return db.Query(filter).Sort([]string{"-" + TimestampKey}).Limit(n)
}

// MostRecentPaginatedPodEvents returns a limited and paginated list of pod events for the
// given pod ID sorted in descending order by timestamp as well as the total number of events.
func MostRecentPaginatedPodEvents(id string, limit, page int) ([]EventLogEntry, int, error) {
	recentPodsQuery := MostRecentPodEvents(id, limit)
	return FindPaginatedWithTotalCount(recentPodsQuery, limit, page)
}
