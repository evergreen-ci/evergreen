package subprocess

// this is a direct copy paste of the darwin file, mostly added to get
// the agent code to compile on freebsd. it is not tested.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mongodb/grip"
)

func TrackProcess(key string, pid int, logger grip.Journaler) {
	// trackProcess is a noop on OSX, because we detect all the processes to be killed in
	// cleanup() and we don't need to do any special bookkeeping up-front.
}

func cleanup(key string, logger grip.Journaler) error {
	out, err := exec.Command("ps", "-E", "-e", "-o", "pid,command").CombinedOutput()
	if err != nil {
		m := "cleanup failed to get output of 'ps'"
		logger.Errorf("%s: %v", m, err)
		return errors.Wrap(err, m)
	}
	myPid := fmt.Sprintf("%v", os.Getpid())

	pidsToKill := []int{}
	lines := strings.Split(string(out), "\n")

	// Look through the output of the "ps" command and find the processes we need to kill.
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		splitLine := strings.Fields(line)
		pid := splitLine[0]
		env := splitLine[2:]

		if pid != myPid && envHasMarkers(key, env) {
			// add it to the list of processes to clean up
			pidAsInt, err := strconv.Atoi(pid)
			if err != nil {
				logger.Errorf("cleanup failed to convert from string to int: %v", err)
				continue
			}
			pidsToKill = append(pidsToKill, pidAsInt)
		}
	}

	// Iterate through the list of processes to kill that we just built, and actually kill them.
	for _, pid := range pidsToKill {
		p := os.Process{}
		p.Pid = pid
		err := p.Kill()
		if err != nil {
			logger.Errorf("Cleanup got error killing pid %d: %v", pid, err)
		} else {
			logger.Infof("Cleanup killed pid %d", pid)
		}
	}
	return nil
}
