//go:build darwin || linux

package util

import (
	"context"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDescendantPIDs(t *testing.T) {
	ctx := context.Background()

	t.Run("NilForEmptyInput", func(t *testing.T) {
		assert.Nil(t, GetDescendantPIDs(ctx, nil))
		assert.Nil(t, GetDescendantPIDs(ctx, []int{}))
	})

	t.Run("NilForNonExistentPID", func(t *testing.T) {
		// PID 0 should have no children findable by pgrep.
		result := GetDescendantPIDs(ctx, []int{0})
		assert.Empty(t, result)
	})

	t.Run("FindsChildProcesses", func(t *testing.T) {
		// Start a shell that spawns a subshell with sleep, ensuring a
		// parent-child chain: sh -> sh -> sleep.
		cmd := exec.CommandContext(ctx, "sh", "-c", "sh -c 'sleep 300' & wait")
		require.NoError(t, cmd.Start())
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		// Give a moment for the child processes to spawn.
		// This is inherently racy but sufficient for a unit test.
		exec.Command("sleep", "0.2").Run()

		parentPID := cmd.Process.Pid
		descendants := GetDescendantPIDs(ctx, []int{parentPID})
		// The shell should have at least one descendant.
		assert.NotEmpty(t, descendants, "expected to find child PIDs of sh process (PID %d)", parentPID)

		// Verify the descendants are valid PIDs (positive integers).
		for _, pid := range descendants {
			assert.Greater(t, pid, 0, "descendant PID should be positive")
		}
	})

	t.Run("NoDuplicates", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 300")
		require.NoError(t, cmd.Start())
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		descendants := GetDescendantPIDs(ctx, []int{cmd.Process.Pid})
		seen := make(map[int]bool)
		for _, pid := range descendants {
			assert.False(t, seen[pid], "duplicate PID %d", pid)
			seen[pid] = true
		}
	})
}

func TestPgrepChildren(t *testing.T) {
	ctx := context.Background()

	t.Run("ReturnsNilForNoChildren", func(t *testing.T) {
		// Use a PID that is unlikely to have children.
		result := pgrepChildren(ctx, 1<<20)
		assert.Nil(t, result)
	})

	t.Run("ParsesOutput", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 300 & wait")
		require.NoError(t, cmd.Start())
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		exec.Command("sleep", "0.2").Run()

		children := pgrepChildren(ctx, cmd.Process.Pid)
		assert.NotEmpty(t, children)
		for _, pid := range children {
			_, err := strconv.Atoi(strconv.Itoa(pid))
			assert.NoError(t, err)
		}
	})
}
