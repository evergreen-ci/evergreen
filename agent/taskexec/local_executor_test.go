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

func TestLoadProjectWithIncludes(t *testing.T) {
	t.Run("ResolvesLocalIncludes", func(t *testing.T) {
		tmpDir := t.TempDir()

		includeContent := `
tasks:
  - name: included-task
    commands:
      - command: shell.exec
        params:
          script: echo "from include"
`
		err := os.WriteFile(filepath.Join(tmpDir, "extra.yml"), []byte(includeContent), 0644)
		require.NoError(t, err)

		mainContent := `
include:
  - filename: extra.yml
tasks:
  - name: main-task
    commands:
      - command: shell.exec
        params:
          script: echo "main"
buildvariants:
  - name: ubuntu
    tasks:
      - name: main-task
`
		mainFile := filepath.Join(tmpDir, "main.yml")
		err = os.WriteFile(mainFile, []byte(mainContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		project, err := executor.LoadProject(mainFile)
		require.NoError(t, err)
		assert.NotNil(t, project)
		assert.Len(t, project.Tasks, 2)

		taskNames := []string{project.Tasks[0].Name, project.Tasks[1].Name}
		assert.Contains(t, taskNames, "main-task")
		assert.Contains(t, taskNames, "included-task")
	})

	t.Run("ResolvesSubdirectoryIncludes", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0755))

		includeContent := `
tasks:
  - name: sub-task
    commands:
      - command: shell.exec
        params:
          script: echo "from subdir"
`
		err := os.WriteFile(filepath.Join(subDir, "extra.yml"), []byte(includeContent), 0644)
		require.NoError(t, err)

		mainContent := `
include:
  - filename: subdir/extra.yml
tasks:
  - name: main-task
    commands:
      - command: shell.exec
        params:
          script: echo "main"
`
		mainFile := filepath.Join(tmpDir, "main.yml")
		err = os.WriteFile(mainFile, []byte(mainContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		project, err := executor.LoadProject(mainFile)
		require.NoError(t, err)
		assert.NotNil(t, project)
		assert.Len(t, project.Tasks, 2)

		taskNames := []string{project.Tasks[0].Name, project.Tasks[1].Name}
		assert.Contains(t, taskNames, "main-task")
		assert.Contains(t, taskNames, "sub-task")
	})

	t.Run("ErrorsOnMissingInclude", func(t *testing.T) {
		tmpDir := t.TempDir()

		mainContent := `
include:
  - filename: nonexistent.yml
tasks:
  - name: main-task
    commands:
      - command: shell.exec
        params:
          script: echo "main"
`
		mainFile := filepath.Join(tmpDir, "main.yml")
		err := os.WriteFile(mainFile, []byte(mainContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(mainFile)
		assert.Error(t, err)
	})
}

func TestLoadProjectWithModuleIncludes(t *testing.T) {
	t.Run("ModuleIncludeWithLocalModules", func(t *testing.T) {
		tmpDir := t.TempDir()
		moduleDir := filepath.Join(tmpDir, "mymodule")
		require.NoError(t, os.MkdirAll(moduleDir, 0755))

		moduleContent := `
tasks:
  - name: module-task
    commands:
      - command: shell.exec
        params:
          script: echo "from module"
`
		err := os.WriteFile(filepath.Join(moduleDir, "module.yml"), []byte(moduleContent), 0644)
		require.NoError(t, err)

		mainContent := `
modules:
  - name: mymod
    repo: git@github.com:test/test.git
    branch: main
include:
  - filename: module.yml
    module: mymod
tasks:
  - name: main-task
    commands:
      - command: shell.exec
        params:
          script: echo "main"
`
		mainFile := filepath.Join(tmpDir, "main.yml")
		err = os.WriteFile(mainFile, []byte(mainContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{
			LocalModules: map[string]string{
				"mymod": moduleDir,
			},
		})
		require.NoError(t, err)

		project, err := executor.LoadProject(mainFile)
		require.NoError(t, err)
		assert.NotNil(t, project)
		assert.Len(t, project.Tasks, 2)

		taskNames := []string{project.Tasks[0].Name, project.Tasks[1].Name}
		assert.Contains(t, taskNames, "main-task")
		assert.Contains(t, taskNames, "module-task")
	})

	t.Run("ModuleIncludeWithoutLocalModulesErrors", func(t *testing.T) {
		tmpDir := t.TempDir()

		mainContent := `
modules:
  - name: mymod
    repo: git@github.com:test/test.git
    branch: main
include:
  - filename: module.yml
    module: mymod
tasks:
  - name: main-task
    commands:
      - command: shell.exec
        params:
          script: echo "main"
`
		mainFile := filepath.Join(tmpDir, "main.yml")
		err := os.WriteFile(mainFile, []byte(mainContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(mainFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "local path for module 'mymod' is unspecified")
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
