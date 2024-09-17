//go:build darwin || linux

package util

import (
	"context"
	"sync"
	"syscall"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	cleanupCheckAttempts   = 10
	cleanupCheckTimeoutMin = 100 * time.Millisecond
	cleanupCheckTimeoutMax = time.Second
	contextTimeout         = 10 * time.Minute
)

var (
	errProcessStillRunning = errors.New("process still running")
	registry               processRegistry
)

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
	for _, pid := range pidsToKill {
		// shell.exec and subprocess.exec processes are started as group leaders. This means
		// they and their child processes all share a single process group id (PGID) which is equal to
		// the PID of the group leader. The POSIX spec stipulates that a negative PID signifies
		// that a kill signal should be sent to the entire group.
		// https://pubs.opengroup.org/onlinepubs/9699919799/functions/kill.html
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
			logger.Errorf("Cleanup got error killing process with PGID %d: %s.", pid, err)
		} else {
			logger.Infof("Cleanup killed process group with PGID %d.", pid)
		}
	}

	pidsStillRunning, err := waitForExit(ctx, pidsToKill)
	if err != nil {
		if cause := errors.Cause(err); cause == errProcessStillRunning {
			for _, pid := range pidsStillRunning {
				logger.Errorf("Failed to clean up process group with PGID %d.", pid)
			}
		} else {
			logger.Errorf("Problem waiting for processes to exit: %s.", err)
		}
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
				return true, errors.Wrapf(errProcessStillRunning, "%d of %d processes are still running", len(unkilledPIDs), len(pidsToWait))
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
	processes, err := process.ProcessesWithContext(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			grip.Alert(errors.New("getting processes encountered context deadline"))
		}
		return nil, errors.Wrap(err, "getting processes")
	}
	pids := make([]int, 0, len(processes))
	for _, p := range processes {
		statuses, err := p.StatusWithContext(ctx)
		if err != nil {
			continue
		}
		// Zombie processes are dead processes that haven't been reaped by their parent. They do not consume
		// resources and will go away by themselves when their parent dies.
		// This is expected for processes that run in the background, such as a subprocess.exec with Background true.
		if !utility.StringSliceContains(statuses, process.Zombie) {
			pids = append(pids, int(p.Pid))
		}
	}
	return pids, nil
}
