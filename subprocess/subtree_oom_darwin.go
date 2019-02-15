// +build darwin

package subprocess

import (
	"bufio"
	"context"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

func (o *OOMTracker) Clear(ctx context.Context) error {
	var err error
	o.IsSudo, err = isSudo(ctx)
	if err != nil {
		return errors.Wrap(err, "error checking sudo")
	}

	if o.IsSudo {
		return errors.Wrap(exec.Command("sudo", "log", "erase", "--all").Run(), "error clearing log")
	}

	return errors.Wrap(exec.Command("log", "erase", "--all").Run(), "error clearing log")
}

func (o *OOMTracker) Check(ctx context.Context) error {
	wasOOMKilled, pids, err := analyzeLogs(ctx, o.IsSudo)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.Pids = pids
	return nil
}

func analyzeLogs(ctx context.Context, isSudo bool) (bool, []int, error) {
	var cmd *exec.Cmd
	wasOOMKilled := false
	errs := make(chan error)

	if isSudo {
		cmd = exec.CommandContext(ctx, "sudo", "log", "show")
	} else {
		cmd = exec.CommandContext(ctx, "log", "show")
	}
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return false, []int{}, errors.Wrap(err, "error creating StdoutPipe for log command")
	}

	scanner := bufio.NewScanner(cmdReader)
	if err = cmd.Start(); err != nil {
		return false, []int{}, errors.Wrap(err, "Error starting log command")
	}

	go func() {
		errs <- cmd.Wait()
	}()

	pids := []int{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "low swap") {
			wasOOMKilled = true
			pid, hasPid := getPidFromLog(line)
			if hasPid {
				pids = append(pids, pid)
			}
		}
	}

	err = <-errs
	return wasOOMKilled, pids, errors.Wrap(err, "Error waiting for dmesg command")
}
