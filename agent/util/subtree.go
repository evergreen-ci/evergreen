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
	// ErrContainerExecUnavailable indicates the container could not be reached
	// to clean up its processes. Callers should warn rather than treat this as a
	// hard kill failure, since failing here would mask the task's own failure.
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
