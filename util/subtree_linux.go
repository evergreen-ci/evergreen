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
	"github.com/pkg/errors"
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

// processesToKill returns a list of pids that should be killed.
// Only usable on systems with a /proc filesystem.
func processesToKill(key, workingDir string, logger grip.Journaler) ([]int, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	results := make([]int, 0, 50)
	myPid := os.Getpid()
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

			if pid == myPid {
				continue
			}

			if !processHasMarkers(pid, key, logger) && !executableInWorkingDir(pid, workingDir, logger) {
				continue
			}

			results = append(results, pid)
		}
	}
	return results, nil
}

func cleanup(key, workingDir string, logger grip.Journaler) error {
	pids, err := processesToKill(key, workingDir, logger)
	if err != nil {
		return errors.Wrap(err, "can't get list of processes to kill")
	}

	// Kill processes
	for _, pid := range pids {
		p := os.Process{}
		p.Pid = pid
		if err = p.Kill(); err != nil {
			logger.Infof("Killing %d failed: %s", pid, err.Error())
		} else {
			logger.Infof("Killed process %d", pid)
		}
	}

	var unkilledPids []int
	// Retry listing processes until all have successfully exited
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	err = Retry(
		ctx,
		func() (bool, error) {
			unkilledPids = []int{}
			pids, err = processesToKill(key, workingDir, logger)
			if err != nil {
				return false, errors.Wrap(err, "can't get list of processes to kill")
			}
			for _, pid := range pids {
				unkilledPids = append(unkilledPids, pid)
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

func processHasMarkers(pid int, key string, logger grip.Journaler) bool {
	env, err := getEnv(pid)
	if err != nil {
		if !os.IsPermission(err) {
			logger.Infof("Could not get environment for process %d", pid)
		}
		return false
	}
	return envHasMarkers(key, env)
}

func executableInWorkingDir(pid int, workingDir string, logger grip.Journaler) bool {
	if workingDir == "" {
		return false
	}

	executablePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		if !os.IsPermission(err) {
			logger.Infof("Could not get executable path for process %d", pid)
		}
		return false
	}

	return strings.HasPrefix(executablePath, workingDir)
}
