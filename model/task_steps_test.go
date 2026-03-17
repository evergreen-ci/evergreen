package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskExecutionSteps(t *testing.T) {
	t.Run("SimpleMainBlockCommands", func(t *testing.T) {
		project := &Project{
			Tasks: []ProjectTask{
				{
					Name: "my_task",
					Commands: []PluginCommandConf{
						{Command: "shell.exec"},
						{Command: "s3.put", DisplayName: "upload artifacts"},
					},
				},
			},
		}

		steps, err := GetTaskExecutionSteps(project, "my_task")
		require.NoError(t, err)
		require.Len(t, steps, 2)

		assert.Equal(t, "1", steps[0].StepNumber)
		assert.Equal(t, "shell.exec", steps[0].CommandName)
		assert.Equal(t, "'shell.exec' (step 1 of 2)", steps[0].DisplayName)
		assert.False(t, steps[0].IsFunction)
		assert.Empty(t, steps[0].BlockType)

		assert.Equal(t, "2", steps[1].StepNumber)
		assert.Equal(t, "s3.put", steps[1].CommandName)
		assert.Equal(t, "'s3.put' ('upload artifacts') (step 2 of 2)", steps[1].DisplayName)
	})

	t.Run("PreMainPostBlocks", func(t *testing.T) {
		project := &Project{
			Pre: &YAMLCommandSet{
				SingleCommand: &PluginCommandConf{Command: "shell.exec", DisplayName: "setup"},
			},
			Post: &YAMLCommandSet{
				SingleCommand: &PluginCommandConf{Command: "attach.results"},
			},
			Tasks: []ProjectTask{
				{
					Name: "my_task",
					Commands: []PluginCommandConf{
						{Command: "shell.exec"},
					},
				},
			},
		}

		steps, err := GetTaskExecutionSteps(project, "my_task")
		require.NoError(t, err)
		require.Len(t, steps, 3)

		assert.Equal(t, "pre:1", steps[0].StepNumber)
		assert.Equal(t, "pre", steps[0].BlockType)
		assert.Contains(t, steps[0].DisplayName, "in block 'pre'")

		assert.Equal(t, "1", steps[1].StepNumber)
		assert.Empty(t, steps[1].BlockType)

		assert.Equal(t, "post:1", steps[2].StepNumber)
		assert.Equal(t, "post", steps[2].BlockType)
		assert.Contains(t, steps[2].DisplayName, "in block 'post'")
	})

	t.Run("FunctionExpansion", func(t *testing.T) {
		project := &Project{
			Functions: map[string]*YAMLCommandSet{
				"my_func": {
					MultiCommand: []PluginCommandConf{
						{Command: "shell.exec", DisplayName: "step one"},
						{Command: "shell.exec", DisplayName: "step two"},
						{Command: "s3.put"},
					},
				},
			},
			Tasks: []ProjectTask{
				{
					Name: "my_task",
					Commands: []PluginCommandConf{
						{Function: "my_func"},
						{Command: "shell.exec"},
					},
				},
			},
		}

		steps, err := GetTaskExecutionSteps(project, "my_task")
		require.NoError(t, err)
		require.Len(t, steps, 4)

		assert.Equal(t, "1.1", steps[0].StepNumber)
		assert.Equal(t, "shell.exec", steps[0].CommandName)
		assert.True(t, steps[0].IsFunction)
		assert.Equal(t, "my_func", steps[0].FunctionName)
		assert.Contains(t, steps[0].DisplayName, "in function 'my_func'")
		assert.Contains(t, steps[0].DisplayName, "step 1.1 of 2")

		assert.Equal(t, "1.2", steps[1].StepNumber)
		assert.Equal(t, "1.3", steps[2].StepNumber)

		assert.Equal(t, "2", steps[3].StepNumber)
		assert.False(t, steps[3].IsFunction)
	})

	t.Run("FunctionInPreBlock", func(t *testing.T) {
		project := &Project{
			Pre: &YAMLCommandSet{
				MultiCommand: []PluginCommandConf{
					{Function: "setup_func"},
				},
			},
			Functions: map[string]*YAMLCommandSet{
				"setup_func": {
					MultiCommand: []PluginCommandConf{
						{Command: "shell.exec"},
						{Command: "shell.exec"},
					},
				},
			},
			Tasks: []ProjectTask{
				{
					Name:     "my_task",
					Commands: []PluginCommandConf{{Command: "shell.exec"}},
				},
			},
		}

		steps, err := GetTaskExecutionSteps(project, "my_task")
		require.NoError(t, err)
		require.Len(t, steps, 3)

		assert.Equal(t, "pre:1.1", steps[0].StepNumber)
		assert.Equal(t, "pre", steps[0].BlockType)
		assert.Equal(t, "pre:1.2", steps[1].StepNumber)
		assert.Equal(t, "1", steps[2].StepNumber)
	})

	t.Run("NonexistentTask", func(t *testing.T) {
		project := &Project{
			Tasks: []ProjectTask{
				{Name: "existing_task"},
			},
		}

		_, err := GetTaskExecutionSteps(project, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("SingleSubCommandFunctionNoSubNumber", func(t *testing.T) {
		project := &Project{
			Functions: map[string]*YAMLCommandSet{
				"single_func": {
					SingleCommand: &PluginCommandConf{Command: "shell.exec"},
				},
			},
			Tasks: []ProjectTask{
				{
					Name: "my_task",
					Commands: []PluginCommandConf{
						{Function: "single_func"},
					},
				},
			},
		}

		steps, err := GetTaskExecutionSteps(project, "my_task")
		require.NoError(t, err)
		require.Len(t, steps, 1)

		assert.Equal(t, "1", steps[0].StepNumber)
		assert.True(t, steps[0].IsFunction)
	})
}
