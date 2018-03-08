package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2/bson"
)

var embeddedResourceTypeKey = DataKey + "." + resourceTypeKey

func eitherResourceTypeKeyIs(key string) bson.M {
	return bson.M{
		"$or": []bson.M{
			{
				embeddedResourceTypeKey: key,
			},
			{
				resourceTypeKey: key,
			},
		},
	}
}

// === DB Logic ===

// Find takes a collection storing events and a query, generated
// by one of the query functions, and returns a slice of events.
func Find(coll string, query db.Q) ([]EventLogEntry, error) {
	events := []EventLogEntry{}
	err := db.FindAllQ(coll, query, &events)
	return events, err
}

// CountSystemEvents returns the total number of system metrics events
// captured for the specified task. If taskId is "", then this will
// return a count of all system events captured.
func CountSystemEvents(taskId string) (int, error) {
	filter := eitherResourceTypeKeyIs(EventTaskSystemInfo)
	filter[TypeKey] = EventTaskSystemInfo

	if taskId != "" {
		filter[ResourceIdKey] = taskId
	}

	return db.CountQ(TaskLogCollection, db.Query(filter))
}

// CountProcessEvents returns the total number of process tree metrics events
// captured for the specified task. If taskId is "", then this will
// return a count of all process metrics captured.
func CountProcessEvents(taskID string) (int, error) {
	filter := eitherResourceTypeKeyIs(EventTaskProcessInfo)
	filter[TypeKey] = EventTaskProcessInfo

	if taskID != "" {
		filter[ResourceIdKey] = taskID
	}

	return db.CountQ(TaskLogCollection, db.Query(filter))
}

// === Queries ===

// Host Events
func HostEventsForId(id string) db.Q {
	filter := eitherResourceTypeKeyIs(ResourceTypeHost)
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
	filter := eitherResourceTypeKeyIs(ResourceTypeTask)
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
	filter := eitherResourceTypeKeyIs(ResourceTypeDistro)
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
	filter := eitherResourceTypeKeyIs(ResourceTypeScheduler)
	filter[ResourceIdKey] = distroID

	return db.Query(filter)
}

func RecentSchedulerEvents(distroId string, n int) db.Q {
	return SchedulerEventsForId(distroId).Sort([]string{"-" + TimestampKey}).Limit(n)
}

// Admin Events
// RecentAdminEvents returns the N most recent admin events
func RecentAdminEvents(n int) db.Q {
	filter := eitherResourceTypeKeyIs(ResourceTypeAdmin)
	filter[ResourceIdKey] = ""

	return db.Query(filter).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func ByGuid(guid string) db.Q {
	return db.Query(bson.M{
		bsonutil.GetDottedKeyName(DataKey, "guid"): guid,
	})
}

func AdminEventsBefore(before time.Time, n int) db.Q {
	return db.Query(bson.M{
		DataKey + "." + resourceTypeKey: ResourceTypeAdmin,
		ResourceIdKey:                   "",
		TimestampKey: bson.M{
			"$lt": before,
		},
	}).Sort([]string{"-" + TimestampKey}).Limit(n)
}

// TaskSystemInfoEvents builds a query for system info,
// (e.g. aggregate information about the system as a whole) collected
// during a task.
//
// If the sort value is less than 0, the query will return all
// matching events that occur before the specified time, and otherwise
// will return all matching events that occur after the specified time.
func TaskSystemInfoEvents(taskID string, ts time.Time, limit, sort int) db.Q {
	filter := eitherResourceTypeKeyIs(EventTaskSystemInfo)
	filter[ResourceIdKey] = taskID
	filter[TypeKey] = EventTaskSystemInfo

	sortSpec := TimestampKey

	if sort < 0 {
		sortSpec = "-" + sortSpec
		filter[TimestampKey] = bson.M{"$lte": ts}
	} else {
		filter[TimestampKey] = bson.M{"$gte": ts}
	}

	return db.Query(filter).Sort([]string{sortSpec}).Limit(limit)
}

// TaskProcessInfoEvents builds a query for process info, which
// returns information about each process (and children) spawned
// during task execution.
//
// If the sort value is less than 0, the query will return all
// matching events that occur before the specified time, and otherwise
// will return all matching events that occur after the specified time.
func TaskProcessInfoEvents(taskID string, ts time.Time, limit, sort int) db.Q {
	filter := eitherResourceTypeKeyIs(EventTaskProcessInfo)
	filter[ResourceIdKey] = taskID
	filter[TypeKey] = EventTaskProcessInfo

	sortSpec := TimestampKey

	if sort < 0 {
		sortSpec = "-" + sortSpec
		filter[TimestampKey] = bson.M{"$lte": ts}
	} else {
		filter[TimestampKey] = bson.M{"$gte": ts}
	}

	return db.Query(filter).Sort([]string{sortSpec}).Limit(limit)
}
