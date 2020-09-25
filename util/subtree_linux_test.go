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

func TestGetPidsToKill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inEvergreenCmd := exec.CommandContext(ctx, "sleep", "10")
	inEvergreenCmd.Env = append(inEvergreenCmd.Env, fmt.Sprintf("%s=true", MarkerInEvergreen))
	require.NoError(t, inEvergreenCmd.Start())
	inEvergreenPid := inEvergreenCmd.Process.Pid

	fullSleepPath, err := exec.LookPath("sleep")
	require.NoError(t, err)

	inWorkingDirCmd := exec.CommandContext(ctx, fullSleepPath, "10")
	require.NoError(t, inWorkingDirCmd.Start())
	inWorkingDirPid := inWorkingDirCmd.Process.Pid

	pids, err := processesToKill("", filepath.Dir(fullSleepPath), grip.GetDefaultJournaler())
	assert.NoError(t, err)
	assert.Contains(t, pids, inEvergreenPid)
	assert.Contains(t, pids, inWorkingDirPid)
	assert.NotContains(t, pids, os.Getpid())
}
