package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestCreateConfig(t *testing.T) {
	assert.NoError(t, db.ClearCollections(ProjectConfigCollection))
	projYml := `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
`
	pc, err := CreateProjectConfig([]byte(projYml), "")
	assert.Nil(t, err)
	assert.Nil(t, pc)

	projYml = `
task_groups:
- name: task_group_name
  setup_task:
  - command: shell.exec
    params:
      script: "echo hi"
build_baron_settings:
  ticket_create_project: BF
  ticket_search_projects: ["BF"]

github_pr_aliases:
  - variant: "^ubuntu1604$"
    task: ".*"

`
	pc, err = CreateProjectConfig([]byte(projYml), "")
	assert.Nil(t, err)
	assert.NotNil(t, pc)
	assert.Equal(t, "BF", pc.BuildBaronSettings.TicketCreateProject)
}
