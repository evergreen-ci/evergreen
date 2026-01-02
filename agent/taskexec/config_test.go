package taskexec

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalTaskConfig(t *testing.T) {
	t.Run("ValidOptions", func(t *testing.T) {
		opts := LocalTaskConfigOptions{
			TaskName:    "test_task",
			VariantName: "test_variant",
			WorkDir:     "/test/dir",
			Expansions: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			TimeoutSecs: 3600,
			RedactKeys:  []string{"secret"},
		}

		config, err := NewLocalTaskConfig(opts)
		require.NoError(t, err)
		assert.NotNil(t, config)

		assert.Equal(t, "test_task", config.TaskName)
		assert.Equal(t, "/test/dir", config.WorkDir)
		assert.Equal(t, 3600, config.MaxExecTimeoutSecs)
		assert.Equal(t, 3600, config.Timeout.ExecTimeoutSecs)
		assert.Equal(t, []string{"secret"}, config.RedactKeys)

		expansions := config.GetExpansions()
		assert.NotNil(t, expansions)
		assert.Equal(t, "/test/dir", expansions.Get("workdir"))
		assert.Equal(t, "test_task", expansions.Get("task_name"))
		assert.Equal(t, "test_variant", expansions.Get("build_variant"))
		assert.Equal(t, "value1", expansions.Get("key1"))
		assert.Equal(t, "value2", expansions.Get("key2"))

		assert.NotNil(t, config.GetNewExpansions())
		assert.NotNil(t, config.InternalRedactions)

		assert.NotNil(t, config.ProjectVars)
		assert.NotNil(t, config.ModulePaths)
	})

	t.Run("MissingTaskName", func(t *testing.T) {
		opts := LocalTaskConfigOptions{
			WorkDir: "/test/dir",
		}

		config, err := NewLocalTaskConfig(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task name is required")
		assert.Nil(t, config)
	})

	t.Run("MissingWorkDir", func(t *testing.T) {
		opts := LocalTaskConfigOptions{
			TaskName: "test_task",
		}

		config, err := NewLocalTaskConfig(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "working directory is required")
		assert.Nil(t, config)
	})
}

func TestTaskConfigTimeouts(t *testing.T) {
	config := &TaskConfig{
		Timeout: Timeout{
			IdleTimeoutSecs: 100,
			ExecTimeoutSecs: 200,
		},
	}

	t.Run("SetIdleTimeout", func(t *testing.T) {
		config.SetIdleTimeout(300)
		assert.Equal(t, 300, config.GetIdleTimeout())
	})

	t.Run("SetExecTimeout", func(t *testing.T) {
		config.SetExecTimeout(400)
		assert.Equal(t, 400, config.GetExecTimeout())
	})
}

func TestTaskConfigProject(t *testing.T) {
	config := &TaskConfig{}

	t.Run("SetValidProject", func(t *testing.T) {
		project := &model.Project{
			Identifier:  "test_project",
			DisplayName: "Test Project",
		}

		err := config.SetProject(project)
		assert.NoError(t, err)
		assert.Equal(t, "test_project", config.Project.Identifier)
		assert.Equal(t, "Test Project", config.Project.DisplayName)
	})

	t.Run("SetNilProject", func(t *testing.T) {
		err := config.SetProject(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "project cannot be nil")
	})
}

func TestTaskConfigExpansions(t *testing.T) {
	opts := LocalTaskConfigOptions{
		TaskName: "test_task",
		WorkDir:  "/test/dir",
		Expansions: map[string]string{
			"initial_key": "initial_value",
		},
	}

	config, err := NewLocalTaskConfig(opts)
	require.NoError(t, err)

	t.Run("UpdateExpansions", func(t *testing.T) {
		updates := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}

		config.UpdateExpansions(updates)

		expansions := config.GetExpansions()
		assert.Equal(t, "value1", expansions.Get("key1"))
		assert.Equal(t, "value2", expansions.Get("key2"))
		assert.Equal(t, "initial_value", expansions.Get("initial_key"))
	})
}

func TestFindProjectTask(t *testing.T) {
	config := &TaskConfig{
		Project: model.Project{
			Tasks: []model.ProjectTask{
				{Name: "task1", Priority: 1},
				{Name: "task2", Priority: 2},
				{Name: "task3", Priority: 3},
			},
		},
	}

	t.Run("FindExistingTask", func(t *testing.T) {
		task := config.FindProjectTask("task2")
		require.NotNil(t, task)
		assert.Equal(t, "task2", task.Name)
		assert.Equal(t, int64(2), task.Priority)
	})

	t.Run("FindNonExistentTask", func(t *testing.T) {
		task := config.FindProjectTask("non_existent")
		assert.Nil(t, task)
	})
}

func TestGetTaskName(t *testing.T) {
	t.Run("WithTaskID", func(t *testing.T) {
		config := &TaskConfig{
			Task: task.Task{
				Id:          "task_id_123",
				DisplayName: "Display Task Name",
			},
			TaskName: "fallback_name",
		}

		name := config.GetTaskName()
		assert.Equal(t, "Display Task Name", name)
	})

	t.Run("WithoutTaskID", func(t *testing.T) {
		config := &TaskConfig{
			Task: task.Task{
				DisplayName: "",
			},
			TaskName: "config_task_name",
		}

		name := config.GetTaskName()
		assert.Equal(t, "config_task_name", name)
	})
}
