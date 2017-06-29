package apimodels

import "time"

// for the different types of remote logging
const (
	SystemLogPrefix = "S"
	AgentLogPrefix  = "E"
	TaskLogPrefix   = "T"

	LogErrorPrefix = "E"
	LogWarnPrefix  = "W"
	LogDebugPrefix = "D"
	LogInfoPrefix  = "I"
)

// Also used in the task_logg collection in the database.
type LogMessage struct {
	Type      string    `bson:"t" json:"t"`
	Severity  string    `bson:"s" json:"s"`
	Message   string    `bson:"m" json:"m"`
	Timestamp time.Time `bson:"ts" json:"ts"`
	Version   int       `bson:"v" json:"v"`
}
