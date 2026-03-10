package taskexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogFile(t *testing.T) {
	t.Run("WriteAndRead", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")

		lf, err := newLogFile(path)
		require.NoError(t, err)
		defer lf.Close()

		lf.WriteLogLine(3, "hello world")
		lf.WriteLogLine(3, "second line")

		require.NoError(t, lf.Close())

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "[step:3] hello world")
		assert.Contains(t, content, "[step:3] second line")
		lines := strings.Split(strings.TrimSpace(content), "\n")
		assert.Len(t, lines, 2)
	})

	t.Run("StepDelimiters", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")

		lf, err := newLogFile(path)
		require.NoError(t, err)
		defer lf.Close()

		lf.WriteStepStart(5, "shell.exec", "main")
		lf.WriteLogLine(5, "command output")
		lf.WriteStepEnd(5, true, "1.2s")

		require.NoError(t, lf.Close())

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "=== STEP 5 START shell.exec (main) ===")
		assert.Contains(t, content, "=== STEP 5 END success=true duration=1.2s ===")
	})
}

func TestLogFilterOptions(t *testing.T) {
	t.Run("FilterByStep", func(t *testing.T) {
		tmpDir := t.TempDir()
		logDir := filepath.Join(tmpDir, "session")
		require.NoError(t, os.MkdirAll(logDir, 0755))

		// Write a test log file with multiple steps
		content := `=== STEP 0 START shell.exec (pre) ===
[2024-01-15T10:30:45.123Z] [step:0] pre command
=== STEP 0 END success=true duration=0.1s ===
=== STEP 1 START shell.exec (main) ===
[2024-01-15T10:30:46.123Z] [step:1] main command
[2024-01-15T10:30:46.456Z] [step:1] main output
=== STEP 1 END success=true duration=1.0s ===
=== STEP 2 START shell.exec (post) ===
[2024-01-15T10:30:47.123Z] [step:2] post command
=== STEP 2 END success=true duration=0.5s ===
`
		require.NoError(t, os.WriteFile(filepath.Join(logDir, "task.log"), []byte(content), 0644))

		lines, err := readLogFileLines(filepath.Join(logDir, "task.log"), LogFilterOptions{
			Step:    1,
			HasStep: true,
		})
		require.NoError(t, err)

		assert.Len(t, lines, 4)
		for _, line := range lines {
			assert.True(t, strings.Contains(line, "STEP 1") || strings.Contains(line, "step:1"),
				"unexpected line: %s", line)
		}
	})

	t.Run("NoFilter", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")
		content := "line1\nline2\nline3\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))

		lines, err := readLogFileLines(path, LogFilterOptions{})
		require.NoError(t, err)
		assert.Len(t, lines, 3)
	})
}
