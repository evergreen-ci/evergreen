//go:build darwin || linux
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

// fakeDocker installs a stub `docker` binary in a temp dir that exits with
// the given exit code, and prepends that dir to PATH so jasper resolves it.
func fakeDocker(t *testing.T, exitCode int) {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nexit %d\n", exitCode)
	path := filepath.Join(dir, "docker")
	require.NoError(t, os.WriteFile(path, []byte(script), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestKillSpawnedProcs(t *testing.T) {
	for testName, test := range map[string]func(ctx context.Context, t *testing.T){
		"ErrorsWithContextTimeout": func(ctx context.Context, t *testing.T) {
			expiredContext, cancel := context.WithTimeout(ctx, -time.Second)
			defer cancel()

			err := KillSpawnedProcs(expiredContext, "", "", "", grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.Equal(t, ErrPSTimeout, errors.Cause(err))
		},
		"ErrorsWithContextCancelled": func(ctx context.Context, t *testing.T) {
			cancelledContext, cancel := context.WithCancel(ctx)
			cancel()

			err := KillSpawnedProcs(cancelledContext, "", "", "", grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.NotEqual(t, ErrPSTimeout, errors.Cause(err))
		},
		"SucceedsWithNoContextError": func(ctx context.Context, t *testing.T) {
			err := KillSpawnedProcs(ctx, "", "", "", grip.GetDefaultJournaler())
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

func TestKillSpawnedProcsInContainer(t *testing.T) {
	ctx := t.Context()

	t.Run("EmptyContainerIDReturnsError", func(t *testing.T) {
		err := KillSpawnedProcsInContainer(ctx, "", "someuser")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "containerID cannot be empty")
	})

	t.Run("EmptyExecUserReturnsError", func(t *testing.T) {
		err := KillSpawnedProcsInContainer(ctx, "somecontainer", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execUser cannot be empty")
	})

	t.Run("ExitOneIsNilNoProcsBenign", func(t *testing.T) {
		// exit 1 = pkill found no matching processes — not an error.
		fakeDocker(t, 1)
		err := KillSpawnedProcsInContainer(ctx, "somecontainer", "someuser")
		assert.NoError(t, err)
	})

	t.Run("ExitOneTwentyFiveReturnsErrContainerExecUnavailable", func(t *testing.T) {
		// exit 125 = docker exec daemon-level failure (container not running, etc.)
		fakeDocker(t, 125)
		err := KillSpawnedProcsInContainer(ctx, "somecontainer", "someuser")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrContainerExecUnavailable)
	})

	t.Run("NonZeroNonBenignExitReturnsWrappedError", func(t *testing.T) {
		// exit 2 = generic pkill error — should surface as a real failure.
		fakeDocker(t, 2)
		err := KillSpawnedProcsInContainer(ctx, "somecontainer", "someuser")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrContainerExecUnavailable)
	})

	t.Run("ZeroExitIsNilProcessesKilled", func(t *testing.T) {
		// exit 0 = pkill found and killed at least one process — success.
		fakeDocker(t, 0)
		err := KillSpawnedProcsInContainer(ctx, "somecontainer", "someuser")
		assert.NoError(t, err)
	})
}
