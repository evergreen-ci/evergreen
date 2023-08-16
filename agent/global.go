package agent

import (
	"time"
)

const (
	// minAgentSleepInterval is the minimum amount of time an agent sleeps in between
	// polling for a new task if no new task is found
	minAgentSleepInterval = 10 * time.Second

	// maxAgentSleepInterval is the max amount of time an agent sleeps in between
	// polling for a new task if no new task is found
	maxAgentSleepInterval = time.Minute

	// defaultCmdTimeout specifies the duration after which the agent sends
	// an IdleTimeout signal if a task's command does not produce logs on stdout.
	// timeout_secs can be specified only on a command.
	defaultIdleTimeout = 2 * time.Hour

	// DefaultExecTimeout specifies the maximum time a task is allowed to run
	// for, even if it is not idle. This default is used if exec_timeout_secs is
	// not specified in the project file. exec_timeout_secs can be specified
	// only at the project and task level.
	DefaultExecTimeout = 6 * time.Hour

	// defaultHeartbeatInterval is the interval after which agent sends a
	// heartbeat to API server.
	defaultHeartbeatInterval = 30 * time.Second

	// defaultStatsInterval is the interval after which agent sends system stats
	// to API server
	defaultStatsInterval = time.Minute

	// defaultCallbackCmdTimeout specifies the duration after when the "post" or
	// "timeout" block sets should be shut down.
	defaultCallbackCmdTimeout = 15 * time.Minute

	// defaultPreTimeout specifies the default duration after when the pre block
	// should time out and stop the current command.
	defaultPreTimeout = 2 * time.Hour

	// defaultPostTimeout specifies the default duration after when the post
	// block should time out and stop the current command.
	defaultPostTimeout = 2 * time.Hour

	// maxHeartbeats is the number of failed heartbeats after which an agent
	// reports an error
	maxHeartbeats = 10

	// dockerTimeout is the duration to timeout the Docker cleanup that happens
	// after an agent completes a task
	dockerTimeout = 1 * time.Minute
)
