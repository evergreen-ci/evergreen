package util

import (
	"runtime"
	"syscall"

	"github.com/pkg/errors"
)

const (
	MarkerTaskID      = "EVR_TASK_ID"
	MarkerAgentPID    = "EVR_AGENT_PID"
	MarkerInEvergreen = "IN_EVERGREEN"
)

var ErrPSTimeout = errors.New("ps timeout")

const (
	// MinNice is the minimum nice value (i.e. highest priority).
	MinNice = -20
	// DefaultNice is the default nice value.
	DefaultNice = 0
	// MaxNice is the maximum nice value (i.e. lowest priority).
	MaxNice = 19
)

// SetNice sets the nice on the current process. This determines its relative
// scheduling priority for host resources.
// This is only available if the current process has sufficient permissions to
// set the nice.
func SetNice(nice int) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if nice < MinNice || nice > MaxNice {
		return errors.Errorf("nice must be between %d and %d", MinNice, MaxNice)
	}
	// 0 refers to the current process.
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, nice)
}
