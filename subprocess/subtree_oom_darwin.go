// +build darwin

package subprocess

import (
	"bufio"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

func (o *OOMTracker) Clear() error {
	var err error
	o.IsSudo, err = isSudo()
	if err != nil {
		return errors.Wrap(err, "error checking sudo")
	}

	if o.IsSudo {
		return errors.Wrap(exec.Command("sudo", "log", "erase", "--all").Run(), "error clearing log")
	}

	return errors.Wrap(exec.Command("log", "erase", "--all").Run(), "error clearing log")
}

func (o *OOMTracker) Check() error {
	wasOOMKilled, pids, err := analyzeLogs(o.IsSudo)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.Pids = pids
	return nil
}

func analyzeLogs(isSudo bool) (bool, []int, error) {
	var cmd *exec.Cmd
	wasOOMKilled := false
	pidChannel := make(chan *int)

	if isSudo {
		cmd = exec.Command("sudo", "log show")
	} else {
		cmd = exec.Command("log show")
	}
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return false, []int{}, errors.Wrap(err, "error creating StdoutPipe for log command")
	}
	scanner := bufio.NewScanner(cmdReader)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "swap") { // OOM killed!
				wasOOMKilled = true
				pid := getPidFromLog(line)
				if pid != nil {
					pidChannel <- pid
				}
			}
		}
	}()

	err = cmd.Start()
	if err != nil {
		return false, []int{}, errors.Wrap(err, "Error starting log command")
	}

	err = cmd.Wait()
	if err != nil {
		return false, []int{}, errors.Wrap(err, "Error waiting on log command")
	}

	close(pidChannel)
	pids := []int{}
	for pid := range pidChannel {
		pids = append(pids, *pid)
	}

	return wasOOMKilled, pids, nil
}
