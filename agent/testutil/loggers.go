package testutil

import (
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/slogger"
)

// NewTestLogger creates a logger for testing. This Logger
// stores everything in memory.
func NewTestLogger(sender send.Sender) *comm.StreamLogger {
	return &comm.StreamLogger{
		Local: &slogger.Logger{
			Name:      "local",
			Appenders: []send.Sender{sender},
		},

		System: &slogger.Logger{
			Name:      model.SystemLogPrefix,
			Appenders: []send.Sender{sender},
		},

		Task: &slogger.Logger{
			Name:      model.TaskLogPrefix,
			Appenders: []send.Sender{sender},
		},

		Execution: &slogger.Logger{
			Name:      model.AgentLogPrefix,
			Appenders: []send.Sender{sender},
		},
	}
}
