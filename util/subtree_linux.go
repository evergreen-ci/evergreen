package util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mongodb/grip"
)

const (
	cleanupCheckAttempts   = 10
	cleanupCheckTimeoutMin = 100 * time.Millisecond
	cleanupCheckTimeoutMax = 1 * time.Second
	contextTimeout         = 10 * time.Second
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

// listProc() returns a list of active pids on the system, by listing the
// contents of /proc and looking for entries that appear to be valid pids. Only
// usable on systems with a /proc filesystem.
func listProc() ([]int, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	results := make([]int, 0, 50)
	for {
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, fi := range fis {
			// Pid must be a directory with a numeric name
			if !fi.IsDir() {
				continue
			}

			// Using Atoi here will also filter out . and ..
			pid, err := strconv.Atoi(fi.Name())
			if err != nil {
				continue
			}
			results = append(results, pid)
		}
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
			if !strings.Contains(err.Error(), os.ErrPermission.Error()) {
				logger.Infof("Could not get environment for process %d", pid)
			}
			continue
		}
		if pid != myPid && envHasMarkers(key, env) {
			p := os.Process{}
			p.Pid = pid
			if err := p.Kill(); err != nil {
				logger.Infof("Killing %d failed: %s", pid, err.Error())
			} else {
				logger.Infof("Killed process %d", pid)
			}
		}
	}

	unkilledPids := []int{}
	// Retry listing processes until all have successfully exited
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	err = Retry(
		ctx,
		func() (bool, error) {
			unkilledPids = []int{}
			pids, err = listProc()
			if err != nil {
				return false, err
			}
			for _, pid := range pids {
				env, err := getEnv(pid)
				if err != nil {
					if !strings.Contains(err.Error(), os.ErrPermission.Error()) {
						logger.Infof("Could not get environment for process %s", pid)
					}
					continue
				}
				if pid != myPid && envHasMarkers(key, env) {
					unkilledPids = append(unkilledPids, pid)
				}
			}
			return len(unkilledPids) != 0, nil
		},
		cleanupCheckAttempts,
		cleanupCheckTimeoutMin,
		cleanupCheckTimeoutMax,
	)
	if err != nil {
		return err
	}

	// Log each process that was not cleaned up
	for _, pid := range unkilledPids {
		logger.Infof("Failed to clean up process %d", pid)
	}

	return nil
}
