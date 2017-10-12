package agent

import (
	"time"
)

const (
	// agentSleepInterval is the amount of time an agent sleeps in between
	// polling for a new task if no new task is found
	defaultAgentSleepInterval = 30 * time.Second

	// defaultCmdTimeout specifies the duration after which the agent sends
	// an IdleTimeout signal if a task's command does not produce logs on stdout.
	// timeout_secs can be specified only on a command.
	defaultIdleTimeout = 2 * time.Hour

	// defaultExecTimeoutSecs specifies in seconds the maximum time a task
	// is allowed to run for, even if it is not idle. This default is used
	// if exec_timeout_secs is not specified in the project file.
	// exec_timeout_secs can be specified only at the project and task level.
	defaultExecTimeoutSecs = 60 * 60 * 6

	// defaultHeartbeatInterval is the interval after which agent sends a
	// heartbeat to API server.
	defaultHeartbeatInterval = 30 * time.Second

	// defaultStatsInterval is the interval after which agent sends system stats
	// to API server
	defaultStatsInterval = time.Minute

	// defaultCallbackCmdTimeout specifies the duration after when the "post" or
	// "timeout" command sets should be shut down.
	defaultCallbackCmdTimeout = 15 * time.Minute

	// maxHeartbeats is the number of failed heartbeats after which an agent
	// reports an error
	maxHeartbeats = 10

	// setupTimeout is how long the agent will wait before timing out running
	// the setup script.
	setupTimeout = 1 * time.Minute
)
