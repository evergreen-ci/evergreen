package event

import (
	"github.com/evergreen-ci/evergreen/db"
)

type EventLogger interface {
	LogEvent(event *Event) error
}

type DBEventLogger struct {
	collection string
}

func NewDBEventLogger(collection string) *DBEventLogger {
	return &DBEventLogger{
		collection: collection,
	}
}

func (self *DBEventLogger) LogEvent(event Event) error {
	return db.Insert(self.collection, event)
}
