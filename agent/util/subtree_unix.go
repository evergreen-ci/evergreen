//go:build darwin || linux

package util

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

const (
	cleanupCheckAttempts   = 10
	cleanupCheckTimeoutMin = 100 * time.Millisecond
	cleanupCheckTimeoutMax = time.Second
	contextTimeout         = 10 * time.Minute

	pkillNoMatchingProcessesExitCode = 1
)

// TrackProcess is a noop by default if we don't need to do any special
// bookkeeping up-front.
func TrackProcess(key string, pid int, logger grip.Journaler) {}

// KillSpawnedProcs kills processes that descend from the agent and waits
// for them to terminate.
func KillSpawnedProcs(ctx context.Context, key, workingDir, execUser string, logger grip.Journaler) error {
	// When execUser is set all subprocess.exec and shell.exec processes are run under that user. This facilitates
	// easy cleanup of processes started by a task. It is enabled on a distro-by-distro basis.
	if execUser != "" {
		return killUserProcesses(ctx, execUser)
	}

	pidsToKill, err := getPIDsToKill(ctx, key, workingDir, logger)
	if err != nil {
		return errors.Wrap(err, "getting list of PIDs to kill")
	}

	for _, pid := range pidsToKill {
		p := os.Process{Pid: pid}
		err := p.Kill()
		if err != nil {
			logger.Errorf("Cleanup got error killing process with PID %d: %s.", pid, err)
		} else {
			logger.Infof("Cleanup killed process with PID %d.", pid)
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

func killUserProcesses(ctx context.Context, execUser string) error {
	if execUser == "" {
		return errors.New("execUser cannot be empty")
	}
	cmd := jasper.NewCommand().Add([]string{"pkill", "-SIGKILL", "-U", execUser}).Sudo(true)
	if err := cmd.Run(ctx); err != nil {
		exitCode, _ := cmd.Wait(ctx)
		if exitCode != pkillNoMatchingProcessesExitCode {
			return errors.Wrapf(err, "killing processes for user '%s'", execUser)
		}
	}
	return nil
}

func getPIDsToKill(ctx context.Context, key, workingDir string, logger grip.Journaler) ([]int, error) {
	var pidsToKill []int

	processes, err := psAllProcesses(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting all processes")
	}
	myPid := os.Getpid()
	for _, process := range processes {
		if process.pid == myPid {
			continue
		}

		if !envHasMarkers(key, process.env) && !commandInWorkingDir(process.command, workingDir) {
			continue
		}

		pidsToKill = append(pidsToKill, process.pid)
	}

	return pidsToKill, nil
}

func envHasMarkers(key string, env []string) bool {
	// If this agent was started by an integration test, only kill a proc if it was started by this agent
	if os.Getenv(MarkerAgentPID) != "" {
		for _, envVar := range env {
			if strings.HasPrefix(envVar, MarkerTaskID) {
				split := strings.Split(envVar, "=")
				if len(split) != 2 {
					continue
				}
				if split[1] == key {
					return true
				}
			}
		}
		return false
	}

	// Otherwise, kill any proc started by any agent
	for _, envVar := range env {
		if strings.HasPrefix(envVar, MarkerTaskID) || strings.HasPrefix(envVar, MarkerAgentPID) || strings.HasPrefix(envVar, MarkerInEvergreen) {
			return true
		}
	}
	return false
}

func commandInWorkingDir(command, workingDir string) bool {
	if workingDir == "" {
		return false
	}

	return strings.HasPrefix(command, workingDir)
}

// waitForExit is a best-effort attempt to wait for processes to exit.
// Any processes still running when the context is cancelled or when we run
// out of attempts will have their pids included in the returned slice.
func waitForExit(ctx context.Context, pidsToWait []int) ([]int, error) {
	var unkilledPids []int
	err := utility.Retry(
		ctx,
		func() (bool, error) {
			unkilledPids = []int{}
			processes, err := psAllProcesses(ctx)
			if err != nil {
				return false, errors.Wrap(err, "listing processes still running")
			}

			for _, process := range processes {
				for _, pid := range pidsToWait {
					if process.pid == pid {
						unkilledPids = append(unkilledPids, process.pid)
					}
				}
			}
			if len(unkilledPids) > 0 {
				return true, errors.Errorf("%d of %d processes are still running", len(unkilledPids), len(pidsToWait))
			}

			return false, nil
		}, utility.RetryOptions{
			MaxAttempts: cleanupCheckAttempts,
			MinDelay:    cleanupCheckTimeoutMin,
			MaxDelay:    cleanupCheckTimeoutMax,
		})

	return unkilledPids, err
}

type process struct {
	pid     int
	command string
	env     []string
}

func psAllProcesses(ctx context.Context) ([]process, error) {
	/*
		Usage of ps for extracting environment variables:
		e: print the environment of the process (VAR1=FOO VAR2=BAR ...)
		-A: list *all* processes, not just ones that we own
		-o: print output according to the given format. We supply 'pid=,command=' to
		print the pid and command columns without headers

		Each line of output has a format with the pid, command, and environment, e.g.:
		1084 foo.sh PATH=/usr/bin/sbin TMPDIR=/tmp LOGNAME=xxx
	*/
	psCtx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()

	args := []string{"e", "-A", "-o", "pid=,command="}
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

func parsePs(psOutput string) []process {
	lines := strings.Split(psOutput, "\n")
	processes := make([]process, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		splitLine := strings.Fields(line)
		if len(splitLine) < 2 {
			continue
		}

		pidString := splitLine[0]
		pid, err := strconv.Atoi(pidString)
		if err != nil {
			continue
		}

		command := splitLine[1]

		// arguments to the command will be included in the process.env, but it's good enough for our purposes.
		var env []string
		if len(splitLine) > 2 {
			env = splitLine[2:]
		}

		processes = append(processes, process{
			pid:     pid,
			command: command,
			env:     env,
		})
	}

	return processes
}

// SetNice sets the nice for the process given by PID. This determines its
// relative scheduling priority for host CPU.
// This is only available if the current process has sufficient permissions to
// set the nice.
func SetNice(pid, nice int) error {
	if runtime.GOOS != "linux" {
		// No-op for MacOS.
		return nil
	}
	if nice < minNice || nice > maxNice {
		return errors.Errorf("nice must be between %d and %d", minNice, maxNice)
	}
	// 0 refers to the current process.
	return syscall.Setpriority(syscall.PRIO_PROCESS, pid, nice)
}
