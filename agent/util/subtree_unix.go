//go:build darwin || linux

package util

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	cleanupCheckAttempts   = 10
	cleanupCheckTimeoutMin = 100 * time.Millisecond
	cleanupCheckTimeoutMax = time.Second
	contextTimeout         = 10 * time.Minute
)

var registry processRegistry

type processRegistry struct {
	sync.Mutex
	pids []int
}

func (r *processRegistry) trackProcess(pid int) {
	r.Lock()
	defer r.Unlock()

	r.pids = append(r.pids, pid)
}

func (r *processRegistry) popProcessList() []int {
	r.Lock()
	defer r.Unlock()

	pids := r.pids
	r.pids = nil
	return pids
}

// TrackProcess records the pid of a process started by the agent.
func TrackProcess(pid int, _ string, _ grip.Journaler) {
	registry.trackProcess(pid)
}

// KillSpawnedProcs kills processes that have been tracked and their descendants.
// for them to terminate.
func KillSpawnedProcs(ctx context.Context, _ string, logger grip.Journaler) error {
	pidsToKill := registry.popProcessList()
	logger.Infof("killing pids: %v", pidsToKill)
	for _, pid := range pidsToKill {
		// shell.exec and subprocess.exec processes are started as group leaders. This means
		// they and their child processes all share a single process group id (PGID). The POSIX spec stipulates
		// that a negative PID signifies that a kill signal should be sent to the entire group.
		// https://pubs.opengroup.org/onlinepubs/9699919799/functions/kill.html
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
			logger.Errorf("Cleanup got error killing process with PGID %d: %s.", pid, err)
		} else {
			logger.Infof("Cleanup killed process group with PGID %d.", pid)
		}
	}

	pidsStillRunning, err := waitForExit(ctx, pidsToKill)
	if err != nil {
		logger.Infof("Problem waiting for processes to exit: %s.", err)
	}
	for _, pid := range pidsStillRunning {
		logger.Infof("Failed to clean up process with PID %d.", pid)
	}

	return nil

}

// waitForExit is a best-effort attempt to wait for processes to exit.
// Any processes still running when the context is cancelled or when we run
// out of attempts will have their pids included in the returned slice.
func waitForExit(ctx context.Context, pidsToWait []int) ([]int, error) {
	var unkilledPIDs []int
	err := utility.Retry(
		ctx,
		func() (bool, error) {
			unkilledPIDs = []int{}
			runningPIDs, err := psAllProcesses(ctx)
			if err != nil {
				return false, errors.Wrap(err, "listing processes still running")
			}

			for _, pid := range pidsToWait {
				for _, runningPID := range runningPIDs {
					if pid == runningPID {
						unkilledPIDs = append(unkilledPIDs, pid)
					}
				}
			}
			if len(unkilledPIDs) > 0 {
				return true, errors.Errorf("%d of %d processes are still running", len(unkilledPIDs), len(pidsToWait))
			}

			return false, nil
		}, utility.RetryOptions{
			MaxAttempts: cleanupCheckAttempts,
			MinDelay:    cleanupCheckTimeoutMin,
			MaxDelay:    cleanupCheckTimeoutMax,
		})

	return unkilledPIDs, err
}

func psAllProcesses(ctx context.Context) ([]int, error) {
	/*
		Usage of ps:
		-A: list *all* processes, not just ones that we own
		-o: print output according to the given format. We supply 'pid=' to
		print just the pid column without headers.
	*/
	psCtx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()

	args := []string{"-A", "-o", "pid="}
	out, err := exec.CommandContext(psCtx, "ps", args...).CombinedOutput()
	if err != nil {
		// If the context's deadline was exceeded we conclude the process blocked
		// and was killed when the context was closed.
		if psCtx.Err() == context.DeadlineExceeded {
			return nil, ErrPSTimeout
		}
		return nil, errors.Wrap(err, "running ps")
	}
	return parsePs(string(out)), nil
}

func parsePs(psOutput string) []int {
	lines := strings.Split(psOutput, "\n")
	pids := make([]int, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}

		pids = append(pids, pid)
	}

	return pids
}
