package util

import (
	"github.com/pkg/errors"
)

const (
	MarkerTaskID      = "EVR_TASK_ID"
	MarkerAgentPID    = "EVR_AGENT_PID"
	MarkerInEvergreen = "IN_EVERGREEN"
)

var (
	ErrPSTimeout = errors.New("ps timeout")
	// ErrContainerExecUnavailable is returned by KillSpawnedProcsInContainer
	// when docker exec fails at the daemon level (exit 125): the container is
	// not running, paused, or unreachable. Callers should warn rather than
	// treat this as a hard kill failure.
	ErrContainerExecUnavailable = errors.New("container exec unavailable")
)

const (
	// minNice is the minimum nice value (i.e. highest priority).
	minNice = -20
	// AgentNice is a nice value that the agent runs at by default. This makes
	// the process more important than those running at the default but is not
	// as critical as other basic system operations.
	AgentNice = -10
	// DefaultNice is the default nice value.
	DefaultNice = 0
	// maxNice is the maximum nice value (i.e. lowest priority).
	maxNice = 19
)
