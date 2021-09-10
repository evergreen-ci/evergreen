// +build darwin linux

package util

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPIDsToKill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentPID, ok := os.LookupEnv(MarkerAgentPID)
	if ok {
		// For CI testing, temporarily simulate this process as running outside
		// of the agent so that getPIDsToKill performs its special checks for
		// agent-external processes.
		os.Unsetenv(MarkerAgentPID)
		defer os.Setenv(MarkerAgentPID, agentPID)
	}

	inEvergreenCmd := exec.CommandContext(ctx, "sleep", "10")
	inEvergreenCmd.Env = append(inEvergreenCmd.Env, fmt.Sprintf("%s=true", MarkerInEvergreen))
	require.NoError(t, inEvergreenCmd.Start())
	inEvergreenPID := inEvergreenCmd.Process.Pid

	fullSleepPath, err := exec.LookPath("sleep")
	require.NoError(t, err)

	inWorkingDirCmd := exec.CommandContext(ctx, fullSleepPath, "10")
	require.NoError(t, inWorkingDirCmd.Start())
	inWorkingDirPID := inWorkingDirCmd.Process.Pid

	pids, err := getPIDsToKill(ctx, "", filepath.Dir(fullSleepPath), grip.GetDefaultJournaler())
	require.NoError(t, err)
	assert.Contains(t, pids, inEvergreenPID)
	assert.Contains(t, pids, inWorkingDirPID)
	assert.NotContains(t, pids, os.Getpid())
}

func TestKillSpawnedProcs(t *testing.T) {
	ctx := context.Background()
	for testName, test := range map[string]func(*testing.T){
		"expired context": func(t *testing.T) {
			expiredContext, cancel := context.WithTimeout(ctx, -time.Second)
			defer cancel()

			err := KillSpawnedProcs(expiredContext, "", "", grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.Equal(t, ErrPSTimeout, errors.Cause(err))
		},
		"cancelled context": func(t *testing.T) {
			cancelledContext, cancel := context.WithCancel(ctx)
			cancel()

			err := KillSpawnedProcs(cancelledContext, "", "", grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.NotEqual(t, ErrPSTimeout, errors.Cause(err))
		},
		"not cancelled": func(t *testing.T) {
			err := KillSpawnedProcs(ctx, "", "", grip.GetDefaultJournaler())
			assert.NoError(t, err)
		},
	} {
		t.Run(testName, test)
	}

}

func TestWaitForExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, test := range map[string]func(*testing.T){
		"non-existent process": func(t *testing.T) {
			waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			pids, err := waitForExit(waitCtx, []int{1234567890})
			assert.NoError(t, err)
			assert.Empty(t, pids)
		},
		"long-running process": func(t *testing.T) {
			longProcess := exec.CommandContext(ctx, "sleep", "30")
			require.NoError(t, longProcess.Start())
			waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			pids, err := waitForExit(waitCtx, []int{longProcess.Process.Pid})
			assert.Error(t, err)
			require.Len(t, pids, 1)
			assert.Equal(t, longProcess.Process.Pid, pids[0])
		},
	} {
		t.Run(testName, test)
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
