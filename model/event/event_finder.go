package event

import (
	"10gen.com/mci/db"
	"labix.org/v2/mgo/bson"
)

// === DB Logic ===

func Find(query db.Q) ([]Event, error) {
	events := []Event{}
	err := db.FindAllQ(Collection, query, &events)
	return events, err
}

// === Queries ===

func HostEventsForId(id string) db.Q {
	return db.Query(bson.D{
		{DataKey + "." + ResourceTypeKey, ResourceTypeHost},
		{ResourceIdKey, id},
	})
}

func MostRecentHostEvents(id string, n int) db.Q {
	return HostEventsForId(id).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func HostEventsInOrder(id string) db.Q {
	return HostEventsForId(id).Sort([]string{TimestampKey})
}

func TaskEventsForId(id string) db.Q {
	return db.Query(bson.D{
		{DataKey + "." + ResourceTypeKey, ResourceTypeTask},
		{ResourceIdKey, id},
	})
}

func MostRecentTaskEvents(id string, n int) db.Q {
	return TaskEventsForId(id).Sort([]string{"-" + TimestampKey}).Limit(n)
}

func TaskEventsInOrder(id string) db.Q {
	return TaskEventsForId(id).Sort([]string{TimestampKey})
}
