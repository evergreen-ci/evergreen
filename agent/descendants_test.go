package agent

import (
	"os/exec"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDescendantPIDs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pgrep is not available on Windows")
	}

	t.Run("NilForEmptyInput", func(t *testing.T) {
		assert.Nil(t, getDescendantPIDs(t.Context(), nil, nil, nil))
		assert.Nil(t, getDescendantPIDs(t.Context(), []int{}, nil, nil))
	})

	t.Run("NilForNonExistentPID", func(t *testing.T) {
		result := getDescendantPIDs(t.Context(), []int{0}, nil, nil)
		assert.Empty(t, result)
	})

	t.Run("FindsChildProcesses", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), "sh", "-c", "sh -c 'sleep 300' & wait")
		require.NoError(t, cmd.Start())

		// Give a moment for the child processes to spawn.
		time.Sleep(200 * time.Millisecond)

		parentPID := cmd.Process.Pid
		descendants := getDescendantPIDs(t.Context(), []int{parentPID}, nil, nil)
		assert.NotEmpty(t, descendants, "expected to find child PIDs of sh process (PID %d)", parentPID)

		for _, pid := range descendants {
			assert.Greater(t, pid, 0, "descendant PID should be positive")
		}
	})

	t.Run("NoDuplicates", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), "sh", "-c", "sleep 300")
		require.NoError(t, cmd.Start())

		descendants := getDescendantPIDs(t.Context(), []int{cmd.Process.Pid}, nil, nil)
		seen := make(map[int]bool)
		for _, pid := range descendants {
			assert.False(t, seen[pid], "duplicate PID %d", pid)
			seen[pid] = true
		}
	})
}

func TestPgrepChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pgrep is not available on Windows")
	}

	t.Run("ReturnsNilForNoChildren", func(t *testing.T) {
		result := pgrepChildren(t.Context(), 1048576, nil)
		assert.Nil(t, result)
	})

	t.Run("ParsesOutput", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), "sh", "-c", "sleep 300 & wait")
		require.NoError(t, cmd.Start())

		time.Sleep(200 * time.Millisecond)

		children := pgrepChildren(t.Context(), cmd.Process.Pid, nil)
		assert.NotEmpty(t, children)
		for _, pid := range children {
			_, err := strconv.Atoi(strconv.Itoa(pid))
			assert.NoError(t, err)
		}
	})
}
