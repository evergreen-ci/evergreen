package internal

import (
	"testing"

	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTaskConfig(t *testing.T) {
	curdir := testutil.GetDirectoryOfFile()
	taskName := "some_task"
	bvName := "bv"
	p := &model.Project{
		Tasks: []model.ProjectTask{
			{
				Name: taskName,
			},
		},
		BuildVariants: []model.BuildVariant{{Name: bvName}},
	}
	task := &task.Task{
		Id:           "task_id",
		DisplayName:  taskName,
		BuildVariant: bvName,
		Version:      "v1",
	}

	d := &apimodels.DistroView{
		Mountpoints: []string{"/dev/sdb"},
	}
	h := &apimodels.HostView{
		Hostname: "hostname",
	}
	tcOpts := TaskConfigOptions{
		WorkDir: curdir,
		Distro:  d,
		Host:    h,
		Project: p,
		Task:    task,
		ProjectRef: &model.ProjectRef{
			Id:         "project_id",
			Identifier: "project_identifier",
		},
		Patch: &patch.Patch{},
		ExpansionsAndVars: &apimodels.ExpansionsAndVars{
			Vars: map[string]string{
				"num_hosts":      "",
				"aws_token":      "",
				"my_pass_secret": "",
				"myPASSWORD":     "",
				"mySecret":       "",
				"git_token":      "",
			},
			PrivateVars: map[string]bool{
				"aws_token": true,
			},
			RedactKeys: []string{"pass", "secret", "token"},
		},
	}
	taskConfig, err := NewTaskConfig(tcOpts)
	assert.NoError(t, err)

	assert.Empty(t, taskConfig.DynamicExpansions)
	assert.Empty(t, taskConfig.Expansions)
	assert.ElementsMatch(t, []string{"aws_token", "my_pass_secret", "myPASSWORD", "mySecret", "git_token"}, taskConfig.Redacted)
	assert.Equal(t, d, taskConfig.Distro)
	assert.Equal(t, h, taskConfig.Host)
	assert.Equal(t, p, &taskConfig.Project)
	assert.Equal(t, task, &taskConfig.Task)
}

func TestApplyFunctionVars(t *testing.T) {
	makeTaskConfig := func() *TaskConfig {
		expansions := util.Expansions{"existing_key": "original_value", "ref": "expanded_ref"}
		return &TaskConfig{
			Expansions:        expansions,
			NewExpansions:     agentutil.NewDynamicExpansions(expansions),
			DynamicExpansions: util.Expansions{},
		}
	}

	t.Run("CleanupRestoresPreviousValues", func(t *testing.T) {
		tc := makeTaskConfig()

		cleanup, err := tc.ApplyFunctionVarsToExpansions(map[string]string{
			"existing_key": "new_value",
		}, "shell.exec")
		require.NoError(t, err)

		assert.Equal(t, "new_value", tc.NewExpansions.Get("existing_key"))

		cleanup()
		assert.Equal(t, "original_value", tc.NewExpansions.Get("existing_key"))
	})

	t.Run("VarsWithExpansionReferencesAreExpanded", func(t *testing.T) {
		tc := makeTaskConfig()

		cleanup, err := tc.ApplyFunctionVarsToExpansions(map[string]string{
			"derived": "${ref}_suffix",
		}, "shell.exec")
		require.NoError(t, err)
		defer cleanup()

		assert.Equal(t, "expanded_ref_suffix", tc.NewExpansions.Get("derived"))
	})

	t.Run("ExpansionsUpdatePreservesDynamicallyUpdatedKeys", func(t *testing.T) {
		tc := makeTaskConfig()

		tc.DynamicExpansions.Put("existing_key", "dynamic_value")

		cleanup, err := tc.ApplyFunctionVarsToExpansions(map[string]string{
			"existing_key": "func_value",
		}, "expansions.update")
		require.NoError(t, err)

		cleanup()
		// The key was dynamically updated, so it should not be restored.
		assert.Equal(t, "func_value", tc.NewExpansions.Get("existing_key"))
		assert.Empty(t, tc.DynamicExpansions)
	})

	t.Run("NonExpansionsUpdateResetsDynamicExpansions", func(t *testing.T) {
		tc := makeTaskConfig()
		tc.DynamicExpansions.Put("some_key", "some_val")

		cleanup, err := tc.ApplyFunctionVarsToExpansions(map[string]string{
			"existing_key": "temp",
		}, "shell.exec")
		require.NoError(t, err)

		cleanup()
		assert.Empty(t, tc.DynamicExpansions)
	})

	t.Run("EmptyVarsMapProducesSafeCleanup", func(t *testing.T) {
		tc := makeTaskConfig()

		cleanup, err := tc.ApplyFunctionVarsToExpansions(map[string]string{}, "shell.exec")
		require.NoError(t, err)

		assert.Equal(t, "original_value", tc.NewExpansions.Get("existing_key"))
		cleanup()
		assert.Equal(t, "original_value", tc.NewExpansions.Get("existing_key"))
	})
}
