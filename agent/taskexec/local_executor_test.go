package taskexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/command"
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

		err = executor.PrepareTask(t.Context(), "test-task", "")
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

		err = executor.PrepareTask(t.Context(), "nonexistent-task", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task 'nonexistent-task' not found")
	})

	t.Run("ReturnsErrorWhenProjectNotLoaded", func(t *testing.T) {
		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		err = executor.PrepareTask(t.Context(), "any-task", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "project not loaded")
	})

	t.Run("PreparesTaskWithVariantExpansions", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "test.yml")
		yamlContent := `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
buildvariants:
  - name: ubuntu2204
    display_name: Ubuntu 22.04
    expansions:
      distro_id: ubuntu2204
      edition: enterprise
    tasks:
      - name: test-task
`
		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)

		err = executor.PrepareTask(t.Context(), "test-task", "ubuntu2204")
		require.NoError(t, err)
		assert.Equal(t, "test-task", executor.debugState.SelectedTask)
		assert.Equal(t, "ubuntu2204", executor.debugState.SelectedVariant)
		assert.Equal(t, "ubuntu2204", executor.taskConfig.Expansions.Get("build_variant"))
		assert.Equal(t, "ubuntu2204", executor.taskConfig.Expansions.Get("distro_id"))
		assert.Equal(t, "enterprise", executor.taskConfig.Expansions.Get("edition"))
	})

	t.Run("ReturnsErrorForNonexistentVariant", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "test.yml")
		yamlContent := `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
buildvariants:
  - name: ubuntu2204
    tasks:
      - name: test-task
`
		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)

		err = executor.PrepareTask(t.Context(), "test-task", "nonexistent-variant")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in project")
	})

	t.Run("ReturnsErrorForTaskNotOnVariant", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "test.yml")
		yamlContent := `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
  - name: other-task
    commands:
      - command: shell.exec
        params:
          script: echo "other"
buildvariants:
  - name: ubuntu2204
    tasks:
      - name: other-task
`
		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)

		err = executor.PrepareTask(t.Context(), "test-task", "ubuntu2204")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not defined on build variant")
	})
}

func TestCommandInfoStepNumber(t *testing.T) {
	t.Run("RegularCommand", func(t *testing.T) {
		ci := CommandInfo{BlockCmdNum: 3, BlockTotalCmds: 5}
		assert.Equal(t, "3", ci.stepNumber())
	})

	t.Run("FunctionSingleSubCmd", func(t *testing.T) {
		ci := CommandInfo{BlockCmdNum: 2, FuncSubCmdNum: 1, FuncTotalSubCmds: 1}
		assert.Equal(t, "2", ci.stepNumber())
	})

	t.Run("FunctionMultipleSubCmds", func(t *testing.T) {
		ci := CommandInfo{BlockCmdNum: 2, FuncSubCmdNum: 3, FuncTotalSubCmds: 4}
		assert.Equal(t, "2.3", ci.stepNumber())
	})
}

func TestCommandInfoFullStepNumber(t *testing.T) {
	t.Run("MainBlock", func(t *testing.T) {
		ci := CommandInfo{BlockCmdNum: 5, BlockType: command.MainTaskBlock}
		assert.Equal(t, "5", ci.FullStepNumber())
	})

	t.Run("PreBlock", func(t *testing.T) {
		ci := CommandInfo{BlockCmdNum: 1, BlockType: command.PreBlock}
		assert.Equal(t, "pre:1", ci.FullStepNumber())
	})

	t.Run("PostBlockWithSubCmd", func(t *testing.T) {
		ci := CommandInfo{BlockCmdNum: 2, BlockType: command.PostBlock, FuncSubCmdNum: 1, FuncTotalSubCmds: 3}
		assert.Equal(t, "post:2.1", ci.FullStepNumber())
	})

	t.Run("EmptyBlockType", func(t *testing.T) {
		ci := CommandInfo{BlockCmdNum: 3}
		assert.Equal(t, "3", ci.FullStepNumber())
	})
}

