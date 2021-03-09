// +build darwin linux

package util

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mongodb/grip"
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

	pids, err := getPIDsToKill("", filepath.Dir(fullSleepPath), grip.GetDefaultJournaler())
	require.NoError(t, err)
	assert.Contains(t, pids, inEvergreenPID)
	assert.Contains(t, pids, inWorkingDirPID)
	assert.NotContains(t, pids, os.Getpid())
}
