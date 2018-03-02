package event

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
)

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
	if event.Data == nil {
		return errors.New("event log entry cannot have nil Data")
	}
	return db.Insert(self.collection, event)
}
