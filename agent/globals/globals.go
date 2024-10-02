package globals

import (
	"time"

	"github.com/evergreen-ci/evergreen"
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
	TaskSyncTimeout      TimeoutType = "task_sync"
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
	// PodMode indicates that the agent will run in a pod's container.
	PodMode Mode = "pod"
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
	// HostSecret is the environment variable within the agent that is unique to its running host.
	HostSecret = "HOST_SECRET"
)

var (
	// ExpansionsToRedact are expansion names that should be redacted from logs and expansion exports.
	ExpansionsToRedact = []string{
		evergreen.GithubAppToken,
		// HostServicePasswordExpansion exists to redact the host's ServicePassword in the logs,
		// which is used for some jasper commands for Windows hosts. It is populated as a default
		// expansion only for tasks running on Windows hosts.
		evergreen.HostServicePasswordExpansion,
		AWSAccessKeyId,
		AWSSecretAccessKey,
		AWSSessionToken,
		HostSecret,
	}
)
