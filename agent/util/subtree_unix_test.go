//go:build darwin || linux
// +build darwin linux

package util

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKillSpawnedProcs(t *testing.T) {
	for testName, test := range map[string]func(ctx context.Context, t *testing.T){
		"ErrorsWithContextTimeout": func(ctx context.Context, t *testing.T) {
			expiredContext, cancel := context.WithTimeout(ctx, -time.Second)
			defer cancel()

			err := KillSpawnedProcs(expiredContext, "", grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.Equal(t, ErrPSTimeout, errors.Cause(err))
		},
		"ErrorsWithContextCancelled": func(ctx context.Context, t *testing.T) {
			cancelledContext, cancel := context.WithCancel(ctx)
			cancel()

			err := KillSpawnedProcs(cancelledContext, "", grip.GetDefaultJournaler())
			assert.Error(t, err)
			assert.NotEqual(t, ErrPSTimeout, errors.Cause(err))
		},
		"SucceedsWithNoContextError": func(ctx context.Context, t *testing.T) {
			err := KillSpawnedProcs(ctx, "", grip.GetDefaultJournaler())
			assert.NoError(t, err)
		},
		"KillsTrackedProcesses": func(ctx context.Context, t *testing.T) {
			registry.popProcessList()
			defer registry.popProcessList()

			longProcess := exec.CommandContext(ctx, "sleep", "30")
			require.NoError(t, longProcess.Start())
			registry.trackProcess(longProcess.Process.Pid)

			assert.NoError(t, KillSpawnedProcs(ctx, "", grip.GetDefaultJournaler()))
			runningProcesses, err := psAllProcesses(ctx)
			assert.NoError(t, err)
			assert.NotContains(t, runningProcesses, longProcess.Process.Pid)
		},
		"KillsTrackedProcessDescendants": func(ctx context.Context, t *testing.T) {
			registry.popProcessList()
			defer registry.popProcessList()

			longProcess := exec.CommandContext(ctx, "bash", "-c", "nohup sleep 1000 > /dev/null 2>&1 &")
			longProcess.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			require.NoError(t, longProcess.Start())
			registry.trackProcess(longProcess.Process.Pid)

			assert.NoError(t, KillSpawnedProcs(ctx, "", grip.GetDefaultJournaler()))
			psOutput, err := exec.CommandContext(ctx, "ps", "-A").CombinedOutput()
			assert.NoError(t, err)
			assert.NotContains(t, string(psOutput), "sleep 1000")
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
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
	cases := map[string][]int{
		"1":       {1},
		"1\n1267": {1, 1267},
		"":        {},
		"NaN":     {},
	}

	for psOutput, processes := range cases {
		assert.Equal(t, processes, parsePs(psOutput))
	}
}
