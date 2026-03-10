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
		result := GetDescendantPIDs(ctx, []int{0})
		assert.Empty(t, result)
	})

	t.Run("FindsChildProcesses", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "sh", "-c", "sh -c 'sleep 300' & wait")
		require.NoError(t, cmd.Start())
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		// Give a moment for the child processes to spawn.
		assert.NoError(t, exec.Command("sleep", "0.2").Run())

		parentPID := cmd.Process.Pid
		descendants := GetDescendantPIDs(ctx, []int{parentPID})
		assert.NotEmpty(t, descendants, "expected to find child PIDs of sh process (PID %d)", parentPID)

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

		assert.NoError(t, exec.Command("sleep", "0.2").Run())

		children := pgrepChildren(ctx, cmd.Process.Pid)
		assert.NotEmpty(t, children)
		for _, pid := range children {
			_, err := strconv.Atoi(strconv.Itoa(pid))
			assert.NoError(t, err)
		}
	})
}
