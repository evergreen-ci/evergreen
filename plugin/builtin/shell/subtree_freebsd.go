package shell

// this is a direct copy paste of the darwin file, mostly added to get
// the agent code to compile on freebsd. it is not tested.

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mongodb/grip/slogger"
)

func trackProcess(key string, pid int, log plugin.Logger) {
	// trackProcess is a noop on OSX, because we detect all the processes to be killed in
	// cleanup() and we don't need to do any special bookkeeping up-front.
}

func cleanup(key string, log plugin.Logger) error {
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
		log.LogSystem(slogger.ERROR, "cleanup failed to get output of 'ps': %v", err)
		return err
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
				log.LogSystem(slogger.ERROR, "Cleaup failed to convert from string to int: %v", err)
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
			log.LogSystem(slogger.ERROR, "Cleanup got error killing pid %v: %v", pid, err)
		} else {
			log.LogSystem(slogger.INFO, "Cleanup killed pid %v", pid)
		}
	}
	return nil

}
