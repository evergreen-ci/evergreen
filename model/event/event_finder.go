package event

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
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
func Find(ctx context.Context, query db.Q) ([]EventLogEntry, error) {
	events := []EventLogEntry{}
	err := db.FindAllQ(ctx, EventCollection, query, &events)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return events, nil
}

func FindPaginatedWithTotalCount(ctx context.Context, query db.Q, limit, page int) ([]EventLogEntry, int, error) {
	events := []EventLogEntry{}
	skip := page * limit
	if skip > 0 {
		query = query.Skip(skip)
	}

	err := db.FindAllQ(ctx, EventCollection, query, &events)
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	// Count ignores skip and limit by default, so this will return the total number of events.
	count, err := db.CountQ(ctx, EventCollection, query)
	if err != nil {
		return nil, 0, errors.Wrap(err, "fetching total count for events")
	}

	return events, count, nil
}

// FindUnprocessedEvents returns all unprocessed events in EventCollection.
// Events are considered unprocessed if their "processed_at" time IsZero
func FindUnprocessedEvents(ctx context.Context, limit int) ([]EventLogEntry, error) {
	out := []EventLogEntry{}
	query := db.Query(unprocessedEvents()).Sort([]string{TimestampKey})
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := db.FindAllQ(ctx, EventCollection, query, &out)
	if err != nil {
		return nil, errors.Wrap(err, "fetching unprocessed events")
	}

	return out, nil
}

// FindByID finds a single event matching the given event ID.
func FindByID(ctx context.Context, eventID string) (*EventLogEntry, error) {
	query := bson.M{
		idKey: eventID,
	}

	var e EventLogEntry
	if err := db.FindOneQ(ctx, EventCollection, db.Query(query), &e); err != nil {
		if adb.ResultsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "finding event by ID")
	}
	return &e, nil
}

func FindLastProcessedEvent(ctx context.Context) (*EventLogEntry, error) {
	q := db.Query(bson.M{
		processedAtKey: bson.M{
			"$gt": time.Time{},
		},
	}).Sort([]string{"-" + processedAtKey})

	e := EventLogEntry{}
	if err := db.FindOneQ(ctx, EventCollection, q, &e); err != nil {
		if adb.ResultsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "fetching most recently processed event")
	}

	return &e, nil
}

func CountUnprocessedEvents(ctx context.Context) (int, error) {
	q := db.Query(unprocessedEvents())

	n, err := db.CountQ(ctx, EventCollection, q)
	if err != nil {
		return 0, errors.Wrap(err, "fetching number of unprocessed events")
	}

	return n, nil
}

// === Queries ===

// HostEventsOpts represent filter arguments to the HostEvents function.
type HostEventsOpts struct {
	ID         string
	Tag        string
	Limit      int
	SortAsc    bool
	EventTypes []string
}

// HostEvents builds a query that can be used to return the n = opts.Limit events that satisfy the
// filters and sorting method provided in opts.
func HostEvents(opts HostEventsOpts) db.Q {
	filter := ResourceTypeKeyIs(ResourceTypeHost)
	if opts.Tag != "" {
		filter[ResourceIdKey] = bson.M{"$in": []string{opts.ID, opts.Tag}}
	} else {
		filter[ResourceIdKey] = opts.ID
	}
	if len(opts.EventTypes) > 0 {
		filter[eventTypeKey] = bson.M{"$in": opts.EventTypes}
	}
	sortMethod := []string{"-" + TimestampKey}
	if opts.SortAsc {
		sortMethod = []string{TimestampKey}
	}
	return db.Query(filter).Sort(sortMethod).Limit(opts.Limit)
}

// PaginatedHostEventsOpts represent filter arguments to the GetPaginatedHostEvents function.
type PaginatedHostEventsOpts struct {
	ID         string
	Tag        string
	Limit      int
	Page       int
	SortAsc    bool
	EventTypes []string
}

