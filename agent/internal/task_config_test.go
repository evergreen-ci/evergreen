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

	taskConfig, err := NewTaskConfig(curdir, &apimodels.DistroView{}, p, task, &model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}, &patch.Patch{}, &apimodels.ExpansionsAndVars{
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
	})
	assert.NoError(t, err)

	assert.Empty(t, taskConfig.DynamicExpansions)
	assert.Empty(t, taskConfig.Expansions)
	assert.ElementsMatch(t, []string{"aws_token", "my_pass_secret", "myPASSWORD", "mySecret", "git_token"}, taskConfig.Redacted)
	assert.Equal(t, &apimodels.DistroView{}, taskConfig.Distro)
	assert.Equal(t, p, &taskConfig.Project)
	assert.Equal(t, task, &taskConfig.Task)
}

func TestCreatesCheckRun(t *testing.T) {
	task := &task.Task{
		DisplayName:  "some_task",
		BuildVariant: "bv",
	}

	p := &model.Project{
		BuildVariants: []model.BuildVariant{
			{
				Name: "bv",
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name: "some_task",
						CreateCheckRun: &model.CheckRun{
							PathToOutputs: "",
						},
					},
				},
			},
		},
	}

	tc, err := NewTaskConfig(testutil.GetDirectoryOfFile(), &apimodels.DistroView{}, p, task, &model.ProjectRef{}, &patch.Patch{}, &apimodels.ExpansionsAndVars{})
	assert.NoError(t, err)
	assert.Equal(t, true, tc.createsCheckRun())
}
