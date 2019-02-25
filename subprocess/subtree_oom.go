package subprocess

import (
	"context"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/evergreen-ci/gimlet"

	"github.com/pkg/errors"
)

type OOMTracker struct {
	WasOOMKilled bool  `json:"was_oom_killed"`
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

func ClearOOMHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := NewOOMTracker()

		if err := resp.Clear(r.Context()); err != nil {
			gimlet.WriteJSONInternalError(w, err.Error())
			return
		}

		gimlet.WriteJSON(w, resp)
	}
}

func CheckOOMHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := NewOOMTracker()

		if err := resp.Check(r.Context()); err != nil {
			gimlet.WriteJSONInternalError(w, err.Error())
			return
		}

		gimlet.WriteJSON(w, resp)
	}
}
