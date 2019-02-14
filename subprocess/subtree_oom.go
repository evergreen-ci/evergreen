package subprocess

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type OOMTracker struct {
	WasOOMKilled bool  `json:"was_oom_killed"`
	IsSudo       bool  `json:"is_sudo"`
	Pids         []int `json:"pids"`
}

func NewOOMTracker() *OOMTracker {
	return &OOMTracker{}
}

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
		strings.Contains(line, "Killed process") || strings.Contains(line, "oom")
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
