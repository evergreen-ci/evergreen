package jasper

import (
	"bufio"
	"context"
	"os/exec"

	"github.com/pkg/errors"
)

type oomTrackerImpl struct {
	Lines []string `json:"lines"`
	PIDs  []int    `json:"pids"`
}

// OOMTracker provides a tool for detecting if there have been OOM
// events on the system. The Clear operation may affect the state the
// system logs and the data reported will reflect the entire system,
// not simply processes managed by Jasper tools.
type OOMTracker interface {
	Check(context.Context) error
	Clear(context.Context) error
	Report() ([]string, []int)
}

// NewOOMTracker returns an implementation of the OOMTracker interface
// for the current platform.
func NewOOMTracker() OOMTracker                     { return &oomTrackerImpl{} }
func (o *oomTrackerImpl) Report() ([]string, []int) { return o.Lines, o.PIDs }

func isSudo(ctx context.Context) (bool, error) {
	if err := exec.CommandContext(ctx, "sudo", "-n", "date").Run(); err != nil {
		switch err.(type) {
		case *exec.ExitError:
			return false, nil
		default:
			return false, errors.Wrap(err, "error executing sudo date")
		}
	}

	return true, nil
}

type logAnalyzer struct {
	cmdArgs        []string
	lineHasOOMKill func(string) bool
	extractPID     func(string) (int, bool)
}

func (a *logAnalyzer) analyzeKernelLog(ctx context.Context) ([]string, []int, error) {
	sudo, err := isSudo(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error checking sudo")
	}

	var cmd *exec.Cmd
	if sudo {
		cmd = exec.CommandContext(ctx, "sudo", a.cmdArgs...)
	} else {
		cmd = exec.CommandContext(ctx, a.cmdArgs[0], a.cmdArgs[1:]...)
	}
	logPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error creating StdoutPipe for log command")
	}
	scanner := bufio.NewScanner(logPipe)
	if err := cmd.Start(); err != nil {
		return nil, nil, errors.Wrap(err, "Error starting log command")
	}

	lines := []string{}
	pids := []int{}
	for scanner.Scan() {
		line := scanner.Text()
		if a.lineHasOOMKill(line) {
			lines = append(lines, line)
			if pid, hasPID := a.extractPID(line); hasPID {
				pids = append(pids, pid)
			}
		}
	}

	errs := make(chan error, 1)
	select {
	case <-ctx.Done():
		return nil, nil, errors.New("request cancelled")
	case errs <- cmd.Wait():
		err = <-errs
		return lines, pids, errors.Wrap(err, "Error waiting for log command")
	}
}
