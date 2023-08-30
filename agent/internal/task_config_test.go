package internal

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskConfigGetWorkingDirectory(t *testing.T) {
	curdir := testutil.GetDirectoryOfFile()

	conf := &TaskConfig{
		WorkDir: curdir,
	}

	// make sure that we fall back to the configured working directory
	out, err := conf.GetWorkingDirectory("")
	assert.NoError(t, err)
	assert.Equal(t, conf.WorkDir, out)

	// check for a directory that we know exists
	out, err = conf.GetWorkingDirectory("testutil")
	require.NoError(t, err)
	assert.Equal(t, out, filepath.Join(curdir, "testutil"))

	// check for a file not a directory
	out, err = conf.GetWorkingDirectory("task_config.go")
	assert.Error(t, err)
	assert.Equal(t, "", out)

	// presumably for a directory that doesn't exist
	out, err = conf.GetWorkingDirectory("does-not-exist")
	assert.Error(t, err)
	assert.Equal(t, "", out)
}

func TestTaskConfigGetTaskGroup(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.VersionCollection))
	tgName := "example_task_group"
	projYml := `
timeout:
  - command: shell.exec
    params:
      script: "echo timeout"
tasks:
- name: example_task_1
- name: example_task_2
task_groups:
- name: example_task_group
  max_hosts: 2
  setup_group:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_group:
  - command: shell.exec
    params:
      script: "echo teardown_group"
  setup_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  tasks:
  - example_task_1
  - example_task_2
`
	p := &model.Project{}
	ctx := context.Background()
	_, err := model.LoadProjectInto(ctx, []byte(projYml), nil, "", p)
	require.NoError(t, err)
	v := model.Version{
		Id: "v1",
	}
	t1 := task.Task{
		Id:        "t1",
		TaskGroup: tgName,
		Version:   v.Id,
	}

	tc := TaskConfig{Task: &t1, Project: p}
	tg, err := tc.GetTaskGroup(tgName)
	assert.NoError(t, err)
	assert.Equal(t, tgName, tg.Name)
	assert.Len(t, tg.Tasks, 2)
	assert.Equal(t, 2, tg.MaxHosts)
}

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

	taskConfig, err := NewTaskConfig(curdir, &apimodels.DistroView{}, p, task, &model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}, &patch.Patch{}, util.Expansions{})
	assert.NoError(t, err)

	assert.Equal(t, util.Expansions{}, taskConfig.DynamicExpansions)
	assert.Equal(t, &util.Expansions{}, taskConfig.Expansions)
	assert.Equal(t, &apimodels.DistroView{}, taskConfig.Distro)
	assert.Equal(t, p, taskConfig.Project)
	assert.Equal(t, task, taskConfig.Task)
}
