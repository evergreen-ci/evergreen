package jasper

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type oomTrackerImpl struct {
	WasOOMKilled bool  `json:"was_oom_killed"`
	Pids         []int `json:"pids"`
}

// OOMTracker provides a tool for detecting if there have been OOM
// events on the system. The Clear operation may affect the state the
// system logs and the data reported will reflect the entire system
// not simply processes managed by Jasper tools.
type OOMTracker interface {
	Check(context.Context) error
	Clear(context.Context) error
	Report() (bool, []int)
}

// NewOOMTracker returns an implementation of the OOMTracker interface
// for the current platform.
func NewOOMTracker() OOMTracker                 { return &oomTrackerImpl{} }
func (o *oomTrackerImpl) Report() (bool, []int) { return o.WasOOMKilled, o.Pids }

func isSudo(ctx context.Context) (bool, error) {
	if err := exec.CommandContext(ctx, "sudo", "date").Run(); err != nil {
		switch err.(type) {
		case *exec.ExitError:
			return false, nil
		default:
			return false, errors.Wrap(err, "error executing sudo date")
		}
	}

	return true, nil
}

func dmesgContainsOOMKill(line string) bool {
	return strings.Contains(line, "Out of memory") ||
		strings.Contains(line, "Killed process") || strings.Contains(line, "OOM killer") ||
		strings.Contains(line, "OOM-killer")
}

func getPidFromDmesg(line string) (int, bool) {
	split := strings.Split(line, "Killed process")
	if len(split) <= 1 {
		return 0, false
	}
	newSplit := strings.Split(strings.TrimSpace(split[1]), " ")
	pid, err := strconv.Atoi(newSplit[0])
	if err != nil {
		return 0, false
	}
	return pid, true
}

func getPidFromLog(line string) (int, bool) {
	split := strings.Split(line, "pid")
	if len(split) <= 1 {
		return 0, false
	}
	newSplit := strings.Split(strings.TrimSpace(split[1]), " ")
	pid, err := strconv.Atoi(newSplit[0])
	if err != nil {
		return 0, false
	}
	return pid, true
}