func TestResolveStepNumber(t *testing.T) {
	ds := &DebugState{
		CommandList: []CommandInfo{
			{Index: 0, BlockCmdNum: 1, BlockType: command.PreBlock, BlockTotalCmds: 2},
			{Index: 1, BlockCmdNum: 2, BlockType: command.PreBlock, BlockTotalCmds: 2, FuncSubCmdNum: 1, FuncTotalSubCmds: 2},
			{Index: 2, BlockCmdNum: 2, BlockType: command.PreBlock, BlockTotalCmds: 2, FuncSubCmdNum: 2, FuncTotalSubCmds: 2},
			{Index: 3, BlockCmdNum: 1, BlockType: command.MainTaskBlock, BlockTotalCmds: 3},
			{Index: 4, BlockCmdNum: 2, BlockType: command.MainTaskBlock, BlockTotalCmds: 3},
			{Index: 5, BlockCmdNum: 3, BlockType: command.MainTaskBlock, BlockTotalCmds: 3},
		},
	}

	t.Run("MainBlockStep", func(t *testing.T) {
		idx, err := ds.ResolveStepNumber("2")
		require.NoError(t, err)
		assert.Equal(t, 4, idx)
	})

	t.Run("PreBlockStep", func(t *testing.T) {
		idx, err := ds.ResolveStepNumber("pre:1")
		require.NoError(t, err)
		assert.Equal(t, 0, idx)
	})

	t.Run("PreBlockFuncSubCmd", func(t *testing.T) {
		idx, err := ds.ResolveStepNumber("pre:2.2")
		require.NoError(t, err)
		assert.Equal(t, 2, idx)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := ds.ResolveStepNumber("99")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestLogFile(t *testing.T) {
	t.Run("WriteAndRead", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")

		lf, err := newLogFile(path)
		require.NoError(t, err)
		defer lf.Close()

		lf.WriteLogLine("3", "hello world")
		lf.WriteLogLine("3", "second line")

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

		lf.WriteStepStart("5", "shell.exec", "main")
		lf.WriteLogLine("5", "command output")
		lf.WriteStepEnd("5", true, "1.2s")

		require.NoError(t, lf.Close())

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "=== STEP 5 START shell.exec (main) ===")
		assert.Contains(t, content, "=== STEP 5 END success=true duration=1.2s ===")
	})
}

func TestReloadProject(t *testing.T) {
	makeYAML := func(t *testing.T, dir, content string) string {
		t.Helper()
		path := filepath.Join(dir, "test.yml")
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		return path
	}

	t.Run("PreservesStepIndexAndState", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := makeYAML(t, tmpDir, `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "step1"
      - command: shell.exec
        params:
          script: echo "step2"
      - command: shell.exec
        params:
          script: echo "step3"
`)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)
		require.NoError(t, executor.PrepareTask(t.Context(), "test-task", ""))

		executor.debugState.CurrentStepIndex = 2
		executor.debugState.CustomVars["myvar"] = "myval"
		executor.debugState.ExecutionHistory = []executionRecord{
			{stepIndex: 0, success: true},
			{stepIndex: 1, success: true},
		}

		project, err := executor.ReloadProject(t.Context(), yamlFile)
		require.NoError(t, err)
		assert.NotNil(t, project)

		assert.Equal(t, 2, executor.debugState.CurrentStepIndex)
		assert.Equal(t, "myval", executor.debugState.CustomVars["myvar"])
		assert.Len(t, executor.debugState.ExecutionHistory, 2)
		assert.Equal(t, "test-task", executor.debugState.SelectedTask)
	})

	t.Run("ClampsStepIndexWhenFewerSteps", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := makeYAML(t, tmpDir, `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "step1"
      - command: shell.exec
        params:
          script: echo "step2"
      - command: shell.exec
        params:
          script: echo "step3"
`)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)
		require.NoError(t, executor.PrepareTask(t.Context(), "test-task", ""))

		executor.debugState.CurrentStepIndex = 2

		// Rewrite the file with fewer commands.
		makeYAML(t, tmpDir, `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "only-step"
`)

		project, err := executor.ReloadProject(t.Context(), yamlFile)
		require.NoError(t, err)
		assert.NotNil(t, project)

		expectedSteps := len(executor.debugState.CommandList)
		assert.Equal(t, expectedSteps, executor.debugState.CurrentStepIndex)
		assert.False(t, executor.debugState.HasMoreSteps())
	})

	t.Run("ReloadsWithNoSelectedTask", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := makeYAML(t, tmpDir, `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
`)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)

		project, err := executor.ReloadProject(t.Context(), yamlFile)
		require.NoError(t, err)
		assert.NotNil(t, project)
		assert.Equal(t, 0, executor.debugState.CurrentStepIndex)
		assert.Empty(t, executor.debugState.CommandList)
	})

	t.Run("RestoresCustomVarsToExpansions", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := makeYAML(t, tmpDir, `
tasks:
  - name: test-task
    commands:
      - command: shell.exec
        params:
          script: echo "test"
`)

		executor, err := NewLocalExecutor(t.Context(), LocalExecutorOptions{})
		require.NoError(t, err)

		_, err = executor.LoadProject(yamlFile)
		require.NoError(t, err)
		require.NoError(t, executor.PrepareTask(t.Context(), "test-task", ""))

		executor.SetVariable("custom_key", "custom_value")

		_, err = executor.ReloadProject(t.Context(), yamlFile)
		require.NoError(t, err)

		assert.Equal(t, "custom_value", executor.expansions.Get("custom_key"))
		assert.Equal(t, "custom_value", executor.debugState.CustomVars["custom_key"])
	})
}
