package util

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/mongodb/grip"
)

const (
	processCleanupAttempts   = 10
	processCleanupTimeoutMin = 1 * time.Second
	processCleanupTimeoutMax = 10 * time.Minute
)

func TrackProcess(key string, pid int, logger grip.Journaler) {
	// trackProcess is a noop on linux, because we detect all the processes to be killed in
	// cleanup() and we don't need to do any special bookkeeping up-front.
}

// getEnv returns a slice of environment variables for the given pid, in the form
// []string{"VAR1=FOO", "VAR2=BAR", ...}
// This function works by reading from /proc/$PID/environ, so the values returned only reflect
// the values of the environment variables at the time that the process was started.
func getEnv(pid int) ([]string, error) {
	env, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		// This is probably either "permission denied" because we do not own the process,
		// or the process simply doesn't exist anymore.
		return nil, err
	}
	parts := bytes.Split(env, []byte{0})
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		results = append(results, string(part))
	}
	return results, nil
}

func cleanup(key string, logger grip.Journaler) error {
	myPid := os.Getpid()
	pids, err := listProc()
	if err != nil {
		return err
	}

	// Kill processes
	for _, pid := range pids {
		env, err := getEnv(pid)
		if err != nil {
			continue
		}
		if pid != myPid && envHasMarkers(key, env) {
			p := os.Process{}
			p.Pid = pid
			if err := p.Kill(); err != nil {
				logger.Infof("Killing %d failed: %v", pid, err)
			} else {
				logger.Infof("Killed process %d", pid)
			}
		}
	}

	// Retry listing processes until all have successfully exited
	ctx := context.Background()
	err = util.Retry(
		ctx,
		func() (bool, error) {
			pids, err = listProc()
			if err != nil {
				return false, err
			}
			if len(remainingPids == 0) {
				return false, nil
			}
			return true, nil
		},
		processCleanupAttempts,
		processCleanupTimeoutMin,
		processCleanupTimeoutMax,
	)
	if err != nil {
		return err
	}

	// Log each process that was not cleaned up
	for pid := range pids {
		logger.Infof("Failed to clean up process &d", pid)
	}

	return nil
}
