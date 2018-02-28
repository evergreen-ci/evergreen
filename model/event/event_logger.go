package event

import "github.com/evergreen-ci/evergreen/db"

type EventLogger interface {
	LogEvent(event *EventLogEntry) error
}

type DBEventLogger struct {
	collection string
}

func NewDBEventLogger(collection string) *DBEventLogger {
	return &DBEventLogger{
		collection: collection,
	}
}

func (self *DBEventLogger) LogEvent(event EventLogEntry) error {
	return db.Insert(self.collection, event)
}
