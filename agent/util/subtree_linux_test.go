package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPIDsToKill(t *testing.T) {
	const timeoutSecs = 10
	ctx := t.Context()

	agentPID, ok := os.LookupEnv(MarkerAgentPID)
	if ok {
		// For CI testing, temporarily simulate this process as running outside
		// of the agent so that getPIDsToKill performs its special checks for
		// agent-external processes.
		os.Unsetenv(MarkerAgentPID)
		defer os.Setenv(MarkerAgentPID, agentPID)
	}

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
		pids, err := getPIDsToKill(ctx, "", filepath.Dir(fullSleepPath), grip.GetDefaultJournaler())
		require.NoError(t, err)

		var (
			foundInEvergreenPID  bool
			foundInWorkingDirPID bool
		)
		for _, pid := range pids {
			if pid == inEvergreenPID {
				foundInEvergreenPID = true
			}
			if pid == inWorkingDirPID {
				foundInWorkingDirPID = true
			}
			if foundInEvergreenPID && foundInWorkingDirPID {
				break
			}
		}
		return foundInEvergreenPID && foundInWorkingDirPID
	}, timeoutSecs*time.Second, 100*time.Millisecond, "in Evergreen process (pid %d) and in working directory process (pid %d) both should have eventually appeared in the listed PID")
}
