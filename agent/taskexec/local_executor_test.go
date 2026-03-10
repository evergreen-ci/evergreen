package taskexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProject(t *testing.T) {
	t.Run("LoadsValidYAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "test.yml")
		yamlContent := `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
  - name: another-task
    commands:
      - command: git.get_project
buildvariants:
  - name: ubuntu
    tasks:
      - name: test-task
`
		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		project, err := executor.LoadProject(yamlFile)
		require.NoError(t, err)
		assert.NotNil(t, project)
		assert.Len(t, project.Tasks, 2)
		assert.Equal(t, "test-task", project.Tasks[0].Name)
		assert.Equal(t, "another-task", project.Tasks[1].Name)
		assert.Len(t, project.BuildVariants, 1)
		assert.Equal(t, "ubuntu", project.BuildVariants[0].Name)
	})
}

func TestPrepareTask(t *testing.T) {
	t.Run("PreparesExistingTask", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "test.yml")
		yamlContent := `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
      - command: shell.exec
        params:
          script: echo "another"
`
		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)

		err = executor.PrepareTask("test-task")
		require.NoError(t, err)
		assert.Equal(t, "test-task", executor.debugState.SelectedTask)
		assert.Greater(t, len(executor.debugState.CommandList), 0)
		assert.Len(t, executor.commandBlocks, 1)
	})

	t.Run("ReturnsErrorForNonexistentTask", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "test.yml")
		yamlContent := `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
`
		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)

		err = executor.PrepareTask("nonexistent-task")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task 'nonexistent-task' not found")
	})

	t.Run("ReturnsErrorWhenProjectNotLoaded", func(t *testing.T) {
		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		err = executor.PrepareTask("any-task")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "project not loaded")
	})
}

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
		require.NoError(t, os.WriteFile(filepath.Join(logDir, "output.log"), []byte(content), 0644))

		lines, err := readLogFileLines(filepath.Join(logDir, "output.log"), LogFilterOptions{
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
}
