package proto

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
)

const (
	// agentSleepInterval is the amount of time an agent sleeps in between
	// polling for a new task if no new task is found
	agentSleepInterval = 30 * time.Second

	// defaultCmdTimeout specifies the duration after which agent sends
	// an IdleTimeout signal if a task's command does not run to completion.
	defaultCmdTimeout = 2 * time.Hour

	// defaultExecTimeoutSecs specifies in seconds the maximum time a task
	// is allowed to run for, even if it is not idle. This default is used
	// if exec_timeout_secs is not specified in the project file.
	defaultExecTimeoutSecs = 60 * 60 * 6

	// defaultIdleTimeout specifies the duration after which agent sends an
	// IdleTimeout signal if a task produces no logs.
	defaultIdleTimeout = 20 * time.Minute

	// defaultHeartbeatInterval is the interval after which agent sends a
	// heartbeat to API server.
	heartbeatInterval = 30 * time.Second

	// defaultStatsInterval is the interval after which agent sends system stats
	// to API server
	defaultStatsInterval = time.Minute

	// defaultCallbackCmdTimeout specifies the duration after when the "post" or
	// "timeout" command sets should be shut down.
	defaultCallbackCmdTimeout = 15 * time.Minute

	// maxHeartbeats is the number of failed heartbeats after which an agent
	// reports an error
	maxHeartbeats = 10

	// initialSetupTimeout indicates the time allowed for the agent to collect
	// relevant information - for running a task - from the API server.
	initialSetupTimeout = 20 * time.Minute

	initialSetupCommandDisplayName = "initial task setup"
	initialSetupCommandType        = model.SystemCommandType
)
