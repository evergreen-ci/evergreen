package apimodels

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

const (
	SystemLogPrefix  = "S"
	AgentLogPrefix   = "E"
	TaskLogPrefix    = "T"
	AllTaskLevelLogs = "ALL"

	LogErrorPrefix = "E"
	LogWarnPrefix  = "W"
	LogDebugPrefix = "D"
	LogInfoPrefix  = "I"
)

// The LogMessage type is used by the the GraphQL resolver and HTML logs.
type LogMessage struct {
	Type      string    `bson:"t" json:"t"`
	Severity  string    `bson:"s" json:"s"`
	Message   string    `bson:"m" json:"m"`
	Timestamp time.Time `bson:"ts" json:"ts"`
	Version   int       `bson:"v" json:"v"`
}

func GetSeverityMapping(s level.Priority) string {
	switch {
	case s >= level.Error:
		return LogErrorPrefix
	case s >= level.Warning:
		return LogWarnPrefix
	case s >= level.Info:
		return LogInfoPrefix
	case s < level.Info:
		return LogDebugPrefix
	default:
		return LogInfoPrefix
	}
}

// ReadLogToSlice returns a slice of log message pointers from a log iterator.
func ReadLogToSlice(it log.LogIterator) ([]*LogMessage, error) {
	var lines []*LogMessage
	for it.Next() {
		item := it.Item()
		lines = append(lines, &LogMessage{
			Severity:  GetSeverityMapping(item.Priority),
			Message:   item.Data,
			Timestamp: time.Unix(0, item.Timestamp),
		})
	}
	if err := it.Err(); err != nil {
		return nil, errors.Wrap(err, "iterating log lines")
	}

	return lines, errors.Wrap(it.Close(), "closing log iterator")
}

// StreamFromLogIterator streams log lines from the given iterator to the
// returned log message channel. It is the responsibility of the caller to
// close the log iterator.
func StreamFromLogIterator(it log.LogIterator) chan LogMessage {
	lines := make(chan LogMessage)
	go func() {
		defer recovery.LogStackTraceAndContinue("streaming lines from log iterator")
		defer close(lines)

		for it.Next() {
			item := it.Item()
			lines <- LogMessage{
				Severity:  GetSeverityMapping(item.Priority),
				Message:   item.Data,
				Timestamp: time.Unix(0, item.Timestamp),
			}
		}

		if err := it.Err(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "streaming lines from log iterator",
			}))
		}
	}()

	return lines
}
