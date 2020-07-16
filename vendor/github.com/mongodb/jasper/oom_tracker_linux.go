// +build linux

package jasper

import (
	"bufio"
	"context"
	"os/exec"

	"github.com/pkg/errors"
)

func (o *oomTrackerImpl) Clear(ctx context.Context) error {
	sudo, err := isSudo(ctx)
	if err != nil {
		return errors.Wrap(err, "error checking sudo")
	}

	if sudo {
		return errors.Wrap(exec.CommandContext(ctx, "sudo", "dmesg", "-c").Run(), "error clearing dmesg")
	}

	return errors.Wrap(exec.CommandContext(ctx, "dmesg", "-c").Run(), "error clearing dmesg")
}

func (o *oomTrackerImpl) Check(ctx context.Context) error {
	wasOOMKilled, pids, err := analyzeDmesg(ctx)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.Pids = pids
	return nil
}

func analyzeDmesg(ctx context.Context) (bool, []int, error) {
	var cmd *exec.Cmd
	wasOOMKilled := false
	errs := make(chan error)

	sudo, err := isSudo(ctx)
	if err != nil {
		return false, nil, errors.Wrap(err, "error checking sudo")
	}

	if sudo {
		cmd = exec.CommandContext(ctx, "sudo", "dmesg")
	} else {
		cmd = exec.CommandContext(ctx, "dmesg")
	}
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return false, nil, errors.Wrap(err, "error creating StdoutPipe for dmesg command")
	}

	scanner := bufio.NewScanner(cmdReader)
	if err = cmd.Start(); err != nil {
		return false, nil, errors.Wrap(err, "Error starting dmesg command")
	}
	pids := []int{}
	for scanner.Scan() {
		line := scanner.Text()
		if dmesgContainsOOMKill(line) {
			wasOOMKilled = true
			if pid, hasPid := getPidFromDmesg(line); hasPid {
				pids = append(pids, pid)
			}
		}
	}

	select {
	case <-ctx.Done():
		return false, nil, errors.New("request cancelled")
	case errs <- cmd.Wait():
		err = <-errs
		return wasOOMKilled, pids, errors.Wrap(err, "Error waiting for dmesg command")
	}
}
