package testutil

import (
	slogger "github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/model"
)

// NewTestLogger creates a logger for testing. This Logger
// stores everything in memory.
func NewTestLogger(appender slogger.Appender) *comm.StreamLogger {
	return &comm.StreamLogger{
		Local: &slogger.Logger{
			Prefix:    "local",
			Appenders: []slogger.Appender{appender},
		},

		System: &slogger.Logger{
			Prefix:    model.SystemLogPrefix,
			Appenders: []slogger.Appender{appender},
		},

		Task: &slogger.Logger{
			Prefix:    model.TaskLogPrefix,
			Appenders: []slogger.Appender{appender},
		},

		Execution: &slogger.Logger{
			Prefix:    model.AgentLogPrefix,
			Appenders: []slogger.Appender{appender},
		},
	}
}
