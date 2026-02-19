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

// SetNice sets the nice for the process given by PID. This determines its
// relative scheduling priority for host CPU.
// This is only available if the current process has sufficient permissions to
// set the nice.
func SetNice(pid, nice int) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if nice < minNice || nice > maxNice {
		return errors.Errorf("nice must be between %d and %d", minNice, maxNice)
	}
	// 0 refers to the current process.
	return syscall.Setpriority(syscall.PRIO_PROCESS, pid, nice)
}
