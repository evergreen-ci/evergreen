package model

import (
	"10gen.com/mci/db"
	"labix.org/v2/mgo/bson"
)

type EventFinder struct {
	resourceType string
}

func (self *EventFinder) FindEvents(resourceId string, sort []string,
	limit int) ([]Event, error) {
	events := []Event{}
	err := db.FindAll(
		EventLogCollection,
		bson.M{
			EventResourceIdKey:                            resourceId,
			EventDataKey + "." + EventDataResourceTypeKey: self.resourceType,
		},
		db.NoProjection,
		sort,
		db.NoSkip,
		limit,
		&events,
	)
	return events, err
}

func (self *EventFinder) FindAllEventsInOrder(resourceId string) ([]Event,
	error) {
	return self.FindEvents(
		resourceId,
		[]string{EventTimestampKey},
		db.NoLimit,
	)
}

// returns the last n events for the resource, in reverse order
func (self *EventFinder) FindMostRecentEvents(resourceId string,
	n int) ([]Event, error) {
	return self.FindEvents(
		resourceId,
		[]string{"-" + EventTimestampKey},
		n,
	)
}
