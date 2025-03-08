package internal

import (
	"testing"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
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
