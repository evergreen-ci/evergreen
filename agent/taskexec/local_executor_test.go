package taskexec

import (
	"os"
	"path/filepath"
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
