// +build darwin

package jasper

import (
	"bufio"
	"context"
	"os/exec"
	"strings"

	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

func (o *oomTrackerImpl) Clear(ctx context.Context) error {
	sudo, err := isSudo(ctx)
	if err != nil {
		return errors.Wrap(err, "error checking sudo")
	}

	if sudo {
		return errors.Wrap(exec.CommandContext(ctx, "sudo", "log", "erase", "--all").Run(), "error clearing log")
	}

	return errors.Wrap(exec.CommandContext(ctx, "log", "erase", "--all").Run(), "error clearing log")
}

func (o *oomTrackerImpl) Check(ctx context.Context) error {
	wasOOMKilled, pids, err := analyzeLogs(ctx)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.Pids = pids
	return nil
}

func analyzeLogs(ctx context.Context) (bool, []int, error) {
	var cmd *exec.Cmd
	wasOOMKilled := false
	errs := make(chan error)
	sudo, err := isSudo(ctx)
	if err != nil {
		return false, nil, errors.Wrap(err, "error checking sudo")
	}

	if sudo {
		cmd = exec.CommandContext(ctx, "sudo", "log", "show")
	} else {
		cmd = exec.CommandContext(ctx, "log", "show")
	}
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return false, nil, errors.Wrap(err, "error creating StdoutPipe for log command")
	}

	scanner := bufio.NewScanner(cmdReader)
	if err = cmd.Start(); err != nil {
		return false, nil, errors.Wrap(err, "Error starting log command")
	}

	go func() {
		defer recovery.LogStackTraceAndContinue("log analysis")
		select {
		case <-ctx.Done():
			return
		case errs <- cmd.Wait():
			return
		}
	}()

	pids := []int{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "low swap") {
			wasOOMKilled = true
			if pid, hasPid := getPidFromLog(line); hasPid {
				pids = append(pids, pid)
			}
		}
	}

	select {
	case <-ctx.Done():
		return false, nil, errors.New("request cancelled")
	case err = <-errs:
		return wasOOMKilled, pids, errors.Wrap(err, "Error waiting for dmesg command")

	}
}
