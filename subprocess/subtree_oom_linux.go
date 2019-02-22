// +build linux

package subprocess

import (
	"bufio"
	"context"
	"os/exec"

	"github.com/pkg/errors"
)

func (o *OOMTracker) Clear(ctx context.Context) error {
	var err error
	o.IsSudo, err = isSudo(ctx)
	if err != nil {
		return errors.Wrap(err, "error checking sudo")
	}

	if o.IsSudo {
		return errors.Wrap(exec.CommandContext(ctx, "sudo", "dmesg", "-c").Run(), "error clearing dmesg")
	}

	return errors.Wrap(exec.CommandContext(ctx, "dmesg", "-c").Run(), "error clearing dmesg")
}

func (o *OOMTracker) Check(ctx context.Context) error {
	wasOOMKilled, pids, err := analyzeDmesg(ctx, o.IsSudo)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.Pids = pids
	return nil
}

func analyzeDmesg(ctx context.Context, isSudo bool) (bool, []int, error) {
	var cmd *exec.Cmd
	wasOOMKilled := false
	errs := make(chan error)

	if isSudo {
		cmd = exec.CommandContext(ctx, "sudo", "dmesg")
	} else {
		cmd = exec.CommandContext(ctx, "dmesg")
	}
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return false, nil, errors.Wrap(err, "error creating StdoutPipe for dmesg command")
	}

	scanner := bufio.NewScanner(cmdReader)
	err = cmd.Start()
	if err != nil {
		return false, nil, errors.Wrap(err, "Error starting dmesg command")
	}

	go func() {
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
	case err = <-errs:
		return wasOOMKilled, pids, errors.Wrap(err, "Error waiting for dmesg command")
	}
}
