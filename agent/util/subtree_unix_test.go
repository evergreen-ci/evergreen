//go:build darwin || linux
// +build darwin linux

package util

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPIDsToKill(t *testing.T) {
	const timeoutSecs = 10
	ctx, cancel := context.WithTimeout(context.Background(), timeoutSecs*time.Second)
	defer cancel()

	agentPID, ok := os.LookupEnv(MarkerAgentPID)
	if ok {
		// For CI testing, temporarily simulate this process as running outside
		// of the agent so that getPIDsToKill performs its special checks for
		// agent-external processes.
		os.Unsetenv(MarkerAgentPID)
		defer os.Setenv(MarkerAgentPID, agentPID)
	}

	// This command should not be in the list of PID's to clean up.
	longRunningCommnad := exec.CommandContext(ctx, "sleep", strconv.Itoa(timeoutSecs))
	require.NoError(t, longRunningCommnad.Start())
	longRunningCommandPID := longRunningCommnad.Process.Pid

	// This is simulating some other process that is not part of the task and is long running.
	time.Sleep(2 * time.Second)
	lastKillTime := time.Now()

	inEvergreenCmd := exec.CommandContext(ctx, "sleep", strconv.Itoa(timeoutSecs))
	inEvergreenCmd.Env = append(inEvergreenCmd.Env, fmt.Sprintf("%s=true", MarkerInEvergreen))
	require.NoError(t, inEvergreenCmd.Start())
	inEvergreenPID := inEvergreenCmd.Process.Pid

	fullSleepPath, err := exec.LookPath("sleep")
	require.NoError(t, err)

	inWorkingDirCmd := exec.CommandContext(ctx, fullSleepPath, strconv.Itoa(timeoutSecs))
	require.NoError(t, inWorkingDirCmd.Start())
	inWorkingDirPID := inWorkingDirCmd.Process.Pid

	assert.Eventually(t, func() bool {
		// Since the processes run in the background, we have to poll them until
		// they actually start, at which point they should appear in the listed
		// PIDs.
		pids, err := getPIDsToKill(ctx, "", filepath.Dir(fullSleepPath), lastKillTime)
		require.NoError(t, err)

		var (
			foundInEvergreenPID  bool
			foundInWorkingDirPID bool
			foundLongRunningPID  bool // This command should not be in the list of PID's to clean up.
		)
		for _, pid := range pids {
			if pid == inEvergreenPID {
				foundInEvergreenPID = true
			}
			if pid == inWorkingDirPID {
				foundInWorkingDirPID = true
			}
			if pid == longRunningCommandPID {
				foundLongRunningPID = true
			}
			if foundInEvergreenPID && foundInWorkingDirPID && foundLongRunningPID {
				break
			}
		}

		// If this is a macos build, proccesses only linked by environment
		// variables will not be found and should be excluded.
		if strings.HasPrefix(runtime.GOOS, "darwin") {
			foundInEvergreenPID = true
		}

		return foundInEvergreenPID && foundInWorkingDirPID && !foundLongRunningPID
	}, timeoutSecs*time.Second, 100*time.Millisecond, "in Evergreen process (pid %d) and in working directory process (pid %d) both should have eventually appeared in the listed PID")
}

func TestKillSpawnedProcs(t *testing.T) {
	for testName, test := range map[string]func(ctx context.Context, t *testing.T){
		"ErrorsWithContextTimeout": func(ctx context.Context, t *testing.T) {
			expiredContext, cancel := context.WithTimeout(ctx, -time.Second)
			defer cancel()

			err := KillSpawnedProcs(expiredContext, KillSpawnedProcsOptions{}, grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.Equal(t, ErrPSTimeout, errors.Cause(err))
		},
		"ErrorsWithContextCancelled": func(ctx context.Context, t *testing.T) {
			cancelledContext, cancel := context.WithCancel(ctx)
			cancel()

			err := KillSpawnedProcs(cancelledContext, KillSpawnedProcsOptions{}, grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.NotEqual(t, ErrPSTimeout, errors.Cause(err))
		},
		"SucceedsWithNoContextError": func(ctx context.Context, t *testing.T) {
			err := KillSpawnedProcs(ctx, KillSpawnedProcsOptions{}, grip.GetDefaultJournaler())
			assert.NoError(t, err)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			test(ctx, t)
		})
	}

}

func TestWaitForExit(t *testing.T) {
	for testName, test := range map[string]func(ctx context.Context, t *testing.T){
		"DoesNotReturnNonexistentProcess": func(ctx context.Context, t *testing.T) {
			pids, err := waitForExit(ctx, []int{1234567890})
			require.NoError(t, ctx.Err())
			assert.NoError(t, err)
			assert.Empty(t, pids)
		},
		"ReturnsLongRunningProcess": func(ctx context.Context, t *testing.T) {
			longProcess := exec.CommandContext(ctx, "sleep", "30")
			require.NoError(t, longProcess.Start())
			pids, err := waitForExit(ctx, []int{longProcess.Process.Pid})
			require.NoError(t, ctx.Err())
			assert.Error(t, err)
			require.Len(t, pids, 1)
			assert.Equal(t, longProcess.Process.Pid, pids[0])
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			test(ctx, t)
		})
	}
}

func TestParsePs(t *testing.T) {
	cases := make(map[string][]process)
	cases[`
    1 /sbin/init
   1267 /lib/systemd/systemd --user LANG=C.UTF-8
`] = []process{
		{pid: 1, command: "/sbin/init"},
		{pid: 1267, command: "/lib/systemd/systemd", env: []string{"--user", "LANG=C.UTF-8"}},
	}

	cases[""] = []process{}

	cases[`
    NaN /sbin/init
`] = []process{}

	cases["1 /sbin/init"] = []process{{pid: 1, command: "/sbin/init"}}

	cases["1"] = []process{}

	for psOutput, processes := range cases {
		assert.Equal(t, processes, parsePs(psOutput))
	}
}
