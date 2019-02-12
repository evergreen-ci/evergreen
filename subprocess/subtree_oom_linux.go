// +build linux

package subprocess

import (
	"bufio"
	"os/exec"

	"github.com/pkg/errors"
)

func (o *OOMTracker) Clear() error {
	var err error
	o.IsSudo, err = isSudo()
	if err != nil {
		return errors.Wrap(err, "error checking sudo")
	}

	if o.IsSudo {
		return errors.Wrap(exec.Command("sudo", "dmesg", "-c").Run(), "error clearing dmesg")
	}

	return errors.Wrap(exec.Command("dmesg", "-c").Run(), "error clearing dmesg")
}

func (o *OOMTracker) Check() error {
	wasOOMKilled, pids, err := analyzeDmesg(o.IsSudo)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.Pids = pids
	return nil
}

func analyzeDmesg(isSudo bool) (bool, []int, error) {
	var cmd *exec.Cmd
	wasOOMKilled := false
	pidChannel := make(chan *int)

	if isSudo {
		cmd = exec.Command("sudo", "dmesg")
	} else {
		cmd = exec.Command("dmesg")
	}
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return false, []int{}, errors.Wrap(err, "error creating StdoutPipe for dmesg command")
	}
	scanner := bufio.NewScanner(cmdReader)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if dmesgContainsOOMKill(line) {
				wasOOMKilled = true
				pid := getPidFromDmesg(line)
				if pid != nil {
					pidChannel <- pid
				}
			}
		}
	}()

	err = cmd.Start()
	if err != nil {
		return false, []int{}, errors.Wrap(err, "Error starting dmesg command")
	}

	err = cmd.Wait()
	if err != nil {
		return false, []int{}, errors.Wrap(err, "Error waiting on dmesg command")
	}

	close(pidChannel)
	pids := []int{}
	for pid := range pidChannel {
		pids = append(pids, *pid)
	}

	return wasOOMKilled, pids, nil
}
