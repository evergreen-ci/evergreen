package subprocess

import (
	"os/exec"
	"runtime"
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
	oomTracker := OOMTracker{}
	return &oomTracker
}

func (o *OOMTracker) Clear() error {
	return o.clear()
}

func (o *OOMTracker) Check() error {
	return o.check()
}

func (o *OOMTracker) clear() error {
	var err error
	switch os := runtime.GOOS; os {
	case "linux":
		err = o.clearLinux()
		break
	case "darwin":
		err = o.clearDarwin()
		break
	default:
		return errors.New("os does not handle OOM")
	}

	return errors.Wrap(err, "error in clear")
}

func (o *OOMTracker) setIsSudo() error {
	err := exec.Command("sudo", "date").Run()
	if err != nil {
		switch err.(type) {
		case *exec.ExitError:
			o.IsSudo = false
			break
		default:
			return errors.Wrap(err, "error executing sudo date")
		}
	} else {
		o.IsSudo = true
	}
	return nil
}

func (o *OOMTracker) clearLinux() error {
	err := o.setIsSudo()
	if err != nil {
		return errors.Wrap(err, "error clearing dmesg")
	}

	if o.IsSudo {
		err = exec.Command("sudo", "dmesg", "-c").Run()
	} else {
		err = exec.Command("dmesg", "-c").Run()
	}

	if err != nil {
		return errors.Wrap(err, "error clearing dmesg")
	}
	return nil
}

func (o *OOMTracker) clearDarwin() error {
	err := o.setIsSudo()
	if err != nil {
		return errors.Wrap(err, "error clearing logs")
	}

	if o.IsSudo {
		err = exec.Command("sudo", "log", "erase", "--all").Run()
	} else {
		err = exec.Command("log", "erase", "--all").Run()
	}

	if err != nil {
		return errors.Wrap(err, "error clearing log")
	}
	return nil
}

func (o *OOMTracker) check() error {
	var out []byte
	var err error
	switch os := runtime.GOOS; os {
	case "linux":
		out, err = o.checkLinux()
		if err != nil {
			return errors.Wrap(err, "error searching dmesg")
		}

		if len(out) != 0 {
			o.WasOOMKilled = true
			o.Pids = getPidsLinux(out)
			return nil
		}
	case "darwin":
		out, err = o.checkDarwin()
		if err != nil {
			return errors.Wrap(err, "error searching log")
		}
		if len(out) != 0 {
			o.WasOOMKilled = true
			o.Pids = getPidsDarwin(out)
			return nil
		}
	}
	return errors.New("os does not handle OOM")
}

func (o *OOMTracker) checkLinux() ([]byte, error) {
	grep := "grep -iE '(Out of memory|OOM[- ]killer|Killed process)'"
	if o.IsSudo {
		return exec.Command("sudo", "dmesg", "|", grep).CombinedOutput()
	} else {
		return exec.Command("dmesg", "|", grep).CombinedOutput()
	}
}

func (o *OOMTracker) checkDarwin() ([]byte, error) {
	grep := "grep -i swap"
	if o.IsSudo { //make into a parameter
		return exec.Command("sudo", "log show", "|", grep).CombinedOutput()
	} else {
		return exec.Command("log show", "|", grep).CombinedOutput()
	}
}

func getPidsLinux(out []byte) []int {
	pids := []int{}
	lines := strings.Split(string(out), "\n")

	for _, line := range lines {
		split := strings.Split(line, "Killed process")
		if len(split) <= 1 { // sep DNE
			continue
		}
		newSplit := strings.Split(strings.TrimSpace(split[1]), " ")
		pid, err := strconv.Atoi(newSplit[0])
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

func getPidsDarwin(out []byte) []int {
	pids := []int{}
	lines := strings.Split(string(out), "\n")

	for _, line := range lines {
		split := strings.Split(line, "pid")
		if len(split) <= 1 { // sep DNE
			continue
		}
		newSplit := strings.Split(strings.TrimSpace(split[1]), " ")
		pid, err := strconv.Atoi(newSplit[0])
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
