package util

import "github.com/pkg/errors"

const (
	MarkerTaskID      = "EVR_TASK_ID"
	MarkerAgentPID    = "EVR_AGENT_PID"
	MarkerInEvergreen = "IN_EVERGREEN"
)

var PsTimeoutError = errors.New("ps timeout")
