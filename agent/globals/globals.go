package globals

import (
	"time"
)

const (
	// MinAgentSleepInterval is the minimum amount of time an agent sleeps in between
	// polling for a new task if no new task is found
	MinAgentSleepInterval = 10 * time.Second

	// MaxAgentSleepInterval is the max amount of time an agent sleeps in between
	// polling for a new task if no new task is found
	MaxAgentSleepInterval = time.Minute

	// DefaultIdleTimeout specifies the duration after which the agent sends
	// an IdleTimeout signal if a task's command does not produce logs on stdout.
	// timeout_secs can be specified only on a command.
	DefaultIdleTimeout = 2 * time.Hour

	// DefaultExecTimeout specifies the maximum time a task is allowed to run
	// for, even if it is not idle. This default is used if exec_timeout_secs is
	// not specified in the project file. exec_timeout_secs can be specified
	// only at the project and task level.
	DefaultExecTimeout = 6 * time.Hour

	// DefaultHeartbeatInterval is the interval after which agent sends a
	// heartbeat to API server.
	DefaultHeartbeatInterval = 30 * time.Second

	// DefaultHeartbeatTimeout is how long the agent can perform operations when
	// there is no other applicable timeout before the heartbeat times out.
	DefaultHeartbeatTimeout = time.Hour

	// DefaultStatsInterval is the interval after which agent sends system stats
	// to API server
	DefaultStatsInterval = time.Minute

	// DefaultCallbackTimeout specifies the duration after when the timeout
	// block should time out and stop the current command.
	DefaultCallbackTimeout = 15 * time.Minute

	// DefaultPreTimeout specifies the default duration after when the pre,
	// setup_group, or setup_task block should time out and stop the current
	// command.
	DefaultPreTimeout = 2 * time.Hour

	// maxTeardownGroupTimeout specifies the duration after when the
	// teardown_group should time out and stop the current command.
	// this cannot be set higher than the evergreen.MaxTeardownGroupThreshold
	// because hosts will be considered idle if they have been tearing down
	// a task group for longer than that time.
	MaxTeardownGroupTimeout = 3 * time.Minute

	// DefaultPostTimeout specifies the default duration after when the post or
	// teardown_task block should time out and stop the current command.
	DefaultPostTimeout = 30 * time.Minute

	// MaxHeartbeats is the number of failed heartbeats after which an agent
	// reports an error
	MaxHeartbeats = 10

	// DockerTimeout is the duration to timeout the Docker cleanup that happens
	// after an agent completes a task
	DockerTimeout = 1 * time.Minute

	// EndTaskMessageLimit is the length limit of a user-defined end task response.
	EndTaskMessageLimit = 500
	// MaxPercentageDataVolumeUsage is the maximum percentage of usage on the
	// data volume that the agent permits after running a task and cleaning up
	// after it. If the current disk usage still exceeds this threshold after
	// task cleanup, there is too little disk space to run a new task and so it
	// is considered unhealthy.
	// Excessive disk usage could happen in some scenarios where the agent can't
	// clean up state left behind by previous tasks, like if the task writes a
	// lot of data outside of the task directory or if the agent fails to clean
	// up the task directory.
	// Mac hosts have have larger volumes and are more limited. It can therefore
	// tolerate a higher threshold.
	MaxPercentageDataVolumeUsageDefault = 50
	MaxPercentageDataVolumeUsageDarwin  = 80
)

// TimeoutType indicates the type of task timeout.
type TimeoutType string

const (
	ExecTimeout          TimeoutType = "exec"
	IdleTimeout          TimeoutType = "idle"
	CallbackTimeout      TimeoutType = "callback"
	PreTimeout           TimeoutType = "pre"
	PostTimeout          TimeoutType = "post"
	SetupGroupTimeout    TimeoutType = "setup_group"
	SetupTaskTimeout     TimeoutType = "setup_task"
	TeardownTaskTimeout  TimeoutType = "teardown_task"
	TeardownGroupTimeout TimeoutType = "teardown_group"
)

// LogOutput represents the output locations for the agent's logs.
type LogOutputType string

const (
	// LogOutputFile indicates that the agent will log to a file.
	LogOutputFile LogOutputType = "file"
	// LogOutputStdout indicates that the agent will log to standard output.
	LogOutputStdout LogOutputType = "stdout"
)

// Mode represents a mode that the agent will run in.
type Mode string

const (
	// HostMode indicates that the agent will run in a host.
	HostMode Mode = "host"
)

const (
	// AWSAccessKeyId is the expansion name for a temporary AWS access key.
	AWSAccessKeyId = "AWS_ACCESS_KEY_ID"
	// AWSSecretAccessKey is the expansion name for a temporary AWS secret.
	AWSSecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	// AWSSessionToken is the expansion name for an AWS session token.
	AWSSessionToken = "AWS_SESSION_TOKEN"
	// AWSRoleExpiration is the expansion name for the expiration of a temporary AWS access key.
	AWSRoleExpiration = "AWS_ROLE_EXPIRATION"

	// HostSecret is the placeholder name within the agent for the host's
	// secret. The host secret is not an expansion, but is still a sensitive
	// Evergreen-internal value that should be redacted.
	HostSecret = "HOST_SECRET"
)

var (
	// ExpansionsToRedact are expansion names that should be redacted from logs and expansion exports.
	ExpansionsToRedact = []string{
		AWSAccessKeyId,
		AWSSecretAccessKey,
		AWSSessionToken,
	}
)
