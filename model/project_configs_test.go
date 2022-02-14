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
	pc, err := createProjectConfig([]byte(projYml))
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

commit_queue_aliases:
  - project_id: evg
    variant: ubuntu1604

`
	pc, err = createProjectConfig([]byte(projYml))
	assert.Nil(t, err)
	assert.NotNil(t, pc)
	assert.Equal(t, "BF", pc.BuildBaronSettings.TicketCreateProject)
}
