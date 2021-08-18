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

	pids, err := getPIDsToKill(context.Background(), "", filepath.Dir(fullSleepPath), grip.GetDefaultJournaler())
	require.NoError(t, err)
	assert.Contains(t, pids, inEvergreenPID)
	assert.Contains(t, pids, inWorkingDirPID)
	assert.NotContains(t, pids, os.Getpid())
}

func TestKillSpawnedProcs(t *testing.T) {
	expiredContext, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()

	err := KillSpawnedProcs(expiredContext, "", "", grip.GetDefaultJournaler())
	assert.Error(t, err)
	assert.Equal(t, PsTimeoutError, errors.Cause(err))
}

func TestWaitForExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, test := range map[string]func(*testing.T){
		"non-existent process": func(t *testing.T) {
			waitCtx, cancel := context.WithTimeout(ctx, time.Second*10)
			defer cancel()
			require.Empty(t, waitForExit(waitCtx, []int{1234567890}))
		},
		"long-running process": func(t *testing.T) {
			longProcess := exec.CommandContext(ctx, "sleep", "10")
			require.NoError(t, longProcess.Start())
			waitCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			pids := waitForExit(waitCtx, []int{longProcess.Process.Pid})
			require.Len(t, pids, 1)
			assert.Equal(t, longProcess.Process.Pid, pids[0])
		},
	} {
		t.Run(testName, test)
	}
}

func TestParsePs(t *testing.T) {
	psOutput := `
    1 /sbin/init
   1267 /lib/systemd/systemd --user LANG=C.UTF-8
`
	processes := parsePs(psOutput)
	require.Len(t, processes, 2)

	assert.Equal(t, 1, processes[0].pid)
	assert.Equal(t, "/sbin/init", processes[0].command)

	assert.Equal(t, 1267, processes[1].pid)
	assert.Equal(t, "/lib/systemd/systemd", processes[1].command)
	require.Len(t, processes[1].env, 2)
	assert.Equal(t, processes[1].env[0], "--user")
	assert.Equal(t, processes[1].env[1], "LANG=C.UTF-8")
}