// GetPaginatedHostEvents returns a limited and paginated list of host events for the given
// filters sorted in ascending or descending order by timestamp, as well as the total number
// of host events.
func GetPaginatedHostEvents(ctx context.Context, opts PaginatedHostEventsOpts) ([]EventLogEntry, int, error) {
	queryOpts := HostEventsOpts{
		ID:         opts.ID,
		Tag:        opts.Tag,
		Limit:      opts.Limit,
		SortAsc:    opts.SortAsc,
		EventTypes: opts.EventTypes,
	}
	hostEventsQuery := HostEvents(queryOpts)
	return FindPaginatedWithTotalCount(ctx, hostEventsQuery, opts.Limit, opts.Page)
}

type eventTypeResult struct {
	EventTypes []string `bson:"event_types"`
}

// GetEventTypesForHost returns the event types that have occurred on the host.
func GetEventTypesForHost(ctx context.Context, hostID string, tag string) ([]string, error) {
	filter := ResourceTypeKeyIs(ResourceTypeHost)
	if tag != "" {
		filter[ResourceIdKey] = bson.M{"$in": []string{hostID, tag}}
	} else {
		filter[ResourceIdKey] = hostID
	}

	pipeline := []bson.M{
		{"$match": filter},
		{
			"$group": bson.M{
				"_id":         nil,
				"event_types": bson.M{"$addToSet": "$" + eventTypeKey},
			},
		},
		{
			"$project": bson.M{
				"event_types": bson.M{
					"$sortArray": bson.M{
						"input":  "$event_types",
						"sortBy": 1,
					},
				},
			},
		},
	}

	out := []eventTypeResult{}
	if err := db.Aggregate(ctx, EventCollection, pipeline, &out); err != nil {
		return nil, errors.Errorf("finding event types for host '%s': %s", hostID, err)
	}
	if len(out) == 0 {
		return []string{}, nil
	}
	return out[0].EventTypes, nil
}

// HasNoRecentStoppedHostEvent returns true if no host event exists that is more recent than the passed in time stamp.
func HasNoRecentStoppedHostEvent(ctx context.Context, id string, ts time.Time) (bool, error) {
	filter := ResourceTypeKeyIs(ResourceTypeHost)
	filter[ResourceIdKey] = id
	filter[eventTypeKey] = EventHostStopped
	filter[TimestampKey] = bson.M{"$gte": ts}
	count, err := db.CountQ(ctx, EventCollection, db.Query(filter))
	if err != nil {
		return false, errors.Wrap(err, "fetching count of stopped host events")
	}
	return count == 0, nil
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
func FindLatestPrimaryDistroEvents(ctx context.Context, id string, n int, before time.Time) ([]EventLogEntry, error) {
	events := []EventLogEntry{}
	err := db.Aggregate(ctx, EventCollection, latestDistroEventsPipeline(id, n, false, before), &events)
	if err != nil {
		return nil, err
	}
	return events, err
}

// FindLatestAMIModifiedDistroEvent returns the most recent AMI event. Returns an empty struct if nothing exists.
func FindLatestAMIModifiedDistroEvent(ctx context.Context, id string) (EventLogEntry, error) {
	events := []EventLogEntry{}
	res := EventLogEntry{}
	err := db.Aggregate(ctx, EventCollection, latestDistroEventsPipeline(id, 1, true, time.Now()), &events)
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

	if !utility.IsZeroTime(before) {
		resourceFilter[TimestampKey] = bson.M{"$lt": before}
	}

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
	if !utility.IsZeroTime(before) {
		filter[TimestampKey] = bson.M{
			"$lt": before,
		}
	}
	return db.Query(filter).Sort([]string{"-" + TimestampKey}).Limit(n)
}

// FindLatestAdminEvents return the n most recent admin events before the given timestamp.
func FindLatestAdminEvents(ctx context.Context, n int, before time.Time) ([]EventLogEntry, error) {
	events, err := FindAdmin(ctx, AdminEventsBefore(before, n))
	if err != nil {
		return nil, err
	}
	return events, err
}

func FindAllByResourceID(ctx context.Context, resourceID string) ([]EventLogEntry, error) {
	return Find(ctx, db.Query(bson.M{ResourceIdKey: resourceID}))
}
