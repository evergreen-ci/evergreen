package subprocess

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func TrackProcess(key string, pid int, logger grip.Journaler) {
	// trackProcess is a noop on OSX, because we detect all the processes to be killed in
	// cleanup() and we don't need to do any special bookkeeping up-front.
}

func cleanup(_ string, logger grip.Journaler) error {
	/*
		Usage of ps on OSX for extracting environment variables:
		-E: print the environment of the process (VAR1=FOO VAR2=BAR ...)
		-e: list *all* processes, not just ones that we own
		-o: print output according to the given format. We supply 'pid,command' so that
		only those two columns are printed, and then we extract their values using the regexes.

		Each line of output has a format with the pid, command, and environment, e.g.:
		1084 foo.sh PATH=/usr/bin/sbin TMPDIR=/tmp LOGNAME=xxx
	*/

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

		if pid != myPid && envHasMarkers(env) {
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
