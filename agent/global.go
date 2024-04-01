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

	// defaultHeartbeatTimeout is how long the agent can perform operations when
	// there is no other applicable timeout before the heartbeat times out.
	defaultHeartbeatTimeout = time.Hour

	// defaultStatsInterval is the interval after which agent sends system stats
	// to API server
	defaultStatsInterval = time.Minute

	// defaultCallbackTimeout specifies the duration after when the timeout
	// block should time out and stop the current command.
	defaultCallbackTimeout = 15 * time.Minute

	// maxTeardownGroupTimeout specifies the duration after when the
	// teardown_group should time out and stop the current command.
	// this cannot be set higher than the evergreen.MaxTeardownGroupThreshold
	// because hosts will be considered idle if they have been tearing down
	// a task group for longer than that time.
	maxTeardownGroupTimeout = 3 * time.Minute

	// defaultPreTimeout specifies the default duration after when the pre,
	// setup_group, or setup_task block should time out and stop the current
	// command.
	defaultPreTimeout = 2 * time.Hour

	// defaultPostTimeout specifies the default duration after when the post or
	// teardown_task block should time out and stop the current command.
	defaultPostTimeout = 30 * time.Minute

	// maxHeartbeats is the number of failed heartbeats after which an agent
	// reports an error
	maxHeartbeats = 10

	// dockerTimeout is the duration to timeout the Docker cleanup that happens
	// after an agent completes a task
	dockerTimeout = 1 * time.Minute

	endTaskMessageLimit = 500
)

type timeoutType string

const (
	execTimeout          timeoutType = "exec"
	idleTimeout          timeoutType = "idle"
	callbackTimeout      timeoutType = "callback"
	preTimeout           timeoutType = "pre"
	postTimeout          timeoutType = "post"
	setupGroupTimeout    timeoutType = "setup_group"
	setupTaskTimeout     timeoutType = "setup_task"
	teardownTaskTimeout  timeoutType = "teardown_task"
	teardownGroupTimeout timeoutType = "teardown_group"
	taskSyncTimeout      timeoutType = "task_sync"
)

// Mode represents a mode that the agent will run in.
type Mode string

const (
	// HostMode indicates that the agent will run in a host.
	HostMode Mode = "host"
	// PodMode indicates that the agent will run in a pod's container.
	PodMode Mode = "pod"
)

// LogOutput represents the output locations for the agent's logs.
type LogOutputType string

const (
	// LogOutputFile indicates that the agent will log to a file.
	LogOutputFile LogOutputType = "file"
	// LogOutputStdout indicates that the agent will log to standard output.
	LogOutputStdout LogOutputType = "stdout"
)

// GetMaxTeardownGroupTimeout returns the maximum teardown group timeout.
func GetMaxTeardownGroupTimeout() time.Duration {
	return maxTeardownGroupTimeout
}
